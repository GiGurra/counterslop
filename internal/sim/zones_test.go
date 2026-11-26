package sim

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// zoneWorld: a floor at y 0 (x -50..50), the Moon's zone over x > 10, space
// over y > 20, a launch pad at x -5.
func zoneWorld() *World {
	m := zoneMap()
	return NewWorld(&m)
}

func zoneMap() content.Map {
	return content.Map{
		Boxes: []content.Box{{Min: [3]float64{-50, -1, -50}, Max: [3]float64{50, 0, 50}, Kind: "concrete"}},
		GravZones: []content.GravZone{
			{Name: "moon", Min: [3]float64{10, -5, -50}, Max: [3]float64{50, 20, 50}, G: 1.0 / 6, Jump: 0.6},
			{Name: "space", Min: [3]float64{-50, 20, -50}, Max: [3]float64{50, 200, 50}, G: 0},
		},
		Launchers: []content.Launcher{{Name: "pad", Min: [3]float64{-6, -0.1, -1}, Max: [3]float64{-4, 0.2, 1}, Vel: [3]float64{0, 30, 0}}},
	}
}

// jumpArc: one jump from rest at x; the peak over the feet and the air time.
func jumpArc(w *World, x float64) (peak, air float64) {
	b := NewBody(Vec3{x, 0, 0})
	settle(w, b, Input{}, 20)
	w.Step(b, Input{Jump: true}, 0.01)
	for i := range 2000 {
		peak = math.Max(peak, b.Pos[1])
		if b.OnGround {
			return peak, float64(i+1) * 0.01
		}
		w.Step(b, Input{}, 0.01)
	}
	return peak, -1
}

// On the Moon a jump goes about twice as high and lasts over three times as
// long; outside the zones it is the world's jump, bit for bit.
func TestMoonJump(t *testing.T) {
	w := zoneWorld()
	ep, ea := jumpArc(w, 0)
	plain := flat()
	pp, pa := jumpArc(plain, 0)
	if ep != pp || ea != pa {
		t.Fatalf("outside the zones the jump changed: %v %v vs %v %v", ep, ea, pp, pa)
	}
	mp, ma := jumpArc(w, 30)
	if mp < 2.2 || mp > 2.8 || ma < 2.0 || ma > 2.6 {
		t.Errorf("moon jump peak %.2f m, air %.2f s (earth %.2f m %.2f s)", mp, ma, ep, ea)
	}
}

// In space a body coasts: no gravity, no ground, its velocity kept.
func TestSpaceCoasts(t *testing.T) {
	w := zoneWorld()
	b := NewBody(Vec3{0, 50, 0})
	b.Vel = Vec3{3, 1, -2}
	settle(w, b, Input{}, 100)
	if b.Vel != (Vec3{3, 1, -2}) || b.OnGround {
		t.Fatalf("vel %v ground %v", b.Vel, b.OnGround)
	}
	if math.Abs(b.Pos[0]-3) > 1e-9 || math.Abs(b.Pos[1]-51) > 1e-9 || math.Abs(b.Pos[2]+2) > 1e-9 {
		t.Errorf("pos %v", b.Pos)
	}
}

// Thrust pushes along the view up to spaceSpeed; it steers a faster body
// without braking it.
func TestSpaceThrust(t *testing.T) {
	w := zoneWorld()
	b := NewBody(Vec3{0, 50, 0})
	settle(w, b, Input{F: 1}, 300) // yaw 0: forward is -z
	if math.Abs(b.Vel[2]+spaceSpeed) > 1e-6 || math.Abs(b.Vel[0]) > 1e-9 {
		t.Errorf("full thrust vel %v", b.Vel)
	}
	up := NewBody(Vec3{0, 50, 0})
	settle(w, up, Input{JumpHeld: true}, 50)
	if up.Vel[1] <= 0 {
		t.Errorf("jump held: no thrust up %v", up.Vel)
	}
	fast := NewBody(Vec3{0, 50, 0})
	fast.Vel = Vec3{15, 0, 0}
	settle(w, fast, Input{F: 1}, 100)
	sp := math.Sqrt(fast.Vel[0]*fast.Vel[0] + fast.Vel[2]*fast.Vel[2])
	if math.Abs(sp-15) > 1e-6 || fast.Vel[2] >= 0 {
		t.Errorf("a fast body thrusting sideways: vel %v (speed %.3f)", fast.Vel, sp)
	}
}

// A launch pad throws a body standing on it, once, with an event.
func TestLaunchPad(t *testing.T) {
	w := zoneWorld()
	b := NewBody(Vec3{-5, 0, 0})
	ev := w.Step(b, Input{}, 0.01) // lands, then the pad throws it
	if !ev.Launched || b.OnGround || b.Vel[1] < 29 {
		t.Fatalf("not launched: %+v vel %v", ev, b.Vel)
	}
	ev = w.Step(b, Input{}, 0.01)
	if ev.Launched {
		t.Error("launched twice")
	}
	settle(w, b, Input{}, 200)
	if b.Pos[1] < 20 {
		t.Errorf("did not reach space: %v", b.Pos)
	}
}

// A body in a rotated gravity frame (the boots) ignores the zones (v1).
func TestZonesNotInRotatedFrame(t *testing.T) {
	w := zoneWorld()
	b := NewBody(Vec3{30, 5, 0})
	b.Grav = GravNX
	settle(w, b, Input{}, 10)
	if b.Vel[0] > -2 {
		t.Errorf("rotated body in the moon zone: vel %v (want the full fall along -x)", b.Vel)
	}
}

// lab_moon's launch pads: Earth's throws you through space onto the Moon's
// plateau, the Moon's back onto Earth's field, in both movement models.
func TestLabMoonFlights(t *testing.T) {
	m := maps.LabMoon()
	for _, mv := range []MoveRules{{}, {Model: MoveSource, AutoBhop: true}} {
		for _, l := range m.Launchers {
			w := NewWorld(&m)
			w.Move = mv
			b := NewBody(Vec3{(l.Min[0] + l.Max[0]) / 2, l.Min[1] + 0.1, (l.Min[2] + l.Max[2]) / 2})
			launched, air := false, 0.0
			for range 3000 {
				ev := w.Step(b, Input{}, 0.01)
				launched = launched || ev.Launched
				if launched {
					air += 0.01
				}
				if launched && b.OnGround && air > 1 {
					break
				}
			}
			moon := b.Pos[0] > 40 && math.Abs(b.Pos[1]-moonTop) < 0.01
			earth := b.Pos[0] < -20 && math.Abs(b.Pos[1]) < 0.01
			if !launched || !b.OnGround || (l.Name == "earth pad") != moon || (l.Name == "moon pad") != earth || air < 6 || air > 12 {
				t.Errorf("model %d, %s: launched %v, landed %v at %v after %.2f s", mv.Model, l.Name, launched, b.OnGround, b.Pos, air)
			} else {
				t.Logf("model %d, %s: landed at %.1f %.1f %.1f after %.2f s", mv.Model, l.Name, b.Pos[0], b.Pos[1], b.Pos[2], air)
			}
		}
	}
}

const moonTop = 20 // lab_moon's plateau
