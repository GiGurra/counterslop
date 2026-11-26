package sim

import (
	"github.com/gigurra/counterslop/internal/detmath"
)

// Noclip is the map editor's flight (internal/game mapedit.go: an admin
// editing a map, not playtesting): along the view (pitch included), jump up,
// crouch down, through every solid, no gravity. Walk flies slower.
const (
	NoclipSpeed = 2.5 * Run
	noclipWalk  = 0.4
)

// noclip moves the body one step of dt.
func (w *World) noclip(s *Body, in Input, dt float64) {
	sy, cy := detmath.Sin(s.Yaw), detmath.Cos(s.Yaw)
	sp, cp := detmath.Sin(in.Pitch), detmath.Cos(in.Pitch)
	fx, fy, fz := -float64(sy*cp), sp, -float64(cy*cp)
	wx := float64(fx*in.F) + float64(cy*in.R)
	wy := float64(fy * in.F)
	wz := float64(fz*in.F) - float64(sy*in.R)
	if in.Jump || in.JumpHeld {
		wy++
	}
	if in.Crouch {
		wy--
	}
	speed := NoclipSpeed
	if in.Walk {
		speed *= noclipWalk
	}
	l := detmath.Hypot(detmath.Hypot(wx, wy), wz)
	if l > 1 {
		wx, wy, wz = wx/l, wy/l, wz/l
	}
	s.Vel = Vec3{float64(wx * speed), float64(wy * speed), float64(wz * speed)}
	s.Pos = Vec3{s.Pos[0] + float64(s.Vel[0]*dt), s.Pos[1] + float64(s.Vel[1]*dt), s.Pos[2] + float64(s.Vel[2]*dt)}
	s.OnGround, s.Crouched, s.H, s.Grav = false, false, H, GravDown
	s.Hook.On = false
	s.EyeH, s.EyeOff = Eye, 0
}
