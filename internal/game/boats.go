package game

import (
	"encoding/json"

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
