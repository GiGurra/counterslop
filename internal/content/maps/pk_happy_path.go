package maps

import (
	"math"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// The course's sections, along +x (each a stage of the pipeline; its gate at
// its end). The tests (pk_happy_path_test.go) run every one with the real
// movement code.
const (
	pkLint    = 3.5  // the lint plate's top (after the stepping stones)
	pkIslandA = 9.0  // build: the pad islands' tops
	pkIslandB = 15.0 //
	pkIslandC = 21.0 // build's last island; staging's lifts start here
	pkLift    = 3.0  // each lift's travel (up)
	pkD       = 30.0 // the void's first island (after the lifts)
	pkE       = 27.0 // the void's far island, under the tower (each block 1 m lower)
	pkTower   = 60.0 // the wall run's tower top
	pkF       = 50.0 // canary's last island, the road, the quays
	pkSurf    = 49.4 // the basin's water
)

// PkHappyPath is the parkour course (docs/racing.md "Parkour"): a race on
// foot, point to point, along the happy path of a deployment pipeline,
// floating in space, a fall at any point back on the last gate at once. Ten
// gates, each the end of a section, run east (+x):
//
//   - localhost: the start plate (the team spawns), the start gate;
//   - lint: stepping stones over the void, each half a metre higher;
//   - build: a chain of jump pads: each throws you onto the next pad, up two
//     tiers, the last onto an island;
//   - staging: three lifts out of step, each up 3 m: ride one up, hop to the
//     next at its bottom, up again, onto the void's island;
//   - the void: 10 m gaps between tall floating blocks: jump, hook the next
//     block's face (your gravity turns onto it), walk up and over its top
//     (everyone wears the Gravity Override: the map's rule);
//   - wall run: hook the tower's face (your gravity turns into it), walk up
//     the wall and over its top edge onto the top;
//   - canary: low gravity, long hops down a line of floating slabs;
//   - deploy: the car leg: a station of buggies, a road with a jump;
//   - rollout: the boat leg: dinghies at the quay, across the basin, a water
//     jump out onto the far quay;
//   - prod: the finish.
//
// Nobody hurts anybody (mp_race_combat 0). Bots don't run it (v1).
func PkHappyPath() c.Map {
	east := -math.Pi / 2 // facing +x
	b := mapgen.New("pk_happy_path").Mode(c.ModeRace).OnFoot(true).Sky(c.SkySpace)
	slab := func(x0, z0, x1, z1, top, thick float64, mat string) {
		b.Add(c.B(x0, top-thick, z0, x1, top, z1, mat))
	}
	// gate: a checkpoint across the course at x (a 1 m slice, h high over
	// floor y, z -w..w), its spawn 2 m on, and an arch to see it by: two
	// posts and the banner.
	gate := func(name string, x, y, w, h float64) {
		b.Checkpoint(c.Checkpoint{Name: name, Min: [3]float64{x, y - 1, -w}, Max: [3]float64{x + 1, y + h, w}, Spawn: [3]float64{x + 2, y, 0}, Yaw: east})
		b.Add(c.B(x+0.05, y, -w-0.55, x+0.55, y+h, -w, c.Metal), c.B(x+0.05, y, w, x+0.55, y+h, w+0.55, c.Metal), // posts under the banner
			c.B(x, y+h, -w-0.6, x+0.6, y+h+0.8, w+0.6, c.RaceGate))
	}
	board := func(text string, x, y, z float64) { // a section's name on a board facing back down the course
		b.Add(c.B(x, y, z-4, x+0.3, y+2.2, z+4, c.Lambda))
		b.Sign(c.Sign{Text: text, Pos: [3]float64{x - 0.02, y + 1.1, z}, Face: c.SignNX, Size: 0.7, Mat: c.SignLed})
	}
	pad := func(name string, x, y, z float64, to [3]float64, apex float64) {
		b.Add(c.B(x-1.2, y, z-1.2, x+1.2, y+0.06, z+1.2, c.JumpPad))
		b.JumpPad(name, x, y+0.06, z, 2.4, 2.4, to, apex)
	}

	// localhost: the start plate.
	slab(-30, -10, 8, 10, 0, 2, c.Concrete)
	b.Add(c.B(-30.3, 0, -6, -30, 4.6, 6, c.Lambda)) // the start's board, behind the spawns
	b.Sign(c.Sign{Text: "THE HAPPY PATH\nlocalhost -> prod", Pos: [3]float64{-29.98, 2.6, 0}, Face: c.SignPX, Size: 0.7, Mat: c.SignLed})
	gate("start", 0, 0, 10, 5)

	// lint: six stepping stones, 3 m long, 2.5 m gaps, each 0.5 m higher;
	// then the lint plate.
	for k := range 6 {
		x0 := 10.5 + 5.5*float64(k)
		slab(x0, -2, x0+3, 2, 0.5*float64(k+1), 1, c.LGTM)
	}
	slab(43.5, -6, 64, 6, pkLint, 1.5, c.Concrete)
	board("LINT", 43.6, pkLint, -8.5)
	gate("lint", 56, pkLint, 6, 5)

	// build: the pad chain. Each pad throws you onto the next pad (a chain:
	// landing on it throws you on), the last onto an island long enough to
	// stop on.
	pad("build 1", 62, pkLint, 0, [3]float64{89, pkIslandA + 0.06, 0}, 3)
	slab(78, -5, 94, 5, pkIslandA, 1.5, c.PCB)
	pad("build 2", 89, pkIslandA, 0, [3]float64{121, pkIslandB + 0.06, 0}, 3)
	slab(110, -5, 126, 5, pkIslandB, 1.5, c.PCB)
	pad("build 3", 121, pkIslandB, 0, [3]float64{146, pkIslandC, 0}, 3)
	slab(142, -6, 175, 6, pkIslandC, 1.5, c.PCB)
	board("BUILD", 142.1, pkIslandC, -8.5)
	gate("build", 166, pkIslandC, 6, 5)

	// staging: three lifts (5 x 4 m, 2 m apart), each up pkLift and down
	// again in 6 s, out of step: when one is at its top the next is at its
	// bottom, level with it: jump across then. Then the void's first island.
	for k := range 3 {
		x0 := 177 + 7*float64(k)
		y := pkIslandC + pkLift*float64(k)
		b.Mover(c.Mover{Kind: c.MoverSlide, Mat: c.Metal, Min: [3]float64{x0, y - 1, -2}, Max: [3]float64{x0 + 5, y, 2},
			Move: [3]float64{0, pkLift, 0}, Period: 6, Phase: 0.5 * float64(k%2)})
	}
	slab(198, -6, 230, 6, pkD, 2, c.Lambda)
	board("STAGING", 198.1, pkD, -8.5)
	gate("staging", 222, pkD, 6, 5)

	// the void: 10 m gaps (a running jump flies 4.6) between tall blocks,
	// each 1 m lower: jump off the edge, hook the next block's face, your
	// gravity turns onto it (the boots), walk up it and over onto its top.
	slab(240, -3, 250, 3, pkD-1, 8, c.Lambda)
	slab(260, -3, 270, 3, pkD-2, 8, c.Lambda)
	slab(280, -6, 300, 6, pkE, 8, c.Lambda)
	board("THE VOID", 280.1, pkE, -8.5)
	gate("the void", 290, pkE, 6, 5)

	// wall run: the tower (a rack of servers, 12 x 12 m, 60 m tall) 4 m on.
	// Hook its face: your gravity turns into it; walk up and over the top.
	b.Add(c.B(304, 0, -6, 316, pkTower, 6, c.Rack))
	b.Sign(c.Sign{Text: "WALL RUN\nhook the wall, walk up", Pos: [3]float64{303.98, pkE + 5, 0}, Face: c.SignNX, Size: 0.7, Mat: c.SignLed})
	gate("wall run", 310, pkTower, 6, 5)

	// canary: low gravity (0.3 g, jumps at 0.7) over a line of slabs 10 m
	// long, 6 m apart, each 2 m lower, down to the station island: a running
	// jump off an edge flies about 13 m (one taken a little early, 11).
	b.GravZone("canary", 316, pkF-10, -40, 420, pkTower+40, 40, 0.3, 0.7)
	for k := range 4 {
		x0 := 322 + 16*float64(k)
		slab(x0, -3, x0+10, 3, pkTower-2-2*float64(k), 1, c.Glass)
	}
	slab(386, -10, 404, 10, pkF, 2, c.Concrete)
	board("CANARY", 386.1, pkF, -12.5)
	gate("canary", 388, pkF, 10, 6)

	// deploy: the buggies at their station, the road east, a ramp and a
	// 10 m gap, the landing, the gate, the quay.
	for _, z := range []float64{-7.5, -2.5, 2.5, 7.5} {
		b.Car("buggy", 398, pkF, z, east)
	}
	slab(404, -6, 478, 6, pkF, 1.5, c.Tarmac)
	b.Add(prefabs.Ramp(470, -6, 478, 6, pkF, 1.5, c.RampPX, c.Chevron))
	slab(488, -6, 575, 6, pkF, 1.5, c.Tarmac)
	for _, z := range [][2]float64{{-6.6, -6}, {6, 6.6}} { // low curbs along the road (not over the ramp)
		b.Add(c.B(404, pkF, z[0], 470, pkF+0.5, z[1], c.Curb), c.B(488, pkF, z[0], 575, pkF+0.5, z[1], c.Curb))
	}
	board("DEPLOY", 404.1, pkF, -11)
	gate("deploy", 550, pkF, 7, 6)

	// rollout: the near quay's end is the basin's west wall; the dinghies
	// moored there; the basin east to the far quay (a water jump out).
	slab(575, -16, 700, 16, pkSurf-5, 1, c.Concrete)             // the basin's floor
	b.Add(c.B(574, pkSurf-5, -16, 575, pkF-1.5, 16, c.Concrete)) // its west wall, under the road's end
	b.Water(575, pkSurf-5, -15, 700, pkSurf, 15)
	b.Add(c.B(575, pkSurf-5, -16, 700, pkF+0.8, -15, c.Concrete), c.B(575, pkSurf-5, 15, 700, pkF+0.8, 16, c.Concrete))
	for _, z := range []float64{-8, 0, 8} {
		b.Boat(577.2, pkSurf, z, east)
	}
	slab(700, -16, 760, 16, pkF, 6, c.Concrete)
	board("ROLLOUT", 575.2, pkF, -11.5)
	gate("rollout", 712, pkF, 16, 6)

	// prod: the finish.
	b.Sign(c.Sign{Text: "PROD", Pos: [3]float64{759.68, pkF + 11, 0}, Face: c.SignNX, Size: 2.5, Mat: c.SignLed})
	b.Add(c.B(759.7, pkF, -16, 760, pkF+13, 16, c.Lambda))
	gate("finish", 750, pkF, 16, 8)

	// The void under every section (a fall: back on your last gate at once).
	b.Pit("under localhost and lint", -60, -100, -80, 64, -6, 80)
	b.Pit("under build", 64, -100, -80, 176, 3, 80)
	b.Pit("under staging", 176, -100, -80, 198, 15, 80)
	b.Pit("under the void", 198, -100, -80, 304, pkE-12, 80)
	b.Pit("under the tower", 304.5, -100, -80, 318, pkTower-6, 80) // (inside the tower: its face is walked on)
	b.Pit("under canary", 318, -100, -80, 404, pkF-6, 80)
	b.Pit("under the road", 404, -100, -80, 575, pkF-6, 80)
	b.Pit("under the basin", 575, -100, -80, 700, pkSurf-8, 80)
	b.Pit("under prod", 700, -100, -80, 800, pkF-10, 80)

	var vibe, art [][3]float64
	for i := range 4 {
		z := -7 + 2*float64(i)
		vibe = append(vibe, [3]float64{-24, 0, z}, [3]float64{-20, 0, z})
		art = append(art, [3]float64{-24, 0, z + 8}, [3]float64{-20, 0, z + 8})
	}
	return b.
		TeamSpawns(c.Vibe, vibe...).
		TeamSpawns(c.Artisan, art...).
		BuyAroundSpawns(2).
		Rule("mp_race_combat", "0").
		Rule("mp_gravityboots", "2").
		Rule("sv_hook_recharge", "40"). // the void and the wall run hook four times in a row
		Rule("mp_race_races", "3").
		Meta("The happy path: a parkour race on foot from localhost to prod, over the void. Stepping stones, a chain of jump pads, lifts, hook swings, a wall run up a server tower, low gravity, a car leg and a boat leg. Every gate's split against your best.", 16).
		Category(c.Parkour).
		Map()
}
