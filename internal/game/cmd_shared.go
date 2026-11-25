//gojs:shared

package game

// The command protocol rules shared with the client: cmd/gojs translates the
// //gojs:shared files of this package into web/game/shared.gen.js.

import "github.com/gigurra/counterslop/internal/sim"

// Command buttons (Cmd.B bits, the held state; edges come from comparing
// with the previous command's B, on the client and on the server alike).
const (
	BtnJump = 1 << iota
	BtnCrouch
	BtnWalk
	BtnFire
	BtnScope
	BtnReload // down edge starts a reload
	BtnUse    // plant / defuse The Big Diff (held)
	BtnAlt    // attack2 (+zoom) held: with a grenade in hand, the underhand lob (Registry.throwTick)
	BtnHook   // +hook held: the Gravity Override's hook (internal/sim hook.go); fires on its down edge
)

// MoveInput is the physics input for one command. A jump fires only on the
// jump button's down edge (holding it never re-jumps on landing) and never
// while crouching on the ground; crouching in the air (the crouch-jump) is
// untouched. Speed (the held weapon's) is the caller's.
func MoveInput(c Cmd, prevB int, body *sim.Body) sim.Input {
	crouch := c.B&BtnCrouch != 0
	held := c.B&BtnJump != 0 && !(body.OnGround && (crouch || body.Crouched))
	jump := held && prevB&BtnJump == 0
	hook := c.B&BtnHook != 0
	return sim.Input{F: c.F, R: c.R, Walk: c.B&BtnWalk != 0, Jump: jump, Crouch: crouch, JumpHeld: held, Pitch: c.Pitch, Hook: hook, HookPress: hook && prevB&BtnHook == 0}
}

// ShotView is the view a shot of command c goes along: where it was at the
// click when the command carries it (CS2 subtick: a flick keeps moving after
// the click, within the tick), else the command's view.
func ShotView(c Cmd) (float64, float64) {
	if c.ShotView {
		return c.ShotYaw, c.ShotPitch
	}
	return c.Yaw, c.Pitch
}

// CmdTime is when command seq starts on its player's command clock (tickBase:
// me.tb in the self-snapshot), in seconds: the time the movers are at for it
// (sim.World.T), on the server and in the client's prediction and replays.
func CmdTime(tickBase int64, seq int) float64 { return float64(tickBase+int64(seq)) / cmdHz }

const cmdHz = 100 // = SimHz (simtick.go is not shared)
