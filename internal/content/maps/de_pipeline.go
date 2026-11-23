package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// DePipeline is a CS-scale CI/CD plant, 140 × 120 m. The Vibe Coders commit
// from the loading docks in the south; the Artisans guard prod in the north.
// Three ways north: the Registry road (A long, east, open), the Pipeline
// itself (mid: a conveyor corridor through the test gate into Merge square)
// and the Cache tunnel (west, roofed, to B). Merge square opens east through
// the Hotfix door onto A (the container yard) and west through Merge
// Conflict into B (the build hall). The Rollback corridor links the tunnel to
// mid. A has a container stack to climb, B a catwalk along its west wall.
func DePipeline() c.Map {
	b := mapgen.New("de_pipeline")
	const wallH, blockH = 10.0, 8.0
	b.Add(mapgen.Arena(-70, -60, 70, 60, wallH, 1, c.Concrete, c.Concrete)...)
	block := func(x0, z0, x1, z1 float64, mat string) { b.Add(p.Wall(x0, z0, x1, z1, blockH, mat)) }
	cont := func(x, y, z float64, alongX, long bool, colour int) {
		b.Add(p.Container(x, y, z, alongX, long, colour))
	}
	const ch = p.ContainerH

	// The solid blocks between the routes.
	block(-70, -38, -60, 0, c.Brick)                       // west of the Cache tunnel
	block(-52, -38, -6, -24, c.Brick)                      // build farm, south of Rollback
	block(-52, -20, -6, 0, c.Brick)                        // build farm, north of Rollback
	block(6, -38, 52, 0, c.Brick)                          // the server barn between mid and A long
	block(-28, 14, -4, 40, c.Plaster)                      // offices west of Approval
	block(4, 14, 28, 40, c.Plaster)                        // offices east of Approval
	b.Add(p.Slab(-60, -32, -52, -4, 3.8, 0.4, c.Concrete)) // the tunnel's roof

	// Loading docks (Vibe Coders' spawn): container rows for cover.
	cont(-30, 0, -58, true, true, 0)
	cont(-30, ch, -58, true, false, 1)
	cont(18, 0, -58, true, true, 2)
	cont(-22, 0, -50, false, false, 3)
	cont(20, 0, -51, false, false, 1)
	cont(48, 0, -57, true, false, 0)
	cont(-55, 0, -57, true, false, 2)
	b.Add(p.Crate(-12, 0, -46, 2, 1.5, 2), p.Crate(10, 0, -47, 2, 2, 2))
	// a long container across the dock front: the spawn can't see up mid into prod
	cont(-6, 0, -47.5, true, true, 2)

	// A long (the Registry road): sparse cover, a long sightline.
	cont(64, 0, -34, false, false, 1)
	cont(54, 0, -24, false, false, 2)
	// two containers across the top of the road: no sightline from the yard down it
	cont(52, 0, -8, true, false, 1)
	cont(64, 0, -2, true, false, 0)
	b.Add(p.Crate(65, 0, -14, 2, 2, 2))

	// The Pipeline (mid): a conveyor down the east side, the test gate at z -12.
	b.Add(c.B(2.6, 0, -36, 4.6, 0.8, -14.5, c.Hazard))
	b.Add(p.WallGaps(true, -12, -6, 6, 0.8, 0, blockH, c.Metal, p.Gap{From: -2.5, To: 2.5, H: 3.5})...)
	b.Add(p.Crate(-5.5, 0, -6, 1.5, 1.5, 1.5))

	// Cache tunnel (west) and Rollback (tunnel <-> mid).
	b.Add(p.Crate(-59.5, 0, -14, 1.5, 1.5, 1.5), p.Crate(-53.5, 0, -28, 1.5, 1.5, 1.5))
	b.Add(p.Crate(-30, 0, -24, 1.5, 1.2, 1.0))

	// Merge square (the hub), its doors into B and A.
	b.Add(p.WallGaps(false, -28, 0, 14, 2, 0, blockH, c.Concrete, p.Gap{From: 4, To: 9, H: 3.5})...) // Merge Conflict
	b.Add(p.WallGaps(false, 26, 0, 14, 2, 0, blockH, c.Concrete, p.Gap{From: 5, To: 10, H: 3.5})...) // Hotfix
	cont(-14, 0, 1, true, false, 0)
	cont(9, 0, 10.5, true, false, 3)
	b.Add(p.Crate(2, 0, 11, 2, 2, 2))
	b.Add(p.CratePyramid(16, 1, 1.5)...)

	// B: the build hall. South wall (the tunnel comes in), north wall (two CT doors).
	b.Add(p.WallGaps(true, 0, -70, -28, 1, 0, 7, c.Concrete, p.Gap{From: -60, To: -52})...)
	b.Add(p.WallGaps(true, 40, -70, -28, 1, 0, 7, c.Concrete, p.Gap{From: -62, To: -57, H: 3.5})...)
	// Build runners (machines) and conveyors on site B.
	b.Add(
		c.B(-58, 0, 22, -54, 2.4, 25, c.Metal),
		c.B(-46, 0, 28, -43, 2.4, 31, c.Metal),
		c.B(-48, 0, 31, -42, 0.9, 32.5, c.Hazard),
		c.B(-62, 0, 16, -58, 0.9, 17.5, c.Hazard),
		c.B(-38, 0, 8, -34, 2.4, 12, c.Metal), // blocks the site's view of Merge Conflict
	)
	b.Add(p.Crate(-36, 0, 30, 2, 2, 2), p.Crate(-36, 2, 30.25, 1.5, 1.5, 1.5))
	b.Add(p.Crate(-44, 0, 23, 2, 1.5, 2), p.Crate(-55, 0, 21, 2, 1.5, 2)) // post-plant cover
	// The catwalk along the west wall, 3 m up, stairs at its south end.
	const cw = 3.0
	b.Add(p.Slab(-70, 9, -65, 38, cw, 0.3, c.Metal))
	b.Add(p.Railing(-65.3, 9, -65, 38, cw, c.Hazard))
	b.Add(p.Flight(true, 9, -70, -66, 0, 0.8, 0.5, 6, -1, c.Metal)...)
	b.Add(c.B(-66, 0, 20, -65, cw-0.3, 21, c.Concrete), c.B(-66, 0, 32, -65, cw-0.3, 33, c.Concrete))
	// North of the hall: the artifact store (Artisan side).
	cont(-68, 0, 52, true, false, 1)
	cont(-50, 0, 52, true, true, 3)
	b.Add(p.Crate(-44, 0, 44, 2, 2, 2))

	// A: the container yard. CT wall with the yard gate.
	b.Add(p.WallGaps(false, 28, 40, 60, 1, 0, blockH, c.Concrete, p.Gap{From: 44, To: 52})...)
	// Stacks on and around the site.
	cont(43, 0, 24, true, false, 0)
	cont(43, ch, 24, true, false, 2)
	cont(53, 0, 33, false, false, 1)
	cont(36, 0, 30, false, false, 3)
	cont(62, 0, 14, true, false, 2)
	cont(44, 0, 4, false, true, 1)
	cont(34, 0, 18, true, false, 0)
	// The stack: two-high containers in the north-east corner, stairs up the side.
	cont(60, 0, 36, true, false, 3)
	cont(60, 0, 38.6, true, false, 0)
	cont(60, ch, 38.6, true, false, 1)
	b.Add(p.Flight(true, 36, 60, 62, 0, 0.8, 0.52, 5, -1, c.Metal)...)
	b.Add(p.Flight(false, 60, 38.8, 40.8, ch, 0.8, 0.52, 5, -1, c.Metal)...)
	cont(48, 0, 50, true, true, 2)
	b.Add(p.Crate(36, 0, 55, 2, 2, 2), p.Crate(64, 0, 52, 2, 2, 2))

	// Prod (the Artisans' spawn), and Approval (prod <-> Merge square).
	b.Add(p.WallGaps(true, 30, -4, 4, 0.8, 0, blockH, c.Plaster, p.Gap{From: -2, To: 2, H: 3.5})...)
	b.Add(p.Crate(-16, 0, 44, 2, 2, 2), p.Crate(14, 0, 45, 2, 2, 2))
	b.Add(p.DeskPod(-12, 0, 56)...)
	b.Add(p.DeskPod(12, 0, 56)...)
	// a long container in front of prod's desks: no sightline down Approval and mid
	cont(-6, 0, 43, true, true, 0)

	b.Item("codereview", 0, 0, -40)

	b.TeamSpawns(c.Vibe, [3]float64{-6, 0, -55}, [3]float64{-3, 0, -52}, [3]float64{0, 0, -55},
		[3]float64{3, 0, -52}, [3]float64{6, 0, -55})
	b.TeamSpawns(c.Artisan, [3]float64{-6, 0, 51}, [3]float64{-3, 0, 48}, [3]float64{0, 0, 51},
		[3]float64{3, 0, 48}, [3]float64{6, 0, 51})
	b.BuyAroundSpawns(3)
	b.Site("A", 42, -0.5, 25, 60, 3, 40)
	b.Site("B", -60, -0.5, 19, -42, 3, 36)

	// Nav: the three ways north (lanes), the hub, both sites, prod.
	b.Nav("vibe", 0, 0, -53, "dockW", "dockE").
		Nav("dockW", -9, 0, -51, "tS").
		Nav("dockE", 8.5, 0, -51, "tSE").
		Nav("tS", -9, 0, -43, "tW", "mid0", "tSE").
		Nav("tSE", 8.5, 0, -43, "tE", "mid0").
		Nav("tE", 40, 0, -43, "eS").
		Nav("eS", 61, 0, -42, "long").
		NavLane("long", 61, 0, -20, "eN").
		Nav("eN", 60, 0, -2, "aLong").
		Nav("aLong", 57, 0, 8, "A").
		Nav("tW", -40, 0, -43, "wS").
		Nav("wS", -56, 0, -42, "tunnel").
		NavLane("tunnel", -56, 0, -30, "rbW", "wN").
		Nav("rbW", -56, 0, -22, "wN", "rbE").
		Nav("rbE", -3, 0, -22, "mid").
		Nav("wN", -56, 0, -2, "bS").
		Nav("bS", -56, 0, 4, "B").
		Nav("mid0", 0, 0, -36, "mid").
		NavLane("mid", 0, 0, -18, "gate").
		Nav("gate", 0, 0, -11.6, "hub").
		Nav("hub", 0, 0, 6, "hubE", "hubW", "app").
		Nav("hubE", 22, 0, 7.5, "hotfix").
		Nav("hotfix", 30, 0, 7.5, "aShort").
		Nav("aShort", 38, 0, 13, "A").
		Nav("hubW", -22, 0, 6.5, "merge").
		Nav("merge", -31, 0, 6.5, "bEast").
		Nav("bEast", -36, 0, 15, "B").
		Nav("A", 50, 0, 30).
		Nav("B", -50, 0, 25).
		Nav("artisan", 0, 0, 50, "ctE", "ctW", "ctMid").
		Nav("ctMid", -8.5, 0, 46.5, "ctMidS").
		Nav("ctMidS", -7.5, 0, 41.5, "appN").
		Nav("appN", -2, 0, 41.5, "app").
		Nav("app", 0, 0, 28, "hub").
		Nav("ctE", 22, 0, 48, "ctA").
		Nav("ctA", 34, 0, 48, "A").
		Nav("ctW", -22, 0, 48, "ctB").
		Nav("ctB", -59.5, 0, 46, "bNorth").
		Nav("bNorth", -59.5, 0, 37, "B")
	b.Meta("CS-scale CI/CD plant: a container yard (A), a build hall (B), the Pipeline down mid; Registry road long, Cache tunnel west.", 16)
	b.FillSpawns(true) // one spawn per player, in the same spawn area
	b.Category(c.Competitive)
	return b.Map()
}
