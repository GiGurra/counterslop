package game

import (
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/sim"
)

// pkServer: lab_parkour's course with two racers on foot, the countdown run
// out (the race live).
func pkServer(t *testing.T) (*Server, *Player, *Player, *time.Time) {
	t.Helper()
	s, v, a := econServer(t)
	s.Weapons = registry(t)
	s.Map = maps.LabParkour()
	s.round = newRound()
	s.resetCars()
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_respawn"] = "0"
	now := time.Now()
	for _, p := range []*Player{v, a} {
		s.queueCmds(p, []Cmd{{Seq: 1}})
	}
	s.simTick(now)
	ticks(s, 3, &now, nil)
	if s.round.Phase != PhaseFreeze {
		t.Fatalf("phase %q, want freeze (the start)", s.round.Phase)
	}
	goRace(t, s, &now)
	return s, v, a, &now
}

// walkTo: p (on foot) at pos, then a tick of the race.
func walkTo(s *Server, p *Player, pos [3]float64, now *time.Time) {
	p.Pos = Vec3(pos)
	p.body.Pos = sim.Vec3(pos)
	ticks(s, 1, now, nil)
}

func TestParkourOnFoot(t *testing.T) {
	s, v, a, now := pkServer(t)
	r := &s.round
	if !r.P2P || r.Laps != 1 || r.Gates != 3 || len(r.Racers) != 2 {
		t.Fatalf("round: p2p %v laps %d gates %d racers %+v", r.P2P, r.Laps, r.Gates, r.Racers)
	}
	for _, p := range []*Player{v, a} {
		if p.car != 0 || s.racer(p).Car != 0 || p.Dead || p.Pos[0] > 2 {
			t.Fatalf("%s: car %d, racer %+v, dead %v at %v: want on foot at the start", p.Name, p.car, s.racer(p), p.Dead, p.Pos)
		}
	}
	// Through the start and the middle: splits, no best yet.
	walkTo(s, v, [3]float64{10.5, 0, 0}, now)
	walkTo(s, v, [3]float64{40.5, 0, 0}, now)
	rv := s.racer(v)
	if rv.CP != 2 || rv.Split <= 0 || rv.HasDelta || len(rv.splits) != 2 {
		t.Fatalf("after two gates: %+v", *rv)
	}
	// A fall into the void's pit: back on the middle gate at once, facing
	// along the course (a respawn: no death), still 2 gates.
	g := s.Map.Checkpoints[1]
	seq := v.spawnSeq
	v.body.Pos = sim.Vec3{65, -7, 0}
	ticks(s, 1, now, map[*Player]Cmd{v: {}})
	if v.Dead || v.Pos != Vec3(g.Spawn) || v.Yaw != g.Yaw || v.spawnSeq != seq+1 || s.racer(v).CP != 2 {
		t.Fatalf("fell: at %v yaw %v dead %v cp %d, want the middle gate %v %v", v.Pos, v.Yaw, v.Dead, s.racer(v).CP, g.Spawn, g.Yaw)
	}
	// Below the world (no pit there): the body back on the gate too.
	v.body.Pos = sim.Vec3{200, -30, 200}
	ticks(s, 1, now, map[*Player]Cmd{v: {}})
	if v.Pos[0] != g.Spawn[0] || v.Pos[2] != g.Spawn[2] {
		t.Fatalf("out of the world: at %v, want the middle gate %v", v.Pos, g.Spawn)
	}
	// Killed (combat on, a grenade, ...): back after the respawn time on the
	// middle gate, its heading, still 2 gates.
	s.killBy(nil, v, "world", false)
	ticks(s, int(raceRespawnSecs*SimHz)+5, now, nil)
	if v.Dead || v.Pos != Vec3(g.Spawn) || v.Yaw != g.Yaw || s.racer(v).CP != 2 {
		t.Fatalf("respawned at %v yaw %v dead %v cp %d, want the middle gate %v %v", v.Pos, v.Yaw, v.Dead, s.racer(v).CP, g.Spawn, g.Yaw)
	}
	// The finish (point to point: the last gate), first place.
	walkTo(s, v, [3]float64{90.5, 0, 0}, now)
	rv = s.racer(v)
	if rv.Place != 1 || rv.Time <= 0 || !rv.Best || rv.PB != rv.Time {
		t.Fatalf("finished: %+v", *rv)
	}
	if s.racer(a).Place != 0 || s.racer(a).CP != 0 {
		t.Fatalf("bob: %+v", *s.racer(a))
	}
	// Bob walks into the middle gate without the start: out of order, nothing.
	walkTo(s, a, [3]float64{40.5, 0, 0}, now)
	if s.racer(a).CP != 0 {
		t.Fatal("a gate out of order counted")
	}
}

// The next run is timed against the best: each gate's split minus the best
// run's there; a slower finish keeps the old best.
func TestParkourSplitsAgainstTheBest(t *testing.T) {
	s, v, _, now := pkServer(t)
	run := func(waits ...int) *racerState {
		for i, x := range []float64{10.5, 40.5, 90.5} {
			ticks(s, waits[i], now, nil)
			walkTo(s, v, [3]float64{x, 0, 0}, now)
		}
		return s.racer(v)
	}
	first := *run(100, 300, 300)
	if !first.Best || math.Abs(first.Time-7.03) > 0.05 {
		t.Fatalf("the first run: %+v", first)
	}
	// A new race: the same course, gates faster, then slower.
	s.startRace()
	s.round.started = *now
	s.round.Phase = PhaseLive
	walkTo(s, v, [3]float64{0, 0, 0}, now)
	ticks(s, 50, now, nil)
	walkTo(s, v, [3]float64{10.5, 0, 0}, now)
	rv := s.racer(v)
	if !rv.HasDelta || math.Abs(rv.Delta-(rv.Split-first.splits[0])) > 1e-9 || rv.Delta > -0.4 {
		t.Fatalf("the start 0.5 s faster: delta %v (split %v, best %v)", rv.Delta, rv.Split, first.splits[0])
	}
	ticks(s, 600, now, nil)
	walkTo(s, v, [3]float64{40.5, 0, 0}, now)
	ticks(s, 300, now, nil)
	walkTo(s, v, [3]float64{90.5, 0, 0}, now)
	rv = s.racer(v)
	if rv.Place == 0 || rv.Best || rv.PB != first.Time || rv.Delta <= 0 {
		t.Fatalf("a slower run: %+v (best %v)", *rv, first.Time)
	}
}

// mp_race_combat 0 (lab_parkour pins it): nobody hurts anybody, also with a
// car's items; 1: the race's free for all.
func TestRaceCombatOff(t *testing.T) {
	s, v, a, _ := pkServer(t)
	if s.cvar("mp_race_combat") != "0" {
		t.Fatalf("mp_race_combat %q: the map pins 0", s.cvar("mp_race_combat"))
	}
	if !s.spared(v, a) || s.spared(v, v) || s.spared(nil, a) {
		t.Fatal("spared: combat off spares everyone else")
	}
	hp := a.HP
	s.hurt(v, a, 30, "test", v.Pos)
	if a.HP != hp {
		t.Fatalf("hurt with combat off: %d -> %d", hp, a.HP)
	}
	c := s.cars[0]
	chp := c.sim.HP
	s.damageCar(c, 30, v, "test")
	if c.sim.HP != chp {
		t.Fatal("a car hurt by a racer with combat off")
	}
	s.cvars["mp_race_combat"] = "1"
	s.Map.Rules = nil
	if s.spared(v, a) {
		t.Fatal("spared with combat on: a race is free for all")
	}
	s.hurt(v, a, 30, "test", v.Pos)
	if a.HP != hp-30 {
		t.Fatalf("hurt with combat on: %d -> %d", hp, a.HP)
	}
}

// A car left out on the course goes back to its station after a while,
// repaired; one with a driver, or at its station, stays.
func TestParkourStations(t *testing.T) {
	s, v, _, now := pkServer(t)
	c := s.cars[0]
	home := sim.Vec3(s.Map.Cars[0].Pos)
	c.sim.Pos, c.sim.HP = sim.Vec3{80, 0, 0}, 10
	at := *now
	s.tickStations(at)
	s.tickStations(at.Add(time.Duration((stationWait - 0.1) * float64(time.Second))))
	if c.sim.Pos == home {
		t.Fatal("back before stationWait")
	}
	s.tickStations(at.Add(time.Duration(stationWait * float64(time.Second))))
	if c.sim.Pos != home || c.sim.HP != c.MaxHP {
		t.Fatalf("not back at its station, repaired: %v HP %d", c.sim.Pos, c.sim.HP)
	}
	c.sim.Pos, c.Driver = sim.Vec3{80, 0, 0}, v.ID
	s.tickStations(at)
	s.tickStations(at.Add(time.Minute))
	if c.sim.Pos == home {
		t.Fatal("a driven car sent home")
	}
	// On foot the car is free: no lock in it.
	v.car = c.ID
	if s.carLocked(v) {
		t.Fatal("locked in a free car on foot")
	}
}

// Bots don't run a parkour course without a route (Map.Route; lab_parkour):
// they aren't racers there.
func TestParkourNoBots(t *testing.T) {
	s, _, _, _ := pkServer(t)
	b := s.addAI("vibe")
	if s.racing(b) {
		t.Fatal("a bot races on foot")
	}
}

// Bots run pk_happy_path, start to finish, in both movement models: the
// route (Map.Route, ai_parkour.go) through the real server: the stones, the
// pad chain, the lifts, the hooks and wall walks, the low-gravity hops, a
// buggy over the jump and a dinghy across the basin. The winner finishes;
// the other is at least on the last leg when the race ends after it.
func TestBotRunsTheHappyPath(t *testing.T) {
	for _, model := range []string{"classic", "source"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			var bots []*Player
			s := botRace(t, "pk_happy_path", 1, "1", 400, func(s *Server, b []*Player) {
				s.cvars["sv_movement"] = model
				bots = b
			})
			gates := len(s.Map.Checkpoints)
			won := false
			for _, bot := range bots {
				rs := s.racer(bot)
				if rs == nil {
					t.Fatalf("%s isn't racing", bot.Name)
				}
				won = won || rs.Place == 1
				k := bot.ai.pk
				t.Logf("%s: place %d time %.1f gates %d deaths %d, on step %d", bot.Name, rs.Place, rs.Time, rs.CP, bot.Deaths, k.i)
				if rs.Place == 0 && rs.CP < gates-1 {
					st := content.RouteStep{}
					if k.i < len(s.Map.Route) {
						st = s.Map.Route[k.i]
					}
					t.Errorf("%s stuck at gate %d of %d: at %.2v (gravity %d), on step %d %+v for %d ticks", bot.Name, rs.CP, gates, bot.body.Pos, bot.body.Grav, k.i, st, k.ticks)
				}
			}
			if !won {
				t.Error("nobody finished")
			}
		})
	}
}

// A course bot that can't finish a step in 25 s is put back on its gate (as a
// fall would) and starts that gate's steps again; on a course without a
// route bots don't race (TestParkourNoBots).
func TestParkourBotStuckGoesBack(t *testing.T) {
	var bot *Player
	moved := false
	botRace(t, "pk_happy_path", 1, "1", 36, func(s *Server, b []*Player) {
		bot = b[0]
		s.Map.Route = []content.RouteStep{{Do: content.RouteWait, Period: 1, Phase: 5}} // never: the clock modulo 1 is never 5
	}, func(s *Server) {
		if s.raceLive() && !moved {
			moved = true
			bot.Pos, bot.body.Pos = Vec3{-10, 0, 5}, sim.Vec3{-10, 0, 5} // off its spawn
		}
	})
	if bot.ai.pk.ticks > pkStepTicks || dist3(bot.body.Pos, sim.Vec3{-10, 0, 5}) < 1 {
		t.Fatalf("after 30 s on a step it can't do: %d ticks on it, at %v: want back on the start (its spawn)", bot.ai.pk.ticks, bot.body.Pos)
	}
}
