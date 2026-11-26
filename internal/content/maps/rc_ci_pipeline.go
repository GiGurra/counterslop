package maps

import (
	"math"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// RcCiPipeline is the first race track (Slop n Roll Racing, docs/racing.md):
// a figure eight of floating roads on pillars over the void, a motherboard
// glowing far below (the data sea). Seen from above (x right, z down) it is
// driven:
//
//   - the start straight west along z 0 (the grid at its east end, the
//     start/finish arch at x 31), under the flyover;
//   - a banked left-hander (the berms rise on the outside) into the west
//     leg, south: a ramp and an 8 m gap to jump;
//   - a flat left-hander into the south leg, east: a narrow bridge (7 m) on
//     the way;
//   - a left-hander north into the flyover leg: up a ramp onto the bridge
//     over the start straight (the crossover) and down the other side;
//   - a right-hander east along the north leg, a banked right-hander south
//     down the east leg, and a right-hander west onto the start straight.
//
// Every road is 12 m wide (the bridge 7) at y 0 (the flyover's deck 7 m up),
// edged with low rails (0.7 m: they stop a car that runs along them, a jump
// or a bad line goes over them). Off the track is the void: a pit, a wreck.
// Six gates: the start/finish line, the west leg, the narrow bridge, the
// flyover, the north leg, the east leg; a wreck comes back on the last one
// passed.
func RcCiPipeline() c.Map {
	const (
		hw   = 6.0  // half a road's width
		rail = 0.7  // rail height: over a car's step (0.6), under its roof
		rt   = 0.4  // rail thickness
		fly  = 7.0  // the flyover's deck
		bh   = 3.5  // half the narrow bridge's width
		ramp = 2.2  // the jump's lip
		sea  = -48. // the data sea's top (drawing only: the pit is far above it)
	)
	b := mapgen.New("rc_ci_pipeline").Mode(c.ModeRace)
	road := func(x0, z0, x1, z1 float64) { b.Add(c.B(x0, -1, z0, x1, 0, z1, c.Concrete)) }
	// railX: a rail along x (from x0 to x1) on the z = z edge (outside: side -1 / +1 along z).
	railX := func(x0, x1, z, side, y float64) {
		if side < 0 {
			b.Add(c.B(x0, y, z-rt, x1, y+rail, z, c.Hazard))
		} else {
			b.Add(c.B(x0, y, z, x1, y+rail, z+rt, c.Hazard))
		}
	}
	railZ := func(z0, z1, x, side, y float64) {
		if side < 0 {
			b.Add(c.B(x-rt, y, z0, x, y+rail, z1, c.Hazard))
		} else {
			b.Add(c.B(x, y, z0, x+rt, y+rail, z1, c.Hazard))
		}
	}
	pillar := func(x, z, top float64) { b.Add(c.B(x-1, -30, z-1, x+1, top, z+1, c.Metal)) }
	// arch: a gate's posts beside the road and its banner over it (y: the road's surface).
	archX := func(x, z0, z1, y float64) { // across a road along x
		b.Add(c.B(x-0.3, y+0.05, z0-1.1, x+0.3, y+5, z0-rt-0.05, c.Metal), c.B(x-0.3, y+0.05, z1+rt+0.05, x+0.3, y+5, z1+1.1, c.Metal),
			c.B(x-0.35, y+5, z0-1.15, x+0.35, y+5.6, z1+1.15, c.Neon))
	}
	archZ := func(z, x0, x1, y float64) { // across a road along z
		b.Add(c.B(x0-1.1, y+0.05, z-0.3, x0-rt-0.05, y+5, z+0.3, c.Metal), c.B(x1+rt+0.05, y+0.05, z-0.3, x1+1.1, y+5, z+0.3, c.Metal),
			c.B(x0-1.15, y+5, z-0.35, x1+1.15, y+5.6, z+0.35, c.Neon))
	}

	// The start straight (z -6..6, x -80..80), heading west, under the flyover (x -6..6).
	road(-80, -hw, 80, hw)
	railX(-80, 80, -hw, -1, 0)
	railX(-80, 80, hw, 1, 0)
	// The west hairpin's pad (x -92..-80, z -6..6): banked, the berms rising 2 m outward on the west and north.
	road(-92, -hw, -80, hw)
	b.Add(prefabs.Ramp(-98, -12, -92, hw, 0, 2, c.RampNX, c.Concrete), prefabs.Ramp(-92, -12, -80, -hw, 0, 2, c.RampNZ, c.Concrete))
	// The west leg (x -92..-80), south: flat, the jump's ramp (z 30..44, up to 2.2 m), the gap (z 44..52), the
	// landing and 40 m to brake for the corner.
	road(-92, hw, -80, 30)
	b.Add(prefabs.Ramp(-92, 30, -80, 44, 0, ramp, c.RampPZ, c.Metal))
	road(-92, 52, -80, 92)
	railZ(hw, 44, -92, -1, 0)
	railZ(hw+rt, 44, -80, 1, 0)
	railZ(52, 92, -92, -1, 0)
	railZ(52, 80-rt, -80, 1, 0)
	// The south-west corner (x -92..-80, z 80..92): its outside rail is the south leg's.
	// The south leg (z 80..92), east: road to x -60, the narrow bridge (z 82.5..89.5) to -30, road to the corner.
	road(-80, 80, -60, 92)
	b.Add(c.B(-60, -1, 86-bh, -30, 0, 86+bh, c.Metal))
	road(-30, 80, -6, 92)
	railX(-80, -60, 80, -1, 0)
	railX(-60, -30, 86-bh, -1, 0)
	railX(-30, -6, 80, -1, 0)
	railX(-92, -60, 92, 1, 0)
	railX(-60, -30, 86+bh, 1, 0)
	railX(-30, 6, 92, 1, 0)
	// The south-east corner (x -6..6, z 80..92) and the flyover leg (x -6..6), north: flat to z 50,
	// up a ramp to the deck at 7 m (z 16), the deck over the start straight (to z -16), down to z -50, flat on.
	road(-hw, 50, hw, 92)
	railZ(50, 92, hw, 1, 0)
	railZ(50, 80-rt, -hw, -1, 0)
	b.Add(prefabs.Ramp(-hw, 16, hw, 50, 0, fly, c.RampNZ, c.Concrete))
	b.Add(c.B(-hw, fly-1, -16, hw, fly, 16, c.Metal))
	b.Add(prefabs.Ramp(-hw, -50, hw, -16, 0, fly, c.RampPZ, c.Concrete))
	road(-hw, -92, hw, -50)
	for _, x := range [][2]float64{{-hw - rt, -hw}, {hw, hw + rt}} { // the deck's rails, and the ramps' (their slope, a rail higher)
		b.Add(c.B(x[0], fly, -16, x[1], fly+rail, 16, c.Hazard))
		b.Add(c.W(x[0], rail, 16, x[1], fly+rail, 50, c.Hazard, c.RampNZ))
		b.Add(c.W(x[0], rail, -50, x[1], fly+rail, -16, c.Hazard, c.RampPZ))
	}
	railZ(-92, -50, -hw, -1, 0)
	railZ(-80+rt, -50, hw, 1, 0)
	railX(-hw, hw, -92, -1, 0)
	// The north leg (z -92..-80), east, to the banked north-east corner (x 80..92).
	road(hw, -92, 80, -80)
	railX(hw, 80, -80, 1, 0)
	road(80, -92, 92, -80)
	b.Add(prefabs.Ramp(80, -98, 92, -92, 0, 2, c.RampNZ, c.Concrete), prefabs.Ramp(92, -98, 98, -80, 0, 2, c.RampPX, c.Concrete))
	railX(hw, 80, -92, -1, 0)
	// The east leg (x 80..92), south, to the last corner (z -6..6) onto the start straight.
	road(80, -80, 92, hw)
	railZ(-80+rt, -hw-rt, 80, -1, 0)
	railZ(-80, hw, 92, 1, 0)
	railX(80, 92, hw, 1, 0)
	// Pillars under the roads, and the flyover's.
	for _, p := range [][3]float64{{-60, 0, -1}, {-30, 0, -1}, {30, 0, -1}, {60, 0, -1}, {-86, 20, -1}, {-86, 70, -1},
		{-70, 86, -1}, {-20, 86, -1}, {0, 70, -1}, {0, -70, -1}, {30, -86, -1}, {60, -86, -1}, {86, -50, -1}, {86, -20, -1},
		{-4, 10, fly - 1}, {4, 10, fly - 1}, {-4, -10, fly - 1}, {4, -10, fly - 1}} {
		pillar(p[0], p[1], p[2])
	}
	// The data sea: a motherboard far below (falling to it is long over: the pit is 12 m down).
	b.Add(c.B(-260, sea-2, -260, 260, sea, 260, c.PCB))
	b.Pit("void", -400, sea-50, -400, 400, -12, 400)

	// The gates, in driving order: the start/finish line, the west leg, the
	// narrow bridge, the flyover's deck, the north leg, the east leg.
	west, south, east, north := math.Pi/2, math.Pi, -math.Pi/2, 0.0 // yaw facing -x, +z, +x, -z
	gate := func(name string, x0, y0, z0, x1, y1, z1, sx, sy, sz, yaw float64) {
		b.Checkpoint(c.Checkpoint{Name: name, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}, Spawn: [3]float64{sx, sy, sz}, Yaw: yaw})
	}
	gate("start", 30, -1, -hw, 32, 5, hw, 24, 0, 0, west)
	archX(31, -hw, hw, 0)
	gate("the jump", -92, -1, 8, -80, 5, 10, -86, 0, 12, south)
	archZ(9, -92, -80, 0)
	gate("the narrow bridge", -46, -1, 80, -44, 5, 92, -38, 0, 86, east)
	archX(-45, 86-bh, 86+bh, 0)
	gate("the flyover", -hw, fly-0.5, -1, hw, fly+5, 1, 0, fly, -6, north)
	archZ(0, -hw, hw, fly)
	gate("the north leg", 40, -1, -92, 42, 5, -80, 48, 0, -86, east)
	archX(41, -92, -80, 0)
	gate("the east leg", 80, -1, -40, 92, 5, -38, 86, 0, -32, south)
	archZ(-39, 80, 92, 0)

	// The grid: eight slots two by two behind the line, facing west.
	for i := range 8 {
		x := 40 + float64(i/2)*7
		z := -3.0
		if i%2 == 1 {
			z = 3
		}
		b.Car("buggy", x, 0, z, west)
	}
	return b.
		TeamSpawns(c.Vibe, [3]float64{70, 0, -4.5}, [3]float64{70, 0, -1.5}, [3]float64{70, 0, 1.5}, [3]float64{70, 0, 4.5}).
		TeamSpawns(c.Artisan, [3]float64{75, 0, -4.5}, [3]float64{75, 0, -1.5}, [3]float64{75, 0, 1.5}, [3]float64{75, 0, 4.5}).
		BuyAroundSpawns(2).
		Meta("CI Pipeline: a figure eight of floating roads over the data sea: a banked hairpin, a jump over a gap, a narrow bridge, a flyover over the start straight. Low rails, a long way down.", 8).
		Category(c.Racing).
		Map()
}
