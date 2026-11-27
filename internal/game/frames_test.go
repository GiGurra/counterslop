package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/sim"
)

// Two passengers stand in MVP-1's aisle through the climb (lab_moon), their
// commands arriving unevenly (bunched: the command clocks run apart from the
// server's, as on a real link). The server replicates bob in the cabin's
// frame at one place all the way up, and alice's commands see him there: the
// rewound pose a shot is traced against and the hull she bumps into are at
// that place in the cabin where the cabin is at her command's time (where
// her client draws it), not metres off it in the world.
func TestRidersInTheirFrame(t *testing.T) {
	s := authServer(t)
	s.Test = true
	if err := s.ResetWorldOn("lab_moon"); err != nil {
		t.Fatal(err)
	}
	dep := s.Map.Departures[0]
	m := &s.Map.Movers[dep.Mover-1]
	alice, bob := s.join("10.0.0.1"), s.join("10.0.0.2")
	for i, p := range []*Player{alice, bob} {
		p.Name, p.ready, p.Team = []string{"alice", "bob"}[i], true, "vibe"
		s.teleport(p, Vec3{dep.Board[0] - 3 + float64(6*i), dep.Board[1], dep.Board[2]}, -math.Pi/2, false, 0)
		p.inv = s.Weapons.Loadout(p.Team)
	}
	var local *Vec3
	seqA, seqB, checked, worstWorld := 0, 0, 0, 0.0
	for tick := 1; tick <= int((m.Hide[0]-1)*SimHz); tick++ {
		vt := float64(s.tick) - drawnDelay
		var ca, cb []Cmd
		for seqA < tick+(tick%4) { // alice: bunched, ahead of the server by up to 3 commands
			seqA++
			ca = append(ca, Cmd{Seq: seqA, Yaw: -math.Pi / 2, VT: vt})
		}
		for seqB < tick-(tick/3)%3 { // bob: up to 2 behind
			seqB++
			cb = append(cb, Cmd{Seq: seqB, Yaw: -math.Pi / 2, VT: vt})
		}
		s.queueCmds(alice, ca)
		s.queueCmds(bob, cb)
		s.Step(1)
		ph := math.Mod(alice.cmdTime(alice.ackSeq), m.Period)
		if ph < dep.Close+0.5 {
			continue // docked
		}
		if bob.Frame != dep.Mover || bob.Local == nil {
			t.Fatalf("tick %d (%.2f s into the cycle): bob at %v is in frame %d, want the cabin's %d", tick, ph, bob.Pos, bob.Frame, dep.Mover)
		}
		if local == nil {
			l := *bob.Local
			local = &l
		}
		if d := dist(*bob.Local, *local); d > 0.002 {
			t.Fatalf("tick %d: bob's place in the cabin moved %.3f m (standing still)", tick, d)
		}
		// Alice's next command at its time: bob where the cabin is then.
		ft := alice.cmdTime(alice.ackSeq + 1)
		want := Vec3(sim.FromFrameLocal(s.Map.Movers, dep.Mover, ft, sim.Vec3(*local)))
		rew := s.posesAt(vt, ft)[bob.ID]
		if d := dist(rew.Pos, want); d > 0.002 {
			t.Fatalf("tick %d: a shot rewound to %.1f sees bob at %v, %.3f m from his place in the cabin %v", tick, vt, rew.Pos, d, want)
		}
		worstWorld = math.Max(worstWorld, dist(s.history.At(s.rewindTick(vt))[bob.ID].Pos, want))
		if got := s.peerPos(bob, ft); dist(Vec3(got), want) > 0.002 {
			t.Fatalf("tick %d: alice bumps into bob at %v, want %v", tick, got, want)
		}
		checked++
	}
	if checked < 300 {
		t.Fatalf("only %d ticks in flight checked", checked)
	}
	if worstWorld < 1 {
		t.Errorf("in the world (History.At) bob was at most %.2f m off: the test doesn't show the clocks apart", worstWorld)
	}
	t.Logf("%d ticks in flight; the world rewind was up to %.1f m off the cabin, the frame's 0", checked, worstWorld)
}

func dist(a, b Vec3) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
}

// History.AtIn: a rider in one frame in both snapshots is interpolated there
// and placed; one entering a frame between them, in the world; place nil: At.
func TestHistoryAtInFrames(t *testing.T) {
	var h History
	poses := func(x float64) map[int]PoseState {
		return map[int]PoseState{1: {Pos: Vec3{x, 0, 0}}, 2: {Pos: Vec3{x, 5, 0}}}
	}
	h.RecordFrames(10, poses(0), map[int]framePos{1: {3, Vec3{1, 0, 0}}})
	h.RecordFrames(12, poses(40), map[int]framePos{1: {3, Vec3{1.2, 0, 0}}}) // 20 m a tick in the world: a ship
	place := func(fr int, l Vec3) (Vec3, bool) {
		if fr != 3 {
			return l, false
		}
		return Vec3{l[0] + 100, l[1], l[2]}, true
	}
	got := h.AtIn(11, place)
	if p := got[1].Pos; math.Abs(p[0]-101.1) > 1e-9 || p[1] != 0 {
		t.Errorf("rider: %v, want its place half way (1.1) placed (+100)", p)
	}
	if p := got[2].Pos; p != (Vec3{40, 5, 0}) { // 20 m a tick in the world: a teleport
		t.Errorf("off movers: %v, want the later pose", p)
	}
	if p := h.At(11)[1].Pos; p != (Vec3{40, 0, 0}) {
		t.Errorf("At (no place): %v, want the world's (a teleport at this speed)", p)
	}
	h.RecordFrames(14, poses(40.2), map[int]framePos{1: {4, Vec3{9, 9, 9}}}) // another frame (one the place doesn't know)
	if p := h.AtIn(13, place)[1].Pos; math.Abs(p[0]-40.1) > 1e-9 {
		t.Errorf("changing frames: %v, want interpolated in the world", p)
	}
}
