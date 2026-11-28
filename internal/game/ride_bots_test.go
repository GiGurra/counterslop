package game

import (
	"math"
	"testing"
)

// Bots riding a ship stand still in its cabin: from a second after the doors
// shut until the floor opens, each one's place in the cabin (its frame: what
// clients draw, Player.Local) stays put but for looking round. Before, a
// riding bot kept the waypoint it had when its goal last changed, a point in
// the world, and walked back towards it at run speed against the ship's
// travel (up to 6 m along the cabin, jumping at waypoints above it, shoving
// the passengers it walked into): the bots wobbled along whichever way the
// ship moved, sideways in the cross, up and down in the climb.
func TestBotsRideStill(t *testing.T) {
	for _, mapName := range []string{"lab_moon", "cq_moonshot"} {
		t.Run(mapName, func(t *testing.T) {
			s := authServer(t)
			if err := s.ResetWorldOn(mapName); err != nil {
				t.Fatal(err)
			}
			dep := s.Map.Departures[0]
			m := &s.Map.Movers[dep.Mover-1]
			var bots []*Player
			for range 4 {
				bots = append(bots, s.addAI("vibe"))
			}
			for math.Mod(s.moverClock(), m.Period) < dep.Close-0.5 { // boarding: aboard as the doors shut
				s.Step(1)
			}
			for i, q := range bots {
				s.teleport(q, Vec3{dep.Board[0] + 3 - 2*float64(i), dep.Board[1], dep.Board[2] + 1}, 0, false, q.ai.seq)
			}
			type span struct{ lo, hi Vec3 }
			spans := map[int]*span{}
			for range int((m.Hide[0] + 1) * SimHz) {
				s.Step(1)
				ph := math.Mod(s.moverClock(), m.Period)
				if ph < dep.Close+1 || ph > m.Hide[0]-0.1 {
					continue
				}
				for _, q := range bots {
					if q.Frame != dep.Mover || q.Local == nil {
						t.Fatalf("%s left the cabin at %.2f s into the cycle (at %v)", q.Name, ph, q.Pos)
					}
					l := *q.Local
					sp := spans[q.ID]
					if sp == nil {
						sp = &span{l, l}
						spans[q.ID] = sp
					}
					for a := range 3 {
						sp.lo[a], sp.hi[a] = math.Min(sp.lo[a], l[a]), math.Max(sp.hi[a], l[a])
					}
				}
			}
			for _, q := range bots {
				sp := spans[q.ID]
				if sp == nil {
					t.Fatalf("%s never rode", q.Name)
				}
				if d := max(sp.hi[0]-sp.lo[0], sp.hi[1]-sp.lo[1], sp.hi[2]-sp.lo[2]); d > 0.05 {
					t.Errorf("%s moved %.2f m in the cabin while riding (x %.2f, y %.2f, z %.2f)", q.Name, d, sp.hi[0]-sp.lo[0], sp.hi[1]-sp.lo[1], sp.hi[2]-sp.lo[2])
				}
			}
		})
	}
}
