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
	escSpeed     = 0.75
	escSlope     = 5.0 / 16 / 0.45 // rise per run on the climb: 20 treads, 2 flat at each end
)

func megabyte(t *testing.T) *World {
	m := maps.GsMegabyte()
	w := NewWorld(&m)
	if len(w.Movers) != 4 {
		t.Fatalf("gs_megabyte has %d movers, want 4", len(w.Movers))
	}
	if len(w.Boxes) >= len(m.Boxes) {
		t.Fatalf("the static treads are still in the movement world (%d of %d boxes)", len(w.Boxes), len(m.Boxes))
	}
	return w
}

// clock steps bodies on a command clock: tick k runs at k/100 s (CmdTime).
type clock struct {
	w    *World
	tick int
}

func (c *clock) step(b *Body, in Input) Events {
	c.tick++
	c.w.T = float64(c.tick) / 100
	return c.w.Step(b, in, 0.01)
}

func (c *clock) run(b *Body, in Input, sec float64, each func()) {
	for range int(sec*100 + 0.5) {
		c.step(b, in)
		if each != nil {
			each()
		}
	}
}

// standingOn is the mover box under b's feet at the end of the last step, or nil.
func (c *clock) standingOn(b *Body) *content.Box {
	for _, x := range MoverBoxes(c.w.Movers, float64(c.tick+1)/100) {
		if math.Abs(x.Max[1]-b.Pos[1]) <= 1e-6 && under(b, &x) {
			return &x
		}
	}
	return nil
}

// A rider standing still on an escalator is glued to its tread: the same
// height above it every tick, moving smoothly diagonally (the conveyor rode
// static treads and popped up 0.25 m a step), and off at the top onto the
// floor without a bump.
func TestEscalatorRideIsSmooth(t *testing.T) {
	for _, lane := range []struct {
		name         string
		start        Vec3
		dir          float64 // +1 up the flight (z+), -1 down
		endY, endMin float64
	}{
		{"up", Vec3{upX, 0, escZ0 - 0.6}, 1, escTop, escZ1},
		{"down", Vec3{downX, escTop, escZ1 + 0.6}, -1, 0, escZ0 - R},
	} {
		t.Run(lane.name, func(t *testing.T) {
			c := &clock{w: megabyte(t)}
			b := NewBody(lane.start)
			c.run(b, Input{}, 0.1, nil)
			// Walk onto the flight, then stand still.
			b.Yaw = 0
			if lane.dir > 0 {
				b.Yaw = math.Pi // facing +z
			}
			c.run(b, Input{F: 1, Walk: true}, 0.4, nil)
			c.run(b, Input{}, 0.3, nil) // stop walking
			if c.standingOn(b) == nil {
				t.Fatalf("boarding: not on a tread at %v", b.Pos)
			}
			maxDy := escSpeed * escSlope * 0.01 * 1.0001
			rode, prev := 0, b.Pos
			c.run(b, Input{}, 16, func() {
				dy, dz := b.Pos[1]-prev[1], b.Pos[2]-prev[2]
				if math.Abs(dy) > maxDy+1e-9 {
					t.Fatalf("tick %d: height jumped %.4f m (pos %v)", c.tick, dy, b.Pos)
				}
				if testing.Verbose() && c.tick > 70 && c.tick < 90 {
					t.Logf("tick %d pos %v vel %v on %v", c.tick, b.Pos, b.Vel, c.standingOn(b))
				}
				if math.Abs(dz) > escSpeed*0.01+1e-9 || dz*lane.dir < -1e-12 {
					t.Fatalf("tick %d: moved %.4f m along the flight", c.tick, dz)
				}
				if !b.OnGround {
					t.Fatalf("tick %d: off the ground at %v", c.tick, b.Pos)
				}
				if b.Pos[0] != lane.start[0] {
					t.Fatalf("tick %d: drifted sideways to x %v", c.tick, b.Pos[0])
				}
				if c.standingOn(b) != nil {
					rode++
				}
				prev = b.Pos
			})
			if rode < 1000 {
				t.Fatalf("on a tread for only %d ticks", rode)
			}
			if b.Pos[1] != lane.endY || (b.Pos[2]-lane.endMin)*lane.dir < 0 {
				t.Fatalf("after the ride: pos %v, want on the floor at y %v past z %v", b.Pos, lane.endY, lane.endMin)
			}
			if b.Vel != (Vec3{}) || c.standingOn(b) != nil {
				t.Fatalf("after the ride: still moving (vel %v) or still on a tread", b.Vel)
			}
		})
	}
}

// Walking on an escalator is walking on its treads: with it you get there
// sooner than riding, against it slower than on stairs, and the body's own
// velocity is relative to the treads (leaving keeps no belt momentum).
func TestEscalatorWalking(t *testing.T) {
	timeTo := func(start Vec3, yaw float64, goal func(*Body) bool, in Input) int {
		c := &clock{w: megabyte(t)}
		b := NewBody(start)
		b.Yaw = yaw
		c.run(b, Input{}, 0.1, nil)
		for i := 0; i < 3000; i++ {
			c.step(b, in)
			if goal(b) {
				return i
			}
		}
		t.Fatalf("never got there from %v (at %v)", start, b.Pos)
		return 0
	}
	top := func(b *Body) bool { return b.Pos[1] == escTop && b.Pos[2] > escZ1+R }
	foot := func(b *Body) bool { return b.Pos[1] == 0 && b.Pos[2] < escZ0-R }
	run := Input{F: 1}
	withUp := timeTo(Vec3{upX, 0, escZ0 - 0.6}, math.Pi, top, run)
	againstDown := timeTo(Vec3{downX, 0, escZ0 - 0.6}, math.Pi, top, run)
	ride := timeTo(Vec3{upX, 0, escZ0 - 0.6}, math.Pi, top, Input{F: 1, Walk: true})
	withDown := timeTo(Vec3{downX, escTop, escZ1 + 0.6}, 0, foot, run)
	againstUp := timeTo(Vec3{upX, escTop, escZ1 + 0.6}, 0, foot, run)
	if !(withUp < againstDown && withDown < againstUp && withUp < ride) {
		t.Fatalf("ticks: up the up lane %d, up the down lane %d, walking up %d; down the down lane %d, down the up lane %d",
			withUp, againstDown, ride, withDown, againstUp)
	}
	// 9 m of flight at 7 m/s + 0.75: about 1.2 s; against it at 7 - 0.75: 1.45 s.
	if withUp > 140 || againstDown > 175 {
		t.Fatalf("running up: %d ticks with the treads, %d against", withUp, againstDown)
	}
}

// Jumping on an escalator: in the air the treads move on underneath and the
// jump carries no belt velocity; the body lands on a tread and rides again.
func TestEscalatorJump(t *testing.T) {
	c := &clock{w: megabyte(t)}
	b := NewBody(Vec3{upX, 1.5, -2.5})
	c.run(b, Input{}, 0.3, nil)
	if c.standingOn(b) == nil {
		t.Fatalf("not riding at %v", b.Pos)
	}
	c.step(b, Input{Jump: true})
	if b.OnGround || b.Vel[2] != 0 {
		t.Fatalf("jumped: ground %v vel %v", b.OnGround, b.Vel)
	}
	c.run(b, Input{}, 1, nil)
	if !b.OnGround || c.standingOn(b) == nil {
		t.Fatalf("after the jump: ground %v at %v, want riding a tread", b.OnGround, b.Pos)
	}
	c.run(b, Input{Crouch: true}, 1, nil)
	if !b.Crouched || c.standingOn(b) == nil {
		t.Fatalf("crouched: %v at %v, want riding crouched", b.Crouched, b.Pos)
	}
}

// The generic test movers: a platform sliding along x, a lift, a pusher.
func slideWorld(mv content.Mover) (*World, *clock) {
	m := content.Map{Boxes: []content.Box{content.B(-20, -1, -20, 20, 0, 20, "")}, Movers: []content.Mover{mv}}
	w := NewWorld(&m)
	return w, &clock{w: w}
}

func TestMoverCarriesHorizontally(t *testing.T) {
	// A 2x2 platform 0.3 high, 4 m out along x and back every 8 s.
	_, c := slideWorld(content.Mover{Kind: content.MoverSlide, Min: [3]float64{-1, 0, -1}, Max: [3]float64{1, 0.3, 1}, Move: [3]float64{4, 0, 0}, Period: 8})
	b := NewBody(Vec3{0, 0.3, 0})
	prev := b.Pos[0]
	c.run(b, Input{}, 3.99, func() {
		if !b.OnGround || b.Pos[1] != 0.3 {
			t.Fatalf("tick %d: fell off the platform: %v", c.tick, b.Pos)
		}
		if d := b.Pos[0] - prev; d < 0 || d > 0.01+1e-9 {
			t.Fatalf("tick %d: moved %v along x, want the platform's 0.01", c.tick, d)
		}
		prev = b.Pos[0]
	})
	if math.Abs(b.Pos[0]-4) > 0.02 || b.Pos[2] != 0 {
		t.Fatalf("after half a cycle: at %v, want carried to x 4", b.Pos)
	}
	c.run(b, Input{}, 4.01, nil)
	if math.Abs(b.Pos[0]) > 0.02 {
		t.Fatalf("after a cycle: at %v, want back at x 0", b.Pos)
	}
	if b.Vel != (Vec3{}) {
		t.Fatalf("carried, the body's own velocity is %v, want 0", b.Vel)
	}
}

func TestMoverLifts(t *testing.T) {
	// A lift: a 2x2 slab from y -0.5 (flush below the floor) up 3 m and down every 6 s.
	_, c := slideWorld(content.Mover{Kind: content.MoverSlide, Min: [3]float64{-1, -0.5, -1}, Max: [3]float64{1, 0, 1}, Move: [3]float64{0, 3, 0}, Period: 6})
	b := NewBody(Vec3{0, 0, 0})
	top := 0.0
	c.run(b, Input{}, 2.99, func() {
		if !b.OnGround {
			t.Fatalf("tick %d: left the lift at %v", c.tick, b.Pos)
		}
		if b.Pos[1] < top-1e-9 {
			t.Fatalf("tick %d: sank to %v", c.tick, b.Pos[1])
		}
		top = b.Pos[1]
	})
	if math.Abs(b.Pos[1]-3) > 0.02 {
		t.Fatalf("at the top: y %v, want 3", b.Pos[1])
	}
	c.run(b, Input{}, 3.01, nil)
	if b.Pos[1] > 0.02 || !b.OnGround {
		t.Fatalf("back down: %v ground %v, want on the floor", b.Pos, b.OnGround)
	}
}

func TestMoverPushesSideways(t *testing.T) {
	// A 1 m wall block sliding 3 m along +x into a body standing at x 1.5.
	_, c := slideWorld(content.Mover{Kind: content.MoverSlide, Min: [3]float64{-1, 0, -1}, Max: [3]float64{0, 2, 1}, Move: [3]float64{3, 0, 0}, Period: 12})
	b := NewBody(Vec3{1.5, 0, 0})
	c.run(b, Input{}, 6, func() {
		box := MoverBoxes(c.w.Movers, float64(c.tick+1)/100)[0]
		if c.w.overlaps(b.Pos[0], b.Pos[1], b.Pos[2], &box, b.H) {
			t.Fatalf("tick %d: inside the pusher (body %v, box %v)", c.tick, b.Pos, box)
		}
	})
	if math.Abs(b.Pos[0]-(3+R)) > 0.02 || b.Pos[1] != 0 {
		t.Fatalf("pushed to %v, want x %v on the floor", b.Pos, 3+R)
	}
}

// A pusher doesn't shove a body through a wall (no tunnelling).
func TestMoverDoesNotPushThroughWalls(t *testing.T) {
	m := content.Map{Boxes: []content.Box{content.B(-20, -1, -20, 20, 0, 20, ""), content.B(2.5, 0, -5, 3, 3, 5, "")},
		Movers: []content.Mover{{Kind: content.MoverSlide, Min: [3]float64{-1, 0, -1}, Max: [3]float64{0, 2, 1}, Move: [3]float64{3, 0, 0}, Period: 12}}}
	c := &clock{w: NewWorld(&m)}
	b := NewBody(Vec3{1.5, 0, 0})
	c.run(b, Input{}, 6, func() {
		if b.Pos[0] > 2.5-R+1e-9 {
			t.Fatalf("tick %d: pushed into the wall: %v", c.tick, b.Pos)
		}
	})
}
