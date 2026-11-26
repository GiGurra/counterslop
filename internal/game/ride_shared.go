//gojs:shared

package game

// Riding a boat (docs/physics.md, Boats), shared with the client: the seats
// and one command of a rider. A rider sits (crouched) at their seat, carried
// with the boat; the driver (seat 0, at the outboard) steers it with the
// move keys, and their client predicts the boat with this same code.
// Boarding is the server's (boats.go: who sits where): the client adopts it
// from the self-snapshot. Getting out is predicted (RideLeave).

import (
	"github.com/gigurra/counterslop/internal/detmath"
	"github.com/gigurra/counterslop/internal/sim"
)

const (
	BoatSeats  = 3   // the driver and two passengers
	BoardReach = 2.6 // m from the boat's centre (feet, in x and z) that E boards it
)

// SeatLocal is seat i in the boat's frame: x to starboard (right), y up from
// the waterline, z aft (the bow is -z). Seat 0 is the driver's, at the stern.
func SeatLocal(seat int) (float64, float64, float64) {
	switch seat {
	case 0:
		return 0, 0.1, 1.15
	case 1:
		return 0, 0.1, 0.2
	}
	return 0, 0.1, -0.75
}

// SeatWorld is where a rider's feet are in seat i of boat b.
func SeatWorld(b *sim.Boat, seat int) sim.Vec3 {
	hx, hz := sim.BoatHeading(b.Yaw)
	x, y, z := SeatLocal(seat)
	// right = (-hz, hx); the seat is x to the right and z behind (against the heading).
	return sim.Vec3{b.Pos[0] - float64(hz*x) - float64(hx*z), b.Pos[1] + y, b.Pos[2] + float64(hx*x) - float64(hz*z)}
}

// RideStep is one command (move axes f, r) of a rider in seat i of boat b
// over dt: the driver steps the boat with them (throttle f, tiller r), then
// the rider sits at their seat, crouched, moving with the boat.
func RideStep(w *sim.World, b *sim.Boat, s *sim.Body, f, r float64, seat int, dt float64) {
	if seat == 0 {
		w.StepBoat(b, sim.BoatInput{F: f, R: r}, dt)
	}
	s.Pos = SeatWorld(b, seat)
	s.Vel = b.Vel
	s.OnGround = true
	s.Crouched = true
	s.H = sim.HCrouch
	s.EyeH += float64((sim.EyeCrouch - s.EyeH) * (1 - detmath.Exp(-dt*22)))
}

// RideLeaves: command buttons b after prevB get a rider out (E or jump pressed).
func RideLeaves(b, prevB int) bool { return (b&^prevB)&(BtnUse|BtnJump) != 0 }

// RideLeave puts a rider out of boat b where they sat, moving with the boat;
// hop (jump pressed): up at jump speed. They move on foot from there on
// (this same command).
func RideLeave(b *sim.Boat, s *sim.Body, hop bool) {
	s.Pos[1] += 0.05
	s.Vel = b.Vel
	s.OnGround = false
	if hop {
		s.Vel[1] = sim.JumpSpeed
	}
}
