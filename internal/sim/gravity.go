package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

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
	if len(w.frames) == 0 { // a World made without NewWorld
		w.frames = make([]*World, 6)
	}
	f := w.frames[g]
	if f == nil {
		f = &World{Boxes: make([]content.Box, len(w.Boxes)), Movers: nil, T: 0, Move: w.Move, Peers: nil, Water: nil, dyn: nil, cur: nil, peer: nil, ghost: nil, riding: false, onRamp: false, frames: nil, keepCur: true, sup: nil}
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

// stepRotated steps body s (gravity s.Grav) in its frame; walking over a
// box's edge there wraps it onto the next face (edgeWrap).
func (w *World) stepRotated(s *Body, in Input, dt float64) Events {
	g := s.Grav
	f := w.frame(g)
	ls := *s
	ls.Pos, ls.Vel = ToFrame(g, s.Pos), ToFrame(g, s.Vel)
	grounded := ls.OnGround && !ls.Hook.On
	if grounded {
		f.standingOn(&ls)
	}
	ev := f.step(&ls, in, dt)
	ng := -1
	if grounded && !ls.OnGround && !ev.Jumped && ls.Vel[1] <= 0 {
		ng = f.edgeWrap(&ls)
	}
	ls.Pos, ls.Vel = FromFrame(g, ls.Pos), FromFrame(g, ls.Vel)
	if ng >= 0 {
		ng = GravOf(FromFrame(g, GravDir(ng))) // the frame's gravity in the world
		ev.Turn = wrapTurn(g, ng)
		ls.Yaw += ev.Turn
		ls.Grav, ev.Wrapped = ng, true
	}
	*s = ls
	return ev
}

// Edge wrap (docs/physics.md): in a gravity of its own, a body that walks
// over the edge of the box it stands on turns a quarter turn round that edge
// onto the box's side face, its gravity into the face (the outside of a
// ship's hull, over the top edge, onto the deck). Not on a jump, not with
// the hook on, not with a floor within wrapDrop below (a step down), not
// where the turned hull has no room, and never in the world's own gravity:
// walking off a ledge there is a drop.
const wrapDrop = 1.0

// standingOn notes the static boxes s (frame coordinates) stands on.
func (w *World) standingOn(s *Body) {
	w.sup = w.sup[:0]
	for i := range w.Boxes {
		b := &w.Boxes[i]
		if math.Abs(b.Max[1]-s.Pos[1]) < 1e-6 && s.Pos[0]+R > b.Min[0]+eps && s.Pos[0]-R < b.Max[0]-eps && s.Pos[2]+R > b.Min[2]+eps && s.Pos[2]-R < b.Max[2]-eps {
			w.sup = append(w.sup, i)
		}
	}
}

// edgeWrap: s (frame coordinates, just walked off the boxes in w.sup) turned
// onto the side face of the one whose edge it went over, and its new gravity
// in the frame; -1: none (it falls).
func (w *World) edgeWrap(s *Body) int {
	if !w.free(s.Pos[0], s.Pos[1]-wrapDrop, s.Pos[2], s.H+wrapDrop) {
		return -1 // a floor close below: a step down, not an edge
	}
	for _, i := range w.sup {
		b := &w.Boxes[i]
		ax, side, past := -1, 0.0, 0.0
		x, z := s.Pos[0], s.Pos[2]
		inX, inZ := x > b.Min[0] && x < b.Max[0], z > b.Min[2] && z < b.Max[2]
		switch {
		case inZ && x-R >= b.Max[0]-eps:
			ax, side, past = 0, 1, x-b.Max[0]
		case inZ && x+R <= b.Min[0]+eps:
			ax, side, past = 0, -1, b.Min[0]-x
		case inX && z-R >= b.Max[2]-eps:
			ax, side, past = 2, 1, z-b.Max[2]
		case inX && z+R <= b.Min[2]+eps:
			ax, side, past = 2, -1, b.Min[2]-z
		}
		if ax < 0 {
			continue
		}
		// Round the edge: the feet on the face, as far down it as they were
		// past the edge; along the face's normal what was up.
		var p Vec3
		p[0], p[1], p[2] = s.Pos[0], b.Max[1]-past, s.Pos[2]
		if side > 0 {
			p[ax] = b.Max[ax]
		} else {
			p[ax] = b.Min[ax]
		}
		var d Vec3
		d[ax] = -side // the new gravity: into the face
		ng := GravOf(d)
		var hull content.Box
		HullBox(p, s.H, ng, &hull)
		if !w.roomFor(&hull) {
			continue
		}
		out := s.Vel[ax] * side // the speed over the edge: down the face now
		s.Pos = p
		s.Vel[ax] = 0
		s.Vel[1] = -out
		s.OnGround = true
		return ng
	}
	return -1
}

// roomFor: no solid (static, mover, peer) overlaps box h.
func (w *World) roomFor(h *content.Box) bool {
	for i := range w.solids() {
		b := w.box(i)
		if h.Max[0] > b.Min[0]+eps && h.Min[0] < b.Max[0]-eps && h.Max[1] > b.Min[1]+eps && h.Min[1] < b.Max[1]-eps && h.Max[2] > b.Min[2]+eps && h.Min[2] < b.Max[2]-eps {
			return false
		}
	}
	return true
}

// wrapTurn: what a wrap from world gravity g to ng over an edge along world
// axis e (the axis neither gravity is on) does to the yaw, so that the view
// turns with the body round the edge (a quarter-turn multiple). The edge's
// axis is the same line in both frames; the yaw that looks along it in each
// differs by the turn.
func wrapTurn(g, ng int) float64 {
	gd, nd := GravDir(g), GravDir(ng)
	var e Vec3
	for a := range 3 {
		if gd[a] == 0 && nd[a] == 0 {
			e[a] = 1
		}
	}
	t := yawAlong(ToFrame(ng, e)) - yawAlong(ToFrame(g, e))
	if t > math.Pi {
		t -= 2 * math.Pi
	} else if t <= -math.Pi {
		t += 2 * math.Pi
	}
	return t
}

// yawAlong: the yaw looking along horizontal unit axis v (frame
// coordinates): the view looks along (-sin yaw, -cos yaw) in x, z.
func yawAlong(v Vec3) float64 {
	switch {
	case v[0] < 0:
		return math.Pi / 2
	case v[0] > 0:
		return -math.Pi / 2
	case v[2] > 0:
		return math.Pi
	}
	return 0
}
