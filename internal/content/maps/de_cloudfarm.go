package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeCloudfarm is a competitive-size data centre (148 × 120 m): "someone
// else's computer". The Vibe Coders unload at the west loading dock, the
// Artisans hold from the NOC in the east. A is Hall A's rack aisles (north
// east), B the cooling plant (south east). The ways in:
//   - north: up the yard road, through the generator yard and the cable
//     gallery to A's main door;
//   - mid: the cold aisle (its containment door halfway) into the core switch
//     room; from there A short or B short, or the ops corridor to the NOC.
//     Two flanks leave the cold aisle: north to the generator yard, south to
//     the battery room;
//   - south: the battery room, then up onto the pipe bridge and B's catwalk,
//     with steps down into the plant.
//
// The floor plan is the open space below; mapgen.Fill makes the rest building.
func DeCloudfarm() c.Map {
	b := mapgen.New("de_cloudfarm")
	bounds := mapgen.Rect{X0: -74, Z0: -60, X1: 74, Z1: 60}
	b.Add(mapgen.Arena(bounds.X0, bounds.Z0, bounds.X1, bounds.Z1, 11, 1, c.Concrete, c.Concrete)...)
	open := []mapgen.Rect{
		{X0: -74, Z0: -14, X1: -56, Z1: 14},  // the loading dock (Vibe Coders)
		{X0: -70, Z0: 14, X1: -58, Z1: 42},   // the yard road north
		{X0: -58, Z0: 30, X1: -20, Z1: 42},   // the generator yard
		{X0: -24, Z0: 30, X1: -16, Z1: 56},   //   and its way up to
		{X0: -16, Z0: 44, X1: 19, Z1: 56},    // the cable gallery
		{X0: 19, Z0: 46, X1: 20, Z1: 52},     // A main door
		{X0: 20, Z0: 24, X1: 74, Z1: 60},     // Hall A
		{X0: -56, Z0: -5, X1: 0, Z1: 5},      // the cold aisle (mid)
		{X0: -42, Z0: 5, X1: -36, Z1: 30},    //   its north flank
		{X0: -32, Z0: -30, X1: -26, Z1: -5},  //   its south flank
		{X0: 0, Z0: -14, X1: 16, Z1: 14},     // the core switch room
		{X0: 16, Z0: 8, X1: 28, Z1: 14},      // A short
		{X0: 22, Z0: 14, X1: 28, Z1: 24},     //
		{X0: 16, Z0: -14, X1: 28, Z1: -8},    // B short
		{X0: 22, Z0: -24, X1: 28, Z1: -14},   //
		{X0: 16, Z0: -4, X1: 50, Z1: 4},      // the ops corridor
		{X0: 50, Z0: -12, X1: 74, Z1: 12},    // the NOC (Artisans)
		{X0: 58, Z0: 12, X1: 74, Z1: 24},     //   ramp to A
		{X0: 58, Z0: -24, X1: 74, Z1: -12},   //   ramp to B
		{X0: 20, Z0: -60, X1: 74, Z1: -24},   // the cooling plant
		{X0: -70, Z0: -42, X1: -58, Z1: -14}, // the yard road south
		{X0: -58, Z0: -42, X1: -20, Z1: -30}, // the battery room
		{X0: -24, Z0: -58, X1: -16, Z1: -30}, //   and its way down to
		{X0: -16, Z0: -58, X1: 20, Z1: -46},  // the pipe bridge
	}
	for i, r := range mapgen.Fill(bounds, open) {
		h := 9.0 + float64(i%3) // a ragged roofline
		b.Add(c.B(r.X0, 0, r.Z0, r.X1, h, r.Z1, c.Plaster))
	}

	// The loading dock: a container and pallets.
	b.Add(p.Container(-73.5, 0, -12, true, false, 1), p.Crate(-60, 0, -12, 2, 1.2, 2), p.Crate(-60, 0, 10, 1.5, 1.5, 1.5))
	// The cold aisle: racks along its walls, the containment door halfway.
	b.Add(
		c.B(-50, 0, -5, -44, 2.2, -3.8, c.Rack),
		c.B(-24, 0, 3.8, -18, 2.2, 5, c.Rack),
		c.B(-10, 0, -5, -6, 2.2, -3.8, c.Rack),
	)
	b.Add(p.WallGaps(false, -14, -5, 5, 0.4, 0, 3, c.Metal, p.Gap{From: -1.5, To: 1.5, H: 2.4})...)
	// The core switch room: the switch, blocking the aisle-to-NOC line.
	b.Add(c.B(4, 0, -6, 12, 3, 6, c.Rack), p.Crate(1, 0, -13, 2, 1.5, 2), p.Crate(13, 0, 11.5, 2, 1.2, 2))
	// The generator yard and the cable gallery.
	b.Add(
		c.B(-52, 0, 31, -46, 2.4, 35, c.Metal),
		c.B(-32, 0, 37, -28, 2.4, 41, c.Metal),
		p.Crate(-6, 0, 44, 2, 1.5, 2),
		c.B(-14, 3.2, 49.5, 18, 3.4, 50.5, c.Metal), // a cable tray overhead
	)
	// The battery room: battery strings.
	b.Add(c.B(-52, 0, -32, -44, 2, -31, c.Rack), c.B(-36, 0, -41, -28, 2, -40, c.Rack))
	// The pipe bridge: steps up from the battery side, a deck at 3 m, B's catwalk.
	const deck = 3.0
	b.Add(p.Flight(false, -12, -58, -46, 0, 0.8, 0.5, 6, -1, c.Metal)...)
	b.Add(p.Slab(-12, -58, 20, -46, deck, deck, c.Metal), p.Slab(20, -60, 26, -34, deck, deck, c.Metal))
	b.Add(p.Flight(false, 26, -44, -38, 0, 0.8, 0.5, 6, 1, c.Metal)...)
	b.Add(
		p.Railing(26, -60, 26.3, -44, deck, c.Hazard),
		p.Railing(26, -38, 26.3, -34, deck, c.Hazard),
		p.Railing(20, -34.3, 26, -34, deck, c.Hazard),
	)

	// Hall A: rack rows with a crossing gap, cooling units in the east aisle.
	for _, z := range []float64{30, 36, 50, 56} {
		b.Add(c.B(30, 0, z, 39, 2.2, z+1, c.Rack), c.B(41, 0, z, 50, 2.2, z+1, c.Rack))
	}
	b.Add(
		c.B(54, 0, 34, 58, 2.5, 38, c.Metal),
		c.B(54, 0, 46, 58, 2.5, 50, c.Metal),
		p.Crate(34, 0, 39.5, 2, 1.5, 1.5), p.Crate(46, 0, 44, 1.5, 1.2, 1.5),
		p.Crate(21, 0, 34, 2, 2, 2),
	)
	// The cooling plant: chillers, a pump, the cooling tower, pipes overhead.
	b.Add(
		c.B(32, 0, -54, 38, 2.6, -50, c.Metal),
		c.B(44, 0, -54, 50, 2.6, -50, c.Metal),
		c.B(40, 0, -38, 43, 1.4, -36, c.Metal),
		c.B(58, 0, -58, 68, 9, -48, c.Concrete),
		c.B(28, 3.4, -30, 56, 4, -29, c.Metal),
		p.Crate(48, 0, -42, 2, 1.5, 2),
	)
	// The NOC: a desk row facing the wall screens.
	b.Add(p.Desk(52, 0, -10, 2, 1, false)...)
	b.Add(p.Desk(52, 0, 8.5, 2, 1, true)...)

	b.TeamSpawns(c.Vibe, [3]float64{-68, 0, -6}, [3]float64{-68, 0, 0}, [3]float64{-68, 0, 6},
		[3]float64{-64, 0, -3}, [3]float64{-64, 0, 3})
	b.TeamSpawns(c.Artisan, [3]float64{66, 0, -6}, [3]float64{66, 0, 0}, [3]float64{66, 0, 6},
		[3]float64{62, 0, -3}, [3]float64{62, 0, 3})
	b.BuyAroundSpawns(3)
	b.Site("A", 30, -0.5, 32, 52, 3, 54)
	b.Site("B", 30, -0.5, -56, 52, 3, -34)

	b.Nav("vibe", -66, 0, 0, "ca1", "nRoad", "sRoad").
		// north
		Nav("nRoad", -64, 0, 24, "genW").
		Nav("genW", -62, 0, 36, "gen").
		NavLane("gen", -40, 0, 36, "genE").
		Nav("genE", -20, 0, 36, "nConn").
		Nav("nConn", -20, 0, 50, "gal").
		Nav("gal", 0, 0, 50, "aDoor").
		Nav("aDoor", 19.5, 0, 49, "aW").
		Nav("aW", 26, 0, 43, "A", "aS").
		Nav("A", 40, 0, 43).
		// mid
		Nav("ca1", -48, 0, 0, "caF1").
		Nav("caF1", -39, 0, 0, "caF2", "fN").
		Nav("caF2", -29, 0, 0, "ca3", "fS").
		NavLane("ca3", -3, 0, 0, "hubW").
		Nav("hubW", 2, 0, 0, "hubN", "hubS").
		Nav("hubN", 2, 0, 10, "hubNE").
		Nav("hubNE", 14, 0, 10, "asc", "hubE").
		Nav("asc", 25, 0, 11, "asc2").
		Nav("asc2", 25, 0, 20, "aS").
		Nav("aS", 25, 0, 27).
		Nav("hubS", 2, 0, -10, "hubSE").
		Nav("hubSE", 14, 0, -10, "bsc", "hubE").
		Nav("bsc", 25, 0, -11, "bsc2").
		Nav("bsc2", 25, 0, -20, "bS").
		Nav("bS", 25, 0, -27, "B").
		Nav("B", 40, 0, -45).
		Nav("hubE", 14, 0, 0, "opsE").
		Nav("opsE", 48, 0, 0, "artisan").
		NavLane("fN", -39, 0, 18, "gen").
		NavLane("fS", -29, 0, -18, "fS2").
		Nav("fS2", -29, 0, -34, "bat").
		// south
		Nav("sRoad", -64, 0, -24, "batW").
		Nav("batW", -62, 0, -36, "bat").
		NavLane("bat", -40, 0, -36, "batE").
		Nav("batE", -20, 0, -36, "sConn").
		Nav("sConn", -20, 0, -52, "bridgeUp").
		Nav("bridgeUp", -10, deck, -52, "bridge").
		NavLane("bridge", 6, deck, -52, "cat").
		Nav("cat", 23, deck, -50, "catS").
		Nav("catS", 23, deck, -41, "bStair").
		Nav("bStair", 34, 0, -41, "B").
		// the Artisans
		Nav("artisan", 66, 0, 0, "ctN", "ctS").
		Nav("ctN", 66, 0, 18, "aE").
		Nav("aE", 60, 0, 43, "A").
		Nav("ctS", 66, 0, -18, "bE").
		Nav("bE", 56, 0, -42, "B")
	b.Meta("Competitive. A data centre: Hall A's rack aisles, the cooling plant on B, the cold aisle and the core switch room in mid.", 10)
	b.FillSpawns(true) // one spawn per player, in the same spawn area
	b.Category(c.Competitive)
	return b.Map()
}
