package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeSlop is a sandy courtyard with a mid building, crates to peek from and a
// raised "A site" reached by steps.
func DeSlop() c.Map {
	b := mapgen.New("de_slop")
	b.Add(mapgen.Enclosure(32, 7, 1, c.Sand, c.Brick)...)

	b.Add(p.Building(0, 0, 12, 8, 5, 3, 3, c.Brick)...) // mid, doorway along z

	// long corridor walls
	b.Add(p.Wall(-20, 10, -18, 30, 5, c.Brick), p.Wall(18, -30, 20, -10, 5, c.Brick))

	// A site: raised platform with steps up from the west
	b.Add(c.B(16, 0, 14, 31, 1.5, 31, c.Hazard))
	b.Add(p.StairsDownX(16, 18, 26, 1, 0.5, 2, c.Hazard)...)

	// crates
	b.Add(p.CratePyramid(-12, -14, 2)...)
	b.Add(
		p.Crate(8, 0, 10, 2, 2, 2),
		p.Crate(22, 1.5, 22, 2, 2, 2), // on A site
		p.Crate(-26, 0, -26, 3, 3, 3),
		p.Crate(-4, 0, 16, 2, 1.5, 2),
		p.Crate(4, 0, -18, 3, 2, 3),
	)

	b.Add(p.Pillar(-26, 20, 2, 10, c.Hazard)) // silly pillar

	b.Spawns(
		[3]float64{-26, 0, -10}, [3]float64{-26, 0, 0}, [3]float64{-22, 0, 10},
		[3]float64{26, 0, -20}, [3]float64{26, 0, 0}, [3]float64{10, 0, 26},
	)
	return b.Map()
}
