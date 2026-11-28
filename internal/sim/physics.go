// Package sim is the movement code of both the server and the client: cmd/gojs
// translates it into web/engine/sim.gen.js (go generate ./internal/gojs), so it
// must stay in the gojs subset (internal/gojs). physics_test.go runs the
// generated JS under node and requires the same trajectories, bit for bit.
//
// Position is the feet centre. Boxes are the map's AABBs.
package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/detmath"

	"github.com/gigurra/counterslop/internal/content"
)

const (
	R          = 0.4
	H          = 1.8
	Eye        = 1.62
	HCrouch    = 1.2
	EyeCrouch  = 1.05
	crouchMax  = 7.0 / 3
	legs       = H - HCrouch
	stepUp     = 0.55
	Grav       = 22.0
	JumpSpeed  = 7.2
	Run        = 7.0
	Walk       = 3.5
	groundAcc  = 70.0
	airAcc     = 14.0
	stride     = 2.2
	eps        = 1e-4
	subStepMax = 1.0 / 120
)

type Vec3 = [3]float64

// Body is one player's movement state.
type Body struct {
	Pos      Vec3
	Vel      Vec3
	Yaw      float64
	Pitch    float64
	OnGround bool
	Crouched bool
	H        float64
	EyeH     float64 // smoothed eye height (render)
	EyeOff   float64 // step-up camera smoothing (render)
	Stride   float64
	Grav     int     // the axis gravity pulls along (gravity.go; GravDown: the world's)
	Boots    bool    // wears the Gravity Override: the hook and gravity flips (hook.go)
	Energy   float64 // the hook's energy (hook.go: HookRules.Energy full)
	Hook     Hook    // the hook's state (hook.go)
	Noclip   bool    // flies through everything (noclip.go: a map editor)
}

func NewBody(spawn Vec3) *Body { return &Body{Pos: spawn, H: H, EyeH: Eye} }

// Input is one tick's movement intent. Jump means "jump now if on the
// ground"; edge detection (one jump per press) happens in the command layer.
type Input struct {
	F, R               float64
	Walk, Jump, Crouch bool
	Speed              float64 // max-speed scale (the held weapon: CS-style lighter = faster); 0 = 1
	JumpHeld           bool    // jump is held (and allowed): MoveRules.AutoBhop jumps on it
	Pitch              float64 // the view's pitch (radians, + up): swimming goes along it (water.go)
	Hook, HookPress    bool    // the hook button held, pressed this command (hook.go)
}

// Events are the sound-worthy things that happened during a step.
type Events struct {
	Jumped   bool
	Landed   float64
	Steps    int
	Splash   float64 // fell into water at this speed (m/s; water.go)
	Launched bool    // thrown by a launch pad (zones.go)
	Hook     int     // hook.go: HookFired, HookMissed, HookReleased, HookFlipped (sounds)
	// Wrapped: the body walked over a box's edge in a gravity of its own and
	// turned onto the next face (gravity.go edgeWrap); Turn is what that did
	// to its yaw (a quarter-turn multiple): the client turns its view by it.
	Wrapped bool
	Turn    float64
}

type World struct {
	Boxes   []content.Box      // static boxes the body collides with
	Movers  []content.Mover    // moving solids (movers.go)
	T       float64            // the time the stepped command starts (the movers' clock), seconds
	Move    MoveRules          // the movement rules in force (source.go)
	Peers   []Peer             // the other solid players (peers.go), set per stepped body
	Water   []content.Box      // water volumes (water.go)
	Cars    []CarDisc          // the other cars' discs (car.go), set per stepped car
	Zones   []content.GravZone // gravity zones (zones.go)
	Launch  []content.Launcher // launch pads (zones.go)
	jumpK   float64            // the jump-speed scale of the current sub-step's zone (zones.go)
	st      float64            // the current sub-step's end on the movers' clock (zones.go: where carried zones are)
	dyn     []solid            // the movers near the body in the current sub-step
	cur     []content.Box      // their boxes at its end: collided with like Boxes
	peer    []content.Box      // the peers' hulls this body collides with in the current Step
	ghost   []content.Box      // the peers' hulls it overlaps (not solid, but no moving deeper)
	riding  bool               // the body is carried by a mover in this sub-step
	onRamp  bool               // moveV put the body on a walkable wedge (wedge.go)
	frames  []*World           // the world turned into each gravity's frame (gravity.go), built when first used
	keepCur bool               // a frame world (gravity.go frame): w.cur is its own
	fg      int                // a frame world: the gravity it is turned into (GravDown: the world itself); its movers are turned in per sub-step (movers.go)
	sup     []int              // a frame world: the static boxes the body stood on as the step began (edgeWrap)
}

// NewWorld is the movement world of a map: its boxes (less the static treads
// its escalators replace) and its movers.
func NewWorld(m *content.Map) *World {
	w := &World{Boxes: m.Boxes, Movers: m.Movers, T: 0, Move: MoveRules{Model: MoveClassic, AirAccel: 0, AutoBhop: false}, Peers: nil, Water: m.Water, Cars: nil, Zones: m.GravZones, Launch: m.Launchers, jumpK: 1, st: 0, dyn: nil, cur: nil, peer: nil, ghost: nil, riding: false, onRamp: false, frames: make([]*World, 6), keepCur: false, fg: GravDown, sup: nil} // frames: the slots shared by copies of the world (the server copies it per tick)
	if len(m.Movers) == 0 {
		return w
	}
	var boxes []content.Box
	for i := range m.Boxes {
		if !Replaced(&m.Boxes[i], m.Movers) {
			boxes = append(boxes, m.Boxes[i])
		}
	}
	w.Boxes = boxes
	return w
}

// box i of the solids collided with: the static boxes, the movers', then the
// other players' hulls.
func (w *World) solids() int { return len(w.Boxes) + len(w.cur) + len(w.peer) }

func (w *World) box(i int) *content.Box {
	if i < len(w.Boxes) {
		return &w.Boxes[i]
	}
	if i < len(w.Boxes)+len(w.cur) {
		return &w.cur[i-len(w.Boxes)]
	}
	return &w.peer[i-len(w.Boxes)-len(w.cur)]
}

func (w *World) overlaps(x, y, z float64, b *content.Box, h float64) bool {
	return x+R > b.Min[0]+eps && x-R < b.Max[0]-eps &&
		y+h > b.Min[1]+eps && y < b.Max[1]-eps &&
		z+R > b.Min[2]+eps && z-R < b.Max[2]-eps &&
		(!IsWedge(b) || float64(slopeDepth(b, x, y, z, R)*SlopeNormal(b)[1]) > eps)
}

func (w *World) free(x, y, z, h float64) bool {
	for i := range w.solids() {
		if w.overlaps(x, y, z, w.box(i), h) {
			return false
		}
	}
	return true
}

func (w *World) setCrouch(s *Body, want bool) {
	x, y, z := s.Pos[0], s.Pos[1], s.Pos[2]
	if want && !s.Crouched {
		s.Crouched, s.H = true, HCrouch
		if !s.OnGround && w.free(x, y+legs, z, HCrouch) {
			s.Pos[1] += legs
			s.EyeOff -= legs
		}
	} else if !want && s.Crouched {
		if !s.OnGround && w.free(x, y-legs, z, H) {
			s.Pos[1] -= legs
			s.EyeOff += legs
		} else if !w.free(x, y, z, H) {
			return
		}
		s.Crouched, s.H = false, H
	}
}

func (w *World) moveH(s *Body, ax int, d float64) {
	if d == 0 {
		return
	}
	old := s.Pos[ax]
	s.Pos[ax] += d
	if len(w.ghost) > 0 && w.deeper(s, ax, old) {
		s.Pos[ax] = old
		s.Vel[ax] = 0
		return
	}
	for i := range w.solids() {
		b := w.box(i)
		if !w.overlaps(s.Pos[0], s.Pos[1], s.Pos[2], b, s.H) {
			continue
		}
		if IsWedge(b) {
			w.hitWedge(s, b, ax, d, old)
			continue
		}
		rise := b.Max[1] - s.Pos[1]
		if s.OnGround && rise > 0 && rise <= stepUp && w.free(s.Pos[0], b.Max[1], s.Pos[2], s.H) {
			s.Pos[1] = b.Max[1]
			s.EyeOff -= rise
			continue
		}
		if d > 0 {
			s.Pos[ax] = b.Min[ax] - R
		} else {
			s.Pos[ax] = b.Max[ax] + R
		}
		s.Vel[ax] = 0
	}
}

func (w *World) moveV(s *Body, d float64) {
	s.OnGround = false
	w.onRamp = false
	s.Pos[1] += d
	for i := range w.solids() {
		b := w.box(i)
		if !w.overlaps(s.Pos[0], s.Pos[1], s.Pos[2], b, s.H) {
			continue
		}
		if d < 0 {
			if i >= len(w.Boxes) && !w.free(s.Pos[0], b.Max[1], s.Pos[2], s.H) {
				continue // a mover (or a head) risen into a body with no room above: it passes through (never crushes)
			}
			if IsWedge(b) {
				if !Walkable(b) {
					pushOff(s, b) // too steep to stand on: slide
					continue
				}
				s.Pos[1] = wedgeTop(b, s.Pos[0], s.Pos[2], R)
				s.OnGround, w.onRamp = true, true
				s.Vel[1] = 0
				continue
			}
			s.Pos[1] = b.Max[1]
			s.OnGround = true
			w.onRamp = false
		} else {
			s.Pos[1] = b.Min[1] - s.H
		}
		s.Vel[1] = 0
	}
}

// Step advances the body by dt (capped at 0.1 s, sub-stepped at 120 Hz).
// A body with its own gravity steps in its frame (gravity.go).
func (w *World) Step(s *Body, in Input, dt float64) Events {
	if s.Noclip {
		w.noclip(s, in, math.Min(dt, 0.1))
		return Events{}
	}
	hev := 0
	if s.Boots {
		hev = w.hook(s, in, math.Min(dt, 0.1))
	}
	var ev Events
	if s.Grav != GravDown {
		ev = w.stepRotated(s, in, dt)
	} else {
		ev = w.step(s, in, dt)
	}
	if s.Hook.On {
		t1 := w.T + math.Min(dt, 0.1) // the step's end: where the movers are now
		if s.Hook.Mover > 0 && !w.hookFollow(s, t1) {
			s.Hook = Hook{}
			hev = HookReleased
		} else if f := w.hookLand(s, t1); f != 0 {
			hev = f
		}
	}
	ev.Hook = hev
	return ev
}

func (w *World) step(s *Body, in Input, dt float64) Events {
	var ev Events
	dt = math.Min(dt, 0.1)
	if len(w.Movers) > 0 {
		w.movers(s, w.T, w.T)
	} else if !w.keepCur {
		w.cur = w.cur[:0] // (touchOut checks it)
	}
	w.peers(s)
	sy, cy := detmath.Sin(s.Yaw), detmath.Cos(s.Yaw)
	w.unstick(s, float64(-sy*in.F)+float64(cy*in.R), float64(-cy*in.F)-float64(sy*in.R), dt) // the wish direction, as sub()
	if len(w.Movers) > 0 {
		w.movers(s, w.T, w.T) // where they are as the command starts (crouch room)
	}
	w.setCrouch(s, in.Crouch)
	target := Eye
	if s.Crouched {
		target = EyeCrouch
	}
	s.EyeH += float64((target - s.EyeH) * (1 - detmath.Exp(-dt*22)))
	n := max(1, int(math.Ceil(dt/subStepMax)))
	h := dt / float64(n)
	for i := range n {
		w.riding = false
		w.st = w.T + float64(float64(i+1)*h)
		if len(w.Movers) > 0 {
			t0, t1 := w.T+float64(float64(i)*h), w.T+float64(float64(i+1)*h)
			w.movers(s, t0, t1)
			w.carry(s, t0, t1) // in a ship's cabin: moved with it (frames.go)
			w.ride(s)
		}
		w.sub(s, in, h, &ev)
	}
	s.EyeOff *= detmath.Exp(-dt * 14)
	return ev
}

func clampAbs(v, m float64) float64 { return math.Max(-m, math.Min(m, v)) }

func (w *World) sub(s *Body, in Input, dt float64, ev *Events) {
	sy, cy := detmath.Sin(s.Yaw), detmath.Cos(s.Yaw)
	// float64(a*b): no fused multiply-add on any CPU, so JS gets the same bits
	// (internal/gojs checks this).
	wx, wz := float64(-sy*in.F)+float64(cy*in.R), float64(-cy*in.F)-float64(sy*in.R)
	wl := detmath.Hypot(wx, wz)
	if wl > 0 {
		wx /= wl
		wz /= wl
	}
	mx := Run
	if s.Crouched && s.OnGround {
		mx = crouchMax
	} else if in.Walk {
		mx = Walk
	}
	if in.Speed > 0 {
		mx *= in.Speed
	}
	lvl := 0
	if len(w.Water) > 0 {
		lvl = w.WaterLevel(s)
		if lvl >= 2 {
			w.swim(s, in, lvl, dt, ev)
			return
		}
		if lvl == 1 {
			mx *= wadeScale
		}
	}
	zg, zj := w.zoneRules(s)
	if zg == 0 {
		w.space(s, in, dt)
		return
	}
	w.jumpK = zj
	jump := in.Jump || (w.Move.AutoBhop && in.JumpHeld)
	if w.Move.Model == MoveSource {
		w.sourceMove(s, jump, wx, wz, wl, mx, dt, ev)
	} else {
		if s.OnGround || wl > 0 {
			acc := airAcc
			if s.OnGround {
				acc = groundAcc
			}
			a := acc * dt
			h0 := math.Sqrt(float64(s.Vel[0]*s.Vel[0]) + float64(s.Vel[2]*s.Vel[2]))
			s.Vel[0] += clampAbs(float64(wx*mx)-s.Vel[0], a)
			s.Vel[2] += clampAbs(float64(wz*mx)-s.Vel[2], a)
			// In the air, faster than a run (a pad's throw, a hook's swing let
			// go): the keys steer it, they don't brake it (as in space, zones.go).
			if h1 := math.Sqrt(float64(s.Vel[0]*s.Vel[0]) + float64(s.Vel[2]*s.Vel[2])); !s.OnGround && h0 > Run && h1 < h0 && h1 > 1e-9 {
				k := h0 / h1
				s.Vel[0] *= k
				s.Vel[2] *= k
			}
		}
		if jump && s.OnGround {
			s.Vel[1] = float64(JumpSpeed * zj)
			s.OnGround = false
			ev.Jumped = true
		}
	}
	if len(w.Launch) > 0 {
		w.launch(s, ev)
	}
	s.Vel[1] -= float64(float64(Grav*zg) * dt)

	wasGround, fall, p0 := s.OnGround, -s.Vel[1], s.Pos
	w.moveH(s, 0, s.Vel[0]*dt)
	w.moveH(s, 2, s.Vel[2]*dt)
	dy := s.Vel[1] * dt
	if zg < 1 && dy < 0 && dy > -2*eps {
		dy = -2 * eps // low gravity (zones.go): a fall slower than the overlap tolerance still finds the ground
	}
	w.moveV(s, dy)
	if w.riding && !s.OnGround && s.Vel[1] <= 0 {
		w.stepDown(s)
	}
	if wasGround && !s.OnGround && s.Vel[1] <= 0 {
		// Walking down a ramp: stay on it (as steep as ground gets, plus a margin).
		w.snapRamp(s, float64(detmath.Hypot(s.Vel[0], s.Vel[2])*dt*1.1)+0.01, p0)
	}
	if len(w.Water) > 0 && lvl == 0 && fall > 0 && w.WaterLevel(s) > 0 {
		ev.Splash = math.Max(ev.Splash, fall)
	}
	if s.OnGround && !wasGround && fall > 3 {
		ev.Landed = math.Max(ev.Landed, fall)
	}
	sp := detmath.Hypot(s.Vel[0], s.Vel[2])
	if s.OnGround && sp > Walk+0.5 {
		s.Stride += float64(sp * dt)
		if s.Stride > stride {
			s.Stride -= stride
			ev.Steps++
		}
	}
}
