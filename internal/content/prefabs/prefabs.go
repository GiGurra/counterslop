// Package prefabs are reusable props made of boxes. Each returns boxes in
// world space; maps place them. Visuals come from the box material on the client.
package prefabs

import c "github.com/gigurra/counterslop/internal/content"

// Crate is one wooden crate with its corner at (x, y, z) and size w×h×d.
func Crate(x, y, z, w, h, d float64) c.Box { return c.B(x, y, z, x+w, y+h, z+d, c.Crate) }

// CratePyramid is two s-sized crates side by side along x with a third centred on top.
func CratePyramid(x, z, s float64) []c.Box {
	return []c.Box{
		Crate(x, 0, z, s, s, s),
		Crate(x+s, 0, z, s, s, s),
		Crate(x+s/2, s, z, s, s, s),
	}
}

// Wall is a solid block from (x0, z0) to (x1, z1), h high.
func Wall(x0, z0, x1, z1, h float64, mat string) c.Box { return c.B(x0, 0, z0, x1, h, z1, mat) }

// Pillar is a w×w column h high.
func Pillar(x, z, w, h float64, mat string) c.Box { return c.B(x, 0, z, x+w, h, z+w, mat) }

// Building is a solid block centred on (cx, cz), w wide (x) and d deep (z),
// with a doorway doorW wide and doorH high running through it along z.
func Building(cx, cz, w, d, h, doorW, doorH float64, mat string) []c.Box {
	x0, x1, z0, z1, dx := cx-w/2, cx+w/2, cz-d/2, cz+d/2, doorW/2
	return []c.Box{
		c.B(x0, 0, z0, cx-dx, h, z1, mat),
		c.B(cx+dx, 0, z0, x1, h, z1, mat),
		c.B(cx-dx, doorH, z0, cx+dx, h, z1, mat), // lintel
	}
}

// StairsDownX is a flight of n steps leaving a ledge at x = edge towards -x,
// each run deep and rise high, spanning z0..z1. Step i covers y in [i·rise, (i+1)·rise].
func StairsDownX(edge, z0, z1, run, rise float64, n int, mat string) []c.Box {
	out := make([]c.Box, 0, n)
	for i := range n {
		out = append(out, c.B(edge-run*float64(n-i), float64(i)*rise, z0, edge, float64(i+1)*rise, z1, mat))
	}
	return out
}

// Slab is a horizontal floor/deck over x0..x1, z0..z1 with its top at y and
// thick metres deep.
func Slab(x0, z0, x1, z1, y, thick float64, mat string) c.Box {
	return c.B(x0, y-thick, z0, x1, y, z1, mat)
}

// Flight is a stair flight of n steps (run deep, rise high) whose top step
// meets a ledge at the given edge: along x (alongZ false) or z (alongZ true),
// descending towards -axis (dir -1) or +axis (dir +1). The steps span the
// cross axis from a0 to a1 and start at height base. Step i covers
// y in [base+i·rise, base+(i+1)·rise].
func Flight(alongZ bool, edge, a0, a1, base, run, rise float64, n, dir int, mat string) []c.Box {
	out := make([]c.Box, 0, n)
	for i := range n {
		near := edge + float64(dir)*run*float64(n-i) // the low end of step i
		lo, hi := min(near, edge), max(near, edge)
		y0, y1 := base+float64(i)*rise, base+float64(i+1)*rise
		if alongZ {
			out = append(out, c.B(a0, y0, lo, a1, y1, hi, mat))
		} else {
			out = append(out, c.B(lo, y0, a0, hi, y1, a1, mat))
		}
	}
	return out
}

// Gap is an opening [From, To] along a wall, H high (a doorway); H 0 = full height.
type Gap struct{ From, To, H float64 }

// WallGaps is a wall t thick at the fixed coordinate `at` (the wall spans
// at..at+t on that axis), running a0..a1 along x (alongX) or z, base..base+h
// high, with openings. Doorways with H > 0 get a lintel above them.
func WallGaps(alongX bool, at, a0, a1, t, base, h float64, mat string, gaps ...Gap) []c.Box {
	box := func(u0, u1, y0, y1 float64) c.Box {
		if alongX {
			return c.B(u0, y0, at, u1, y1, at+t, mat)
		}
		return c.B(at, y0, u0, at+t, y1, u1, mat)
	}
	var out []c.Box
	u := a0
	for _, g := range gaps {
		if g.From > u {
			out = append(out, box(u, g.From, base, base+h))
		}
		if g.H > 0 && g.H < h {
			out = append(out, box(g.From, g.To, base+g.H, base+h))
		}
		u = g.To
	}
	if u < a1 {
		out = append(out, box(u, a1, base, base+h))
	}
	return out
}

// Doors are the openings of a Room, per side: -z (south), +z (north), -x (west), +x (east).
type Doors struct{ S, N, W, E []Gap }

// Room is four walls t thick just inside x0..x1, z0..z1, h high from base,
// with doorways. The inside is open (no roof): the sun lights everything.
func Room(x0, z0, x1, z1, base, h, t float64, mat string, d Doors) []c.Box {
	var out []c.Box
	out = append(out, WallGaps(true, z0, x0, x1, t, base, h, mat, d.S...)...)
	out = append(out, WallGaps(true, z1-t, x0, x1, t, base, h, mat, d.N...)...)
	out = append(out, WallGaps(false, x0, z0+t, z1-t, t, base, h, mat, d.W...)...)
	out = append(out, WallGaps(false, x1-t, z0+t, z1-t, t, base, h, mat, d.E...)...)
	return out
}

// Desk is a w×d office desk (0.75 m, can be jumped onto) with its corner at
// (x, z) on the floor at y, and a monitor on top, facing ±z (monitorN puts the
// screen at the +z edge).
func Desk(x, y, z, w, d float64, monitorN bool) []c.Box {
	mz := z + 0.15
	if monitorN {
		mz = z + d - 0.2
	}
	mx := x + w/2
	return []c.Box{
		c.B(x, y, z, x+w, y+0.75, z+d, c.Desk),
		c.B(mx-0.3, y+0.75, mz, mx+0.3, y+1.15, mz+0.05, c.Metal),
	}
}

// DeskPod is four desks back to back around a 1.3 m privacy screen, centred
// on (cx, cz): two rows along x, the screen between them. 3.4 × 3.4 m.
func DeskPod(cx, y, cz float64) []c.Box {
	out := []c.Box{c.B(cx-1.7, y, cz-0.05, cx+1.7, y+1.3, cz+0.05, c.Plaster)}
	for _, dx := range []float64{-1.7, 0} {
		out = append(out, Desk(cx+dx, y, cz-1.65, 1.7, 1.6, true)...)
		out = append(out, Desk(cx+dx, y, cz+0.05, 1.7, 1.6, false)...)
	}
	return out
}

// Whiteboard is a free-standing board 2 m high, thin along z or x.
func Whiteboard(x0, z0, x1, z1 float64) c.Box { return c.B(x0, 0, z0, x1, 2.0, z1, c.Desk) }

// Railing is a 1 m parapet on a deck at height base.
func Railing(x0, z0, x1, z1, base float64, mat string) c.Box {
	return c.B(x0, base, z0, x1, base+1.0, z1, mat)
}

// ContainerH is a shipping container's height: two stack to 5.2 m.
const ContainerH = 2.6

// Container is a shipping container with its corner at (x, y, z), lying along
// x (alongX) or z, 20 ft (6 m) or 40 ft (long, 12 m). colour 0..3 (red, blue,
// green, yellow) is encoded in its footprint, which the container material
// reads: width 2.40 + 0.05·colour, length 6.0 + 0.1·colour (12.0 + 0.2·colour).
func Container(x, y, z float64, alongX, long bool, colour int) c.Box {
	k := float64(((colour % 4) + 4) % 4)
	w, l := 2.4+0.05*k, 6.0+0.1*k
	if long {
		l = 12.0 + 0.2*k
	}
	if alongX {
		return c.B(x, y, z, x+l, y+ContainerH, z+w, c.Container)
	}
	return c.B(x, y, z, x+w, y+ContainerH, z+l, c.Container)
}

// Stall is a market stall standing on y with its counter's corner at (x, z):
// a w×d crate counter 1 m high, two poles at the front corners and an awning
// at 2.5 m reaching `reach` metres out over the front (towards +z when
// frontN, else -z). Players walk under the awning; the counter is cover.
func Stall(x, y, z, w, d, reach float64, frontN bool) []c.Box {
	const pole = 0.12
	pz, a0, a1 := z-reach-pole, z-reach-pole, z+d // poles and awning out towards -z
	if frontN {
		pz, a0, a1 = z+d+reach, z, z+d+reach+pole
	}
	return []c.Box{
		c.B(x, y, z, x+w, y+1, z+d, c.Crate),
		c.B(x, y, pz, x+pole, y+2.5, pz+pole, c.Metal),
		c.B(x+w-pole, y, pz, x+w, y+2.5, pz+pole, c.Metal),
		c.B(x-0.2, y+2.5, a0, x+w+0.2, y+2.62, a1, c.Awning),
	}
}

// Car is a parked car standing on y with its corner at (x, z), lying along x
// (alongX) or z: four tyres, a painted body from 0.3 to 1.0 m and a glass
// cabin on top to 1.5 m. colour 0..3 (red, blue, silver, green) is encoded in
// the body's footprint, which the car material reads: width 1.80 + 0.02·colour,
// length 4.2 + 0.1·colour. Cover to crouch behind, not to stand on.
func Car(x, y, z float64, alongX bool, colour int) []c.Box {
	k := float64(((colour % 4) + 4) % 4)
	w, l := 1.8+0.02*k, 4.2+0.1*k
	// in car space: u along the length, v across
	box := func(u0, y0, v0, u1, y1, v1 float64, mat string) c.Box {
		if alongX {
			return c.B(x+u0, y+y0, z+v0, x+u1, y+y1, z+v1, mat)
		}
		return c.B(x+v0, y+y0, z+u0, x+v1, y+y1, z+u1, mat)
	}
	out := []c.Box{box(0, 0.3, 0, l, 1.0, w, c.Car), box(1.1, 1.0, 0.12, l-1.0, 1.5, w-0.12, c.Glass)}
	for _, u := range []float64{0.5, l - 1.2} {
		for _, v := range []float64{0.08, w - 0.33} {
			out = append(out, box(u, 0, v, u+0.7, 0.62, v+0.25, c.Handrail))
		}
	}
	return out
}
