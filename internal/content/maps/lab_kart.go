package maps

import (
	"math"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// LabKart is the car test track (internal/sim car.go, docs/racing.md): a
// loop round a low island, driven with the island on your left (+x along
// the north straight, -x along the south one). On the north straight a ramp ends in a
// 2.2 m drop (a jump); the east end's outside is a bank rising 4 m to the
// wall (a banked turn); the south straight dips through a 1.6 m deep water
// basin on gentle slopes (the hovercraft floats over it, the rest wade).
// One car of each type stands on the grid at the west end of the north
// straight, the spawns between them.
func LabKart() c.Map {
	const (
		bx0, bx1 = -14.0, 14.0 // the basin, across the south straight
		bz0, bz1 = -55.0, -19.0
		bed      = -2.0 // its bed
		surf     = -0.4 // the water's surface
	)
	b := mapgen.New("lab_kart").
		Add(c.B(-90, -3, -55, bx0, 0, 55, c.Concrete), c.B(bx1, -3, -55, 90, 0, 55, c.Concrete), // the floor round the basin
			c.B(bx0, -3, bz1, bx1, 0, 55, c.Concrete),
			c.B(bx0, bed-1, bz0, bx1, bed, bz1, c.Concrete),                            // the basin's bed
			c.B(-91, 0, -56, 91, 6, -55, c.Brick), c.B(-91, 0, 55, 91, 6, 56, c.Brick), // the walls
			c.B(-91, 0, -55, -90, 6, 55, c.Brick), c.B(90, 0, -55, 91, 6, 55, c.Brick),
			c.B(-50, 0, -18, 50, 1.2, 18, c.Concrete),                           // the island
			prefabs.Ramp(bx0, bz0, bx0+8, bz1, bed, -bed, c.RampNX, c.Concrete), // into the basin
			prefabs.Ramp(bx1-8, bz0, bx1, bz1, bed, -bed, c.RampPX, c.Concrete), // and out
			prefabs.Ramp(-24, 28, -10, 48, 0, 2.2, c.RampPX, c.Metal),           // the jump
			prefabs.Ramp(72, -55, 90, 55, 0, 4, c.RampPX, c.Concrete)).          // the banked turn
		Water(bx0, bed, bz0, bx1, surf, bz1)
	east := -math.Pi / 2 // facing +x
	for i, t := range []string{"buggy", "racecar", "tank", "hovercraft"} {
		b.Car(t, -60, 0, 24+float64(i)*7, east)
	}
	return b.
		TeamSpawns(c.Vibe, [3]float64{-64, 0, 27.5}, [3]float64{-64, 0, 34.5}).
		TeamSpawns(c.Artisan, [3]float64{-64, 0, 41.5}, [3]float64{-64, 0, 48.5}).
		BuyAroundSpawns(2).
		Meta("Kart lab: a loop with a jump, a banked turn and a water basin, and one car of each type on the grid (E to drive).", 4).
		Category(c.Training).
		Map()
}
