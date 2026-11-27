package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// DeServerless is a defusal arena in space with no server under anything:
// floating platforms over the void (a fall is the end of you), jump pads
// that throw you across the map and up to the high tier, an accelerator pad
// over a gap, two moving platforms, and everyone wears the Gravity Override
// (the hook on: mp_gravityboots 2). Seen from above (x right, z down):
//
//   - the spawns: the Vibe's island west ("cold start"), the Artisans' east
//     ("warm start"), each with a bridge and a ramp up to the hub;
//   - the hub (the gateway, 4 m up) in the middle, a bridge and a ramp down
//     south to site B;
//   - site B (cold storage) south, on the spawns' level: from the west the
//     spawns' jump pads, half the map in one throw; or from the west the
//     ferry (a moving platform from the west pier) or the pier's jump pad,
//     from the east the accelerator pad on the east pier, hopping a 10 m gap;
//   - site A (the hot path) north, the high tier (8 m up): only by the jump
//     pads (from the hub, from each spawn, and the long one from B's back,
//     over everything), or the shuttle (a platform sliding between the hub's
//     north edge and A's dock, a ramp up from there). Back down: a pad to the
//     hub, or the shuttle.
//
// The pads draw their arcs; bots fly them: the map has no hand-made graph,
// the server builds one at load (autoNav: navgen's, with the pads' one-way
// links).
func DeServerless() c.Map {
	const (
		hub = 4.0 // the hub's top
		top = 8.0 // site A's
	)
	b := mapgen.New("de_serverless").Sky(c.SkySpace)
	plat := func(x0, z0, x1, z1, y, thick float64) { b.Add(c.B(x0, y-thick, z0, x1, y, z1, c.Lambda)) }
	pad := func(name string, x, y, z float64, to [3]float64, apex float64) {
		b.Add(c.B(x-1.2, y, z-1.2, x+1.2, y+0.06, z+1.2, c.JumpPad))
		b.JumpPad(name, x, y+0.06, z, 2.4, 2.4, to, apex)
	}

	// The spawn islands, their bridges to the hub and their piers south.
	plat(-78, -12, -54, 12, 0, 2)
	plat(54, -12, 78, 12, 0, 2)
	plat(-54, -2, -20, 2, 0, 1)
	plat(20, -2, 54, 2, 0, 1)
	b.Add(prefabs.Ramp(-20, -2, -12, 2, 0, hub, c.RampPX, c.Lambda), prefabs.Ramp(12, -2, 20, 2, 0, hub, c.RampNX, c.Lambda))
	plat(-64, 12, -60, 44, 0, 1) // the west pier, out to the ferry's landing
	plat(-64, 44, -52, 52, 0, 1)
	plat(60, 12, 64, 44, 0, 1) // the east pier, out west to the accelerator
	plat(26, 44, 64, 52, 0, 1)

	// The hub: a load balancer in the middle, crates.
	plat(-12, -12, 12, 12, hub, 2)
	b.Add(c.B(-2, hub, -2, 2, hub+2, 2, c.Lambda))
	b.Add(prefabs.Crate(-10, hub, 3, 1.6, 1.6, 1.6), prefabs.Crate(7.5, hub, -10, 1.6, 1.6, 1.6), prefabs.Crate(-10, hub, -11, 1.4, 1.2, 1.4))
	// The way down to B: a ramp and a bridge.
	b.Add(prefabs.Ramp(-2, 12, 2, 20, 0, hub, c.RampNZ, c.Lambda))
	plat(-2, 20, 2, 40, 0, 1)

	// Site A, the hot path, up high: crates, a block; the dock and its ramp south.
	plat(-16, -64, 16, -44, top, 2)
	b.Add(c.B(3, top, -63, 7, top+1.6, -60, c.Lambda))
	b.Add(prefabs.Crate(-14.5, top, -63, 1.6, 1.4, 1.6), prefabs.Crate(12, top, -48, 1.6, 1.4, 1.6), prefabs.Crate(-13, top, -47, 1.4, 1.2, 1.4))
	b.Add(prefabs.Ramp(-3, -44, 3, -36, hub, top-hub, c.RampNZ, c.Lambda))
	plat(-3, -36, 3, -34, hub, 1)
	b.Site("A", -12, top-0.5, -62, 12, top+3, -46)

	// Site B, cold storage: crates and a rack of cold functions.
	plat(-16, 40, 16, 64, 0, 2)
	b.Add(c.B(-4, 0, 54, 4, 1.6, 57, c.Lambda))
	b.Add(prefabs.Crate(-11, 0, 47, 1.6, 1.6, 1.6), prefabs.Crate(8, 0, 49, 1.6, 1.6, 1.6), prefabs.Crate(10, 0, 58, 1.4, 1.2, 1.4))
	b.Site("B", -12, -0.5, 44, 12, 3, 62)

	// The movers: the shuttle (the hub's north edge <-> A's dock) and the
	// ferry (the west pier's landing <-> B's west edge).
	b.Mover(c.Mover{Kind: c.MoverSlide, Mat: c.Lambda, Min: [3]float64{-3, hub - 1, -34}, Max: [3]float64{3, hub, -28}, Move: [3]float64{0, 0, 16}, Period: 10},
		c.Mover{Kind: c.MoverSlide, Mat: c.Lambda, Min: [3]float64{-52, -1, 44}, Max: [3]float64{-46, 0, 52}, Move: [3]float64{30, 0, 0}, Period: 14})

	// The pads: to A from the hub, from each spawn and from B's back; back down to the hub from A's.
	pad("hub to A", -6, hub, -8, [3]float64{-4, top, -52}, 4)
	pad("cold start to A", -60, 0, -8, [3]float64{-10, top, -55}, 6)
	pad("warm start to A", 60, 0, -8, [3]float64{10, top, -55}, 6)
	// (A throw lands fast and skids on: each lands short of its platform's far
	// edge by that much: TestServerlessPadsLandOnPlatforms.)
	pad("B to A", 0, 0, 61, [3]float64{0, top, -47}, 18)
	pad("A to hub", 9, top, -46.5, [3]float64{5, hub, -6}, 6)
	pad("west pier to B", -57, 0, 48, [3]float64{-9, 0, 50}, 3)
	pad("cold start to B", -57, 0, 9, [3]float64{-8, 0, 47}, 5)
	pad("warm start to B", 57, 0, 9, [3]float64{8, 0, 47}, 5)
	// The accelerator on the east pier: over the gap onto B.
	b.Add(c.B(27, 0, 46, 29.5, 0.06, 50, c.AccelPadR))
	b.Launcher("east accelerator", 28.25, 0.06, 48, 2.5, 4, [3]float64{-22, 8, 0})

	// Dressing: cover on every platform (decommissioned racks, low barriers),
	// tall cover beside the pads, which also carries the pads' labels (lit
	// amber, as the boards), and the zone boards at the spawns' far edges.
	// Kept off the pads' arcs, the spots they land on and where a landing
	// skids (TestServerlessPadsLandOnPlatforms), and off the ramps' lanes.
	rack := func(x0, y, z0, x1, z1, h float64) { b.Add(c.B(x0, y, z0, x1, y+h, z1, c.Rack)) }
	barrier := func(x0, y, z0, x1, z1 float64) { b.Add(c.B(x0, y, z0, x1, y+1.1, z1, c.Metal)) }
	label := func(text string, x, y, z float64, face string) {
		b.Sign(c.Sign{Text: text, Pos: [3]float64{x, y, z}, Face: face, Size: 0.34, Mat: c.SignLed})
	}
	// The hub: a rack beside the pad to A (its label), a rack and a barrier east, crates west.
	rack(-9, hub, -7.5, -8, -4, 2.6)
	label("PAD\nTO A", -7.97, hub+1.7, -5.75, c.SignPX)
	rack(7, hub, -6, 8, -3, 2.4)
	barrier(5, hub, 5, 9, 5.6)
	b.Add(prefabs.Crate(-8.5, hub, 6, 1.6, 1.6, 1.6), prefabs.Crate(-8.3, hub+1.6, 6.2, 1.2, 1.2, 1.2))
	// Site A: a rack column north-west, a barrier west, racks by the pad down and east.
	rack(-14.5, top, -58, -12.5, -57, 3)
	barrier(-15, top, -50, -11, -49.4)
	rack(5.5, top, -50, 6.5, -47.5, 2.6)
	label("PAD\nTO HUB", 6.53, top+1.7, -48.75, c.SignPX)
	rack(12, top, -60, 13, -56, 2.6)
	// Site B: two racks flanking the pad to A (labelled on the side you come from), barriers.
	rack(-3.6, 0, 59, -2.2, 62, 2.6)
	rack(2.2, 0, 59, 3.6, 62, 2.6)
	label("TO A", -2.9, 1.9, 58.97, c.SignNZ)
	label("TO A", 2.9, 1.9, 58.97, c.SignNZ)
	barrier(-14, 0, 58, -10, 58.6)
	barrier(11, 0, 42, 15, 42.6)
	// The spawns: racks by their pads (labels), a board at the far edge, the west pier's pad.
	for _, s := range []float64{-1, 1} { // west, east
		face := c.SignPX
		if s > 0 {
			face = c.SignNX
		}
		rack(s*63-0.5, 0, -10.5, s*63+0.5, -7, 2.6) // by the pad to A
		label("PAD\nTO A", s*63-s*0.53, 1.7, -8.75, face)
		rack(s*61-0.5, 0, 7.5, s*61+0.5, 10.5, 2.6) // by the pad to B
		label("PAD\nTO B", s*61-s*0.53, 1.7, 9, face)
		b.Add(c.B(s*77.85-0.15, 0, -9, s*77.85+0.15, 4, 9, c.Lambda)) // the board
		name := "COLD START ZONE"
		if s > 0 {
			name = "WARM START ZONE"
		}
		b.Sign(c.Sign{Text: name, Pos: [3]float64{s * 77.7, 2.9, 0}, Face: face, Size: 0.8, Mat: c.SignLed})
		b.Sign(c.Sign{Text: "NO SERVERS BEYOND THIS POINT", Pos: [3]float64{s * 77.7, 1.4, 0}, Face: face, Size: 0.45, Mat: c.SignLed})
	}
	rack(-63.5, 0, 46, -62.5, 50, 2.6) // the west pier's end: by its pad to B
	label("PAD\nTO B", -62.47, 1.7, 48, c.SignPX)

	// The void: below every platform (the pit: bots keep out of it; a fall
	// goes on past the world's bottom).
	b.Pit("the void", -120, -40, -100, 120, -4, 100)

	b.TeamSpawns(c.Vibe, [3]float64{-72, 0, -4.5}, [3]float64{-72, 0, -1.5}, [3]float64{-72, 0, 1.5}, [3]float64{-72, 0, 4.5},
		[3]float64{-68, 0, -3}, [3]float64{-68, 0, 0}, [3]float64{-68, 0, 3})
	b.TeamSpawns(c.Artisan, [3]float64{72, 0, -4.5}, [3]float64{72, 0, -1.5}, [3]float64{72, 0, 1.5}, [3]float64{72, 0, 4.5},
		[3]float64{68, 0, -3}, [3]float64{68, 0, 0}, [3]float64{68, 0, 3})
	b.BuyAroundSpawns(3)
	b.Rule("mp_gravityboots", "2") // everyone wears the Gravity Override: the hook on
	b.Meta("A defusal arena in space with no server under anything: floating platforms over the void, jump pads that throw you across the map and up to site A, an accelerator over a gap, a shuttle and a ferry. Everyone has the grappling hook. Don't look down.", 10)
	b.Category(c.Competitive)
	return b.Map()
}
