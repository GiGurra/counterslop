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
	if !s.Hook.On {
		s.Energy = math.Min(r.Energy, s.Energy+float64(r.Recharge*dt))
	}
	if in.HookPress && !s.Hook.On && r.Range > 0 && s.Energy >= r.Cost {
		s.Energy -= r.Cost
		eye := EyePos(s)
		cp := detmath.Cos(s.Pitch)
		d := FromFrame(s.Grav, Vec3{-float64(detmath.Sin(s.Yaw) * cp), detmath.Sin(s.Pitch), -float64(detmath.Cos(s.Yaw) * cp)})
		t, bi, n := Ray(w.Boxes, eye, d, r.Range)
		if bi < 0 {
			ev = HookMissed
		} else {
			b := &w.Boxes[bi]
			s.Hook = Hook{On: true, At: Vec3{eye[0] + float64(d[0]*t), eye[1] + float64(d[1]*t), eye[2] + float64(d[2]*t)}, N: n,
				Lo: Vec3(b.Min), Hi: Vec3(b.Max), Flip: !IsWedge(b), Rope: t}
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

// hookLand: after the step, a hooked body touching the hooked box face flips
// its gravity into it (wearing the boots), standing on it where it touched,
// if there is room; the hook lets go. Returns HookFlipped, or 0.
func (w *World) hookLand(s *Body) int {
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
	// The new feet: the hull's middle, on the face.
	p := Vec3{(hull.Min[0] + hull.Max[0]) / 2, (hull.Min[1] + hull.Max[1]) / 2, (hull.Min[2] + hull.Max[2]) / 2}
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
	s.Vel[a] = 0 // into the face: stopped; along it: kept
	s.Pos, s.Grav, s.OnGround, s.Hook = p, g, true, Hook{}
	return HookFlipped
}
