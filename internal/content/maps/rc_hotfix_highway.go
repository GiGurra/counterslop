package maps

import (
	"math"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// RcHotfixHighway is the third race track (Slop n Roll Racing,
// docs/racing.md): an elevated highway at dusk, a city's lights far below,
// the longest of the three (about 1.1 km a lap). Seen from above (x right,
// z down) it is driven:
//
//   - the start straight west along z 0 (the grid at its east end, the line
//     at x 100), under the interchange;
//   - a banked left-hander (berms west and north) into the west leg, south:
//     a narrow bridge (7 m) for 36 m;
//   - a flat left-hander into the south leg, east: a ramp and an 8 m gap;
//   - a left-hander north up the interchange: a flyover 8 m over the start
//     straight and down again;
//   - a banked right-hander east into the road works: the highway detours
//     north round a closed block (four corners), or, for the brave, straight
//     on up a narrow ramp and over a 9 m gap;
//   - a banked right-hander south down the east leg, a right-hander west onto
//     the start straight.
//
// Roads 12 m wide with a highway's paint, concrete barriers (low: a car that
// hits one at an angle goes over), green gantry signs; off the road is a long
// way down. Six gates: the line, the bridge, the jump, the interchange, the
// road works, the east leg.
func RcHotfixHighway() c.Map {
	const (
		hw   = 6.0  // half a road's width
		bh   = 3.5  // half a narrow road's width (the bridge, the shortcut)
		ramp = 2.2  // a jump's lip
		fly  = 8.0  // the interchange's deck
		city = -70. // the city's roofs (drawing only: the pit is far above them)
		rt   = kitRT
	)
	b := mapgen.New("rc_hotfix_highway").Mode(c.ModeRace).Sky(c.SkyDusk)
	b.Fog(0.0035, [3]float64{0.3, 0.14, 0.24}) // the dusk haze over the city
	k := &raceKit{b: b, road: c.Highway, rail: c.Concrete, post: c.Metal, banner: c.RaceGate}
	west, south, east, north := math.Pi/2, math.Pi, -math.Pi/2, 0.0 // yaw facing -x, +z, +x, -z

	// The start straight (x -160..160) and its pit lane on the north side (x 30..74).
	k.deck(-160, -hw, 160, hw, 0)
	k.railX(-160, 30, -hw, -1, 0)
	k.railX(36, 68, -hw, -1, 0)
	k.railX(74, 160, -hw, -1, 0)
	k.railX(-160, 172, hw, 1, 0) // and the last corner's
	k.deck(30, -13, 74, -hw, 0)
	b.Add(c.B(36, 0, -10, 68, 0.02, -9.6, c.RaceGate)) // the pit lane's line
	k.railX(30-rt, 74+rt, -13, -1, 0)
	k.railZ(-13, -hw-rt, 30, -1, 0)
	k.railZ(-13, -hw-rt, 74, 1, 0)
	b.PitLane(36, -1, -13, 68, 4, -hw-rt)

	// The west corner (x -172..-160): banked, berms west and north; the west leg south with
	// the narrow bridge (z 40..76); the south-west corner (z 110..122).
	k.deck(-172, -hw, -160, 40, 0)
	b.Add(prefabs.Ramp(-178, -12, -172, hw, 0, 2, c.RampNX, c.Highway), prefabs.Ramp(-172, -12, -160, -hw, 0, 2, c.RampNZ, c.Highway))
	k.deck(-166-bh, 40, -166+bh, 76, 0)
	k.deck(-172, 76, -160, 122, 0)
	k.railZ(hw, 40, -172, -1, 0)
	k.railZ(hw+rt, 40, -160, 1, 0)
	k.railZ(40, 76, -166-bh, -1, 0)
	k.railZ(40, 76, -166+bh, 1, 0)
	k.railZ(76, 122, -172, -1, 0)
	k.railZ(76, 110-rt, -160, 1, 0)
	// The south leg (z 110..122), east: the jump's ramp (x -120..-108), the gap (to -100), the
	// landing; the south-east corner (x -40..-28).
	k.deck(-160, 110, -120, 122, 0)
	b.Add(prefabs.Ramp(-120, 110, -108, 122, 0, ramp, c.RampPX, c.Metal))
	k.deck(-100, 110, -28, 122, 0)
	k.railX(-160, -108, 110, -1, 0)
	k.railX(-100, -40, 110, -1, 0)
	k.railX(-172, -108, 122, 1, 0)
	k.railX(-100, -28+rt, 122, 1, 0)
	// The interchange (x -40..-28), north: flat to z 56, up to the deck 8 m over the start
	// straight (z -20..20), down at z -56, flat on to the north-west corner.
	k.deck(-40, 56, -28, 110, 0)
	k.railZ(56, 110-rt, -40, -1, 0)
	k.railZ(56, 122, -28, 1, 0)
	b.Add(prefabs.Ramp(-40, 20, -28, 56, 0, fly, c.RampNZ, c.Highway))
	k.deck(-40, -20, -28, 20, fly)
	b.Add(prefabs.Ramp(-40, -56, -28, -20, 0, fly, c.RampPZ, c.Highway))
	for _, x := range [][2]float64{{-40 - rt, -40}, {-28, -28 + rt}} { // the deck's barriers, and the ramps' (a barrier higher)
		b.Add(c.B(x[0], fly, -20, x[1], fly+kitRail, 20, c.Concrete))
		b.Add(c.W(x[0], kitRail, 20, x[1], fly+kitRail, 56, c.Concrete, c.RampNZ))
		b.Add(c.W(x[0], kitRail, -56, x[1], fly+kitRail, -20, c.Concrete, c.RampPZ))
	}
	k.deck(-40, -100, -28, -56, 0)
	k.railZ(-100, -56, -40, -1, 0)
	k.railZ(-100+rt, -56, -28, 1, 0)
	// The north-west corner (x -40..-28, z -112..-100): banked, berms north and west.
	k.deck(-40, -112, -28, -100, 0)
	b.Add(prefabs.Ramp(-40, -118, -28, -112, 0, 2, c.RampNZ, c.Highway), prefabs.Ramp(-46, -118, -40, -100, 0, 2, c.RampNX, c.Highway))
	// The road works (z -112..-100): at x 20..32 the highway turns north round the closed block
	// (up x 20..32 to z -162, east along z -162..-150, down x 80..92) and on east from x 92.
	// The shortcut: straight on from x 32, a narrow ramp (to x 44), a 9 m gap, a narrow landing.
	k.deck(-28, -112, 32, -100, 0)
	k.deck(20, -150, 32, -112, 0)
	k.deck(20, -162, 92, -150, 0)
	k.deck(80, -150, 92, -100, 0)
	k.deck(92, -112, 172, -100, 0) // and the north-east corner (x 160..172)
	b.Add(prefabs.Ramp(32, -106-bh, 44, -106+bh, 0, ramp, c.RampPX, c.Metal))
	k.deck(53, -106-bh, 80, -106+bh, 0)
	k.railX(-28, 20-rt, -112, -1, 0)
	k.railX(-28, 32, -100, 1, 0)
	k.railZ(-112, -106-bh, 32, 1, 0)
	k.railZ(-106+bh, -100, 32, 1, 0)
	k.railX(32+rt, 44, -106-bh, -1, 0)
	k.railX(32+rt, 44, -106+bh, 1, 0)
	k.railX(53, 80-rt, -106-bh, -1, 0)
	k.railX(53, 80-rt, -106+bh, 1, 0)
	k.railZ(-162, -112, 20, -1, 0)
	k.railZ(-150, -112, 32, 1, 0)
	k.railX(20, 92, -162, -1, 0)
	k.railX(32+rt, 80-rt, -150, 1, 0)
	k.railZ(-150, -112, 80, -1, 0)
	k.railZ(-112, -106-bh, 80, -1, 0)
	k.railZ(-106+bh, -100, 80, -1, 0)
	k.railZ(-162, -112, 92, 1, 0)
	k.railX(80, 160-rt, -100, 1, 0)
	k.railX(92+rt, 160, -112, -1, 0)
	// The north-east corner: banked, berms north and east; the east leg south to the last corner.
	b.Add(prefabs.Ramp(160, -118, 172, -112, 0, 2, c.RampNZ, c.Highway), prefabs.Ramp(172, -118, 178, -100, 0, 2, c.RampPX, c.Highway))
	k.deck(160, -100, 172, hw, 0)
	k.railZ(-100, hw, 172, 1, 0)
	k.railZ(-100, -hw-rt, 160, -1, 0)

	// Pillars down into the void (and the interchange's, beside the start straight).
	for _, p := range [][3]float64{{-130, 0, -1}, {-90, 0, -1}, {-60, 0, -1}, {0, 0, -1}, {20, 0, -1}, {90, 0, -1}, {130, 0, -1},
		{-166, 0, -1}, {-166, 25, -1}, {-166, 58, -1}, {-166, 95, -1}, {-166, 116, -1}, {-140, 116, -1}, {-80, 116, -1}, {-55, 116, -1},
		{-34, 116, -1}, {-34, 85, -1}, {-38, 12, fly - 1}, {-30, 12, fly - 1}, {-38, -12, fly - 1}, {-30, -12, fly - 1}, {-34, -78, -1},
		{-34, -106, -1}, {-5, -106, -1}, {26, -106, -1}, {26, -131, -1}, {26, -156, -1}, {56, -156, -1}, {86, -156, -1}, {86, -131, -1},
		{66, -106, -1}, {86, -106, -1}, {120, -106, -1}, {166, -106, -1}, {166, -70, -1}, {166, -35, -1}, {166, 0, -1}} {
		k.pillar(p[0], p[1], p[2])
	}
	// The city far below at dusk; a fall is over long before (the pit's top 12 m down).
	b.Add(c.B(-400, city-2, -400, 400, city, 400, c.NightCity))
	b.Pit("void", -500, city-50, -500, 500, -12, 500)

	// The gates, in driving order.
	k.gate("start", 99, -1, -hw, 101, 5, hw, 93, 0, 0, west)
	k.archX(100, -hw, hw, 0)
	k.gate("the narrow bridge", -172, -1, 85, -160, 5, 87, -166, 0, 92, south)
	k.archZ(86, -172, -160, 0)
	k.gate("the jump", -81, -1, 110, -79, 5, 122, -72, 0, 116, east)
	k.archX(-80, 110, 122, 0)
	k.gate("the interchange", -40, fly-0.5, -1, -28, fly+5, 1, -34, fly, -8, north)
	k.archZ(0, -40, -28, fly)
	k.gate("the road works", 99, -1, -112, 101, 5, -100, 108, 0, -106, east)
	k.archX(100, -112, -100, 0)
	k.gate("the east leg", 160, -1, -41, 172, 5, -39, 166, 0, -33, south)
	k.archZ(-40, 160, 172, 0)

	// Signs: chevrons outside every corner (the road works' too), green gantries over the road.
	k.sign(-179.75, 2, -5, -179.5, 5, c.Chevron)
	k.sign(-171, 0, 123.5, -161, 123.75, c.Chevron)
	k.sign(-26.5, 0, 111, -26.25, 121, c.Chevron)
	k.sign(-39, 2, -119.75, -29, -119.5, c.ChevronR)
	k.sign(21, 0, -98.5, 31, -98.25, c.Chevron)
	k.sign(21, 0, -163.75, 31, -163.5, c.ChevronR)
	k.sign(93.5, 0, -161, 93.75, -151, c.ChevronR)
	k.sign(81, 0, -98.5, 91, -98.25, c.Chevron)
	k.sign(179.5, 2, -111, 179.75, -101, c.ChevronR)
	k.sign(161, 0, 7.5, 171, 7.75, c.ChevronR)
	gantryX := func(x, z0, z1 float64) { // a sign board over a road along x, on two posts
		b.Add(c.B(x-0.3, 0.05, z0-1.6, x+0.3, 7.7, z0-1, c.Metal), c.B(x-0.3, 0.05, z1+1, x+0.3, 7.7, z1+1.6, c.Metal),
			c.B(x-0.15, 5.5, z0-1.3, x+0.15, 7.5, z1+1.3, c.ExitSign))
	}
	gantryX(-100, -hw, hw)
	gantryX(0, -112, -100)
	b.Add(c.B(158.4, 0.05, -70.3, 159, 7.7, -69.7, c.Metal), c.B(173, 0.05, -70.3, 173.6, 7.7, -69.7, c.Metal),
		c.B(158.7, 5.5, -70.15, 173.3, 7.5, -69.85, c.ExitSign))

	// The bots' racing line, from the grid (round the road works, not over them).
	b.RaceLine(
		[3]float64{110, 0, 0}, [3]float64{80, 0, 0}, [3]float64{20, 0, 0}, [3]float64{-60, 0, 0}, [3]float64{-130, 0, -2.5},
		[3]float64{-150, 0, -3}, [3]float64{-163, 0, 1}, [3]float64{-167, 0, 9}, [3]float64{-166, 0, 24}, // the west corner
		[3]float64{-166, 0, 40}, [3]float64{-166, 0, 58}, [3]float64{-166, 0, 76}, [3]float64{-166, 0, 96}, // the narrow bridge
		[3]float64{-167, 0, 107}, [3]float64{-164, 0, 115}, [3]float64{-156, 0, 116.5}, [3]float64{-140, 0, 116}, // the south-west corner
		[3]float64{-125, 0, 116}, [3]float64{-114, 1.1, 116}, [3]float64{-108.5, 2.2, 116}, [3]float64{-90, 0, 116}, [3]float64{-70, 0, 116}, // the jump
		[3]float64{-55, 0, 116}, [3]float64{-46, 0, 116.5}, [3]float64{-36, 0, 114}, [3]float64{-33.5, 0, 106}, [3]float64{-34, 0, 85}, // the south-east corner
		[3]float64{-34, 0, 60}, [3]float64{-34, 4, 38}, [3]float64{-34, fly, 20}, [3]float64{-34, fly, 0}, [3]float64{-34, fly, -20},
		[3]float64{-34, 4, -38}, [3]float64{-34, 0, -56}, [3]float64{-34, 0, -80}, // the interchange
		[3]float64{-34.5, 0, -96}, [3]float64{-32, 0, -104}, [3]float64{-24, 0, -106.5}, [3]float64{-5, 0, -106}, [3]float64{10, 0, -106}, // the north-west corner
		[3]float64{17, 0, -106.5}, [3]float64{24, 0, -109}, [3]float64{26.5, 0, -117}, [3]float64{26, 0, -132}, [3]float64{25.5, 0, -146}, // the road works
		[3]float64{27, 0, -154}, [3]float64{34, 0, -156.5}, [3]float64{55, 0, -156}, [3]float64{74, 0, -156}, [3]float64{82, 0, -155},
		[3]float64{86.5, 0, -149}, [3]float64{86, 0, -132}, [3]float64{85.5, 0, -118}, [3]float64{87, 0, -110}, [3]float64{94, 0, -106},
		[3]float64{120, 0, -106}, [3]float64{145, 0, -106.5}, [3]float64{158, 0, -107}, [3]float64{165, 0, -103}, [3]float64{166.5, 0, -94}, // the north-east berm
		[3]float64{166, 0, -70}, [3]float64{166, 0, -40}, [3]float64{166.5, 0, -13}, [3]float64{164, 0, -1.5}, [3]float64{156, 0, 0}, [3]float64{130, 0, 0}, // the last corner
	)

	// The grid: eight slots two by two behind the line, facing west.
	for i := range 8 {
		z := -3.0
		if i%2 == 1 {
			z = 3
		}
		b.Car("buggy", 108+float64(i/2)*7, 0, z, west)
	}
	return b.
		TeamSpawns(c.Vibe, [3]float64{142, 0, -4.5}, [3]float64{142, 0, -1.5}, [3]float64{142, 0, 1.5}, [3]float64{142, 0, 4.5}).
		TeamSpawns(c.Artisan, [3]float64{147, 0, -4.5}, [3]float64{147, 0, -1.5}, [3]float64{147, 0, 1.5}, [3]float64{147, 0, 4.5}).
		BuyAroundSpawns(2).
		Meta("Hotfix Highway: an elevated highway at dusk over the city's lights, the longest track. A banked corner, a narrow bridge, a jump, the interchange flying over the start straight, and the road works: round the closed block, or straight over the gap for the brave.", 8).
		Category(c.Racing).
		Map()
}
