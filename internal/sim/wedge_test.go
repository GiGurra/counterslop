package sim

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// rampWorld: a floor and one wedge 4 m long rising towards +x from x 0 to 4,
// rise m high, 6 m wide (z -3..3).
func rampWorld(rise float64) *World {
	m := content.Map{Boxes: []content.Box{
		content.B(-20, -1, -20, 20, 0, 20, content.Concrete),
		content.W(0, 0, -3, 4, rise, 3, content.Concrete, content.RampPX),
	}}
	return NewWorld(&m)
}

func TestWedgeGeometry(t *testing.T) {
	b := content.W(0, 0, -3, 4, 2, 3, content.Concrete, content.RampPX)
	if got := TopAt(&b, 1, 0); math.Abs(got-0.5) > 1e-12 {
		t.Fatalf("TopAt x=1: %v, want 0.5", got)
	}
	if got := wedgeTop(&b, 1, 0, R); math.Abs(got-0.7) > 1e-12 {
		t.Fatalf("hull top at x=1: %v, want 0.7 (its uphill edge at x 1.4)", got)
	}
	n := SlopeNormal(&b)
	if math.Abs(n[0]+2/math.Sqrt(20)) > 1e-12 || math.Abs(n[1]-4/math.Sqrt(20)) > 1e-12 || n[2] != 0 {
		t.Fatalf("normal %v", n)
	}
	for _, c := range []struct {
		ramp int
		p    Vec3 // a point just above the slope's middle, then one just under it
	}{
		{content.RampPX, Vec3{1, 0.5, 0}}, {content.RampNX, Vec3{3, 0.5, 0}},
		{content.RampPZ, Vec3{2, 0.5, -1.5}}, {content.RampNZ, Vec3{2, 0.5, 1.5}},
	} {
		b := content.W(0, 0, -3, 4, 2, 3, content.Concrete, c.ramp)
		if !Walkable(&b) {
			t.Fatalf("ramp %d: walkable %v", c.ramp, Walkable(&b))
		}
		// Straight down onto the slope: hits it at the plane, through face 3.
		o := Vec3{c.p[0], 5, c.p[2]}
		t0, _, f := RaySolid(o, Vec3{0, -1, 0}, &b)
		if f != 3 || math.Abs(t0-(5-c.p[1])) > 1e-9 {
			t.Fatalf("ramp %d: down ray t0 %v face %d, want %v face 3", c.ramp, t0, f, 5-c.p[1])
		}
		// Level just above the slope's middle, across the whole box along the
		// other axis: misses (the box would be hit).
		d := Vec3{0, 0, 1}
		start := Vec3{c.p[0], c.p[1] + 0.01, -10}
		if c.ramp >= content.RampPZ {
			d, start = Vec3{1, 0, 0}, Vec3{-10, c.p[1] + 0.01, c.p[2]}
		}
		if t0, _, _ := RaySolid(start, d, &b); t0 >= 0 {
			t.Fatalf("ramp %d: a ray above the slope hit at %v", c.ramp, t0)
		}
		// Just under it: hits a side face.
		start[1] = c.p[1] - 0.01
		if t0, _, f := RaySolid(start, d, &b); t0 < 0 || f == 3 {
			t.Fatalf("ramp %d: a ray under the slope: t0 %v face %d", c.ramp, t0, f)
		}
	}
	// Up the slope from below the low edge: rises over it (misses), or into it.
	if t0, _, _ := RaySolid(Vec3{-1, 0.1, 0}, Vec3{1, 0, 0}, &b); t0 < 0 || math.Abs(t0-1.2) > 1e-9 {
		t.Fatalf("level ray into the slope: t0 %v, want 1.2", t0)
	}
	if t0, _, _ := RaySolid(Vec3{-1, 0.1, 0}, Vec3{0.8, 0.6, 0}, &b); t0 >= 0 {
		t.Fatalf("a ray steeper than the slope hit at %v", t0)
	}
}

// Running up a 26.6 degree ramp: on the ground all the way up, on top at the
// end, never stuck; then back down it: still on the ground every tick.
func TestWedgeWalkUpAndDown(t *testing.T) {
	w := rampWorld(2)
	b := NewBody(Vec3{-2, 0, 0})
	b.Yaw = -math.Pi / 2 // facing +x
	for i := range 90 {
		w.Step(b, Input{F: 1}, 0.01)
		if i > 5 && !b.OnGround {
			t.Fatalf("tick %d: in the air walking up at %v", i, b.Pos)
		}
		if b.Pos[0] > 0 && b.Pos[0] < 3.5 {
			want := TopAt(&w.Boxes[1], b.Pos[0]+R, 0)
			if math.Abs(b.Pos[1]-want) > 1e-6 {
				t.Fatalf("tick %d: at %v, the slope under the hull is at %v", i, b.Pos, want)
			}
		}
	}
	if b.Pos[0] < 2 || b.Pos[1] < 1 {
		t.Fatalf("didn't get up the ramp: %v", b.Pos)
	}
	b.Pos = Vec3{3.5, TopAt(&w.Boxes[1], 3.9, 0), 0}
	b.Vel = Vec3{}
	b.Yaw = math.Pi / 2 // facing -x: down the slope
	for i := range 80 {
		w.Step(b, Input{F: 1}, 0.01)
		if !b.OnGround {
			t.Fatalf("tick %d: left the ground walking down at %v vel %v", i, b.Pos, b.Vel)
		}
	}
	if b.Pos[0] > -1 || math.Abs(b.Pos[1]) > 1e-9 {
		t.Fatalf("didn't get down the ramp: %v", b.Pos)
	}
}

// Standing still on a walkable slope: no sliding.
func TestWedgeStandStill(t *testing.T) {
	w := rampWorld(2)
	b := NewBody(Vec3{2, 2, 0})
	for range 200 {
		w.Step(b, Input{}, 0.01)
	}
	if !b.OnGround || math.Abs(b.Pos[0]-2) > 1e-9 {
		t.Fatalf("slid on a walkable slope: %v ground %v", b.Pos, b.OnGround)
	}
}

// A 63 degree wedge is not ground: you slide down it, and running at it
// doesn't take you up.
func TestWedgeSteepSlides(t *testing.T) {
	w := rampWorld(8)
	b := NewBody(Vec3{2, 6, 0})
	for i := range 100 {
		w.Step(b, Input{}, 0.01)
		if b.OnGround && b.Pos[1] > 0.01 {
			t.Fatalf("tick %d: standing on a steep slope at %v", i, b.Pos)
		}
	}
	if b.Pos[0] > 1 || b.Vel[0] > -0.5 && b.Pos[1] > 0.01 {
		t.Fatalf("didn't slide down: %v vel %v", b.Pos, b.Vel)
	}
	b = NewBody(Vec3{-2, 0, 0})
	b.Yaw = -math.Pi / 2
	top := 0.0
	for range 300 {
		w.Step(b, Input{F: 1}, 0.01)
		top = math.Max(top, b.Pos[1])
		if w.overlaps(b.Pos[0], b.Pos[1], b.Pos[2], &w.Boxes[1], b.H) {
			t.Fatalf("inside the wedge at %v", b.Pos)
		}
	}
	if top > 1.5 {
		t.Fatalf("ran %.2f m up a steep slope", top)
	}
}

// The wedge's high end is a wall; its sides are walls where it is taller
// than a step.
func TestWedgeFaces(t *testing.T) {
	w := rampWorld(2)
	b := NewBody(Vec3{6, 0, 0})
	b.Yaw = math.Pi / 2 // facing -x, at the 2 m back face
	for range 100 {
		w.Step(b, Input{F: 1}, 0.01)
	}
	if math.Abs(b.Pos[0]-(4+R)) > 1e-9 || b.Pos[1] != 0 {
		t.Fatalf("back face: %v", b.Pos)
	}
	b = NewBody(Vec3{3, 0, 5})
	b.Yaw = 0 // facing -z, at the side where it is 1.5 m tall
	for range 100 {
		w.Step(b, Input{F: 1}, 0.01)
	}
	if math.Abs(b.Pos[2]-(3+R)) > 1e-9 || b.Pos[1] != 0 {
		t.Fatalf("side face: %v", b.Pos)
	}
	// Near the low end the side is under a step: you step onto the slope.
	b = NewBody(Vec3{0.3, 0, 5})
	for range 100 {
		w.Step(b, Input{F: 1}, 0.01)
	}
	if b.Pos[2] > 2 || b.Pos[1] <= 0 || !b.OnGround {
		t.Fatalf("low side: %v ground %v", b.Pos, b.OnGround)
	}
}

// Walking off a box edge onto a flat floor still falls (the ramp snap only
// takes slopes): the classic movement is unchanged.
func TestWedgeSnapOnlyRamps(t *testing.T) {
	m := content.Map{Boxes: []content.Box{
		content.B(-20, -1, -20, 20, 0, 20, content.Concrete),
		content.B(-5, 0, -3, 0, 0.05, 3, content.Concrete),
	}}
	w := NewWorld(&m)
	b := NewBody(Vec3{-1, 0.05, 0})
	b.Yaw = -math.Pi / 2
	air := 0
	for range 40 {
		w.Step(b, Input{F: 1}, 0.01)
		if !b.OnGround {
			air++
		}
	}
	if air == 0 {
		t.Fatal("snapped down a box ledge")
	}
}

func TestRayNearest(t *testing.T) {
	w := rampWorld(2)
	tt, i, n := Ray(w.Boxes, Vec3{1, 5, 0}, Vec3{0, -1, 0}, 100)
	if i != 1 || math.Abs(tt-4.5) > 1e-9 || n[1] < 0.8 {
		t.Fatalf("down onto the slope: t %v box %d n %v", tt, i, n)
	}
	tt, i, n = Ray(w.Boxes, Vec3{-1, 5, 0}, Vec3{0, -1, 0}, 100)
	if i != 0 || math.Abs(tt-5) > 1e-9 || n != (Vec3{0, 1, 0}) {
		t.Fatalf("down onto the floor: t %v box %d n %v", tt, i, n)
	}
	if _, i, _ = Ray(w.Boxes, Vec3{-1, 5, 0}, Vec3{0, 1, 0}, 100); i != -1 {
		t.Fatalf("up into the sky hit %d", i)
	}
}
