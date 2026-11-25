package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/detmath"
)

// Water (content.Map.Water: box volumes whose top is the surface). A body's
// water level (WaterLevel) is how deep it is in:
//
//	0 dry; 1 wading (the feet under the surface: a little slower);
//	2 swimming (the waist under, the eyes out: at the surface);
//	3 under (the eyes under too).
//
// Swimming (2 and 3) has no gravity and no ground: the body moves along the
// view (pitch counts), jump held swims up and crouch dives, and with nothing
// held it sinks slowly. Velocity approaches the wanted one at swimAccel plus
// a drag that grows with the difference (a plunge is stopped within a metre
// or two). At the surface you can't swim higher than it, but jump held while
// swimming forward into a ledge is a water jump: up and out onto it (rising
// faster than swimming goes, the body has gravity, not the water's drag). The same
// in both movement models. In a rotated gravity frame (gravity.go) there is
// no water in v1.
const (
	SwimSpeed  = 3.6  // m/s, the wanted speed swimming (walk: half)
	swimAccel  = 10.0 // m/s² towards the wanted velocity, per axis
	swimDrag   = 12.0 // and this times the difference, per second
	sinkSpeed  = 0.6  // m/s down with nothing held
	wadeScale  = 0.8  // max speed wading
	waterJump  = 10.5 // m/s up out of the water at a ledge: feet from 2.2 m under its top to over it
	ledgeClimb = 2.6  // a ledge is no higher than this over the feet (a wall is)
	ledgeReach = 0.3  // a ledge this far in front of the hull counts
)

// waterTop is the surface over (x, y, z): the top of a water volume holding
// that point (math.Inf(-1): none).
func (w *World) waterTop(x, y, z float64) float64 {
	top := math.Inf(-1)
	for i := range w.Water {
		b := &w.Water[i]
		if x >= b.Min[0] && x <= b.Max[0] && z >= b.Min[2] && z <= b.Max[2] && y >= b.Min[1] && y < b.Max[1] {
			top = math.Max(top, b.Max[1])
		}
	}
	return top
}

// WaterLevel is how deep body s is in the water (0..3, above).
func (w *World) WaterLevel(s *Body) int {
	if len(w.Water) == 0 {
		return 0
	}
	top := w.waterTop(s.Pos[0], s.Pos[1], s.Pos[2])
	if s.Pos[1] >= top {
		return 0
	}
	eye := Eye
	if s.Crouched {
		eye = EyeCrouch
	}
	switch {
	case s.Pos[1]+eye < top:
		return 3
	case s.Pos[1]+float64(s.H*0.5) < top:
		return 2
	}
	return 1
}

// swim is one sub-step of swimming (level 2 or 3).
func (w *World) swim(s *Body, in Input, lvl int, dt float64, ev *Events) {
	sy, cy := detmath.Sin(s.Yaw), detmath.Cos(s.Yaw)
	sp, cp := detmath.Sin(in.Pitch), detmath.Cos(in.Pitch)
	fx, fy, fz := -float64(sy*cp), sp, -float64(cy*cp)
	wx := float64(fx*in.F) + float64(cy*in.R)
	wy := float64(fy * in.F)
	wz := float64(fz*in.F) - float64(sy*in.R)
	if in.JumpHeld {
		wy++
	}
	if in.Crouch {
		wy--
	}
	speed := SwimSpeed
	if in.Walk {
		speed *= 0.5
	}
	if in.Speed > 0 {
		speed *= in.Speed
	}
	var want Vec3
	if l := math.Sqrt(float64(wx*wx) + float64(wy*wy) + float64(wz*wz)); l > 1e-9 {
		want = Vec3{float64(wx/l) * speed, float64(wy/l) * speed, float64(wz/l) * speed}
	} else {
		want = Vec3{0, -sinkSpeed, 0}
	}
	if lvl == 2 {
		want[1] = math.Min(want[1], 0) // not out over the surface by swimming
	}
	for a := range 3 {
		if a == 1 && s.Vel[1] > SwimSpeed { // a water jump under way: it rises against gravity, not the water
			s.Vel[1] -= float64(Grav * dt)
			continue
		}
		d := want[a] - s.Vel[a]
		s.Vel[a] += clampAbs(d, float64((swimAccel+float64(swimDrag*math.Abs(d)))*dt))
	}
	// The water jump: at the surface, swimming forward into a ledge.
	if lvl == 2 && in.JumpHeld && in.F > 0 {
		hx, hz := -sy, -cy
		px, pz := s.Pos[0]+float64(hx*ledgeReach), s.Pos[2]+float64(hz*ledgeReach)
		if !w.free(px, s.Pos[1], pz, s.H) && w.free(px, s.Pos[1]+ledgeClimb, pz, s.H) {
			s.Vel[1] = waterJump
			ev.Jumped = true
		}
	}
	s.OnGround = false
	w.moveH(s, 0, s.Vel[0]*dt)
	w.moveH(s, 2, s.Vel[2]*dt)
	w.moveV(s, s.Vel[1]*dt)
	if s.Vel[1] <= 0 {
		s.OnGround = false // the bottom of the pool is not ground while swimming
	}
}
