package mapgen

import (
	"math"
	"slices"

	c "github.com/gigurra/counterslop/internal/content"
)

// Spawn layout: FillSpawns tops every team up to one spawn point per player
// (Meta's MaxPlayers / 2) inside the team's buy zones, the same area as its
// hand-placed spawns, when the map is built (Map). The new points:
//
//   - stand on the floor the team's spawns stand on (one of their heights),
//     with a standing player's hull (SpawnR, SpawnH) clear of every box and
//     every mover's swept volume;
//   - are SpawnGap apart from each other and from the hand-placed ones;
//   - fill outward from the middle of the hand-placed ones, on a SpawnStep grid;
//   - with cover, see no enemy spawn point (eye to eye, eye to chest, either
//     way), so a team still starts out of the other's sight.

// Player hull and eye heights the spawn layout keeps clear (internal/sim's R,
// H, Eye; mapgen can't import sim's world without a cycle in the map tests).
const (
	SpawnR     = 0.4
	SpawnH     = 1.8
	spawnEye   = 1.62
	spawnChest = 1.2
	// SpawnGap is the least distance (xz) between two spawn points: two hulls
	// (0.8) and a margin.
	SpawnGap  = 1.2
	SpawnStep = 0.5
)

type fill struct{ cover bool }

// FillSpawns asks Map to add spawn points until each team has MaxPlayers/2
// (see the package notes above). cover keeps the new points out of the
// enemy spawns' sight. Call it after the hand-placed spawns and buy zones.
func (b *Builder) FillSpawns(cover bool) *Builder {
	b.fill = &fill{cover: cover}
	return b
}

func (b *Builder) fillSpawns() {
	if b.fill == nil {
		return
	}
	want := b.m.MaxPlayers / 2
	for _, team := range c.Teams {
		enemy := c.Artisan
		if team == c.Artisan {
			enemy = c.Vibe
		}
		got := b.m.TeamSpawns[team]
		if len(got) == 0 || len(got) >= want {
			continue
		}
		add := SpawnPoints(b.m, b.m.BuyZones[team], got, want-len(got), func(p [3]float64) bool {
			return !b.fill.cover || !SeesAny(b.m.Boxes, p, b.m.TeamSpawns[enemy])
		})
		b.TeamSpawns(team, add...)
	}
}

// SpawnPoints lays out up to n new spawn points inside zones (see the package
// notes) around the existing points have, keeping only those keep accepts.
func SpawnPoints(m c.Map, zones []c.Zone, have [][3]float64, n int, keep func([3]float64) bool) [][3]float64 {
	if len(have) == 0 || n <= 0 {
		return nil
	}
	var mid [3]float64
	var levels []float64
	for _, s := range have {
		mid[0], mid[2] = mid[0]+s[0]/float64(len(have)), mid[2]+s[2]/float64(len(have))
		if !slices.Contains(levels, s[1]) {
			levels = append(levels, s[1])
		}
	}
	solids := slices.Clone(m.Boxes)
	for _, mv := range m.Movers {
		box := c.Box{Min: mv.Min, Max: mv.Max}
		for a := range 3 {
			box.Min[a] = min(box.Min[a], mv.Min[a]+mv.Move[a])
			box.Max[a] = max(box.Max[a], mv.Max[a]+mv.Move[a])
		}
		solids = append(solids, box)
	}
	type cand struct {
		p [3]float64
		d float64
	}
	var cands []cand
	for _, z := range zones {
		// whole multiples of SpawnStep, a hull's radius inside the zone
		i0, i1 := math.Ceil((z.Min[0]+SpawnR)/SpawnStep), math.Floor((z.Max[0]-SpawnR)/SpawnStep)
		k0, k1 := math.Ceil((z.Min[2]+SpawnR)/SpawnStep), math.Floor((z.Max[2]-SpawnR)/SpawnStep)
		for i := i0; i <= i1; i++ {
			for k := k0; k <= k1; k++ {
				x, zz := float64(i*SpawnStep), float64(k*SpawnStep)
				for _, y := range levels {
					p := [3]float64{x, y, zz}
					if z.Contains(p) && OnFloor(solids, p) && HullFree(solids, p) {
						cands = append(cands, cand{p, math.Hypot(x-mid[0], zz-mid[2])})
					}
				}
			}
		}
	}
	slices.SortStableFunc(cands, func(a, b cand) int {
		if a.d != b.d {
			if a.d < b.d {
				return -1
			}
			return 1
		}
		// ties: by x, then z (deterministic)
		if a.p[0] != b.p[0] {
			if a.p[0] < b.p[0] {
				return -1
			}
			return 1
		}
		if a.p[2] < b.p[2] {
			return -1
		}
		if a.p[2] > b.p[2] {
			return 1
		}
		return 0
	})
	taken := slices.Clone(have)
	var out [][3]float64
	for _, cd := range cands {
		if len(out) >= n {
			break
		}
		if spaced(taken, cd.p) && (keep == nil || keep(cd.p)) {
			taken = append(taken, cd.p)
			out = append(out, cd.p)
		}
	}
	return out
}

// spaced: p is SpawnGap (xz) from every point at its level.
func spaced(pts [][3]float64, p [3]float64) bool {
	for _, q := range pts {
		if math.Abs(q[1]-p[1]) < SpawnH && math.Hypot(q[0]-p[0], q[2]-p[2]) < SpawnGap {
			return false
		}
	}
	return true
}

// HullFree: a standing hull at feet p overlaps no box.
func HullFree(boxes []c.Box, p [3]float64) bool {
	const e = 1e-4
	for _, b := range boxes {
		if p[0]+SpawnR > b.Min[0]+e && p[0]-SpawnR < b.Max[0]-e &&
			p[1]+SpawnH > b.Min[1]+e && p[1] < b.Max[1]-e &&
			p[2]+SpawnR > b.Min[2]+e && p[2]-SpawnR < b.Max[2]-e {
			return false
		}
	}
	return true
}

// OnFloor: a box top is right under feet p (within 1 cm).
func OnFloor(boxes []c.Box, p [3]float64) bool {
	for _, b := range boxes {
		if math.Abs(b.Max[1]-p[1]) < 0.01 && p[0] >= b.Min[0] && p[0] <= b.Max[0] && p[2] >= b.Min[2] && p[2] <= b.Max[2] {
			return true
		}
	}
	return false
}

// SeesAny: a standing player at p and one at any of pts see each other (eye to
// eye, or either's eye to the other's chest, through no box).
func SeesAny(boxes []c.Box, p [3]float64, pts [][3]float64) bool {
	for _, q := range pts {
		eP, eQ := [3]float64{p[0], p[1] + spawnEye, p[2]}, [3]float64{q[0], q[1] + spawnEye, q[2]}
		cP, cQ := [3]float64{p[0], p[1] + spawnChest, p[2]}, [3]float64{q[0], q[1] + spawnChest, q[2]}
		if !Blocked(boxes, eP, eQ) || !Blocked(boxes, eP, cQ) || !Blocked(boxes, eQ, cP) {
			return true
		}
	}
	return false
}

// Blocked reports whether a box cuts the segment a-b (slab test).
func Blocked(boxes []c.Box, a, b [3]float64) bool {
	for _, box := range boxes {
		t0, t1 := 0.0, 1.0
		hit := true
		for k := range 3 {
			d := b[k] - a[k]
			if math.Abs(d) < 1e-12 {
				if a[k] < box.Min[k] || a[k] > box.Max[k] {
					hit = false
					break
				}
				continue
			}
			u, v := (box.Min[k]-a[k])/d, (box.Max[k]-a[k])/d
			if u > v {
				u, v = v, u
			}
			t0, t1 = math.Max(t0, u), math.Min(t1, v)
			if t0 > t1 {
				hit = false
				break
			}
		}
		if hit {
			return true
		}
	}
	return false
}
