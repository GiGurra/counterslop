package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/detmath"
)

// The Gravity Override's hook (docs/physics.md): a Quake-style grappling
// hook. Press the hook button: a line from the eye along the view latches
// onto the first solid face within HookRules.Range (any solid: boxes and
// wedges), and the body is pulled toward the anchor while the button is held:
// up to Pull m/s along the rope, on a rope that only gets shorter (swing
// under it), gravity still pulling. Let go and the body keeps its momentum.
// Energy: each shot costs Cost, holding drains Drain per second, out of
// energy the hook lets go; it recharges by Recharge per second while not
// hooked, up to Energy.
//
// Touching the face it is hooked to (a box: not a wedge, v1) flips the body's
// gravity into that face (gravity.go): it stands on it. Hook the floor to come
// back down.
//
// Movers (movers.go) are hooked like any solid, at the command's time: the
// anchor rides with the mover (Hook.Mover, U, Local: recomputed from the
// mover's pose every command), and touching any face of a mover flips onto it
// as onto a static one: standing there the body rides it (a rotated frame
// sees the movers move, gravity.go), carried along the face or out and back
// with a face moving along its normal. A tread that slides under its comb
// plate lets go.

// HookRules: the hook's tuning (cvars sv_hook_*, map Rules may pin them).
type HookRules struct {
	Range    float64 // m: how far the hook reaches
	Energy   float64 // a full meter
	Recharge float64 // per second while not hooked
	Drain    float64 // per second while hooked
	Cost     float64 // per shot (hit or miss)
	Pull     float64 // m/s: the speed it pulls up to along the rope
}

// DefaultHook: the defaults (sv_hook_* cvars).
func DefaultHook() HookRules {
	return HookRules{Range: 20, Energy: 100, Recharge: 12, Drain: 20, Cost: 15, Pull: 15}
}

// Hook is a body's hook (the self-snapshot carries it).
type Hook struct {
	On   bool    // latched
	At   Vec3    // the anchor
	N    Vec3    // the face's outward normal (an axis on a box face)
	Lo   Vec3    // the hooked solid's box: where the face is (hookLand)
	Hi   Vec3    //
	Flip bool    // a box face: touching it flips gravity (not a wedge)
	Rope float64 // m: the rope's length so far (never longer)
	// A mover's box (movers.go): Mover is its index + 1 (0: a static box),
	// U the tread (an escalator: its u minus the treads' travel, an integer;
	// a slide: 0), Local the anchor from the box's origin (moverOrigin).
	Mover int
	U     float64
	Local Vec3
}

// Hook events (Events.Hook).
const (
	HookFired    = 1 // latched
	HookMissed   = 2 // nothing in reach (energy spent anyway)
	HookReleased = 3 // let go, or out of energy
	HookFlipped  = 4 // touched the hooked face: gravity flipped onto it
)

// hookAccel: m/s² the hook pulls with, up to Pull (beats gravity).
const hookAccel = 60.0

// hookTouch: m from the hooked face that counts as touching it.
const hookTouch = 0.02

// hook runs the hook for one command (world coordinates): fire, let go,
// drain or recharge, pull. Returns a hook event (0: none).
func (w *World) hook(s *Body, in Input, dt float64) int {
	r := w.Move.Hook
	ev := 0
	if s.Hook.On && s.Hook.Mover > 0 && !w.hookFollow(s, w.T) {
		s.Hook = Hook{}
		return HookReleased
	}
	if !s.Hook.On {
		s.Energy = math.Min(r.Energy, s.Energy+float64(r.Recharge*dt))
	}
	if in.HookPress && !s.Hook.On && r.Range > 0 && s.Energy >= r.Cost {
		s.Energy -= r.Cost
		eye := EyePos(s)
		cp := detmath.Cos(s.Pitch)
		d := FromFrame(s.Grav, Vec3{-float64(detmath.Sin(s.Yaw) * cp), detmath.Sin(s.Pitch), -float64(detmath.Cos(s.Yaw) * cp)})
		t, bi, n := Ray(w.Boxes, eye, d, r.Range)
		var mb content.Box
		mt, mi, mu, mn := w.moverRay(eye, d, t, &mb)
		if mi >= 0 && (bi < 0 || mt < t) { // a mover's box, nearer than any static one
			at := Vec3{eye[0] + float64(d[0]*mt), eye[1] + float64(d[1]*mt), eye[2] + float64(d[2]*mt)}
			o := moverOrigin(&w.Movers[mi], mu, w.T)
			s.Hook = Hook{On: true, At: at, N: mn, Lo: Vec3(mb.Min), Hi: Vec3(mb.Max), Flip: true, Rope: mt,
				Mover: mi + 1, U: mu, Local: Vec3{at[0] - o[0], at[1] - o[1], at[2] - o[2]}}
			ev = HookFired
		} else if bi < 0 {
			ev = HookMissed
		} else {
			b := &w.Boxes[bi]
			s.Hook = Hook{On: true, At: Vec3{eye[0] + float64(d[0]*t), eye[1] + float64(d[1]*t), eye[2] + float64(d[2]*t)}, N: n,
				Lo: Vec3(b.Min), Hi: Vec3(b.Max), Flip: !IsWedge(b), Rope: t, Mover: 0, U: 0, Local: Vec3{}}
			ev = HookFired
		}
	}
	if !s.Hook.On {
		return ev
	}
	if !in.Hook {
		s.Hook = Hook{}
		return HookReleased
	}
	s.Energy -= float64(r.Drain * dt)
	if s.Energy <= 0 {
		s.Energy = 0
		s.Hook = Hook{}
		return HookReleased
	}
	// Pull along the rope from the body's middle.
	c := Above(s.Pos, s.Grav, s.H/2)
	dx, dy, dz := s.Hook.At[0]-c[0], s.Hook.At[1]-c[1], s.Hook.At[2]-c[2]
	dist := detmath.Hypot(detmath.Hypot(dx, dy), dz)
	if dist < 1e-6 {
		return ev
	}
	u := Vec3{dx / dist, dy / dist, dz / dist}
	vr := float64(s.Vel[0]*u[0]) + float64(s.Vel[1]*u[1]) + float64(s.Vel[2]*u[2])
	if dist >= s.Hook.Rope && vr < 0 { // taut: no moving away (swing)
		s.Vel = Vec3{s.Vel[0] - float64(u[0]*vr), s.Vel[1] - float64(u[1]*vr), s.Vel[2] - float64(u[2]*vr)}
		vr = 0
	}
	if vr < r.Pull {
		k := math.Min(float64(hookAccel*dt), r.Pull-vr)
		s.Vel = Vec3{s.Vel[0] + float64(u[0]*k), s.Vel[1] + float64(u[1]*k), s.Vel[2] + float64(u[2]*k)}
	}
	s.Hook.Rope = math.Min(s.Hook.Rope, dist)
	up := Up(s.Grav)
	if s.OnGround && float64(u[0]*up[0])+float64(u[1]*up[1])+float64(u[2]*up[2]) > 0.1 {
		s.OnGround = false // lifted off
	}
	return ev
}

// hookLand: after the step (t: its end, the movers' clock), a hooked body
// touching the hooked box face flips its gravity into it (wearing the boots),
// standing on it where it touched, if there is room; the hook lets go.
// Returns HookFlipped or 0.
func (w *World) hookLand(s *Body, t float64) int {
	hk := &s.Hook
	if !hk.Flip {
		return 0
	}
	a := 0
	for i := range 3 {
		if hk.N[i] != 0 {
			a = i
		}
	}
	var hull content.Box
	HullBox(s.Pos, s.H, s.Grav, &hull)
	plane, gap := hk.Lo[a], hull.Max[a]-hk.Lo[a]
	if hk.N[a] > 0 {
		plane, gap = hk.Hi[a], hk.Hi[a]-hull.Min[a]
	}
	if math.Abs(gap) > hookTouch {
		return 0
	}
	for i := range 3 {
		if i != a && (hull.Max[i] <= hk.Lo[i] || hull.Min[i] >= hk.Hi[i]) {
			return 0 // beside the face, not on it
		}
	}
	g := GravOf(Vec3{-hk.N[0], -hk.N[1], -hk.N[2]})
	if g == s.Grav {
		return 0 // already its floor
	}
	// The new feet: the hull's middle, on the face, moved in so the whole
	// footprint is on it (a face narrower than the footprint: its middle).
	// Pulled in low or to one side, the body stands on the face, not on a
	// sliver of its edge.
	p := Vec3{(hull.Min[0] + hull.Max[0]) / 2, (hull.Min[1] + hull.Max[1]) / 2, (hull.Min[2] + hull.Max[2]) / 2}
	for i := range 3 {
		if i != a {
			lo, hi := hk.Lo[i]+R, hk.Hi[i]-R
			if lo > hi {
				lo = (hk.Lo[i] + hk.Hi[i]) / 2
				hi = lo
			}
			p[i] = math.Max(lo, math.Min(hi, p[i]))
		}
	}
	p[a] = plane
	var nb content.Box
	HullBox(p, s.H, g, &nb)
	for i := range w.Boxes {
		b := &w.Boxes[i]
		if nb.Min[0]+eps < b.Max[0] && nb.Max[0]-eps > b.Min[0] && nb.Min[1]+eps < b.Max[1] && nb.Max[1]-eps > b.Min[1] &&
			nb.Min[2]+eps < b.Max[2] && nb.Max[2]-eps > b.Min[2] {
			return 0 // no room to stand there
		}
	}
	if len(w.Movers) > 0 {
		mbs := MoverBoxes(w.Movers, t)
		for i := range mbs {
			b := &mbs[i]
			if nb.Min[0]+eps < b.Max[0] && nb.Max[0]-eps > b.Min[0] && nb.Min[1]+eps < b.Max[1] && nb.Max[1]-eps > b.Min[1] &&
				nb.Min[2]+eps < b.Max[2] && nb.Max[2]-eps > b.Min[2] {
				return 0 // a mover in the way
			}
		}
	}
	s.Vel[a] = 0 // into the face: stopped; along it: kept
	s.Pos, s.Grav, s.OnGround, s.Hook = p, g, true, Hook{}
	return HookFlipped
}

// moverRay: the nearest mover box (at w.T) the ray o + d·t hits within maxT:
// its t, the mover's index (-1: none), the tread (Hook.U), the face's normal,
// and the box (out).
func (w *World) moverRay(o, d Vec3, maxT float64, out *content.Box) (float64, int, float64, Vec3) {
	best, bi, bu, bf := maxT, -1, 0.0, -1
	for i := range w.Movers {
		m := &w.Movers[i]
		if m.Kind == content.MoverEscalator {
			k := w.T * m.Speed / m.Run
			fl := math.Floor(k)
			p := k - fl
			for j := -1; j < treads(m); j++ {
				var b content.Box
				if !treadBox(m, float64(j)+p, &b) {
					continue
				}
				t0, _, f := RaySolid(o, d, &b)
				if f >= 0 && t0 <= best && (bi < 0 || t0 < best) {
					best, bi, bu, bf, *out = t0, i, float64(j)-fl, f, b
				}
			}
		} else if m.Kind == content.MoverSlide {
			var b content.Box
			slideBox(m, slideAt(m, w.T), &b)
			t0, _, f := RaySolid(o, d, &b)
			if f >= 0 && t0 <= best && (bi < 0 || t0 < best) {
				best, bi, bu, bf, *out = t0, i, 0, f, b
			}
		}
	}
	if bi < 0 {
		return maxT, -1, 0, Vec3{}
	}
	return best, bi, bu, FaceNormal(out, bf, d)
}

// treadU is tread U's u at time t (treadBox).
func treadU(m *content.Mover, u, t float64) float64 { return u + t*m.Speed/m.Run }

// moverOrigin is the point of mover m's box (tread u for an escalator) that
// moves with it, at time t: the anchor is Hook.Local from it.
func moverOrigin(m *content.Mover, u, t float64) Vec3 {
	if m.Kind == content.MoverEscalator {
		v := treadU(m, u, t)
		low := m.Min[m.Axis]
		if m.Up < 0 {
			low = m.Max[m.Axis]
		}
		var o Vec3
		o[m.Axis] = low + float64(float64(m.Up)*float64(v*m.Run))
		o[1] = treadTop(m, v)
		return o
	}
	f := slideAt(m, t)
	return Vec3{m.Min[0] + float64(m.Move[0]*f), m.Min[1] + float64(m.Move[1]*f), m.Min[2] + float64(m.Move[2]*f)}
}

// hookFollow moves a hook latched to a mover to where the mover is at time
// t: the anchor and the hooked box. False: it is gone (a tread under its
// comb plate, or a map edit took the mover away).
func (w *World) hookFollow(s *Body, t float64) bool {
	hk := &s.Hook
	if hk.Mover > len(w.Movers) {
		return false
	}
	m := &w.Movers[hk.Mover-1]
	var b content.Box
	if m.Kind == content.MoverEscalator {
		if !treadBox(m, treadU(m, hk.U, t), &b) {
			return false
		}
	} else if m.Kind == content.MoverSlide {
		slideBox(m, slideAt(m, t), &b)
	} else {
		return false
	}
	o := moverOrigin(m, hk.U, t)
	hk.At = Vec3{o[0] + hk.Local[0], o[1] + hk.Local[1], o[2] + hk.Local[2]}
	hk.Lo, hk.Hi = Vec3(b.Min), Vec3(b.Max)
	return true
}
