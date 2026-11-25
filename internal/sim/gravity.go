package sim

import "github.com/gigurra/counterslop/internal/content"

// Per-body gravity (the Gravity Override boots, docs/physics.md): a body falls
// along one of the six axis directions, Body.Grav:
//
//	GravDown 0 (-y, the world's), GravUp 1 (+y: the ceiling),
//	GravNX 2 (-x), GravPX 3 (+x), GravNZ 4 (-z), GravPZ 5 (+z)
//
// Each axis has a frame: a rotation by quarter turns that takes the world to
// coordinates where that gravity is -y. A rotated body steps in its frame:
// the map, the movers and the other players are turned into it, the body
// moves by the same code as everyone else (its hull stays an axis-aligned
// box, the dimensions swapped), and is turned back. The rotations are signed
// permutations of the axes, so the round trip is exact (no rounding: Go and
// the generated JS agree bit for bit), and GravDown is the world itself.
//
// In a rotated frame (v1) a wedge collides as its box, and a mover as its box
// at the command's time (no riding).
const (
	GravDown = 0
	GravUp   = 1
	GravNX   = 2
	GravPX   = 3
	GravNZ   = 4
	GravPZ   = 5
)

// GravDir is the unit vector gravity g pulls along.
func GravDir(g int) Vec3 {
	switch g {
	case GravUp:
		return Vec3{0, 1, 0}
	case GravNX:
		return Vec3{-1, 0, 0}
	case GravPX:
		return Vec3{1, 0, 0}
	case GravNZ:
		return Vec3{0, 0, -1}
	case GravPZ:
		return Vec3{0, 0, 1}
	}
	return Vec3{0, -1, 0}
}

// GravOf is the gravity index pulling along unit axis vector d (GravDown for
// anything else).
func GravOf(d Vec3) int {
	for g := range 6 {
		v := GravDir(g)
		if v[0] == d[0] && v[1] == d[1] && v[2] == d[2] {
			return g
		}
	}
	return GravDown
}

// FromFrame: v (in gravity g's frame) in world coordinates.
func FromFrame(g int, v Vec3) Vec3 {
	switch g {
	case GravUp:
		return Vec3{v[0], -v[1], -v[2]}
	case GravNX:
		return Vec3{v[1], -v[0], v[2]}
	case GravPX:
		return Vec3{-v[1], v[0], v[2]}
	case GravNZ:
		return Vec3{v[0], -v[2], v[1]}
	case GravPZ:
		return Vec3{v[0], v[2], -v[1]}
	}
	return v
}

// ToFrame: world vector v in gravity g's frame (FromFrame undone).
func ToFrame(g int, v Vec3) Vec3 {
	switch g {
	case GravUp:
		return Vec3{v[0], -v[1], -v[2]}
	case GravNX:
		return Vec3{-v[1], v[0], v[2]}
	case GravPX:
		return Vec3{v[1], -v[0], v[2]}
	case GravNZ:
		return Vec3{v[0], v[2], -v[1]}
	case GravPZ:
		return Vec3{v[0], -v[2], v[1]}
	}
	return v
}

// GravIn is gravity g as seen in frame f's coordinates (an index again).
func GravIn(f, g int) int { return GravOf(ToFrame(f, GravDir(g))) }

// boxThrough sets out to b's corners mapped by to (true: ToFrame, false:
// FromFrame), as an axis-aligned box again. A wedge becomes its box.
func boxThrough(g int, b *content.Box, out *content.Box, to bool) {
	var p, q Vec3
	if to {
		p, q = ToFrame(g, Vec3(b.Min)), ToFrame(g, Vec3(b.Max))
	} else {
		p, q = FromFrame(g, Vec3(b.Min)), FromFrame(g, Vec3(b.Max))
	}
	for a := range 3 {
		if p[a] <= q[a] {
			out.Min[a], out.Max[a] = p[a], q[a]
		} else {
			out.Min[a], out.Max[a] = q[a], p[a]
		}
	}
	out.Kind = b.Kind
	out.Ramp = 0
}

// BoxToFrame: world box b in gravity g's frame (a wedge: its box).
func BoxToFrame(g int, b *content.Box, out *content.Box) { boxThrough(g, b, out, true) }

// BoxFromFrame: box b of gravity g's frame in world coordinates.
func BoxFromFrame(g int, b *content.Box, out *content.Box) { boxThrough(g, b, out, false) }

// HullBox is the hull of a player with gravity g whose feet are at pos
// (world), h tall along its up: PeerBox turned with its frame.
func HullBox(pos Vec3, h float64, g int, out *content.Box) {
	if g == GravDown {
		PeerBox(pos, h, out)
		return
	}
	var b content.Box
	PeerBox(ToFrame(g, pos), h, &b)
	BoxFromFrame(g, &b, out)
}

// coreHull: CoreBox turned with gravity g (as HullBox).
func coreHull(pos Vec3, h float64, g int, out *content.Box) {
	if g == GravDown {
		CoreBox(pos, h, out)
		return
	}
	var b content.Box
	CoreBox(ToFrame(g, pos), h, &b)
	BoxFromFrame(g, &b, out)
}

// Up is the body's up direction (against its gravity), world coordinates.
func Up(g int) Vec3 { return FromFrame(g, Vec3{0, 1, 0}) }

// Above is the point h up from feet pos along gravity g's up (exact: the up
// is an axis, the products are 0 or ±h).
func Above(pos Vec3, g int, h float64) Vec3 {
	if g == GravDown {
		return Vec3{pos[0], pos[1] + h, pos[2]}
	}
	u := Up(g)
	return Vec3{pos[0] + float64(u[0]*h), pos[1] + float64(u[1]*h), pos[2] + float64(u[2]*h)}
}

// EyePos is the body's eye: EyeH + EyeOff up from its feet, along its up.
// (World gravity: summed as the eye always was, the same bits.)
func EyePos(s *Body) Vec3 {
	if s.Grav == GravDown {
		return Vec3{s.Pos[0], s.Pos[1] + s.EyeH + s.EyeOff, s.Pos[2]}
	}
	return Above(s.Pos, s.Grav, s.EyeH+s.EyeOff)
}

// frame is w turned into gravity g's frame: the static boxes (cached), the
// movers' boxes at w.T and the peers (each step).
func (w *World) frame(g int) *World {
	if len(w.frames) == 0 {
		w.frames = make([]*World, 6)
	}
	f := w.frames[g]
	if f == nil {
		f = &World{Boxes: make([]content.Box, len(w.Boxes)), Movers: nil, T: 0, Move: w.Move, Peers: nil, dyn: nil, cur: nil, peer: nil, ghost: nil, riding: false, onRamp: false, frames: nil, keepCur: true}
		for i := range w.Boxes {
			BoxToFrame(g, &w.Boxes[i], &f.Boxes[i])
		}
		w.frames[g] = f
	}
	f.T, f.Move = w.T, w.Move
	f.cur = f.cur[:0]
	if len(w.Movers) > 0 {
		for _, b := range MoverBoxes(w.Movers, w.T) {
			var lb content.Box
			BoxToFrame(g, &b, &lb)
			f.cur = append(f.cur, lb)
		}
	}
	f.Peers = f.Peers[:0]
	for i := range w.Peers {
		p := w.Peers[i]
		f.Peers = append(f.Peers, Peer{Pos: ToFrame(g, p.Pos), H: p.H, Lo: p.Lo, Core: p.Core, Drawn: p.Drawn, G: GravIn(g, p.G)})
	}
	return f
}

// stepRotated steps body s (gravity s.Grav) in its frame.
func (w *World) stepRotated(s *Body, in Input, dt float64) Events {
	g := s.Grav
	f := w.frame(g)
	ls := *s
	ls.Pos, ls.Vel = ToFrame(g, s.Pos), ToFrame(g, s.Vel)
	ev := f.step(&ls, in, dt)
	ls.Pos, ls.Vel = FromFrame(g, ls.Pos), FromFrame(g, ls.Vel)
	*s = ls
	return ev
}
