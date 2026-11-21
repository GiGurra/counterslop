package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeHackathon is a CS-scale conference centre, 120 × 150 m. The Vibe Coders
// arrive in the car park (south); the Artisans hold the green room (north
// west). A is the main stage in the auditorium (north east), B the expo hall
// (west). Routes: the food court into the expo hall (B long), the atrium
// (mid) with Breakout into B and the auditorium's front door to A, the
// atrium's mezzanine over the bridge onto the auditorium balcony (the high
// way to A) and the service corridor up the east side to the stage wing
// (A long). The sponsor lounge links the atrium and the service corridor.
func DeHackathon() c.Map {
	b := mapgen.New("de_hackathon")
	const wallH = 9.0
	b.Add(mapgen.Arena(-60, -75, 60, 75, 10, 1, c.Concrete, c.Plaster)...)
	block := func(x0, z0, x1, z1 float64, mat string) { b.Add(p.Wall(x0, z0, x1, z1, wallH, mat)) }
	gap := func(from, to, h float64) p.Gap { return p.Gap{From: from, To: to, H: h} }

	// The facade on the car park: food court, main and service entrances.
	b.Add(p.WallGaps(true, -55, -60, 60, 1, 0, wallH, c.Brick, gap(-50, -44, 3.5), gap(-8, 8, 3.5), gap(48, 56, 3.5))...)
	// Food court | registration, registration | atrium, food court | expo hall.
	b.Add(p.WallGaps(false, -26, -54, -31, 4, 0, wallH, c.Plaster, gap(-46, -41, 3.2))...)
	b.Add(p.WallGaps(true, -31, -26, 22, 1, 0, wallH, c.Plaster, gap(-10, 10, 3.5))...)
	b.Add(p.WallGaps(true, -20, -60, -27, 1, 0, wallH, c.Plaster, gap(-50, -42, 3.5))...)
	// Atrium west wall (Breakout door to B) and north wall (the keynote hallway).
	b.Add(p.WallGaps(false, -27, -30, 11, 1, 0, wallH, c.Plaster, gap(2, 7, 3.2))...)
	b.Add(p.WallGaps(true, 10, -26, 0, 1, 0, wallH, c.Plaster, gap(-8, 0, 3.5))...)
	b.Add(p.WallGaps(true, 30, -8, 0, 0.6, 0, wallH, c.Plaster, gap(-6, -2, 3))...)
	// Solid blocks: breakout rooms, the hallway's east side, the east wing
	// (with the sponsor lounge cut through it).
	block(-26, 11, -8, 46, c.Brick)
	block(0, 10, 8, 46, c.Brick)
	block(22, -54, 48, -31, c.Brick)
	block(26, -31, 48, -12, c.Brick)
	block(26, -8, 48, 20, c.Brick)
	// Expo hall north wall (the Artisans' door into B).
	b.Add(p.WallGaps(true, 40, -60, -26, 1, 0, wallH, c.Plaster, gap(-46, -40, 3.5))...)

	// The auditorium: walls, stage (A), seating, balcony.
	const balc = 4.5
	b.Add(p.WallGaps(true, 20, 8, 60, 1, 0, balc-0.3, c.Plaster, gap(22, 28, 3.2), gap(50, 58, 3.5))...)
	b.Add(p.WallGaps(true, 20, 8, 60, 1, balc-0.3, wallH-balc+0.3, c.Plaster, gap(14, 18, 2.6))...)
	b.Add(p.WallGaps(false, 6, 46, 66, 2, 0, wallH, c.Plaster, gap(50, 55, 3.5))...)
	b.Add(p.WallGaps(true, 66, 6, 60, 1, 0, wallH, c.Plaster, gap(36, 42, 3.5))...)
	const stage = 1.2
	b.Add(c.B(40, 0, 30, 58, stage, 58, c.Desk))
	b.Add(p.Flight(false, 40, 36, 42, 0, 0.8, 0.4, 3, -1, c.Desk)...) // front
	b.Add(p.Flight(true, 30, 50, 56, 0, 0.8, 0.4, 3, -1, c.Desk)...)  // the east wing
	b.Add(p.Flight(true, 58, 38, 42, 0, 0.8, 0.4, 3, 1, c.Desk)...)   // backstage
	b.Add(
		c.B(41, stage, 31, 42.5, stage+2.4, 32.5, c.Metal), // speaker stacks
		c.B(41, stage, 56.3, 42.5, stage+2.4, 57.8, c.Metal),
		c.B(52, stage, 47, 53.2, stage+1.2, 48.5, c.Desk), // the lectern
		p.Crate(55, stage, 38, 2, 1.5, 2), p.Crate(43, stage, 46, 1.5, 1.5, 1.5),
		c.B(58, 0, 34, 59.5, 6, 54, c.Concrete), // the screen
	)
	for _, x := range []float64{26, 29.5, 33} { // seat rows, an aisle at z 34..42
		b.Add(c.B(x, 0, 24, x+0.6, 1.0, 34, c.Carpet), c.B(x, 0, 42, x+0.6, 1.0, 63, c.Carpet))
	}
	b.Add(p.Slab(8, 20, 18, 66, balc, 0.3, c.Metal))
	b.Add(p.Railing(17.7, 20, 18, 34.5, balc, c.Hazard), p.Railing(17.7, 37.5, 18, 66, balc, c.Hazard))
	b.Add(p.Flight(false, 18, 34.5, 37.5, 0, 0.75, 0.5, 9, 1, c.Metal)...)
	for _, z := range []float64{28, 48, 58} {
		b.Add(c.B(17.4, 0, z, 18, balc-0.3, z+0.6, c.Concrete))
	}

	// The atrium: info desk, the sponsor banner across the keynote hallway's
	// mouth (no straight look down the hall), the mezzanine (east side, 4.5 m)
	// and its stairs.
	b.Add(c.B(-12, 0, -2, -6, 1.1, 2, c.Desk))
	b.Add(c.B(-9.5, 0, 2.6, 1, 3.2, 3.2, c.Desk))
	b.Add(p.Slab(14, -22, 26, 20, balc, 0.3, c.Metal))
	b.Add(p.Flight(true, -22, 18, 22, 0, 0.75, 0.5, 9, -1, c.Metal)...)
	b.Add(
		p.Railing(14, -22, 14.3, 20, balc, c.Hazard),
		p.Railing(14.3, -22, 18, -21.7, balc, c.Hazard),
		p.Railing(22, -22, 26, -21.7, balc, c.Hazard),
	)
	for _, z := range []float64{-4, 12} {
		b.Add(c.B(14, 0, z, 14.6, balc-0.3, z+0.6, c.Concrete))
	}

	// Registration: badge desks.
	b.Add(c.B(-16, 0, -42, -10, 1.1, -40.5, c.Desk), c.B(10, 0, -42, 16, 1.1, -40.5, c.Desk))
	b.Add(p.Crate(-20, 0, -52, 2, 2, 2), p.Crate(18, 0, -36, 1.5, 1.5, 1.5))

	// The service corridor (A long): a roofed loading bay, containers, crates.
	b.Add(p.Slab(48, -24, 60, 6, 3.6, 0.4, c.Concrete))
	b.Add(p.Container(56, 0, -48, false, false, 1), p.Container(49, 0, -30, false, false, 2))
	b.Add(p.Crate(57, 0, -12, 2, 2, 2), p.Crate(49, 0, 10, 1.5, 1.5, 1.5), p.Crate(57.5, 0, 14, 1.5, 1.5, 1.5))
	// Two staggered partitions (a dogleg): no look down the whole corridor.
	b.Add(c.B(48, 0, -36, 52.5, 3.2, -35.4, c.Concrete), c.B(52.5, 0, 0, 60, 3.2, 0.6, c.Concrete))

	// B: the expo hall. Booths (three low walls each) and the sponsor stand on site.
	booth := func(x, z float64, openN bool) {
		bz := z
		if openN {
			bz = z
		} else {
			bz = z + 3.8
		}
		b.Add(c.B(x, 0, bz, x+5, 2.2, bz+0.2, c.Plaster), c.B(x, 0, z, x+0.2, 2.2, z+4, c.Plaster), c.B(x+4.8, 0, z, x+5, 2.2, z+4, c.Plaster))
	}
	booth(-58, -14, false)
	booth(-36, -14, false)
	// The welcome banner just inside the expo hall's south door: no straight
	// look from the site down into the food court.
	b.Add(c.B(-52, 0, -13.4, -41.5, 3.2, -12.8, c.Desk))
	// The poster wall inside the Breakout door: you come in and turn south
	// (an L), not straight onto the site.
	b.Add(c.B(-31, 0, -1, -30.4, 3.2, 13.5, c.Desk))
	booth(-34, 14, true)
	booth(-58, 30, true)
	booth(-36, 30, true)
	b.Add(
		c.B(-54, 0, 16, -50, 2.4, 20, c.Metal), // the sponsor stand
		c.B(-42, 0, 22, -39, 0.9, 26, c.Desk),  // demo table
		c.B(-53, 0, -4, -50, 1.2, -1, c.Desk),  // the hack demo kiosks on site
		c.B(-42, 0, 4, -40, 2.2, 6, c.Metal),
		p.Crate(-56, 0, 9, 2, 2, 2), p.Crate(-40, 0, 9, 1.5, 1.5, 1.5),
	)

	// The food court: tables and the coffee counter.
	for _, xz := range [][2]float64{{-56, -45}, {-56, -32}, {-38, -45}, {-38, -33}} {
		b.Add(p.Desk(xz[0], 0, xz[1], 1.6, 1.6, true)...)
	}
	b.Add(c.B(-58, 0, -26, -50, 1.1, -24.5, c.Desk))

	// The car park: cars (body and cabin).
	for _, xz := range [][2]float64{{-40, -66}, {-28, -66}, {28, -66}, {40, -66}, {-20, -72}, {20, -72}} {
		x, z := xz[0], xz[1]
		b.Add(c.B(x, 0, z, x+4.4, 1.0, z+1.9, c.Metal), c.B(x+1, 1.0, z+0.1, x+3.4, 1.5, z+1.8, c.Concrete))
	}

	// The green room and hotel lobby (Artisan side).
	b.Add(
		c.B(-40, 0, 56, -34, 0.8, 57.5, c.Carpet), // sofas
		c.B(-52, 0, 64, -46, 0.8, 65.5, c.Carpet),
		c.B(-20, 0, 52, -14, 1.1, 53.5, c.Desk), // concierge
		p.Crate(-56, 0, 46, 2, 2, 2), p.Crate(20, 0, 73.4, 1.5, 1.5, 1.5),
	)

	b.Item("codereview", 0, 0, -20)

	b.TeamSpawns(c.Vibe, [3]float64{-6, 0, -65}, [3]float64{-3, 0, -62}, [3]float64{0, 0, -65},
		[3]float64{3, 0, -62}, [3]float64{6, 0, -65})
	b.TeamSpawns(c.Artisan, [3]float64{-14, 0, 70}, [3]float64{-11, 0, 67}, [3]float64{-8, 0, 70},
		[3]float64{-5, 0, 67}, [3]float64{-2, 0, 70})
	b.BuyAroundSpawns(3)
	b.Site("A", 38, 0.5, 28, 58, 4, 58)
	b.Site("B", -56, -0.5, -6, -36, 3, 14)

	b.Nav("vibe", 0, 0, -63, "pk").
		Nav("pk", 0, 0, -60, "pW", "pE", "regS").
		Nav("pW", -47, 0, -60, "fcDoor").
		Nav("fcDoor", -47, 0, -52, "food").
		NavLane("food", -47, 0, -36, "bMain", "fcE").
		Nav("fcE", -30, 0, -43.5, "regW").
		Nav("regW", -18, 0, -43.5, "regS").
		Nav("bMain", -46, 0, -19.5, "expoIn").
		Nav("expoIn", -39.5, 0, -16, "expoS").
		Nav("expoS", -39.5, 0, -8, "B").
		Nav("regS", 0, 0, -50, "mid").
		NavLane("mid", 0, 0, -40, "turn").
		Nav("turn", 0, 0, -29, "atr", "mezzApp").
		Nav("mezzApp", 12, 0, -29.6, "mezzBot").
		Nav("atr", 0, 0, -12, "breakE", "um", "atrE").
		Nav("breakE", -22, 0, 4.5, "breakW").
		Nav("breakW", -29, 0, 4.5, "breakS").
		Nav("breakS", -29, 0, -3.5, "expoS").
		Nav("um", 20, 0, 8, "amain").
		Nav("amain", 25, 0, 19, "aDoor").
		Nav("aDoor", 25, 0, 22.8, "aSouth").
		Nav("aSouth", 36.5, 0, 22.8, "aFront").
		Nav("aFront", 36.5, 0, 39.5, "stageF").
		Nav("stageF", 42, stage, 39.5, "A").
		Nav("mezzBot", 20, 0, -29.6, "mezzTop").
		Nav("mezzTop", 20, balc, -21, "balcony").
		NavLane("balcony", 20, balc, 10, "bridge").
		Nav("bridge", 16, balc, 21, "balc").
		Nav("balc", 13, balc, 30, "balc2").
		Nav("balc2", 13, balc, 36, "stairTop").
		Nav("stairTop", 17.4, balc, 36, "stairBot").
		Nav("stairBot", 25.6, 0, 36, "aFront").
		Nav("atrE", 22, 0, -10, "svcM").
		Nav("pE", 52, 0, -60, "svcDoor").
		Nav("svcDoor", 52, 0, -53, "service").
		NavLane("service", 55, 0, -20, "svcM").
		Nav("svcM", 54, 0, -10, "svcW").
		Nav("svcW", 50, 0, 0.3, "svcN").
		Nav("svcN", 54, 0, 14, "wing").
		Nav("wing", 54, 0, 20.5, "wingIn").
		Nav("wingIn", 53, 0, 25, "stageS").
		Nav("stageS", 53, stage, 32, "A").
		Nav("A", 49, stage, 44).
		Nav("B", -46, 0, 2).
		Nav("artisan", -8, 0, 68, "ctE", "ctW", "hallN", "ctN").
		Nav("ctE", 2, 0, 61, "audW").
		Nav("audW", 7, 0, 52.5, "audIn").
		Nav("audIn", 12, 0, 52, "audAisle").
		Nav("audAisle", 12, 0, 40, "aFront").
		Nav("ctN", 2, 0, 71, "bsE").
		Nav("bsE", 39, 0, 71, "bsDoor").
		Nav("bsDoor", 39, 0, 66.5, "stageN").
		Nav("stageN", 40, stage, 56, "A").
		Nav("ctW", -30, 0, 50, "expoN").
		Nav("expoN", -43, 0, 40.5, "B").
		Nav("hallN", -4, 0, 46, "hall").
		Nav("hall", -4, 0, 25, "atrN").
		Nav("atrN", -4, 0, 8, "atrNE").
		Nav("atrNE", 2.6, 0, 5.2, "atr")
	b.Meta("CS-scale conference centre: A on the main stage, B in the expo hall; the atrium mid, its mezzanine to the auditorium balcony, the service corridor to the stage wing.", 10)
	b.Category(c.Competitive)
	return b.Map()
}
