package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeLegacy is a competitive-size old town (116 × 118 m) around the crumbling
// monolith nobody dares refactor. The Vibe Coders come up from the south
// street (z -60..-48), the Artisans hold from the north square behind the
// founder's statue. Three ways in:
//   - long A: the west road (x -58..-46), through the long doors and up the
//     steps onto A's raised plaza (y 2);
//   - mid: the courtyard round the monolith (x -20..20, z -16..10), then west
//     down the alley and up the short stairs onto A, or east through the
//     B link; a crack from the alley flanks onto long A, and CT mid runs north
//     to the Artisans' square;
//   - B main: the east road (x 46..58) and the roofed tunnel into B's square.
//
// Everything between the routes is solid old town: sun-baked stucco and brick.
func DeLegacy() c.Map {
	b := mapgen.New("de_legacy")
	b.Add(mapgen.Arena(-58, -60, 58, 58, 9, 1, c.Sand, c.Stucco)...)
	block := func(x0, z0, x1, z1, h float64) { b.Add(c.B(x0, 0, z0, x1, h, z1, c.Stucco)) }
	brick := func(x0, z0, x1, z1, h float64) { b.Add(c.B(x0, 0, z0, x1, h, z1, c.Brick)) }
	const up = 2.0 // A's plaza

	// The old town between the routes.
	block(-46, -48, -5, -16, 7) // south-west quarter
	brick(-46, -16, -20, -8, 6) // south of the alley
	block(-46, -8, -34, -6, 6)  // the crack's south side
	block(-46, -3, -34, 12, 7)  // between long A and the short stairs
	block(-26, 0, -20, 10, 6)   // east of the short stairs
	block(-26, 10, -24, 12, 6)  //
	block(-24, 10, -4, 40, 10)  // the old hall (between A and CT mid)
	block(-24, 40, -16, 44, 8)  //
	block(-24, 50, -16, 58, 8)  //
	brick(5, -48, 46, -16, 7)   // south-east quarter
	block(20, -16, 46, -6, 6)   // south of the B link
	block(34, -6, 46, 12, 7)    // between the B link and B main
	block(20, 2, 26, 10, 6)     //
	block(24, 10, 26, 12, 6)    //
	brick(4, 10, 24, 40, 10)    // the archive (between CT mid and B)
	block(16, 40, 24, 44, 8)    //
	block(16, 50, 24, 58, 8)    //

	// Mid: the monolith, crumbling from the top, and the bits that fell off.
	b.Add(
		c.B(-6, 0, -6, 6, 9, 2, c.Concrete),
		c.B(-5, 9, -5, 3, 13, 1, c.Concrete),
		c.B(-4, 13, -4, 0, 15.5, -1, c.Concrete),
		c.B(9, 0, -13, 13, 1.4, -10, c.Concrete),
		c.B(-15, 0, 3, -12, 1.1, 7, c.Concrete),
		c.B(13, 0, 3, 14.5, 2.2, 4.5, c.Concrete),
	)
	b.Add(p.Crate(-18, 0, -15, 2, 2, 2), p.Crate(16, 0, 6.5, 1.5, 1.5, 1.5))
	// CT mid: a crate halfway up, the founder's statue at the top.
	b.Add(p.Crate(1.5, 0, 24, 2, 1.5, 2), c.B(-4, 0, 46, 4, 1.2, 49, c.Stucco), c.B(-1, 1.2, 46.8, 1, 4, 48.2, c.Stucco))

	// Long A: crates up the road, a broken column, the doors, the steps.
	b.Add(
		p.Crate(-58, 0, -32, 2, 2, 2), p.Crate(-58, 0, -30, 2, 1.2, 1.5),
		p.Pillar(-50, -14, 1.2, 4, c.Stucco),
		p.Crate(-48, 0, -2, 2, 1.5, 2),
	)
	b.Add(p.WallGaps(true, 5, -58, -46, 1, 0, 7, c.Stucco, p.Gap{From: -54, To: -49, H: 3.5})...)
	b.Add(p.Flight(true, 12, -58, -46, 0, 0.8, 0.4, 5, -1, c.Stucco)...)
	// Short A: the alley, the stairs onto the ledge, a low wall at the top.
	b.Add(p.Crate(-24, 0, -7.5, 1.5, 1.5, 1.5))
	b.Add(p.Flight(true, 4, -34, -26, 0, 0.8, 0.4, 5, -1, c.Stucco)...)
	b.Add(p.Slab(-34, 4, -26, 12, up, up, c.Stucco), c.B(-34, up, 9, -31, up+1, 9.5, c.Stucco))

	// A: the raised plaza, its cover, and the back steps down to the square.
	b.Add(p.Slab(-58, 12, -24, 58, up, up, c.Stucco))
	b.Add(p.Flight(false, -24, 44, 50, 0, 0.8, 0.4, 5, 1, c.Stucco)...)
	b.Add(
		p.Crate(-50, up, 34, 2, 2, 2), p.Crate(-48, up, 34, 1.5, 1.5, 1.5),
		c.B(-40, up, 44, -37, up+4, 47, c.Stucco), // a fallen shard of the monolith
		c.B(-56, up, 22, -52, up+1.1, 23, c.Stucco),
		p.Crate(-35, up, 30, 2, 1.2, 2),
		c.B(-30, up, 52, -26, up+1.1, 53, c.Stucco),
		c.B(-38, up, 38, -31, up+2.6, 39, c.Stucco), // a broken wall: cover from the back steps
	)

	// B main: crates up the road, then the tunnel (a roof over z 0..10).
	b.Add(p.Crate(56, 0, -36, 2, 2, 2), p.Crate(46, 0, -24, 1.5, 1.5, 1.5), p.Crate(55.5, 0, -8, 2.5, 1.2, 2))
	b.Add(c.B(46, 3.4, 0, 58, 7, 10, c.Stucco))
	// The B link: a pillar narrows it at the top.
	b.Add(p.Pillar(26, 9, 1.2, 4, c.Stucco))
	// B: the dead mainframe in the middle, crates round it.
	b.Add(
		c.B(40, 0, 34, 44, 2.5, 40, c.Metal),
		p.Crate(48, 0, 30, 2, 2, 2), p.Crate(50, 0, 30, 1.5, 1.2, 1.5),
		p.Crate(36, 0, 44, 2, 1.5, 2),
		c.B(52, 0, 44, 58, 1.1, 45, c.Stucco),
		p.Crate(28, 0, 22, 1.5, 1.5, 1.5),
		c.B(31, 0, 36, 36, 2.6, 37, c.Stucco), // a broken wall: cover from the back passage
	)
	// The south street: crates round the Vibe Coders' bus stop.
	b.Add(p.Crate(-14, 0, -50, 2, 1.5, 2), p.Crate(13, 0, -51, 1.5, 1.5, 1.5), p.Crate(-36, 0, -57, 2, 2, 2), p.Crate(34, 0, -50, 2, 1.2, 2))

	b.TeamSpawns(c.Vibe, [3]float64{-8, 0, -55}, [3]float64{-4, 0, -55}, [3]float64{0, 0, -55},
		[3]float64{4, 0, -55}, [3]float64{8, 0, -55})
	b.TeamSpawns(c.Artisan, [3]float64{-8, 0, 54}, [3]float64{-4, 0, 54}, [3]float64{0, 0, 54},
		[3]float64{4, 0, 54}, [3]float64{8, 0, 54})
	b.BuyAroundSpawns(3)
	b.Site("A", -52, up-0.5, 30, -34, up+3, 50)
	b.Site("B", 34, -0.5, 26, 54, 3, 46)

	b.Nav("vibe", 0, 0, -54, "tw", "te", "ms").
		Nav("tw", -52, 0, -54, "la1").
		NavLane("la1", -52, 0, -20, "laC").
		Nav("laC", -52, 0, -4.5, "laDoor", "crack").
		Nav("laDoor", -51.5, 0, 7, "laTop").
		Nav("laTop", -51.5, up, 14, "aLong").
		Nav("aLong", -47, up, 26, "A").
		Nav("A", -43, up, 40).
		Nav("ms", 0, 0, -30, "mpS").
		Nav("mpS", 0, 0, -11, "mpW", "mpE").
		Nav("mpW", -13, 0, -1, "alley", "mpN").
		Nav("alley", -28, 0, -4, "crack", "shortBot").
		NavLane("crack", -40, 0, -4.5).
		Nav("shortBot", -30, 0, -3.5, "shortTop").
		NavLane("shortTop", -30, up, 7, "aShort").
		Nav("aShort", -30, up, 16, "A").
		Nav("mpE", 13, 0, -1, "link", "mpN").
		NavLane("link", 27, 0, -2, "linkN").
		Nav("linkN", 30, 0, 6, "bShort").
		Nav("bShort", 31, 0, 18, "B").
		Nav("B", 44, 0, 30).
		Nav("te", 52, 0, -54, "bm1").
		NavLane("bm1", 52, 0, -20, "bm2").
		Nav("bm2", 52, 0, 5, "bMain").
		Nav("bMain", 50, 0, 18, "B").
		Nav("mpN", 0, 0, 7, "ctm").
		Nav("ctm", 0, 0, 30, "ctmN").
		Nav("ctmN", 0, 0, 42, "ctW", "ctE").
		Nav("artisan", 0, 0, 53, "ctW", "ctE").
		Nav("ctW", -12, 0, 47, "aBack").
		Nav("aBack", -18, 0, 47, "aBackTop").
		Nav("aBackTop", -28, up, 47, "A").
		Nav("ctE", 12, 0, 47, "bBack").
		Nav("bBack", 21, 0, 47, "bBackIn").
		Nav("bBackIn", 30, 0, 45, "B")
	b.Meta("Competitive. A sun-baked old town round a crumbling monolith: long A road, short A stairs, the mid courtyard, the B tunnel.", 10)
	b.Category(c.Competitive)
	return b.Map()
}
