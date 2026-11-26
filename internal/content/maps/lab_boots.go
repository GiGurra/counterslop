package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// LabBoots is the Gravity Override lab (per-player gravity, internal/sim
// gravity.go; sim/cases grav-*): a closed 24 m cube (floor, four walls and a
// ceiling, all standable), a pillar floor to ceiling in the middle, a crate on
// the floor, a ledge on the +x wall and a block hung from the ceiling, so
// every gravity has something to walk into, step onto and stand under.
func LabBoots() c.Map {
	return mapgen.New("lab_boots").
		Add(mapgen.Arena(-12, -12, 12, 12, 12, 1, c.Concrete, c.Hazard)...).
		Add(c.B(-13, 12, -13, 13, 13, 13, c.Metal)). // ceiling
		Add(c.B(-1, 0, -1, 1, 12, 1, c.Concrete)).   // pillar
		Add(prefabs.Crate(6, 0, 6, 1, 1, 1)).        // crate on the floor
		Add(c.B(10, 5, -3, 12, 6, 3, c.Metal)).      // ledge on the +x wall
		Add(c.B(-8, 10.8, -8, -5, 12, -5, c.Metal)). // block under the ceiling
		TeamSpawns(c.Vibe, [3]float64{-8, 0, 4}, [3]float64{-8, 0, 6}).
		TeamSpawns(c.Artisan, [3]float64{8, 0, -4}, [3]float64{8, 0, -6}).
		BuyAroundSpawns(2).
		Rule("mp_gravityboots", "1"). // the Gravity Override is for sale here (B, gear)
		Meta("Gravity lab: every wall is a floor if your boots say so. Buy the Gravity Override (gear), hold F.", 4).
		Category(c.Training).
		Map()
}
