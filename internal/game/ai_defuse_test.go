package game

import (
	"cmp"
	"fmt"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/content/rng"
	"github.com/gigurra/counterslop/internal/sim"
)

// plantSpots: n seeded spots in the site where a planter can stand (a body
// dropped there settles on a floor or a crate top inside the zone), plus the
// site's middle when it is one.
func plantSpots(m *content.Map, z content.Zone, seed uint32, n int) []Vec3 {
	w := sim.NewWorld(m)
	r := rng.New(seed)
	var out []Vec3
	try := func(x, zz float64) {
		b := sim.NewBody(sim.Vec3{x, z.Max[1] - sim.H - 0.05, zz})
		if overlapsMap(m, b.Pos) {
			return
		}
		for range 300 {
			w.Step(b, sim.Input{}, 0.01)
		}
		if b.OnGround && z.Contains(b.Pos) {
			out = append(out, Vec3(b.Pos))
		}
	}
	c := z.Center()
	try(c[0], c[2])
	for tries := 0; len(out) < n+1 && tries < 50*n; tries++ {
		try(r.Range(z.Min[0]+sim.R, z.Max[0]-sim.R), r.Range(z.Min[2]+sim.R, z.Max[2]-sim.R))
	}
	return out
}

// overlapsMap: a standing player's hull at pos is inside a map box.
func overlapsMap(m *content.Map, pos sim.Vec3) bool {
	for _, b := range m.Boxes {
		if pos[0]+sim.R > b.Min[0] && pos[0]-sim.R < b.Max[0] && pos[2]+sim.R > b.Min[2] && pos[2]-sim.R < b.Max[2] &&
			pos[1]+sim.H > b.Min[1] && pos[1] < b.Max[1] {
			return true
		}
	}
	return false
}

// reachDefuse: an Artisan bot from spawn walks to the Big Diff planted at
// bomb and starts reverting it within limit seconds.
func reachDefuse(t *testing.T, mapName string, spawn, bomb Vec3, limit float64) (float64, Vec3, bool) {
	t.Helper()
	s := authServer(t)
	s.Map = maps.All[mapName]()
	s.Seed(1)
	a := s.addAI(content.Artisan)
	v := s.addAI(content.Vibe) // a dead attacker: the round stays on
	s.Step(1)
	v.Dead, v.HP = true, 0
	s.round.Phase, s.round.Number = PhasePlanted, 1
	a.ai.round, a.ai.site = 1, "A"
	a.Dead, a.HP = false, maxHP
	put(s, a, spawn)
	s.round.Bomb = bombState{State: BombPlanted, Pos: bomb}
	s.round.endsAt = s.now.Add(secs(1000))
	for n := 0; n < int(limit*SimHz); n++ {
		s.Step(1)
		if s.round.Bomb.Worker == a.ID || s.round.Reason == "defused" {
			return float64(n) / SimHz, a.Pos, true
		}
	}
	return limit, a.Pos, false
}

// Every Big Diff an attacker can plant (seeded spots all over each site, and
// the middle), a defender bot from its spawns gets to and starts reverting it:
// no crate between the last nav node and the bomb strands it. A spot only a
// boost gets up to (no defender could get to it) refuses the plant instead.
func TestDefuserReachesEveryPlant(t *testing.T) {
	for _, name := range maps.Order {
		m := maps.All[name]()
		if len(m.Sites) == 0 || m.NoBomb() { // no bomb, nothing to revert
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for si, z := range m.Sites {
				spots := plantSpots(&m, z, uint32(si+1)*7919, 8)
				reach := authServer(t)
				reach.Map = m
				var ok []Vec3
				for _, bomb := range spots {
					switch {
					case reach.defusable(bomb):
						ok = append(ok, bomb)
					case walkable(reach, bomb):
						t.Errorf("site %s: %s: plant refused, but a player walks onto the spot", z.Name, v3(bomb))
					case plants(t, name, bomb):
						t.Errorf("site %s: %s: no defender gets there, yet the plant went in", z.Name, v3(bomb))
					}
				}
				if len(ok) < 6 {
					t.Errorf("site %s: only %d plant spots found (%d defusable)", z.Name, len(spots), len(ok))
				}
				spawns := m.TeamSpawns[content.Artisan]
				for bi, bomb := range ok {
					for i, sp := range spawns {
						// Two spawns per spot, rotating (every spawn's way to
						// the site is TestWalkToSites's); DEFUSE_ALL=1: all.
						if os.Getenv("DEFUSE_ALL") == "" && i != bi%len(spawns) && i != (bi+len(spawns)/2)%len(spawns) {
							continue
						}
						secs, at, ok := reachDefuse(t, name, Vec3(sp), bomb, 45)
						if !ok {
							t.Errorf("site %s: bomb at %s: spawn %d %s: no defuse in %.0f s, bot at %s", z.Name, v3(bomb), i, v3(Vec3(sp)), secs, v3(at))
						}
					}
				}
			}
		})
	}
}

// walkable: a player gets to spot from the nav graph walking, stepping,
// dropping and jumping (no boost): a walk-grid route from one of the three
// nearest nav nodes ends on it.
func walkable(s *Server, spot Vec3) bool {
	nodes := slices.Clone(s.Map.Nav)
	d := func(n content.NavNode) float64 {
		return math.Hypot(n.Pos[0]-spot[0], n.Pos[2]-spot[2]) + 3*math.Abs(n.Pos[1]-spot[1])
	}
	slices.SortFunc(nodes, func(a, b content.NavNode) int { return cmp.Compare(d(a), d(b)) })
	for _, n := range nodes[:min(3, len(nodes))] {
		if path, ok := s.gridPath(Vec3(n.Pos), spot); ok {
			end := path[len(path)-1]
			if math.Hypot(end[0]-spot[0], end[2]-spot[2]) < 0.3 && math.Abs(end[1]-spot[1]) < 0.3 {
				return true
			}
		}
	}
	return false
}

func v3(v Vec3) string { return fmt.Sprintf("(%.1f %.1f %.1f)", v[0], v[1], v[2]) }

// de_stack: the Artisans spawn on the roof, right above the escalator's foot.
// Full teams in warmup (seeded): every bot walking to B gets down off the
// roof. A waypoint counts as reached only on its floor: walking over the
// escalator's foot "reached" it, and the bot then walked at the next one, on
// the floor below, into the roof's railing for the rest of the warmup.
func TestRoofSpawnsGetDownToB(t *testing.T) {
	for seed := 1; seed <= 3; seed++ {
		s := authServer(t)
		s.Map = maps.All["de_stack"]()
		s.Seed(uint64(seed))
		s.cvars["mp_warmup_time"] = "60"
		for range s.Map.MaxPlayers / 2 {
			s.addAI(content.Vibe)
			s.addAI(content.Artisan)
		}
		for _, p := range s.ordered() {
			p.ai.r = rng.New(uint32(seed)*2654435761 + uint32(p.ID)*40503 + 1)
		}
		// Up there 20 s after a (re)spawn while walking to B: stuck. (Warmup
		// respawns the dead at the spawn.)
		deaths, since := map[int]int{}, map[int]int{}
		for sec := 1; sec <= 50; sec++ {
			s.Step(SimHz)
			for _, p := range s.ordered() {
				if p.Deaths != deaths[p.ID] || p.Dead || p.ai.goalKey != "site:B" || p.Pos[1] < 6 {
					deaths[p.ID], since[p.ID] = p.Deaths, sec
				}
				if sec-since[p.ID] == 20 {
					t.Errorf("seed %d: %s on the roof at %s for 20 s, walking to B: path %v", seed, p.Name, v3(p.Pos), p.ai.path)
				}
			}
		}
	}
}
