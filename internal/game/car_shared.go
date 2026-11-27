//gojs:shared

package game

// Driving a car (docs/racing.md), shared with the client: the driver's
// controls and one command of a driver. The driver sits (crouched) in the
// car's seat (CarSpec.Seat), carried with it, and steers it with the move
// keys (W/S throttle and brake/reverse, A/D steering, jump: the handbrake);
// their client predicts the car with this same code. Getting in is the
// server's (cars.go): the client adopts it from the self-snapshot. Getting
// out (E) is predicted (CarLeave).

import (
	"math"

	"github.com/gigurra/counterslop/internal/detmath"
	"github.com/gigurra/counterslop/internal/sim"
)

// CarBoardReach is how far from a car's footprint centre (feet, in x and z)
// E gets you in.
const CarBoardReach = 3.2

// CarInputOf is the driver's car input from a command's move axes f, r and
// buttons b: the jump button is the handbrake, +boost the boost.
func CarInputOf(f, r float64, b int) sim.CarInput {
	return sim.CarInput{Throttle: f, Steer: r, Handbrake: b&BtnJump != 0, Boost: b&BtnBoost != 0}
}

// DriveStep is one command (move axes f, r, buttons b) of the driver of car c
// (type sp) over dt: the car steps (cars: the other cars' discs), then the
// driver sits in its seat, crouched, moving with it.
func DriveStep(w *sim.World, sp *sim.CarSpec, c *sim.Car, s *sim.Body, f, r float64, b int, cars []sim.CarDisc, dt float64) {
	w.Cars = cars
	w.StepCar(sp, c, CarInputOf(f, r, b), dt)
	w.Cars = nil
	CarSeat(sp, c, s, dt)
}

// CarSeat puts the driver s in car c's seat, crouched, moving with it. The
// body itself is at rest in the seat (Vel zero): the gun's spread counts the
// shooter's own movement, not the car's (ShotSpread), so a driver shoots as
// one standing crouched (getting out takes the car's velocity: CarLeave).
func CarSeat(sp *sim.CarSpec, c *sim.Car, s *sim.Body, dt float64) {
	s.Pos = sim.CarSeatWorld(sp, c)
	s.Vel = sim.Vec3{0, 0, 0}
	s.OnGround = true
	s.Crouched = true
	s.H = sim.HCrouch
	s.EyeH += float64((sim.EyeCrouch - s.EyeH) * (1 - detmath.Exp(-dt*22)))
}

// CarLeaves: command buttons b after prevB get the driver out: E pressed,
// unless the car is locked (a race is on: me.cl in the self-snapshot).
func CarLeaves(b, prevB int, locked bool) bool { return !locked && (b&^prevB)&BtnUse != 0 }

// CarLeave puts the driver of car c out over its seat, standing on top of
// its roof, moving with it (cars are solid to bodies on foot: you ride the
// roof until you step off or it drives out from under you) and on foot from
// there on (this same command).
func CarLeave(sp *sim.CarSpec, c *sim.Car, s *sim.Body) {
	p := sim.CarSeatWorld(sp, c)
	s.Pos = sim.Vec3{p[0], c.Pos[1] + sp.Size[1] + 0.05, p[2]}
	s.Vel = c.Vel
	s.Vel[1] = 0
	s.OnGround = false
	s.Crouched = false // standing up out of the seat (not an unduck in the air: the feet stay on the roof)
	s.H = sim.H
}

// What a garage's upgrades make of a car (internal/game carshop.go, cars.json
// "shop"); the server fits a car with these (fitCar, refillCharges, carTank)
// and the client's garage (web/game/garage.js) shows the same numbers.

// FitMaxHP is the car HP of a type with base HP hp under plating tiers of k
// (cars.json shop.plating) each.
func FitMaxHP(hp int, plating int, k float64) int {
	return int(math.Floor(float64(float64(hp)*float64(1+float64(k*float64(plating)))) + 0.5))
}

// FitCharges is an item's charges a lap (base) with charge packs of k each.
func FitCharges(base int, packs int, k float64) int {
	return int(math.Floor(float64(float64(base)*float64(1+float64(k*float64(packs)))) + 0.5))
}

// FitTank is a full boost tank (s): base plus per a tank tier.
func FitTank(base, per float64, tier int) float64 { return base + float64(per*float64(tier)) }

// FitTopSpeed is a type's top speed (m/s) with engine tiers (= sim StepCar).
func FitTopSpeed(top float64, engine int) float64 {
	return float64(top * float64(1+float64(sim.CarEngineTop*float64(engine))))
}
