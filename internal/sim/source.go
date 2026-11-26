package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/detmath"
)

// Movement models (MoveRules.Model): the server cvar sv_movement, which a
// map's Rules may pin (content.Map.Rules; docs/physics.md).
const (
	// MoveClassic is counterslop's own: the velocity eases toward the wish
	// velocity (groundAcc, airAcc), no friction, no air strafing.
	MoveClassic = 0
	// MoveSource is Quake/Source: friction on the ground, acceleration along
	// the wish direction only up to the wish speed, and in the air a wish
	// speed capped at airCap, so turning while strafing gains speed (air
	// strafe, bunny hop) and a wide air turn keeps it (surf).
	MoveSource = 1
)

// Source movement in metres: Run (7 m/s) is CS's 250 u/s, so 1 u = 0.028 m.
const (
	SourceUnit       = 7.0 / 250 // metres per Source unit
	sourceAccel      = 5.5       // sv_accelerate (CS2)
	sourceFriction   = 5.2       // sv_friction (CS2)
	sourceStop       = 80 * SourceUnit
	airCap           = 30 * SourceUnit // the air wish speed cap (30 u/s)
	maxVelocity      = 3500 * SourceUnit
	DefaultAirAccel  = 12.0 // sv_airaccelerate (CS2); surf servers run 100-150
	frictionMinSpeed = 0.1 * SourceUnit
	MaxAirAccelerate = 1000.0
)

// MoveRules are the movement rules in force: the server's cvars, less what
// the map pins (internal/game moveRules). Both the server and the client's
// prediction set the same on their World.
type MoveRules struct {
	Model    int       // MoveClassic or MoveSource
	AirAccel float64   // source: sv_airaccelerate (0 = DefaultAirAccel)
	AutoBhop bool      // sv_autobunnyhopping: holding jump jumps again on landing
	Hook     HookRules // the Gravity Override's hook (hook.go; zero: no hook)
}

// srcAccelerate adds speed along (wx, wz) up to wishspeed: Source's
// Accelerate/AirAccelerate. capped is the air rule: the speed along the wish
// direction may only reach min(wishspeed, airCap), while the acceleration
// still scales with the whole wishspeed.
func srcAccelerate(s *Body, wx, wz, wishspeed, accel, dt float64, capped bool) {
	ws := wishspeed
	if capped {
		ws = math.Min(ws, airCap)
	}
	cur := float64(s.Vel[0]*wx) + float64(s.Vel[2]*wz)
	add := ws - cur
	if add <= 0 {
		return
	}
	as := math.Min(float64(float64(accel*wishspeed)*dt), add)
	s.Vel[0] += float64(as * wx)
	s.Vel[2] += float64(as * wz)
}

// srcFriction is Source's ground friction on the horizontal velocity.
func srcFriction(s *Body, dt float64) {
	sp := detmath.Hypot(s.Vel[0], s.Vel[2])
	if sp < frictionMinSpeed {
		s.Vel[0], s.Vel[2] = 0, 0
		return
	}
	drop := float64(float64(math.Max(sp, sourceStop)*sourceFriction) * dt)
	k := math.Max(0, sp-drop) / sp
	s.Vel[0] *= k
	s.Vel[2] *= k
}

// sourceMove is one sub-step's jump, friction and acceleration under
// MoveSource. (wx, wz) is the unit wish direction (0, 0 for none), mx the max
// speed (walk, crouch and the held weapon included).
func (w *World) sourceMove(s *Body, jump bool, wx, wz, wl, mx, dt float64, ev *Events) {
	if jump && s.OnGround { // before friction: a jump on the landing tick keeps all the speed (bhop)
		s.Vel[1] = float64(JumpSpeed * w.jumpK)
		s.OnGround = false
		ev.Jumped = true
	}
	if s.OnGround {
		srcFriction(s, dt)
	}
	if wl > 0 {
		if s.OnGround {
			srcAccelerate(s, wx, wz, mx, sourceAccel, dt, false)
		} else {
			aa := w.Move.AirAccel
			if aa <= 0 {
				aa = DefaultAirAccel
			}
			srcAccelerate(s, wx, wz, mx, aa, dt, true)
		}
	}
	for a := range 3 {
		s.Vel[a] = clampAbs(s.Vel[a], maxVelocity)
	}
}
