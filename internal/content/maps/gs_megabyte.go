package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	p "github.com/gigurra/counterslop/internal/content/prefabs"
)

// GsMegabyte is MegaByte Mart, a computer superstore on three floors under
// one roof (docs/gpu-drop.md): the scalpers (Vibe Coders) come in from the
// parking lot for the last boxed GPUs; the staff (Artisans) hold the
// stockroom in the basement.
//
//   - Ground floor (y 0): the storefront and entrance on the south, checkout
//     lanes, the gaming section (west: demo kiosks, the TV wall), "RGB
//     Everything" (east: gondola canyons), the atrium in the middle with two
//     escalator pairs up. Behind the back wall: the staff corridor, receiving,
//     the break room, both stairwells and the freight lift (out of order: its
//     car is stuck halfway, so it's a one-way drop to the basement).
//   - Upper floor (y 5): the PC build room and the LAN cafe (west), the
//     component aisles (east: Aisle 404, RAM Lane...), the Byte Bistro food
//     court (north), clearance (south) and a catwalk across the atrium.
//   - Basement (y -5): the stockroom (west, the GPUs, the staff's spawn), the
//     server room, the Cable Graveyard, the corridor and the loading dock
//     (east) opening onto the sunken truck bay: one exit. The other is the
//     front entrance.
//
// The ways down: stairwell W (into the stockroom), the lift shaft drop (into
// the corridor), stairwell E and the truck ramp (into the dock).
func GsMegabyte() c.Map {
	b := mapgen.New("gs_megabyte")
	const (
		base  = -5.0 // basement floor
		upper = 5.0  // upper floor
		roof  = 10.0 // roof underside
		slab  = 0.4  // floor slabs
	)
	world := mapgen.Rect{X0: -80, Z0: -80, X1: 80, Z1: 50}
	store := mapgen.Rect{X0: -56, Z0: -40, X1: 56, Z1: 40}    // outer walls included
	inner := mapgen.Rect{X0: -55, Z0: -39, X1: 55, Z1: 39}    // inside the walls
	cellar := mapgen.Rect{X0: -56, Z0: 4, X1: 56, Z1: 40}     // the basement's footprint
	bay := mapgen.Rect{X0: 56, Z0: 5, X1: 76, Z1: 39}         // the sunken truck bay (exit)
	ramp := mapgen.Rect{X0: 62, Z0: -11, X1: 72, Z1: 5}       // the truck ramp down to it
	stairW := mapgen.Rect{X0: -55, Z0: 31.6, X1: -45, Z1: 39} // stairwell footprints
	stairE := mapgen.Rect{X0: 45, Z0: 31.6, X1: 55, Z1: 39}
	shaft := mapgen.Rect{X0: 6, Z0: 33, X1: 12, Z1: 39} // the freight lift

	// ---- outside: the lot, the lanes round the store, the truck bay ----
	b.Add(
		c.B(world.X0-1, -6, world.Z0-1, world.X1+1, 12, world.Z0, c.Concrete),
		c.B(world.X0-1, -6, world.Z1, world.X1+1, 12, world.Z1+1, c.Concrete),
		c.B(world.X0-1, -6, world.Z0, world.X0, 12, world.Z1, c.Concrete),
		c.B(world.X1, -6, world.Z0, world.X1+1, 12, world.Z1, c.Concrete),
	)
	for _, r := range mapgen.Fill(world, []mapgen.Rect{store, bay, ramp}) {
		b.Add(c.B(r.X0, -6, r.Z0, r.X1, 0, r.Z1, c.Concrete))
	}
	b.Add(c.B(bay.X0, -6, bay.Z0, bay.X1, base, bay.Z1, c.Concrete), c.B(ramp.X0, -6, ramp.Z0, ramp.X1, base, ramp.Z1, c.Concrete))
	b.Add(p.Flight(true, ramp.Z0, ramp.X0, ramp.X1, base, 1.6, 0.5, 10, 1, c.Hazard)...)
	b.Add(p.Container(64, base, 30, true, true, 1)) // a trailer at the dock
	// The parking lot: two rows of cars, a cart corral, light poles.
	for i, x := range []float64{-60, -52, -44, -30, -22, -14, 14, 22, 30, 44, 52, 60} {
		for j, z := range []float64{-54, -66} {
			if (i*7+j*3)%5 == 1 {
				continue // a free space
			}
			b.Add(car(x, z, (i+j)%2 == 0)...)
		}
	}
	b.Add(c.B(-36, 0, -60, -32, 1.0, -59.8, c.Metal), c.B(32, 0, -60, 36, 1.0, -59.8, c.Metal)) // cart corrals
	for _, x := range []float64{-40, 40} {
		b.Add(c.B(x-0.15, 0, -60.15, x+0.15, 8, -59.85, c.Metal))
	}
	// The north service alley: dumpsters.
	b.Add(p.Crate(-30, 0, 43, 4, 1.6, 2), p.Crate(20, 0, 43, 4, 1.6, 2))

	// ---- the building shell ----
	// Below the sales floor: solid ground. The basement under the back.
	b.Add(c.B(store.X0, -6, store.Z0, store.X1, 0, cellar.Z0, c.StoreTile))
	b.Add(c.B(cellar.X0, -6, cellar.Z0, cellar.X1, base, cellar.Z1, c.Concrete))
	cellarRooms := []mapgen.Rect{
		{X0: -55, Z0: 5, X1: -24, Z1: 39},   // the stockroom
		{X0: -24, Z0: 8, X1: -18, Z1: 11},   //   its side door into the Cable Graveyard
		{X0: -24, Z0: 26, X1: 22, Z1: 31},   // the corridor
		{X0: -18, Z0: 5, X1: 20, Z1: 24},    // the Cable Graveyard
		{X0: -16, Z0: 24, X1: -13, Z1: 26},  //   its doors to the corridor
		{X0: 14, Z0: 24, X1: 17, Z1: 26},    //
		{X0: -18, Z0: 33, X1: 4, Z1: 39},    // the server room
		{X0: -8, Z0: 31, X1: -5, Z1: 33},    //   its door
		{X0: 7.5, Z0: 31, X1: 10.5, Z1: 33}, // the lift shaft's basement door
		{X0: 20, Z0: 10, X1: 22, Z1: 14},    //   and into the dock
		{X0: 22, Z0: 5, X1: 55, Z1: 39},     // the loading dock
		{X0: 55, Z0: 10, X1: 56, Z1: 16},    //   its three bay doors
		{X0: 55, Z0: 20, X1: 56, Z1: 26},    //
		{X0: 55, Z0: 28, X1: 56, Z1: 34},    //
		shaft,
	}
	for _, r := range mapgen.Fill(cellar, cellarRooms) {
		b.Add(c.B(r.X0, base, r.Z0, r.X1, -slab, r.Z1, c.Concrete))
	}
	// The ground floor over the basement, open at the stairwells and the shaft.
	for _, r := range mapgen.Fill(cellar, []mapgen.Rect{stairW, stairE, shaft}) {
		b.Add(floor(r, 0, c.StoreTile, c.Concrete)...)
	}
	// The upper floor: open over the atrium and the escalators.
	escW := mapgen.Rect{X0: -21.15, Z0: 4 - p.EscalatorLen(upper), X1: -18.15, Z1: 4}
	escE := mapgen.Rect{X0: 18.15, Z0: -5, X1: 21.15, Z1: -5 + p.EscalatorLen(upper)}
	atrium := mapgen.Rect{X0: -12, Z0: -14, X1: 12, Z1: 10}
	lan := []mapgen.Rect{{X0: inner.X0, Z0: -8, X1: -24, Z1: stairW.Z0}, {X0: stairW.X1, Z0: stairW.Z0, X1: -24, Z1: inner.Z1}} // the LAN cafe: carpet
	for _, r := range mapgen.Fill(inner, append([]mapgen.Rect{atrium, escW, escE, stairW, stairE}, lan...)) {
		b.Add(floor(r, upper, c.StoreTile, c.Plaster)...)
	}
	for _, r := range lan {
		b.Add(floor(r, upper, c.LanCarpet, c.Plaster)...)
	}
	b.Add(p.Roof(inner, []mapgen.Rect{{X0: -10, Z0: -12, X1: 10, Z1: 8}}, roof, 0.5, c.Concrete)...)
	// Outer walls, 0..11 (a parapet above the roof): high window bands on
	// both floors (sills 2 m up), the storefront and doors on the south, a
	// fire door on the west.
	wall := func(alongX bool, at, a0, a1 float64, door *p.Gap) {
		low, band := []p.Gap(nil), windows(a0, a1)
		if door != nil { // a door, and the window band open above it
			low = []p.Gap{*door}
			band = append(band, *door)
			sortGaps(band)
		}
		b.Add(p.WallGaps(alongX, at, a0, a1, 1, 0, 2, c.Concrete, low...)...)
		b.Add(p.WallGaps(alongX, at, a0, a1, 1, 2, 1.8, c.Concrete, band...)...)
		b.Add(p.WallGaps(alongX, at, a0, a1, 1, 3.8, 3.2, c.Concrete)...)
		b.Add(p.WallGaps(alongX, at, a0, a1, 1, 7, 1.8, c.Concrete, windows(a0, a1)...)...)
		b.Add(p.WallGaps(alongX, at, a0, a1, 1, 8.8, 2.2, c.Concrete)...)
	}
	wall(true, store.Z1-1, store.X0, store.X1, nil)
	wall(false, store.X0, store.Z0+1, store.Z1-1, &p.Gap{From: -2, To: 1}) // the fire door
	wall(false, store.X1-1, store.Z0+1, store.Z1-1, nil)
	// The south front: storefront glass either side of the entrance (a 3 m
	// lintel over the doors), then the upper floor's window band.
	b.Add(p.WallGaps(true, store.Z0, store.X0, store.X1, 1, 0, 3.8, c.Concrete,
		p.Gap{From: -40, To: -10}, p.Gap{From: -8, To: 8, H: 3}, p.Gap{From: 10, To: 40})...)
	b.Add(c.B(-40, 0, -39.55, -10, 3.8, -39.45, c.Glass), c.B(10, 0, -39.55, 40, 3.8, -39.45, c.Glass))
	b.Add(p.WallGaps(true, store.Z0, store.X0, store.X1, 1, 3.8, 3.2, c.Concrete)...)
	b.Add(p.WallGaps(true, store.Z0, store.X0, store.X1, 1, 7, 1.8, c.Concrete, windows(store.X0, store.X1)...)...)
	b.Add(p.WallGaps(true, store.Z0, store.X0, store.X1, 1, 8.8, 2.2, c.Concrete)...)

	// ---- ground floor ----
	const g = 0.0
	// The back wall: staff doors W, by the lift, E.
	b.Add(p.WallGaps(true, 24, inner.X0, inner.X1, 1, g, upper-slab, c.Plaster,
		p.Gap{From: -42, To: -39, H: 2.4}, p.Gap{From: 7, To: 10, H: 2.4}, p.Gap{From: 39, To: 42, H: 2.4})...)
	// Back of house: the staff corridor (z 25..31.6), receiving, the lift, the break room.
	b.Add(p.WallGaps(true, 31.4, -45, stairE.X0, 0.2, g, upper-slab, c.Plaster,
		p.Gap{From: -30, To: -27, H: 2.4}, p.Gap{From: shaft.X0 + 1.5, To: shaft.X1 - 1.5, H: 2.4}, p.Gap{From: 26, To: 29, H: 2.4})...)
	b.Add(c.B(4, g, 31.6, 6, upper-slab, 39, c.Plaster), c.B(12, g, 31.6, 14, upper-slab, 39, c.Plaster))
	b.Add(p.Crate(-40, g, 34, 2, 1.5, 2), p.Crate(-37.5, g, 35, 1.5, 1, 1.5), p.Crate(-20, g, 33, 2, 2, 2)) // receiving
	b.Add(p.Desk(20, g, 34, 3, 1.4, false)...)                                                              // the break room
	// The lift shaft (its basement walls are the basement fill): the car stuck at -2.5.
	b.Add(c.B(shaft.X0+0.2, -2.8, shaft.Z0+0.2, shaft.X1-0.2, -2.5, shaft.Z1-2.2, c.Hazard)) // the stuck car (the back 2 m is open: drop again)
	// The checkout lanes, either side of the doors.
	for _, x := range []float64{-40, -35.5, -31, -26.5, -22, -17.5, -13, 12, 16.5, 21, 25.5, 30, 34.5, 39} {
		b.Add(p.CheckoutLane(x, g, -34, false)...)
	}
	// The atrium: the 9090 display tower in the middle; the escalators up.
	b.Add(c.B(-2, g, -4, 2, 0.6, 0, c.Metal), c.B(-1, 0.6, -3, 1, 3.2, -1, c.PCTower))
	// Each pair: an up lane and a down lane; the treads carry whoever stands on them.
	const escSpeed = 0.75 // m/s along the run: a real escalator's 0.5, a bit quicker for play
	escalator := func(edge, a0 float64, dir int, speed float64) {
		b.Add(p.Escalator(true, edge, a0, 1.2, g, upper, dir, c.Escalator, c.Glass)...)
		b.Mover(p.EscalatorMover(true, edge, a0, 1.2, g, upper, dir, speed, c.Escalator))
	}
	escalator(escW.Z1, -21, -1, escSpeed)
	escalator(escW.Z1, -19.5, -1, -escSpeed)
	escalator(escE.Z0, 18.3, 1, escSpeed)
	escalator(escE.Z0, 19.8, 1, -escSpeed)
	// The gaming section (west): demo kiosks, the TV wall.
	for _, x := range []float64{-49, -42, -35, -28} {
		for _, z := range []float64{-17, -9, 7, 15} {
			b.Add(p.Kiosk(x, g, z)...)
		}
	}
	b.Add(p.TVWall(inner.X0, -20, inner.X0+0.3, -4, g)...) // the TV wall, either side of the fire door
	b.Add(p.TVWall(inner.X0, 4, inner.X0+0.3, 20, g)...)
	// RGB Everything (east): gondola canyons with a cross aisle.
	for _, x := range []float64{27, 32, 37, 42, 47} {
		b.Add(p.Gondola(x, g, -20, 17.5, false)...)
		b.Add(p.Gondola(x, g, 2.5, 15.5, false)...)
	}

	// ---- upper floor ----
	const u = upper
	// Railings round the atrium (the catwalk crosses it at z -3..-1) and the escalator wells.
	b.Add(p.Catwalk(atrium.X0, -3, atrium.X1, -1, u, c.Metal, c.Metal)...)
	b.Add(
		p.Railing(atrium.X0, atrium.Z1, atrium.X1, atrium.Z1+0.1, u, c.Metal),
		p.Railing(atrium.X0, atrium.Z0-0.1, atrium.X1, atrium.Z0, u, c.Metal),
		p.Railing(atrium.X0-0.1, atrium.Z0, atrium.X0, -3, u, c.Metal),
		p.Railing(atrium.X0-0.1, -1, atrium.X0, atrium.Z1, u, c.Metal),
		p.Railing(atrium.X1, atrium.Z0, atrium.X1+0.1, -3, u, c.Metal),
		p.Railing(atrium.X1, -1, atrium.X1+0.1, atrium.Z1, u, c.Metal),
		p.Railing(escW.X0, escW.Z0-0.1, escW.X1, escW.Z0, u, c.Metal),
		p.Railing(escE.X0, escE.Z1, escE.X1, escE.Z1+0.1, u, c.Metal),
	)
	// West: the PC build room and the LAN cafe behind a partition with doors.
	b.Add(p.WallGaps(false, -24, -39, 39, 0.2, u, roof-u, c.Plaster,
		p.Gap{From: -30, To: -27, H: 2.4}, p.Gap{From: 0, To: 3, H: 2.4}, p.Gap{From: 16, To: 19, H: 2.4})...)
	b.Add(p.WallGaps(true, -10.2, inner.X0, -24, 0.2, u, roof-u, c.Plaster, p.Gap{From: -40, To: -37, H: 2.4})...)
	for _, z := range []float64{-34, -26, -18} {
		b.Add(p.Desk(-50, u, z, 3, 1.4, true)...)
		b.Add(p.Desk(-42, u, z, 3, 1.4, true)...)
		b.Add(p.Desk(-34, u, z, 3, 1.4, true)...)
	}
	for _, z := range []float64{-4, 4, 12, 20} {
		b.Add(p.LanDesk(-52, u, z, true, 12)...)
		b.Add(p.LanDesk(-38, u, z, true, 10)...)
	}
	// North: the Byte Bistro (tables, the counter).
	b.Add(c.B(-10, u, 34, 10, u+1.1, 35.2, c.Desk))
	for _, x := range []float64{-8, -2, 4, 10, 16} {
		for _, z := range []float64{16, 22, 28} {
			b.Add(c.B(x-0.6, u, z-0.6, x+0.6, u+0.75, z+0.6, c.Desk))
		}
	}
	// East: the component aisles.
	for _, x := range []float64{26, 31, 36, 41, 46, 51} {
		b.Add(p.Gondola(x, u, -36, 16, false)...)
		b.Add(p.Gondola(x, u, -16, 16, false)...)
		b.Add(p.Gondola(x, u, 4, 20, false)...)
	}
	// South: clearance (pallets, a cart pile).
	b.Add(p.Crate(-8, u, -30, 2, 1.2, 2), p.Crate(4, u, -24, 2.5, 1, 2), p.Crate(14, u, -32, 2, 1.5, 2))

	// ---- stairwells, both from the basement to the upper floor ----
	levels := []float64{base, g, u}
	b.Add(p.Stairwell(stairW.X0, stairW.Z0, levels, []bool{true, true, true}, roof-u, c.Concrete, c.Plaster)...)
	b.Add(p.Stairwell(stairE.X0, stairE.Z0, levels, []bool{true, true, true}, roof-u, c.Concrete, c.Plaster)...)

	// ---- basement ----
	// The stockroom: three rows of stock shelving (the GPUs on the middle shelves).
	b.Add(p.StockShelf(-50, base, 14.5, 22, true)...)
	b.Add(p.StockShelf(-50, base, 20.5, 22, true)...)
	b.Add(p.StockShelf(-44, base, 26.5, 16, true)...)
	b.Add(p.Crate(-38, base, 9, 2, 1.5, 2), p.Crate(-30, base, 34, 2, 2, 2))
	// The server room: two rack rows.
	b.Add(c.B(-15, base, 35, -2, base+2.2, 36, c.Rack), c.B(-15, base, 37.5, -2, base+2.2, 38.5, c.Rack))
	// The Cable Graveyard: spools and cartons in a maze.
	for _, s := range [][4]float64{{-10, 5.5, 3, 2}, {-6, 14, 4, 2}, {2, 7, 2, 4}, {8, 16, 3, 3}, {-12, 19, 2, 2}, {15, 5.5, 3, 2}} {
		b.Add(p.Crate(s[0], base, s[1], s[2], 1.6, s[3]))
	}
	// The dock: pallets.
	b.Add(p.Crate(30, base, 12, 2, 1.4, 2), p.Crate(40, base, 17, 2.5, 1.4, 2), p.Crate(34, base, 32, 2, 2, 2))

	// ---- the mode ----
	b.Mode(c.ModeGPUDrop).Category(c.GPUDrop)
	// On the stock shelves' middle shelves, the aisle (-z) half, between the uprights.
	b.GPU(-45, base+p.StockMid, 14.75, 0).GPU(-33, base+p.StockMid, 14.75, 0).GPU(-37, base+p.StockMid, 20.75, 0).GPU(-31, base+p.StockMid, 26.75, 0)
	b.Exit("front", -10, 0, -48, 10, 3, -40.5)
	b.Exit("dock", bay.X0+0.5, base, bay.Z0, bay.X1, base+3, bay.Z1)

	b.TeamSpawns(c.Vibe, [3]float64{-6, 0, -72}, [3]float64{-3, 0, -74}, [3]float64{0, 0, -72}, [3]float64{3, 0, -74}, [3]float64{6, 0, -72})
	b.TeamSpawns(c.Artisan, [3]float64{-50, base, 8}, [3]float64{-46, base, 8}, [3]float64{-42, base, 8}, [3]float64{-48, base, 11}, [3]float64{-44, base, 11})
	b.BuyAroundSpawns(3)

	b.Nav("vibe", 0, 0, -70, "lot").
		Nav("lot", 0, 0, -50, "front", "lotSE", "lotSW").
		Nav("front", 0, 0, -44, "doors").
		Nav("lotSE", 62, 0, -50, "lotE").
		Nav("lotSW", -62, 0, -50, "lotW").
		Nav("doors", 0, 0, -37, "sales").
		NavLane("sales", 0, 0, -22, "salesW", "atriumW", "atriumE").
		// west: the gaming section, the staff door, stairwell W down to the stock
		Nav("salesW", -20, 0, -24, "gaming", "escWb").
		NavLane("gaming", -38, 0, -13, "gamingN", "fire").
		Nav("gamingN", -38, 0, 20, "staffW").
		Nav("staffW", -40.5, 0, 28, "bohW", "boh").
		Nav("bohW", -53.8, 0, 28, "sWg").
		Nav("sWg", -54.2, 0, 34, "sWgB").
		Nav("sWgB", -54.2, 0, 37, "sWmid").
		Nav("sWmid", -47, -2.5, 37, "sWmidA").
		Nav("sWmidA", -47, -2.5, 33.5, "sWb").
		Nav("sWb", -54.2, base, 33.5, "sWbOut").
		Nav("sWbOut", -53.8, base, 29, "stockW3").
		// the stockroom: aisles between the shelf rows, joined at both ends
		Nav("stockW3", -52.5, base, 29.5, "stockN", "stockW2").
		Nav("stockW2", -52.5, base, 24, "aisle2", "stockW1").
		Nav("stockW1", -52.5, base, 18, "aisle1", "stockW0").
		Nav("stockW0", -52.5, base, 10.5, "aisle0").
		Nav("aisle0", -40, base, 12, "gpu1", "gpu2", "artisan", "stockSide").
		Nav("aisle1", -40, base, 18, "gpu3", "stockE1").
		Nav("aisle2", -36, base, 24, "gpu4", "stockE2").
		Nav("stockN", -40, base, 29.5, "stockE").
		Nav("gpu1", -45, base, 14.0).
		Nav("gpu2", -33, base, 14.0).
		Nav("gpu3", -37, base, 20.0).
		Nav("gpu4", -31, base, 26.0).
		Nav("artisan", -46, base, 9, "stockW0").
		Nav("stockSide", -26, base, 9.5, "grave", "stockE1").
		Nav("stockE1", -26, base, 18, "stockE2").
		Nav("stockE2", -26, base, 24, "stockE").
		Nav("stockE", -26, base, 28.5, "corr").
		// the corridor, the Cable Graveyard, the dock
		NavLane("corr", 0, base, 28.5, "corrE").
		Nav("corrE", 18, base, 28.5, "dock").
		NavLane("grave", 0, base, 12, "graveN", "graveE").
		Nav("graveN", -14.5, base, 22, "graveDoor").
		Nav("graveDoor", -14.5, base, 28.5, "corr").
		Nav("graveE", 18, base, 12, "dock").
		NavLane("dock", 36, base, 23, "bay", "sEb").
		Nav("bay", 66, base, 23, "rampB").
		Nav("rampB", 67, base, 7, "rampT").
		Nav("rampT", 67, 0, -13, "lotE").
		Nav("lotE", 62, 0, -30).
		Nav("lotW", -62, 0, -30, "westLane").
		NavLane("westLane", -62, 0, 0, "fire").
		Nav("fire", -50, 0, -0.5, "gaming").
		// the lift: a drop from the staff corridor
		Nav("atriumW", -14, 0, 6, "atriumN").
		Nav("atriumE", 14, 0, 6, "atriumN").
		Nav("atriumN", 0, 0, 16, "liftDoor").
		Nav("liftDoor", 8.5, 0, 22, "boh").
		// (the lift shaft is a one-way drop: players only, no nav)
		NavLane("boh", 8.5, 0, 28, "staffE").
		// east: the staff door, stairwell E down to the dock
		Nav("staffE", 40.5, 0, 28, "sEg").
		Nav("sEg", 45.8, 0, 28, "sEgIn").
		Nav("sEgIn", 45.8, 0, 34, "sEgB").
		Nav("sEgB", 45.8, 0, 37, "sEmid").
		Nav("sEmid", 53, -2.5, 37, "sEmidA").
		Nav("sEmidA", 53, -2.5, 33.5, "sEbIn").
		Nav("sEbIn", 45.8, base, 33.5, "sEb").
		Nav("sEb", 45.8, base, 29).
		// up the escalators: the upper floor's loop
		Nav("escWb", -20.4, 0, -7, "escWt").
		Nav("escWt", -20.4, u, 6, "upW", "lanN", "bistro").
		Nav("upW", -16, u, 6, "catwalkW").
		Nav("lanN", -22.5, u, 6, "lanDoor").
		Nav("lanDoor", -22.5, u, 1.5, "lan").
		NavLane("lan", -33, u, 1.5).
		Nav("catwalkW", -16, u, -2, "catwalk").
		NavLane("catwalk", 0, u, -2, "catwalkE").
		Nav("catwalkE", 15.5, u, -2, "upE").
		Nav("upE", 15.5, u, -8, "escEt2", "aislesW").
		Nav("aislesW", 23, u, -18, "aisles").
		NavLane("aisles", 34.5, u, -18).
		Nav("escEt2", 18.9, u, -7, "escEb").
		Nav("escEb", 18.9, 0, 6, "atriumE").
		Nav("bistro", 0, u, 20)
	b.Meta("GPU Drop. MegaByte Mart, a computer superstore on three floors: the scalpers come in from the parking lot, the staff hold the stockroom in the basement. Grab two GPUs and get them out the front or through the loading dock.", 24)
	b.FillSpawns(false) // one spawn per player, in the same spawn area
	megabyteLights(b)
	return b.Map()
}

// megabyteLights: ceiling panels on every floor (a lamp fixture at the
// ceiling, a spot straight down 10 cm under it: cool on the shop floor, warm
// back of house, purple in the LAN cafe, sodium in the dock, a dying tube in
// the stockroom), the server room's blue, spots on the 9090 tower and the TV
// wall, green exit signs. Values from lumen's calibration (lab_lights).
func megabyteLights(b *mapgen.Builder) {
	cool, warm := [3]float64{0.92, 0.96, 1}, [3]float64{1, 0.92, 0.8}
	// Panels in the play areas cast shadows (the client gives a shadow map to
	// the r_light_shadows most important of them in view); back of house doesn't.
	shadows := true
	panel := func(x, ceiling, z float64, col [3]float64, flicker uint32) {
		l := c.Spot([3]float64{x, ceiling - 0.2, z}, [3]float64{0, -1, 0}, col, 14, 9, 80)
		l.Inner, l.Flicker, l.Shadow = 35, flicker, shadows
		b.Light(l)
		b.Add(c.B(x-0.6, ceiling-0.1, z-0.3, x+0.6, ceiling, z+0.3, c.Lamp))
	}
	grid := func(ceiling, x0, x1, dx, z0, z1, dz float64, col func(x, z float64) ([3]float64, bool)) {
		for x := x0; x <= x1+1e-9; x += dx {
			for z := z0; z <= z1+1e-9; z += dz {
				if cl, ok := col(x, z); ok {
					panel(x, ceiling, z, cl, 0)
				}
			}
		}
	}
	open := func(x, z float64) bool { // not over the atrium's or the escalators' wells
		return !(x > -13 && x < 13 && z > -15 && z < 11) && !(x > -22 && x < -17 && z > -6 && z < 5) && !(x > 17 && x < 22 && z > -6 && z < 5)
	}
	// the shop floor (ceiling 4.6) and back of house
	grid(4.6, -48, 48, 12, -34, 20, 9, func(x, z float64) ([3]float64, bool) { return cool, open(x, z) })
	shadows = false
	grid(4.6, -42, 42, 12, 28, 28, 1, func(x, z float64) ([3]float64, bool) { return warm, true })
	shadows = true
	// the upper floor (ceiling 10: the roof)
	grid(10, -46, 50, 12, -34, 29, 9, func(x, z float64) ([3]float64, bool) {
		if x < -24 && z > -8 {
			return [3]float64{0.62, 0.32, 1}, true // the LAN cafe
		}
		return cool, open(x, z)
	})
	// the basement (ceiling -0.4), clear of the stairwells
	grid(-0.4, -50, -34, 8, 8, 36, 7, func(x, z float64) ([3]float64, bool) {
		return cool, !(x < -44 && z > 31) && (x != -34 || z != 22)
	}) // the stockroom
	panel(-34, -0.4, 22, cool, 404)                                                                               // its dying tube
	grid(-0.4, -18, 18, 12, 28.5, 28.5, 1, func(x, z float64) ([3]float64, bool) { return cool, true })           // the corridor
	grid(-0.4, -12, 14, 13, 10, 20, 10, func(x, z float64) ([3]float64, bool) { return warm, x != 1 || z != 20 }) // the Cable Graveyard
	panel(1, -0.4, 20, warm, 1337)
	grid(-0.4, 28, 50, 11, 10, 34, 12, func(x, z float64) ([3]float64, bool) { return [3]float64{1, 0.7, 0.35}, !(x > 44 && z > 31) }) // the dock: sodium
	blue := [3]float64{0.3, 0.5, 1}
	b.Light(c.Point([3]float64{-13, -1.2, 36.2}, blue, 6, 6), c.Point([3]float64{-4, -1.2, 36.2}, blue, 6, 6)) // the server room
	// display spots: the 9090 tower under the catwalk, the TV wall
	for _, s := range []c.Light{
		c.Spot([3]float64{0, 4.3, -6}, [3]float64{0, -1, 0.9}, [3]float64{1, 0.95, 0.85}, 30, 12, 25),
		c.Spot([3]float64{-46, 4.3, -12}, [3]float64{-1, -0.4, 0}, cool, 30, 12, 30),
		c.Spot([3]float64{-46, 4.3, 12}, [3]float64{-1, -0.4, 0}, cool, 30, 12, 30),
	} {
		s.Shadow = true
		b.Light(s)
	}
	// green exit signs: the front doors, the fire door, the dock's bay doors
	for _, s := range [][3]float64{{0, 3.3, -38.5}, {-54.5, 2.6, -0.5}, {54.5, -2.3, 13}, {54.5, -2.3, 23}, {54.5, -2.3, 31}} {
		b.Light(c.Point(s, [3]float64{0.2, 1, 0.4}, 2, 4))
	}
}

// windows: openings along a0..a1 every 8 m, 4 m wide, kept clear of the corners.
func windows(a0, a1 float64) []p.Gap {
	var out []p.Gap
	for w := a0 + 4; w+4 <= a1-2; w += 8 {
		out = append(out, p.Gap{From: w, To: w + 4})
	}
	return out
}

// sortGaps orders gaps along the wall (WallGaps walks them in order).
func sortGaps(g []p.Gap) {
	for i := 1; i < len(g); i++ {
		for j := i; j > 0 && g[j].From < g[j-1].From; j-- {
			g[j], g[j-1] = g[j-1], g[j]
		}
	}
}

// floor is a 0.4 m floor slab over r with its top at y: a thin top layer of
// mat over the ceiling below it (so the floor below doesn't see tiles overhead).
func floor(r mapgen.Rect, y float64, mat, ceiling string) []c.Box {
	return []c.Box{c.B(r.X0, y-0.05, r.Z0, r.X1, y, r.Z1, mat), c.B(r.X0, y-0.4, r.Z0, r.X1, y-0.05, r.Z1, ceiling)}
}

// car is a parked car (two boxes) at (x, z), along z.
func car(x, z float64, flip bool) []c.Box {
	cz := z + 1.2
	if flip {
		cz = z + 0.4
	}
	return []c.Box{c.B(x-0.95, 0, z-2.2, x+0.95, 1.0, z+2.2, c.Container), c.B(x-0.8, 1.0, cz-1.2, x+0.8, 1.6, cz+1.2, c.Metal)}
}
