package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// Movers push and crush (docs/physics.md "Movers push and crush"). Every
// sub-step each slide or path mover box near the body is swept against it: in
// the box's frame the body moves by minus the box's travel relative to it
// (dr), from where it was to where it is, and if that segment enters the
// box's hull-expanded volume the box hit the body this sub-step, however fast
// it moved (a box moving 1.7 m in a sub-step at 200 m/s no longer skips past
// a body 0.8 m wide). The body is then pushed out along the normal of the
// face that hit it, to that face (frictionless: where the box's motion takes
// its face past the body's side, the body slides off it and is pushed only as
// far as the face swept it). Any direction, up and down included: a lift
// rising into a body lifts it, a ceiling coming down pushes it into the floor.
//
// The push never goes through anything (room): it stops at the first static
// box, mover box that doesn't move out of the way, or other player that can't
// be pushed on itself (chained, chainMax deep) along the way. What the push
// can't do is crush depth (Events.Crushed, metres: the server's damage),
// unless the body is mostly off the pushing face: then it slides off it
// sideways instead (slideOff). A crushed body stays where the push stopped,
// never in or past a wall, and the box that crushed it is not solid to it for
// the rest of the sub-step (it is inside the body; resolving it as a wall
// would throw the body through the wall).
//
// Other players are pushed in a chain: a peer the mover hits (as the body
// would be) is a pusher of its own for this body, its hull placed where the
// mover puts it, moving with the mover's normal travel. Each body steps on
// its own, so each sees the chain from the same data (the mover's clock and
// the peers' rounded positions), server and prediction alike.
//
// Escalator treads keep the older shove (movers.go rideMovers): a conveyor,
// whose treads overlap each other, pushes only where there is room.

const (
	pushTol    = 0.002 // a peer this close ahead of a mover's face is pushed by it (positions are mm-rounded)
	pushPasses = 4     // push rounds per sub-step (pushed by one box into the next)
	chainMax   = 4     // how many players deep a push chains
)

// pusher is one box that pushes this sub-step: a mover box (dyn: its index
// in w.dyn) or a peer pushed by one (dyn -1).
type pusher struct {
	B1   content.Box // at the sub-step's end
	D    Vec3        // its travel over the sub-step (world: the body's carry not taken off)
	dyn  int
	done bool // crushed the body: no more pushes from it this sub-step
	ax   int  // a peer's: the axis it is pushed along
	lo   bool // a peer's: its ID is lower than the stepped body's (peers.go Lo)
}

// sweepHit: box b, having moved by dr relative to a body with its feet at p
// (h tall) over the sub-step, passed through it. ax is the axis of the face
// that hit it, to the body's place on that axis after the push. tol widens
// the box's leading faces (a peer just ahead of it counts as hit). A body
// already in it as the sub-step began is pushed out along its main travel.
func sweepHit(p Vec3, h float64, b *content.Box, dr Vec3, tol float64) (bool, int, float64) {
	var lo, hi, out Vec3
	lo[0], lo[1], lo[2] = b.Min[0]-R, b.Min[1]-h, b.Min[2]-R
	hi[0], hi[1], hi[2] = b.Max[0]+R, b.Max[1], b.Max[2]+R
	u0, u1, ax := math.Inf(-1), math.Inf(1), -1
	for a := range 3 {
		l, u := lo[a]+eps, hi[a]-eps
		if dr[a] > 0 {
			u += tol
		} else if dr[a] < 0 {
			l -= tol
		}
		out[a] = math.Inf(1)
		if dr[a] == 0 {
			if p[a] <= l || p[a] >= u {
				return false, 0, 0
			}
			continue
		}
		// The body's place in the box's frame: q(s) = p + (1-s)*dr, s 0..1.
		ea, eb := 1-(u-p[a])/dr[a], 1-(l-p[a])/dr[a] // entering, leaving (dr > 0)
		if dr[a] < 0 {
			ea, eb = eb, ea
		}
		out[a] = eb
		if ea > u0 {
			u0, ax = ea, a
		}
		u1 = math.Min(u1, eb)
	}
	if math.Max(u0, 0) >= math.Min(u1, 1) {
		return false, 0, 0
	}
	if u0 <= 0 { // in it as the sub-step began (crushed): out along its main travel
		ax = 0
		for a := 1; a < 3; a++ {
			if math.Abs(dr[a]) > math.Abs(dr[ax]) {
				ax = a
			}
		}
	}
	face := lo[ax]
	if dr[ax] > 0 {
		face = hi[ax]
	}
	us := 1.0 // when the face's motion took it past the body's side (it slides off)
	for a := range 3 {
		if a != ax {
			us = math.Min(us, out[a])
		}
	}
	to := face
	if us < 1 {
		to = face - float64((1-us)*dr[ax])
	}
	if dr[ax] > 0 && to < p[ax] || dr[ax] < 0 && to > p[ax] {
		to = p[ax] // a peer touching its face (within tol), or rounding: pushed from where it is
	}
	return true, ax, to
}

// hullAt is the hull of a body with its feet at p, h tall (unrounded).
func hullAt(p Vec3, h float64, out *content.Box) {
	out.Min[0], out.Min[1], out.Min[2] = p[0]-R, p[1], p[2]-R
	out.Max[0], out.Max[1], out.Max[2] = p[0]+R, p[1]+h, p[2]+R
	out.Kind = ""
}

// across: a and b overlap on the two axes other than ax.
func across(a, b *content.Box, ax int) bool {
	for c := range 3 {
		if c != ax && (a.Max[c] <= b.Min[c]+eps || a.Min[c] >= b.Max[c]-eps) {
			return false
		}
	}
	return true
}

func boxesOverlap(a, b *content.Box) bool {
	return across(a, b, -1)
}

// ahead is how far hull hb can move along ax (want: signed) before box b
// stops it: want when b isn't in the way.
func ahead(hb, b *content.Box, ax int, want float64) float64 {
	if !across(hb, b, ax) {
		return want
	}
	if want > 0 && b.Min[ax] >= hb.Max[ax]-eps {
		return math.Min(want, math.Max(0, b.Min[ax]-hb.Max[ax]))
	}
	if want < 0 && b.Max[ax] <= hb.Min[ax]+eps {
		return math.Max(want, math.Min(0, b.Max[ax]-hb.Min[ax]))
	}
	return want
}

// room is how far hull hb (a body's) can be pushed along ax, up to want
// (signed), by a pusher moving pd along ax: as far as anything that won't
// give lets it. That is the first static box, mover box (not self, the
// pusher's dyn) that doesn't move out of the way as fast, or peer that can't
// be pushed on as far itself (depth: the chain so far); what it leaves of
// want is crushed. Nothing it is already in lets it go deeper (blockedBy). A peer that can be pushed on doesn't stop it: the body
// may end up in it for the rest of this Step (not solid to it, peers.go), and
// that peer, stepping its own body, is pushed out ahead of it by its chain
// (setPushers, behind).
func (w *World) room(hb *content.Box, ax int, want, pd float64, self, depth int) float64 {
	got := want
	feet := Vec3{float64((hb.Min[0] + hb.Max[0]) * 0.5), hb.Min[1], float64((hb.Min[2] + hb.Max[2]) * 0.5)}
	h := hb.Max[1] - hb.Min[1]
	for i := range w.Boxes {
		b := &w.Boxes[i]
		if IsWedge(b) {
			if w.overlaps(feet[0], feet[1], feet[2], b, h) {
				continue
			}
			end := feet
			end[ax] += got
			if w.overlaps(end[0], end[1], end[2], b, h) && (!Walkable(b) || wedgeTop(b, end[0], end[2], R)-end[1] > stepUp) {
				got = 0 // a wall of a slope in the way: no room (conservative); a ramp it goes up (moveV puts it on it)
			}
			continue
		}
		got = blockedBy(hb, b, ax, got)
	}
	for i := range w.dyn {
		d := &w.dyn[i]
		if i == self || !d.On1 {
			continue
		}
		if want > 0 && d.D[ax] >= pd-1e-9 || want < 0 && d.D[ax] <= pd+1e-9 {
			continue // moving out of the way as fast (another box of the same ship): it pushes on itself
		}
		got = blockedBy(hb, &d.B1, ax, got)
	}
	if w.fg != GravDown || depth >= chainMax {
		return got
	}
	for i := range w.Peers {
		if w.Peers[i].Drawn || w.Peers[i].G != GravDown {
			continue
		}
		var pb content.Box
		HullBox(w.Peers[i].Pos, w.Peers[i].H, GravDown, &pb)
		if boxesOverlap(hb, &pb) {
			continue
		}
		lim := ahead(hb, &pb, ax, got)
		if lim != got {
			got = lim + w.room(&pb, ax, got-lim, pd, -1, depth+1) // as far as it can be pushed on
		}
	}
	return got
}

// blockedBy is how far of got (signed) along ax box b lets hull hb go: up to
// its face when it is ahead; when hb is already in it (crushed into it, or
// embedded by rounding), out of it but not deeper (towards its centre).
func blockedBy(hb, b *content.Box, ax int, got float64) float64 {
	if !boxesOverlap(hb, b) {
		return ahead(hb, b, ax, got)
	}
	c, hc := float64((b.Min[ax]+b.Max[ax])*0.5), float64((hb.Min[ax]+hb.Max[ax])*0.5)
	if got > 0 && c > hc || got < 0 && c < hc {
		return 0
	}
	return got
}

// behind: pushed peer pj pushes the body with its feet at p only if the
// body is ahead of it along its push: its centre further along, or level
// with it and the peer's ID the lower (the lower ID nearer the mover). Two
// bodies a mover put in one place (the first to step went to the mover's
// face past the other) part the same way whichever steps first.
func behind(p Vec3, h float64, pj *pusher) bool {
	a := pj.ax
	c, me := float64((pj.B1.Min[a]+pj.B1.Max[a])*0.5), p[a]
	if a == 1 {
		c, me = pj.B1.Min[1], p[1]
	}
	if pj.D[a] < 0 {
		me, c = -me, -c
	}
	return me > c+pushTol || math.Abs(me-c) <= pushTol && pj.lo // level within the peers' mm rounding
}

// pushers sets w.pushers for this sub-step: the slide and path mover boxes
// that move, then the peers they push (a chain, chainMax deep).
func (w *World) setPushers() {
	w.pushers = w.pushers[:0]
	for i := range w.dyn {
		d := &w.dyn[i]
		if d.Esc || !d.On1 || d.D[0] == 0 && d.D[1] == 0 && d.D[2] == 0 {
			continue
		}
		w.pushers = append(w.pushers, pusher{B1: d.B1, D: d.D, dyn: i, done: false, ax: 0, lo: false})
	}
	if len(w.pushers) == 0 || len(w.Peers) == 0 || w.fg != GravDown {
		return
	}
	w.pushed = w.pushed[:0]
	for range w.Peers {
		w.pushed = append(w.pushed, false)
	}
	for range chainMax {
		n := len(w.pushers)
		for i := range w.Peers {
			if w.pushed[i] || w.Peers[i].Drawn || w.Peers[i].G != GravDown {
				continue
			}
			var pb content.Box
			HullBox(w.Peers[i].Pos, w.Peers[i].H, GravDown, &pb)
			feet := Vec3{float64((pb.Min[0] + pb.Max[0]) * 0.5), pb.Min[1], float64((pb.Min[2] + pb.Max[2]) * 0.5)}
			for j := range n {
				pj := &w.pushers[j]
				if pj.dyn < 0 && !behind(feet, pb.Max[1]-pb.Min[1], pj) {
					continue
				}
				hit, ax, to := sweepHit(feet, pb.Max[1]-pb.Min[1], &pj.B1, pj.D, pushTol)
				if !hit {
					continue
				}
				mv := w.room(&pb, ax, to-feet[ax], pj.D[ax], pj.dyn, chainMax) // where it really goes (static only): never a pusher through a wall
				var q pusher
				q.B1, q.dyn, q.done, q.ax, q.lo = pb, -1, false, ax, w.Peers[i].Lo
				q.B1.Min[ax] += mv
				q.B1.Max[ax] += mv
				q.D[ax] = pj.D[ax]
				w.pushers = append(w.pushers, q)
				w.pushed[i] = true
				break
			}
		}
		if len(w.pushers) == n {
			break
		}
	}
}

// push runs the sub-step's swept pushes on s; carried is how far riding
// moved it already (taken off each box's travel: it moved with them). The
// result is the crush depth (m).
func (w *World) push(s *Body, carried Vec3) float64 {
	w.setPushers()
	crush := 0.0
	for range pushPasses {
		moved := false
		for j := range w.pushers {
			pj := &w.pushers[j]
			if pj.done {
				continue
			}
			dr := Vec3{pj.D[0] - carried[0], pj.D[1] - carried[1], pj.D[2] - carried[2]}
			if pj.dyn < 0 && !behind(s.Pos, s.H, pj) {
				continue
			}
			hit, ax, to := sweepHit(s.Pos, s.H, &pj.B1, dr, 0)
			if !hit {
				continue
			}
			want := to - s.Pos[ax]
			var hb content.Box
			hullAt(s.Pos, s.H, &hb)
			got := w.room(&hb, ax, want, pj.D[ax], pj.dyn, 0)
			if got != want && w.slideOff(s, &pj.B1, dr, ax, pj.D, pj.dyn) {
				moved = true
				continue
			}
			if got != want {
				crush += math.Min(math.Abs(want-got), math.Abs(dr[ax])) // how far it went into the body this sub-step
				pj.done = true
				if pj.dyn >= 0 {
					w.dyn[pj.dyn].Crush = true // in the body now: not solid to it
				}
				s.Pos[ax] += got
			} else {
				s.Pos[ax] = to
			}
			if got != 0 {
				moved = true
			}
			if ax == 1 && want > 0 { // lifted: standing on it
				s.OnGround = true
				s.Vel[1] = math.Max(0, s.Vel[1])
			} else if ax == 1 {
				s.Vel[1] = math.Min(0, s.Vel[1])
			}
		}
		if !moved {
			break
		}
	}
	return crush
}

// slideOff moves s off box b sideways when the push along ax has no room and
// the body is mostly off the face (its centre past the box's side on an axis
// the box doesn't move along): out to that side, if there's room. The
// shortest such way wins.
func (w *World) slideOff(s *Body, b *content.Box, dr Vec3, ax int, pd Vec3, self int) bool {
	best, bc, bd := math.Inf(1), -1, 0.0
	for c := range 3 {
		if c == ax || dr[c] != 0 {
			continue
		}
		lo, hi, half := b.Min[c]-R, b.Max[c]+R, R
		if c == 1 {
			lo, hi, half = b.Min[1]-s.H, b.Max[1], float64(s.H*0.5)
		}
		for k := range 2 {
			to := lo
			if k == 1 {
				to = hi
			}
			d := to - s.Pos[c]
			if math.Abs(d) >= half || math.Abs(d) >= best {
				continue
			}
			var hb content.Box
			hullAt(s.Pos, s.H, &hb)
			if w.room(&hb, c, d, pd[c], self, 0) == d {
				best, bc, bd = math.Abs(d), c, to
			}
		}
	}
	if bc < 0 {
		return false
	}
	s.Pos[bc] = bd
	return true
}

// passed: a mover box of this sub-step crushed the body (it is in it).
func (w *World) passed() bool {
	for i := range w.dyn {
		if w.dyn[i].Crush {
			return true
		}
	}
	return false
}
