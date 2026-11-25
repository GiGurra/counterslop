package sim

import (
	"math"

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
