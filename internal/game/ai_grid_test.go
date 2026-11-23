package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/sim"
)

// walkGridPath runs the real movement code along a grid route from start:
// straight at each waypoint, jumping for one higher than a step (as aiMove
// does). It returns where the body ended.
func walkGridPath(m *content.Map, start Vec3, path []Vec3) (Vec3, bool) {
	w := sim.NewWorld(m)
	b := sim.NewBody(sim.Vec3(start))
	for range 30 {
		w.Step(b, sim.Input{}, 0.01)
	}
	for _, n := range path {
		for t := 0; ; t++ {
			dx, dz := n[0]-b.Pos[0], n[2]-b.Pos[2]
			if math.Hypot(dx, dz) < 0.3 && math.Abs(n[1]-b.Pos[1]) < aiArriveDY {
				break
			}
			if t > 500 {
				return Vec3(b.Pos), false
			}
			b.Yaw = math.Atan2(-dx, -dz)
			jump := b.OnGround && n[1]-b.Pos[1] > aiStepUp && math.Hypot(dx, dz) < 1.2
			w.Step(b, sim.Input{F: 1, Jump: jump}, 0.01)
		}
	}
	for range 50 {
		w.Step(b, sim.Input{}, 0.01)
	}
	return Vec3(b.Pos), true
}

// The last leg off the nav graph, planned on the walk grid and walked with
// the real movement code: round a long wall (de_hackathon B: the small grid
// doesn't reach the wall's end), up onto a desk (de_standup A: a jump), and
// down the stairs from the floor above (de_stack B: not through the floor).
func TestGridPathLegsWalk(t *testing.T) {
	for _, tc := range []struct {
		m        string
		from, to Vec3
	}{
		{"de_hackathon", Vec3{-29, 0, 4.5}, Vec3{-38.8, 0, 12.4}},
		{"de_standup", Vec3{25.5, 0, 20}, Vec3{20.1, 0.75, 15.7}},
		{"de_stack", Vec3{28.5, 4, -15}, Vec3{27.7, 0, -13.3}},
	} {
		s := authServer(t)
		s.Map = maps.All[tc.m]()
		if s.segmentClear(tc.from, tc.to, 0.3) && tc.to[1]-tc.from[1] <= aiStepUp {
			t.Errorf("%s: %s -> %s is a straight walk: not a grid case", tc.m, v3(tc.from), v3(tc.to))
		}
		path, ok := s.gridPath(tc.from, tc.to)
		if !ok {
			t.Errorf("%s: no grid route %s -> %s", tc.m, v3(tc.from), v3(tc.to))
			continue
		}
		end, ok := walkGridPath(&s.Map, tc.from, path)
		if !ok || math.Hypot(end[0]-tc.to[0], end[2]-tc.to[2]) > 0.5 || math.Abs(end[1]-tc.to[1]) > 0.1 {
			t.Errorf("%s: walking %v from %s ended at %s, want %s", tc.m, path, v3(tc.from), v3(end), v3(tc.to))
		}
	}
}

// A goal no player gets to without a boost has no grid route (de_monorepo A:
// a 2 m crate on its own), and a far one isn't planned on a grid.
func TestGridPathNoWay(t *testing.T) {
	s := authServer(t)
	s.Map = maps.All["de_monorepo"]()
	if path, ok := s.gridPath(Vec3{20, 0, 15}, Vec3{21.2, 2, 17.8}); ok {
		t.Errorf("a route up a 2 m crate: %v", path)
	}
	if _, ok := s.gridPath(Vec3{-40, 0, 0}, Vec3{40, 0, 0}); ok {
		t.Error("planned a grid over 80 m")
	}
}

// A waypoint on the floor below (or above) is not reached by walking over
// (or under) it: de_stack's roof spawn is right above the escalator's foot.
func TestWaypointReachedOnItsFloor(t *testing.T) {
	s := authServer(t)
	s.Map = maps.All["de_stack"]()
	p := s.addAI(content.Artisan)
	s.Step(1)
	put(s, p, Vec3{25.5, 8, -3})
	below := Vec3{25.5, 0, -3.3}
	g := Vec3{22.5, 0, -6}
	p.ai.goTo = &g
	p.ai.goalKey, p.ai.goal = "goto:22.5,-6.0", g
	p.ai.repathAt = math.MaxInt64
	p.ai.path = []Vec3{below, g}
	s.Step(1)
	if len(p.ai.path) != 2 || p.ai.path[0] != below {
		t.Fatalf("walking over the waypoint below reached it: path %v", p.ai.path)
	}
}

// A bot (the real AI: nav graph, last leg, aiMove) from every spawn of
// every team reaches the objectives on every map.
func TestBotsWalkFromEverySpawn(t *testing.T) {
	for _, name := range maps.Order {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := maps.All[name]()
			for _, team := range content.Teams {
				objs := m.Objectives()
				for si, sp := range m.TeamSpawns[team] {
					for oi, site := range objs {
						// Each spawn to two objectives, rotating, so every spawn
						// and every objective is walked (TestWalkToSites walks
						// them all along the graph).
						if oi != si%len(objs) && oi != (si+1)%len(objs) && len(objs) > 2 {
							continue
						}
						s := authServer(t)
						s.Map = maps.All[name]()
						s.Seed(1)
						a := s.addAI(team)
						s.Step(1)
						put(s, a, Vec3(sp))
						c := site.Center()
						g := Vec3(c)
						a.ai.goTo = &g
						ok := false
						for n := 0; n < 40*SimHz && !ok; n += 10 {
							s.Step(10)
							ok = site.Contains(Vec3{a.Pos[0], a.Pos[1] + 0.1, a.Pos[2]})
						}
						if !ok {
							t.Errorf("%s spawn %d %s -> %s: at %s after 40 s", team, si, v3(Vec3(sp)), site.Name, v3(a.Pos))
						}
					}
				}
			}
		})
	}
}
