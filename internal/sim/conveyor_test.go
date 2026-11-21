package sim

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// gs_megabyte's west escalator pair rises 5 m towards +z over z -5..4: the
// up lane's treads span x -21..-19.8, the down lane's -19.5..-18.3.
const (
	upX, downX   = -20.4, -18.9
	escZ0, escZ1 = -5.0, 4.0
	escTop       = 5.0
)

func megabyte(t *testing.T) *World {
	m := maps.GsMegabyte()
	w := NewWorld(&m)
	if len(w.Conveyors) != 4 {
		t.Fatalf("gs_megabyte has %d conveyors, want 4", len(w.Conveyors))
	}
	return w
}

// ride steps the body for sec seconds of 100 Hz ticks with the same input.
func ride(w *World, b *Body, in Input, sec float64) {
	for range int(sec*100 + 0.5) {
		w.Step(b, in, 0.01)
	}
}

func settle(w *World, b *Body) { ride(w, b, Input{}, 0.05) }

func TestConveyorCarriesUp(t *testing.T) {
	w := megabyte(t)
	b := NewBody(Vec3{upX, 0.5, -4.6}) // on the first treads
	settle(w, b)
	if !b.OnGround || math.Abs(b.Vel[2]-0.75) > 1e-9 {
		t.Fatalf("standing on the first tread: ground %v vel %v, want carried at 0.75 m/s +z", b.OnGround, b.Vel)
	}
	ride(w, b, Input{}, 6)
	// 6 s at 0.75 m/s: 4.5 m along, on the highest tread under the body's
	// footprint (z -0.57..0.23: the one at 2.75..3.0 m)
	if math.Abs(b.Pos[2]-(-4.6+0.0375+4.5)) > 0.05 || b.Pos[1] != 3 {
		t.Fatalf("after 6 s: pos %v, want z ~ -0.16, y 3", b.Pos)
	}
	ride(w, b, Input{}, 8)
	// Off the top onto the upper floor, carried a body radius past the comb
	// plate at most, and stopped there.
	if b.Pos[1] != escTop || b.Pos[2] < escZ1 || b.Pos[2] > escZ1+R+0.05 || math.Hypot(b.Vel[0], b.Vel[2]) > 1e-9 {
		t.Fatalf("at the top: pos %v vel %v, want standing still on the upper floor just past z %g", b.Pos, b.Vel, escZ1)
	}
	if b.Pos[0] != upX {
		t.Fatalf("drifted sideways to x %v", b.Pos[0])
	}
}

func TestConveyorCarriesDown(t *testing.T) {
	w := megabyte(t)
	b := NewBody(Vec3{downX, escTop, 3.8}) // on the top tread of the down lane
	settle(w, b)
	if math.Abs(b.Vel[2]+0.75) > 1e-9 {
		t.Fatalf("on the top tread: vel %v, want carried at 0.75 m/s -z", b.Vel)
	}
	// Riding down never leaves the treads (stepDown): no hop at each step.
	for i := range 1400 {
		w.Step(b, Input{}, 0.01)
		if !b.OnGround {
			t.Fatalf("tick %d: airborne at %v riding down", i, b.Pos)
		}
	}
	if b.Pos[1] != 0 || !b.OnGround || b.Pos[2] > escZ0 || b.Pos[2] < escZ0-R-0.3 || math.Hypot(b.Vel[0], b.Vel[2]) > 1e-9 {
		t.Fatalf("at the bottom: pos %v vel %v ground %v, want standing still on the floor just past z %g", b.Pos, b.Vel, b.OnGround, escZ0)
	}
}

// Walking with the belt is faster than on the floor, against it slower.
func TestConveyorWalking(t *testing.T) {
	w := megabyte(t)
	// Up the up lane (yaw pi faces +z) and down it (yaw 0 faces -z), from tread
	// to tread clear of the ends: 1 s of full run.
	for _, c := range []struct {
		name     string
		start    Vec3
		yaw, avg float64
	}{
		{"with it", Vec3{upX, 0.5, -4.0}, math.Pi, Run + 0.75},
		{"against it", Vec3{upX, escTop, 3.5}, 0, Run - 0.75},
	} {
		b := NewBody(c.start)
		b.Yaw = c.yaw
		settle(w, b)
		z0 := b.Pos[2]
		ride(w, b, Input{F: 1}, 0.2) // up to speed
		z1 := b.Pos[2]
		ride(w, b, Input{F: 1}, 0.6)
		if v := math.Abs(b.Pos[2]-z1) / 0.6; math.Abs(v-c.avg) > 0.25 {
			t.Errorf("%s: %.2f m/s along the escalator, want ~%.2f (from z %.2f)", c.name, v, c.avg, z0)
		}
	}
}

// A jump off the belt keeps its velocity; crouching still rides.
func TestConveyorJumpAndCrouch(t *testing.T) {
	w := megabyte(t)
	b := NewBody(Vec3{upX, 1.0, -3.0})
	settle(w, b)
	ride(w, b, Input{Jump: true}, 0.01)
	ride(w, b, Input{}, 0.2)
	if b.OnGround || math.Abs(b.Vel[2]-0.75) > 1e-9 {
		t.Fatalf("in the air off the belt: ground %v vel %v, want 0.75 m/s +z kept", b.OnGround, b.Vel)
	}
	ride(w, b, Input{Crouch: true}, 1.5)
	if !b.Crouched || !b.OnGround || math.Abs(b.Vel[2]-0.75) > 1e-9 {
		t.Fatalf("crouched on the belt: crouched %v ground %v vel %v, want carried", b.Crouched, b.OnGround, b.Vel)
	}
}

// Off the belt nothing changes: the floor beside and below an escalator, and
// the rail tops.
func TestConveyorOnlyOnTreads(t *testing.T) {
	w := megabyte(t)
	for _, p := range []Vec3{
		{upX, 0, escZ0 - 0.5},   // the floor at the foot of the up lane
		{downX, 0, escZ0 - 0.5}, // and of the down lane
		{-23, 0, 0},             // beside the escalators
		{-23, escTop, 6},        // the upper floor away from the comb plate
	} {
		b := NewBody(p)
		ride(w, b, Input{}, 0.5)
		if b.Vel[0] != 0 || b.Vel[2] != 0 || b.Pos[0] != p[0] || b.Pos[2] != p[2] {
			t.Errorf("standing at %v: moved to %v vel %v", p, b.Pos, b.Vel)
		}
	}
}

// A moving walkway along x over a ledge: carried along x, and off the end
// over a 2 m drop the body falls (stepDown snaps only up to a step's height).
func TestConveyorWalkwayOffALedge(t *testing.T) {
	w := &World{
		Boxes: []content.Box{
			content.B(0, 0, -1, 4, 2, 1, "metal"),      // the walkway's deck, ending at x 4
			content.B(-10, -1, -10, 10, 0, 10, "sand"), // the ground 2 m below
		},
		Conveyors: []content.Conveyor{{Min: [3]float64{0, 0, -1}, Max: [3]float64{4, 2, 1}, Vel: [3]float64{1, 0, 0}, Axis: 0, Up: 1}},
	}
	b := NewBody(Vec3{1, 2, 0})
	settle(w, b)
	if math.Abs(b.Vel[0]-1) > 1e-9 || b.Vel[2] != 0 {
		t.Fatalf("on the walkway: vel %v, want 1 m/s +x", b.Vel)
	}
	ride(w, b, Input{}, 3.4) // to x 4.4: off the deck
	if b.OnGround || b.Pos[1] >= 2 {
		t.Fatalf("past the end of the walkway: pos %v ground %v, want falling", b.Pos, b.OnGround)
	}
	ride(w, b, Input{}, 1)
	if b.Pos[1] != 0 || !b.OnGround {
		t.Fatalf("after the fall: pos %v ground %v, want on the ground", b.Pos, b.OnGround)
	}
}
