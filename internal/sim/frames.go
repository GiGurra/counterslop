package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// Frames of reference (docs/physics.md "Frames of reference"). A body riding
// a mover (standing on a lift, a sled, in a ship's cabin) is in that mover's
// frame: its place is the mover's offset at a time plus a place in the
// frame (its local position). Movement doesn't use frames (bodies step in
// the world as ever); they are how a rider is replicated and placed: the
// server sends each rider's frame and local position, and a client puts the
// rider where the frame is when it draws that mover (its own command time),
// so riders standing still in a ship stay still relative to it and to each
// other whatever the two clocks (the remotes' view time, our command time)
// and the ship's speed. Lag compensation places them the same way.
//
// A frame is a mover (1-based index into the map's Movers; 0: the world).
// v1: movers only translate, so a frame's transform is MoverOffset.

// frameRest: feet this close above a mover's top are standing on it (a frame
// only: the movement's own restEps is tighter).
const frameRest = 0.05

// FrameOf is the frame body s is in at time t on the movers' clock (the time
// its position is at: the end of its last command), 0 for none:
//   - a gravity zone carried by a mover holding it (a ship's cabin, as
//     zoneOf finds it: its centre in the zone where the mover is at t);
//   - else, on the ground, its feet on the top of a slide or path mover's
//     box showing at t (a lift, a sled).
//
// Escalator treads are not frames (each tread is gone in a second; the flight
// stays put).
func FrameOf(movers []content.Mover, zones []content.GravZone, s *Body, t float64) int {
	if len(movers) == 0 {
		return 0
	}
	if f := carrierAt(movers, zones, s.Pos, s.H, t); f > 0 {
		return f
	}
	if !s.OnGround || s.Grav != GravDown {
		return 0
	}
	for i := range movers {
		m := &movers[i]
		if m.Kind == content.MoverEscalator || hidden(m, t) {
			continue
		}
		var b content.Box
		offsetBox(m, MoverOffset(m, t), &b)
		if math.Abs(b.Max[1]-s.Pos[1]) <= frameRest && under(s, &b) {
			return i + 1
		}
	}
	return 0
}

// carrierAt is the mover (1-based; 0: none) carrying the gravity zone that
// holds a body with its feet at p, h tall, at time t: its centre in the zone
// where the mover has it then (zoneOf's test). The last such zone wins.
func carrierAt(movers []content.Mover, zones []content.GravZone, p Vec3, h, t float64) int {
	cy := p[1] + float64(h*0.5)
	for i := len(zones) - 1; i >= 0; i-- {
		z := &zones[i]
		if z.Mover <= 0 || z.Mover > len(movers) {
			continue
		}
		o := MoverOffset(&movers[z.Mover-1], t)
		if p[0] >= z.Min[0]+o[0] && p[0] <= z.Max[0]+o[0] && cy >= z.Min[1]+o[1] && cy < z.Max[1]+o[1] && p[2] >= z.Min[2]+o[2] && p[2] <= z.Max[2]+o[2] {
			return z.Mover
		}
	}
	return 0
}

// carry moves body s with the ship whose cabin holds it over the sub-step
// t0 -> t1: a gravity zone carried by a mover is its passengers' frame
// (docs/physics.md "Frames of reference"). The body is moved by the mover's
// travel, in the air as on the floor, and so are the peers' hulls in the same
// cabin; the movers' solids of the sub-step (w.dyn) are then taken relative to
// it, so the cabin's own boxes don't move against it at all (riding, lifting
// and shoving see only what moves in the frame). Its velocity is the one in
// the frame. Before, only a body standing on the floor was carried: one in
// the air (a jump, or lifted off the floor as the climb eased out faster than
// 1 g) kept its world velocity, and the cabin's walls, floor and roof ran into
// it at 60-90 m/s and past it: thrown out of MVP-1 mid-flight
// (TestCabinIsItsPassengersFrame). A docked ship (no travel) and anything off
// a ship step as before. v1: the world's own gravity only (a body with a
// gravity of its own, Gravity Override, isn't carried).
func (w *World) carry(s *Body, t0, t1 float64) {
	if w.fg != GravDown || s.Grav != GravDown || len(w.Zones) == 0 {
		return
	}
	f := carrierAt(w.Movers, w.Zones, s.Pos, s.H, t0)
	if f == 0 {
		return
	}
	m := &w.Movers[f-1]
	o0, o1 := MoverOffset(m, t0), MoverOffset(m, t1)
	d := Vec3{o1[0] - o0[0], o1[1] - o0[1], o1[2] - o0[2]}
	if d[0] == 0 && d[1] == 0 && d[2] == 0 {
		return
	}
	for i := range w.peer {
		w.carryBox(&w.peer[i], f, t0, d)
	}
	for i := range w.ghost {
		w.carryBox(&w.ghost[i], f, t0, d)
	}
	for a := range 3 {
		s.Pos[a] += d[a]
	}
	for i := range w.dyn {
		x := &w.dyn[i]
		for a := range 3 {
			r := x.D[a] - d[a] // its travel in the frame
			if math.Abs(r) < 1e-9 {
				r = 0 // the cabin's own boxes: still in it (rounding aside)
			}
			x.D[a] = r
			x.B0.Min[a], x.B0.Max[a] = x.B1.Min[a]-r, x.B1.Max[a]-r
		}
	}
}

// carryBox moves a peer's hull by d when it is in mover f's cabin at t0.
func (w *World) carryBox(b *content.Box, f int, t0 float64, d Vec3) {
	feet := Vec3{float64((b.Min[0] + b.Max[0]) * 0.5), b.Min[1], float64((b.Min[2] + b.Max[2]) * 0.5)}
	if carrierAt(w.Movers, w.Zones, feet, b.Max[1]-b.Min[1], t0) != f {
		return
	}
	for a := range 3 {
		b.Min[a] += d[a]
		b.Max[a] += d[a]
	}
}

// FrameOffset is where frame fr (1-based mover, 0: the world) is at time t:
// the world position of its local origin. A frame the map doesn't have (a
// map edit removed the mover) is the world's.
func FrameOffset(movers []content.Mover, fr int, t float64) Vec3 {
	if fr <= 0 || fr > len(movers) {
		return Vec3{}
	}
	return MoverOffset(&movers[fr-1], t)
}

// ToFrameLocal is world position p in frame fr at time t (fr 0: p).
func ToFrameLocal(movers []content.Mover, fr int, t float64, p Vec3) Vec3 {
	o := FrameOffset(movers, fr, t)
	return Vec3{p[0] - o[0], p[1] - o[1], p[2] - o[2]}
}

// FromFrameLocal is local position l of frame fr placed at time t: its world
// position (fr 0: l).
func FromFrameLocal(movers []content.Mover, fr int, t float64, l Vec3) Vec3 {
	o := FrameOffset(movers, fr, t)
	return Vec3{l[0] + o[0], l[1] + o[1], l[2] + o[2]}
}
