package maps

import (
	"math"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// segmentBlocked reports whether a map box cuts the segment a-b (slab test).
func segmentBlocked(m c.Map, a, b [3]float64) bool {
	for _, box := range m.Boxes {
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

// spawnArea groups the buy zones that hold a team's spawns into overlapping
// clusters and returns how many clusters there are, and the spawns outside
// every buy zone.
func spawnAreas(m c.Map, team string) (clusters int, outside [][3]float64) {
	zones := m.BuyZones[team]
	used := map[int]bool{}
	for _, s := range m.TeamSpawns[team] {
		in := false
		for i, z := range zones {
			if z.Contains(s) {
				used[i], in = true, true
			}
		}
		if !in {
			outside = append(outside, s)
		}
	}
	overlap := func(a, b c.Zone) bool {
		for k := range 3 {
			if a.Max[k] < b.Min[k] || b.Max[k] < a.Min[k] {
				return false
			}
		}
		return true
	}
	seen := map[int]bool{}
	for i := range used {
		if seen[i] {
			continue
		}
		clusters++
		stack := []int{i}
		seen[i] = true
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for j := range used {
				if !seen[j] && overlap(zones[cur], zones[j]) {
					seen[j] = true
					stack = append(stack, j)
				}
			}
		}
	}
	return clusters, outside
}

// coverChecked are the defusal maps whose spawns must pass TestSpawnsCovered.
// Add a map here once its spawns are behind cover.
var coverChecked = map[string]bool{"de_slop": true}

// Each team starts in one area, inside its buy zone, behind cover: no spawn
// point sees an enemy spawn point (eye to eye or eye to chest, standing).
// Spawns are handed out in turn (Server.spawnFor), so checking every point
// covers every team size; a team needs one point per player up to half the
// map's MaxPlayers so nobody spawns inside a teammate.
func TestSpawnsCovered(t *testing.T) {
	for _, name := range Order {
		m := All[name]()
		if m.Mode != c.ModeDefusal || !coverChecked[name] {
			continue
		}
		for _, team := range c.Teams {
			sp := m.TeamSpawns[team]
			if want := m.MaxPlayers / 2; len(sp) < want {
				t.Errorf("%s: %s has %d spawns, want %d (MaxPlayers %d)", name, team, len(sp), want, m.MaxPlayers)
			}
			n, outside := spawnAreas(m, team)
			if len(outside) > 0 {
				t.Errorf("%s: %s spawns outside its buy zones: %v", name, team, outside)
			}
			if n != 1 {
				t.Errorf("%s: %s spawns are split over %d separate buy areas, want 1", name, team, n)
			}
		}
		for _, s := range m.TeamSpawns[c.Vibe] {
			for _, e := range m.TeamSpawns[c.Artisan] {
				eyeS := [3]float64{s[0], s[1] + sim.Eye, s[2]}
				eyeE := [3]float64{e[0], e[1] + sim.Eye, e[2]}
				chestS := [3]float64{s[0], s[1] + 1.2, s[2]}
				chestE := [3]float64{e[0], e[1] + 1.2, e[2]}
				if !segmentBlocked(m, eyeS, eyeE) || !segmentBlocked(m, eyeS, chestE) || !segmentBlocked(m, eyeE, chestS) {
					t.Errorf("%s: vibe spawn %v and artisan spawn %v see each other", name, s, e)
				}
			}
		}
	}
}
