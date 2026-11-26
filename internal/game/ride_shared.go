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
	SeatDeck   = -1  // the seat of a body standing on a boat's deck (sim.Deck: not seated)
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

// RideLeaves: command buttons b after prevB get a rider out: E or jump
// pressed, or (boots: wearing the Gravity Override) the hook fired: it pulls
// you out of your seat (the same command fires it on foot).
func RideLeaves(b, prevB int, boots bool) bool {
	out := BtnUse | BtnJump
	if boots {
		out |= BtnHook
	}
	return (b&^prevB)&out != 0
}

// DeckLeaves: command buttons b after prevB take a body standing on a deck
// off it: the hook fired (boots), which pulls you off the boat (the same
// command fires it on foot). E sits down (the server's, boats.go); a jump
// jumps on the deck.
func DeckLeaves(b, prevB int, boots bool) bool {
	return boots && (b&^prevB)&BtnHook != 0
}

// RideGetUp gets a rider (RideLeaves: buttons b after prevB) out of seat in
// boat b: the hook (boots) pulls them out into the map (RideLeave, moving with
// the boat); E or a jump stands them up on the deck where they sat (a jump
// hops up from it), in d. Reports whether they are on the deck.
func RideGetUp(bt *sim.Boat, s *sim.Body, seat, b, prevB int, boots bool, d *sim.Deck) bool {
	press := b &^ prevB
	if boots && press&BtnHook != 0 {
		RideLeave(bt, s, press&BtnJump != 0)
		return false
	}
	x, _, z := SeatLocal(seat)
	d.Pos = sim.Vec3{x, sim.DeckTop, z}
	d.Vel = sim.Vec3{}
	s.OnGround = true
	if press&BtnJump != 0 {
		d.Vel[1] = sim.JumpSpeed
		s.OnGround = false
	}
	DeckCarry(bt, s, d)
	return true
}

// DeckCarry puts body s standing on boat b's deck d where that is in the
// world, moving with the boat (after the boat moved: the server's tickBoats,
// the client's prediction of a boat nobody drives).
func DeckCarry(b *sim.Boat, s *sim.Body, d *sim.Deck) {
	s.Pos = sim.DeckWorld(b, d.Pos)
	s.Vel = sim.DeckVel(b, d.Vel)
}

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
