package game

import (
	"fmt"
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/sim"
)

// rideLink is how a rider's commands reach a prod-like server (not test
// mode: the real command budget, overflow drops and backlog trims).
type rideLink struct {
	rate   float64                        // commands the client makes a server tick (its clock: 1.05 runs 5% fast)
	arrive func(tick int) int             // how many of those the server has by tick (nil: all)
	lost   func(seq int) bool             // never arrives, not even resent (past the resend window)
	peers  func(s *Server, at [3]float64) // others aboard
}

// A rider on MVP-1 through a whole flight, standing or walking the aisle,
// whatever the link does to its commands: it stays in the cabin from the
// doors shutting until the floor opens over the Moon. On the test server the
// operator was thrown out early in the climb: commands the server skips (a
// backlog trimmed, a queue overflowing, commands lost past the resend
// window) jumped the rider's command clock, the cabin climbed metres in that
// one step (60-80 m/s) and the body, not carried across the jump, fell.
func TestRideRealtime(t *testing.T) {
	links := map[string]rideLink{
		"steady":     {rate: 1},
		"60fps":      {rate: 1, arrive: func(tick int) int { return tick / 5 * 5 / 3 * 3 }},
		"bunched":    {rate: 1, arrive: func(tick int) int { return tick / 8 * 8 }},
		"jitter":     {rate: 1, arrive: func(tick int) int { return max(0, tick-3*((tick*7919)%11)) }},
		"stall300ms": {rate: 1, arrive: func(tick int) int { return tick - min(30, max(0, tick%700-670)) }},
		"clock5fast": {rate: 1.05},                                               // a backlog that never drains: trimmed
		"lossy":      {rate: 1, lost: func(seq int) bool { return seq%97 < 12 }}, // twelve in a row lost now and then
		"bots aboard": {rate: 1, peers: func(s *Server, at [3]float64) {
			for range 3 {
				s.addAI("vibe")
			}
			for _, q := range s.ordered() {
				if q.ai != nil {
					q.Pos = Vec3{at[0] + 3, at[1], at[2] + 1}
				}
			}
		}},
	}
	for _, mapName := range []string{"lab_moon", "cq_moonshot"} {
		for name, link := range links {
			for _, walk := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/walk=%v", mapName, name, walk), func(t *testing.T) {
					t.Parallel()
					rideRealtime(t, mapName, link, walk)
				})
			}
		}
	}
}

func rideRealtime(t *testing.T, mapName string, link rideLink, walk bool) {
	s := authServer(t)
	if err := s.ResetWorldOn(mapName); err != nil {
		t.Fatal(err)
	}
	if len(s.Map.Departures) == 0 {
		t.Fatal("no departure")
	}
	dep := s.Map.Departures[0]
	m := &s.Map.Movers[dep.Mover-1] // the cabin floor
	p := s.join("10.0.0.1")
	p.Name, p.ready, p.Team = "alice", true, "vibe"
	s.teleport(p, Vec3(dep.Board), -math.Pi/2, false, 0)
	p.inv = s.Weapons.Loadout(p.Team)
	if link.peers != nil {
		link.peers(s, dep.Board)
	}
	var sent []Cmd
	queued := 0
	worst, worstT := 0.0, 0.0
	ticks := int((m.Hide[0] + 1) * SimHz)
	for tick := 1; tick <= ticks; tick++ {
		for len(sent) < int(float64(tick)*link.rate) {
			seq := len(sent) + 1
			c := Cmd{Seq: seq, Yaw: -math.Pi / 2}
			if walk && seq > 100 {
				c.F = []float64{1, -1}[(seq/120)%2] // up and down the aisle
			}
			sent = append(sent, c)
		}
		n := len(sent)
		if link.arrive != nil {
			n = min(link.arrive(tick), n)
		}
		if n > queued {
			var batch []Cmd
			for _, c := range sent[max(0, n-maxQueued):n] { // a client resends what is unacked; the server keeps the newest
				if link.lost == nil || !link.lost(c.Seq) {
					batch = append(batch, c)
				}
			}
			s.queueCmds(p, batch)
			queued = n
		}
		s.Step(1)
		ct := p.cmdTime(p.ackSeq-p.skipped) + 1.0/SimHz                             // where the floor is after the last command run (not the trimmed ones after it)
		if ph := math.Mod(ct, m.Period); ph > dep.Close+0.5 && ph < m.Hide[0]-0.1 { // in flight, doors shut: inside the cabin?
			off := sim.MoverOffset(m, ct)
			dx := math.Max(m.Min[0]+off[0]-p.Pos[0], p.Pos[0]-(m.Max[0]+off[0]))
			dy := math.Abs(p.Pos[1] - (m.Max[1] + off[1]))
			dz := math.Max(m.Min[2]+off[2]-p.Pos[2], p.Pos[2]-(m.Max[2]+off[2]))
			if d := math.Max(math.Max(dx, dz), dy); d > worst {
				worst, worstT = d, ph
			}
		}
	}
	if worst > 0.5 {
		t.Errorf("thrown out of the cabin: %.1f m off its floor at %.2f s into the cycle (dropped %d)", worst, worstT, p.cmdsDropped)
	}
}
