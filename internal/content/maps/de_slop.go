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
	// Mid's west screens: the courtyard was open from spawn to spawn (every
	// round a duel across it 2 s in, nobody near a site); now the way east
	// goes round their ends, north to A's stairs or south to B.
	b.Add(p.Wall(-6, 4, -5, 15, 5, c.Brick), p.Wall(-6, -11, -5, -4, 5, c.Brick))

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
	b.Item("codereview", -14, 0, 8)           // the sniper is a pickup until the buy menu exists

	// Bomb sites for The Big Diff: A on the raised platform (and its steps),
	// B in the north-east around the crate by the corridor wall.
	b.Site("A", 13, 0, 14, 31, 5, 31)
	b.Site("B", 0, 0, -31, 17, 4, -20)

	// Vibe Coders push in from the west; the Artisans hold the east side near A.
	// Each team spawns in one yard behind cover: the Artisans east of a brick
	// screen, the Vibe Coders in the west yard out of their sight (TestSpawnsCovered).
	b.Add(p.Wall(22, -10, 23, 2, 3, c.Brick))
	b.TeamSpawns(c.Vibe, [3]float64{-26, 0, -10}, [3]float64{-26, 0, 0}, [3]float64{-26, 0, 10}, [3]float64{-26, 0, -5}, [3]float64{-26, 0, 5})
	b.TeamSpawns(c.Artisan, [3]float64{27, 0, -4}, [3]float64{27, 0, -8}, [3]float64{27, 0, 0}, [3]float64{30, 0, -6}, [3]float64{30, 0, -2})

	b.BuyZone(c.Vibe, -31, -0.5, -14, -18, 3, 14)
	b.BuyZone(c.Artisan, 20, -0.5, -24, 31, 3, 4)
	b.Nav("vibe", -26, 0, 0, "midN", "midS").
		Nav("midN", -12, 0, 8, "northW").
		Nav("northW", -8, 0, 20, "north").
		Nav("north", 4, 0, 20, "stairs").
		Nav("stairs", 12, 0, 22, "A").
		Nav("A", 18, 1.5, 20).
		Nav("midS", -12, 0, -6, "southW").
		Nav("southW", -7, 0, -13, "south").
		Nav("south", -2, 0, -20, "B").
		Nav("B", 12, 0, -22).
		Nav("artisan", 24, 0, 5, "ctN", "ctS").
		Nav("ctN", 12, 0, 10, "stairs").
		Nav("ctS", 14, 0, -8, "B")
	b.Meta("The original sandbox: a sandy courtyard, a mid building, A up on the platform.", 8)
	b.FillSpawns(true) // one spawn per player, in the same spawn area
	b.Category(c.Training)
	return b.Map()
}
