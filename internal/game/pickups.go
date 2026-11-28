package game

import (
	"math"
	"time"

	c "github.com/gigurra/counterslop/internal/content"
)

// Power-ups on a race track (content.Map.Pickups, docs/racing.md
// "Power-ups"): a car driving through one takes it. Server-owned: each
// pickup's kind, place and whether it is up ride in every snapshot's round
// ("pu"); what one did to a car shows in the car's state (HP, boost, charges,
// its shield), the money in the driver's. The "pickup" car event (the sound,
// the announcer's line) is optional.

// pickupState is one pickup as everyone sees it.
type pickupState struct {
	Kind string `json:"k" net:"cold"`
	Pos  Vec3   `json:"p" net:"cold,mm"`
	Up   bool   `json:"up,omitempty" net:"cold"` // there to take

	backAt  time.Time `net:"-"` // taken: when it comes back
	respawn float64   `net:"-"` // seconds it stays away once taken
}

const (
	pickupFreezeSecs = 4.0 // Code Freeze: seconds of no damage
	pickupHotfixFrac = 0.4 // Hotfix Patch: the car's max HP it repairs
)

// pickupRespawn: seconds a kind stays away once taken (content.Pickup.Respawn: a map's own).
var pickupRespawn = map[string]float64{
	c.PickupBonus: 30, c.PickupHotfix: 20, c.PickupReorg: 20, c.PickupCaffeine: 15, c.PickupFreeze: 25, c.PickupFlag: 20,
}

// flagKinds: what a Feature Flag turns out to be.
var flagKinds = []string{c.PickupBonus, c.PickupHotfix, c.PickupReorg, c.PickupCaffeine, c.PickupFreeze}

// resetPickups puts every pickup of the map up (a race's start).
func (s *Server) resetPickups() {
	r := &s.round
	r.Pickups = nil
	if !s.raceMode() {
		return
	}
	for _, mp := range s.Map.Pickups {
		sec := mp.Respawn
		if sec <= 0 {
			sec = pickupRespawn[mp.Kind]
		}
		r.Pickups = append(r.Pickups, pickupState{Kind: mp.Kind, Pos: Vec3(mp.Pos), Up: true, respawn: sec})
	}
}

// tickPickups: in a live race a driven car takes each pickup it drives
// through; a taken one comes back on time. The shields' state for the
// snapshot. Callers hold s.mu.
func (s *Server) tickPickups(now time.Time) {
	for _, car := range s.cars {
		car.Shield = now.Before(car.cs.shieldUntil)
	}
	if !s.raceLive() {
		return
	}
	for i := range s.round.Pickups {
		pu := &s.round.Pickups[i]
		if !pu.Up {
			if !now.Before(pu.backAt) {
				pu.Up = true
			}
			continue
		}
		for _, car := range s.cars {
			d := s.players[car.Driver]
			if d == nil || d.Dead || car.sim.HP <= 0 || !s.pickupSwept(pu.Pos, car, d) {
				continue
			}
			pu.Up, pu.backAt = false, now.Add(time.Duration(pu.respawn*float64(time.Second)))
			kind := pu.Kind
			if kind == c.PickupFlag {
				kind = flagKinds[s.rng.IntN(len(flagKinds))]
			}
			s.applyPickup(car, d, kind, now)
			ev := carEvent{T: "car", Ev: "pickup", Item: kind, At: pu.Pos, Car: car.ID, By: d.ID}
			if pu.Kind == c.PickupFlag {
				ev.Via = c.PickupFlag
			}
			s.sendAll(ev)
			break
		}
	}
}

// pickupSwept: car (driven by d) drove through a pickup at q this tick:
// along every step its driver's commands drove it (sweep.go: a burst of
// commands can't carry it past), the car's middle where the driver's seat
// was (offset as it is now).
func (s *Server) pickupSwept(q Vec3, car *Car, d *Player) bool {
	m := car.sim.Pos
	off := Vec3{m[0] - d.Pos[0], m[1] - d.Pos[1], m[2] - d.Pos[2]}
	return d.swept(func(a, b Vec3) bool {
		a = Vec3{a[0] + off[0], a[1] + off[1], a[2] + off[2]}
		b = Vec3{b[0] + off[0], b[1] + off[1], b[2] + off[2]}
		return nearPath(q, a, b, c.PickupRadius, 2.5)
	})
}

// applyPickup: what a power-up of kind does to car (driven by d).
func (s *Server) applyPickup(car *Car, d *Player, kind string, now time.Time) {
	switch kind {
	case c.PickupBonus:
		s.pay(d, s.econ().Race.Bonus, "Bonus Approved")
	case c.PickupHotfix:
		car.sim.HP = min(car.MaxHP, car.sim.HP+int(math.Round(float64(car.MaxHP)*pickupHotfixFrac)))
		car.cs.repair = 0
	case c.PickupReorg:
		s.refillCharges(car, s.garageOf(d))
	case c.PickupCaffeine:
		car.sim.Boost = carTank(s.garageOf(d))
	case c.PickupFreeze:
		car.cs.shieldUntil = now.Add(time.Duration(pickupFreezeSecs * float64(time.Second)))
		car.Shield = true
	}
	car.sync()
}

// shielded: car c takes no damage now (Code Freeze).
func (s *Server) shielded(car *Car) bool { return s.clock().Before(car.cs.shieldUntil) }
