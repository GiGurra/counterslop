package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// DeRackmount is the inside of one server in the rack, scaled up until the
// RAM is cover: the motherboard is the floor, the next board up the ceiling,
// the rack's side panels the long walls, two case fans the size of a house
// the ends. Its Rules sell the Gravity Override, and the map is built for it:
// every wall and the ceiling has its own cover (RAM sticks plugged in
// sideways, cable looms, hot-swap drive sleds that slide in and out), so a
// flank goes over the blade-server rows and across the ceiling while the
// floor fights it out in three lanes. Bots stay on the floor.
func DeRackmount() c.Map {
	const (
		x0, x1 = -36.0, 36.0 // inside the case
		z0, z1 = -20.0, 20.0
		top    = 16.0
	)
	b := mapgen.New("de_rackmount")
	b.Add(
		c.B(x0-1, -1, z0-1, x1+1, 0, z1+1, c.PCB),      // the motherboard
		c.B(x0-1, top, z0-1, x1+1, top+1, z1+1, c.PCB), // the next server's board, upside down
		c.B(x0-1, 0, z0-1, x1+1, top, z0, c.Rack),      // the rack's side panels
		c.B(x0-1, 0, z1, x1+1, top, z1+1, c.Rack),
		c.B(x0-1, 0, z0, x0, top, z1, c.Fan), // intake (the Vibe Coders' end)
		c.B(x1, 0, z0, x1+1, top, z1, c.Fan), // exhaust (the Artisans' end)
	)

	// Blade servers: two rows of 4 m chassis split the floor into three lanes
	// (north z 8..20, mid, south z -20..-8), with a gap at each end of mid.
	for _, z := range [][2]float64{{6, 8}, {-8, -6}} {
		for _, x := range [][2]float64{{-24, -8}, {-5, 9}, {12, 24}} {
			b.Add(c.B(x[0], 0, z[0], x[1], 4, z[1], c.Rack))
		}
	}

	// Mid: the CPU under a finned heatsink, its fan on top (a perch up 6.6 m:
	// reach it with the hook, or from the blade rows' tops).
	b.Add(c.B(-4, 0, -3, 4, 1, 3, c.Metal))
	for i := range 10 {
		x := -3.95 + float64(i)*0.85
		b.Add(c.B(x, 1, -3, x+0.3, 6, 3, c.Metal))
	}
	b.Add(c.B(-4, 6, -3, 4, 6.6, 3, c.Fan))

	// RAM sticks standing in their slots: A's run along x, B's along z, two
	// more in the lanes; capacitors (fat cans) by the spawns and in mid.
	ram := func(xa, za, xb, zb, y0, y1 float64) { b.Add(c.B(xa, y0, za, xb, y1, zb, c.RAM)) }
	ram(14, 12, 19, 12.4, 0, 3)
	ram(14, 16, 19, 16.4, 0, 3)
	ram(21, 14, 26, 14.4, 0, 3)
	ram(15, -18, 15.4, -13, 0, 3)
	ram(19, -16, 19.4, -11, 0, 3)
	ram(23, -18, 23.4, -13, 0, 3)
	ram(-14, 13, -9, 13.4, 0, 3)
	ram(-4, -14.4, 1, -14, 0, 3)
	for _, cp := range [][2]float64{{-31, 15}, {-31, -16.6}, {30, 15}, {30, -16.6}, {-12, 3}, {14, -3}, {-20, -15}, {6, 16}} {
		b.Add(c.B(cp[0], 0, cp[1], cp[0]+1.6, 2.2, cp[1]+1.6, c.Metal))
	}

	// The walls' and the ceiling's own cover: RAM plugged in sideways (3 m
	// out of the side panels), sticks hanging 3 m from the ceiling.
	ram(-20, z1-3, -15, z1, 6, 6.4)
	ram(-2, z1-3, 3, z1, 10, 10.4)
	ram(14, z1-3, 19, z1, 7, 7.4)
	ram(-18, z0, -13, z0+3, 9, 9.4)
	ram(0, z0, 5, z0+3, 6, 6.4)
	ram(16, z0, 21, z0+3, 10, 10.4)
	ram(-16, 2, -11, 2.4, top-3, top)
	ram(8, -2.4, 13, -2, top-3, top)
	ram(18, 12, 23, 12.4, top-3, top)
	ram(18, -12.4, 23, -12, top-3, top)
	ram(-23, -0.2, -18, 0.2, top-3, top)

	// Cable looms: along the ceiling's long edges, across it over mid, and up
	// the side panels (hook anchors everywhere, low cover on the ceiling).
	b.Add(
		c.B(x0, top-1, z1-1, x1, top, z1, c.Cable),
		c.B(x0, top-1, z0, x1, top, z0+1, c.Cable),
		c.B(-0.6, top-0.8, z0+1, 0.6, top, z1-1, c.Cable),
		c.B(-10.6, 0, z1-0.8, -9.4, top-1, z1, c.Cable),
		c.B(9.4, 0, z0, 10.6, top-1, z0+0.8, c.Cable),
		c.B(x0, 7.6, -0.6, x0+0.8, 8.4, 0.6, c.Cable), // across each fan's hub
		c.B(x1-0.8, 7.6, -0.6, x1, 8.4, 0.6, c.Cable),
	)

	// Hot-swap drive sleds sliding out of the side panels and back (4..5.5 m
	// up: over the floor's heads; cover that moves for whoever walks the walls).
	sled := func(xa, xb, z, dz, phase float64) {
		za, zb := z, z+1
		if dz > 0 {
			za, zb = z-1, z
		}
		b.Mover(c.Mover{Kind: c.MoverSlide, Mat: c.Rack, Min: [3]float64{xa, 4, za}, Max: [3]float64{xb, 5.5, zb}, Move: [3]float64{0, 0, dz}, Period: 6, Phase: phase})
	}
	sled(-4, 2, z1-1, -3, 0)
	sled(-2, 4, z0+1, 3, 0.5)
	sled(18, 24, z1-1, -3, 0.25)
	sled(-26, -20, z0+1, 3, 0.75)

	// Lights: cool spots from the ceiling board, a green glow off the CPU.
	cool := [3]float64{0.8, 0.92, 1}
	for _, x := range []float64{-26, -10, 10, 26} {
		for _, z := range []float64{-13, 0, 13} {
			if x == -10 && z == 0 || x == 10 && z == 0 {
				continue // the heatsink's own glow lights mid
			}
			l := c.Spot([3]float64{x, top - 0.15, z}, [3]float64{0, -1, 0}, cool, 700, 24, 70)
			l.Inner = 40
			b.Light(l)
			b.Add(c.B(x-0.6, top-0.08, z-0.3, x+0.6, top, z+0.3, c.Lamp))
		}
	}
	b.Light(c.Point([3]float64{0, 7.5, 0}, [3]float64{0.3, 1, 0.45}, 40, 14))
	// The fans' blue glow at each end, over the spawns.
	b.Light(c.Point([3]float64{x0 + 2, 8, 0}, [3]float64{0.35, 0.55, 1}, 90, 20), c.Point([3]float64{x1 - 2, 8, 0}, [3]float64{0.35, 0.55, 1}, 90, 20))
	// Mid-air glow (every wall is somebody's floor: light them all, not just the board).
	for _, x := range []float64{-22, -6, 6, 22} {
		for _, z := range []float64{-12, 12} {
			b.Light(c.Point([3]float64{x, 9, z}, [3]float64{0.75, 0.85, 1}, 110, 22)) // ~1 on a surface 8 m off (lights fall off as 1/d²)
		}
	}
	b.Fog(0.006, [3]float64{0.12, 0.15, 0.2}) // the inside of a case: dark, a little dusty

	b.TeamSpawns(c.Vibe, [3]float64{-32, 0, -4}, [3]float64{-32, 0, -1.5}, [3]float64{-32, 0, 1.5},
		[3]float64{-32, 0, 4}, [3]float64{-30, 0, 0})
	b.TeamSpawns(c.Artisan, [3]float64{32, 0, -4}, [3]float64{32, 0, -1.5}, [3]float64{32, 0, 1.5},
		[3]float64{32, 0, 4}, [3]float64{30, 0, 0})
	b.BuyAroundSpawns(3)
	b.Site("A", 12, -0.5, 8.5, 27, 3, 19.5)
	b.Site("B", 12, -0.5, -19.5, 27, 3, -8.5)

	b.Nav("vibe", -31, 0, 0, "vN", "vS", "vM").
		Nav("vN", -26, 0, 12, "nMid").
		Nav("nMid", -6, 0, 11, "nEast", "gW").
		Nav("nEast", 10, 0, 11, "A").
		Nav("A", 16.5, 0, 10).
		Nav("vS", -26, 0, -12, "sMid").
		Nav("sMid", -6, 0, -11, "sEast", "gW").
		Nav("sEast", 10, 0, -11, "B").
		Nav("B", 17, 0, -10).
		Nav("vM", -20, 0, 0, "gW").
		Nav("gW", -6.5, 0, 0, "mNW", "mSW").
		Nav("mNW", -5.5, 0, 4.5, "mNE"). // round the heatsink
		Nav("mNE", 5.5, 0, 4.5, "gE").
		Nav("mSW", -5.5, 0, -4.5, "mSE").
		Nav("mSE", 5.5, 0, -4.5, "gE").
		Nav("gE", 10.5, 0, 0, "nEast", "sEast", "artisan").
		Nav("artisan", 31, 0, 0, "aN", "aS").
		Nav("aN", 29, 0, 12, "A").
		Nav("aS", 29, 0, -12, "B")
	b.Cut(top)                     // seen from above (the Maps preview, the overview): the lid off
	b.Rule("mp_gravityboots", "1") // the point of the map
	b.Meta("Inside a server, scaled up: RAM sticks for cover, a fan at each end, and every wall is a floor (the Gravity Override is on sale here: hold F).", 10)
	b.FillSpawns(true)
	b.Category(c.Competitive)
	return b.Map()
}
