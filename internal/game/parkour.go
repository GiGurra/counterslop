package game

import (
	"math"
	"time"

	"github.com/gigurra/counterslop/internal/sim"
)

// Parkour (a race on foot: Map.OnFoot, docs/racing.md "Parkour"): the race
// rules of race.go without the grid's cars (racers start at the team spawns,
// a fall or a death puts them back on their last gate, raceSpawn), point to
// point or laps, splits against each racer's best run (raceSplit). The
// map's cars and boats are free vehicles at stations for the course's legs:
// one left alone away from its station goes back to it (tickStations), so
// the next racer finds it there.

const (
	stationAway = 4.0 // m from its station: left out on the course
	stationWait = 6.0 // s left alone out there before it goes back
)

// tickStations sends a car or a boat nobody is in, left out on the course
// for stationWait, back to its station (a car repaired). Callers hold s.mu.
func (s *Server) tickStations(now time.Time) {
	for _, c := range s.cars {
		home := c.spawn >= 0 && c.spawn < len(s.Map.Cars)
		if !home || c.Driver != 0 || dist3(c.sim.Pos, sim.Vec3(s.Map.Cars[c.spawn].Pos)) < stationAway {
			c.cs.idleSince = time.Time{}
			continue
		}
		if c.cs.idleSince.IsZero() {
			c.cs.idleSince = now
		}
		if now.Sub(c.cs.idleSince).Seconds() >= stationWait {
			s.carHome(c)
			c.sim.HP = c.MaxHP
			c.cs.idleSince = time.Time{}
			c.sync()
		}
	}
	for _, b := range s.boats {
		if b.spawn < 0 || b.spawn >= len(s.Map.Boats) || s.boatRidden(b) || dist3(b.sim.Pos, sim.Vec3(s.Map.Boats[b.spawn].Pos)) < stationAway {
			b.idleSince = time.Time{}
			continue
		}
		if b.idleSince.IsZero() {
			b.idleSince = now
		}
		if now.Sub(b.idleSince).Seconds() >= stationWait {
			sp := s.Map.Boats[b.spawn]
			b.sim = sim.Boat{Pos: sim.Vec3(sp.Pos), Yaw: sp.Yaw}
			b.start = b.sim
			b.idleSince = time.Time{}
			b.sync()
		}
	}
}

// boatRidden: someone sits in (or stands on) boat b.
func (s *Server) boatRidden(b *Boat) bool {
	for _, p := range s.players {
		if p.ride == b.ID && !p.Dead {
			return true
		}
	}
	return false
}

func dist3(a, b sim.Vec3) float64 {
	return math.Hypot(math.Hypot(a[0]-b[0], a[1]-b[1]), a[2]-b[2])
}
