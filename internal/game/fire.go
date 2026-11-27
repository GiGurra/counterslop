package game

import (
	"encoding/json"
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// Fire grenades (GitHub #11): Hot Reload (Vibe Coders) and Burndown Chart
// (Artisans), CS's Molotov and incendiary. Thrown like any grenade; it
// bursts where it first comes down on a floor, or after grenade.fuse s in
// the air (over a drop: the flames land on the floor below, within
// fireDropMax, else it fizzles). The pool is a set of flame points on a
// fireGrid over the floor within grenade.radius of the burst: each one needs
// a clear line from the burst at knee height (walls and crates stop the
// spread) and a floor under it within fireStepMax (it runs down a step, not a
// ledge). It burns grenade.duration s; a body whose feet are within
// flameReach of a flame takes grenade.dps damage per second (dealt in
// fireChunk pieces), its thrower's included, teammates only under
// mp_friendlyfire 1, armor or not. A Smoke Test that pops over a fire puts
// out the flames under the cloud (and a fire that bursts in a standing
// smoke is out at once): an "out" event; the pool ends when its last flame
// is out.

const (
	fireGrid     = 0.7  // metres between flame points
	fireKnee     = 0.4  // the spread's sight line from the burst, above the floor
	fireStepMax  = 1.1  // a flame runs down at most this far below the burst's floor ...
	fireStepUp   = 0.45 // ... and up a step this high
	fireDropMax  = 4.0  // an airburst: the floor must be this close below
	flameReach   = 0.55 // a body's feet this close (across) to a flame burn
	flameHeight  = 1.2  // ... and at most this far above it (a jump over the edge still burns)
	fireChunk    = 8.0  // damage is dealt in pieces of this much (CS's ~8 per quarter second)
	smokeOutMult = 1.0  // a smoke puts out flames within its radius × this
)

// fireDPS is a fire grenade's damage per second (weapons.json damage).
func fireDPS(w *Weapon) float64 { return w.Damage }

// ignite bursts fire grenade n into a pool (or fizzles: no floor below, or
// it burst inside a smoke). Returns whether the pool stands.
func (s *Server) ignite(n *Nade) bool {
	g := n.w.Grenade
	floor, ok := s.fireFloor(Vec3{n.Pos[0], n.Pos[1] + 0.05, n.Pos[2]}, fireDropMax)
	if !ok {
		s.sendAll(map[string]any{"t": "nade", "ev": "out", "id": n.ID, "w": n.Weapon, "p": n.Pos})
		return false
	}
	c := Vec3{n.Pos[0], floor, n.Pos[2]}
	n.Pos, n.Vel, n.resting = c, Vec3{}, true
	n.Flames = s.flamePoints(c, g.Radius)
	for _, sm := range s.nades {
		if sm.State == "smoke" {
			n.Flames = smokeOut(n.Flames, sm)
		}
	}
	if len(n.Flames) == 0 {
		s.sendAll(map[string]any{"t": "nade", "ev": "out", "id": n.ID, "w": n.Weapon, "p": c})
		return false
	}
	n.State, n.Left = "fire", g.Duration
	s.sendAll(map[string]any{"t": "nade", "ev": "fire", "id": n.ID, "w": n.Weapon, "p": c})
	return true
}

// fireFloor is the height of the first box top under p within max metres.
func (s *Server) fireFloor(p Vec3, max float64) (float64, bool) {
	d := mapDistance(&s.Map, p, Vec3{0, -1, 0})
	if d > max {
		return 0, false
	}
	return p[1] - d, true
}

// flamePoints lays a pool's flames on the floor around c (the burst's floor point).
func (s *Server) flamePoints(c Vec3, radius float64) []Vec3 {
	var out []Vec3
	n := int(math.Floor(radius / fireGrid))
	knee := Vec3{c[0], c[1] + fireKnee, c[2]}
	for i := -n; i <= n; i++ {
		for j := -n; j <= n; j++ {
			x, z := c[0]+float64(i)*fireGrid, c[2]+float64(j)*fireGrid
			if math.Hypot(x-c[0], z-c[2]) > radius {
				continue
			}
			top := Vec3{x, c[1] + fireStepUp + fireKnee, z} // over a step up
			if !s.lineClear(knee, Vec3{x, c[1] + fireKnee, z}) && !s.lineClear(knee, top) {
				continue // a wall in the way
			}
			y, ok := s.fireFloor(top, fireStepUp+fireKnee+fireStepMax)
			if !ok || s.nadeSolid(Vec3{x, y + 0.1, z}) {
				continue
			}
			out = append(out, Vec3{mm(x), mm(y), mm(z)})
		}
	}
	return out
}

// smokeOut drops the flames under smoke cloud sm.
func smokeOut(fl []Vec3, sm *Nade) []Vec3 {
	r := sm.w.Grenade.Radius * smokeOutMult
	kept := fl[:0]
	for _, f := range fl {
		if math.Hypot(f[0]-sm.Pos[0], f[2]-sm.Pos[2]) < r && math.Abs(f[1]-sm.Pos[1]) < r {
			continue
		}
		kept = append(kept, f)
	}
	return kept
}

// smokeOutFires: smoke sm just popped: the fires under it lose those flames
// (an "out" event for each fire it touched; a pool with none left is gone).
func (s *Server) smokeOutFires(sm *Nade) {
	for _, f := range s.nades {
		if f.State != "fire" {
			continue
		}
		before := len(f.Flames)
		f.Flames = smokeOut(f.Flames, sm)
		if len(f.Flames) < before {
			if len(f.Flames) == 0 {
				f.Left = 0
			}
			s.sendAll(map[string]any{"t": "nade", "ev": "out", "id": f.ID, "w": f.Weapon, "p": sm.Pos, "left": len(f.Flames)})
		}
	}
}

// inFire reports whether a body with its feet at p stands in fire n.
func inFire(n *Nade, p Vec3) bool {
	for _, f := range n.Flames {
		if math.Hypot(p[0]-f[0], p[2]-f[2]) < flameReach && p[1] > f[1]-0.3 && p[1] < f[1]+flameHeight {
			return true
		}
	}
	return false
}

// burnTick burns for dt: the bodies in it take its damage. Returns whether it still burns.
func (s *Server) burnTick(n *Nade, dt float64) bool {
	n.Left -= dt
	if n.Left <= 0 || len(n.Flames) == 0 {
		return false
	}
	dps := fireDPS(n.w)
	for _, q := range s.ordered() {
		if q.Dead || q.Team == content.Spectator || !q.ready || q.Bot || !inFire(n, q.Pos) {
			continue // target practice bots never burn
		}
		if s.spared(n.by, q) {
			continue
		}
		q.burn += dps * dt
		if q.burn < fireChunk {
			continue
		}
		dmg := math.Floor(q.burn)
		q.burn -= dmg
		s.burnHurt(n, q, dmg)
	}
	return true
}

// burnHurt deals dmg of fire n to q: hurt, the thrower's hit marker, the kill.
func (s *Server) burnHurt(n *Nade, q *Player, dmg float64) {
	s.noteDamage(n.by, q, int(dmg), q.HP)
	q.HP -= int(dmg)
	from := 0
	if n.by != nil {
		from = n.by.ID
	}
	hurt, _ := json.Marshal(map[string]any{"t": "hurt", "dmg": dmg, "from": from, "hp": max(q.HP, 0), "fire": true})
	trySend(q, hurt)
	if n.by != nil && q != n.by {
		hm, _ := json.Marshal(hitMsg{T: "hit", ID: q.ID, Name: q.Name, Part: "body", Weapon: n.Weapon, Damage: dmg, At: Vec3{q.Pos[0], q.Pos[1] + 0.3, q.Pos[2]}})
		trySend(n.by, hm)
	}
	if q.HP <= 0 {
		s.killBy(n.by, q, n.Weapon, false)
	}
}
