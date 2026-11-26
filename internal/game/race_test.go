package game

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/sim"
)

// raceServer: race mode on lab_race (mp_race_laps laps), alice (vibe) and
// bob (artisan) racing, on the grid (freeze: the countdown).
func raceServer(t *testing.T, laps string) (*Server, *Player, *Player, *time.Time) {
	t.Helper()
	s, v, a := econServer(t)
	s.Weapons = registry(t)
	s.Map = maps.LabRace()
	s.round = newRound()
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_respawn"] = "0"
	s.cvars["mp_race_laps"] = laps
	now := time.Now()
	for _, p := range []*Player{v, a} {
		s.queueCmds(p, []Cmd{{Seq: 1}})
	}
	s.simTick(now)
	ticks(s, 3, &now, nil)
	if s.round.Phase != PhaseFreeze {
		t.Fatalf("phase %q, want freeze (the grid)", s.round.Phase)
	}
	return s, v, a, &now
}

// goRace runs the countdown out: the race is live.
func goRace(t *testing.T, s *Server, now *time.Time) {
	t.Helper()
	ticks(s, int(raceGridSecs*SimHz)+2, now, nil)
	if s.round.Phase != PhaseLive {
		t.Fatalf("phase %q, want live", s.round.Phase)
	}
}

// putAt puts p (their car with them) at pos for the next race tick.
func putAt(s *Server, p *Player, pos [3]float64) {
	p.Pos = Vec3(pos)
	if c := s.carByID(p.car); c != nil {
		c.sim.Pos = sim.Vec3(pos)
		c.sync()
	}
}

func TestRaceGrid(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	r := &s.round
	if r.MaxRounds != 1 || r.Laps != 2 || r.Gates != 4 || len(r.Racers) != 2 || r.Bomb.State != "" {
		t.Fatalf("round: max %d laps %d gates %d racers %+v bomb %q", r.MaxRounds, r.Laps, r.Gates, r.Racers, r.Bomb.State)
	}
	for i, p := range []*Player{v, a} {
		c := s.carByID(p.car)
		if c == nil || c.Driver != p.ID || s.racer(p).Car != c.ID {
			t.Fatalf("%s: car %d (%+v), racer %+v", p.Name, p.car, c, s.racer(p))
		}
		if g := s.Map.Cars[i]; c.sim.Pos != sim.Vec3(g.Pos) || c.sim.Yaw != g.Yaw {
			t.Fatalf("%s: car at %v yaw %v, want grid slot %d %v", p.Name, c.sim.Pos, c.sim.Yaw, i, g)
		}
	}
	// The countdown holds the cars: full throttle goes nowhere, E gets nobody out.
	c0 := s.carByID(v.car).sim.Pos
	ticks(s, 100, now, map[*Player]Cmd{v: {F: 1}, a: {B: BtnUse}})
	if s.carByID(v.car).sim.Pos != c0 || a.car == 0 {
		t.Fatalf("the grid: car moved %v -> %v, bob's car %d", c0, s.carByID(v.car).sim.Pos, a.car)
	}
	goRace(t, s, now)
	ticks(s, 50, now, map[*Player]Cmd{v: {F: 1}})
	if d := s.carByID(v.car).sim.Pos[0] - c0[0]; d > -1 {
		t.Fatalf("GO: the car moved %v along x, want west", d)
	}
}

// Gates count in order only: the start line, then each one after it. A lap
// is all of them; crossing the line after the last lap finishes.
func TestRaceGatesInOrder(t *testing.T) {
	s, v, _, now := raceServer(t, "1")
	goRace(t, s, now)
	cp := s.Map.Checkpoints
	mid := func(i int) [3]float64 {
		g := cp[i]
		return [3]float64{(g.Min[0] + g.Max[0]) / 2, 0, (g.Min[2] + g.Max[2]) / 2}
	}
	rs := func() *racerState { return s.racer(v) }
	for _, step := range []struct {
		gate, want int
	}{{1, 0}, {0, 1}, {0, 1}, {3, 1}, {1, 2}, {3, 2}, {2, 3}, {0, 3}, {3, 4}} {
		putAt(s, v, mid(step.gate))
		ticks(s, 1, now, nil)
		if rs().CP != step.want {
			t.Fatalf("in gate %d: CP %d, want %d", step.gate, rs().CP, step.want)
		}
	}
	before := v.Points
	putAt(s, v, mid(0))
	ticks(s, 1, now, nil)
	if r := rs(); r.CP != 5 || r.Place != 1 || r.Time <= 0 || v.Points != before+10 {
		t.Fatalf("finish: %+v, points %d (was %d)", *r, v.Points, before)
	}
	if left := s.roundMsgOf(*now).Left; left > raceFinishWaitSecs {
		t.Fatalf("after the winner: %v s left, want at most %v", left, raceFinishWaitSecs)
	}
	if s.round.Racers[0].ID != v.ID {
		t.Fatalf("positions %+v, want alice first", s.round.Racers)
	}
}

// Positions: the finished by place, then gates passed, then nearest the next gate.
func TestRacePositions(t *testing.T) {
	s, v, a, now := raceServer(t, "1")
	goRace(t, s, now)
	putAt(s, v, [3]float64{0, 0, 12})
	putAt(s, a, [3]float64{0, 0, 12})
	ticks(s, 1, now, nil) // both through the line
	putAt(s, v, [3]float64{-20, 0, 12})
	putAt(s, a, [3]float64{-30, 0, 12}) // nearer the west end
	ticks(s, 1, now, nil)
	if s.round.Racers[0].ID != a.ID {
		t.Fatalf("positions %+v, want bob ahead (nearer the next gate)", s.round.Racers)
	}
	putAt(s, v, [3]float64{-50, 0, 0}) // alice through the west end: a gate more
	ticks(s, 1, now, nil)
	if s.round.Racers[0].ID != v.ID {
		t.Fatalf("positions %+v, want alice ahead (a gate more)", s.round.Racers)
	}
}

// Off the plate's edge into the void: the driver dies, and is back 3 s later
// in their car, repaired, on the last gate they passed, the lap kept.
func TestRaceWreckRespawnAtTheLastGate(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	goRace(t, s, now)
	putAt(s, v, [3]float64{0, 0, 12})
	ticks(s, 1, now, nil)
	putAt(s, v, [3]float64{-50, 0, 12}) // through the west end: CP 2
	ticks(s, 1, now, nil)
	c := s.carByID(v.car)
	c.sim = *sim.NewCar(c.spec, sim.Vec3{-55, 0, 20}, math.Pi) // facing +z, 4 m from the north edge
	c.sim.HP = 40
	c.sync()
	died := 0
	for i := range 300 {
		ticks(s, 1, now, map[*Player]Cmd{v: {F: 1}})
		if v.Dead {
			died = i
			break
		}
	}
	if !v.Dead || s.racer(v).CP != 2 {
		t.Fatalf("over the edge: dead %v, racer %+v", v.Dead, *s.racer(v))
	}
	ticks(s, int((raceRespawnSecs-0.5)*SimHz), now, nil)
	if w := s.racer(v).Wreck; !v.Dead || w <= 0 || w > 0.6 {
		t.Fatalf("2.5 s after: dead %v, wreck %v s left, want ~0.5", v.Dead, w)
	}
	ticks(s, int(0.6*SimHz), now, nil)
	c = s.carByID(s.racer(v).Car)
	g := s.Map.Checkpoints[1]
	if v.Dead || v.car != c.ID || c.sim.Pos != sim.Vec3(g.Spawn) || c.sim.Yaw != g.Yaw || c.HP != c.MaxHP || s.racer(v).CP != 2 || s.racer(v).Wreck != 0 {
		t.Fatalf("back (died at tick %d): dead %v car %d/%d at %v yaw %v hp %d, racer %+v; want at %v", died, v.Dead, v.car, c.ID, c.sim.Pos, c.sim.Yaw, c.HP, *s.racer(v), g.Spawn)
	}
}

// A car at 0 HP is a wreck too.
func TestRaceCarHPZeroWrecks(t *testing.T) {
	s, v, _, now := raceServer(t, "1")
	goRace(t, s, now)
	c := s.carByID(v.car)
	c.HP = 0
	ticks(s, 1, now, nil)
	if !v.Dead || s.racer(v).Wreck <= 0 {
		t.Fatalf("car HP 0: dead %v racer %+v", v.Dead, *s.racer(v))
	}
}

// During the race nobody gets out (me.cl); in warmup E gets out as always.
func TestRaceLockedInTheCar(t *testing.T) {
	s, v, _, now := raceServer(t, "1")
	goRace(t, s, now)
	ticks(s, 1, now, map[*Player]Cmd{v: {B: BtnUse}})
	ticks(s, 1, now, nil)
	if v.car == 0 || !s.carLocked(v) || !s.meStateOf(v).CarLock {
		t.Fatalf("E in the race: car %d locked %v", v.car, s.carLocked(v))
	}
	if CarLeaves(BtnUse, 0, true) || !CarLeaves(BtnUse, 0, false) {
		t.Fatal("CarLeaves ignores the lock")
	}
}

// Everyone over the line: the results, the winner's team takes the round
// (the match: one race).
func TestRaceEnds(t *testing.T) {
	s, v, a, now := raceServer(t, "1")
	goRace(t, s, now)
	for _, p := range []*Player{a, v} { // bob first
		for _, g := range []int{0, 1, 2, 3, 0} {
			cp := s.Map.Checkpoints[g]
			putAt(s, p, [3]float64{(cp.Min[0] + cp.Max[0]) / 2, 0, (cp.Min[2] + cp.Max[2]) / 2})
			ticks(s, 1, now, nil)
		}
	}
	r := &s.round
	if r.Phase != PhaseOver || r.Winner != content.Artisan || r.Reason != "race" {
		t.Fatalf("phase %q winner %q reason %q", r.Phase, r.Winner, r.Reason)
	}
	if r.Racers[0].ID != a.ID || r.Racers[0].Place != 1 || r.Racers[1].Place != 2 || a.Points < 10 || v.Points < 8 {
		t.Fatalf("results %+v, points %d / %d", r.Racers, a.Points, v.Points)
	}
}

// The time limit: the leader's team wins.
func TestRaceTimeLimit(t *testing.T) {
	s, v, _, now := raceServer(t, "4")
	goRace(t, s, now)
	putAt(s, v, [3]float64{0, 0, 12})
	ticks(s, 1, now, nil)
	s.round.endsAt = *now
	ticks(s, 1, now, nil)
	if s.round.Phase != PhaseOver || s.round.Winner != content.Vibe {
		t.Fatalf("time: phase %q winner %q", s.round.Phase, s.round.Winner)
	}
}

// A player who joins during the race spawns on the timer and races from a
// free grid slot, lap 0.
func TestRaceLateJoin(t *testing.T) {
	s, _, _, now := raceServer(t, "1")
	goRace(t, s, now)
	c := s.join("3.3.3.3")
	c.ready, c.Team = true, content.Vibe
	s.queueCmds(c, []Cmd{{Seq: 1}})
	c.Dead, c.respawnAt = true, *now
	ticks(s, 5, now, nil)
	rs := s.racer(c)
	if c.Dead || rs == nil || c.car == 0 || rs.CP != 0 || rs.Car != c.car {
		t.Fatalf("late joiner: dead %v car %d racer %+v", c.Dead, c.car, rs)
	}
	if g := s.Map.Cars[rs.slot]; math.Abs(s.carByID(c.car).sim.Pos[0]-g.Pos[0]) > 1e-9 {
		t.Fatalf("late joiner's car at %v, want grid slot %d %v", s.carByID(c.car).sim.Pos, rs.slot, g.Pos)
	}
}

// Every snapshot carries the race: laps, gates, the racers, the clock.
func TestRaceInTheSnapshot(t *testing.T) {
	s, v, _, now := raceServer(t, "3")
	goRace(t, s, now)
	putAt(s, v, [3]float64{0, 0, 12})
	ticks(s, 30, now, nil)
	var msg map[string]any
	json.Unmarshal(s.roundJSON(*now), &msg)
	race, _ := msg["race"].([]any)
	if msg["mode"] != "race" || msg["laps"] != 3.0 || msg["gates"] != 4.0 || len(race) != 2 || msg["rt"].(float64) < 0.25 {
		t.Fatalf("round JSON %v", msg)
	}
	if r0 := race[0].(map[string]any); r0["id"] != float64(v.ID) || r0["cp"] != 1.0 || r0["car"] == nil {
		t.Fatalf("racer %v", r0)
	}
}

// Bots sit races out (phase 5 teaches them): on a race map they stand at
// their spawn, alive, whatever the round.
func TestAIIdleOnARaceMap(t *testing.T) {
	s := aiMatch(t, "rc_ci_pipeline", 1, 300)
	for _, p := range s.ordered() {
		if p.ai == nil || p.body == nil {
			continue
		}
		at := p.body.Pos
		s.Step(200)
		if p.Dead || p.body.Pos != at || s.racer(p) != nil {
			t.Fatalf("bot %s: dead %v, moved %v -> %v, racing %v", p.Name, p.Dead, at, p.body.Pos, s.racer(p) != nil)
		}
	}
}
