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
}

func NewBody(spawn Vec3) *Body { return &Body{Pos: spawn, H: H, EyeH: Eye} }

// Input is one tick's movement intent. Jump means "jump now if on the
// ground"; edge detection (one jump per press) happens in the command layer.
type Input struct {
	F, R               float64
	Walk, Jump, Crouch bool
	Speed              float64 // max-speed scale (the held weapon: CS-style lighter = faster); 0 = 1
}

// Events are the sound-worthy things that happened during a step.
type Events struct {
	Jumped bool
	Landed float64
	Steps  int
}

type World struct {
	Boxes  []content.Box   // static boxes the body collides with
	Movers []content.Mover // moving solids (movers.go)
	T      float64         // the time the stepped command starts (the movers' clock), seconds
	Peers  []Peer          // the other solid players (peers.go), set per stepped body
	dyn    []solid         // the movers near the body in the current sub-step
	cur    []content.Box   // their boxes at its end: collided with like Boxes
	peer   []content.Box   // the peers' hulls this body collides with in the current Step
	riding bool            // the body is carried by a mover in this sub-step
}

// NewWorld is the movement world of a map: its boxes (less the static treads
// its escalators replace) and its movers.
func NewWorld(m *content.Map) *World {
	w := &World{Boxes: m.Boxes, Movers: m.Movers, T: 0, Peers: nil, dyn: nil, cur: nil, peer: nil, riding: false}
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
		z+R > b.Min[2]+eps && z-R < b.Max[2]-eps
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
	s.Pos[ax] += d
	for i := range w.solids() {
		b := w.box(i)
		if !w.overlaps(s.Pos[0], s.Pos[1], s.Pos[2], b, s.H) {
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
			s.Pos[1] = b.Max[1]
			s.OnGround = true
		} else {
			s.Pos[1] = b.Min[1] - s.H
		}
		s.Vel[1] = 0
	}
}

// Step advances the body by dt (capped at 0.1 s, sub-stepped at 120 Hz).
func (w *World) Step(s *Body, in Input, dt float64) Events {
	var ev Events
	dt = math.Min(dt, 0.1)
	w.peers(s)
	if len(w.Movers) > 0 {
		w.movers(s, w.T, w.T)
	}
	w.unstick(s, dt)
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
		if len(w.Movers) > 0 {
			w.movers(s, w.T+float64(float64(i)*h), w.T+float64(float64(i+1)*h))
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
	if s.OnGround || wl > 0 {
		acc := airAcc
		if s.OnGround {
			acc = groundAcc
		}
		a := acc * dt
		s.Vel[0] += clampAbs(float64(wx*mx)-s.Vel[0], a)
		s.Vel[2] += clampAbs(float64(wz*mx)-s.Vel[2], a)
	}
	if in.Jump && s.OnGround {
		s.Vel[1] = JumpSpeed
		s.OnGround = false
		ev.Jumped = true
	}
	s.Vel[1] -= float64(Grav * dt)

	wasGround, fall := s.OnGround, -s.Vel[1]
	w.moveH(s, 0, s.Vel[0]*dt)
	w.moveH(s, 2, s.Vel[2]*dt)
	w.moveV(s, s.Vel[1]*dt)
	if w.riding && !s.OnGround && s.Vel[1] <= 0 {
		w.stepDown(s)
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
