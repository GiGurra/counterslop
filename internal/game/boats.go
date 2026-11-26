package game

import (
	"encoding/json"
	"math"

	"github.com/gigurra/counterslop/internal/sim"
)

// Boats (docs/physics.md): the map's inflatable dinghies (content.Map.Boats),
// back at their starts every round and on a map change, stepped every tick
// (internal/sim boat.go). Every snapshot carries them ("boats").

// Boat is one boat as everyone sees it.
type Boat struct {
	ID     int      `json:"id" net:"cold"`
	Pos    Vec3     `json:"p" net:"hot,mm"`
	Yaw    Angle    `json:"y" net:"hot,angle"`
	Vel    Vec3     `json:"v" net:"hot,mm"` // the client draws it ahead of the snapshot by this
	sim    sim.Boat `net:"-"`
	start  sim.Boat `net:"-"` // as this tick started: what its deck's commands stand on (whoever's command ran first)
	spawn  int      `net:"-"` // its start in Map.Boats
	driven bool     `net:"-"` // stepped by its driver's command this tick
}

// resetBoats puts every boat back at its start, still. Callers hold s.mu.
func (s *Server) resetBoats() {
	for _, p := range s.players { // everyone off: the boats go back without them
		p.ride, p.seat = 0, 0
	}
	s.boats = s.boats[:0]
	for i, sp := range s.Map.Boats {
		b := &Boat{ID: i + 1, spawn: i}
		b.sim = sim.Boat{Pos: sim.Vec3(sp.Pos), Yaw: sp.Yaw}
		b.sync()
		b.start = b.sim
		s.boats = append(s.boats, b)
	}
}

// sync copies the simulated state to what is sent.
func (b *Boat) sync() {
	b.Pos, b.Yaw, b.Vel = Vec3(b.sim.Pos), Angle(b.sim.Yaw), Vec3(b.sim.Vel)
}

// tickBoats steps the boats nobody drove this tick (they drift to a stop).
// Callers hold s.mu.
func (s *Server) tickBoats(dt float64) {
	if len(s.boats) == 0 {
		return
	}
	w := s.simWorld()
	for _, b := range s.boats {
		if !b.driven {
			w.StepBoat(&b.sim, sim.BoatInput{}, dt)
		}
		b.driven = false
		b.sync()
	}
	// The riders sit (or stand) where their boat is now (a passenger's
	// command may have run before the driver's this tick).
	for _, p := range s.players {
		if b := s.boatByID(p.ride); p.ride != 0 && b != nil && p.body != nil {
			if p.seat == SeatDeck {
				DeckCarry(&b.sim, p.body, &p.deck)
			} else {
				p.body.Pos, p.body.Vel = SeatWorld(&b.sim, p.seat), b.sim.Vel
			}
			p.Pos = Vec3(p.body.Pos)
		}
	}
	for _, b := range s.boats {
		b.start = b.sim
	}
}

// boatsJSON is the boats for the snapshot ("boats":[...]; nothing on a map without).
func (s *Server) boatsJSON() []byte {
	if len(s.boats) == 0 {
		return nil
	}
	b, _ := json.Marshal(s.boats)
	return append([]byte(`"boats":`), b...)
}

// boatByID is boat id, or nil.
func (s *Server) boatByID(id int) *Boat {
	for _, b := range s.boats {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// rideExact is the boat p rides, exact, for its self-snapshot (nil on foot).
func (s *Server) rideExact(p *Player) *boatExact {
	if p.ride == 0 {
		return nil
	}
	b := s.boatByID(p.ride)
	if b == nil {
		return nil
	}
	return &boatExact{Pos: b.sim.Pos, Vel: b.sim.Vel, Yaw: b.sim.Yaw, YawVel: b.sim.YawVel, Driven: s.seatTaken(b.ID, 0)}
}

// deckExact is where p stands on its boat's deck, exact, for its
// self-snapshot (nil: not standing on one).
func (s *Server) deckExact(p *Player) *deckExact {
	if p.ride == 0 || p.seat != SeatDeck {
		return nil
	}
	return &deckExact{Pos: p.deck.Pos, Vel: p.deck.Vel}
}

// seatTaken: someone alive rides boat id in seat.
func (s *Server) seatTaken(id, seat int) bool {
	for _, q := range s.players {
		if q.ride == id && q.seat == seat && !q.Dead && q.body != nil {
			return true
		}
	}
	return false
}

// boardBoat seats p: standing on a boat's deck, in that boat; else in the
// nearest boat within BoardReach (feet, x and z; within 2 m up or down) with
// a free seat. The driver's seat if it is free, else the free passenger seat
// nearest to p. Callers hold s.mu.
func (s *Server) boardBoat(p *Player) bool {
	if (p.ride != 0 && p.seat != SeatDeck) || p.body == nil || p.Dead {
		return false
	}
	var best *Boat
	if p.ride != 0 {
		best = s.boatByID(p.ride)
		if best == nil || s.freeSeat(best, p.body.Pos) < 0 {
			return false
		}
	} else {
		bestD := BoardReach
		for _, b := range s.boats {
			d := math.Hypot(b.sim.Pos[0]-p.body.Pos[0], b.sim.Pos[2]-p.body.Pos[2])
			if d < bestD && math.Abs(b.sim.Pos[1]-p.body.Pos[1]) < 2 && s.freeSeat(b, p.body.Pos) >= 0 {
				best, bestD = b, d
			}
		}
	}
	if best == nil {
		return false
	}
	s.sit(p, best, s.freeSeat(best, p.body.Pos))
	return true
}

// freeSeat is the seat of boat b someone at pos would sit down in: the
// driver's if it is free, else the nearest free one; -1: none.
func (s *Server) freeSeat(b *Boat, pos sim.Vec3) int {
	if !s.seatTaken(b.ID, 0) {
		return 0
	}
	l := sim.ToDeck(&b.sim, pos)
	best, bestD := -1, math.Inf(1)
	for seat := 1; seat < BoatSeats; seat++ {
		if s.seatTaken(b.ID, seat) {
			continue
		}
		x, _, z := SeatLocal(seat)
		if d := math.Hypot(l[0]-x, l[2]-z); d < bestD {
			best, bestD = seat, d
		}
	}
	return best
}

// switchSeat (+seat) moves a seated p of boat b: a passenger to the driver's
// seat if it is free, the driver to the free passenger seat nearest to it.
// Reports whether p moved. Callers hold s.mu.
func (s *Server) switchSeat(p *Player, b *Boat) bool {
	to := -1
	if p.seat != 0 {
		if !s.seatTaken(b.ID, 0) {
			to = 0
		}
	} else {
		for seat := 1; seat < BoatSeats && to < 0; seat++ { // seat 1 is the nearest to the driver's
			if !s.seatTaken(b.ID, seat) {
				to = seat
			}
		}
	}
	if to < 0 {
		return false
	}
	s.sit(p, b, to)
	return true
}

// sit puts p in seat of boat b, crouched, moving with it.
func (s *Server) sit(p *Player, b *Boat, seat int) {
	p.ride, p.seat, p.deck = b.ID, seat, sim.Deck{}
	bd := p.body
	bd.Pos, bd.Vel, bd.OnGround, bd.Crouched, bd.H = SeatWorld(&b.sim, seat), b.sim.Vel, true, true, sim.HCrouch
	p.Pos, p.Crouch = Vec3(p.body.Pos), true
}

// landOnBoats: p's command (on foot) took its feet from y0 onto a boat's
// floor: it stands on that deck (as its client predicts, sim.LandOnBoat;
// each boat as the tick started). Callers hold s.mu.
func (s *Server) landOnBoats(p *Player, y0 float64) {
	if p.ride != 0 || p.body == nil || p.Dead {
		return
	}
	for _, b := range s.boats {
		if sim.LandOnBoat(&b.start, p.body, y0, &p.deck) {
			p.ride, p.seat = b.ID, SeatDeck
			return
		}
	}
}

// leaveBoat gets p up out of its seat (RideGetUp: E or a jump stands it up
// on the deck, the hook pulls it out), or off the deck (the hook). Callers
// hold s.mu.
func (s *Server) leaveBoat(p *Player, c Cmd) {
	b := s.boatByID(p.ride)
	p.leftBoat = p.cmdTicks
	if b == nil || p.body == nil {
		p.ride, p.seat = 0, 0
		return
	}
	if p.seat == SeatDeck || !RideGetUp(&b.start, p.body, p.seat, c.B, p.lastCmd.B, p.body.Boots, &p.deck) { // as the client predicts it
		p.ride, p.seat = 0, 0
		return
	}
	p.seat = SeatDeck
}

// rideCmd runs command c (movement in) of p if it rides a boat. Seated: E,
// a jump or the hook gets up (RideLeaves), else RideStep (the driver
// steers). On the deck: the hook takes it off, else it moves there
// (StepDeck) until it walks off the hull. Reports whether p rode this
// command (false: it moves on foot, this same command) and, on the deck, how
// far its feet moved on it (the walk cycle). Callers hold s.mu.
func (s *Server) rideCmd(w *sim.World, p *Player, c Cmd, in sim.Input) (bool, sim.Vec3) {
	if p.ride == 0 {
		return false, sim.Vec3{}
	}
	b := s.boatByID(p.ride)
	if b == nil || p.Dead {
		p.ride, p.seat = 0, 0
		return false, sim.Vec3{}
	}
	if p.seat != SeatDeck && RideLeaves(c.B, p.lastCmd.B, p.body.Boots) {
		s.leaveBoat(p, c)
		if p.ride == 0 {
			return false, sim.Vec3{}
		}
	}
	if p.seat == SeatDeck {
		if DeckLeaves(c.B, p.lastCmd.B, p.body.Boots) {
			s.leaveBoat(p, c)
			return false, sim.Vec3{}
		}
		st := b.start // (a copy: the driver's command may come first this tick)
		from := sim.DeckWorld(&st, p.deck.Pos)
		w.StepDeck(&st, p.body, &p.deck, in, 1.0/SimHz)
		to := sim.DeckWorld(&st, p.deck.Pos)
		if !sim.OnHull(p.deck.Pos[0], p.deck.Pos[2]) { // walked off: on foot from here
			p.ride, p.seat, p.deck = 0, 0, sim.Deck{}
		}
		return true, sim.Vec3{to[0] - from[0], to[1] - from[1], to[2] - from[2]}
	}
	RideStep(w, &b.sim, p.body, c.F, c.R, p.seat, 1.0/SimHz)
	if p.seat == 0 {
		b.driven = true
	}
	b.sync()
	return true, sim.Vec3{}
}
