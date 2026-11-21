package prefabs

import c "github.com/gigurra/counterslop/internal/content"

// Store props for the hardware superstore (gs_ maps). Each stands on the floor at y
// with its footprint's min corner at (x, z); alongX lays the long side along x (else
// z). Footprints are the contract with the map (docs/gpu-drop.md); every prop is
// z-fight free inside itself and against a floor at its base (store_test.go).

// rect is a footprint helper: a box spanning u0..u1 along the prop's long axis and
// v0..v1 across it, y0..y1 high, at corner (x, z).
func rect(x, z float64, alongX bool, u0, v0, u1, v1, y0, y1 float64, mat string) c.Box {
	if alongX {
		return c.B(x+u0, y0, z+v0, x+u1, y1, z+v1, mat)
	}
	return c.B(x+v0, y0, z+u0, x+v1, y1, z+u1, mat)
}

// GondolaH is a gondola shelf's height: it blocks sight.
const GondolaH = 2.2

// Gondola is a store shelving run, 1.2 m deep and length long (>= 1.5: shorter faces
// read as end caps), GondolaH high, stocked on both long faces (the shelf material),
// with a neon sign on top over its middle.
func Gondola(x, y, z, length float64, alongX bool) []c.Box {
	sign := min(length*0.4, 1.6)
	return []c.Box{
		rect(x, z, alongX, 0, 0, length, 1.2, y, y+GondolaH, c.Shelf),
		rect(x, z, alongX, (length-sign)/2, 0.56, (length+sign)/2, 0.64, y+GondolaH, y+GondolaH+0.45, c.Neon),
	}
}

// Stock shelf heights: the tops of the three open shelves. GPU spots go on StockMid.
const (
	StockLow  = 0.3
	StockMid  = 1.2
	StockHigh = 2.0
	StockH    = 2.6
)

// StockShelf is stockroom racking, 1.0 m deep and length long, StockH high: steel
// uprights at the ends (and every ~2 m), open shelves with their tops at StockLow,
// StockMid and StockHigh, cartons on the low and high shelves; the middle shelf is
// left clear for the GPUs (Map.GPUs at y + StockMid).
func StockShelf(x, y, z, length float64, alongX bool) []c.Box {
	const post, board, in = 0.08, 0.05, 0.02
	var out []c.Box
	bays := max(1, int(length/2+0.5))
	bay := length / float64(bays)
	for i := 0; i <= bays; i++ {
		u := min(float64(i)*bay, length-post)
		out = append(out,
			rect(x, z, alongX, u, 0, u+post, post, y, y+StockH, c.Metal),
			rect(x, z, alongX, u, 1-post, u+post, 1, y, y+StockH, c.Metal))
	}
	for _, top := range []float64{StockLow, StockMid, StockHigh} {
		out = append(out, rect(x, z, alongX, in, in, length-in, 1-in, y+top-board, y+top, c.Metal))
	}
	// cartons: a row per bay on the low and high shelves, a hand's width apart
	for i := range bays {
		u0, u1 := float64(i)*bay+post+0.07, float64(i+1)*bay-0.07
		out = append(out,
			rect(x, z, alongX, u0, 0.12, u1, 0.88, y+StockLow, y+StockLow+0.6, c.Carton),
			rect(x, z, alongX, u0+0.1, 0.15, u1-0.1, 0.85, y+StockHigh, y+StockHigh+0.45, c.Carton))
	}
	return out
}

// CheckoutH is the counter height.
const CheckoutH = 0.9

// CheckoutLane is a checkout in a 1.0 x 4.0 m footprint (4 m along the axis): a
// counter with the moving belt, the till (a screen on the counter), a bagging shelf
// at the far end and a lane sign on a pole. The free 0.4 m strip on the +side is the
// customers' side of the counter; space lanes 3.5 m apart.
func CheckoutLane(x, y, z float64, alongX bool) []c.Box {
	return []c.Box{
		rect(x, z, alongX, 0, 0, 3.0, 0.6, y, y+CheckoutH, c.Checkout),
		rect(x, z, alongX, 3.05, 0.02, 4.0, 0.58, y, y+0.85, c.Desk),                     // bagging shelf
		rect(x, z, alongX, 2.5, 0.1, 2.9, 0.15, y+CheckoutH, y+CheckoutH+0.32, c.Screen), // the till
		rect(x, z, alongX, 0.05, 0.05, 0.13, 0.13, y+CheckoutH, y+2.4, c.Metal),          // the sign pole
		rect(x, z, alongX, 0.0, 0.06, 0.58, 0.12, y+2.4, y+2.75, c.Neon),                 // lane sign
	}
}

// LanDesk is a LAN cafe desk row: seats x 1.0 m wide (along the axis), 1.6 m deep: a
// 0.75 m desk 0.8 deep with a monitor, PC tower and keyboard per seat, and a gaming
// chair in the 0.8 m in front of it (the -side across the axis).
func LanDesk(x, y, z float64, alongX bool, seats int) []c.Box {
	seats = max(1, seats)
	w := float64(seats)
	out := []c.Box{rect(x, z, alongX, 0, 0.8, w, 1.6, y, y+0.75, c.Desk)}
	for i := range seats {
		u := float64(i)
		top := y + 0.75
		out = append(out,
			rect(x, z, alongX, u+0.47, 1.37, u+0.53, 1.45, top, top+0.18, c.Metal),     // monitor stand
			rect(x, z, alongX, u+0.2, 1.38, u+0.8, 1.44, top+0.18, top+0.56, c.Screen), // monitor
			rect(x, z, alongX, u+0.82, 1.05, u+0.98, 1.5, top, top+0.45, c.PCTower),    // tower
			rect(x, z, alongX, u+0.28, 0.95, u+0.72, 1.12, top, top+0.025, c.Metal),    // keyboard
			rect(x, z, alongX, u+0.27, 0.15, u+0.73, 0.6, y+0.42, y+0.5, c.LanCarpet),  // chair seat
			rect(x, z, alongX, u+0.27, 0.07, u+0.73, 0.15, y+0.42, y+1.2, c.LanCarpet), // backrest
			rect(x, z, alongX, u+0.46, 0.32, u+0.54, 0.4, y, y+0.42, c.Metal),          // chair post
		)
	}
	return out
}

// TVWall is a wall of TVs hung flush on a wall: the strip x0..x1, z0..z1 is the space
// in front of the wall (its thin side, <= 0.3 m, is the depth; the TVs fill it from its
// low side, so make it touch the wall). Two rows of 1.2 x 0.7 m
// screens from 1.0 m above base, as many as fit, under a neon sign.
func TVWall(x0, z0, x1, z1, base float64) []c.Box {
	alongX := x1-x0 >= z1-z0
	length, depth := x1-x0, z1-z0
	if !alongX {
		length, depth = depth, length
	}
	d := min(depth, 0.3)
	n := max(1, int((length+0.1)/1.3))
	pad := (length - float64(n)*1.3 + 0.1) / 2
	var out []c.Box
	for row := range 2 {
		y0 := base + 1.0 + float64(row)*0.8
		for i := range n {
			u := pad + float64(i)*1.3
			out = append(out, rect(x0, z0, alongX, u, 0, u+1.2, d, y0, y0+0.7, c.Screen))
		}
	}
	sign := min(length*0.5, 3.0)
	out = append(out, rect(x0, z0, alongX, (length-sign)/2, 0, (length+sign)/2, d, base+2.7, base+3.3, c.Neon))
	return out
}

// PriceSign is a free-standing price-tag sign in a 0.7 x 0.08 m footprint: a pole 1.5 m
// high with a 0.7 x 0.45 m neon card on top, the card's face along the axis.
func PriceSign(x, y, z float64, alongX bool) []c.Box {
	return []c.Box{
		rect(x, z, alongX, 0.31, 0, 0.39, 0.08, y, y+1.5, c.Metal),
		rect(x, z, alongX, 0, 0.01, 0.7, 0.07, y+1.5, y+1.95, c.Neon),
	}
}

// Cart is a shopping cart in a 0.7 x 1.1 m footprint, 1.0 m high (cover): a wheeled
// frame, a basket and the push handle. yaw4 0..3 turns it in quarter turns (the handle
// at +z, +x, -z, -x); the footprint stays at (x, z) .. (x+0.7, z+1.1) or, turned
// sideways (yaw4 1 and 3), (x+1.1, z+0.7).
func Cart(x, y, z float64, yaw4 int) []c.Box {
	r := ((yaw4 % 4) + 4) % 4
	alongX := r == 1 || r == 3
	flip := r == 2 || r == 3 // handle at the low end of the long axis
	u := func(a, b float64) (float64, float64) {
		if flip {
			return 1.1 - b, 1.1 - a
		}
		return a, b
	}
	box := func(a, v0, b, v1, y0, y1 float64, mat string) c.Box {
		a, b = u(a, b)
		return rect(x, z, alongX, a, v0, b, v1, y+y0, y+y1, mat)
	}
	return []c.Box{
		box(0.05, 0.1, 0.95, 0.6, 0.1, 0.16, c.Metal),  // frame
		box(0.1, 0.05, 0.95, 0.65, 0.4, 0.92, c.Metal), // basket
		box(0.98, 0.02, 1.1, 0.68, 0.95, 1.0, c.Neon),  // handle (a neon grip: store branding)
		box(0.95, 0.06, 1.0, 0.12, 0.3, 0.97, c.Metal), // handle post
		box(0.95, 0.58, 1.0, 0.64, 0.3, 0.97, c.Metal), // handle post
		box(0.08, 0.12, 0.16, 0.2, 0.0, 0.1, c.Metal),  // wheels
		box(0.08, 0.5, 0.16, 0.58, 0.0, 0.1, c.Metal),
		box(0.84, 0.12, 0.92, 0.2, 0.0, 0.1, c.Metal),
		box(0.84, 0.5, 0.92, 0.58, 0.0, 0.1, c.Metal),
	}
}

// Kiosk is a demo kiosk (the gaming section) in a 1 x 1 m footprint, 1.3 m high: a
// pedestal and a screen on top running the demo.
func Kiosk(x, y, z float64) []c.Box {
	return []c.Box{
		c.B(x+0.1, y, z+0.1, x+0.9, y+0.8, z+0.9, c.Checkout),
		c.B(x+0.1, y+0.8, z+0.45, x+0.9, y+1.3, z+0.53, c.Screen),
	}
}
