package sim

import (
	"fmt"
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// pushWorld: a 40x40 floor (top y 0) with extra static boxes and movers.
func pushWorld(boxes []content.Box, movers ...content.Mover) *clock {
	m := content.Map{Boxes: append([]content.Box{content.B(-20, -1, -20, 20, 0, 20, "")}, boxes...), Movers: movers}
	return &clock{w: NewWorld(&m)}
}

// slideAtSpeed is a slide mover with box lo..hi going travel along its
// motion at speed v (m/s; a slide's speed is 2|Move|/Period).
func slideAtSpeed(lo, hi, travel Vec3, v float64) content.Mover {
	l := math.Sqrt(travel[0]*travel[0] + travel[1]*travel[1] + travel[2]*travel[2])
	return content.Mover{Kind: content.MoverSlide, Min: lo, Max: hi, Move: travel, Period: 2 * l / v}
}

// moverBoxAt is mover 0's box at the end of tick c.tick.
func (c *clock) moverBoxAt() content.Box {
	return MoverBoxes(c.w.Movers, float64(c.tick+1)/100)[0]
}

func hullOverlap(b *Body, x *content.Box) bool {
	var hb content.Box
	hullAt(b.Pos, b.H, &hb)
	return boxesOverlap(&hb, x)
}

func staticOverlap(c *clock, b *Body) bool {
	for i := range c.w.Boxes {
		if hullOverlap(b, &c.w.Boxes[i]) {
			return true
		}
	}
	return false
}

// A wall moving at 10-200 m/s along x, z (both ways) and the diagonals
// passes a body standing in its way: the body is pushed ahead of it, never
// left inside it or behind it (before, from ~100 m/s a 0.2 m wall skipped
// past the body in one sub-step, and anything a shove couldn't place went
// through it).
func TestFastMoverPushesInEveryDirection(t *testing.T) {
	dirs := []Vec3{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {1, 0, 1}, {-1, 0, 1}, {1, 0, -1}, {-1, 0, -1}}
	for _, v := range []float64{10, 30, 60, 100, 150, 200} {
		for _, d := range dirs {
			name := fmt.Sprintf("%v m/s along %v", v, d)
			t.Run(name, func(t *testing.T) {
				diag := d[0] != 0 && d[2] != 0
				// A thin wall across the motion, 3 m from the body; a 1.2 m block on a diagonal.
				lo, hi := Vec3{-1.5, 0, -1.5}, Vec3{1.5, 3, 1.5}
				for a := 0; a < 3; a += 2 {
					if d[a] != 0 && !diag {
						lo[a], hi[a] = -0.1, 0.1
					}
					if diag {
						lo[a], hi[a] = -0.6, 0.6
					}
					lo[a] -= 3 * d[a]
					hi[a] -= 3 * d[a]
				}
				travel := Vec3{10 * d[0], 0, 10 * d[2]}
				c := pushWorld(nil, slideAtSpeed(lo, hi, travel, v))
				b := NewBody(Vec3{0, 0, 0})
				hit := false
				steps := int(c.w.Movers[0].Period*50) - 1 // to the far end
				for range steps {
					ev := c.step(b, Input{})
					x := c.moverBoxAt()
					if hullOverlap(b, &x) {
						t.Fatalf("tick %d: inside the mover (body %v, box %v)", c.tick, b.Pos, x)
					}
					if ev.Crushed > 0 {
						t.Fatalf("tick %d: crushed %v in open space", c.tick, ev.Crushed)
					}
					if staticOverlap(c, b) || b.Pos[1] < -1e-9 {
						t.Fatalf("tick %d: in the floor: %v", c.tick, b.Pos)
					}
					hit = hit || b.Pos != (Vec3{0, 0, 0})
				}
				if !diag {
					// Ahead of its leading face at the end of the travel.
					x := c.moverBoxAt()
					ax := 0
					if d[2] != 0 {
						ax = 2
					}
					want := x.Max[ax] + R
					if d[ax] < 0 {
						want = x.Min[ax] - R
					}
					if math.Abs(b.Pos[ax]-want) > 1e-6 {
						t.Fatalf("ended at %v, want %v on axis %d (the wall's face)", b.Pos, want, ax)
					}
				} else if !hit {
					t.Fatal("never pushed by the block that passed through its place")
				}
			})
		}
	}
}

// A slab rising at 10-200 m/s through a hole the body stands over lifts it
// (never through it); one coming down on a body standing on the floor
// crushes it, and the body stays on the floor.
func TestFastMoverPushesUpCrushesDown(t *testing.T) {
	for _, v := range []float64{10, 50, 100, 200} {
		t.Run(fmt.Sprintf("up %v", v), func(t *testing.T) {
			// The floor with a 0.6 m hole at the origin (the body's feet bridge it).
			m := content.Map{Boxes: []content.Box{
				content.B(-20, -1, -20, -0.3, 0, 20, ""), content.B(0.3, -1, -20, 20, 0, 20, ""),
				content.B(-0.3, -1, -20, 0.3, 0, -0.3, ""), content.B(-0.3, -1, 0.3, 0.3, 0, 20, ""),
			}, Movers: []content.Mover{slideAtSpeed(Vec3{-0.25, -5, -0.25}, Vec3{0.25, -4.8, 0.25}, Vec3{0, 10, 0}, v)}}
			c := &clock{w: NewWorld(&m)}
			b := NewBody(Vec3{0, 0, 0})
			b.OnGround = true
			top := 0.0
			for range int(c.w.Movers[0].Period * 50) {
				c.step(b, Input{})
				x := c.moverBoxAt()
				if hullOverlap(b, &x) {
					t.Fatalf("tick %d: inside the slab (body %v, slab %v)", c.tick, b.Pos, x)
				}
				top = math.Max(top, b.Pos[1])
			}
			if top < 4.8 {
				t.Fatalf("lifted to %v at most, want onto the slab's top (5.2 at the end of its travel)", top)
			}
		})
		t.Run(fmt.Sprintf("down %v", v), func(t *testing.T) {
			c := pushWorld(nil, slideAtSpeed(Vec3{-1, 6, -1}, Vec3{1, 6.3, 1}, Vec3{0, -6.5, 0}, v))
			b := NewBody(Vec3{0, 0, 0})
			crushed := 0.0
			for range int(c.w.Movers[0].Period*50) - 1 { // to the bottom of its travel
				ev := c.step(b, Input{})
				crushed += ev.Crushed
				if b.Pos[1] != 0 || b.Pos[0] != 0 || b.Pos[2] != 0 {
					t.Fatalf("tick %d: moved to %v (into the floor or out sideways)", c.tick, b.Pos)
				}
			}
			if crushed < 1.5 {
				t.Fatalf("crush depth %v, want about the 1.8 m the slab went into the body", crushed)
			}
		})
	}
}

// A body against a thin static wall, hit by a fast slide box from the other
// side: crushed, never pushed into or through the wall, not on the tick it is
// crushed either.
func TestFastMoverCrushesAgainstAThinWall(t *testing.T) {
	for _, v := range []float64{1, 10, 50, 100, 200} {
		t.Run(fmt.Sprint(v), func(t *testing.T) {
			wall := content.B(R, 0, -3, R+0.05, 3, 3, "") // 5 cm thick, the body's +x side touching it
			c := pushWorld([]content.Box{wall}, slideAtSpeed(Vec3{-3, 0, -1}, Vec3{-2.8, 2, 1}, Vec3{6, 0, 0}, v))
			b := NewBody(Vec3{0, 0, 0})
			crushed := 0.0
			for range int(c.w.Movers[0].Period * 50) {
				ev := c.step(b, Input{})
				crushed += ev.Crushed
				if b.Pos[0] > 1e-9 || staticOverlap(c, b) {
					t.Fatalf("tick %d: into the wall: %v", c.tick, b.Pos)
				}
			}
			if crushed < 0.5 {
				t.Fatalf("crush depth %v, want the box's way into the body", crushed)
			}
		})
	}
}

// Pushes chain through other players: a mover shoves the body next to it
// into another; both move ahead of it, neither ends up in it or deep in the
// other, whichever steps first. Against a wall, both are crushed and neither
// goes into it.
func TestMoverPushChains(t *testing.T) {
	for _, v := range []float64{2, 20, 100, 200} {
		for _, wall := range []bool{false, true} {
			for _, bFirst := range []bool{false, true} {
				t.Run(fmt.Sprintf("%v m/s wall %v b first %v", v, wall, bFirst), func(t *testing.T) {
					var boxes []content.Box
					if wall {
						boxes = append(boxes, content.B(1.2, 0, -3, 1.5, 3, 3, ""))
					}
					c := pushWorld(boxes, slideAtSpeed(Vec3{-8, 0, -1}, Vec3{-7.5, 2, 1}, Vec3{12, 0, 0}, v))
					a, b := NewBody(Vec3{-1, 0, 0}), NewBody(Vec3{-0.2, 0, 0}) // b just ahead of a (0.8: touching)
					var crushA, crushB float64
					for range int(c.w.Movers[0].Period * 50) {
						c.tick++
						c.w.T = float64(c.tick) / 100
						stepA := func() {
							c.w.Peers = []Peer{{Pos: b.Pos, H: b.H, Lo: false, Core: false}}
							crushA += c.w.Step(a, Input{}, 0.01).Crushed
						}
						stepB := func() {
							c.w.Peers = []Peer{{Pos: a.Pos, H: a.H, Lo: true, Core: false}}
							crushB += c.w.Step(b, Input{}, 0.01).Crushed
						}
						if bFirst {
							stepB()
							stepA()
						} else {
							stepA()
							stepB()
						}
						x := c.moverBoxAt()
						if !wall && (hullOverlap(a, &x) || hullOverlap(b, &x)) {
							t.Fatalf("tick %d: in the mover: a %v b %v box %v", c.tick, a.Pos, b.Pos, x)
						}
						if staticOverlap(c, a) || staticOverlap(c, b) {
							t.Fatalf("tick %d: in the wall: a %v b %v", c.tick, a.Pos, b.Pos)
						}
						if !wall && b.Pos[0]-a.Pos[0] < 2*R-0.05 {
							t.Fatalf("tick %d: a %v deep in b %v", c.tick, a.Pos, b.Pos)
						}
					}
					if wall {
						if crushA <= 0 || crushB <= 0 {
							t.Fatalf("against the wall: crushed a %v b %v, want both", crushA, crushB)
						}
						return
					}
					if a.Pos[0] < 4.5-1e-6 || b.Pos[0] < a.Pos[0]+2*R-0.05 {
						t.Fatalf("pushed to a %v b %v, want a at the mover's face (4.5) and b ahead of it", a.Pos, b.Pos)
					}
				})
			}
		}
	}
}

// A lift carrying a body into a low ceiling crushes it: the head never goes
// into the ceiling. Riders whose centre is off the lift's edge slide off it
// instead (no room up, room sideways).
func TestLiftIntoACeiling(t *testing.T) {
	for _, tc := range []struct {
		x     float64
		crush bool
	}{{0, true}, {0.5, true}, {0.9, true}, {1.2, false}} {
		t.Run(fmt.Sprint(tc.x), func(t *testing.T) {
			ceil := content.B(-5, 2.5, -5, 5, 3, 5, "")
			c := pushWorld([]content.Box{ceil}, content.Mover{Kind: content.MoverSlide, Min: Vec3{-1, -0.5, -1}, Max: Vec3{1, 0, 1}, Move: Vec3{0, 2, 0}, Period: 4})
			b := NewBody(Vec3{tc.x, 0, 0})
			crushed := 0.0
			for range 200 {
				crushed += c.step(b, Input{}).Crushed
				if b.Pos[1]+b.H > 2.5+1e-9 || staticOverlap(c, b) {
					t.Fatalf("tick %d: into the ceiling: %v", c.tick, b.Pos)
				}
			}
			if (crushed > 0) != tc.crush {
				t.Fatalf("crush depth %v (body at %v), want crushed %v", crushed, b.Pos, tc.crush)
			}
		})
	}
}

// A door closing on a body in the doorway (one leaf sliding shut against
// the frame): crushed, never pushed into the frame; with the doorway clear
// of the frame side, pushed through and out.
func TestDoorClosingOnABody(t *testing.T) {
	// Frame: walls at z < -1 and z > 1 (x 0..0.2); the leaf slides from z 1..3 to -1..1.
	frame := []content.Box{content.B(0, 0, -5, 0.2, 3, -1, ""), content.B(0, 0, 1, 0.2, 3, 5, "")}
	for _, v := range []float64{0.5, 2, 20} {
		t.Run(fmt.Sprint(v), func(t *testing.T) {
			door := slideAtSpeed(Vec3{0.02, 0, 1}, Vec3{0.18, 2.8, 3}, Vec3{0, 0, -2}, v)
			c := pushWorld(frame, door)
			b := NewBody(Vec3{0.1, 0, -0.6}) // in the doorway, against the far frame
			crushed := 0.0
			for range int(door.Period * 50) {
				crushed += c.step(b, Input{}).Crushed
				if staticOverlap(c, b) {
					t.Fatalf("tick %d: into the frame: %v", c.tick, b.Pos)
				}
			}
			if crushed <= 0 {
				t.Fatalf("not crushed (at %v)", b.Pos)
			}
		})
	}
}

// sweepHit: a box moving along any 3D direction hits a body in its path at
// the face its motion leads with, and pushes it to that face; a box whose
// motion takes it past the body's side pushes it only as far as it swept it
// (frictionless); a box that misses doesn't hit.
func TestSweepHit(t *testing.T) {
	b := content.Box{Min: Vec3{-0.5, 0, -0.5}, Max: Vec3{0.5, 1, 0.5}}
	for _, tc := range []struct {
		name string
		p    Vec3
		dr   Vec3
		hit  bool
		ax   int
		to   float64
	}{
		{"head on +x, skipped past in one step", Vec3{-1, 0, 0}, Vec3{3, 0, 0}, true, 0, 0.5 + R},
		{"from below", Vec3{0, -0.5, 0}, Vec3{0, 3, 0}, true, 1, 1},
		{"from above", Vec3{0, -1, 0}, Vec3{0, -3, 0}, true, 1, -H},
		{"diagonal up +x+z: the x face", Vec3{0.8, 0, 0}, Vec3{2, 0.5, 1}, true, 0, 0.5 + R},
		{"diagonal: slides off the side", Vec3{0.85, 0.2, -1.0}, Vec3{1, 0, 3}, true, 0, 0.5 + R - 1.0/30},
		{"misses", Vec3{0, 0, 2}, Vec3{3, 0, 0}, false, 0, 0},
		{"moving away", Vec3{-1, 0, 0}, Vec3{-1, 0, 0}, false, 0, 0},
		{"touching only", Vec3{-0.5 - R, 0, 0}, Vec3{-0.001, 0, 0}, false, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hit, ax, to := sweepHit(tc.p, H, &b, tc.dr, 0)
			if hit != tc.hit || hit && (ax != tc.ax || math.Abs(to-tc.to) > 1e-3) {
				t.Fatalf("hit %v ax %d to %v, want %v %d %v", hit, ax, to, tc.hit, tc.ax, tc.to)
			}
		})
	}
}

// Pushed into a slope: up a walkable ramp (the movement puts the body on
// it), crushed against one too steep to stand on (a wall that leans).
func TestPushIntoASlope(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rise  float64 // over 4 m along +x
		crush bool
	}{{"ramp", 1, false}, {"steep", 12, true}} {
		t.Run(tc.name, func(t *testing.T) {
			ramp := content.W(1, 0, -3, 5, tc.rise, 3, "", content.RampPX)
			c := pushWorld([]content.Box{ramp}, slideAtSpeed(Vec3{-3, 0, -1}, Vec3{-2, 2, 1}, Vec3{5, 0, 0}, 1))
			b := NewBody(Vec3{0, 0, 0})
			crushed, top := 0.0, 0.0
			for range 450 {
				crushed += c.step(b, Input{}).Crushed
				top = math.Max(top, b.Pos[1])
				if c.w.overlaps(b.Pos[0], b.Pos[1], b.Pos[2], &ramp, b.H) {
					t.Fatalf("tick %d: in the slope: %v", c.tick, b.Pos)
				}
			}
			if (crushed > 0) != tc.crush || !tc.crush && top < 0.1 {
				t.Fatalf("crush depth %v, highest %v (at %v): want crushed %v", crushed, top, b.Pos, tc.crush)
			}
		})
	}
}

// Two doors sliding shut on each other with a body between: it can't be
// pushed out of one into the other: crushed, and it stays where it was.
func TestDoorsClosingOnEachOther(t *testing.T) {
	c := pushWorld(nil,
		slideAtSpeed(Vec3{-3, 0, -1}, Vec3{-2, 2.5, 1}, Vec3{2.1, 0, 0}, 2),
		slideAtSpeed(Vec3{2, 0, -1}, Vec3{3, 2.5, 1}, Vec3{-2.1, 0, 0}, 2))
	b := NewBody(Vec3{0, 0, 0})
	crushed := 0.0
	for range 100 {
		crushed += c.step(b, Input{}).Crushed
		if math.Abs(b.Pos[0]) > 0.2 || b.Pos[1] != 0 || b.Pos[2] != 0 {
			t.Fatalf("tick %d: squeezed out to %v", c.tick, b.Pos)
		}
	}
	if crushed <= 0 {
		t.Fatalf("not crushed between the doors (at %v)", b.Pos)
	}
}

// Chains along -x, three deep. A drawn hull (lag compensated, peers.go) neither stops a push
// nor is pushed: it is only where a client saw the peer.
func TestMoverPushChainsMore(t *testing.T) {
	t.Run("three along -x", func(t *testing.T) {
		c := pushWorld(nil, slideAtSpeed(Vec3{5.5, 0, -1}, Vec3{6, 2, 1}, Vec3{-12, 0, 0}, 30))
		bs := []*Body{NewBody(Vec3{5, 0, 0}), NewBody(Vec3{4.2, 0, 0}), NewBody(Vec3{3.4, 0, 0})}
		for range 40 {
			c.tick++
			c.w.T = float64(c.tick) / 100
			for i, b := range bs {
				c.w.Peers = c.w.Peers[:0]
				for j, o := range bs {
					if j != i {
						c.w.Peers = append(c.w.Peers, Peer{Pos: o.Pos, H: o.H, Lo: j < i, Core: false})
					}
				}
				c.w.Step(b, Input{}, 0.01)
			}
			x := c.moverBoxAt()
			for i, b := range bs {
				if hullOverlap(b, &x) {
					t.Fatalf("tick %d: body %d in the mover: %v", c.tick, i, b.Pos)
				}
				if i > 0 && bs[i-1].Pos[0]-b.Pos[0] < 2*R-0.05 {
					t.Fatalf("tick %d: body %d %v deep in body %d %v", c.tick, i, b.Pos, i-1, bs[i-1].Pos)
				}
			}
		}
		if bs[0].Pos[0] > -5.6 {
			t.Fatalf("pushed to %v %v %v, want the first at the mover's far face", bs[0].Pos, bs[1].Pos, bs[2].Pos)
		}
	})
	t.Run("a drawn hull doesn't count", func(t *testing.T) {
		wall := content.B(2, 0, -3, 2.5, 3, 3, "")
		c := pushWorld([]content.Box{wall}, slideAtSpeed(Vec3{-3, 0, -1}, Vec3{-2, 2, 1}, Vec3{2.5, 0, 0}, 2))
		b := NewBody(Vec3{0, 0, 0})
		crushed := 0.0
		for range 120 {
			c.tick++
			c.w.T = float64(c.tick) / 100
			c.w.Peers = []Peer{{Pos: Vec3{0.8, 0, 0}, H: H, Lo: false, Core: false, Drawn: true}}
			crushed += c.w.Step(b, Input{}, 0.01).Crushed
		}
		if crushed > 0 || b.Pos[0] < 0.4 {
			t.Fatalf("at %v crushed %v: want pushed on (the drawn hull is no body)", b.Pos, crushed)
		}
	})
}

// behind: a pushed peer pushes only a body ahead of it along its push, any
// axis and either way; level (within the mm rounding) the lower ID pushes.
func TestBehind(t *testing.T) {
	var hull content.Box
	hullAt(Vec3{0, 1, 0}, H, &hull) // a peer whose feet are at y 1
	for _, tc := range []struct {
		name string
		ax   int
		d    float64
		lo   bool
		p    Vec3
		want bool
	}{
		{"+x ahead", 0, 1, false, Vec3{0.5, 1, 0}, true},
		{"+x behind", 0, 1, true, Vec3{-0.5, 1, 0}, false},
		{"-x ahead", 0, -1, false, Vec3{-0.5, 1, 0}, true},
		{"-x behind", 0, -1, true, Vec3{0.5, 1, 0}, false},
		{"level, peer lower ID", 2, 1, true, Vec3{0, 1, 0.001}, true},
		{"level, peer higher ID", 2, 1, false, Vec3{0, 1, 0.001}, false},
		{"up: standing above", 1, 1, false, Vec3{0, 2.8, 0}, true},
		{"up: under it", 1, 1, true, Vec3{0, -0.8, 0}, false},
		{"down: under it", 1, -1, false, Vec3{0, -0.8, 0}, true},
	} {
		pj := pusher{B1: hull, dyn: -1, ax: tc.ax, lo: tc.lo}
		pj.D[tc.ax] = tc.d
		if got := behind(tc.p, H, &pj); got != tc.want {
			t.Errorf("%s: behind %v, want %v", tc.name, got, tc.want)
		}
	}
}
