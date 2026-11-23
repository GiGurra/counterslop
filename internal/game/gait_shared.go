//gojs:shared

package game

// The walk cycle and idle sway (a cartoon gait), shared with the client: the
// server steps it per command from the body's movement, replicates it in
// every snapshot, and ray-traces shots against the pose it gives; the client
// draws the same pose (web/gen/gait.js, web/assets/models/player.js). So a
// swinging foot or a leaning head is hit where it is drawn.
//
// The state (Gait) is a function of the command stream only: the distance the
// body moved, whether it is on the ground, crouching. The pose adds the time
// (the server tick being drawn or traced) for the idle sway and the airborne
// flail, and per-player factors seeded by the player id (GaitKOf).

import (
	"math"

	"github.com/gigurra/counterslop/internal/content/rng"
	"github.com/gigurra/counterslop/internal/detmath"
)

// Tunables (metres / radians at full amp). Per-player factors multiply some.
const (
	gaitTeleport   = 150.0 // m/s: faster than this between two commands is a teleport (resets the cycle)
	gaitRun        = 7.0   // ground speed for amp 1
	gaitCrouchRun  = 2.4   // ...when crouched (a full-size shuffle at crouch speed)
	gaitSwing      = 0.28  // foot fore/aft reach
	gaitSwingCr    = 0.12
	gaitLift       = 0.18 // foot lift on the swing
	gaitLiftCr     = 0.07
	gaitWaddle     = 0.11 // side-to-side roll per step
	gaitBob        = 0.07 // body bounce
	gaitLean       = 0.16 // forward lean at full tilt
	gaitWobble     = 0.22 // head wobble
	gaitFlail      = 0.2  // airborne foot flail reach
	gaitSway       = 0.035
	gaitTau        = 2 * math.Pi
	gaitPeriod     = gaitTau // the phase wraps here (it only runs forward: interpolation knows the way; < 1.3 rad a tick)
	gaitMinStride  = 0.35
	gaitHipStand   = 0.45 // joint heights: top of the legs block, top of the body
	gaitHipCrouch  = 0.3
	gaitNeckStand  = 1.35
	gaitNeckCrouch = 0.75
)

// Gait is the replicated walk-cycle state (snapshot field "g").
type Gait struct {
	Phase  float64 // radians, advances with distance walked, wraps at gaitPeriod
	Amp    float64 // 0 at rest .. 1 at full speed (eased)
	Air    float64 // 0 grounded .. 1 airborne (eased)
	Crouch float64 // 0 standing .. 1 crouched (eased)
}

// GaitK is one player's silliness: how wide they swing, waddle, bob, wobble, lean.
type GaitK struct {
	Swing, Waddle, Bob, Wobble, Lean, Offset float64
}

// GaitKOf draws a player's factors from rng(7000 + id).
func GaitKOf(id int) GaitK {
	r := rng.New(uint32(7000 + id))
	var k GaitK
	k.Swing = r.Range(0.85, 1.2)
	k.Waddle = r.Range(0.6, 1.5)
	k.Bob = r.Range(0.75, 1.3)
	k.Wobble = r.Range(0.5, 1.6)
	k.Lean = r.Range(0.7, 1.3)
	k.Offset = r.Range(0, gaitTau)
	return k
}

// ease moves cur toward target at rate (1/s) over one command tick.
func ease(cur, target, rate float64) float64 {
	return cur + float64((target-cur)*(1-detmath.Exp(-rate/SimHz)))
}

// mix is a + (b-a)·k.
func mix(a, b, k float64) float64 { return a + float64((b-a)*k) }

// gaitReach is the foot's fore/aft reach at this amp and crouch.
func gaitReach(swing, amp, crouch float64) float64 {
	return float64(swing*amp) * mix(gaitSwing, gaitSwingCr, crouch)
}

// Step advances the gait by one command tick in which the body moved
// (dx, dy, dz). swing is the player's GaitK.Swing.
func (g *Gait) Step(dx, dy, dz float64, onGround, crouched bool, swing float64) {
	const dt = 1.0 / SimHz
	d := detmath.Hypot(dx, dz)
	if !(d <= gaitTeleport*dt) || !(math.Abs(dy) <= gaitTeleport*dt) { // a teleport, a respawn (or NaN)
		g.Phase, g.Amp, g.Air, g.Crouch = 0, 0, 0, 0
		return
	}
	if onGround {
		g.Air = ease(g.Air, 0, 9)
	} else {
		g.Air = ease(g.Air, 1, 14)
	}
	cr := 0.0
	if crouched {
		cr = 1
	}
	g.Crouch = ease(g.Crouch, cr, 18)
	target := math.Min(1, (d/dt)/mix(gaitRun, gaitCrouchRun, g.Crouch))
	g.Amp = ease(g.Amp, target, 10)
	// A foot sweeps 2·reach per half cycle while the body moves half a stride:
	// stride = 2π·reach keeps the planted foot still (no skating). The floor
	// keeps a tiny amp from spinning the phase; the stride follows the speed
	// (not the eased amp) so a standing start doesn't whirl the feet.
	stride := math.Max(gaitMinStride, float64(gaitTau*gaitReach(swing, math.Max(target, 0.25), g.Crouch)))
	g.Phase += float64(float64(float64(d/stride)*gaitTau) * (1 - g.Air))
	if g.Phase >= gaitPeriod {
		g.Phase -= gaitPeriod
	}
}

// LerpGait interpolates between two replicated states (k in 0..1), the phase
// forward only (it never runs backwards; a smaller b wrapped).
func LerpGait(a, b *Gait, k float64) *Gait {
	if a == nil || b == nil {
		return b
	}
	dp := b.Phase - a.Phase
	if dp < 0 {
		dp += gaitPeriod
	}
	ph := a.Phase + float64(dp*k)
	if ph >= gaitPeriod {
		ph -= gaitPeriod
	}
	return &Gait{Phase: ph, Amp: mix(a.Amp, b.Amp, k), Air: mix(a.Air, b.Air, k), Crouch: mix(a.Crouch, b.Crouch, k)}
}

// footPose is one foot's offset (player frame, forward is -Z) and toe pitch.
type footPose struct {
	X, Y, Z, RX float64
}

// GaitPose is the animation at one instant: feet, the legs block's roll, the
// upper body's bounce/roll/lean, the head's roll/pitch.
type GaitPose struct {
	FootL, FootR               footPose
	LegsRoll                   float64
	BodyY, BodyRoll, BodyPitch float64
	HeadRoll, HeadPitch        float64
}

// PoseOf is the gait's pose at time t (seconds) for factors k.
func PoseOf(g *Gait, t float64, k GaitK) GaitPose {
	ph := g.Phase + k.Offset
	ground, rest := float64(g.Amp*(1-g.Air)), 1-g.Amp
	r := gaitReach(k.Swing, ground, g.Crouch)
	lift := float64(ground*k.Swing) * mix(gaitLift, gaitLiftCr, g.Crouch)
	flail := g.Air * gaitFlail
	var p GaitPose
	p.FootL = footAt(ph, r, lift, flail, t, 1)
	p.FootR = footAt(ph+math.Pi, r, lift, flail, t+0.37, -1)
	s := detmath.Sin(ph)
	sway := float64(float64(float64(gaitSway*rest)*(1-g.Air)) * detmath.Sin(float64(t*1.7)+k.Offset)) // float64(): Go may fuse across statements
	// The legs block rocks onto the planted foot; the body over-rolls on top of it.
	p.LegsRoll = float64(float64(float64(float64(gaitWaddle*k.Waddle)*ground)*s)*float64(1-float64(0.5*g.Crouch))) + float64(sway*0.5)
	p.BodyRoll = float64(p.LegsRoll*1.4) + sway
	p.BodyY = float64(float64(float64(gaitBob*k.Bob)*ground)*math.Abs(s)) + float64(0.06*g.Air)
	p.BodyPitch = float64(float64(-gaitLean*k.Lean)*ground) + float64(float64(0.18*g.Air)*detmath.Sin(t*9))
	walk := float64(float64(ground*detmath.Sin(float64(ph*2)+0.6))*0.5) + float64(g.Air*detmath.Sin(t*13))
	p.HeadRoll = float64(float64(gaitWobble*k.Wobble)*walk) - sway
	p.HeadPitch = float64(float64(0.08*ground)*detmath.Cos(ph*2)) + float64(float64(0.03*rest)*detmath.Sin(t*1.1))
	return p
}

// footAt: side +1 left, -1 right.
func footAt(ph, reach, lift, flail, t, side float64) footPose {
	s, c := detmath.Sin(ph), detmath.Cos(ph)
	up := math.Max(c, 0)
	var f footPose
	f.Z = float64(-reach*s) - float64(flail*detmath.Sin(t*19))
	f.Y = float64(lift*up) + float64(float64(flail*0.4)*float64(0.5+float64(0.5*detmath.Sin(t*23))))
	f.X = float64(float64(side*flail)*0.35) * float64(0.5+float64(0.5*detmath.Cos(t*17)))
	// Heel up as it leaves the ground (behind), toe up as it reaches (in front).
	f.RX = float64(float64(float64(0.5*(lift/gaitLift))*s)*up) + float64(float64(flail*1.5)*detmath.Sin(t*19))
	return f
}

// Xform is a rigid transform v' = R·v + T (R row-major).
type Xform struct {
	R [9]float64
	T Vec3
}

func xIdentity() Xform { return Xform{R: [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1}} }

func xTranslate(x, y, z float64) Xform {
	m := xIdentity()
	m.T = Vec3{x, y, z}
	return m
}

// xRotX: y' = c·y - s·z, z' = s·y + c·z (engine/math.js rotX).
func xRotX(a float64) Xform {
	c, s := detmath.Cos(a), detmath.Sin(a)
	return Xform{R: [9]float64{1, 0, 0, 0, c, -s, 0, s, c}}
}

// xRotZ: x' = c·x - s·y, y' = s·x + c·y (engine/math.js rotZ).
func xRotZ(a float64) Xform {
	c, s := detmath.Cos(a), detmath.Sin(a)
	return Xform{R: [9]float64{c, -s, 0, s, c, 0, 0, 0, 1}}
}

// xMul is a·b (b applied first).
func xMul(a, b Xform) Xform {
	var m Xform
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			m.R[3*i+j] = float64(a.R[3*i]*b.R[j]) + float64(a.R[3*i+1]*b.R[3+j]) + float64(a.R[3*i+2]*b.R[6+j])
		}
		m.T[i] = float64(a.R[3*i]*b.T[0]) + float64(a.R[3*i+1]*b.T[1]) + float64(a.R[3*i+2]*b.T[2]) + a.T[i]
	}
	return m
}

// xInvPoint maps a point back through m: Rᵀ·(v - T).
func xInvPoint(m *Xform, v Vec3) Vec3 {
	return xInvDir(m, Vec3{v[0] - m.T[0], v[1] - m.T[1], v[2] - m.T[2]})
}

// xInvDir maps a direction back through m: Rᵀ·v.
func xInvDir(m *Xform, v Vec3) Vec3 {
	var o Vec3
	for j := 0; j < 3; j++ {
		o[j] = float64(m.R[j]*v[0]) + float64(m.R[3+j]*v[1]) + float64(m.R[6+j]*v[2])
	}
	return o
}

// Part groups the gait moves as one.
const (
	GroupLegs  = 0
	GroupUpper = 1
	GroupHead  = 2
	GroupFootL = 3
	GroupFootR = 4
)

// PartGroup is the group a part moves with: the feet by name, the legs block,
// head-region parts with the head, everything else with the upper body.
func PartGroup(p *ModelPart) int {
	switch {
	case p.Name == "footL":
		return GroupFootL
	case p.Name == "footR":
		return GroupFootR
	case p.Name == "legs":
		return GroupLegs
	case p.Region == "head":
		return GroupHead
	}
	return GroupUpper
}

// Frames are the five group transforms in the player's frame (feet at the
// origin, -Z forward, before its position and yaw) for st's gait at st.T; nil
// without a gait (the still pose).
func (m *Model) Frames(st PoseState) []Xform {
	if st.Gait == nil {
		return nil
	}
	p := PoseOf(st.Gait, st.T, GaitKOf(st.ID))
	hip, neck := gaitHipStand, gaitNeckStand
	if st.Crouch {
		hip, neck = gaitHipCrouch, gaitNeckCrouch
	}
	legs := xMul(xTranslate(0, p.BodyY, 0), xRotZ(p.LegsRoll))
	upper := xMul(xMul(xMul(xMul(legs, xTranslate(0, hip, 0)), xRotZ(p.BodyRoll-p.LegsRoll)), xRotX(p.BodyPitch)), xTranslate(0, -hip, 0))
	head := xMul(xMul(xMul(xMul(upper, xTranslate(0, neck, 0)), xRotZ(p.HeadRoll)), xRotX(p.HeadPitch)), xTranslate(0, -neck, 0))
	out := make([]Xform, 5)
	out[GroupLegs], out[GroupUpper], out[GroupHead] = legs, upper, head
	out[GroupFootL] = m.footFrame(p.FootL, "footL", st.Crouch)
	out[GroupFootR] = m.footFrame(p.FootR, "footR", st.Crouch)
	return out
}

// footFrame: the foot moved by f, its toe pitch about its own centre.
func (m *Model) footFrame(f footPose, name string, crouched bool) Xform {
	c := Vec3{}
	for i := range m.Parts {
		if m.Parts[i].Name == name {
			c, _, _ = m.Parts[i].box(crouched)
			break
		}
	}
	return xMul(xMul(xTranslate(f.X+c[0], f.Y+c[1], f.Z+c[2]), xRotX(f.RX)), xTranslate(-c[0], -c[1], -c[2]))
}

// PoseTime is the pose time (seconds) between two snapshots at ticks a and b,
// k of the way: what the client draws and the server rewinds a shot to.
func PoseTime(a, b, k float64) float64 { return mix(a, b, k) / SimHz }

// GaitBetween is the gait at tick t from the two full snapshots around it
// (ticks ta <= tb; the gait only travels in full snapshots): interpolated,
// clamped to the nearer one outside, either one when the other is missing.
func GaitBetween(ta float64, ga *Gait, tb float64, gb *Gait, t float64) *Gait {
	if ga == nil {
		return gb
	}
	if gb == nil || tb <= ta {
		return ga
	}
	return LerpGait(ga, gb, math.Max(0, math.Min(1, (t-ta)/(tb-ta))))
}
