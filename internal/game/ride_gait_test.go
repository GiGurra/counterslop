package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/sim"
)

// Passengers standing in a moving ship's cabin don't walk: the walk cycle
// counts the body's move in its frame, not the ship's 60-90 m/s. Before, the
// gait was stepped by the world move: in the cross (sideways: the cycle
// counts ground speed) every standing rider's legs ran at full amplitude
// and 10-100 times run speed, a vibration (the operator: "a walking animation
// at 10-100x speed"). The gait is replicated and part of the hit pose, so the
// pose shot at was that too.
func TestRidersDontWalkWithTheShip(t *testing.T) {
	for _, mapName := range []string{"lab_moon", "cq_moonshot"} {
		t.Run(mapName, func(t *testing.T) {
			s := authServer(t)
			s.Test = true
			if err := s.ResetWorldOn(mapName); err != nil {
				t.Fatal(err)
			}
			dep := s.Map.Departures[0]
			m := &s.Map.Movers[dep.Mover-1]
			alice := s.join("10.0.0.1")
			alice.Name, alice.ready, alice.Team = "alice", true, "vibe"
			alice.inv = s.Weapons.Loadout(alice.Team)
			bot := s.addAI("vibe")
			seq := 0
			cmd := func() { // alice's commands from the start: her clock is the server's
				seq++
				s.queueCmds(alice, []Cmd{{Seq: seq, Yaw: -math.Pi / 2, VT: float64(s.tick) - drawnDelay}})
				s.Step(1)
			}
			for math.Mod(alice.cmdTime(seq+1), m.Period) < dep.Close-0.5 {
				cmd()
			}
			s.teleport(alice, Vec3{dep.Board[0] - 3, dep.Board[1], dep.Board[2]}, -math.Pi/2, false, seq)
			s.teleport(bot, Vec3{dep.Board[0] + 3, dep.Board[1], dep.Board[2] + 1}, 0, false, bot.ai.seq)
			type seen struct {
				amp, phase0, dphase float64
				n                   int
			}
			got := map[string]*seen{}
			for range int((m.Hide[0] + 1) * SimHz) {
				cmd()
				ph := math.Mod(alice.cmdTime(seq), m.Period)
				if ph < dep.Close+1 || ph > m.Hide[0]-0.1 {
					continue
				}
				for _, q := range []*Player{alice, bot} {
					if q.Frame != dep.Mover {
						t.Fatalf("%s is not aboard at %.2f s into the cycle (%v)", q.Name, ph, q.Pos)
					}
					g := got[q.Name]
					if g == nil {
						g = &seen{phase0: q.Gait.Phase}
						got[q.Name] = g
					}
					g.amp = math.Max(g.amp, q.Gait.Amp)
					g.dphase = math.Max(g.dphase, math.Abs(q.Gait.Phase-g.phase0))
					g.n++
				}
			}
			for name, g := range got {
				if g.n < 200 {
					t.Fatalf("%s: only %d ticks in flight", name, g.n)
				}
				if g.amp > 0.02 || g.dphase > 0.01 {
					t.Errorf("%s walked while standing in the cabin: amp up to %.2f (1: running), phase moved %.2f rad", name, g.amp, g.dphase)
				}
			}
		})
	}
}

// GaitMove takes the frame's travel off a rider's move and keeps its own
// steps in the cabin; off movers it is the world move.
func TestGaitMoveInTheFrame(t *testing.T) {
	m := maps.LabMoon()
	dep := m.Departures[0]
	mv := &m.Movers[dep.Mover-1]
	t0 := 23.0 // the cross
	t1 := t0 + 1.0/SimHz
	o0, o1 := sim.MoverOffset(mv, t0), sim.MoverOffset(mv, t1)
	if math.Hypot(o1[0]-o0[0], o1[2]-o0[2]) < 0.2 {
		t.Fatalf("the ship moves only %v in a tick at %.1f s: not the cross", sim.Vec3{o1[0] - o0[0], o1[1] - o0[1], o1[2] - o0[2]}, t0)
	}
	walk := sim.Vec3{0.05, 0, -0.03}
	b := sim.NewBody(sim.Vec3{dep.Board[0] + o1[0], dep.Board[1] + o1[1], dep.Board[2] + o1[2]})
	b.OnGround = true
	from := sim.Vec3{b.Pos[0] - (o1[0] - o0[0]) - walk[0], b.Pos[1] - (o1[1] - o0[1]) - walk[1], b.Pos[2] - (o1[2] - o0[2]) - walk[2]}
	got := GaitMove(m.Movers, m.GravZones, b, from, t0, t1)
	for a := range 3 {
		if math.Abs(got[a]-walk[a]) > 1e-9 {
			t.Fatalf("in the cabin: GaitMove %v, want the walk %v", got, walk)
		}
	}
	off := sim.NewBody(sim.Vec3{0, 0, 0})
	off.OnGround = true
	if got := GaitMove(m.Movers, m.GravZones, off, sim.Vec3{-0.07, 0, 0}, t0, t1); got != (sim.Vec3{0.07, 0, 0}) {
		t.Fatalf("off the ship: GaitMove %v, want the world move", got)
	}
}
