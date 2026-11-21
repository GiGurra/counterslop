package prefabs

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// Multi-storey building pieces (gs_megabyte and later): escalators,
// stairwells, catwalks and roofs. Each piece's boxes keep clear of the floor
// it stands on (no coplanar faces with it: TestNoZFighting).

// Escalator tread size: shallow steps, well under the 0.55 m step-up.
const (
	EscRise = 0.25
	EscRun  = 0.45
	EscRail = 0.15 // balustrade thickness
	EscGrip = 0.06 // the handrail on top of it (material Handrail)
)

// Escalator is a flight of small steps rising `rise` from base, its top step
// meeting the upper floor's edge at `edge` (along z if alongZ, else x),
// descending towards dir (-1: -axis, +1: +axis). The treads span a0..a0+width
// across; a solid, stepped balustrade 1 m above the treads runs along each
// side (a0-EscRail..a0 and a0+width..a0+width+EscRail), so nobody steps off,
// topped by an EscGrip handrail box (c.Handrail: the glass shader can't tell a
// handrail from a face's width; it flickered when it tried).
// Length: rise/EscRise steps of EscRun. Treads use mat, balustrades side.
func Escalator(alongZ bool, edge, a0, width, base, rise float64, dir int, mat, side string) []c.Box {
	n := int(rise/EscRise + 0.5)
	out := Flight(alongZ, edge, a0, a0+width, base, EscRun, EscRise, n, dir, mat)
	for i := range n {
		near := edge + float64(dir)*EscRun*float64(n-i)
		far := edge + float64(dir)*EscRun*float64(n-i-1)
		lo, hi := min(near, far), max(near, far)
		top := base + float64(i+1)*EscRise + 1.0
		for _, s := range [][2]float64{{a0 - EscRail, a0}, {a0 + width, a0 + width + EscRail}} {
			if alongZ {
				out = append(out, c.B(s[0], base, lo, s[1], top-EscGrip, hi, side), c.B(s[0], top-EscGrip, lo, s[1], top, hi, c.Handrail))
			} else {
				out = append(out, c.B(lo, base, s[0], hi, top-EscGrip, s[1], side), c.B(lo, top-EscGrip, s[0], hi, top, s[1], c.Handrail))
			}
		}
	}
	return out
}

// EscalatorBelt is the moving surface of the Escalator built with the same
// arguments: its treads move at speed (m/s along the run; > 0 up, < 0 down)
// and carry whoever stands on them. The client draws the moving steps in mat
// in place of the static treads.
func EscalatorBelt(alongZ bool, edge, a0, width, base, rise float64, dir int, speed float64, mat string) c.Conveyor {
	n := int(rise/EscRise + 0.5)
	far := edge + float64(dir)*EscRun*float64(n) // the low end
	lo, hi := min(far, edge), max(far, edge)
	cv := c.Conveyor{Axis: 2, Up: -dir, Rise: EscRise, Run: EscRun, Kind: mat,
		Min: [3]float64{a0, base, lo}, Max: [3]float64{a0 + width, base + float64(n)*EscRise, hi}}
	if !alongZ {
		cv.Axis = 0
		cv.Min[0], cv.Min[2], cv.Max[0], cv.Max[2] = lo, a0, hi, a0+width
	}
	cv.Vel[cv.Axis] = float64(cv.Up) * speed
	return cv
}

// EscalatorLen is an escalator's run for a rise.
func EscalatorLen(rise float64) float64 { return float64(int(rise/EscRise+0.5)) * EscRun }

// Stairwell footprint: 10 × 7.4 m from its corner, walls 0.2 thick.
const (
	StairW = 10.0
	StairD = 7.4
)

// Stairwell is a switchback stair shaft with its corner at (x0, z0) serving
// the floors at levels (ascending, evenly spaced up to 5 m). Per storey: from
// the west landing a flight rises east (lane z0+0.2..z0+3.6) to the east
// mid-landing, then a flight rises west (lane z0+3.8..z0+7.2) to the next
// floor's west landing. A wall divides the lanes. The shaft's walls run from
// the lowest level to the top level + headroom; doors[i] puts a 2.2 m door in
// the south wall at level i's landing. The lowest landing is the floor below
// it (the map's), so it isn't drawn.
func Stairwell(x0, z0 float64, levels []float64, doors []bool, headroom float64, mat, wall string) []c.Box {
	const t, run, rise = 0.2, 0.8, 0.5
	var out []c.Box
	bot, top := levels[0], levels[len(levels)-1]+headroom
	// walls: south (doors per storey), north, west, east, the lane divider
	for i, l := range levels {
		hi := top
		if i+1 < len(levels) {
			hi = levels[i+1]
		}
		var gaps []Gap
		if i < len(doors) && doors[i] {
			gaps = append(gaps, Gap{From: x0 + t, To: x0 + 2.2, H: 2.2})
		}
		out = append(out, WallGaps(true, z0, x0, x0+StairW, t, l, hi-l, wall, gaps...)...)
	}
	out = append(out,
		c.B(x0, bot, z0+StairD-t, x0+StairW, top, z0+StairD, wall),
		c.B(x0, bot, z0+t, x0+t, top, z0+StairD-t, wall),
		c.B(x0+StairW-t, bot, z0+t, x0+StairW, top, z0+StairD-t, wall),
		c.B(x0+2.2, bot, z0+3.6, x0+6.2, top, z0+3.8, wall),
	)
	for i := 0; i+1 < len(levels); i++ {
		l, h := levels[i], (levels[i+1]-levels[i])/2
		n := int(h/rise + 0.5)
		r := h / float64(n)
		ru := 4.0 / float64(n)
		out = append(out, Flight(false, x0+6.2, z0+t, z0+3.6, l, ru, r, n, -1, mat)...)
		out = append(out, Slab(x0+6.2, z0+t, x0+StairW-t, z0+StairD-t, l+h, 0.4, mat))
		out = append(out, Flight(false, x0+2.2, z0+3.8, z0+StairD-t, l+h, ru, r, n, 1, mat)...)
		out = append(out, Slab(x0+t, z0+t, x0+2.2, z0+StairD-t, levels[i+1], 0.4, mat))
	}
	return out
}

// Catwalk is a deck from (x0, z0) to (x1, z1) with its top at y, 0.3 thick,
// with 1 m railings 0.1 thick along both long sides.
func Catwalk(x0, z0, x1, z1, y float64, mat, rail string) []c.Box {
	out := []c.Box{Slab(x0, z0, x1, z1, y, 0.3, mat)}
	if x1-x0 >= z1-z0 { // along x
		out = append(out, Railing(x0, z0, x1, z0+0.1, y, rail), Railing(x0, z1-0.1, x1, z1, y, rail))
	} else {
		out = append(out, Railing(x0, z0, x0+0.1, z1, y, rail), Railing(x1-0.1, z0, x1, z1, y, rail))
	}
	return out
}

// Roof is a flat roof slab over bounds with its underside at y, thick deep,
// open where the skylights are.
func Roof(bounds mapgen.Rect, skylights []mapgen.Rect, y, thick float64, mat string) []c.Box {
	var out []c.Box
	for _, r := range mapgen.Fill(bounds, skylights) {
		out = append(out, c.B(r.X0, y, r.Z0, r.X1, y+thick, r.Z1, mat))
	}
	return out
}
