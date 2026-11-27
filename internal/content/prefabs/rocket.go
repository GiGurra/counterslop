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
	// Height is the nose's tip above the cabin floor (0: 80 m).
	Height float64
	// Flank, when set, splits the drop pods in three along the cabin: its
	// aft third (x -8..-2.7, by the door) throws at PodVel plus Flank m/s
	// towards -z, the fore third towards +z, the middle straight; FlankNames
	// (-z, +z) are where they land, on signs over each side's bins.
	Flank      float64
	FlankNames [2]string
	// Team redeploys aboard while it boards (content.Departure.Team).
	Team string
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
	// First class (a ship a team redeploys on): one plush seat at the front, its
	// passenger's name on the wall (the team's top scorer, a live sign).
	if tt.Team != "" {
		add(c.Carpet, tt.Eject, 5.6, 0, 1.4, 7.0, 0.55, 3.6)
		add(c.Carpet, tt.Eject, 7.0, 0, 1.4, 7.4, 1.5, 3.6)
		add(c.Lamp, tt.Eject, 5.6, 0.55, 1.2, 7.4, 0.75, 1.4)
		add(c.Lamp, tt.Eject, 5.6, 0.55, 3.6, 7.4, 0.75, 3.8)
	}
	// Overhead bins along both sides (over the window bands' top).
	add(c.Metal, never, -7.5, 2.4, -4, 7.5, 3.2, -3.3)
	add(c.Metal, never, -7.5, 2.4, 3.3, 7.5, 3.2, 4)
	// Outside: the stack, duct tape, the nose; boosters; engine bells.
	h := tt.Height
	if h <= 0 {
		h = 80
	}
	top := h - 18 // the stack's top, under the nose
	add(c.Plaster, never, -7, 3.7, -4.5, 7, top, 4.5)
	for _, f := range []float64{0.25, 0.5, 0.75} { // duct tape
		y0 := 3.7 + float64(f*(top-3.7))
		add(c.Hazard, never, -7.1, y0, -4.6, 7.1, y0+1, 4.6)
	}
	add(c.Plaster, never, -5.5, top, -3.5, 5.5, h-10, 3.5)
	add(c.Plaster, never, -3.5, h-10, -2.2, 3.5, h-4, 2.2)
	add(c.Metal, never, -1.5, h-4, -1, 1.5, h, 1)
	bt := float64(0.5 * h) // the boosters' tops
	for _, s := range []float64{-1, 1} {
		z0, z1 := 4.5, 7.5
		if s < 0 {
			z0, z1 = -7.5, -4.5
		}
		add(c.Container, never, -1.5, -7, z0, 1.5, bt, z1)
		add(c.Metal, never, -1, bt, z0+0.5, 1, bt+4, z1-0.5)
	}
	add(c.Metal, tt.Eject, -8, -7, -3, -6, -0.5, 3)
	add(c.Metal, tt.Eject, 6, -7, -3, 8, -0.5, 3)

	sh.Zones = []c.GravZone{{Name: name + " cabin", Min: [3]float64{x - 8, y - 0.5, z - 4}, Max: [3]float64{x + 8, y + 3.2, z + 4}, G: 1, Jump: 1, Mover: 1}}
	pods := func(label string, x0, x1, dz float64) c.Launcher {
		return c.Launcher{Name: name + label, Min: [3]float64{x + x0, y - 1.5, z - 4}, Max: [3]float64{x + x1, y + 1, z + 4},
			Vel: [3]float64{tt.PodVel[0], tt.PodVel[1], tt.PodVel[2] + dz}, Mover: 1, From: tt.Eject[0], To: tt.Eject[1], Air: true}
	}
	if tt.Flank == 0 {
		sh.Launchers = []c.Launcher{pods(" drop pods", -8, 8, 0)}
	} else {
		sh.Launchers = []c.Launcher{pods(" drop pods", -2.7, 2.7, 0), pods(" aft pods", -8, -2.7, -tt.Flank), pods(" fore pods", 2.7, 8, tt.Flank)}
		sh.Signs = append(sh.Signs,
			c.Sign{Text: "PODS THIS SIDE:\n" + tt.FlankNames[0], Pos: [3]float64{x - 5.3, y + 2.8, z - 3.28}, Face: c.SignPZ, Size: 0.16, Mat: c.SignLed, Mover: 1},
			c.Sign{Text: "PODS THIS SIDE:\n" + tt.FlankNames[1], Pos: [3]float64{x + 5.3, y + 2.8, z + 3.28}, Face: c.SignNZ, Size: 0.16, Mat: c.SignLed, Mover: 1})
	}
	// Its name down both flanks, the BETA sticker, and inside the seatbelt
	// sign and the escape pods' small print (signs on mover 1).
	for _, f := range []struct {
		face string
		z    float64
	}{{c.SignPZ, 4.5}, {c.SignNZ, -4.5}} {
		sh.Signs = append(sh.Signs,
			c.Sign{Text: name, Pos: [3]float64{x, y + float64(0.62*h), z + f.z}, Face: f.face, Size: 2.4, Mat: c.SignInk, Mover: 1},
			c.Sign{Text: "BETA", Pos: [3]float64{x + 3.5, y + float64(0.62*h) - 3.2, z + f.z}, Face: f.face, Size: 1.1, Mat: c.SignLed, Mover: 1})
	}
	sh.Signs = append(sh.Signs,
		c.Sign{Text: "FASTEN SEATBELT", Pos: [3]float64{x + 8, y + 2.75, z}, Face: c.SignNX, Size: 0.22, Mat: c.SignLed, Mover: 1},
		c.Sign{Text: "ESCAPE PODS:\nDO NOT USE UNLESS\nLAID OFF", Pos: [3]float64{x + 8, y + 1.6, z}, Face: c.SignNX, Size: 0.14, Mat: c.SignInk, Mover: 1})
	if tt.Team != "" {
		sh.Signs = append(sh.Signs, c.Sign{Text: "FIRST CLASS:\n{first:" + tt.Team + "}", Pos: [3]float64{x + 8, y + 2.05, z + 2.5}, Face: c.SignNX, Size: 0.16, Mat: c.SignLed, Mover: 1, Live: true})
	}
	sh.Departure = &c.Departure{Name: name, Mover: 1, Close: tt.Close, Door: [3]float64{x - 9.6, y, z}, Board: [3]float64{x, y, z}, Team: tt.Team}
	return sh
}
