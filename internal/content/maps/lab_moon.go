package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// LabMoon is Star Slop's gravity lab (phase 2, docs/starslop.md): Earth's
// field in the west at the world's gravity, a void of space between (no
// gravity: you coast and steer with your suit's thrusters), and the Moon in
// the east, a regolith plateau 20 m up at 1/6 g. A launch pad on each side
// throws you across: Earth's up and east through space onto the Moon, the
// Moon's back west onto Earth. Falling into the void below kills. It plays
// the tickets mode: each side's main, a point by each pad, a crater point.
func LabMoon() c.Map {
	b := mapgen.New("lab_moon").Mode(c.ModeTickets).Sky(c.SkySpace)
	b.Fog(0.0015, [3]float64{0.01, 0.012, 0.025}) // vacuum: hardly any haze, black at the horizon
	// Earth: an 60 x 48 m field, open to the east (the cliff over the void).
	b.Add(c.B(-80, -1, -24, -20, 0, 24, c.Sand),
		c.B(-81, 0, -25, -20, 1.2, -24, c.Concrete), c.B(-81, 0, 24, -20, 1.2, 25, c.Concrete), c.B(-81, 0, -24, -80, 1.2, 24, c.Concrete))
	b.Add(prefabs.CratePyramid(-50, -10, 1.2)...)
	b.Add(prefabs.Crate(-44, 0, 8, 2, 1.4, 2), prefabs.Wall(-62, 4, -60, 14, 1.6, c.Concrete))
	// The Moon: a plateau 20 m up, open to the west; craters as low rims.
	b.Add(c.B(40, 10, -24, 120, moonTop, 24, c.Regolith),
		c.B(40, moonTop, -25, 121, moonTop+1.2, -24, c.Regolith), c.B(40, moonTop, 24, 121, moonTop+1.2, 25, c.Regolith), c.B(120, moonTop, -24, 121, moonTop+1.2, 24, c.Regolith))
	for _, cr := range [][3]float64{{70, -10, 5}, {88, 9, 4}, {64, 12, 3}} {
		x, z, r := cr[0], cr[1], cr[2]
		b.Add(c.B(x-r, moonTop, z-r, x+r, moonTop+0.5, z-r+0.6, c.Regolith), c.B(x-r, moonTop, z+r-0.6, x+r, moonTop+0.5, z+r, c.Regolith),
			c.B(x-r, moonTop, z-r+0.6, x-r+0.6, moonTop+0.5, z+r-0.6, c.Regolith), c.B(x+r-0.6, moonTop, z-r+0.6, x+r, moonTop+0.5, z+r-0.6, c.Regolith))
	}
	b.Add(prefabs.Crate(96, moonTop, -4, 2, 1.4, 2), prefabs.Crate(78, moonTop, 2, 1.5, 1.2, 3))
	// The pads (drawn as hazard plates in the floor) and their throws.
	b.Add(c.B(-27.5, 0, -1.5, -24.5, 0.05, 1.5, c.Hazard), c.B(46.5, moonTop, -1.5, 49.5, moonTop+0.05, 1.5, c.Hazard))
	b.Launcher("earth pad", -26, 0.05, 0, 3, 3, [3]float64{12, 16, 0}).
		Launcher("moon pad", 48, moonTop+0.05, 0, 3, 3, [3]float64{-12, 5, 0})
	// Gravity: space over the void (all heights), the Moon over its plateau.
	b.GravZone("space", -20, -60, -40, 38, 300, 40, 0, 0).
		GravZone("moon", 38, 10, -40, 130, 300, 40, 1.0/6, 0.6).
		Pit("the void", -20, -60, -40, 38, -40, 40)
	main := func(id, name, team string, x, y float64) c.CapturePoint {
		return c.CapturePoint{ID: id, Name: name, Pos: [3]float64{x, y, 0}, Radius: 7, Owner: team, Locked: true, Spawn: true}
	}
	pt := func(id, name string, x, y, z float64) c.CapturePoint {
		return c.CapturePoint{ID: id, Name: name, Pos: [3]float64{x, y, z}, Radius: 6, Spawn: true}
	}
	b.Point(main("earth", "Earth", c.Vibe, -70, 0)).
		Point(pt("A", "Launch Pad", -34, 0, 0)).
		Point(pt("B", "Crater", 70, moonTop, -10)).
		Point(pt("C", "Moon Base", 54, moonTop, 14)).
		Point(main("moon", "Moon", c.Artisan, 108, moonTop))
	m := b.Map()
	for _, p := range m.Points {
		if p.Locked {
			b.TeamSpawns(p.Owner, p.Spawns...)
		}
	}
	return b.BuyAroundSpawns(2).
		Meta("Gravity lab: Earth's field, space between, the Moon at 1/6 g; launch pads throw you across.", 16).
		Category(c.Training).
		Map()
}

// moonTop is the height of lab_moon's plateau.
const moonTop = 20.0
