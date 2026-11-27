package game

import (
	"github.com/gigurra/counterslop/internal/sim"
)

// Frames of reference (docs/physics.md "Frames of reference", internal/sim
// frames.go): a player riding a mover (MVP-1's cabin, a lift) is replicated
// as its frame and its place in it (Player.Frame, Local) besides its world
// position. Clients interpolate it in the frame and place it where they draw
// the mover; the lag-compensation rewind (History.AtIn) and the peers a
// command collides with (peersOf) place it the same way, at the command's
// time, so what is hit and bumped into is what was drawn.

// frameOf sets p's frame and its place in it from where its body is now (the
// end of its last command run, on its command clock). Off movers, not a
// body (dead, spectating: solid), driving a car or in a boat (its own frame:
// boats.go): none.
func (s *Server) frameOf(p *Player) {
	p.Frame, p.Local = 0, nil
	if p.body == nil || !solid(p) || p.car != 0 || p.ride != 0 || len(s.Map.Movers) == 0 {
		return
	}
	t := p.cmdTime(p.ackSeq - p.skipped + 1) // the commands acknowledged but not run yet: coast steps it through them first
	fr := sim.FrameOf(s.Map.Movers, s.Map.GravZones, p.body, t)
	if fr == 0 {
		return
	}
	l := Vec3(sim.ToFrameLocal(s.Map.Movers, fr, t, sim.Vec3(p.Pos)))
	for i := range 3 { // what the client gets: millimetres (Vec3.MarshalJSON); RoundMM: the case runner's JS gets the same bits
		l[i] = sim.RoundMM(l[i])
	}
	p.Frame, p.Local = fr, &l
}

// updateFrames sets every player's frame (before the snapshot and the
// history are taken). Callers hold s.mu.
func (s *Server) updateFrames() {
	for _, p := range s.players {
		s.frameOf(p)
	}
}

// placeAt places a pose's frame-local position at time t on the movers'
// clock (History.AtIn's place).
func (s *Server) placeAt(t float64) func(fr int, l Vec3) (Vec3, bool) {
	if len(s.Map.Movers) == 0 {
		return nil
	}
	return func(fr int, l Vec3) (Vec3, bool) {
		if fr <= 0 || fr > len(s.Map.Movers) {
			return l, false
		}
		return Vec3(sim.FromFrameLocal(s.Map.Movers, fr, t, sim.Vec3(l))), true
	}
}

// peerPos is where q is for a command at time ft on the movers' clock: a
// rider of a mover at its place in the frame, the frame where it is at ft
// (as the client predicts it: predict.js peersFrom); anyone else where it is.
func (s *Server) peerPos(q *Player, ft float64) sim.Vec3 {
	if q.Frame > 0 && q.Local != nil && q.Frame <= len(s.Map.Movers) {
		return sim.FromFrameLocal(s.Map.Movers, q.Frame, ft, sim.Vec3(*q.Local))
	}
	return sim.Vec3(q.Pos)
}
