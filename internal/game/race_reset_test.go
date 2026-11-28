package game

import (
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// gateMid: the middle of gate i, on the ground.
func gateMid(s *Server, i int) [3]float64 {
	g := s.Map.Checkpoints[i]
	return [3]float64{(g.Min[0] + g.Max[0]) / 2, 0, (g.Min[2] + g.Max[2]) / 2}
}

// nextRace runs the results out: the next race's grid.
func nextRace(t *testing.T, s *Server, now *time.Time) {
	t.Helper()
	n := s.round.Number
	for i := 0; i < 60*SimHz && s.round.Number == n; i++ {
		ticks(s, 1, now, nil)
	}
	if s.round.Number != n+1 || s.round.Phase != PhaseFreeze {
		t.Fatalf("round %d phase %q, want round %d on the grid", s.round.Number, s.round.Phase, n+1)
	}
}

// Race 2 of a match starts everyone level whatever they did at the end of
// race 1: finished early and idling past the line, in the pit stop, wrecked
// (then the garage open over the results) or just driving: everyone on their grid slot
// in their car, CP 0, the same next gate (the line), and over it together.
func TestRaceTwoStartsEveryoneLevel(t *testing.T) {
	s, v, a, now := raceServer(t, "1")
	s.cvars["mp_race_races"] = "3"
	c, d := s.join("3.3.3.3"), s.join("4.4.4.4")
	for i, p := range []*Player{c, d} {
		p.ready, p.Team = true, []string{content.Vibe, content.Artisan}[i]
		s.queueCmds(p, []Cmd{{Seq: 1}})
	}
	// Back to warmup and a fresh match with all four on the grid.
	s.round = newRound()
	ticks(s, 3, now, nil)
	if s.round.Phase != PhaseFreeze || len(s.round.Racers) != 4 {
		t.Fatalf("phase %q racers %d, want 4 on the grid", s.round.Phase, len(s.round.Racers))
	}
	goRace(t, s, now)
	// v finishes the one lap and idles past the line.
	for _, g := range []int{0, 1, 2, 3, 0} {
		putAt(s, v, gateMid(s, g))
		ticks(s, 1, now, nil)
	}
	if s.racer(v).Place != 1 {
		t.Fatalf("v did not finish: %+v", *s.racer(v))
	}
	putAt(s, v, gateMid(s, 1))
	// a two gates in, then into the pit stop.
	for _, g := range []int{0, 1} {
		putAt(s, a, gateMid(s, g))
		ticks(s, 1, now, nil)
	}
	s.pitMsg(a, true, 1)
	// c over the line, then wrecked.
	putAt(s, c, gateMid(s, 0))
	ticks(s, 1, now, nil)
	s.killBy(nil, c, "wreck", false)
	// d over the line and on to gate 1, just driving.
	for _, g := range []int{0, 1} {
		putAt(s, d, gateMid(s, g))
		ticks(s, 1, now, nil)
	}
	ticks(s, 1, now, nil)
	s.round.endsAt = *now // the time limit: race 1 over with all of them as they are
	ticks(s, 1, now, nil)
	if s.round.Phase != PhaseOver {
		t.Fatalf("phase %q, want the results", s.round.Phase)
	}
	s.pitMsg(c, true, 2) // the garage open over the results (the shop) and into the grid
	nextRace(t, s, now)
	check := func(when string) {
		t.Helper()
		if len(s.round.Racers) != 4 {
			t.Fatalf("%s: racers %+v, want 4", when, s.round.Racers)
		}
		for _, p := range []*Player{v, a, c, d} {
			rs := s.racer(p)
			if rs == nil {
				t.Fatalf("%s: %s not racing", when, p.Name)
			}
			car := s.carByID(p.car)
			if rs.CP != 0 || rs.Place != 0 || rs.Pit || p.pit || p.Dead || car == nil || car.parked || rs.Car != car.ID {
				t.Fatalf("%s: %s racer %+v pit %v dead %v car %d (%+v)", when, p.Name, *rs, p.pit, p.Dead, p.car, car)
			}
			if g := s.Map.Cars[rs.slot]; car.sim.Pos[0] != g.Pos[0] || car.sim.Pos[2] != g.Pos[2] {
				t.Fatalf("%s: %s car at %v, want grid slot %d %v", when, p.Name, car.sim.Pos, rs.slot, g.Pos)
			}
		}
	}
	check("race 2 grid")
	goRace(t, s, now)
	ticks(s, 1, now, nil)
	check("race 2 GO")
	// Over the line together: all on lap 1, the next gate 1.
	for _, p := range []*Player{v, a, c, d} {
		putAt(s, p, gateMid(s, 0))
	}
	ticks(s, 1, now, nil)
	for _, p := range []*Player{v, a, c, d} {
		if rs := s.racer(p); rs.CP != 1 || p.pit || p.Dead {
			t.Fatalf("race 2 over the line: %s racer %+v pit %v dead %v", p.Name, *rs, p.pit, p.Dead)
		}
	}
}

// A burst of commands in one server tick (a client's hitch: its queued
// commands all run at once) carries a fast car right through the 2 m start
// line: the line still counts (the gates are swept along the path the
// commands drove, not sampled once a tick), or that racer drives a whole
// lap for nothing.
func TestRaceGateCountsThroughACommandBurst(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	goRace(t, s, now)
	c := s.carByID(v.car)
	c.sim.Pos, c.sim.Vel = [3]float64{4, c.sim.Pos[1], 9}, [3]float64{-25, 0, 0} // 4 m before the line, flat out west
	c.sync()
	ticks(s, 1, now, nil)
	if v.Pos[0] < 1 || s.racer(v).CP != 0 {
		t.Fatalf("set-up: at x %.2f, CP %d; want short of the line, CP 0", v.Pos[0], s.racer(v).CP)
	}
	burst := make([]Cmd, 40)
	for i := range burst {
		burst[i] = Cmd{Seq: v.lastQueued + 1 + i, F: 1}
	}
	s.queueCmds(v, burst)
	ticks(s, 1, now, nil)
	if v.Pos[0] > -2 {
		t.Fatalf("the burst left the car at x %.2f; want well past the line (x < -1)", v.Pos[0])
	}
	if rs := s.racer(v); rs.CP != 1 {
		t.Fatalf("through the line in one burst: CP %d, want 1 (lap 1)", rs.CP)
	}
}
