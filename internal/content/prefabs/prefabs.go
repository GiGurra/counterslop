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
