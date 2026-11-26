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
	// The riders sit where their boat is now (a passenger's command may have
	// run before the driver's this tick).
	for _, p := range s.players {
		if b := s.boatByID(p.ride); p.ride != 0 && b != nil && p.body != nil {
			p.body.Pos, p.body.Vel = SeatWorld(&b.sim, p.seat), b.sim.Vel
			p.Pos = Vec3(p.body.Pos)
		}
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
	return &boatExact{Pos: b.sim.Pos, Vel: b.sim.Vel, Yaw: b.sim.Yaw, YawVel: b.sim.YawVel}
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

// boardBoat seats p in the nearest boat within BoardReach (feet, x and z;
// within 2 m up or down) with a free seat: the driver's first. Callers hold
// s.mu.
func (s *Server) boardBoat(p *Player) bool {
	if p.ride != 0 || p.body == nil || p.Dead {
		return false
	}
	var best *Boat
	bestD := BoardReach
	for _, b := range s.boats {
		d := math.Hypot(b.sim.Pos[0]-p.body.Pos[0], b.sim.Pos[2]-p.body.Pos[2])
		if d < bestD && math.Abs(b.sim.Pos[1]-p.body.Pos[1]) < 2 {
			for seat := range BoatSeats {
				if !s.seatTaken(b.ID, seat) {
					best, bestD = b, d
					break
				}
			}
		}
	}
	if best == nil {
		return false
	}
	for seat := range BoatSeats {
		if !s.seatTaken(best.ID, seat) {
			p.ride, p.seat = best.ID, seat
			break
		}
	}
	bd := p.body
	bd.Pos, bd.Vel, bd.OnGround, bd.Crouched, bd.H = SeatWorld(&best.sim, p.seat), best.sim.Vel, true, true, sim.HCrouch
	p.Pos, p.Crouch = Vec3(p.body.Pos), true
	return true
}

// leaveBoat puts p out of its boat beside its seat, moving with the boat;
// hop: jumping out (up at jump speed). Callers hold s.mu.
func (s *Server) leaveBoat(p *Player, hop bool) {
	b := s.boatByID(p.ride)
	p.ride, p.seat, p.leftBoat = 0, 0, p.cmdTicks
	if b == nil || p.body == nil {
		return
	}
	RideLeave(&b.sim, p.body, hop) // as the client predicts it
}

// rideCmd runs command c of p if it rides a boat: E or a jump press gets out
// (then it moves on foot this command), else RideStep (the driver steers).
// Reports whether p rode. Callers hold s.mu.
func (s *Server) rideCmd(w *sim.World, p *Player, c Cmd) bool {
	if p.ride == 0 {
		return false
	}
	b := s.boatByID(p.ride)
	if b == nil || p.Dead {
		p.ride, p.seat = 0, 0
		return false
	}
	if RideLeaves(c.B, p.lastCmd.B) {
		s.leaveBoat(p, (c.B&^p.lastCmd.B)&BtnJump != 0)
		return false
	}
	RideStep(w, &b.sim, p.body, c.F, c.R, p.seat, 1.0/SimHz)
	if p.seat == 0 {
		b.driven = true
	}
	b.sync()
	return true
}
