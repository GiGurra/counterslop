package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeStack is vertical: a ground floor, a metal deck over the east half at 4 m
// and a roof deck at 8 m. B is in the basement under the deck, A up on the
// roof; "the stack" is a stairwell through the deck between them.
func DeStack() c.Map {
	b := mapgen.New("de_stack")
	b.Add(mapgen.Arena(-30, -30, 30, 30, 13, 1, c.Concrete, c.Brick)...)

	const l1, l2, slab = 4.0, 8.0, 0.4
	// Deck 1 (east half), with the stairwell hole x 12..16, z -12..-4.
	b.Add(
		p.Slab(0, -30, 12, 30, l1, slab, c.Metal),
		p.Slab(16, -30, 30, 30, l1, slab, c.Metal),
		p.Slab(12, -30, 16, -12, l1, slab, c.Metal),
		p.Slab(12, -4, 16, 30, l1, slab, c.Metal),
	)
	for _, x := range []float64{6, 18} {
		for _, z := range []float64{-24, -12, 0, 12, 24} {
			b.Add(c.B(x, 0, z, x+1, l1-slab, z+1, c.Concrete))
		}
	}
	// The stack: stairs down the hole into the basement.
	b.Add(p.Flight(true, -4, 12, 16, 0, 0.8, 0.5, 8, -1, c.Metal)...)
	// Ground -> deck 1: two flights up to the deck's west edge.
	b.Add(p.Flight(false, 0, -4, 0, 0, 0.8, 0.5, 8, -1, c.Metal)...)
	b.Add(p.Flight(false, 0, 20, 24, 0, 0.8, 0.5, 8, -1, c.Metal)...)
	// Railing along deck 1's west edge, open at the stairs.
	b.Add(
		p.Railing(0, -30, 0.3, -4, l1, c.Hazard),
		p.Railing(0, 0, 0.3, 20, l1, c.Hazard),
		p.Railing(0, 24, 0.3, 30, l1, c.Hazard),
	)

	// Deck 2 (the roof, north-east) on columns standing on deck 1.
	b.Add(p.Slab(10, 4, 30, 30, l2, slab, c.Metal))
	for _, cz := range [][2]float64{{12, 12}, {12, 28}, {28, 6}, {20, 28}} {
		b.Add(c.B(cz[0], l1, cz[1], cz[0]+1, l2-slab, cz[1]+1, c.Concrete))
	}
	b.Add(p.Flight(false, 10, 6, 9, l1, 0.8, 0.5, 8, -1, c.Metal)...) // from the west
	b.Add(p.Flight(true, 4, 24, 28, l1, 0.8, 0.5, 8, -1, c.Metal)...) // from the south
	b.Add(
		p.Railing(10, 4, 24, 4.3, l2, c.Hazard),
		p.Railing(28, 4, 30, 4.3, l2, c.Hazard),
		p.Railing(10, 9, 10.3, 30, l2, c.Hazard),
	)

	// A on the roof: crates and an air-con unit.
	b.Add(p.Crate(18, l2, 20, 2, 2, 2), p.Crate(24, l2, 24, 1.5, 1.5, 1.5))
	b.Add(c.B(25, l2, 14, 28, l2+1.6, 16, c.Metal))
	// B in the basement: server racks.
	for _, z := range []float64{-25, -21} {
		b.Add(c.B(23, 0, z, 27, 2.2, z+0.8, c.Metal))
	}
	b.Add(p.Crate(15, 0, -22, 2, 1.5, 2))

	// Ground floor west: racks and crates.
	b.Add(
		c.B(-16, 0, -20, -15.2, 2.2, -10, c.Metal),
		c.B(-16, 0, 10, -15.2, 2.2, 20, c.Metal),
		p.Crate(-13, 0, 2.5, 2, 2, 2),
		p.Crate(-20, 0, 22, 3, 2, 3),
		p.Crate(-20, 0, -26, 3, 2, 3),
	)
	b.Add(p.CratePyramid(-8, 12, 1.5)...)
	// Deck 1 cover.
	b.Add(p.Crate(4, l1, -20, 2, 1.5, 2), p.Crate(22, l1, -22, 2, 2, 2), p.Crate(4, l1, 14, 1.5, 1.5, 1.5))

	b.Item("codereview", 20, l2, 10)

	b.TeamSpawns(c.Vibe, [3]float64{-26, 0, -6}, [3]float64{-26, 0, 0}, [3]float64{-26, 0, 6},
		[3]float64{-23, 0, -3}, [3]float64{-23, 0, 3})
	b.TeamSpawns(c.Artisan, [3]float64{22, l1, -8}, [3]float64{26, l1, -8}, [3]float64{22, l1, -12},
		[3]float64{26, l1, -12}, [3]float64{24, l1, -10})
	b.BuyAroundSpawns(3)
	b.Site("A", 14, l2-0.5, 12, 30, l2+3, 30)
	b.Site("B", 14, -0.5, -27, 30, 3, -12)

	b.Nav("vibe", -26, 0, 0, "g1", "g2").
		Nav("g1", -8, 0, 8, "g3").
		Nav("g3", 3, 0, 6, "g4").
		Nav("g4", 3, 0, -16, "B").
		Nav("B", 20, 0, -17).
		Nav("g2", -8, 0, -2, "stairsW").
		Nav("stairsW", 1, l1, -2, "d1").
		Nav("d1", 3, l1, 7.5, "roofW").
		Nav("roofW", 11, l2, 7.5, "A").
		Nav("A", 20, l2, 18).
		Nav("artisan", 24, l1, -10, "d1S", "stackTop").
		Nav("d1S", 26, l1, -4, "roofS").
		Nav("roofS", 26, l2, 6, "A").
		Nav("stackTop", 14, l1, -1, "stackBot").
		Nav("stackBot", 14, 0, -12.5, "B")
	b.Meta("Vertical: B in the basement, A on the roof, the stack's stairwell between them.", 10)
	return b.Map()
}
