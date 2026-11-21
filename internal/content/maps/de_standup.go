package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeStandup is one big open-plan office: desk pods for cover, a standup ring
// of whiteboards in the middle, A in the meeting room and B in the kitchen.
func DeStandup() c.Map {
	b := mapgen.New("de_standup")
	b.Add(mapgen.Arena(-34, -34, 34, 34, 5, 1, c.Carpet, c.Plaster)...)

	// A: the meeting room (north-east), doors west, south and north.
	const dh = 2.4
	b.Add(p.Room(14, 8, 34, 24, 0, 3.5, 0.4, c.Plaster, p.Doors{
		W: []p.Gap{{From: 14, To: 17, H: dh}},
		S: []p.Gap{{From: 20, To: 23, H: dh}},
		N: []p.Gap{{From: 24, To: 27, H: dh}},
	})...)
	b.Add(c.B(19, 0, 14, 27, 0.75, 18, c.Desk)) // the conference table
	b.Add(p.Whiteboard(31, 12, 31.2, 20))

	// B: the kitchen (south-west), doors east and north.
	b.Add(p.Room(-34, -22, -14, -6, 0, 3.5, 0.4, c.Plaster, p.Doors{
		E: []p.Gap{{From: -16, To: -13, H: dh}},
		N: []p.Gap{{From: -26, To: -23, H: dh}},
	})...)
	b.Add(
		c.B(-33.6, 0, -7.4, -27, 1, -6.4, c.Desk),      // counter along the north wall
		c.B(-24, 0, -16, -20, 1, -13, c.Desk),          // the island
		c.B(-33.6, 0, -21.6, -32, 2.2, -19.6, c.Metal), // fridge
		c.B(-33.6, 0, -19.4, -32.6, 1, -14, c.Desk),    // counter along the west wall
	)

	// The standup ring: four whiteboards around the middle.
	b.Add(
		p.Whiteboard(-2, 3, 2, 3.1), p.Whiteboard(-2, -3.1, 2, -3),
		p.Whiteboard(3, -2, 3.1, 2), p.Whiteboard(-3.1, -2, -3, 2),
	)

	// Desk pods all over the floor, and structural columns.
	for _, d := range [][2]float64{{-22, 8}, {-22, 20}, {-10, 22}, {10, -10}, {22, -6}, {22, -18}, {10, -22}, {-8, -12}, {8, 12}, {0, 14}, {0, -16}, {-22, -28}, {24, 6}, {-26, 30}, {26, 30}} {
		b.Add(p.DeskPod(d[0], 0, d[1])...)
	}
	for _, x := range []float64{-12.5, 11.5} {
		for _, z := range []float64{-1, 13, -25} {
			b.Add(p.Pillar(x, z, 1, 5, c.Concrete))
		}
	}

	// Bookshelves (1.8 m: cover you can't see over, but jump onto from a desk).
	b.Add(
		p.Wall(-14, 6, -6, 6.4, 1.8, c.Desk),
		p.Wall(16, -16, 16.4, -8, 1.8, c.Desk),
		p.Wall(-20, -2, -19.6, 4, 1.8, c.Desk),
		p.Wall(8, 20, 14, 20.4, 1.8, c.Desk),
		p.Wall(-4, -24, 4, -23.6, 1.8, c.Desk),
	)

	// Phone booths: full-height cover in the open.
	for _, pb := range [][2]float64{{-15, -7}, {-27, 11}, {13, -27}, {25, -13}, {-6, 16}} {
		b.Add(c.B(pb[0], 0, pb[1], pb[0]+2, 2.4, pb[1]+2, c.Metal))
	}

	b.Item("codereview", -4, 0, 9)

	b.TeamSpawns(c.Vibe, [3]float64{-6, 0, -30}, [3]float64{-2, 0, -30}, [3]float64{2, 0, -30},
		[3]float64{6, 0, -30}, [3]float64{0, 0, -27})
	b.TeamSpawns(c.Artisan, [3]float64{-6, 0, 30}, [3]float64{-2, 0, 30}, [3]float64{2, 0, 30},
		[3]float64{6, 0, 30}, [3]float64{0, 0, 27})
	b.BuyAroundSpawns(3)
	b.Site("A", 14.4, -0.5, 8.4, 33.6, 3, 23.6)
	b.Site("B", -33.6, -0.5, -21.6, -14.4, 3, -6.4)

	b.Nav("vibe", 0, 0, -29, "b1", "a1").
		Nav("b1", -6, 0, -20, "bE").
		Nav("bE", -11, 0, -14.5, "B").
		Nav("B", -16, 0, -14.5).
		Nav("a1", 6, 0, -6, "aS").
		Nav("aS", 21.5, 0, 4, "A").
		Nav("A", 21.5, 0, 10).
		Nav("artisan", 0, 0, 29, "aN", "bN").
		Nav("aN", 25.5, 0, 27, "A2").
		Nav("A2", 25.5, 0, 20).
		Nav("bN", -24.5, 0, 0, "B2").
		Nav("B2", -24.5, 0, -10)
	b.Meta("Open-plan office: desk pods as cover, A in the meeting room, B in the kitchen.", 12)
	return b.Map()
}
