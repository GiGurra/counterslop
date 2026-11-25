package sim

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content/maps"
)

func harbor() *World {
	m := maps.LabHarbor()
	return NewWorld(&m)
}

func steps(w *World, b *Body, in Input, ticks int) Events {
	var all Events
	for range ticks {
		ev := w.Step(b, in, 0.01)
		all.Jumped = all.Jumped || ev.Jumped
		all.Splash = math.Max(all.Splash, ev.Splash)
	}
	return all
}

// The levels: dry on the quay, wading on the slipway's foot, swimming with the
// head out, under.
func TestWaterLevel(t *testing.T) {
	w := harbor()
	for _, c := range []struct {
		pos  Vec3
		want int
	}{{Vec3{0, 0, 20}, 0}, {Vec3{24, -1.0, 5}, 1}, {Vec3{-10, -1.8, 0}, 2}, {Vec3{-10, -3, 0}, 3}} {
		if got := w.WaterLevel(NewBody(c.pos)); got != c.want {
			t.Errorf("%v: level %d, want %d", c.pos, got, c.want)
		}
	}
}

// Falling in from the quay splashes and the water stops the plunge within
// 2 m; with nothing held you then sink slowly; holding jump you rise to the
// surface and stay there, the head out (never jumping out of open water).
func TestSwimSinkAndSurface(t *testing.T) {
	w := harbor()
	b := NewBody(Vec3{-10, 3, 0})
	ev, low := Events{}, b.Pos[1]
	for range 150 {
		e := w.Step(b, Input{}, 0.01)
		ev.Splash = math.Max(ev.Splash, e.Splash)
		low = math.Min(low, b.Pos[1])
	}
	if ev.Splash < 10 {
		t.Fatalf("no splash: %v", ev.Splash)
	}
	if low < -3.2 { // the feet went in at -0.6 at 13 m/s; the floor is at -4
		t.Fatalf("plunged to %v", low)
	}
	if v := b.Vel[1]; math.Abs(v+sinkSpeed) > 0.05 {
		t.Fatalf("sinking at %v, want %v", v, -sinkSpeed)
	}
	steps(w, b, Input{JumpHeld: true}, 300)
	if lvl := w.WaterLevel(b); lvl != 2 || b.Pos[1]+Eye < -0.6 {
		t.Fatalf("held jump: level %d at %v", lvl, b.Pos)
	}
	if b.Vel[1] > 1e-9 {
		t.Fatalf("rising out of open water: %v", b.Vel)
	}
}

// Swimming goes along the view: forward pitched down dives, pitched up
// (under water) rises; the speed approaches SwimSpeed.
func TestSwimAlongView(t *testing.T) {
	w := harbor()
	b := NewBody(Vec3{-10, -2.5, 0})
	b.Yaw = math.Pi // facing +z
	y0 := b.Pos[1]
	steps(w, b, Input{F: 1, Pitch: -0.5}, 50)
	if b.Pos[1] >= y0-0.3 || b.Pos[2] <= 1 {
		t.Fatalf("dive: %v", b.Pos)
	}
	if sp := math.Sqrt(b.Vel[0]*b.Vel[0] + b.Vel[1]*b.Vel[1] + b.Vel[2]*b.Vel[2]); math.Abs(sp-SwimSpeed) > 0.05 {
		t.Fatalf("speed %v, want %v", sp, SwimSpeed)
	}
	y1 := b.Pos[1]
	steps(w, b, Input{F: 1, Pitch: 0.5}, 50)
	if b.Pos[1] <= y1+0.3 {
		t.Fatalf("rise: %v from %v", b.Pos, y1)
	}
}

// The water jump: at the surface, jump held while swimming into the quay
// gets you up onto it; without jump you stay in the water; at the ship's hull
// (a wall, no ledge) jump does nothing.
func TestWaterJumpOntoQuay(t *testing.T) {
	for _, jump := range []bool{false, true} {
		w := harbor()
		b := NewBody(Vec3{-10, -1.7, 7})
		b.Yaw = math.Pi // facing +z, the quay's edge at z 10
		steps(w, b, Input{F: 1, JumpHeld: jump}, 200)
		on := b.OnGround && b.Pos[1] == 0 && b.Pos[2] > 10
		if on != jump {
			t.Fatalf("jump %v: at %v ground %v", jump, b.Pos, b.OnGround)
		}
	}
}

// Wading (the feet in, the waist out, on the slipway) is slower than
// running on dry ground.
func TestWade(t *testing.T) {
	w := harbor()
	b := NewBody(Vec3{21.5, -1.0, 8}) // the slipway's top at z 8
	b.Yaw = -math.Pi / 2              // facing +x, across it
	steps(w, b, Input{F: 1}, 40)
	sp := math.Hypot(b.Vel[0], b.Vel[2])
	if w.WaterLevel(b) != 1 || math.Abs(sp-Run*wadeScale) > 0.01 {
		t.Fatalf("level %d speed %v at %v", w.WaterLevel(b), sp, b.Pos)
	}
}

func TestNoWaterJumpAtAWall(t *testing.T) {
	w := harbor()
	b := NewBody(Vec3{-10, -2.2, -28})
	ev := steps(w, b, Input{F: 1, JumpHeld: true}, 200) // yaw 0: facing -z, the hull at z -30
	if ev.Jumped || b.Pos[1] > -1.5 || b.Pos[2] > -29.5 {
		t.Fatalf("at the hull: jumped %v, at %v", ev.Jumped, b.Pos)
	}
}
