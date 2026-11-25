package game

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/sim"
)

func harborServer(t *testing.T) *Server {
	t.Helper()
	s := authServer(t)
	s.Test = true
	if err := s.ResetWorldOn("lab_harbor"); err != nil {
		t.Fatal(err)
	}
	return s
}

// The map's boats start at their moorings, a pushed boat drifts and comes
// to rest against the ship's hull, and a new round puts it back.
func TestBoatsDriftRestAndReset(t *testing.T) {
	s := harborServer(t)
	if len(s.boats) != 2 || s.boats[0].Pos != (Vec3{-12, -0.6, 4}) {
		t.Fatalf("boats %+v", s.boats)
	}
	s.mu.Lock()
	s.boats[0].sim.Pos, s.boats[0].sim.Vel = sim.Vec3{-12, -0.6, -24}, sim.Vec3{0, 0, -9}
	s.mu.Unlock()
	s.Step(300)
	b := s.boats[0]
	if want := -30 + sim.BoatDisc + sim.BoatDiscOff; math.Abs(b.sim.Pos[2]-want) > 1e-6 || b.sim.Vel[2] != 0 {
		t.Fatalf("against the hull: %+v, want z %v", b.sim, want)
	}
	if b.Pos != Vec3(b.sim.Pos) {
		t.Fatalf("sent %v, simulated %v", b.Pos, b.sim.Pos)
	}
	s.mu.Lock()
	s.startRound(s.clock())
	s.mu.Unlock()
	if s.boats[0].Pos != (Vec3{-12, -0.6, 4}) || s.boats[0].Vel != (Vec3{}) {
		t.Fatalf("after the round start: %+v", s.boats[0])
	}
}

// Every full snapshot carries the boats (id, position, heading, velocity);
// a map without boats sends none.
func TestSnapshotCarriesBoats(t *testing.T) {
	s := harborServer(t)
	var m struct {
		Boats []struct {
			ID int
			P  [3]float64
			Y  float64
			V  [3]float64
		}
	}
	s.mu.Lock()
	b := s.boatsJSON()
	s.mu.Unlock()
	if err := json.Unmarshal(append(append([]byte("{"), b...), '}'), &m); err != nil || len(m.Boats) != 2 || m.Boats[1].ID != 2 || m.Boats[1].P != [3]float64{12, -0.6, 4} {
		t.Fatalf("%s: %+v %v", b, m, err)
	}
	if err := s.ResetWorldOn("de_slop"); err != nil {
		t.Fatal(err)
	}
	if s.boatsJSON() != nil {
		t.Fatal("boats on de_slop")
	}
}

// The MCP debugger's boat_set moves a boat and lists them all.
func TestMCPBoatSet(t *testing.T) {
	s := harborServer(t)
	c := mcpFor(t, s, true)
	r := c.call("boat_set", map[string]any{"id": 2, "pos": []float64{5, -0.6, -10}, "yaw": 1, "vel": []float64{1, 0, 2}})
	bs := r["boats"].([]any)
	if len(bs) != 2 || bs[1].(map[string]any)["yaw"].(float64) != 1 || s.boats[1].Pos != (Vec3{5, -0.6, -10}) {
		t.Fatalf("boat_set %v", r)
	}
	c.call("boat_set", map[string]any{"id": 3}, true)
	c.call("boat_set", map[string]any{"id": 1, "pos": []float64{1, 2}}, true)
}
