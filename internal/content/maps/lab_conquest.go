package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// LabConquest is the tickets mode's test yard (Star Slop phase 1,
// docs/starslop.md): a flat 128 x 64 m field, Earth's main in the west (the
// Vibe Coders), the Moon's in the east (the Artisans), both locked, and five
// capture points between them: A and E by each main, B and D on the north
// and south flanks, C in the middle (the only one nobody spawns at). Crates
// and low walls give cover between the points. The internal/game ticket
// tests and the sim/cases tickets-* play on it.
func LabConquest() c.Map {
	b := mapgen.New("lab_conquest").Mode(c.ModeTickets)
	b.Add(mapgen.Arena(-64, -32, 64, 32, 5, 1, c.Sand, c.Concrete)...)
	// Cover: crate stacks around the middle, walls on the flanks' approaches.
	b.Add(prefabs.CratePyramid(-6, -6, 1.2)...)
	b.Add(prefabs.CratePyramid(5, 4, 1.2)...)
	b.Add(prefabs.Wall(-22, -22, -20, -12, 1.6, c.Concrete), prefabs.Wall(20, 12, 22, 22, 1.6, c.Concrete),
		prefabs.Wall(-22, 12, -20, 22, 1.6, c.Concrete), prefabs.Wall(20, -22, 22, -12, 1.6, c.Concrete),
		prefabs.Crate(-41, 0, -9, 2, 1.4, 2), prefabs.Crate(39, 0, 7, 2, 1.4, 2),
		prefabs.Crate(-2, 0, -24, 4, 1.2, 1.5), prefabs.Crate(-2, 0, 22.5, 4, 1.2, 1.5))
	main := func(id, name, team string, x float64) c.CapturePoint {
		return c.CapturePoint{ID: id, Name: name, Pos: [3]float64{x, 0, 0}, Radius: 7, Owner: team, Locked: true, Spawn: true}
	}
	pt := func(id, name string, x, z float64, spawn bool) c.CapturePoint {
		return c.CapturePoint{ID: id, Name: name, Pos: [3]float64{x, 0, z}, Radius: 6, Spawn: spawn}
	}
	b.Point(main("earth", "Earth", c.Vibe, -56)).
		Point(pt("A", "Launch Pad", -32, 0, true)).
		Point(pt("B", "Crater", 0, -16, true)).
		Point(pt("C", "Relay", 0, 0, false)).
		Point(pt("D", "Antenna", 0, 16, true)).
		Point(pt("E", "Dome", 32, 0, true)).
		Point(main("moon", "Moon", c.Artisan, 56))
	m := b.Map()
	// Warmup (no tickets yet) spawns each side at its main, like the mode does.
	for _, p := range m.Points {
		if p.Locked {
			b.TeamSpawns(p.Owner, p.Spawns...)
		}
	}
	return b.BuyAroundSpawns(2).
		Meta("Conquest lab: Earth vs the Moon on a flat field, five capture points, ticket backlogs.", 16).
		Category(c.Training).
		Map()
}
