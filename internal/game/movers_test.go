package game

import (
	"encoding/json"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// A floor and a platform sliding 4 m along x and back every 8 s.
func sliderMap() content.Map {
	return content.Map{Name: "slider", Boxes: []content.Box{content.B(-30, -1, -30, 30, 0, 30, content.Concrete)},
		Spawns: [][3]float64{{0, 0.3, 0}},
		Movers: []content.Mover{{Kind: content.MoverSlide, Mat: content.Metal, Min: [3]float64{-1, 0, -1}, Max: [3]float64{1, 0.3, 1}, Move: [3]float64{4, 0, 0}, Period: 8}}}
}

// The server runs each command with the movers at that command's time on the
// player's command clock (CmdTime(tickBase, seq)), whatever server tick it
// runs in: the client's prediction (same clock, from me.tb) agrees bit for bit.
func TestCommandsRunOnTheCommandClock(t *testing.T) {
	s := authServer(t)
	s.Map = sliderMap()
	p := s.join("1.1.1.1")
	p.ready, p.Team = true, content.Vibe
	p.Pos = Vec3{0, 0.3, 0}
	for range 7 {
		stepSim(s)
	}
	s.queueCmds(p, []Cmd{{Seq: 1}})
	tb := p.tickBase
	if tb != s.tick {
		t.Fatalf("tick base %d on joining at tick %d, want the next command to run at the next tick", tb, s.tick)
	}
	// The client's prediction of the same commands.
	m := sliderMap()
	w := sim.NewWorld(&m)
	b := sim.NewBody(sim.Vec3{0, 0.3, 0})
	predict := func(seq int) {
		w.T = CmdTime(tb, seq)
		w.Step(b, MoveInput(Cmd{Seq: seq}, 0, b), 1.0/SimHz)
	}
	predict(1)
	// Commands arrive late and in bursts: they still run at their own times.
	seq := 1
	for burst := range 20 {
		var cmds []Cmd
		for range burst % 4 {
			seq++
			cmds = append(cmds, Cmd{Seq: seq})
			predict(seq)
		}
		s.queueCmds(p, cmds)
		stepSim(s)
	}
	for range 40 { // the backlog (bursts faster than real time) runs on the budget
		stepSim(s)
	}
	if p.tickBase != tb {
		t.Fatalf("tick base moved from %d to %d", tb, p.tickBase)
	}
	if p.ackSeq != seq || p.body.Pos != b.Pos {
		t.Fatalf("server ran to %d at %v, the prediction is at %v", p.ackSeq, p.body.Pos, b.Pos)
	}
	if b.Pos[0] < 0.3 {
		t.Fatalf("the platform didn't carry the body: %v", b.Pos)
	}
	// The tick base goes to the client in every self-snapshot.
	var me meState
	if err := json.Unmarshal(s.meBytes(p), &me); err != nil || me.TB != tb {
		t.Fatalf("me.tb %d (%v), want %d", me.TB, err, tb)
	}
}

// A client skipping seqs can't move its own movers' clock: past the resend
// window the next command runs one tick after the last one run.
func TestSeqJumpDoesNotJumpTheClock(t *testing.T) {
	s := authServer(t)
	s.Map = sliderMap()
	p := s.join("1.1.1.1")
	p.ready, p.Team = true, content.Vibe
	s.queueCmds(p, []Cmd{{Seq: 1}, {Seq: 2}})
	stepSim(s)
	stepSim(s)
	last := p.cmdTime(2)
	s.queueCmds(p, []Cmd{{Seq: 5}}) // two lost: the clock keeps the seqs
	stepSim(s)
	if got := p.cmdTime(5); got != last+0.03 {
		t.Fatalf("after a small gap command 5 runs at %v, want %v", got, last+0.03)
	}
	last = p.cmdTime(5)
	s.queueCmds(p, []Cmd{{Seq: 5 + 100000}})
	stepSim(s)
	if p.ackSeq != 100005 {
		t.Fatalf("ack %d", p.ackSeq)
	}
	if got, want := p.cmdTime(100005), CmdTime(int64(last*SimHz+0.5), 1); got != want {
		t.Fatalf("after a jump of 100000 seqs command 100005 runs at %v, want %v (one tick on)", got, want)
	}
}

// Every spawn restarts the command clock at the server's tick (the client
// replays its commands in flight on the new base from the next snapshot).
func TestSpawnRebasesTheCommandClock(t *testing.T) {
	s := authServer(t)
	s.Map = sliderMap()
	p := s.join("1.1.1.1")
	p.ready, p.Team = true, content.Vibe
	s.queueCmds(p, []Cmd{{Seq: 1}, {Seq: 2}, {Seq: 3}})
	for range 300 {
		stepSim(s)
	}
	for len(p.send) > 0 {
		<-p.send
	}
	s.respawn(p)
	var spawn map[string]any
	for len(p.send) > 0 {
		if m := reply(t, p); m["t"] == "spawn" {
			spawn = m
		}
	}
	if spawn == nil || spawn["tb"] != float64(p.tickBase) {
		t.Fatalf("spawn message %v, want tb %d (the client predicts on it before the next snapshot)", spawn, p.tickBase)
	}
	if p.cmdTime(p.ackSeq+1) != CmdTime(s.tick, 1) {
		t.Fatalf("after the respawn the next command runs at %v, want the next tick %v", p.cmdTime(p.ackSeq+1), CmdTime(s.tick, 1))
	}
}

// A command clock that fell behind (no commands for a while: a hidden tab)
// is put back in step with the server's, but only away from movers: on one,
// moving the clock would move what the body stands on.
func TestCommandClockCatchesUpAwayFromMovers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		at     Vec3
		rebase bool
	}{{"far", Vec3{20, 0, 20}, true}, {"on the platform", Vec3{0, 0.3, 0}, false}} {
		t.Run(tc.name, func(t *testing.T) {
			s := authServer(t)
			s.Map = sliderMap()
			p := s.join("1.1.1.1")
			p.ready, p.Team = true, content.Vibe
			p.Pos = tc.at
			s.queueCmds(p, []Cmd{{Seq: 1}})
			stepSim(s)
			tb := p.tickBase
			for range 200 { // 2 s without commands
				stepSim(s)
			}
			s.queueCmds(p, []Cmd{{Seq: 2}})
			stepSim(s)
			if tc.rebase && p.cmdTime(2) != CmdTime(s.tick, 0) {
				t.Fatalf("command 2 ran at %v, want back in step at %v", p.cmdTime(2), CmdTime(s.tick, 0))
			}
			if !tc.rebase && p.tickBase != tb {
				t.Fatalf("tick base moved from %d to %d on a mover", tb, p.tickBase)
			}
		})
	}
}
