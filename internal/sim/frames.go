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
	cy := s.Pos[1] + float64(s.H*0.5)
	for i := len(zones) - 1; i >= 0; i-- {
		z := &zones[i]
		if z.Mover <= 0 || z.Mover > len(movers) {
			continue
		}
		o := MoverOffset(&movers[z.Mover-1], t)
		if s.Pos[0] >= z.Min[0]+o[0] && s.Pos[0] <= z.Max[0]+o[0] && cy >= z.Min[1]+o[1] && cy < z.Max[1]+o[1] && s.Pos[2] >= z.Min[2]+o[2] && s.Pos[2] <= z.Max[2]+o[2] {
			return z.Mover
		}
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
