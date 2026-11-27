package game

import (
	"math"
	"strings"

	"fmt"
	"github.com/gigurra/counterslop/internal/sim"
	"sort"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// raceMaps: every race track (the rotation's and the labs').
func raceMaps() []string {
	var out []string
	for _, all := range []map[string]func() content.Map{maps.All, maps.Labs} {
		for name, build := range all {
			if build().Mode == content.ModeRace {
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// botRace: perTeam bots a side race laps laps on mapName to the end (or
// maxSecs). setup runs on the grid; each (if set) after every tick. Returns
// the server.
func botRace(t *testing.T, mapName string, perTeam int, laps string, maxSecs int, setup func(s *Server, bots []*Player), each ...func(s *Server)) *Server {
	t.Helper()
	s := authServer(t)
	s.Map = maps.Find(mapName)()
	s.Weapons = registry(t)
	s.Seed(1)
	s.cvars["mp_respawn"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_race_laps"] = laps
	s.cvars["mp_race_races"] = "1"
	s.cvars["bot_difficulty"] = "3"
	var bots []*Player
	for range perTeam {
		bots = append(bots, s.addAI(content.Vibe), s.addAI(content.Artisan))
	}
	s.Step(2) // the match starts: the grid (freeze)
	if s.round.Phase != PhaseFreeze {
		t.Fatalf("phase %q, want freeze (the grid)", s.round.Phase)
	}
	if setup != nil {
		setup(s, bots)
	}
	for range maxSecs * SimHz {
		s.Step(1)
		for _, f := range each {
			f(s)
		}
		if s.round.Phase == PhaseOver || s.round.Phase == PhaseMatchOver {
			break
		}
	}
	return s
}

// The bots race every track: each finishes its laps, none stays stuck.
func TestBotsRaceEveryTrack(t *testing.T) {
	for _, name := range raceMaps() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := botRace(t, name, 2, "2", 400, nil)
			r := &s.round
			if len(r.Racers) != 4 {
				t.Fatalf("%d racers, want the 4 bots", len(r.Racers))
			}
			var lines []string
			for _, rs := range r.Racers {
				p := s.players[rs.ID]
				lines = append(lines, fmt.Sprintf("%s: gates %d place %d time %.1f deaths %d", p.Name, rs.CP, rs.Place, rs.Time, p.Deaths))
			}
			t.Logf("%s:\n%v", name, lines)
			for _, rs := range r.Racers {
				if rs.Place == 0 {
					t.Errorf("%s didn't finish 2 laps (%d gates of %d)", s.players[rs.ID].Name, rs.CP, 2*r.Gates+1)
				}
			}
		})
	}
}

// Every car type can be driven by a bot round every rc_ track without ever
// falling off it: one of each (no money: the shop can't swap them), peaceful
// (nobody fires: this is the driving; rams still happen), two laps. Every car
// but the hovercraft finishes; the hovercraft (it slides: bots take it slowly,
// and don't buy it) drives a lap at least. (The setup runs on the grid: a new
// match puts everyone in stock cars.)
func TestBotsDriveEveryCar(t *testing.T) {
	for _, name := range raceMaps() {
		if !strings.HasPrefix(name, "rc_") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			falls := map[int]int{} // racer -> times its car went below every road (into the void)
			down := map[int]bool{}
			s := botRace(t, name, 2, "2", 500, func(s *Server, bots []*Player) {
				s.aiPeace = true
				for i, p := range bots {
					p.garage, p.Money = &garage{Type: CarTypes[i%len(CarTypes)].ID}, 0
					s.refitOnGrid(p)
				}
			}, func(s *Server) {
				for _, rs := range s.round.Racers {
					c := s.carByID(rs.Car)
					fell := c != nil && c.sim.Pos[1] < -9
					if fell && !down[rs.ID] {
						falls[rs.ID]++
						t.Logf("the %s fell at %.0f %.0f (tick %d)", c.Type, c.sim.Pos[0], c.sim.Pos[2], s.tick)
					}
					down[rs.ID] = fell
				}
			})
			types := map[string]bool{}
			for _, rs := range s.round.Racers {
				c := s.carByID(rs.Car)
				types[c.Type] = true
				t.Logf("%s (%s): gates %d place %d time %.1f deaths %d", s.players[rs.ID].Name, c.Type, rs.CP, rs.Place, rs.Time, s.players[rs.ID].Deaths)
				if falls[rs.ID] > 0 {
					t.Errorf("the %s fell off the track %d times", c.Type, falls[rs.ID])
				}
				if c.Type != "hovercraft" && rs.Place == 0 {
					t.Errorf("the %s didn't finish (%d gates)", c.Type, rs.CP)
				}
				if rs.CP < s.round.Gates+1 {
					t.Errorf("the %s didn't drive a lap (%d gates)", c.Type, rs.CP)
				}
			}
			if len(types) != len(CarTypes) {
				t.Fatalf("types %v", types)
			}
		})
	}
}

// A race of bots plays the same every time (the same seed): no wall clock,
// no map order in their driving.
func TestBotRaceReplays(t *testing.T) {
	t.Parallel()
	run := func() string {
		s := botRace(t, "lab_race", 2, "1", 120, nil)
		out := ""
		for _, rs := range s.round.Racers {
			c := s.carByID(rs.Car)
			out += fmt.Sprintf("%d:%d:%d:%.4f:%v ", rs.ID, rs.CP, rs.Place, rs.Time, c.sim.Pos)
		}
		return out
	}
	a, b := run(), run()
	if a != b {
		t.Fatalf("two runs differ:\n%s\n%s", a, b)
	}
}

// With money a bot buys on the grid: the best car it can afford, then upgrades.
func TestBotRaceBuys(t *testing.T) {
	s := botRace(t, "rc_ci_pipeline", 1, "1", 0, func(s *Server, bots []*Player) {
		bots[0].Money, bots[1].Money = 7000, 3000
		bots[0].ai.round, bots[1].ai.round = 0, 0 // they shopped already (with the start money): again
		s.Step(2)
	})
	g0, g1 := s.garageOf(s.players[s.round.Racers[0].ID]), s.garageOf(s.players[s.round.Racers[1].ID])
	if g0.Type != "tank" || g1.Type != "racecar" {
		t.Fatalf("cars %q %q, want the tank (7000: the best but the hovercraft, which bots don't buy) and the racecar (3000)", g0.Type, g1.Type)
	}
	if g0.Engine+g0.Plating+g0.Tyres == 0 || g1.Engine+g1.Plating+g1.Tyres == 0 {
		t.Fatalf("no upgrades with the change: %+v %+v", g0, g1)
	}
	if c := s.carByID(s.round.Racers[0].Car); c.Type != "tank" {
		t.Fatalf("its car on the grid is a %s", c.Type)
	}
}

// Nose into a rail at a standstill: the bot backs off (reverse, the other
// lock) and drives on.
func TestBotRaceBacksOffWhenStuck(t *testing.T) {
	s := botRace(t, "rc_ci_pipeline", 1, "1", 6, nil) // live by now (the countdown is 5 s)
	if !s.raceLive() {
		t.Fatalf("phase %q", s.round.Phase)
	}
	p := s.players[s.round.Racers[0].ID]
	c := s.carByID(p.car)
	// On the start straight, facing the south rail (z 6) from 1 m away.
	c.sim.Pos, c.sim.Vel, c.sim.Yaw = sim.Vec3{10, 0, 4.2}, sim.Vec3{0, 0, 0}, math.Pi
	c.sync()
	backed := false
	for range raceStuckTicks + 40 {
		cmd := s.aiRace(p)
		if cmd.F < 0 {
			backed = true
			break
		}
		s.Step(1)
		c.sim.Vel = sim.Vec3{0, 0, 0} // pinned against it
	}
	if !backed {
		t.Fatal("never backed off the rail")
	}
	if b := p.ai.race.back; b <= 0 || b >= raceBackTicks { // (the server's tick thinks for it too)
		t.Fatalf("back %d", p.ai.race.back)
	}
}

// Off the line (a wreck put it back at a gate, a bad jump): it finds its
// place again anywhere on the line, the right level of a crossover too.
func TestRaceLinePlace(t *testing.T) {
	s := authServer(t)
	s.Map = maps.Find("rc_ci_pipeline")()
	l := s.raceLineOf()
	if l == nil || l.len < 500 {
		t.Fatalf("line %v", l)
	}
	if s.raceLineOf() != l {
		t.Fatal("not cached")
	}
	on := func(i int, want Vec3) {
		t.Helper()
		if q := l.pts[i]; xzDist(q, want) > 3 || math.Abs(q[1]-want[1]) > 1 {
			t.Fatalf("place %v, want near %v", q, want)
		}
	}
	// The crossover: the start straight (y 0) under the flyover's deck (y 7) at x 0, z 0.
	on(l.place(Vec3{0, 0, 0}, 0, false), Vec3{0, 0, 0})
	on(l.place(Vec3{0, 7, 0}, 0, false), Vec3{0, 7, 0})
	// Tracking: from its last place, never jumping to the deck above.
	i := l.place(Vec3{2, 0, 0}, 0, false)
	on(l.place(Vec3{0, 0, 0}, i, true), Vec3{0, 0, 0})
	// No line drawn: gate to gate (lab_race).
	s.Map = maps.Find("lab_race")()
	if l2 := s.raceLineOf(); l2 == nil || l2 == l || len(l2.pts) < 20 {
		t.Fatal("no line through lab_race's gates")
	}
	s.Map.Checkpoints, s.Map.RaceLine = nil, nil
	s.Map.Name = "none"
	if s.raceLineOf() != nil {
		t.Fatal("a line with nothing to drive through")
	}
}
