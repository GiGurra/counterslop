package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/detmath"
)

// The inflatable dinghy (docs/physics.md, Boats): a small boat on the water,
// stepped by its driver's commands (the driver's prediction runs this same
// code) or, with nobody at the tiller, with no input: it drifts to a stop.
//
// Pos is the hull's centre at the waterline, Yaw its heading (as a player's:
// 0 faces -z), Vel its velocity, YawVel its turn rate. In the water the boat
// sits on the surface (Pos[1] = the water's top: the bobbing is drawn, not
// simulated). Out of water it stays at its height and only slows down.
//
// Thrust (Input.F) pushes along the heading (full ahead to BoatMax, astern
// slower); the tiller (Input.R) turns it, harder the faster it goes (a little
// even at rest). The water drags it: little along the heading, a lot sideways
// (it is a boat, not a puck). The hull is two discs (bow and stern) at the
// waterline against the map's boxes (a wedge as its box) between its keel
// and its tubes: pushed out, the speed into a box taken away, so it comes to
// rest against a quay or a ship's hull and slides along it.
const (
	BoatMax      = 11.0 // m/s, full ahead
	boatAstern   = 4.0  // m/s, full astern
	boatThrust   = 7.0  // m/s² ahead
	boatReverse  = 4.0  // m/s² astern
	boatDragFwd  = 0.35 // per second, along the heading
	boatDragSide = 3.0  // per second, sideways
	boatTurn     = 1.5  // rad/s, full tiller at speed
	boatTurnK    = 5.0  // per second: the turn rate follows the tiller
	BoatDisc     = 0.85 // m: the hull's discs' radius
	BoatDiscOff  = 0.95 // m: and their centres fore and aft of Pos
	boatKeel     = 0.35 // m under the waterline
	boatTubes    = 0.6  // m over it
)

// Boat is one boat's simulated state.
type Boat struct {
	Pos    Vec3
	Vel    Vec3
	Yaw    float64
	YawVel float64
}

// BoatInput is the driver's: F thrust (-1 astern .. 1 ahead), R tiller (-1 left .. 1 right).
type BoatInput struct {
	F, R float64
}

// BoatHeading is the unit vector the boat at yaw points along (x, z).
func BoatHeading(yaw float64) (float64, float64) { return -detmath.Sin(yaw), -detmath.Cos(yaw) }

// StepBoat moves b by one step of dt.
func (w *World) StepBoat(b *Boat, in BoatInput, dt float64) {
	hx, hz := BoatHeading(b.Yaw)
	fwd := float64(b.Vel[0]*hx) + float64(b.Vel[2]*hz)
	side := float64(b.Vel[0]*hz) - float64(b.Vel[2]*hx) // along (hz, -hx): the boat's right
	f := clampAbs(in.F, 1)
	if f > 0 && fwd < BoatMax {
		fwd = math.Min(BoatMax, fwd+float64(f*boatThrust*dt))
	} else if f < 0 && fwd > -boatAstern {
		fwd = math.Max(-boatAstern, fwd+float64(f*boatReverse*dt))
	}
	fwd *= 1 - float64(boatDragFwd*dt)
	side *= math.Max(0, 1-float64(boatDragSide*dt))
	// The tiller: right (R > 0) turns right (yaw falls); astern it swaps.
	k := math.Max(0.25, math.Min(1, math.Abs(fwd)/4))
	if fwd < 0 {
		k = -k
	}
	want := -float64(clampAbs(in.R, 1) * boatTurn * k)
	b.YawVel += float64((want - b.YawVel) * math.Min(1, boatTurnK*dt))
	b.Yaw += float64(b.YawVel * dt)
	hx, hz = BoatHeading(b.Yaw)
	b.Vel[0] = float64(fwd*hx) + float64(side*hz)
	b.Vel[2] = float64(fwd*hz) - float64(side*hx)
	b.Vel[1] = 0
	b.Pos[0] += float64(b.Vel[0] * dt)
	b.Pos[2] += float64(b.Vel[2] * dt)
	if top := w.waterTop(b.Pos[0], b.Pos[1]-boatKeel, b.Pos[2]); top > -1e300 { // (waterTop: -Inf for none)
		b.Pos[1] = top
	}
	for range 2 {
		w.boatDisc(b, hx, hz, BoatDiscOff)
		w.boatDisc(b, hx, hz, -BoatDiscOff)
	}
}

// boatDisc pushes the boat's disc at off along the heading out of the boxes
// it is in, and takes away its speed into them.
func (w *World) boatDisc(b *Boat, hx, hz, off float64) {
	lo, hi := b.Pos[1]-boatKeel, b.Pos[1]+boatTubes
	for i := range w.Boxes {
		bx := &w.Boxes[i]
		if bx.Max[1] <= lo || bx.Min[1] >= hi {
			continue
		}
		cx, cz := b.Pos[0]+float64(hx*off), b.Pos[2]+float64(hz*off)
		px, pz := math.Max(bx.Min[0], math.Min(bx.Max[0], cx)), math.Max(bx.Min[2], math.Min(bx.Max[2], cz))
		dx, dz := cx-px, cz-pz
		d := detmath.Hypot(dx, dz)
		if d >= BoatDisc {
			continue
		}
		var nx, nz, push float64
		if d > 1e-9 {
			nx, nz, push = dx/d, dz/d, BoatDisc-d
		} else { // the centre is in the box: out of its nearest side
			e := [4]float64{cx - bx.Min[0], bx.Max[0] - cx, cz - bx.Min[2], bx.Max[2] - cz}
			j := 0
			for k := 1; k < 4; k++ {
				if e[k] < e[j] {
					j = k
				}
			}
			n := [4][2]float64{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
			nx, nz, push = n[j][0], n[j][1], e[j]+BoatDisc
		}
		b.Pos[0] += float64(nx * push)
		b.Pos[2] += float64(nz * push)
		if vn := float64(b.Vel[0]*nx) + float64(b.Vel[2]*nz); vn < 0 {
			b.Vel[0] -= float64(vn * nx)
			b.Vel[2] -= float64(vn * nz)
		}
		b.YawVel *= 0.5
	}
}

// Standing on a boat (docs/physics.md, Boats: the deck). A body on a boat's
// floor moves in the boat's frame: Deck is its feet and velocity there (x to
// starboard, y up from the waterline, z aft: the bow is -z), and StepDeck
// runs the usual movement (World.Step, either model) in a tiny world of its
// own, the floor across the hull, so walking, jumping and crouching on a
// boat are the same as anywhere. Where the body is in the map follows from
// the boat's pose (DeckWorld): carried with the boat, turning with it. The
// map's boxes don't touch a body on the deck (the hull keeps clear of them:
// boatDisc), nor do other players. Its centre walking off the hull (out of
// the two discs' outline) leaves the deck: a body in the map again, moving
// with the boat.
const (
	DeckTop  = 0.1 // m: the floor's top over the waterline (the seats are on it)
	deckStep = 0.3 // m: feet this far under the floor (coming from a pontoon) step onto it
)

// Deck is a body's place on a boat, in the boat's frame.
type Deck struct {
	Pos Vec3
	Vel Vec3
}

// OnHull: the point x, z of the boat's frame is over the hull (within the
// discs' outline: a stadium BoatDisc round its centre line).
func OnHull(x, z float64) bool {
	c := math.Max(-BoatDiscOff, math.Min(BoatDiscOff, z))
	return detmath.Hypot(x, z-c) < BoatDisc
}

// ToDeck is world point p in boat b's frame (x starboard, y up, z aft).
func ToDeck(b *Boat, p Vec3) Vec3 {
	hx, hz := BoatHeading(b.Yaw)
	dx, dz := p[0]-b.Pos[0], p[2]-b.Pos[2]
	return Vec3{float64(dx*-hz) + float64(dz*hx), p[1] - b.Pos[1], float64(dx*-hx) - float64(dz*hz)}
}

// DeckWorld is point l of boat b's frame in the world.
func DeckWorld(b *Boat, l Vec3) Vec3 {
	hx, hz := BoatHeading(b.Yaw)
	// right = (-hz, hx), aft = (-hx, -hz)
	return Vec3{b.Pos[0] - float64(hz*l[0]) - float64(hx*l[2]), b.Pos[1] + l[1], b.Pos[2] + float64(hx*l[0]) - float64(hz*l[2])}
}

// DeckVel is velocity v of boat b's frame in the world (the boat's own plus
// v turned with it).
func DeckVel(b *Boat, v Vec3) Vec3 {
	hx, hz := BoatHeading(b.Yaw)
	return Vec3{b.Vel[0] - float64(hz*v[0]) - float64(hx*v[2]), b.Vel[1] + v[1], b.Vel[2] + float64(hx*v[0]) - float64(hz*v[2])}
}

// deckBoxes is the deck's world: the floor under the whole hull.
func deckBoxes() []content.Box {
	return []content.Box{{Min: [3]float64{-BoatDisc, -boatKeel, -(BoatDiscOff + BoatDisc)}, Max: [3]float64{BoatDisc, DeckTop, BoatDiscOff + BoatDisc}, Kind: "floor"}}
}

// LandOnBoat puts body s on boat b's deck if this command took its feet
// from y0 (before) down onto the floor (or up to it by deckStep, walking on
// from as low) with its centre over the hull. A body flying (noclip), on a
// hook or in a gravity of its own doesn't land. Reports whether it landed.
func LandOnBoat(b *Boat, s *Body, y0 float64, d *Deck) bool {
	if s.Noclip || s.Hook.On || s.Grav != GravDown {
		return false
	}
	floor := b.Pos[1] + DeckTop
	if s.Pos[1] > floor || y0 < floor-deckStep || s.Vel[1] > 0 {
		return false
	}
	l := ToDeck(b, s.Pos)
	if !OnHull(l[0], l[2]) {
		return false
	}
	hx, hz := BoatHeading(b.Yaw)
	vx, vz := s.Vel[0]-b.Vel[0], s.Vel[2]-b.Vel[2]
	d.Pos = Vec3{l[0], DeckTop, l[2]}
	d.Vel = Vec3{float64(vx*-hz) + float64(vz*hx), 0, float64(vx*-hx) - float64(vz*hz)}
	s.OnGround = true
	s.Pos = DeckWorld(b, d.Pos)
	s.Vel = DeckVel(b, d.Vel)
	return true
}

// StepDeck is one command of body s standing on boat b's deck d (b as the
// command starts): the movement in the boat's frame, then s where that is in
// the world. Off the hull after it (OnHull of d.Pos false), it walked off: a
// body in the map from here on, moving with the boat.
func (w *World) StepDeck(b *Boat, s *Body, d *Deck, in Input, dt float64) Events {
	dw := &World{Boxes: deckBoxes(), Movers: nil, T: 0, Move: w.Move, Peers: nil, Water: nil, dyn: nil, cur: nil, peer: nil, ghost: nil, riding: false, onRamp: false, frames: make([]*World, 6), keepCur: false, fg: GravDown, sup: nil}
	yaw := s.Yaw
	s.Yaw = yaw - b.Yaw
	s.Pos, s.Vel = d.Pos, d.Vel
	din := in
	din.Hook, din.HookPress = false, false // (a hook fired from the deck leaves it first: the caller's)
	ev := dw.Step(s, din, dt)
	d.Pos, d.Vel = s.Pos, s.Vel
	s.Yaw = yaw
	s.Pos, s.Vel = DeckWorld(b, d.Pos), DeckVel(b, d.Vel)
	return ev
}
