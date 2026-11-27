package game

import (
	"slices"

	"github.com/gigurra/counterslop/internal/sim"
)

// The pit stop (docs/racing.md "The garage"): on a race map the garage opens
// at any time, from anywhere (on foot, in a car, mid-air; no zones, no
// distance). Only the phase decides what it does: before GO and over the
// results it is the shop; while a race is live it takes a car racer off
// the track. Their car is parked (out of the world: not drawn, not
// drivable, not hittable, no collisions) and so is the driver (held like a
// body that vanished: no corpse, nothing hits it, no commands run); the race
// clock keeps running. They shop as on the grid. Leaving puts them back on
// the last gate they passed (a wreck's respawn place) in a car built from
// their garage, repaired and refilled, their guns kept. The time spent in
// the pit is the penalty.
//
// The client says what it wants, {t:"pit", c: in, seq}: the garage open or
// not. It is level-triggered and resend-safe: the server keeps the newest
// seq's wish (an older or repeated one is dropped) and acknowledges it in
// every self-snapshot (me.ps); the client resends while its seq is ahead.
// Whether a racer is in the pit is the server's (me.pit, the race list's
// "pit"); the client predicts nothing of it (in the pit there is no body to
// predict: me.dead).

// pitPark is where a parked car waits: far outside every map.
var pitPark = sim.Vec3{0, 1e6, 0}

// pitMsg handles {t:"pit", c, seq}: p's newest wish. Callers hold s.mu.
func (s *Server) pitMsg(p *Player, in bool, seq int) {
	if seq <= p.pitSeq {
		return // a duplicate or older (reordered) wish
	}
	p.pitSeq, p.pitWant = seq, in
	if s.raceLive() {
		s.tickPit(p) // at once: the snapshot after this message already says
	}
}

// pitAllowed: p's wish to be in the pit can be met now: a car racer in a
// live race, not finished. Callers hold s.mu.
func (s *Server) pitAllowed(p *Player) (*racerState, bool) {
	rs := s.racer(p)
	if !s.raceLive() || s.onFoot() || rs == nil || rs.Car == 0 || rs.Place > 0 {
		return rs, false
	}
	return rs, true
}

// tickPit moves p into or out of the pit as their wish says. Callers hold
// s.mu.
func (s *Server) tickPit(p *Player) {
	rs, ok := s.pitAllowed(p)
	switch {
	case p.pit && (!ok || !p.pitWant):
		s.pitOut(p, rs)
	case !p.pit && ok && p.pitWant:
		s.pitIn(p, rs)
	}
	if rs != nil {
		rs.Pit = p.pit
	}
}

// pitIn takes racer p off the track: the car parked, the driver held.
func (s *Server) pitIn(p *Player, rs *racerState) {
	p.pit, p.pitAlive = true, !p.Dead
	if p.Dead && rs.backAt.IsZero() { // wrecked this tick: the wreck's countdown still holds
		rs.backAt = s.clock().Add(secs(raceRespawnSecs))
	}
	if !p.Dead {
		p.Dead, p.Vanish, p.DeathTick = true, true, s.tick // no corpse: gone into the garage
		p.ride, p.seat, p.car = 0, 0, 0
		p.cmds = p.cmds[:0]
	}
	if c := s.carByID(rs.Car); c != nil {
		s.parkCar(c)
	}
}

// parkCar takes car c out of the world (the pit stop).
func (s *Server) parkCar(c *Car) {
	c.parked = true
	c.sim = *sim.NewCar(c.spec, pitPark, 0)
	c.Driver, c.Fx = 0, 0
	c.sync()
}

// pitOut puts p back on the track from the pit (rs nil: not racing any
// more: just out). A racer who was wrecked when they went in waits out
// what is left of the wreck's countdown as ever.
func (s *Server) pitOut(p *Player, rs *racerState) {
	p.pit = false
	if rs != nil {
		rs.Pit = false
	}
	if !p.pitAlive && p.Dead && rs != nil && !rs.backAt.IsZero() && s.clock().Before(rs.backAt) {
		return // still wrecked: tickRace brings them back at backAt
	}
	if rs == nil || rs.Car == 0 {
		if p.Dead && p.pitAlive { // not racing any more: just a player again
			s.respawnKeep(p, nil)
		}
		return
	}
	s.respawnKeep(p, rs)
}

// respawnKeep is raceRespawn keeping what p carries (guns bought in the pit,
// armour, the driver's HP): rs nil, a plain respawn.
func (s *Server) respawnKeep(p *Player, rs *racerState) {
	inv, armor, helmet, kit, hp, alive := p.inv, p.Armor, p.Helmet, p.Kit, p.HP, p.pitAlive
	if rs != nil {
		s.raceRespawn(p, rs)
	} else {
		s.respawn(p)
	}
	if alive && inv != nil {
		p.inv, p.Armor, p.Helmet, p.Kit = inv, armor, helmet, kit
		p.HP = max(1, min(hp, maxHP))
		s.sendInv(p)
	}
}

// pitWishesOff: GO. The garage wishes from the grid are dropped (the client
// closes the garage at GO: opening it again sends a fresh one). Callers hold
// s.mu.
func (s *Server) pitWishesOff() {
	for _, p := range s.players {
		p.pitWant = false
	}
}

// tickPitStops runs the pit stop every live race tick: every racer's wish
// met. Callers hold s.mu.
func (s *Server) tickPitStops() {
	s.parkedOrphans()
	for _, p := range s.ordered() {
		if p.pit || p.pitWant {
			s.tickPit(p)
		}
	}
}

// pitsOut: the race is over (or a new one starts): everyone out of the pit,
// back on their gate for the results. Callers hold s.mu.
func (s *Server) pitsOut() {
	for _, p := range s.ordered() {
		if p.pit {
			s.pitOut(p, s.racer(p))
		}
	}
}

// unparkOf: p leaves the pit stop some other way (a respawn: a team switch,
// a new round): their car back at its start, in the world.
func (s *Server) unparkOf(p *Player) {
	p.pit = false
	rs := s.racer(p)
	if rs == nil {
		return
	}
	rs.Pit = false
	if c := s.carByID(rs.Car); c != nil && c.parked {
		c.parked = false
		s.carHome(c)
		c.sync()
	}
}

// parkedOrphans: a car parked for a racer who is gone (left the server):
// back at its start for the next one. Callers hold s.mu.
func (s *Server) parkedOrphans() {
	for _, c := range s.cars {
		if !c.parked {
			continue
		}
		held := false
		for _, rs := range s.round.Racers {
			if p := s.players[rs.ID]; rs.Car == c.ID && p != nil && p.pit {
				held = true
			}
		}
		if !held {
			c.parked = false
			s.carHome(c)
			c.sync()
		}
	}
}

// garageOwns: car type id is p's this match (bought, or free).
func (s *Server) garageOwns(p *Player, id string) bool {
	sp := CarSpecOf(id)
	if sp == nil {
		return false
	}
	if sp.Price <= 0 {
		return true
	}
	return p.garage != nil && slices.Contains(p.garage.Owned, sp.ID)
}
