package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// SurfScopeCreep is the surf map (docs/physics.md): the project's scope grew
// until the roadmap became four Gantt-chart surf ridges over a void of
// deprecated code. Each team starts on a deck high above the void (a bunny-hop
// runway: the map pins source movement, air accelerate 150 and auto bunny
// hop), and surfs a ridge to either site; the slow way is the legacy path:
// a ramp down and a long, narrow bridge through the middle. Fall into the
// void (under y -20) and you start over at your spawn.
//
// Layout (x across, z from the Artisans' deck at -z to the Vibe Coders' at +z):
// decks at y 12 (z ±78..96), sites A (-x) and B (+x) at y 4 around z -30
// (nearer the defenders), the ridges (60 degree sides, peak at y 8) at x ±31
// between them (the Vibe Coders' long, the Artisans' short), the bridge at y 4
// along x 0, the void's code at y -21.
func SurfScopeCreep() c.Map {
	const (
		deck   = 12.0 // the team decks' top
		site   = 4.0  // the sites' and the bridge's top
		ridgeX = 31.0 // the ridges' centre lines
		ridgeH = 14.0 // base to peak
		ridgeW = 8.0  // half width: 60 degree sides
		ridgeB = site - 10
		dz0    = 78.0  // decks' front edge
		dz1    = 96.0  // decks' back
		siteZ  = -30.0 // the sites' and the middle's centre: nearer the Artisans (defenders get there first)
		rz1    = 74.0  // ridges' far end (by the decks)
		ramp   = 16.0  // the legacy path's ramp run (26.6 degrees)
		bw     = 2.0   // the bridge's half width
	)
	b := mapgen.New("surf_scope_creep")
	// The void: deprecated code far below, never reached (the fall respawns at y -20).
	b.Add(c.B(-64, -22, -100, 64, -21, 100, c.Deprecated))
	for _, sg := range []float64{1, -1} {
		zf, zb := sg*dz0, sg*dz1
		lo, hi := min(zf, zb), max(zf, zb)
		// The team deck: the bunny-hop runway, a wall with screens at the back, rails along the sides.
		b.Add(prefabs.Slab(-44, lo, 44, hi, deck, 1, c.Concrete),
			c.B(-44, deck, min(zb, zb+sg), 44, deck+6, max(zb, zb+sg), c.Brick),
			c.B(-44.2, deck, lo, -44, deck+1.1, hi, c.Metal), c.B(44, deck, lo, 44.2, deck+1.1, hi, c.Metal))
		for _, x := range []float64{-14, 6} {
			b.Add(c.B(x, deck+1, min(zb-sg*0.2, zb), x+8, deck+5, max(zb-sg*0.2, zb), c.Screen))
		}
		// The legacy path: a ramp down from the deck to the bridge.
		dir := c.RampPZ
		if sg < 0 {
			dir = c.RampNZ
		}
		b.Add(prefabs.Ramp(-bw, min(zf, zf-sg*ramp), bw, max(zf, zf-sg*ramp), site, deck-site, dir, c.Concrete))
		// The ridges to A (-x) and B (+x): Gantt charts all the way.
		near := siteZ + sg*12 // a metre short of the site's deck
		for _, x := range []float64{-ridgeX, ridgeX} {
			b.Add(prefabs.SurfRamp(false, x, min(near, sg*rz1), max(near, sg*rz1), ridgeB, ridgeH, ridgeW, c.Gantt)...)
		}
		// The bridge's rails (open where it meets the middle).
		bz0, bz1 := min(siteZ+sg*2, sg*(dz0-ramp)), max(siteZ+sg*2, sg*(dz0-ramp))
		b.Add(c.B(-bw-0.15, site, bz0, -bw, site+1, bz1, c.Metal), c.B(bw, site, bz0, bw+0.15, site+1, bz1, c.Metal))
	}
	// The bridge, and the middle joining it to the sites.
	b.Add(prefabs.Slab(-bw, -(dz0-ramp), bw, dz0-ramp, site, 0.5, c.Metal),
		prefabs.Slab(-20, siteZ-2, -bw, siteZ+2, site, 0.5, c.Metal), prefabs.Slab(bw, siteZ-2, 20, siteZ+2, site, 0.5, c.Metal))
	// The sites: decks with crates (a sprint's backlog) for cover.
	for _, sg := range []float64{-1, 1} {
		b.Add(prefabs.Slab(min(sg*20, sg*42), siteZ-11, max(sg*20, sg*42), siteZ+11, site, 1, c.Metal))
		x, z := sg*31, siteZ
		b.Add(prefabs.Crate(x-3, site, z-5, 1.5, 1.5, 1.5), prefabs.Crate(x+1.5, site, z+3, 1.5, 1.5, 1.5),
			prefabs.Crate(x+3.5, site, z-7.5, 1.5, 1.5, 1.5), prefabs.Crate(x+3.75, site+1.5, z-7.25, 1, 1, 1),
			prefabs.Crate(min(sg*24, sg*25.5), site, z-9, 1.5, 1.2, 1.5), prefabs.Crate(min(sg*36, sg*37.5), site, z+7, 1.5, 1.2, 1.5))
	}
	var vs, as [][3]float64
	for i := range 5 {
		x := -8 + 4*float64(i)
		vs, as = append(vs, [3]float64{x, deck, 90}), append(as, [3]float64{x, deck, -90})
	}
	b.TeamSpawns(c.Vibe, vs...).TeamSpawns(c.Artisan, as...).BuyAroundSpawns(3)
	b.Site("A", -42, site-0.5, siteZ-11, -20, site+3, siteZ+11)
	b.Site("B", 20, site-0.5, siteZ-11, 42, site+3, siteZ+11)
	b.Nav("vibe", 0, deck, 90, "vTop").
		Nav("vTop", 0, deck, dz0+1, "vBot").
		Nav("vBot", 0, site, dz0-ramp-1, "vBridge").
		Nav("vBridge", 0, site, 10, "mid").
		Nav("mid", 0, site, siteZ, "aMid", "bMid").
		Nav("aMid", -17, site, siteZ, "A").
		Nav("A", -29, site, siteZ-7).
		Nav("bMid", 17, site, siteZ, "B").
		Nav("B", 29, site, siteZ-7).
		Nav("artisan", 0, deck, -90, "rTop").
		Nav("rTop", 0, deck, -dz0-1, "rBot").
		Nav("rBot", 0, site, -(dz0-ramp-1), "rBridge").
		Nav("rBridge", 0, site, -50, "mid")
	return b.
		Rule("sv_movement", "source").Rule("sv_airaccelerate", "150").Rule("sv_autobunnyhopping", "1").
		Meta("Surf the roadmap: four Gantt-chart ridges over a void of deprecated code. Bunny-hop off your deck, strafe into a ridge, land on a site. Or take the legacy path.", 10).
		Category(c.Competitive).
		Map()
}
