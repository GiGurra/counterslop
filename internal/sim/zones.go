package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/detmath"
)

// Gravity zones and launch pads (content.Map.GravZones / Launchers,
// docs/starslop.md "Phase 2"). A body's zone is the last one holding its
// centre, looked up every sub-step: no extra state, so the server and the
// prediction agree from the position alone. Outside every zone nothing
// changes (the world's gravity and jump, bit for bit). In a rotated gravity
// frame (the boots, gravity.go) there are no zones in v1: frame worlds get
// none, as with water.
//
// Space (a zone with G 0): no gravity, no ground. The body coasts; thrust
// pushes along the view (pitch counts, jump held up, crouch down) at
// spaceThrust up to spaceSpeed, and never takes a faster body's speed down:
// it only steers it.
const (
	spaceThrust = 5.0 // m/s²
	spaceSpeed  = 6.0 // m/s: thrust's top speed
)

// zoneOf is body s's gravity zone: its index in w.Zones, -1 for none.
func (w *World) zoneOf(s *Body) int {
	cy := s.Pos[1] + float64(s.H*0.5)
	for i := len(w.Zones) - 1; i >= 0; i-- {
		z := &w.Zones[i]
		if s.Pos[0] >= z.Min[0] && s.Pos[0] <= z.Max[0] && cy >= z.Min[1] && cy < z.Max[1] && s.Pos[2] >= z.Min[2] && s.Pos[2] <= z.Max[2] {
			return i
		}
	}
	return -1
}

// zoneRules: the gravity and jump-speed scales at body s (1, 1 outside every
// zone).
func (w *World) zoneRules(s *Body) (float64, float64) {
	if len(w.Zones) == 0 {
		return 1, 1
	}
	i := w.zoneOf(s)
	if i < 0 {
		return 1, 1
	}
	j := w.Zones[i].Jump
	if j <= 0 {
		j = 1
	}
	return w.Zones[i].G, j
}

// GravityAt is the gravity (m/s², down) a point is under: the zone holding it,
// else the world's 22 scaled by g (the server's grenades use it, with their own g).
func GravityAt(zones []content.GravZone, p Vec3, g float64) float64 {
	for i := len(zones) - 1; i >= 0; i-- {
		z := &zones[i]
		if p[0] >= z.Min[0] && p[0] <= z.Max[0] && p[1] >= z.Min[1] && p[1] < z.Max[1] && p[2] >= z.Min[2] && p[2] <= z.Max[2] {
			return float64(g * z.G)
		}
	}
	return g
}

// launch throws body s if it stands on a launch pad (its feet in the box).
func (w *World) launch(s *Body, ev *Events) {
	if !s.OnGround {
		return
	}
	for i := range w.Launch {
		l := &w.Launch[i]
		p := s.Pos
		if p[0] >= l.Min[0] && p[0] <= l.Max[0] && p[1] >= l.Min[1] && p[1] <= l.Max[1] && p[2] >= l.Min[2] && p[2] <= l.Max[2] {
			s.Vel = l.Vel
			s.OnGround = false
			ev.Launched = true
			return
		}
	}
}

// space is one sub-step in a zone without gravity.
func (w *World) space(s *Body, in Input, dt float64) {
	sy, cy := detmath.Sin(s.Yaw), detmath.Cos(s.Yaw)
	sp, cp := detmath.Sin(in.Pitch), detmath.Cos(in.Pitch)
	wx := float64(float64(-sy*cp)*in.F) + float64(cy*in.R)
	wy := float64(sp * in.F)
	wz := float64(float64(-cy*cp)*in.F) - float64(sy*in.R)
	if in.JumpHeld {
		wy++
	}
	if in.Crouch {
		wy--
	}
	if l := math.Sqrt(float64(wx*wx) + float64(wy*wy) + float64(wz*wz)); l > 1e-9 {
		v0 := math.Sqrt(float64(s.Vel[0]*s.Vel[0]) + float64(s.Vel[1]*s.Vel[1]) + float64(s.Vel[2]*s.Vel[2]))
		a := float64(spaceThrust * dt)
		s.Vel[0] += float64(float64(wx/l) * a)
		s.Vel[1] += float64(float64(wy/l) * a)
		s.Vel[2] += float64(float64(wz/l) * a)
		top := math.Max(spaceSpeed, v0)
		if v := math.Sqrt(float64(s.Vel[0]*s.Vel[0]) + float64(s.Vel[1]*s.Vel[1]) + float64(s.Vel[2]*s.Vel[2])); v > top {
			k := top / v
			s.Vel[0] *= k
			s.Vel[1] *= k
			s.Vel[2] *= k
		}
	}
	s.OnGround = false
	w.moveH(s, 0, s.Vel[0]*dt)
	w.moveH(s, 2, s.Vel[2]*dt)
	w.moveV(s, s.Vel[1]*dt)
	s.OnGround = false
}
