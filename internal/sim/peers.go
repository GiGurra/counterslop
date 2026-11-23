package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/detmath"

	"github.com/gigurra/counterslop/internal/content"
)

// Player <-> player collision, CS 1.6 style. The other living players are
// solid boxes (R wide, their H tall) that a body walks into, steps onto only
// within a step (never a 1.2 m crouched player) and lands on: you can stand on
// a head. Nothing carries or shoves: a head that walks away drops you, and a
// body is only ever moved by its own command (and the movers).
//
// Touching is not overlapping: a peer whose hull overlaps the body by at most
// peerSlop when its Step starts (two players pressed together see each
// other's hulls at rounded positions, up to a millimetre inside) stays solid,
// and the body is first set back onto its face (the least deep side, if the
// world has room there).
//
// Never stuck: a peer whose hull overlaps the body deeper than that (spawns,
// teleports, a mover's shove, mp_solid_players turned on) is not solid for
// that Step, and pushes the body gently away from its centre (pushOut, never
// into the world); the peer, stepping its own body, is pushed the other way.
// Meanwhile the body can move away from it but not deeper into it (moveH):
// an overlap never lets anyone walk through. Once separated it is solid again.
//
// Positions are rounded to the millimetre (RoundMM), as every snapshot sends
// them: the server and the client's prediction (which only knows the snapshot)
// collide with the same hulls, bit for bit.

// Peer is another player's body as the stepped one sees it.
type Peer struct {
	Pos Vec3    `json:"pos"` // feet centre
	H   float64 `json:"h"`   // H or HCrouch
	// Lo: the peer's player ID is lower than the stepped body's. Breaks the tie
	// when the two stand exactly on one spot: they are pushed opposite ways.
	Lo bool `json:"lo"`
}

const (
	peerReach = 2.0 // peers farther than this from the body are skipped
	pushOut   = 1.5 // m/s an overlapping peer pushes the body out
	pushTie   = 1e-3
	peerSlop  = 0.01 // m of overlap that still counts as touching (resolved to the face)
)

// RoundMM rounds v to the millimetre, halves away from zero (= Go's
// math.Round(v*1000)/1000, which the translator has no JS twin for).
func RoundMM(v float64) float64 {
	x := float64(v * 1000)
	t := math.Trunc(x)
	if math.Abs(x-t) >= 0.5 {
		if x < 0 {
			t--
		} else {
			t++
		}
	}
	return t / 1000
}

// PeerBox is the hull of a player standing at pos (rounded) with height h.
func PeerBox(pos Vec3, h float64, out *content.Box) {
	x, y, z := RoundMM(pos[0]), RoundMM(pos[1]), RoundMM(pos[2])
	out.Min[0], out.Min[1], out.Min[2] = x-R, y, z-R
	out.Max[0], out.Max[1], out.Max[2] = x+R, y+h, z+R
	out.Kind = ""
}

// peers sets the hulls s collides with in this Step: the peers near it that
// it doesn't already overlap deeper than peerSlop (a shallower overlap is
// resolved first: s is set onto the peer's face). The deeper ones are w.ghost.
func (w *World) peers(s *Body) {
	w.peer, w.ghost = w.peer[:0], w.ghost[:0]
	for i := range w.Peers {
		var b content.Box
		PeerBox(w.Peers[i].Pos, w.Peers[i].H, &b)
		r := R + peerReach
		if s.Pos[0]+r < b.Min[0] || s.Pos[0]-r > b.Max[0] || s.Pos[2]+r < b.Min[2] || s.Pos[2]-r > b.Max[2] ||
			s.Pos[1]+s.H+peerReach < b.Min[1] || s.Pos[1]-peerReach > b.Max[1] {
			continue
		}
		if w.overlaps(s.Pos[0], s.Pos[1], s.Pos[2], &b, s.H) && !w.touchOut(s, &b) {
			w.ghost = append(w.ghost, b)
			continue // inside each other: not solid (unstick pushes them apart)
		}
		w.peer = append(w.peer, b)
	}
}

// touchOut sets s onto the face of peer hull b it overlaps least, if that is
// at most peerSlop deep and the map, the movers (w.cur) and the peers kept so
// far leave room there: the peer is then only touching. Sideways or up (onto
// a head), never down.
func (w *World) touchOut(s *Body, b *content.Box) bool {
	best, ax, to := peerSlop, -1, 0.0
	try := func(a int, d, p float64) {
		if d <= best {
			best, ax, to = d, a, p
		}
	}
	try(0, float64(s.Pos[0]+R)-b.Min[0], b.Min[0]-R)
	try(0, b.Max[0]-float64(s.Pos[0]-R), b.Max[0]+R)
	try(2, float64(s.Pos[2]+R)-b.Min[2], b.Min[2]-R)
	try(2, b.Max[2]-float64(s.Pos[2]-R), b.Max[2]+R)
	try(1, b.Max[1]-s.Pos[1], b.Max[1])
	if ax < 0 {
		return false
	}
	p := s.Pos
	p[ax] = to
	for i := range w.Boxes {
		if w.overlaps(p[0], p[1], p[2], &w.Boxes[i], s.H) {
			return false
		}
	}
	for i := range w.cur {
		if w.overlaps(p[0], p[1], p[2], &w.cur[i], s.H) {
			return false
		}
	}
	for i := range w.peer {
		if w.overlaps(p[0], p[1], p[2], &w.peer[i], s.H) {
			return false
		}
	}
	s.Pos[ax] = to
	return true
}

// deeper: moving s along axis ax from old took it closer to the centre of a
// peer it overlaps (w.ghost). moveH then keeps it at old.
func (w *World) deeper(s *Body, ax int, old float64) bool {
	for i := range w.ghost {
		b := &w.ghost[i]
		if !w.overlaps(s.Pos[0], s.Pos[1], s.Pos[2], b, s.H) {
			continue
		}
		c := float64((b.Min[ax] + b.Max[ax]) / 2)
		if math.Abs(s.Pos[ax]-c) < math.Abs(old-c) {
			return true
		}
	}
	return false
}

// unstick pushes s out of every peer it overlaps (already left out of
// w.peer) by pushOut*dt, away from the peer's centre in x/z (exactly on it:
// along x, the lower ID to -x). Only its position moves (its velocity is
// kept), and only as far as the world lets it.
func (w *World) unstick(s *Body, dt float64) {
	for i := range w.Peers {
		var b content.Box
		PeerBox(w.Peers[i].Pos, w.Peers[i].H, &b)
		if !w.overlaps(s.Pos[0], s.Pos[1], s.Pos[2], &b, s.H) {
			continue
		}
		dx, dz := s.Pos[0]-RoundMM(w.Peers[i].Pos[0]), s.Pos[2]-RoundMM(w.Peers[i].Pos[2])
		d := detmath.Hypot(dx, dz)
		if d < pushTie {
			dx, dz, d = 1, 0, 1
			if !w.Peers[i].Lo {
				dx = -1
			}
		}
		step := pushOut * dt
		vel := s.Vel
		w.moveH(s, 0, float64(dx/d)*step)
		w.moveH(s, 2, float64(dz/d)*step)
		s.Vel = vel
	}
}

// Touching: the body's hull touches (or is within d of) a peer's hull this
// Step collides with. For tests and the server's contact bookkeeping.
func (w *World) Touching(s *Body, d float64) bool {
	for i := range w.peer {
		b := &w.peer[i]
		if s.Pos[0]+R+d > b.Min[0] && s.Pos[0]-R-d < b.Max[0] && s.Pos[1]+s.H+d > b.Min[1] && s.Pos[1]-d < b.Max[1] &&
			s.Pos[2]+R+d > b.Min[2] && s.Pos[2]-R-d < b.Max[2] {
			return true
		}
	}
	return false
}
