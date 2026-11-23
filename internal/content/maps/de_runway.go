package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeRunway is a competitive-size desert market town (140 × 120 m): a startup
// burning its runway in a sun-bleached co-working bazaar. The Vibe Coders
// arrive at the caravan yard in the east, the Artisans hold from the
// accelerator in the west. Three levels: the street (y 0), A's rooftop
// terrace (y 3, north-west) and the old cistern (y -3) under the south of
// town, which opens into B's sunken court (south-west).
//   - The runway: the long market street along the north, past the stalls
//     and through the arch, to the stairs up onto A.
//   - The caravanserai in the middle: a walled courtyard with a gate on each
//     side (east and west ones offset, so no line runs through). North to
//     the runway, west to the accelerator's road and the alley up to A,
//     south down the stairwell into the cistern.
//   - The cistern: from the south yard (or the caravanserai's stairwell)
//     under the town into B's court.
//
// The defenders reach A up the terrace stairs and B down the court stairs.
//
// Balance (TestAIBalanceSweep, was 81% attacker wins): screens stand at the
// top of every stair onto A, so nobody climbing peeks the site head-first
// (the climber saw the defender's body before showing its own); both
// attacker stairs share one gap onto the site. B's stairs have a wall on
// the court side, so retakers reach the bottom unseen; the fountain's
// statue breaks the hold's line down the west tunnel. The back lane no
// longer reaches the west road (attackers came out on the defenders' way
// to B), and A's zone stops short of the stair landing's nook.
func DeRunway() c.Map {
	b := mapgen.New("de_runway")
	bounds := mapgen.Rect{X0: -70, Z0: -60, X1: 70, Z1: 60}
	const (
		ground  = 0.0
		cistern = -3.0
		terrace = 3.0
	)
	// Below street level: the cistern, its stairwells and B's court.
	sunken := []mapgen.Rect{
		{X0: 3, Z0: -44, X1: 36, Z1: -38},    // the cistern: the east tunnel,
		{X0: -3, Z0: -52, X1: 3, Z1: -38},    //   the cross under the caravanserai,
		{X0: -28, Z0: -52, X1: -3, Z1: -46},  //   the west tunnel to B
		{X0: 36, Z0: -44, X1: 42, Z1: -38},   //   its stairwell from the south yard
		{X0: -3, Z0: -38, X1: 3, Z1: -32},    //   and from the caravanserai
		{X0: -58, Z0: -58, X1: -28, Z1: -28}, // B's sunken court
	}
	// Open to the sky at street level (and the sunken ones that are).
	open := []mapgen.Rect{
		{X0: 50, Z0: -14, X1: 70, Z1: 14},    // the caravan yard (Vibe Coders)
		{X0: 54, Z0: 14, X1: 66, Z1: 48},     //   its road north
		{X0: -30, Z0: 38, X1: 66, Z1: 48},    // the runway
		{X0: 30, Z0: 9, X1: 36, Z1: 38},      //   the awning alley onto it
		{X0: -58, Z0: 26, X1: -22, Z1: 58},   // A: the terrace and the strip below it
		{X0: -30, Z0: -4, X1: -22, Z1: 38},   //   the alley up to it
		{X0: -70, Z0: -10, X1: -54, Z1: 10},  // the accelerator (Artisans)
		{X0: -58, Z0: 10, X1: -50, Z1: 26},   //   its stairs up to A
		{X0: -62, Z0: -20, X1: -54, Z1: -10}, //   its way down to B
		{X0: -62, Z0: -28, X1: -52, Z1: -20}, //   the landing above B's stairs
		{X0: -52, Z0: -24, X1: -30, Z1: -20}, // the back lane to it
		{X0: -12, Z0: -12, X1: 12, Z1: 12},   // the caravanserai courtyard
		{X0: 12, Z0: 4, X1: 18, Z1: 9},       //   east gate
		{X0: 18, Z0: 4, X1: 50, Z1: 9},       //   east road (from the yard)
		{X0: -18, Z0: -9, X1: -12, Z1: -4},   //   west gate
		{X0: -54, Z0: -9, X1: -18, Z1: -4},   //   west road (to the accelerator)
		{X0: -2.5, Z0: 12, X1: 2.5, Z1: 38},  //   north gate and lane to the runway
		{X0: -3, Z0: -38, X1: 3, Z1: -12},    //   south gate, lane and stairwell
		{X0: 42, Z0: -44, X1: 66, Z1: -14},   // the south yard
		{X0: 36, Z0: -44, X1: 42, Z1: -38},   //   its stairwell
		{X0: -58, Z0: -58, X1: -28, Z1: -28}, // B's court
	}

	// The ground: paving down to the cistern's floor, with the sunken parts
	// cut out (their floor at -3); walls round it all.
	for _, r := range mapgen.Fill(bounds, sunken) {
		b.Add(c.B(r.X0, cistern-1, r.Z0, r.X1, ground, r.Z1, c.Stucco))
	}
	for _, r := range sunken {
		b.Add(c.B(r.X0, cistern-1, r.Z0, r.X1, cistern, r.Z1, c.Sand))
	}
	const wallH, t = 10.0, 1.0
	b.Add(
		c.B(bounds.X0-t, cistern-1, bounds.Z0-t, bounds.X1+t, wallH, bounds.Z0, c.Stucco),
		c.B(bounds.X0-t, cistern-1, bounds.Z1, bounds.X1+t, wallH, bounds.Z1+t, c.Stucco),
		c.B(bounds.X0-t, cistern-1, bounds.Z0, bounds.X0, wallH, bounds.Z1, c.Stucco),
		c.B(bounds.X1, cistern-1, bounds.Z0, bounds.X1+t, wallH, bounds.Z1, c.Stucco),
	)
	// The town: everything else is buildings, flat-roofed, a ragged skyline
	// (over the cistern too: its roof is their floor).
	for i, r := range mapgen.Fill(bounds, open) {
		b.Add(c.B(r.X0, ground, r.Z0, r.X1, 6.0+float64(i%4), r.Z1, c.Stucco))
	}

	// The cistern's vault: a dark ceiling under the houses above it.
	for _, r := range []mapgen.Rect{{X0: 3, Z0: -44, X1: 36, Z1: -38}, {X0: -3, Z0: -52, X1: 3, Z1: -38}, {X0: -28, Z0: -52, X1: -3, Z1: -46}} {
		b.Add(c.B(r.X0, ground-0.3, r.Z0, r.X1, ground, r.Z1, c.Concrete))
	}
	// Stairwells down into the cistern and B's court.
	b.Add(p.Flight(false, 42, -44, -38, cistern, 0.8, 0.5, 6, -1, c.Stucco)...) // from the south yard
	b.Add(p.Flight(true, -32, -3, 3, cistern, 0.8, 0.5, 6, -1, c.Stucco)...)    // from the caravanserai
	b.Add(p.Flight(true, -28, -58, -52, cistern, 0.8, 0.5, 6, -1, c.Stucco)...) // from the landing, into B
	b.Add(c.B(-52, cistern, -34, -51, ground+1.2, -28, c.Stucco))               // its wall on the court side

	// A: the rooftop terrace, stairs up from the runway, the alley and the accelerator.
	b.Add(c.B(-58, ground, 26, -30, terrace, 58, c.Stucco))
	b.Add(p.Flight(false, -30, 40, 46, ground, 0.8, 0.5, 6, 1, c.Stucco)...)  // the runway stairs
	b.Add(p.Flight(false, -30, 28, 34, ground, 0.8, 0.5, 6, 1, c.Stucco)...)  // the alley stairs
	b.Add(p.Flight(true, 26, -58, -50, ground, 0.8, 0.5, 6, -1, c.Stucco)...) // the accelerator's
	b.Add(
		p.Railing(-30.3, 26, -30, 28, terrace, c.Stucco),
		p.Railing(-30.3, 34, -30, 40, terrace, c.Stucco),
		p.Railing(-30.3, 46, -30, 58, terrace, c.Stucco),
	)
	b.Add(p.Stall(-50, terrace, 50, 3, 1.2, 1.5, false)...)
	b.Add(
		c.B(-46, terrace, 36, -42, terrace+1.2, 38, c.Stucco),   // the pitch stage
		c.B(-35, terrace, 39, -34, terrace+3, 47, c.Stucco),     // the kiosk wall at the runway stairs
		c.B(-35, terrace, 26, -34, terrace+3, 35, c.Stucco),     // and its twin at the alley stairs
		c.B(-58, terrace, 28.5, -50, terrace+3, 29.5, c.Stucco), // and the screen at the accelerator's
		p.Crate(-40, terrace, 46, 2, 2, 2), p.Crate(-38, terrace, 46, 1.5, 1.5, 1.5),
		p.Crate(-56, terrace, 36, 2, 1.5, 2),
		c.B(-40, terrace, 30, -39, terrace+4, 31, c.Stucco), // a column
	)

	// The runway: stalls along it, and the market arch halfway.
	b.Add(p.Stall(44, ground, 38, 3, 1.2, 1.4, true)...)
	b.Add(p.Stall(26, ground, 46.8, 3, 1.2, 1.4, false)...)
	b.Add(p.Stall(8, ground, 38, 3, 1.2, 1.4, true)...)
	b.Add(p.Stall(-2, ground, 46.8, 3, 1.2, 1.4, false)...)
	// The souk bend: two houses jut into the runway before A, so the terrace
	// doesn't see down the whole street.
	b.Add(c.B(-22, ground, 38, -18, 7, 44, c.Stucco), c.B(-12, ground, 42, -8, 7, 48, c.Stucco))
	b.Add(p.WallGaps(false, 18, 38, 48, 1, ground, 7, c.Stucco, p.Gap{From: 41, To: 45, H: 4})...)
	b.Add(p.Crate(56, ground, 30, 2, 1.5, 2))

	// The caravanserai: the well, stalls round the walls.
	b.Add(c.B(-2, ground, -2, 2, 1.1, 2, c.Stucco))
	b.Add(p.Stall(-11, ground, 6, 3, 1.2, 1.4, false)...)
	b.Add(p.Stall(6, ground, -11, 3, 1.2, 1.4, true)...)
	b.Add(p.Crate(8, ground, -2, 2, 1.5, 2))

	// The cistern: crates, a column or two. B's court: the dry fountain, crates.
	b.Add(p.Crate(16, cistern, -44, 2, 1.5, 2), c.B(-14, cistern, -52, -13, ground-0.3, -51, c.Stucco))
	b.Add(
		c.B(-40, cistern, -48, -36, cistern+1.2, -44, c.Stucco),   // the dry fountain
		c.B(-39, cistern+1.2, -47, -37, cistern+3, -45, c.Stucco), //   and the founder's statue
		p.Crate(-52, cistern, -40, 2, 2, 2), p.Crate(-38, cistern, -52, 2, 1.5, 2),
		p.Crate(-36, cistern, -34, 1.5, 1.5, 1.5),
	)

	// The spawns: the caravan yard's camels (crates) and the accelerator's bean bags.
	b.Add(p.Crate(52, ground, -12, 2, 1.5, 2), p.Crate(52, ground, 10, 2, 2, 2))
	b.Add(p.Crate(-68, ground, 6, 1.5, 1, 1.5), p.Crate(-68, ground, -7.5, 1.5, 1, 1.5))

	b.TeamSpawns(c.Vibe, [3]float64{62, 0, -8}, [3]float64{62, 0, -4}, [3]float64{62, 0, 0},
		[3]float64{62, 0, 4}, [3]float64{62, 0, 8})
	b.TeamSpawns(c.Artisan, [3]float64{-64, 0, -8}, [3]float64{-64, 0, -4}, [3]float64{-64, 0, 0},
		[3]float64{-64, 0, 4}, [3]float64{-64, 0, 8})
	b.BuyAroundSpawns(3)
	b.Site("A", -56, terrace-0.5, 30, -38, terrace+3, 54)
	b.Site("B", -54, cistern-0.5, -54, -34, ground, -34)

	b.Nav("vibe", 60, 0, 0, "nRoad", "eRoad", "sYard").
		// the runway
		Nav("nRoad", 60, 0, 30, "nCorner").
		Nav("nCorner", 60, 0, 43, "rw1").
		NavLane("rw1", 36, 0, 43, "rwArch").
		Nav("rwArch", 18.5, 0, 43, "rw2").
		Nav("rw2", 0, 0, 43, "rwA").
		Nav("rwA", -6, 0, 40, "rwB").
		Nav("rwB", -15, 0, 40, "rwC").
		Nav("rwC", -15, 0, 46, "rw3").
		Nav("rw3", -23, 0, 46, "rwTop").
		Nav("rwTop", -32, terrace, 43, "rwLand").
		Nav("rwLand", -32, terrace, 37, "A").
		Nav("A", -44, terrace, 42).
		// the awning alley
		Nav("eRoad", 48, 0, 6.5, "awnS").
		Nav("awnS", 33, 0, 6.5, "eGate", "awn").
		NavLane("awn", 33, 0, 24, "rw1").
		// the caravanserai
		Nav("eGate", 15, 0, 6.5, "cE").
		Nav("cE", 8, 0, 7, "cN", "cS").
		NavLane("cN", 0, 0, 9, "nLane", "cW").
		Nav("nLane", 0, 0, 30, "rw2").
		Nav("cW", -8, 0, -6.5, "wGate", "cS").
		Nav("wGate", -15, 0, -6.5, "wRoad").
		Nav("wRoad", -26, 0, -6.5, "alley", "wRoadW").
		NavLane("alley", -26, 0, 20, "shortBot").
		Nav("shortBot", -23.5, 0, 31, "shortTop").
		Nav("shortTop", -32, terrace, 31, "rwLand").
		Nav("cS", 3, 0, -8, "sLane").
		Nav("sLane", 0, 0, -26, "sPitTop").
		Nav("sPitTop", 0, 0, -31, "tun").
		NavLane("tun", 0, cistern, -41, "tunX", "tunE").
		Nav("tunX", 0, cistern, -49, "tunW").
		Nav("tunW", -26, cistern, -49, "B").
		Nav("B", -44, cistern, -39).
		// the south yard and the cistern
		Nav("sYard", 56, 0, -28, "sYard2").
		Nav("sYard2", 46, 0, -41, "tunE").
		NavLane("tunE", 33, cistern, -41).
		// the Artisans
		Nav("artisan", -62, 0, 0, "ctN", "ctS", "wRoadW").
		Nav("wRoadW", -50, 0, -6.5).
		Nav("ctN", -54, 0, 14, "ctStair").
		Nav("ctStair", -54, 0, 18, "ctTop").
		Nav("ctTop", -54, terrace, 27.2, "ctLand").
		Nav("ctLand", -48.5, terrace, 27.5, "ctWest").
		Nav("ctWest", -49, terrace, 35.5, "A").
		Nav("ctS", -58, 0, -15, "bal").
		Nav("bal", -55, 0, -22, "bTop").
		Nav("bTop", -55, 0, -27.5, "bBot").
		Nav("bBot", -55, cistern, -35, "B")
	b.Meta("Competitive. A desert market town burning its runway: the long market street to A's rooftop terrace, the cistern under town to B's sunken court, the caravanserai in the middle.", 16)
	b.FillSpawns(true) // one spawn per player, in the same spawn area
	b.Category(c.Competitive)
	return b.Map()
}
