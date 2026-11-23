package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeMonorepo is a tight three-lane map: long north ("main") and south ("dev")
// corridors to the sites, and a mid lane into the "merge conflict" junction,
// which has doors to both corridors and to the defenders' side.
func DeMonorepo() c.Map {
	b := mapgen.New("de_monorepo")
	b.Add(mapgen.Arena(-36, -24, 36, 24, 6, 1, c.Concrete, c.Brick)...)

	// The mass between the lanes (brick blocks, 6 m: no going over).
	const h = 6
	b.Add(
		p.Wall(-22, 20, 10, 24, h, c.Brick),   // north outer: main corridor is z 15..20
		p.Wall(-22, 8, 2, 15, h, c.Brick),     // north inner, west of the junction's north door
		p.Wall(6, 8, 14, 15, h, c.Brick),      // north inner, east of it
		p.Wall(-22, -24, 10, -20, h, c.Brick), // south outer: dev corridor is z -20..-15
		p.Wall(-22, -15, 2, -8, h, c.Brick),
		p.Wall(6, -15, 14, -8, h, c.Brick),
		p.Wall(-22, 2.5, -4, 8, h, c.Brick), // mid lane z -2.5..2.5
		p.Wall(-22, -8, -4, -2.5, h, c.Brick),
		p.Wall(14, -8, 15, -3, h, c.Brick), // junction east door z -3..3
		p.Wall(14, 3, 15, 8, h, c.Brick),
		p.Wall(14, 8, 24, 12, h, c.Brick), // separate the sites from defender mid
		p.Wall(14, -12, 24, -8, h, c.Brick),
	)

	// The junction ("merge conflict"): crates and pillars to fight around.
	b.Add(p.CratePyramid(2.5, -0.75, 1.5)...)
	b.Add(
		p.Crate(-3, 0, 5, 2, 2, 2),
		p.Crate(10, 0, -6, 2, 1.2, 2),
		p.Pillar(9, 4, 1, h, c.Hazard),
		p.Pillar(-1, -6, 1, h, c.Hazard),
	)
	// A little cover in the long corridors.
	b.Add(p.Crate(-10, 0, 15, 1.5, 1.5, 1.5), p.Crate(-4, 0, -16.5, 1.5, 1.5, 1.5))
	// A crate at the mid lane's mouth: the defenders can't see into the spawn from theirs.
	b.Add(p.Crate(-24, 0, -0.75, 1.5, 2, 1.5))

	// A site: a release platform with two steps, crates.
	b.Add(c.B(28, 0, 18, 36, 1, 24, c.Hazard))
	b.Add(p.Flight(false, 28, 18, 24, 0, 0.8, 0.5, 2, -1, c.Hazard)...)
	b.Add(p.Crate(20, 0, 17, 2, 2, 2), p.Crate(24, 0, 21, 1.5, 1.5, 1.5), p.Crate(31, 1, 20, 2, 2, 2))
	// B site: open floor with a crate stack and a pillar.
	b.Add(p.CratePyramid(26, -20, 2)...)
	b.Add(p.Crate(20, 0, -17, 2, 2, 2), p.Pillar(31, -15, 1.5, h, c.Hazard))

	b.Item("codereview", 8, 0, 5)

	b.TeamSpawns(c.Vibe, [3]float64{-32, 0, -6}, [3]float64{-32, 0, 0}, [3]float64{-32, 0, 6},
		[3]float64{-29, 0, -3}, [3]float64{-29, 0, 3})
	b.TeamSpawns(c.Artisan, [3]float64{32, 0, -4}, [3]float64{32, 0, 0}, [3]float64{32, 0, 4},
		[3]float64{29, 0, -2}, [3]float64{29, 0, 2})
	b.BuyAroundSpawns(3)
	b.Site("A", 15, -0.5, 12, 36, 4, 24)
	b.Site("B", 15, -0.5, -24, 36, 3, -12)

	b.Nav("vibe", -30, 0, 0, "vN", "vM", "vS").
		Nav("vN", -26, 0, 17.5, "nE").
		Nav("nE", 12, 0, 17.5, "A").
		Nav("A", 20, 0, 15).
		Nav("vM", -26, 0, 1.7, "mid").
		Nav("mid", -6, 0, 0, "junc", "juncS").
		Nav("junc", 0, 0, 4, "jN", "jE").
		Nav("jN", 4, 0, 11.5, "nE").
		Nav("juncS", 0, 0, -4, "jS", "jE").
		Nav("jS", 4, 0, -11.5, "sE").
		Nav("jE", 12, 0, 0, "ct").
		Nav("vS", -26, 0, -17.5, "sE").
		Nav("sE", 12, 0, -17.5, "B").
		Nav("B", 20, 0, -14).
		Nav("ct", 20, 0, 0, "artisan", "ctA", "ctB").
		Nav("artisan", 30, 0, 0).
		Nav("ctA", 25, 0, 10, "A").
		Nav("ctB", 25, 0, -10, "B")
	b.Meta("Tight and long: main and dev corridors, and a merge-conflict junction in mid.", 8)
	b.FillSpawns(true) // one spawn per player, in the same spawn area
	b.Category(c.Training)
	return b.Map()
}
