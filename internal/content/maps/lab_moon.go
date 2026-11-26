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
	// Earth: a 60 x 48 m field, open to the east (the cliff over the void).
	b.Add(c.B(-80, -1, -24, -20, 0, 24, c.Sand),
		c.B(-110, 0, -25, -20, 1.2, -24, c.Concrete), c.B(-110, 0, 24, -20, 1.2, 25, c.Concrete))
	earthTerminal(b)
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
		Pit("the void", -20, -20, -40, 130, -6, 40) // under the void and the Moon: above the out-of-world line (y -20)
	main := func(id, name, team string, x, y float64) c.CapturePoint {
		return c.CapturePoint{ID: id, Name: name, Pos: [3]float64{x, y, 0}, Radius: 7, Owner: team, Locked: true, Spawn: true}
	}
	pt := func(id, name string, x, y, z float64) c.CapturePoint {
		return c.CapturePoint{ID: id, Name: name, Pos: [3]float64{x, y, z}, Radius: 6, Spawn: true}
	}
	b.Point(main("earth", "Earth", c.Vibe, -130, 0)).
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

// earthTerminal is Earth's spaceport (docs/starslop.md, "The opening"): a
// 40 x 40 m terminal hall (Earth's main: everyone on Earth spawns here) with
// rows of chairs, the departures board, the duty-free kiosk and a security
// arch; its east side is a window wall onto the apron, where MVP-1 stands in
// its flame trench, reached by the jet bridge through the "tickets please"
// gate. MVP-1 leaves every 30 s: boarding until 14 s, up 110 m by 20 s, across
// to 50 m over the Moon by 26 s, where the floor opens and the drop pods throw
// everyone down (26..29.5 s); the next one is at the dock at 30.
func earthTerminal(b *mapgen.Builder) {
	// The hall: carpet, walls 5 m, the window wall (a 1 m rail, open to 4 m).
	b.Add(c.B(-150, -1, -20, -110, 0, 20, c.Carpet),
		c.B(-151, 0, 20, -110, 5, 21, c.Plaster), c.B(-151, 0, -21, -110, 5, -20, c.Plaster), c.B(-151, 0, -20, -150, 5, 20, c.Plaster),
		c.B(-110, 0, -20, -109, 1, -2, c.Plaster), c.B(-110, 0, 2, -109, 1, 20, c.Plaster), c.B(-110, 4, -20, -109, 5, 20, c.Plaster),
		c.B(-110, 2.6, -2, -109, 4, 2, c.Plaster),
		c.B(-111, 0, 20, -110, 1.2, 25, c.Concrete), c.B(-111, 0, -25, -110, 1.2, -20, c.Concrete))
	b.Add(c.B(-150, 1.5, -7, -149.6, 4.2, 7, c.Screen)) // DEPARTURES
	b.Add(prefabs.Kiosk(-131, 0, 13)...)                // duty free (the kits)
	for _, x := range []float64{-144, -140, -121} {     // the waiting rows
		for _, z := range []float64{-14, 6} {
			b.Add(c.B(x, 0, z, x+0.6, 0.45, z+8, c.Awning), c.B(x+0.5, 0.45, z, x+0.65, 1.1, z+8, c.Awning))
		}
	}
	b.Add(c.B(-116, 0, -2.4, -115.6, 2.6, -2, c.Metal), c.B(-116, 0, 2, -115.6, 2.6, 2.4, c.Metal), c.B(-116, 2.6, -2.4, -115.6, 3, 2.4, c.Metal)) // security
	b.TicketGate("gate", -111, 0, -2, -108, 2.6, 2)
	// The jet bridge: from the window wall to MVP-1's door sill.
	b.Add(c.B(-109, -1, -2, -103.5, 0, 2, c.Metal), c.B(-109, 0, -2.4, -103.5, 2.6, -2, c.Glass), c.B(-109, 0, 2, -103.5, 2.6, 2.4, c.Glass),
		c.B(-109, 2.6, -2.4, -103.5, 3, 2.4, c.Metal))
	// The apron round the flame trench (x -105..-85, z -8..8: MVP-1's engines
	// hang into it); falling in is the end.
	b.Add(c.B(-110, -1, -24, -80, 0, -8, c.Concrete), c.B(-110, -1, 8, -80, 0, 24, c.Concrete),
		c.B(-110, -1, -8, -105, 0, 8, c.Concrete), c.B(-85, -1, -8, -80, 0, 8, c.Concrete),
		c.B(-105, -11, -8, -85, -10, 8, c.Hazard),
		c.B(-106, -11, -9, -105, -1, 9, c.Concrete), c.B(-85, -11, -9, -84, -1, 9, c.Concrete),
		c.B(-105, -11, -9, -85, -1, -8, c.Concrete), c.B(-105, -11, 8, -85, -1, 9, c.Concrete))
	// Guard rails round it (1.3 m: no jumping them; thin: not a floor), open
	// only where the jet bridge meets it.
	b.Add(c.B(-105.2, 0, -8.2, -84.8, 1.3, -8, c.Metal), c.B(-105.2, 0, 8, -84.8, 1.3, 8.2, c.Metal),
		c.B(-85, 0, -8, -84.8, 1.3, 8, c.Metal), c.B(-105.2, 0, -8, -105, 1.3, -2.4, c.Metal), c.B(-105.2, 0, 2.4, -105, 1.3, 8, c.Metal))
	b.Pit("the flame trench", -104.65, -10, -7.65, -85.35, -2, 7.65) // the hole less a body's radius: its rim stays walkable (the bots' edge guard)
	b.Ship(prefabs.Rocket("MVP-1", -95, 0, 0, prefabs.Timetable{
		Period: 30, Close: 14,
		Path:   [][4]float64{{0, 0, 0, 0}, {14, 0, 0, 0}, {20, 0, 110, 0}, {26, 185, 70, 0}},
		Eject:  [2]float64{26, 29.5},
		PodVel: [3]float64{0, -14, 0},
	}))
}
