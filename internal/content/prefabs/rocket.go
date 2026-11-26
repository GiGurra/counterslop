package prefabs

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// Timetable is a ship's departure cycle (docs/starslop.md, phase 3): every
// Period seconds it is docked until Close, flies Path (MoverPath keys,
// offsets from the dock) and drops its passengers between Eject[0] and
// Eject[1] (the cabin floor, seats and engines fold away, the drop pods throw
// at PodVel). At the cycle's end it is back at the dock (the next one).
type Timetable struct {
	Period, Close float64
	Path          [][4]float64
	Eject         [2]float64
	PodVel        [3]float64
}

// Rocket is MVP-1 (Minimum Viable Payload), Star Slop's passenger rocket: an
// economy cabin 16 x 8 m (x -8..8, z -4..4 round the dock point; its floor's
// top at y) with four rows of seats either side of the aisle, window bands
// along both sides and a door in its west wall (open while docked), under a
// 14 m wide stack 80 m tall (duct-tape bands, a stepped nose), two strap-on
// boosters and two engine bells, which hang 7 m below the floor (a dock
// stands in a flame trench). The cabin keeps its own 1 g through space;
// boarding is through the door from Door (x-9.6) to Board (the aisle).
func Rocket(name string, x, y, z float64, tt Timetable) mapgen.Ship {
	var sh mapgen.Ship
	add := func(mat string, hide [2]float64, x0, y0, z0, x1, y1, z1 float64) {
		sh.Movers = append(sh.Movers, c.Mover{Kind: c.MoverPath, Mat: mat, Min: [3]float64{x + x0, y + y0, z + z0}, Max: [3]float64{x + x1, y + y1, z + z1},
			Period: tt.Period, Path: tt.Path, Hide: hide})
	}
	never, docked := [2]float64{}, [2]float64{0, tt.Close}
	// The floor first: mover 1 (the departure's, the zone's and the pods').
	add(c.Carpet, tt.Eject, -8.5, -0.5, -4, 8, 0, 4) // under the door's sill too: the jet bridge meets it
	// The west wall and its door (there in flight only).
	add(c.Plaster, never, -8.5, -0.5, -4.5, -8, 3.2, -1)
	add(c.Plaster, never, -8.5, -0.5, 1, -8, 3.2, 4.5)
	add(c.Plaster, never, -8.5, 2.4, -1, -8, 3.2, 1)
	add(c.Metal, docked, -8.5, 0, -1, -8, 2.4, 1) // gone while docked: open for boarding
	// The east wall, the window bands (1 m of window at eye height), the roof.
	add(c.Plaster, never, 8, -0.5, -4.5, 8.5, 3.2, 4.5)
	for _, s := range []float64{-1, 1} {
		z0, z1 := 4.0, 4.5
		if s < 0 {
			z0, z1 = -4.5, -4.0
		}
		add(c.Plaster, never, -8, -0.5, z0, 8, 1.2, z1)
		add(c.Plaster, never, -8, 2.2, z0, 8, 3.2, z1)
	}
	add(c.Plaster, never, -8.5, 3.2, -4.5, 8.5, 3.7, 4.5)
	// Economy: four rows each side of the aisle (they fold away with the floor).
	for _, rx := range []float64{-5.5, -2.5, 0.5, 3.5} {
		for _, s := range []float64{-1, 1} {
			z0, z1 := 1.0, 3.8
			if s < 0 {
				z0, z1 = -3.8, -1.0
			}
			add(c.Awning, tt.Eject, rx, 0, z0, rx+0.7, 0.45, z1)
			add(c.Awning, tt.Eject, rx+0.6, 0.45, z0, rx+0.75, 1.1, z1)
		}
	}
	// Outside: the stack, duct tape, the nose; boosters; engine bells.
	add(c.Plaster, never, -7, 3.7, -4.5, 7, 62, 4.5)
	add(c.Hazard, never, -7.1, 20, -4.6, 7.1, 21, 4.6)
	add(c.Hazard, never, -7.1, 41, -4.6, 7.1, 42, 4.6)
	add(c.Plaster, never, -5.5, 62, -3.5, 5.5, 70, 3.5)
	add(c.Plaster, never, -3.5, 70, -2.2, 3.5, 76, 2.2)
	add(c.Metal, never, -1.5, 76, -1, 1.5, 80, 1)
	for _, s := range []float64{-1, 1} {
		z0, z1 := 4.5, 7.5
		if s < 0 {
			z0, z1 = -7.5, -4.5
		}
		add(c.Container, never, -1.5, -7, z0, 1.5, 40, z1)
		add(c.Metal, never, -1, 40, z0+0.5, 1, 44, z1-0.5)
	}
	add(c.Metal, tt.Eject, -8, -7, -3, -6, -0.5, 3)
	add(c.Metal, tt.Eject, 6, -7, -3, 8, -0.5, 3)

	sh.Zones = []c.GravZone{{Name: name + " cabin", Min: [3]float64{x - 8, y - 0.5, z - 4}, Max: [3]float64{x + 8, y + 3.2, z + 4}, G: 1, Jump: 1, Mover: 1}}
	sh.Launchers = []c.Launcher{{Name: name + " drop pods", Min: [3]float64{x - 8, y - 1.5, z - 4}, Max: [3]float64{x + 8, y + 1, z + 4}, Vel: tt.PodVel,
		Mover: 1, From: tt.Eject[0], To: tt.Eject[1], Air: true}}
	sh.Departure = &c.Departure{Name: name, Mover: 1, Close: tt.Close, Door: [3]float64{x - 9.6, y, z}, Board: [3]float64{x, y, z}}
	return sh
}
