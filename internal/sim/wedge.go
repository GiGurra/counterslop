package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// Wedges (content.Box.Ramp > 0): a right triangular prism filling the lower
// half of its box, the sloped face rising along one horizontal axis from the
// box's bottom edge on the low side to its top edge on the high side. The
// player hull stays an AABB; against a wedge it touches the slope with one
// bottom edge (the "corner" here: the hull's lowest edge on the uphill side).
//
//   - A slope whose unit normal has y >= MinGroundNY (about 45.6 degrees or
//     less) is ground: you stand on it, walk up it (lifted onto it like a
//     step) and down it (snapped to it), and don't slide.
//   - A steeper one is not ground: you are pushed out along its normal and
//     your velocity loses its part into the slope (Quake's ClipVelocity), so
//     you slide off it (surfing is that plus air control).
//
// The same geometry serves rays (RaySolid: bullets, sight, sound, grenades)
// and the renderer's mesh (web/engine/boxmesh.js). Everything here is
// generated to the client (sim.gen.js), so the gojs subset rules apply. In JS
// a plain box has no `ramp` field at all: test IsWedge, never Ramp == 0.

// MinGroundNY: the least upward part of a slope's unit normal that still
// counts as ground (Quake/Source use 0.7: 45.57 degrees).
const MinGroundNY = 0.7

// IsWedge reports whether b is a wedge (Box.Ramp set).
func IsWedge(b *content.Box) bool { return b.Ramp > 0 }

// rampAxis is the horizontal axis a wedge's slope rises along (0 x, 2 z) and
// the direction it rises in (+1, -1).
func rampAxis(b *content.Box) (int, float64) {
	switch b.Ramp {
	case content.RampPX:
		return 0, 1
	case content.RampNX:
		return 0, -1
	case content.RampPZ:
		return 2, 1
	}
	return 2, -1
}

// rampRun is how far v is from the low edge along the wedge's axis (not
// clamped: < 0 before it, > the run past the high edge).
func rampRun(b *content.Box, ax int, sg, v float64) float64 {
	if sg > 0 {
		return v - b.Min[ax]
	}
	return b.Max[ax] - v
}

// slopeY is the height of the wedge's sloped plane (not clamped to the box)
// at run u from the low edge.
func slopeY(b *content.Box, ax int, u float64) float64 {
	return b.Min[1] + float64((b.Max[1]-b.Min[1])*(u/(b.Max[ax]-b.Min[ax])))
}

// SlopeNormal is the unit outward normal of the wedge's sloped face.
func SlopeNormal(b *content.Box) Vec3 {
	ax, sg := rampAxis(b)
	h, l := b.Max[1]-b.Min[1], b.Max[ax]-b.Min[ax]
	n := math.Sqrt(float64(h*h) + float64(l*l))
	var out Vec3
	out[ax] = -sg * h / n
	out[1] = l / n
	return out
}

// Walkable: the wedge's slope is ground (not too steep to stand on).
func Walkable(b *content.Box) bool { return SlopeNormal(b)[1] >= MinGroundNY }

// slopeDepth is how far the bottom edge of a hull r wide with its feet at
// x,y,z sits under the wedge's slope, vertically (> 0: in it, if the hull
// overlaps the box at all).
func slopeDepth(b *content.Box, x, y, z, r float64) float64 {
	ax, sg := rampAxis(b)
	v := x
	if ax == 2 {
		v = z
	}
	return slopeY(b, ax, rampRun(b, ax, sg, v+float64(sg*r))) - y
}

// TopAt is the height of the solid's top at the point x,z (a wedge: its
// slope there; a box: its top). x,z is assumed inside the footprint.
func TopAt(b *content.Box, x, z float64) float64 {
	return wedgeTop(b, x, z, 0)
}

// wedgeTop is the highest point of the solid under a hull r wide at x,z.
func wedgeTop(b *content.Box, x, z, r float64) float64 {
	if !IsWedge(b) {
		return b.Max[1]
	}
	ax, sg := rampAxis(b)
	v := x
	if ax == 2 {
		v = z
	}
	u := math.Max(0, math.Min(b.Max[ax]-b.Min[ax], rampRun(b, ax, sg, v+float64(sg*r))))
	return math.Min(b.Max[1], slopeY(b, ax, u))
}

// pushOff moves the body out of the wedge's slope along its normal and takes
// the part of its velocity going into the slope away (it slides along it).
func pushOff(s *Body, b *content.Box) {
	n := SlopeNormal(b)
	d := float64(slopeDepth(b, s.Pos[0], s.Pos[1], s.Pos[2], R)*n[1]) + 1e-9
	vn := float64(float64(s.Vel[0]*n[0])+float64(s.Vel[1]*n[1])) + float64(s.Vel[2]*n[2])
	for a := 0; a < 3; a++ {
		s.Pos[a] += float64(n[a] * d)
		if vn < 0 {
			s.Vel[a] -= float64(n[a] * vn)
		}
	}
}

// hitWedge resolves a body that moved along ax (from old) into wedge b:
// on the ground and walkable, up onto the slope; through the slope (its
// corner was above it before the move), off the slope along its normal;
// through a vertical face, back out along ax.
func (w *World) hitWedge(s *Body, b *content.Box, ax int, d, old float64) {
	top := wedgeTop(b, s.Pos[0], s.Pos[2], R)
	rise := top - s.Pos[1]
	if s.OnGround && rise > 0 && rise <= stepUp && Walkable(b) && w.free(s.Pos[0], top, s.Pos[2], s.H) {
		s.Pos[1] = top // continuous: no step smoothing
		return
	}
	p := s.Pos
	p[ax] = old
	if slopeDepth(b, p[0], p[1], p[2], R) <= 0 {
		pushOff(s, b)
		return
	}
	if d > 0 {
		s.Pos[ax] = b.Min[ax] - R
	} else {
		s.Pos[ax] = b.Max[ax] + R
	}
	s.Vel[ax] = 0
}

// snapRamp keeps a body that walked off the ground (from p0) on a walkable
// slope within drop below (walking down a ramp, or off a ledge onto one), or
// on whatever is under it when it walked off a slope (the foot of a ramp).
// Anything else (off a box onto a box) is left to gravity as before.
func (w *World) snapRamp(s *Body, drop float64, p0 Vec3) {
	pos, vel := s.Pos, s.Vel
	w.moveV(s, -drop)
	if s.OnGround && (w.onRamp || w.onWedge(p0)) {
		s.Vel[1] = 0
		return
	}
	s.Pos, s.Vel, s.OnGround = pos, vel, false
}

// onWedge: a hull with its feet at p stands on a walkable wedge's slope.
func (w *World) onWedge(p Vec3) bool {
	for i := range w.solids() {
		b := w.box(i)
		if IsWedge(b) && Walkable(b) && p[0]+R > b.Min[0] && p[0]-R < b.Max[0] && p[2]+R > b.Min[2] && p[2]-R < b.Max[2] &&
			math.Abs(wedgeTop(b, p[0], p[2], R)-p[1]) < 1e-6 {
			return true
		}
	}
	return false
}

// RaySolid is the ray o + t·d's entry and exit distance through the solid
// (a box or a wedge) and the face it went in through: 0, 1, 2 an axis face,
// 3 the slope, -1 o is inside (t0 0). t0 < 0: a miss.
func RaySolid(o, d Vec3, b *content.Box) (float64, float64, int) {
	t0, t1, face := 0.0, math.Inf(1), -1
	for a := 0; a < 3; a++ {
		if math.Abs(d[a]) < 1e-12 {
			if o[a] < b.Min[a] || o[a] > b.Max[a] {
				return -1, -1, -1
			}
			continue
		}
		ta, tb := (b.Min[a]-o[a])/d[a], (b.Max[a]-o[a])/d[a]
		if ta > tb {
			ta, tb = tb, ta
		}
		if ta > t0 {
			t0, face = ta, a
		}
		t1 = math.Min(t1, tb)
		if t0 > t1 {
			return -1, -1, -1
		}
	}
	if !IsWedge(b) {
		return t0, t1, face
	}
	// The slope's plane: g(p) = (p.y - min.y)·run - u(p)·rise <= 0 inside.
	ax, sg := rampAxis(b)
	h, l := b.Max[1]-b.Min[1], b.Max[ax]-b.Min[ax]
	g := float64((o[1]-b.Min[1])*l) - float64(rampRun(b, ax, sg, o[ax])*h)
	gd := float64(d[1]*l) - float64(float64(sg*d[ax])*h)
	if math.Abs(gd) < 1e-12 {
		if g > 0 {
			return -1, -1, -1
		}
		return t0, t1, face
	}
	ts := -g / gd
	if gd < 0 {
		if ts > t0 {
			t0, face = ts, 3
		}
	} else {
		t1 = math.Min(t1, ts)
	}
	if t0 > t1 {
		return -1, -1, -1
	}
	return t0, t1, face
}

// FaceNormal is the outward normal of face (RaySolid's) of b hit by a ray
// along d.
func FaceNormal(b *content.Box, face int, d Vec3) Vec3 {
	var n Vec3
	if face == 3 {
		return SlopeNormal(b)
	}
	if face >= 0 {
		n[face] = 1
		if d[face] > 0 {
			n[face] = -1
		}
	}
	return n
}

// Ray is the nearest solid of boxes the ray o + t·d meets within maxT: the
// distance, the box's index and the face's outward normal (index -1: none).
// A ray starting inside a solid doesn't hit that one.
func Ray(boxes []content.Box, o, d Vec3, maxT float64) (float64, int, Vec3) {
	best, bi, bf := maxT, -1, -1
	for i := range boxes {
		t0, _, f := RaySolid(o, d, &boxes[i])
		if f >= 0 && t0 <= best && (bi < 0 || t0 < best) {
			best, bi, bf = t0, i, f
		}
	}
	if bi < 0 {
		return maxT, -1, Vec3{}
	}
	return best, bi, FaceNormal(&boxes[bi], bf, d)
}

// AboveSlope: that hull's bottom edge is on or above the wedge's slope (it
// came in through the slope if it is in the wedge now).
func AboveSlope(b *content.Box, x, y, z, r float64) bool { return slopeDepth(b, x, y, z, r) <= 0 }

// HullTop is the highest point of the solid under a player's hull standing
// at x,z (a wedge: its slope under the hull's uphill edge): where it stands.
func HullTop(b *content.Box, x, z float64) float64 { return wedgeTop(b, x, z, R) }
