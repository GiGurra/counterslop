package maps

import (
	"math"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// RcMergeTrain is the second race track (Slop n Roll Racing, docs/racing.md):
// a rectangle of roads paved with diffs on pillars over a molten sea of merge
// conflicts, the middle leg diving under the start straight. Seen from above
// (x right, z down) it is driven:
//
//   - the start straight west along z 0 (the grid at its east end, the line at
//     x 60), over the underpass;
//   - a banked left-hander (berms west and north) into the west leg, south:
//     26 m to straighten up, a ramp and an 8 m gap to jump;
//   - a flat left-hander into the south leg, east: a narrow bridge (7 m);
//   - a left-hander north into the middle leg: down into a trench, under the
//     start straight (the underpass, 6 m down) and up again;
//   - a banked right-hander east into the merge leg: the road splits round a
//     slot of void into two 4.5 m lanes and merges again;
//   - a banked right-hander south down the east leg, a right-hander west onto
//     the start straight.
//
// Roads 12 m wide (the bridge 7), low hazard rails; off the track is the void
// (a wreck). Six gates: the line, the jump, the bridge, the underpass, the
// merge, the east leg.
func RcMergeTrain() c.Map {
	const (
		hw   = 6.0  // half a road's width
		bh   = 3.5  // half the narrow bridge's width
		ramp = 2.2  // the jump's lip
		dive = -6.0 // the underpass's road
		sea  = -60. // the conflict sea's top (drawing only: the pit is far above it)
		rt   = kitRT
	)
	b := mapgen.New("rc_merge_train").Mode(c.ModeRace)
	b.Fog(0.0045, [3]float64{0.38, 0.13, 0.08}) // smoke off the melt: a red-brown haze
	k := &raceKit{b: b, road: c.Diff, rail: c.Hazard, post: c.Metal, banner: c.RaceGate}
	west, south, east, north := math.Pi/2, math.Pi, -math.Pi/2, 0.0 // yaw facing -x, +z, +x, -z

	// The start straight (x -130..130), its pit lane on the north side (x 14..58, as on rc_ci_pipeline).
	k.deck(-130, -hw, 130, hw, 0)
	k.railX(-130, 14, -hw, -1, 0)
	k.railX(20, 52, -hw, -1, 0)
	k.railX(58, 130, -hw, -1, 0)
	k.railX(-130, 142, hw, 1, 0) // and the last corner's
	k.deck(14, -13, 58, -hw, 0)
	b.Add(c.B(20, 0, -10, 52, 0.02, -9.6, c.RaceGate)) // the pit lane's line
	k.railX(14-rt, 58+rt, -13, -1, 0)
	k.railZ(-13, -hw-rt, 14, -1, 0)
	k.railZ(-13, -hw-rt, 58, 1, 0)
	b.PitLane(20, -1, -13, 52, 4, -hw-rt)

	// The west hairpin (x -142..-130, z -6..6): banked, berms west and north.
	k.deck(-142, -hw, -130, hw, 0)
	b.Add(prefabs.Ramp(-148, -12, -142, hw, 0, 2, c.RampNX, c.Diff), prefabs.Ramp(-142, -12, -130, -hw, 0, 2, c.RampNZ, c.Diff))
	// The west leg (x -142..-130), south: 26 m out of the hairpin to straighten up, the jump's
	// ramp (z 32..44), the gap (44..52), the landing.
	k.deck(-142, hw, -130, 32, 0)
	b.Add(prefabs.Ramp(-142, 32, -130, 44, 0, ramp, c.RampPZ, c.Metal))
	k.deck(-142, 52, -130, 86, 0) // and the south-west corner (z 74..86)
	k.railZ(hw, 44, -142, -1, 0)
	k.railZ(hw+rt, 44, -130, 1, 0)
	k.railZ(52, 86, -142, -1, 0)
	k.railZ(52, 74-rt, -130, 1, 0)
	// The south leg (z 74..86), east: the narrow bridge (z 76.5..83.5) from x -100 to -60.
	k.deck(-130, 74, -100, 86, 0)
	k.deck(-100, 80-bh, -60, 80+bh, 0)
	k.deck(-60, 74, -8, 86, 0) // and the south-east corner (x -20..-8)
	k.railX(-130, -100, 74, -1, 0)
	k.railX(-100, -60, 80-bh, -1, 0)
	k.railX(-60, -20, 74, -1, 0)
	k.railX(-142, -100, 86, 1, 0)
	k.railX(-100, -60, 80+bh, 1, 0)
	k.railX(-60, -8+rt, 86, 1, 0)
	// The middle leg (x -20..-8), north: flat to z 36, down a ramp into a trench to the
	// underpass (z -12..12, 6 m down, under the start straight), up again at z -36, flat on.
	k.deck(-20, 36, -8, 74, 0)
	k.railZ(36, 74-rt, -20, -1, 0)
	k.railZ(36, 86, -8, 1, 0)
	b.Add(prefabs.Ramp(-20, 12, -8, 36, dive, -dive, c.RampPZ, c.Diff))
	k.deck(-20, -12, -8, 12, dive)
	b.Add(prefabs.Ramp(-20, -36, -8, -12, dive, -dive, c.RampNZ, c.Diff))
	for _, x := range [][2]float64{{-20 - rt, -20}, {-8, -8 + rt}} { // the trench's walls (under the straight: up to its deck)
		b.Add(c.B(x[0], dive, hw+rt, x[1], kitRail, 36, c.Metal), c.B(x[0], dive, -36, x[1], kitRail, -hw-rt, c.Metal), c.B(x[0], dive, -hw-rt, x[1], -1, hw+rt, c.Metal))
	}
	k.deck(-20, -90, -8, -36, 0)
	k.railZ(-90, -36, -20, -1, 0)
	k.railZ(-90+rt, -36, -8, 1, 0)
	// The north-west corner (x -20..-8, z -102..-90): banked, berms north and west.
	k.deck(-20, -102, -8, -90, 0)
	b.Add(prefabs.Ramp(-20, -108, -8, -102, 0, 2, c.RampNZ, c.Diff), prefabs.Ramp(-26, -108, -20, -90, 0, 2, c.RampNX, c.Diff))
	// The merge leg (z -102..-90), east: from x 30 to 70 the road splits round a 3 m slot of
	// void into two lanes (z -102..-97.5 and -94.5..-90), merging again.
	k.deck(-8, -102, 30, -90, 0)
	k.deck(30, -102, 70, -97.5, 0)
	k.deck(30, -94.5, 70, -90, 0)
	k.deck(70, -102, 130, -90, 0)
	k.railX(-8, 130, -102, -1, 0)
	k.railX(-8, 130-rt, -90, 1, 0)
	k.railX(31, 69, -97.5, 1, 0)
	k.railX(31, 69, -94.5, -1, 0)
	// The north-east corner (x 130..142): banked, berms north and east; the east leg south.
	k.deck(130, -102, 142, -90, 0)
	b.Add(prefabs.Ramp(130, -108, 142, -102, 0, 2, c.RampNZ, c.Diff), prefabs.Ramp(142, -108, 148, -90, 0, 2, c.RampPX, c.Diff))
	k.deck(130, -90, 142, hw, 0) // and the last corner (z -6..6)
	k.railZ(-90, hw, 142, 1, 0)
	k.railZ(-90, -hw-rt, 130, -1, 0)

	// Pillars down into the void (none in the trench, the gap or the slot).
	for _, p := range [][3]float64{{-110, 0, -1}, {-70, 0, -1}, {-40, 0, -1}, {30, 0, -1}, {75, 0, -1}, {110, 0, -1}, {-136, 0, -1}, {-136, 16, -1},
		{-136, 62, -1}, {-136, 80, -1}, {-115, 80, -1}, {-80, 80, -1}, {-40, 80, -1}, {-14, 80, -1}, {-14, 55, -1}, {-14, 0, dive - 1},
		{-14, -60, -1}, {-14, -96, -1}, {10, -96, -1}, {50, -99.75, -1}, {50, -92.25, -1}, {100, -96, -1}, {136, -96, -1}, {136, -50, -1}, {136, 0, -1}} {
		k.pillar(p[0], p[1], p[2])
	}
	// The conflict sea far below; falling is over long before (the pit's top 14 m down).
	b.Add(c.B(-300, sea-2, -300, 300, sea, 300, c.Conflict))
	b.Pit("void", -400, sea-50, -400, 400, -14, 400)

	// The gates, in driving order.
	k.gate("start", 59, -1, -hw, 61, 5, hw, 53, 0, 0, west)
	k.archX(60, -hw, hw, 0)
	k.gate("the jump", -142, -1, 58, -130, 5, 60, -136, 0, 64, south)
	k.archZ(59, -142, -130, 0)
	k.gate("the narrow bridge", -81, -1, 80-bh, -79, 5, 80+bh, -52, 0, 80, east)
	k.archX(-80, 80-bh, 80+bh, 0)
	k.gate("the underpass", -20, -1, -41, -8, 5, -39, -14, 0, -46, north)
	k.archZ(-40, -20, -8, 0)
	k.gate("the merge", 79, -1, -102, 81, 5, -90, 88, 0, -96, east)
	k.archX(80, -102, -90, 0)
	k.gate("the east leg", 130, -1, -41, 142, 5, -39, 136, 0, -33, south)
	k.archZ(-40, 130, 142, 0)

	// Signs: chevrons outside every corner, conflict markers at the split and over the underpass.
	k.sign(-149.75, 2, -5, -149.5, 5, c.Chevron)
	k.sign(-141, 0, 87.5, -131, 87.75, c.Chevron)
	k.sign(-6.5, 0, 75, -6.25, 85, c.Chevron)
	k.sign(-19, 2, -109.75, -9, -109.5, c.ChevronR)
	k.sign(149.5, 2, -101, 149.75, -91, c.ChevronR)
	k.sign(131, 0, 7.5, 141, 7.75, c.ChevronR)
	k.sign(20, 0, -104.75, 30, -104.5, c.ConflictSign)
	k.sign(-30, 0, 8, -20, 8.25, c.ConflictSign)
	k.sign(-8, 0, 8, 2, 8.25, c.ConflictSign)

	// The bots' racing line, from the grid: wide into a corner, the apex, out wide;
	// the merge in the south lane.
	b.RaceLine(
		[3]float64{66, 0, 0}, [3]float64{40, 0, 0}, [3]float64{0, 0, 0}, [3]float64{-60, 0, 0}, [3]float64{-108, 0, -2.5},
		[3]float64{-124, 0, -3}, [3]float64{-133, 0, 1}, [3]float64{-137, 0, 9}, [3]float64{-136, 0, 20}, // the west hairpin
		[3]float64{-136, 0, 30}, [3]float64{-136, 1.1, 38}, [3]float64{-136, 2.2, 43.5}, [3]float64{-136, 0, 56}, [3]float64{-136, 0, 64}, // the jump
		[3]float64{-137, 0, 71}, [3]float64{-134, 0, 79}, [3]float64{-126, 0, 80.5}, [3]float64{-112, 0, 80}, // the south-west corner
		[3]float64{-100, 0, 80}, [3]float64{-80, 0, 80}, [3]float64{-60, 0, 80}, [3]float64{-40, 0, 80}, // the narrow bridge
		[3]float64{-26, 0, 80.5}, [3]float64{-16, 0, 78}, [3]float64{-13.5, 0, 70}, [3]float64{-14, 0, 50}, // the south-east corner
		[3]float64{-14, 0, 36}, [3]float64{-14, -3, 24}, [3]float64{-14, -6, 12}, [3]float64{-14, -6, 0}, [3]float64{-14, -6, -12},
		[3]float64{-14, -3, -24}, [3]float64{-14, 0, -36}, [3]float64{-14, 0, -60}, [3]float64{-14, 0, -78}, // the underpass
		[3]float64{-14.5, 0, -88}, [3]float64{-12, 0, -94}, [3]float64{-4, 0, -96.5}, [3]float64{10, 0, -95}, // the north-west corner
		[3]float64{22, 0, -92.5}, [3]float64{32, 0, -92.25}, [3]float64{50, 0, -92.25}, [3]float64{68, 0, -92.25}, [3]float64{82, 0, -94}, // the merge
		[3]float64{100, 0, -96}, [3]float64{118, 0, -96.5}, [3]float64{130, 0, -97}, [3]float64{135, 0, -93}, [3]float64{136.5, 0, -84}, // the north-east berm
		[3]float64{136, 0, -60}, [3]float64{136, 0, -30}, [3]float64{136.5, 0, -13}, [3]float64{134, 0, -1.5}, [3]float64{126, 0, 0}, [3]float64{100, 0, 0}, // the last corner
	)

	// The grid: eight slots two by two behind the line, facing west.
	for i := range 8 {
		z := -3.0
		if i%2 == 1 {
			z = 3
		}
		b.Car("buggy", 68+float64(i/2)*7, 0, z, west)
	}
	return b.
		TeamSpawns(c.Vibe, [3]float64{105, 0, -4.5}, [3]float64{105, 0, -1.5}, [3]float64{105, 0, 1.5}, [3]float64{105, 0, 4.5}).
		TeamSpawns(c.Artisan, [3]float64{110, 0, -4.5}, [3]float64{110, 0, -1.5}, [3]float64{110, 0, 1.5}, [3]float64{110, 0, 4.5}).
		BuyAroundSpawns(2).
		Meta("Merge Train: diffs paved over a sea of molten merge conflicts. A banked hairpin, a jump, a narrow bridge, an underpass diving under the start straight, and the merge: two lanes round a slot of void. Low rails.", 8).
		Category(c.Racing).
		Map()
}
