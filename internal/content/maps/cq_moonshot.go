package maps

import (
	"math"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// CqMoonshot is Star Slop's map (phase 4, docs/starslop.md "The opening"):
// Earth (the Vibe Coders) against the Moon (the Artisans) in the tickets mode.
//
// Earth is a spaceport terminal in the west, the only way to the front the
// next departure: you spawn in the check-in hall under the departures board,
// pass the security scanners (they beep at everyone), the duty free and the
// "tickets please" gate under Earth's split-flap backlog counter, and walk
// the jet bridge into MVP-1, 150 m tall on its pad outside the terminal's
// window wall. Every 35 s it leaves: doors shut at 12 s, up 150 m by 17 s,
// across the void to 40 m over the Moon's drop zone by 23 s, where the floor
// opens and the drop pods throw everyone down (23..26.5 s); then it flies
// back empty with its doors shut and is on the pad again at 34.5 s, so it
// never jumps back (nobody is shoved where it reappears).
//
// The Moon is the Lunar Office Campus at 1/6 g on a plateau 30 m up in the
// east: the drop zone, the solar farm and the rover parking on the open
// surface, then the campus wall and its badge gates, the quad with standing
// desks for cover, the Lunar Compliance Dept. (open-plan, standing desks),
// the cafeteria (zero-g pudding) and HQ, whose lobby elevator banks are the
// Moon's main. Five capture points between the mains; Earth spawns at any it
// holds, so the war moves to the Moon after the first landing.
func CqMoonshot() c.Map {
	b := mapgen.New("cq_moonshot").Mode(c.ModeTickets).Sky(c.SkySpace)
	b.Fog(0.0012, [3]float64{0.01, 0.012, 0.025})
	moonshotTerminal(b)
	moonshotPad(b)
	moonshotSurface(b)
	moonshotCampus(b)
	// Gravity: Earth's own over the terminal and the pad (the world's), space
	// over the void, the Moon over its plateau; the void below kills.
	b.GravZone("space", -125, -60, -90, 75, 400, 90, 0, 0).
		GravZone("moon", 75, 0, -90, 320, 400, 90, 1.0/6, 0.6).
		Pit("the void", -125, -20, -90, 320, -6, 90)
	// The sky: Earth's blue over the terminal and the pad (the Moon in it), the
	// Earth over the Moon; space with both in between.
	b.SkyZone(c.SkyEarth, -400, -100, -200, -125, 1000, 200).
		SkyZone(c.SkyMoon, 75, -100, -200, 400, 1000, 200)
	pt := func(id, name string, x, z, r float64, owner string) c.CapturePoint {
		y := mTop
		if id == "D" {
			y = fl // indoors, on the carpet
		}
		return c.CapturePoint{ID: id, Name: name, Pos: [3]float64{x, y, z}, Radius: r, Owner: owner}
	}
	earth := c.CapturePoint{ID: "earth", Name: "Terminal 0", Pos: [3]float64{-272, 0, 0}, Radius: 16, Owner: c.Vibe, Locked: true, Spawn: true}
	hq := c.CapturePoint{ID: "moon", Name: "HQ Lobby", Pos: [3]float64{286, mTop + 0.02, 0}, Radius: 14, Owner: c.Artisan, Locked: true, Spawn: true}
	for _, z := range []float64{-7, 0, 7} { // the Moon respawns out of the lobby's elevators
		for _, dz := range []float64{-0.9, 0.9} {
			for _, x := range []float64{291.8, 293.2, 294.6} {
				hq.Spawns = append(hq.Spawns, [3]float64{x, mTop + 0.02, z + dz}) // on the lobby's carpet
			}
		}
	}
	for i := range 16 { // two rings round the check-in hall's middle
		a, r := float64(i)*math.Pi/4, 8.8
		if i >= 8 {
			a, r = a+math.Pi/8, 4.4
		}
		earth.Spawns = append(earth.Spawns, [3]float64{math.Round((earth.Pos[0]+r*math.Sin(a))*100) / 100, 0, math.Round((earth.Pos[2]+r*math.Cos(a))*100) / 100})
	}
	b.Point(earth).
		Point(pt("A", "Drop Zone", 120, 0, 9, "")).
		Point(pt("B", "Solar Farm", 115, -52, 9, "")).
		Point(pt("C", "Rover Parking", 115, 52, 9, "")).
		Point(pt("D", "Compliance Dept.", 232, -24, 9, "")).
		Point(pt("E", "Badge Gates", 160, 0, 9, "")).
		Point(hq)
	m := b.Map()
	for _, p := range m.Points {
		if p.Locked {
			b.TeamSpawns(p.Owner, p.Spawns...)
		}
	}
	// A long backlog: Earth bleeds from the start (the Moon holds its campus)
	// and only lands with the first drop, ~25 s in.
	b.Rule("mp_tickets", "300")
	return b.BuyAroundSpawns(2).
		Meta("Star Slop: board MVP-1 at Earth's spaceport (tickets please), ride it through space and drop onto the Lunar Office Campus at 1/6 g. Five capture points, ticket backlogs.", 32).
		Category(c.Conquest).
		Map()
}

// mTop is the height of the Moon's plateau on cq_moonshot; fl the campus
// buildings' carpet on it.
const (
	mTop = 30.0
	fl   = mTop + 0.02
)

// moonshotDock: where MVP-1 stands (its cabin floor's centre) and its
// timetable.
var moonshotDock = [3]float64{-170, 0, 0}

func moonshotTimetable() prefabs.Timetable {
	return prefabs.Timetable{
		Period: 38, Close: 19, Height: 150,
		Path: [][4]float64{{0, 0, 0, 0}, {19, 0, 0, 0}, {22.5, 0, 150, 0}, {27, 290, 70, 0}, {30.5, 290, 70, 0},
			{34.5, 0, 160, 0}, {37.5, 0, 0, 0}}, // back empty, doors shut: docked again before the cycle wraps
		Eject:  [2]float64{27, 30.5},
		PodVel: [3]float64{0, -14, 0},
		// The aft pods land in the solar farm (B), the fore ones by the
		// rovers (C), the middle in the drop zone (A): a wave can flank.
		Flank:      24,
		FlankNames: [2]string{"SOLAR FARM", "ROVER PARKING"},
		Team:       c.Vibe, // Earth's dead redeploy aboard while it boards
	}
}

// moonshotTerminal is Earth's spaceport terminal: a hall x -290..-200,
// z -40..40, 12 m high. West to east: check-in (Earth's main) under the
// departures board, the security line, airside (duty free, the waiting
// rows) and the gate before the window wall, all glass from 1 m to 11 m.
func moonshotTerminal(b *mapgen.Builder) {
	const x0, x1, z0, z1, top = -290.0, -200.0, -40.0, 40.0, 12.0
	b.Add(c.B(x0, -1, z0, x1, 0, z1, c.Carpet),
		c.B(x0-1, 0, z0-1, x1+1, top, z0, c.Plaster), c.B(x0-1, 0, z1, x1+1, top, z1+1, c.Plaster), // south, north
		c.B(x0-1, 0, z0, x0, top, z1, c.Plaster),             // west
		c.B(x0-1, top, z0-1, x1+1, top+0.6, z1+1, c.Plaster)) // the roof
	// The window wall: open from a 1.3 m rail (no jumping it) up to the
	// lintel at 11 m, mullions every 10 m (the renderer has no see-through
	// glass: the panes are spotless), the bridge's mouth at z -2..2.
	b.Add(c.B(x1, 0, z0, x1+1, 1.3, -2, c.Plaster), c.B(x1, 0, 2, x1+1, 1.3, z1, c.Plaster),
		c.B(x1, 2.6, -2, x1+1, 3, 2, c.Plaster), c.B(x1, 11, z0, x1+1, top, z1, c.Plaster))
	for _, z := range []float64{-30, -20, -10, 10, 20, 30} { // mullions
		b.Add(c.B(x1+0.2, 1.3, z-0.15, x1+0.8, 11, z+0.15, c.Metal))
	}
	// Ceiling lights over the whole hall.
	for x := x0 + 10; x < x1; x += 20 {
		for _, z := range []float64{-24, 0, 24} {
			l := c.Spot([3]float64{x, top - 0.3, z}, [3]float64{0, -1, 0}, [3]float64{1, 0.95, 0.86}, 10, 16, 80)
			l.Inner = 40
			b.Light(l)
			b.Add(c.B(x-0.8, top-0.2, z-0.4, x+0.8, top, z+0.4, c.Lamp))
		}
	}

	// Check-in: two banks of counters, a screen each, the bag belt behind.
	for _, side := range []float64{-1, 1} {
		zA, zB := 12.0, 32.0
		if side < 0 {
			zA, zB = -32, -12
		}
		b.Add(c.B(-252, 0, zA, -251, 1.1, zB, c.Desk), c.B(-249.5, 0, zA, -248, 0.6, zB, c.Metal))
		for z := zA + 2; z < zB; z += 4 {
			b.Add(c.B(-252, 1.1, z-0.4, -251.9, 1.6, z+0.4, c.Screen))
		}
		b.Sign(c.Sign{Text: "CHECK-IN", Pos: [3]float64{-252.05, 3.2, (zA + zB) / 2}, Face: c.SignNX, Size: 0.7, Mat: c.SignLed})
		b.Add(c.B(-252.2, 2.6, (zA+zB)/2-4, -252.05, 3.8, (zA+zB)/2+4, c.Flap))
	}
	// The departures board: the whole of the west wall's middle.
	b.Add(c.B(x0, 2.4, -17, x0+0.25, 10.6, 17, c.Flap))
	b.Sign(c.Sign{Text: "EARTH DEPARTURES · TERMINAL 0 (MOON)", Pos: [3]float64{x0 + 0.25, 9.6, 0}, Face: c.SignPX, Size: 0.75, Mat: c.SignLed})
	b.Sign(c.Sign{Text: "14:05 MOON  MVP-0    CANCELLED - REORG\n" +
		"14:05 MOON  MVP-1    DELAYED - SCOPE CREEP\n" +
		"14:06 MOON  MVP-1    BOARDING GATE 0\n" +
		"14:07 MOON  MVP-1.1  HOTFIX - ON TIME\n" +
		"14:08 MARS  MVP-2    ON THE ROADMAP\n" +
		"14:09 MOON  MVP-1    SEE AGENT", Pos: [3]float64{x0 + 0.25, 5.6, 0}, Face: c.SignPX, Size: 0.5, Mat: c.SignLed})
	b.Sign(c.Sign{Text: "NEXT  MVP-1  {status:MVP-1}  DOORS CLOSE IN {departure:MVP-1}", Pos: [3]float64{x0 + 0.25, 3.1, 0}, Face: c.SignPX, Size: 0.5, Mat: c.SignLed, Live: true})
	// Waiting rows in check-in (round the main's spawn ring) and airside.
	for _, r := range [][3]float64{{-286, -30, -18}, {-286, 18, 30}, {-262, -36, -26}, {-262, 26, 36}} {
		x, za, zb := r[0], r[1], r[2]
		b.Add(c.B(x, 0, za, x+0.6, 0.45, zb, c.Awning), c.B(x+0.5, 0.45, za, x+0.65, 1.1, zb, c.Awning))
	}
	// Airside's rows run along the hall (the way to the gate stays open).
	for _, r := range [][3]float64{{-16, -237, -222}, {-23, -237, -222}, {-16, -219, -208}, {-23, -219, -208}, {16, -219, -208}, {22, -219, -208}} {
		z, xa, xb := r[0], r[1], r[2]
		b.Add(c.B(xa, 0, z, xb, 0.45, z+0.6, c.Awning), c.B(xa, 0.45, z+0.5, xb, 1.1, z+0.65, c.Awning))
	}

	// Security: a glass partition across the hall at x -245 (2.6 m: no
	// jumping it), three walk-through scanners in it, x-ray belts between.
	const sx = -245.0
	gaps := [][2]float64{{-12, -6}, {-3, 3}, {6, 12}} // wide lanes: a queue fits (and the bots' nav graph links through each)
	z := z0
	for _, g := range gaps {
		b.Add(c.B(sx, 0, z, sx+0.3, 2.6, g[0]-0.3, c.Glass))
		z = g[1] + 0.3
	}
	b.Add(c.B(sx, 0, z, sx+0.3, 2.6, z1, c.Glass))
	for _, g := range gaps {
		b.Add(c.B(sx-0.6, 0, g[0]-0.3, sx+0.9, 2.5, g[0], c.Metal), c.B(sx-0.6, 0, g[1], sx+0.9, 2.5, g[1]+0.3, c.Metal),
			c.B(sx-0.6, 2.5, g[0]-0.3, sx+0.9, 2.9, g[1]+0.3, c.Metal))
		b.Scanner("security", sx-0.6, 0, g[0], sx+0.9, 2.5, g[1])
		b.Sign(c.Sign{Text: "SCAN", Pos: [3]float64{sx - 0.6, 2.7, (g[0] + g[1]) / 2}, Face: c.SignNX, Size: 0.22, Mat: c.SignLed})
		// The bots' way through each scanner (the generated nav graph
		// joins the two halves of the hall once; these make three lanes).
		zc := (g[0] + g[1]) / 2
		b.NavHint(sx-6, 0, zc).NavHint(sx+0.15, 0, zc).NavHint(sx+6, 0, zc)
	}
	for _, zz := range [][2]float64{{-5.7, -3.3}, {3.3, 5.7}} { // the x-ray machines and their belts (cover)
		b.Add(c.B(sx-4, 0, zz[0], sx-0.2, 0.8, zz[1], c.Metal), c.B(sx-3.2, 0.8, zz[0]+0.2, sx-1, 1.9, zz[1]-0.2, c.Checkout))
	}
	b.Add(c.B(sx-0.1, 4.2, -12, sx+0.1, 6.2, 12, c.Flap))
	b.Sign(c.Sign{Text: "SECURITY\nPLEASE REMOVE GRENADES, LAPTOPS AND OPINIONS FROM YOUR BAG", Pos: [3]float64{sx - 0.1, 5.2, 0}, Face: c.SignNX, Size: 0.34, Mat: c.SignLed})

	// Airside: the duty free (the kits: Kiosk) with its ads.
	b.Add(c.B(-238, 0, 30, -226, 3, 30.4, c.Plaster))
	b.Add(prefabs.Kiosk(-233, 0, 27)...)
	b.Add(prefabs.Kiosk(-230, 0, 27)...)
	b.Add(c.B(-237, 1, 29.95, -233.5, 2.8, 30, c.Poster), c.B(-230.5, 1, 29.95, -227, 2.8, 30, c.Poster))
	b.Sign(c.Sign{Text: "DUTY FREE", Pos: [3]float64{-232, 3.6, 30}, Face: c.SignNZ, Size: 0.6, Mat: c.SignLed})
	b.Sign(c.Sign{Text: "SENIOR SNIPER\nNOW 0% OFF", Pos: [3]float64{-235.25, 2.45, 29.95}, Face: c.SignNZ, Size: 0.16, Mat: c.SignInk})
	b.Sign(c.Sign{Text: "GRENADES\nTAX FREE", Pos: [3]float64{-228.75, 2.45, 29.95}, Face: c.SignNZ, Size: 0.16, Mat: c.SignInk})

	// The gate: a desk, rails funnelling to the jet bridge, the ticket gate,
	// Earth's backlog on split-flaps over the bridge's mouth.
	b.Add(c.B(-206, 0, 4, -204, 1.1, 7, c.Desk), c.B(-205.8, 1.1, 5, -205.7, 1.6, 6, c.Screen),
		c.B(-212, 0, -2.4, -201, 1.1, -2.2, c.Metal), c.B(-212, 0, 2.2, -201, 1.1, 2.4, c.Metal))
	b.TicketGate("gate 0", -204, 0, -2.2, -200, 2.6, 2.2)
	b.NavHint(-216, 0, 0).NavHint(-206, 0, 0) // into the gate's funnel
	b.Add(c.B(x1-0.25, 4, -9, x1, 10, 9, c.Flap))
	b.Sign(c.Sign{Text: "GATE 0 · MVP-1 TO THE MOON", Pos: [3]float64{x1 - 0.25, 9.2, 0}, Face: c.SignNX, Size: 0.5, Mat: c.SignLed})
	b.Sign(c.Sign{Text: "EARTH BACKLOG", Pos: [3]float64{x1 - 0.25, 7.9, 0}, Face: c.SignNX, Size: 0.45, Mat: c.SignLed})
	b.Sign(c.Sign{Text: "{backlog:vibe}", Pos: [3]float64{x1 - 0.25, 5.95, 0}, Face: c.SignNX, Size: 2.2, Mat: c.SignLed, Live: true})
	b.Sign(c.Sign{Text: "TICKETS PLEASE", Pos: [3]float64{x1 - 0.25, 4.3, 0}, Face: c.SignNX, Size: 0.3, Mat: c.SignLed})
}

// moonshotPad is outside the window wall: the apron, MVP-1 on its pad over
// the flame trench, the gantry and its T-minus clock, and the jet bridge.
func moonshotPad(b *mapgen.Builder) {
	dx := moonshotDock[0]
	// The apron (out of reach: you only see it from the windows and the
	// bridge) round the trench under MVP-1 (x dx-10..dx+10, z -8..8).
	b.Add(c.B(-199, -1, -60, -125, 0, -8, c.Concrete), c.B(-199, -1, 8, -125, 0, 60, c.Concrete),
		c.B(-199, -1, -8, dx-10, 0, -2, c.Concrete), c.B(-199, -1, 2, dx-10, 0, 8, c.Concrete), c.B(dx+10, -1, -8, -125, 0, 8, c.Concrete), // the bridge's floor between
		c.B(dx-10, -11, -8, dx+10, -10, 8, c.Hazard),
		c.B(dx-11, -11, -9, dx-10, -1, 9, c.Concrete), c.B(dx+10, -11, -9, dx+11, -1, 9, c.Concrete),
		c.B(dx-10, -11, -9, dx+10, -1, -8, c.Concrete), c.B(dx-10, -11, 8, dx+10, -1, 9, c.Concrete),
		c.B(-300, -1.25, -80, -125, -1.05, 80, c.Sand)) // Earth beyond the terminal
	b.Pit("the flame trench", dx-9.65, -10, -7.65, dx+9.65, -2, 7.65)
	b.Add(c.B(-125.5, 0, -60, -125, 1.2, 60, c.Concrete)) // the apron's end over the void
	// The jet bridge: from the window wall to MVP-1's door sill (x dx-8.5),
	// glass sides, a roof; its end shut by a door while MVP-1 is away.
	end := dx - 8.5
	b.Add(c.B(-200, -1, -2, end, 0, 2, c.Metal), c.B(-199, 0, -2.4, end, 2.6, -2, c.Glass), c.B(-199, 0, 2, end, 2.6, 2.4, c.Glass),
		c.B(-199, 2.6, -2.4, end, 3, 2.4, c.Metal))
	tt := moonshotTimetable()
	b.Mover(c.Mover{Kind: c.MoverPath, Mat: c.Hazard, Min: [3]float64{end - 0.5, 0, -2}, Max: [3]float64{end - 0.1, 2.6, 2}, Period: tt.Period,
		Path: [][4]float64{{0, 0, 0, 0}}, Hide: [2]float64{0, tt.Close}}) // a still path mover: there unless MVP-1 is docked with its doors open
	b.Sign(c.Sign{Text: "MIND THE GAP", Pos: [3]float64{end - 2.5, 1.9, -2}, Face: c.SignPZ, Size: 0.16, Mat: c.SignInk})
	// MVP-1 (after the bridge door: the rocket's movers come after it).
	b.Ship(prefabs.Rocket("MVP-1", dx, 0, 0, tt))
	// The gantry: a lattice tower south of the boosters, 120 m, and on its
	// face to the terminal the T-minus clock.
	gx0, gx1, gz0, gz1 := dx-2.5, dx+2.5, -16.0, -11.0
	for _, p := range [][2]float64{{gx0, gz0}, {gx1, gz0}, {gx0, gz1}, {gx1, gz1}} {
		b.Add(c.B(p[0]-0.3, 0, p[1]-0.3, p[0]+0.3, 120, p[1]+0.3, c.Metal))
	}
	for y := 10.0; y <= 120; y += 10 {
		b.Add(c.B(gx0+0.3, y-0.4, gz0-0.25, gx1-0.3, y, gz0+0.25, c.Hazard), c.B(gx0+0.3, y-0.4, gz1-0.25, gx1-0.3, y, gz1+0.25, c.Hazard),
			c.B(gx0-0.25, y-0.4, gz0+0.3, gx0+0.25, y, gz1-0.3, c.Hazard), c.B(gx1-0.25, y-0.4, gz0+0.3, gx1+0.25, y, gz1-0.3, c.Hazard))
	}
	b.Add(c.B(gx0-0.6, 22, gz0-3, gx0-0.3, 32, gz1+3, c.Flap))
	b.Sign(c.Sign{Text: "T-{departure:MVP-1}", Pos: [3]float64{gx0 - 0.6, 28.5, (gz0 + gz1) / 2}, Face: c.SignNX, Size: 2.2, Mat: c.SignLed, Live: true})
	b.Sign(c.Sign{Text: "PENDING CODE REVIEW", Pos: [3]float64{gx0 - 0.6, 24, (gz0 + gz1) / 2}, Face: c.SignNX, Size: 0.5, Mat: c.SignLed})
	// Pad lights on the stack.
	for _, z := range []float64{-20, 20} {
		l := c.Spot([3]float64{dx - 14, 2, z / 2}, [3]float64{14, 30, -z / 2}, [3]float64{1, 0.9, 0.75}, 30, 40, 22)
		b.Light(l)
		b.Add(c.B(dx-14.6, 0, z/2-0.6, dx-13.4, 2, z/2+0.6, c.Lamp))
	}
}

// moonshotSurface is the Moon's open ground west of the campus wall (x
// 75..170): the drop zone (craters, boulders), the solar farm to the south,
// the rover parking to the north.
func moonshotSurface(b *mapgen.Builder) {
	b.Add(c.B(75, 0, -82, 302, mTop, 82, c.Regolith)) // the plateau
	rim := func(x0, z0, x1, z1 float64) { b.Add(c.B(x0, mTop, z0, x1, mTop+1.2, z1, c.Regolith)) }
	rim(75, -82, 302, -81)
	rim(75, 81, 302, 82)
	rim(301, -81, 302, 81)
	// Craters round the drop zone (low rims) and boulders for cover.
	for _, cr := range [][3]float64{{100, -14, 5}, {142, 16, 4}, {96, 22, 3.5}, {148, -20, 4.5}} {
		x, z, r := cr[0], cr[1], cr[2]
		b.Add(c.B(x-r, mTop, z-r, x+r, mTop+0.5, z-r+0.6, c.Regolith), c.B(x-r, mTop, z+r-0.6, x+r, mTop+0.5, z+r, c.Regolith),
			c.B(x-r, mTop, z-r+0.6, x-r+0.6, mTop+0.5, z+r-0.6, c.Regolith), c.B(x+r-0.6, mTop, z-r+0.6, x+r, mTop+0.5, z+r-0.6, c.Regolith))
	}
	for _, r := range [][5]float64{{108, -4, 2.2, 1.6, 2}, {132, 6, 2.6, 1.8, 2.4}, {126, -10, 1.6, 1.2, 1.8}, {104, 9, 1.8, 1.4, 1.6}, {150, 2, 3, 2.2, 2.6}} {
		b.Add(c.B(r[0]-r[2]/2, mTop, r[1]-r[4]/2, r[0]+r[2]/2, mTop+r[3], r[1]+r[4]/2, c.Regolith))
	}
	// The welcome billboard facing the drop zone.
	b.Add(c.B(160, mTop, -10, 160.6, mTop+7, -9.4, c.Metal), c.B(160, mTop, 9.4, 160.6, mTop+7, 10, c.Metal),
		c.B(159.8, mTop+3.5, -11, 160, mTop+7.5, 11, c.Poster))
	b.Sign(c.Sign{Text: "WELCOME TO THE LUNAR OFFICE CAMPUS\nRETURN TO OFFICE IS MANDATORY\nVISITORS MUST BADGE IN", Pos: [3]float64{159.8, mTop + 5.2, 0}, Face: c.SignNX, Size: 0.5, Mat: c.SignInk})
	// The solar farm: tilted panel rows (cover) round point B (115, -52).
	for _, x := range []float64{98, 103, 127, 132} {
		for _, zz := range [][2]float64{{-66, -55}, {-49, -38}} {
			b.Add(c.W(x, mTop, zz[0], x+3, mTop+1.5, zz[1], c.Glass, c.RampPX))
		}
	}
	b.Add(c.B(110, mTop, -72, 120, mTop+2.4, -70, c.Container))
	b.Sign(c.Sign{Text: "SOLAR FARM · PLEASE DO NOT STAND IN THE SUN", Pos: [3]float64{115, mTop + 1.6, -70}, Face: c.SignPZ, Size: 0.22, Mat: c.SignInk})
	// The rover parking: rovers in bays round point C (115, 52), the VP's
	// reserved bay, kerbs.
	rover := func(x, z float64) {
		b.Add(c.B(x-1.1, mTop+0.5, z-1.8, x+1.1, mTop+1.5, z+1.8, c.Car), c.B(x-0.9, mTop+1.5, z-0.8, x+0.9, mTop+2.1, z+0.6, c.Glass))
		for _, w := range [][2]float64{{-1.2, -1.2}, {1.2, -1.2}, {-1.2, 1.2}, {1.2, 1.2}} {
			b.Add(c.B(x+w[0]-0.25, mTop, z+w[1]-0.45, x+w[0]+0.25, mTop+0.9, z+w[1]+0.45, c.Metal))
		}
	}
	for _, x := range []float64{97, 102, 128, 133} {
		rover(x, 44)
		rover(x, 61)
	}
	b.Add(c.B(92, mTop, 52, 105, mTop+0.15, 52.3, c.Hazard), c.B(125, mTop, 52, 138, mTop+0.15, 52.3, c.Hazard))
	b.Add(c.B(114.7, mTop, 71, 115.3, mTop+3, 71.6, c.Metal), c.B(112.5, mTop+2, 70.9, 117.5, mTop+3.2, 71, c.Poster))
	b.Sign(c.Sign{Text: "RESERVED\nVP OF SYNERGY", Pos: [3]float64{115, mTop + 2.6, 70.9}, Face: c.SignNZ, Size: 0.32, Mat: c.SignInk})
}

// moonshotCampus is the Lunar Office Campus behind its wall at x 170: the
// badge gates, the quad, the Lunar Compliance Dept. (south), the cafeteria
// (north), the promenade between them to HQ, and HQ's lobby with its
// elevator banks (the Moon's main).
func moonshotCampus(b *mapgen.Builder) {
	const wx0, wx1, wh = 170.0, 172.0, 6.0
	// The wall: the badge gates in the middle (z -6..6, under a lintel),
	// staff gates to the south and north.
	b.Add(c.B(wx0, mTop, -81, wx1, mTop+wh, -46, c.Concrete), c.B(wx0, mTop, -40, wx1, mTop+wh, -6, c.Concrete),
		c.B(wx0, mTop, 6, wx1, mTop+wh, 40, c.Concrete), c.B(wx0, mTop, 46, wx1, mTop+wh, 81, c.Concrete),
		c.B(wx0, mTop+4, -6, wx1, mTop+wh, 6, c.Concrete))
	for _, z := range []float64{-4.5, -1.5, 1.5, 4.5} { // turnstiles, a badge reader on each
		b.Add(c.B(wx0-1, mTop, z-0.15, wx1+1, mTop+1, z+0.15, c.Metal), c.B(wx0-0.9, mTop+1, z-0.12, wx0-0.6, mTop+1.3, z+0.12, c.Screen))
	}
	b.Sign(c.Sign{Text: "LUNAR COMPLIANCE DEPT. · BADGE IN", Pos: [3]float64{wx0, mTop + 5, 0}, Face: c.SignNX, Size: 0.6, Mat: c.SignInk})
	b.Sign(c.Sign{Text: "STAFF ONLY", Pos: [3]float64{wx0, mTop + 3, -43}, Face: c.SignNX, Size: 0.4, Mat: c.SignInk})
	b.Sign(c.Sign{Text: "STAFF ONLY", Pos: [3]float64{wx0, mTop + 3, 43}, Face: c.SignNX, Size: 0.4, Mat: c.SignInk})
	poster := func(x, y, z float64, face, text string) {
		const w, h = 3.0, 4.0
		switch face {
		case c.SignPX:
			b.Add(c.B(x, y-h/2, z-w/2, x+0.05, y+h/2, z+w/2, c.Poster))
			x += 0.05
		case c.SignNX:
			b.Add(c.B(x-0.05, y-h/2, z-w/2, x, y+h/2, z+w/2, c.Poster))
			x -= 0.05
		case c.SignPZ:
			b.Add(c.B(x-w/2, y-h/2, z, x+w/2, y+h/2, z+0.05, c.Poster))
			z += 0.05
		default:
			b.Add(c.B(x-w/2, y-h/2, z-0.05, x+w/2, y+h/2, z, c.Poster))
			z -= 0.05
		}
		b.Sign(c.Sign{Text: text, Pos: [3]float64{x, y + 1.25, z}, Face: face, Size: 0.16, Mat: c.SignInk})
	}
	poster(wx1, mTop+2.8, -25, c.SignPX, "SYNERGY HAS\nNO ATMOSPHERE")
	poster(wx1, mTop+2.8, 25, c.SignPX, "RETURN TO OFFICE:\nIT'S ONLY\n384,400 KM")

	// The quad (x 172..210): standing desks scattered as cover.
	for _, d := range [][2]float64{{180, -30}, {186, -14}, {181, 4}, {192, 18}, {184, 34}, {198, -40}, {200, -4}, {196, 44}, {190, -60}, {188, 62}, {204, 26}, {178, 50}} {
		b.Add(c.B(d[0]-0.4, mTop, d[1]-0.9, d[0]+0.4, mTop+1.15, d[1]+0.9, c.Desk), c.B(d[0]-0.3, mTop+1.15, d[1]-0.3, d[0]-0.25, mTop+1.55, d[1]+0.3, c.Screen))
	}

	// A roofed building x0..x1, z0..z1, h tall, with doorways (x walls: z
	// ranges; z walls: x ranges), lit by ceiling panels.
	building := func(x0, z0, x1, z1, h float64, west, east, south, north [][2]float64) {
		wall := func(fixed0, fixed1 float64, along0, along1 float64, doors [][2]float64, xWall bool) {
			cur := along0
			put := func(a0, a1 float64) {
				if a1-a0 < 0.05 {
					return
				}
				if xWall {
					b.Add(c.B(fixed0, mTop, a0, fixed1, mTop+h, a1, c.Plaster))
				} else {
					b.Add(c.B(a0, mTop, fixed0, a1, mTop+h, fixed1, c.Plaster))
				}
			}
			for _, d := range doors {
				put(cur, d[0])
				if xWall {
					b.Add(c.B(fixed0, mTop+3, d[0], fixed1, mTop+h, d[1], c.Plaster))
				} else {
					b.Add(c.B(d[0], mTop+3, fixed0, d[1], mTop+h, fixed1, c.Plaster))
				}
				cur = d[1]
			}
			put(cur, along1)
		}
		wall(x0, x0+0.5, z0, z1, west, true)
		wall(x1-0.5, x1, z0, z1, east, true)
		wall(z0, z0+0.5, x0+0.5, x1-0.5, south, false)
		wall(z1-0.5, z1, x0+0.5, x1-0.5, north, false)
		b.Add(c.B(x0, mTop+h, z0, x1, mTop+h+0.5, z1, c.Concrete), c.B(x0+0.5, mTop, z0+0.5, x1-0.5, fl, z1-0.5, c.Carpet))
		for x := x0 + 6; x < x1-3; x += 10 {
			for z := z0 + 6; z < z1-3; z += 10 {
				l := c.Spot([3]float64{x, mTop + h - 0.3, z}, [3]float64{0, -1, 0}, [3]float64{0.92, 0.96, 1}, 18, 16, 80)
				l.Inner = 35
				b.Light(l)
				b.Add(c.B(x-0.6, mTop+h-0.15, z-0.3, x+0.6, mTop+h, z+0.3, c.Lamp))
			}
		}
	}

	// The Lunar Compliance Dept. (x 210..255, z -46..4): open plan, rows of
	// standing desks round point D (232, -24); doors west, north (to the
	// promenade) and south.
	building(210, -46, 255, 4, 5, [][2]float64{{-28, -20}}, nil, [][2]float64{{228, 236}}, [][2]float64{{216, 222}, {244, 250}})
	for _, x := range []float64{215, 219, 245, 249} {
		for _, z := range []float64{-40, -34, -14, -8} {
			b.Add(c.B(x-0.4, fl, z-1.2, x+0.4, mTop+1.15, z+1.2, c.Desk), c.B(x-0.3, mTop+1.15, z-0.4, x-0.25, mTop+1.55, z+0.4, c.Screen))
		}
	}
	b.Add(c.B(226, fl, -42, 238, mTop+1.15, -41.2, c.Desk), c.B(226, fl, -7, 238, mTop+1.15, -6.2, c.Desk))
	b.Sign(c.Sign{Text: "LUNAR COMPLIANCE DEPT.", Pos: [3]float64{232, mTop + 4.2, 3.5}, Face: c.SignPZ, Size: 0.45, Mat: c.SignInk})
	poster(254.5, mTop+2.4, -24, c.SignNX, "YOUR BADGE\nIS YOUR\nIDENTITY")
	poster(210.5, mTop+2.4, -38, c.SignPX, "CAMERAS ON.\nHELMETS ON.")
	// The cafeteria (x 210..255, z 14..52): tables and benches round point E
	// (232, 33), the pudding counter on the north wall.
	building(210, 14, 255, 52, 5, [][2]float64{{28, 36}}, nil, [][2]float64{{216, 222}, {244, 250}}, [][2]float64{{230, 236}})
	for _, x := range []float64{216, 248} {
		for _, z := range []float64{22, 30, 38, 46} {
			b.Add(c.B(x-1.5, fl, z-0.6, x+1.5, mTop+0.75, z+0.6, c.Desk), c.B(x-1.5, fl, z-1.4, x+1.5, mTop+0.45, z-1.0, c.Awning), c.B(x-1.5, fl, z+1.0, x+1.5, mTop+0.45, z+1.4, c.Awning))
		}
	}
	b.Add(c.B(214, fl, 49, 228, mTop+1.1, 51.5, c.Checkout))
	b.Sign(c.Sign{Text: "TODAY: ZERO-G PUDDING", Pos: [3]float64{221, mTop + 2.6, 51.5}, Face: c.SignNZ, Size: 0.4, Mat: c.SignLed})
	b.Sign(c.Sign{Text: "CAFETERIA", Pos: [3]float64{232, mTop + 4.2, 14.5}, Face: c.SignNZ, Size: 0.45, Mat: c.SignInk})
	poster(254.5, mTop+2.4, 33, c.SignNX, "LUNCH IS\nA MEETING")

	// HQ (x 262..300, z -30..30, 10 m): the lobby, the moon logo over the
	// door, the reception desk, the elevator banks on the east wall.
	const hx0, hx1, hz0, hz1, hh = 262.0, 300.0, -30.0, 30.0, 10.0
	b.Add(c.B(hx0, mTop, hz0, hx0+0.6, mTop+hh, -4, c.Plaster), c.B(hx0, mTop, 4, hx0+0.6, mTop+hh, hz1, c.Plaster),
		c.B(hx0, mTop+3.5, -4, hx0+0.6, mTop+hh, 4, c.Plaster),
		c.B(hx0+0.6, mTop, hz0, hx1, mTop+hh, hz0+0.6, c.Plaster), c.B(hx0+0.6, mTop, hz1-0.6, 275, mTop+hh, hz1, c.Plaster),
		c.B(281, mTop, hz1-0.6, hx1, mTop+hh, hz1, c.Plaster), c.B(275, mTop+3.5, hz1-0.6, 281, mTop+hh, hz1, c.Plaster),
		c.B(hx0, mTop+hh, hz0, hx1, mTop+hh+0.6, hz1, c.Concrete), c.B(hx0+0.6, mTop, hz0+0.6, hx1-0.6, mTop+0.02, hz1-0.6, c.Carpet))
	// The east wall: three elevator doors (the Moon respawns in front of them).
	cars := [][2]float64{{-9, -5}, {-2, 2}, {5, 9}}
	zc := hz0 + 0.6
	for _, cz := range cars {
		b.Add(c.B(hx1-0.6, mTop, zc, hx1, mTop+hh, cz[0], c.Plaster))
		zc = cz[1]
	}
	b.Add(c.B(hx1-0.6, mTop, zc, hx1, mTop+hh, hz1-0.6, c.Plaster))
	for _, cz := range cars {
		b.Add(c.B(hx1-0.6, mTop+3.2, cz[0], hx1, mTop+hh, cz[1], c.Plaster), // over the door
			c.B(hx1-0.3, mTop, cz[0], hx1, mTop+3.2, cz[1], c.Metal), // the car's doors (just opened: you are spat out of it)
			c.B(hx1-0.75, mTop+3.25, cz[0]+0.5, hx1-0.6, mTop+3.9, cz[1]-0.5, c.Flap))
		b.Sign(c.Sign{Text: "LOBBY", Pos: [3]float64{hx1 - 0.75, mTop + 3.57, (cz[0] + cz[1]) / 2}, Face: c.SignNX, Size: 0.3, Mat: c.SignLed})
	}
	b.Add(c.B(274, mTop+0.02, -6, 276, mTop+1.1, 6, c.Desk), c.B(275.5, mTop+1.1, -1, 275.6, mTop+1.6, 1, c.Screen)) // reception, on the carpet
	for _, x := range []float64{270, 286} {                                                                          // the lobby's chandeliers
		for _, z := range []float64{-16, 0, 16} {
			b.Light(c.Point([3]float64{x, mTop + 8.5, z}, [3]float64{1, 0.95, 0.85}, 16, 20))
			b.Add(c.B(x-0.7, mTop+hh-0.2, z-0.7, x+0.7, mTop+hh, z+0.7, c.Lamp))
		}
	}
	b.Sign(c.Sign{Text: "HQ", Pos: [3]float64{hx0, mTop + 8.6, 0}, Face: c.SignNX, Size: 1.6, Mat: c.SignInk})
	// The moon logo: a cratered disc of slabs over the entrance.
	for i := -5; i <= 5; i++ {
		y := mTop + 5.5 + float64(i)*0.22
		w := math.Sqrt(math.Max(0, 1.3*1.3-float64(i*i)*0.0484*1.0))
		if w > 0 {
			b.Add(c.B(hx0-0.25, y-0.11, -8-w, hx0, y+0.11, -8+w, c.Regolith), c.B(hx0-0.25, y-0.11, 8-w, hx0, y+0.11, 8+w, c.Regolith))
		}
	}
	b.Sign(c.Sign{Text: "LUNAR OFFICE CAMPUS", Pos: [3]float64{hx0, mTop + 4.2, 0}, Face: c.SignNX, Size: 0.5, Mat: c.SignInk})
	poster(hx0+0.6, mTop+2.6, -18, c.SignPX, "THE MOON IS\nA PLACE OF\nWORK")
	poster(hx0+0.6, mTop+2.6, 18, c.SignPX, "DESKS ARE\nFOR\nSTANDING")
}
