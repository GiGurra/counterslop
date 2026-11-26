package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeHostileTakeover: a seed-stage startup (the Vibe Coders) takes over
// Megacorp's container ship MV Synergy (the Artisans) by boarding it. The
// startup's quay in the north (y 0) faces the ship across a 20 m basin: an
// 80 m hull rising 8.6 m out of the water, its deck (y 8) stacked with
// containers, the bridge at the stern. Sites: A on the foredeck, B on the
// afterdeck under the bridge; the Artisans spawn midships between them.
//
// The way up on foot (and the bots' only way: they ignore boats) is the
// loading pier in the west: a ramp up to a gangway tower at the bow, straight
// into A. The rest is the hostile part: take a dinghy (E at the quay's
// edge) across to the accommodation ladder's pontoon and run up the hull's
// side into B, or let the boat come to rest anywhere against the hull and
// hook it from the seat (the Gravity Override is on sale here: its Rules),
// walk up the side, then hook the deck (or a container) to come aboard.
func DeHostileTakeover() c.Map {
	const (
		surf  = -0.6 // the water's surface: the quay's edge 0.6 m over it
		floor = -6.0 // the harbour's floor
		deck  = 8.0  // the ship's deck
		sx0   = -40.0
		sx1   = 40.0 // the hull's length (bow in the west)
		sz0   = -30.0
		sz1   = -10.0 // its beam (the quay side is sz1)
	)
	b := mapgen.New("de_hostile_takeover")
	b.Add(
		c.B(-61, floor-1, -46, 61, floor, 10, c.Concrete), // the harbour floor
		c.B(-60, floor, 10, 60, 0, 40, c.Concrete),        // the startup's quay
		c.B(-61, 0, 40, 61, 5, 41, c.Brick),               // its back wall
		c.B(-61, floor, -46, -60, 5, 40, c.Brick),         // the basin's sides
		c.B(60, floor, -46, 61, 5, 40, c.Brick),
		c.B(-60, floor, -46, 60, 5, -45, c.Brick),
		// MV Synergy: the hull (painted corrugated steel) under a diamond-plate deck.
		c.B(sx0, floor, sz0, sx1, deck-0.2, sz1, c.Container),
		c.B(sx0, deck-0.2, sz0, sx1, deck, sz1, c.Metal),
		// The bridge at the stern, its funnel, and the bow's forecastle rail.
		c.B(30, deck, sz0, sx1, deck+10, sz1, c.Plaster),
		c.B(31, deck+10, -22, sx1, deck+10.4, -18, c.Glass), // the bridge windows' visor
		c.B(34, deck+10, -27, 38, deck+16, -23, c.Hazard),   // the funnel
		c.B(sx0, deck, sz0, sx0+0.5, deck+1, -18, c.Metal),  // the bow rail (the gangway comes in north of it)
		// Rails along the deck's sides (gaps for the accommodation ladder's
		// landing; the bridge closes the stern): nobody strafes overboard.
		c.B(sx0+0.5, deck, sz0, 30, deck+1, sz0+0.3, c.Metal),
		c.B(sx0, deck, sz1-0.3, 18, deck+1, sz1, c.Metal),
		c.B(21, deck, sz1-0.3, 30, deck+1, sz1, c.Metal),
		// The loading pier: a ramp up its length to a gangway tower at the bow.
		c.B(-52, floor, -8, -46, 0, 10, c.Concrete),
		prefabs.Ramp(-51.7, -8, -46.3, 8, 0, deck, c.RampNZ, c.Metal), // 26.6 degrees: walkable
		c.B(-52, floor, -16, sx0, deck, -8, c.Concrete),               // the tower and its gangway to the deck
		c.B(-52, deck, -16, -51.6, deck+1, -8, c.Metal),               // its rails
		c.B(-51.6, deck, -16, sx0, deck+1, -15.7, c.Metal),
		c.B(-52, 0, -8, -51.7, deck+1, 8, c.Concrete), // the ramp's parapets
		c.B(-46.3, 0, -8, -46, deck+1, 8, c.Concrete),
		// The accommodation ladder down the hull's quay side (the boarding
		// party's way up from a dinghy): a pontoon at the waterline (hop out of
		// the boat onto it, or water-jump onto it), a ramp along the hull up to
		// a landing at deck height by B.
		c.B(-3, floor, sz1, 4, 0, sz1+2.5, c.Metal),
		c.B(4, floor, sz1, 18, 0, sz1+1.6, c.Metal),
		prefabs.Ramp(4, sz1, 18, sz1+1.6, 0, deck, c.RampPX, c.Metal), // 29.7 degrees
		c.B(18, floor, sz1, 21, deck, sz1+1.6, c.Metal),
	)
	b.Water(-60, floor, -45, 60, surf, 10)
	// Dinghies moored along the quay, bows to the ship.
	b.Boat(-30, surf, 8, 0).Boat(-14, surf, 8, 0).Boat(0.5, surf, 8, 0).Boat(24, surf, 8, 0) // the third lines up with the ladder's pontoon

	// Deck cargo: a two-high row of 40 ft boxes athwartships on each side of
	// midships (the Artisans' spawn), lanes open along both rails; single
	// boxes and crates for cover on the sites.
	row := func(x float64, colour int) {
		b.Add(prefabs.Container(x, deck, -27, false, true, colour), prefabs.Container(x, deck+prefabs.ContainerH, -27, false, true, colour+1))
	}
	row(-18, 0)
	row(10, 2)
	b.Add(
		prefabs.Container(-34, deck, -26, true, false, 1), // A
		prefabs.Container(-30, deck, -20, false, false, 3),
		prefabs.Crate(-25, deck, -15, 1.2, 1.2, 1.2),
		prefabs.Crate(-36, deck, -14.5, 1.2, 1.2, 1.2),
		prefabs.Container(17, deck, -27, true, false, 2), // B
		prefabs.Container(26, deck, -22, false, false, 0),
		prefabs.Crate(-7, deck, -25, 1.2, 1.2, 1.2), // midships, off the lanes
		prefabs.Crate(5, deck, -16.5, 1.2, 1.2, 1.2),
	)
	// The quay: a crane's legs, cargo waiting to go aboard.
	b.Add(
		c.B(-8, 0, 12, -7, 14, 13, c.Hazard), c.B(8, 0, 12, 9, 14, 13, c.Hazard), c.B(-8, 14, 12, 9, 15, 13, c.Hazard),
		prefabs.Container(-24, 0, 16, true, true, 1),
		prefabs.Container(14, 0, 16, true, true, 3),
		prefabs.Container(30, 0, 22, false, false, 0),
	)
	b.Fog(0.004, [3]float64{0.62, 0.7, 0.78}) // sea haze

	b.TeamSpawns(c.Vibe, [3]float64{-38, 0, 30}, [3]float64{-35, 0, 30}, [3]float64{-32, 0, 30},
		[3]float64{-36.5, 0, 33}, [3]float64{-33.5, 0, 33})
	b.TeamSpawns(c.Artisan, [3]float64{-3, deck, -20}, [3]float64{0, deck, -20}, [3]float64{3, deck, -20},
		[3]float64{-1.5, deck, -23}, [3]float64{1.5, deck, -23})
	b.BuyAroundSpawns(3)
	b.Site("A", sx0, deck-0.5, sz0, -20, deck+4, sz1)
	b.Site("B", 13, deck-0.5, sz0, 30, deck+4, sz1)

	b.Nav("vibe", -35, 0, 28, "quayW").
		Nav("quayW", -49, 0, 14, "rampTop"). // north of the edge: the turn onto the pier on the quay
		Nav("rampTop", -49, deck, -11, "bow").
		Nav("bow", -38, deck, -12, "A").
		Nav("A", -29, deck, -13, "fwdN").
		Nav("fwdN", -14, deck, -12.5, "midN").
		Nav("midN", 0, deck, -12.5, "aftN", "artisan").
		Nav("aftN", 14, deck, -12.5, "B").
		Nav("B", 20, deck, -14).
		Nav("artisan", 0, deck, -18, "midS").
		Nav("midS", 0, deck, -28.5, "fwdS", "aftS").
		Nav("fwdS", -21, deck, -28.5, "A").
		Nav("aftS", 15, deck, -28.5, "B")
	b.Rule("mp_gravityboots", "1") // boarding from the water: hook the hull
	b.Meta("A startup boards Megacorp's container ship MV Synergy: up the loading pier to the bow, or by dinghy and grappling hook up the hull (the Gravity Override is on sale here).", 10)
	b.FillSpawns(true)
	b.Category(c.Competitive)
	return b.Map()
}
