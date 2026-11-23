package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeStack is vertical: an underground garage at -4 m, a ground floor, a
// metal deck over the east half at 4 m and a roof deck at 8 m. The Vibe Coders
// start in the garage and come up its ramp (north-west) or its stairwell
// (south-west); the Artisans start in a walled pen on the roof, next to A, with
// a fire escape down the east wall to the ground just north of B. "The stack"
// is a stairwell through the deck between the two sites.
func DeStack() c.Map {
	b := mapgen.New("de_stack")
	const g, l1, l2, slab = -4.0, 4.0, 8.0, 0.4
	b.Add(mapgen.Arena(-30, -30, 30, 30, 13, 1, c.Concrete, c.Brick)[1:]...) // walls; the floor has holes

	// The ground floor, open over the garage's ramp and stairwell.
	ramp := mapgen.Rect{X0: -30, Z0: 12.4, X1: -24, Z1: 22}
	stair := mapgen.Rect{X0: -24.4, Z0: -14, X1: -18, Z1: -11}
	for _, r := range mapgen.Fill(mapgen.Rect{X0: -30, Z0: -30, X1: 30, Z1: 30}, []mapgen.Rect{ramp, stair}) {
		b.Add(c.B(r.X0, -1, r.Z0, r.X1, 0, r.Z1, c.Concrete))
	}
	garage(b, g, ramp, stair)

	// Deck 1 (east half), open over the stack (x 12..16, z -12..-4) and the
	// fire escape's lower flight (esc).
	esc := mapgen.Rect{X0: 24, Z0: -14, X1: 27, Z1: -7.6}
	for _, r := range mapgen.Fill(mapgen.Rect{X0: 0, Z0: -30, X1: 30, Z1: 30}, []mapgen.Rect{{X0: 12, Z0: -12, X1: 16, Z1: -4}, esc}) {
		b.Add(p.Slab(r.X0, r.Z0, r.X1, r.Z1, l1, slab, c.Metal))
	}
	for _, x := range []float64{6, 18} {
		for _, z := range []float64{-24, -12, 0, 12, 24} {
			b.Add(c.B(x, 0, z, x+1, l1-slab, z+1, c.Concrete))
		}
	}
	// The stack: stairs down the hole to the ground floor.
	b.Add(p.Flight(true, -4, 12, 16, 0, 0.8, 0.5, 8, -1, c.Metal)...)
	// Ground -> deck 1: two flights up to the deck's west edge.
	b.Add(p.Flight(false, 0, -4, 0, 0, 0.8, 0.5, 8, -1, c.Metal)...)
	b.Add(p.Flight(false, 0, 20, 24, 0, 0.8, 0.5, 8, -1, c.Metal)...)
	// Railing along deck 1's west edge, open at the stairs, and round the escape's hole.
	b.Add(
		p.Railing(0, -30, 0.3, -4, l1, c.Hazard),
		p.Railing(0, 0, 0.3, 20, l1, c.Hazard),
		p.Railing(0, 24, 0.3, 30, l1, c.Hazard),
		p.Railing(esc.X0-0.3, esc.Z0, esc.X0, esc.Z1+0.3, l1, c.Hazard),
		p.Railing(esc.X0, esc.Z1, esc.X1, esc.Z1+0.3, l1, c.Hazard),
	)

	// Deck 2 (the roof, north-east, and the Artisans' pen south of it) on
	// columns standing on deck 1.
	b.Add(p.Slab(10, 4, 30, 30, l2, slab, c.Metal), p.Slab(18, -6, 30, 4, l2, slab, c.Metal))
	for _, cz := range [][2]float64{{12, 12}, {12, 28}, {28, 6}, {20, 28}, {18.3, -6}, {24.5, -6}} {
		b.Add(c.B(cz[0], l1, cz[1], cz[0]+0.7, l2-slab, cz[1]+0.7, c.Concrete))
	}
	b.Add(p.Flight(false, 10, 6, 9, l1, 0.8, 0.5, 8, -1, c.Metal)...)     // onto A from the west
	b.Add(p.Flight(false, 10, 26, 29.5, l1, 0.8, 0.5, 8, -1, c.Metal)...) // and from the north-west
	b.Add(
		p.Railing(10, 4, 18, 4.3, l2, c.Hazard),
		p.Railing(10, 9, 10.3, 26, l2, c.Hazard),
		p.Railing(10, 29.5, 10.3, 30, l2, c.Hazard),
	)
	// The pen: brick walls 2.4 m high, a door north onto A, the fire escape's
	// head in the south-east corner. The escape runs down the east wall to deck
	// 1, a landing, then turns back north down through the deck to the ground
	// floor beside B (retakers come in from the north, like down the stack).
	b.Add(p.WallGaps(true, 3.7, 18, 30, 0.3, l2, 2.4, c.Brick, p.Gap{From: 20, To: 22})...)
	b.Add(c.B(18, l2, -6, 18.3, l2+2.4, 3.7, c.Brick), c.B(18.3, l2, -6, 27, l2+2.4, -5.7, c.Brick))
	b.Add(p.Flight(true, -6, 27, 30, l1, 0.8, 0.5, 8, -1, c.Metal)...)
	b.Add(p.Flight(true, esc.Z0, esc.X0, esc.X1, 0, 0.8, 0.5, 8, 1, c.Metal)...)
	b.Add(p.Crate(26, l2, 0.5, 1.5, 1.2, 1.5), c.B(19.5, l2, -5.2, 22.5, l2+1.4, -4, c.Metal)) // a crate and an air-con unit

	// A on the roof: crates and an air-con unit.
	b.Add(p.Crate(18, l2, 20, 2, 2, 2), p.Crate(24, l2, 24, 1.5, 1.5, 1.5))
	b.Add(c.B(25, l2, 14, 28, l2+1.6, 16, c.Metal))
	// B under the deck: server racks.
	for _, z := range []float64{-25, -21} {
		b.Add(c.B(23, 0, z, 27, 2.2, z+0.8, c.Metal))
	}
	b.Add(p.Crate(15, 0, -22, 2, 1.5, 2))
	// A crate stack at the foot of the stack: no shooting down the stairwell onto
	// B's plant spot from deck 1 (bot sweeps: without it retakers won every B plant).
	b.Add(p.Crate(12, 0, -14.2, 2.75, 2.2, 1.2))

	// Ground floor west: racks and crates.
	b.Add(
		c.B(-10, 0, -28, -9.2, 2.2, -20, c.Metal),
		c.B(-16, 0, 10, -15.2, 2.2, 20, c.Metal),
		p.Crate(-13, 0, 2.5, 2, 2, 2),
		p.Crate(-21, 0, 14, 3, 2, 3),
		p.Crate(-20, 0, -26, 3, 2, 3),
	)
	b.Add(p.CratePyramid(-8, 12, 1.5)...)
	// Deck 1 cover.
	b.Add(p.Crate(4, l1, -20, 2, 1.5, 2), p.Crate(22, l1, -22, 2, 2, 2), p.Crate(4, l1, 14, 1.5, 1.5, 1.5))

	b.Item("codereview", 20, l2, 10)

	b.TeamSpawns(c.Vibe, [3]float64{-18, g, -3}, [3]float64{-14, g, -3}, [3]float64{-18, g, 3},
		[3]float64{-14, g, 3}, [3]float64{-16, g, 0})
	b.TeamSpawns(c.Artisan, [3]float64{20.5, l2, -3}, [3]float64{23.5, l2, -3}, [3]float64{26.5, l2, -3},
		[3]float64{22, l2, 1}, [3]float64{25, l2, 1})
	b.BuyAroundSpawns(3)
	b.Site("A", 14, l2-0.5, 12, 30, l2+3, 30)
	b.Site("B", 14, -0.5, -27, 30, 3, -12)

	b.Nav("vibe", -16, g, 0, "gSW", "rampBot").
		Nav("gSW", -26, g, -8, "stairBot").
		Nav("stairBot", -26, g, -12.5, "stairTop").
		Nav("stairTop", -17, 0, -12.5, "g4", "g2").
		Nav("rampBot", -27, g, 11, "rampTop").
		Nav("rampTop", -27, 0, 23.5, "gN").
		Nav("gN", -8, 0, 22, "stairsN").
		Nav("stairsN", 1, l1, 22, "d1N").
		Nav("d1N", 1, l1, 27.75, "roofN").
		Nav("roofN", 11, l2, 27.75, "A").
		Nav("g1", -8, 0, 8, "g3", "g2").
		Nav("g3", 3, 0, 6, "g4").
		Nav("g4", 3, 0, -16, "B").
		Nav("B", 20, 0, -17).
		Nav("g2", -8, 0, -2, "stairsW").
		Nav("stairsW", 1, l1, -2, "d1").
		Nav("d1", 1, l1, 7.5, "roofW", "stackTop").
		Nav("roofW", 11, l2, 7.5, "A").
		Nav("A", 20, l2, 18).
		Nav("artisan", 23, l2, -1, "penN", "escTop").
		Nav("penN", 21, l2, 5.5, "A").
		Nav("escTop", 28.5, l2, -5, "landing").
		Nav("landing", 28.5, l1, -15, "escTurn").
		Nav("escTurn", 25.5, l1, -15, "escBot", "d1W").
		Nav("escBot", 25.5, 0, -5, "escW").
		Nav("escW", 22.5, 0, -6, "B").
		Nav("d1W", 22.5, l1, -15, "d1S").
		Nav("d1S", 22.5, l1, -3, "stackTop").
		Nav("stackTop", 14, l1, -1, "stackBot").
		Nav("stackBot", 16.8, 0, -11.5, "B")
	b.Meta("Vertical: the Vibe Coders come up from the garage, the Artisans down from the roof. A on the roof, B under the deck, the stack's stairwell between them.", 12)
	b.FillSpawns(true) // one spawn per player, in the same spawn area
	b.Category(c.Training)
	return b.Map()
}

// garage is the Vibe Coders' level under the ground floor's west half (floor
// at y, the ground slab its ceiling 3 m up): pillars, painted parking bays with
// cars in some, ceiling lamps, a car ramp up in the north-west corner and a
// stairwell up in the south-west, both rising out through holes in the ground.
func garage(b *mapgen.Builder, y float64, ramp, stair mapgen.Rect) {
	const x0, z0, x1, z1, top = -30.0, -18.0, -4.0, 22.0, -1.0
	b.Add(
		c.B(x0, y-1, z0, x1, y, z1, c.Concrete),
		c.B(x0-0.5, y, z0-0.5, x0, 0, z1+0.5, c.Concrete), // up to the ground: it closes the ramp hole's side
		c.B(x1, y, z0-0.5, x1+0.5, top, z1+0.5, c.Concrete),
		c.B(x0, y, z0-0.5, x1, top, z0, c.Concrete),
		c.B(x0, y, z1, x1, top, z1+0.5, c.Concrete),
	)
	for _, x := range []float64{-22, -12, -6} {
		for _, z := range []float64{-9, 1, 11} {
			b.Add(c.B(x, y, z, x+0.8, top, z+0.8, c.Concrete))
		}
	}
	// The ramp: 16 shallow steps up to the ground at its north end, railed at the top.
	b.Add(p.Flight(true, ramp.Z1, ramp.X0, ramp.X1, y, 0.6, 0.25, 16, -1, c.Concrete)...)
	b.Add(p.Railing(ramp.X1, ramp.Z0, ramp.X1+0.3, ramp.Z1, 0, c.Hazard), p.Railing(ramp.X0, ramp.Z0-0.3, ramp.X1+0.3, ramp.Z0, 0, c.Hazard))
	// The stairwell: one flight rising east, railed round its hole.
	b.Add(p.Flight(false, stair.X1, stair.Z0, stair.Z1, y, 0.8, 0.5, 8, -1, c.Metal)...)
	b.Add(
		p.Railing(stair.X0-0.3, stair.Z0-0.3, stair.X1, stair.Z0, 0, c.Hazard),
		p.Railing(stair.X0-0.3, stair.Z1, stair.X1, stair.Z1+0.3, 0, c.Hazard),
		p.Railing(stair.X0-0.3, stair.Z0, stair.X0, stair.Z1, 0, c.Hazard),
	)
	// Parking bays along the south and north walls, 3 m wide, some taken.
	stripe := func(x, za, zb float64) { b.Add(c.B(x-0.06, y, za, x+0.06, y+0.02, zb, c.Hazard)) }
	for x := -16.0; x <= x1; x += 3 {
		stripe(x, z0, z0+5)
	}
	for x := -22.0; x <= x1; x += 3 {
		stripe(x, z1-5, z1)
	}
	b.Add(p.Car(-15.4, y, z0+0.5, false, 0)...)
	b.Add(p.Car(-9.4, y, z0+0.5, false, 2)...)
	b.Add(p.Car(-18.4, y, z1-4.9, false, 1)...)
	b.Add(p.Car(-9.4, y, z1-4.9, false, 3)...)
	// Ceiling panels: cool fluorescent, every 7 m or so, none over the holes.
	cool := [3]float64{0.85, 0.93, 1}
	for _, x := range []float64{-26, -19, -12.5, -7} {
		for _, z := range []float64{-14.5, -5, 5, 15} {
			if (x == -26 && z == 15) || (x == -19 && z == -14.5) {
				continue
			}
			l := c.Spot([3]float64{x, top - 0.15, z}, [3]float64{0, -1, 0}, cool, 9, 7, 80)
			l.Inner = 40 // no shadow maps: they doubled the lamps' render cost (perf-lights)
			b.Light(l)
			b.Add(c.B(x-0.6, top-0.08, z-0.3, x+0.6, top, z+0.3, c.Lamp))
		}
	}
}
