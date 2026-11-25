package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// LabHarbor is the water test yard (internal/sim water.go, docs/physics.md):
// a quay at y 0 along the north (+z) with the spawns, a 3.4 m deep basin
// (water surface y -0.6, its floor y -4) with a pier into it, a slipway up
// out of it in the east, and a ship's hull to the south: a 16 m wall rising
// out of the water for boats to come to rest against.
func LabHarbor() c.Map {
	const (
		surf  = -0.6 // the water's surface: the quay's edge stands 0.6 m over it
		floor = -4.0 // the basin's floor
		hullZ = -30.0
	)
	b := mapgen.New("lab_harbor").
		Add(c.B(-31, floor-1, hullZ, 31, floor, 10, c.Concrete), // the basin's floor
			c.B(-31, floor, 10, 31, 0, 31, c.Concrete),                                             // the quay
			c.B(-31, floor, hullZ, -30, 4, 31, c.Brick), c.B(30, floor, hullZ, 31, 4, 31, c.Brick), // the sides
			c.B(-31, 0, 30, 31, 4, 31, c.Brick),                               // the quay's back wall
			c.B(-31, floor-1, hullZ-1, 31, 16, hullZ, c.Container),            // the ship's hull
			c.B(-2, floor, -12, 2, 0, 10, c.Metal),                            // the pier
			prefabs.Ramp(20, 2, 28, 10, floor, -floor, c.RampPZ, c.Concrete)). // the slipway (26.6 degrees)
		Water(-30, floor, hullZ, 30, surf, 10)
	return b.
		TeamSpawns(c.Vibe, [3]float64{-22, 0, 22}, [3]float64{-18, 0, 22}).
		TeamSpawns(c.Artisan, [3]float64{18, 0, 22}, [3]float64{22, 0, 22}).
		BuyAroundSpawns(2).
		Meta("Harbor lab: a quay, a 3.4 m deep basin with a pier and a slipway, and a ship's hull to moor against.", 4).
		Category(c.Training).
		Map()
}
