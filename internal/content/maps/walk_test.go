package maps

import (
	"fmt"
	"math"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapcheck"
	"github.com/gigurra/counterslop/internal/sim"
)

// Every nav link can be walked both ways with the real movement code: the
// routes bots take that aren't the shortest to a site (lanes, rotations) work
// too. Competitive and GPU Drop maps (those are the ones with lanes) and the
// maps whose spawns are checked (coverChecked).
func TestWalkEveryNavLink(t *testing.T) {
	for _, name := range Order {
		m := All[name]()
		if m.Category != c.Competitive && m.Category != c.GPUDrop && !coverChecked[name] {
			continue
		}
		byName := map[string]c.NavNode{}
		for _, n := range m.Nav {
			byName[n.Name] = n
		}
		for _, n := range m.Nav {
			for _, l := range n.Links {
				for _, leg := range [][2]c.NavNode{{n, byName[l]}, {byName[l], n}} {
					if _, err := walk(m, leg[0].Pos, []c.NavNode{leg[1]}); err != nil {
						t.Errorf("%s: %s -> %s: %v", name, leg[0].Name, leg[1].Name, err)
					}
				}
			}
		}
	}
}

// siteRace lists the maps whose defenders must get to every site first: the
// slowest Artisan spawn reaches each site at least lead seconds before the
// fastest Vibe Coder spawn, running the nav route with the real movement code.
var siteRace = map[string]float64{"de_stack": 2}

func TestDefendersReachSitesFirst(t *testing.T) {
	for _, name := range Order {
		lead, ok := siteRace[name]
		if !ok {
			continue
		}
		m := All[name]()
		for _, site := range m.Sites {
			times := map[string][2]float64{} // team: fastest, slowest
			for _, team := range c.Teams {
				route := navPath(m, team, site)
				fast, slow := math.Inf(1), 0.0
				for _, sp := range m.TeamSpawns[team] {
					_, secs, err := walkTimed(m, sp, route)
					if err != nil {
						t.Fatalf("%s: %s -> %s: %v", name, team, site.Name, err)
					}
					fast, slow = math.Min(fast, secs), math.Max(slow, secs)
				}
				times[team] = [2]float64{fast, slow}
			}
			def, att := times[c.Artisan][1], times[c.Vibe][0]
			t.Logf("%s %s: defenders %.1f-%.1f s, attackers %.1f-%.1f s", name, site.Name, times[c.Artisan][0], def, att, times[c.Vibe][1])
			if def+lead > att {
				t.Errorf("%s %s: the last defender gets there in %.1f s, the first attacker in %.1f s: want a %.0f s lead", name, site.Name, def, att, lead)
			}
		}
	}
}

// A nav link that changes floors (stairs) can be walked from a little to
// either side of its start too: a bot counts a waypoint reached 0.7 m short
// and then keeps to the leg's line (pure pursuit, internal/game aimOnLeg), so a line along the stairs' edge leaves
// it walking beside them (de_stack's stack stairwell: a round's attackers
// walked up and down beside it until the clock ran out).
func TestWalkClimbingNavLinksOffStart(t *testing.T) {
	for _, name := range Order {
		m := All[name]()
		byName := map[string]c.NavNode{}
		for _, n := range m.Nav {
			byName[n.Name] = n
		}
		for _, n := range m.Nav {
			for _, l := range n.Links {
				for _, leg := range [][2]c.NavNode{{n, byName[l]}, {byName[l], n}} {
					a, b := leg[0].Pos, leg[1].Pos
					d := math.Hypot(b[0]-a[0], b[2]-a[2])
					if math.Abs(b[1]-a[1]) < 1 || d < 1e-9 {
						continue
					}
					for _, side := range []float64{-0.6, 0.6} {
						start := [3]float64{a[0] - (b[2]-a[2])/d*side, a[1], a[2] + (b[0]-a[0])/d*side}
						if err := walkOnLeg(m, start, a, leg[1]); err != nil {
							t.Errorf("%s: %s -> %s from %.1f m to the side: %v", name, leg[0].Name, leg[1].Name, side, err)
						}
					}
				}
			}
		}
	}
}

// walkOnLeg walks from start to the node to as a bot on a floor-changing leg
// does: aiming 1 m ahead of its projection on the line from -> to.
func walkOnLeg(m c.Map, start, from [3]float64, to c.NavNode) error {
	w := sim.NewWorld(&m)
	body := sim.NewBody(sim.Vec3(start))
	tick := 0
	step := func(in sim.Input) {
		tick++
		w.T = float64(tick) / 100
		w.Step(body, in, 0.01)
	}
	for range 50 {
		step(sim.Input{})
	}
	if math.Abs(body.Pos[1]-from[1]) > 0.3 {
		return nil // off the start's floor there (a node at a ledge's edge): no bot starts the leg from it
	}
	b := to.Pos
	lx, lz := b[0]-from[0], b[2]-from[2]
	l := math.Hypot(lx, lz)
	limit := int((l/sim.Run*1.6 + 1) / 0.01)
	for t := 0; ; t++ {
		if math.Hypot(b[0]-body.Pos[0], b[2]-body.Pos[2]) < 0.5 {
			break
		}
		if t > limit {
			return fmt.Errorf("stuck at %.2v on the way to %s %v", body.Pos, to.Name, to.Pos)
		}
		k := math.Min(l, ((body.Pos[0]-from[0])*lx+(body.Pos[2]-from[2])*lz)/l+1)
		dx, dz := from[0]+lx/l*k-body.Pos[0], from[2]+lz/l*k-body.Pos[2]
		body.Yaw = math.Atan2(-dx, -dz)
		step(sim.Input{F: 1})
	}
	if math.Abs(body.Pos[1]-b[1]) > 0.6 {
		return fmt.Errorf("reached %s at height %.2f, want %.2f", to.Name, body.Pos[1], b[1])
	}
	return nil
}

// The walk is mapcheck's (the checks a custom map loads with use it too).
var moveModels = mapcheck.MoveModels

func navPath(m c.Map, from string, zone c.Zone) []c.NavNode { return mapcheck.NavPath(m, from, zone) }
func walk(m c.Map, start [3]float64, route []c.NavNode) (sim.Vec3, error) {
	return mapcheck.Walk(m, start, route)
}
func walkIn(m c.Map, start [3]float64, route []c.NavNode, move sim.MoveRules) (sim.Vec3, error) {
	return mapcheck.WalkIn(m, start, route, move)
}
func walkTimed(m c.Map, start [3]float64, route []c.NavNode) (sim.Vec3, float64, error) {
	return mapcheck.WalkTimed(m, start, route)
}
