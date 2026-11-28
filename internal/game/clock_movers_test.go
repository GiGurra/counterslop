package game

import (
	"math"
	"testing"
)

// A command clock that fell behind the server's (no commands while a tab was
// hidden) gets back in step near movers too: a player standing at the Moon's
// drop zone (under MVP-1's path) stays where it is, a rider in the cabin keeps
// its place in it, where the ship is now. Before, the clock was put back in
// step only away from movers: the hidden-tab player's ships ran on a timeline
// off from everyone's (seconds to minutes) for as long as it stayed near one.
func TestCommandClockBackInStepNearMovers(t *testing.T) {
	s := authServer(t)
	s.Test = true
	if err := s.ResetWorldOn("cq_moonshot"); err != nil {
		t.Fatal(err)
	}
	dep := s.Map.Departures[0]
	m := &s.Map.Movers[dep.Mover-1]
	alice := s.join("10.0.0.1")
	alice.Name, alice.ready, alice.Team = "alice", true, "vibe"
	alice.inv = s.Weapons.Loadout(alice.Team)
	seq := 0
	cmd := func() {
		seq++
		s.queueCmds(alice, []Cmd{{Seq: seq, Yaw: -math.Pi / 2, VT: float64(s.tick) - drawnDelay}})
		s.Step(1)
	}
	drift := func() int64 { return s.tick - (alice.tickBase + int64(alice.ackSeq)) }
	hide := func(ticks int) { // the tab hidden: no commands, the server ticks on
		for range ticks {
			s.Step(1)
		}
	}
	for range 20 {
		cmd()
	}

	// On the Moon, at the drop zone under the ship's path.
	s.teleport(alice, Vec3{120, 30, 0}, 0, false, seq)
	for range 50 {
		cmd()
	}
	w := s.simWorld()
	if !w.NearMovers(alice.body) {
		t.Fatalf("the drop zone at %v is not near a mover: the test needs one", alice.Pos)
	}
	at := alice.Pos
	hide(3000)
	if d := drift(); d < 2900 {
		t.Fatalf("drift %d after 30 s hidden: want about 3000", d)
	}
	for range 20 {
		cmd()
	}
	if d := drift(); d > maxClockDrift || d < -maxClockDrift {
		t.Errorf("at the drop zone: the command clock is %d ticks off the server's after coming back (max %d)", d, maxClockDrift)
	}
	if dist(alice.Pos, at) > 0.05 {
		t.Errorf("at the drop zone: moved from %v to %v", at, alice.Pos)
	}

	// Aboard: board at the dock, hide in the climb, back in the cross.
	cyc := func() float64 { return math.Mod(alice.cmdTime(seq+1), m.Period) }
	for cyc() < 2 || cyc() > dep.Close-3 { // boarding
		cmd()
	}
	s.teleport(alice, Vec3{dep.Board[0] + 2, dep.Board[1], dep.Board[2]}, -math.Pi/2, false, seq)
	for cyc() < 21 {
		cmd()
	}
	if alice.Frame != dep.Mover || alice.Local == nil {
		t.Fatalf("not aboard at %.2f s into the cycle: %v", cyc(), alice.Pos)
	}
	local := *alice.Local
	hide(300)
	for range 20 {
		cmd()
	}
	if d := drift(); d > maxClockDrift || d < -maxClockDrift {
		t.Errorf("aboard: the command clock is %d ticks off the server's after coming back (max %d)", d, maxClockDrift)
	}
	if alice.Frame != dep.Mover || alice.Local == nil {
		t.Fatalf("aboard: out of the cabin after coming back at %.2f s into the cycle: %v", cyc(), alice.Pos)
	}
	if d := dist(*alice.Local, local); d > 0.05 {
		t.Errorf("aboard: its place in the cabin moved %.2f m (%v -> %v)", d, local, *alice.Local)
	}
}
