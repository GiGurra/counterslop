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

// zoneOf is body s's gravity zone: its index in w.Zones, -1 for none. Zones
// carried by a mover (a ship's cabin) win over fixed ones; within each kind
// the last listed wins.
func (w *World) zoneOf(s *Body) int {
	cy := s.Pos[1] + float64(s.H*0.5)
	for pass := range 2 {
		for i := len(w.Zones) - 1; i >= 0; i-- {
			z := &w.Zones[i]
			if (z.Mover > 0) != (pass == 0) {
				continue
			}
			o := w.carried(z.Mover)
			if s.Pos[0] >= z.Min[0]+o[0] && s.Pos[0] <= z.Max[0]+o[0] && cy >= z.Min[1]+o[1] && cy < z.Max[1]+o[1] && s.Pos[2] >= z.Min[2]+o[2] && s.Pos[2] <= z.Max[2]+o[2] {
				return i
			}
		}
	}
	return -1
}

// carried is how far mover mv (1-based, 0: none) has moved its zones and
// launchers at the current sub-step's end (w.st: where the movers are).
func (w *World) carried(mv int) Vec3 {
	if mv <= 0 || mv > len(w.Movers) {
		return Vec3{}
	}
	return MoverOffset(&w.Movers[mv-1], w.st)
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

// GravityAt is the gravity (m/s², down) a point is under: the fixed zone
// holding it, else the world's 22 scaled by g (the server's grenades use it,
// with their own g; zones carried by a mover don't count for them).
func GravityAt(zones []content.GravZone, p Vec3, g float64) float64 {
	for i := len(zones) - 1; i >= 0; i-- {
		z := &zones[i]
		if z.Mover == 0 && p[0] >= z.Min[0] && p[0] <= z.Max[0] && p[1] >= z.Min[1] && p[1] < z.Max[1] && p[2] >= z.Min[2] && p[2] <= z.Max[2] {
			return float64(g * z.G)
		}
	}
	return g
}

// launch throws body s if it stands on a launch pad (its feet in the box):
// a pad on a mover moves with it and throws only in its window of the
// mover's cycle; an Air one throws a body off the ground too, unless it is
// already going at 90% of the pad's speed its way (no throw per sub-step).
func (w *World) launch(s *Body, ev *Events) {
	for i := range w.Launch {
		l := &w.Launch[i]
		if !s.OnGround && !l.Air {
			continue
		}
		if l.Mover > 0 && l.Mover <= len(w.Movers) && (l.From != 0 || l.To != 0) {
			c := CycleT(&w.Movers[l.Mover-1], w.st)
			if c < l.From || c >= l.To {
				continue
			}
		}
		o := w.carried(l.Mover)
		p := s.Pos
		if p[0] >= l.Min[0]+o[0] && p[0] <= l.Max[0]+o[0] && p[1] >= l.Min[1]+o[1] && p[1] <= l.Max[1]+o[1] && p[2] >= l.Min[2]+o[2] && p[2] <= l.Max[2]+o[2] {
			v := l.Vel
			if l.Apex > 0 {
				g, _ := w.zoneRules(s)
				if g <= 0 {
					continue // no gravity: no arc
				}
				v = PadVel(p, l.Target, l.Apex, float64(Grav*g))
			}
			along := float64(s.Vel[0]*v[0]) + float64(s.Vel[1]*v[1]) + float64(s.Vel[2]*v[2])
			if !s.OnGround && along >= float64(0.9*(float64(v[0]*v[0])+float64(v[1]*v[1])+float64(v[2]*v[2]))) {
				continue // already going (at least about) that fast that way: thrown once
			}
			s.Vel = v
			s.OnGround = false
			ev.Launched = true
			return
		}
	}
}

// PadVel is a jump pad's throw (content.Launcher with Apex): the velocity
// that takes a body from its feet at from to land on to, the top of its arc
// apex m over the higher of the two, under gravity g (m/s², down). The
// client draws the same arc (its preview) from it.
func PadVel(from, to Vec3, apex, g float64) Vec3 {
	top := math.Max(from[1], to[1]) + apex
	up := math.Sqrt(float64(2 * g * (top - from[1])))
	t := float64(up/g) + math.Sqrt(float64(2*(top-to[1]))/g)
	return Vec3{(to[0] - from[0]) / t, up, (to[2] - from[2]) / t}
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
