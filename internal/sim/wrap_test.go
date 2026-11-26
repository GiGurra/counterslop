package sim

import (
	"math"
	"os/exec"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// fwd: the world direction the view of a body with gravity g and yaw looks
// along (level).
func fwd(g int, yaw float64) Vec3 {
	return FromFrame(g, Vec3{-math.Sin(yaw), 0, -math.Cos(yaw)})
}

func nearV(a, b Vec3) bool {
	return math.Abs(a[0]-b[0]) < 1e-9 && math.Abs(a[1]-b[1]) < 1e-9 && math.Abs(a[2]-b[2]) < 1e-9
}

// walkUntilWrap walks b forward for up to n ticks; the wrapping tick's
// events, or false.
func walkUntilWrap(w *World, b *Body, n int) (Events, int, bool) {
	for i := range n {
		ev := w.Step(b, Input{F: 1}, 0.01)
		if ev.Wrapped {
			return ev, i, true
		}
	}
	return Events{}, n, false
}

// On every face of a free-standing cube, in every gravity that stands on it,
// walking to each of its four edges wraps the body round it: onto the face
// it walked to (the feet on it, below the edge by what they were past it),
// gravity into that face, the speed carried down it, and the view turned
// with the body (it looks along where its walk went: what was down).
func TestEdgeWrapEveryEdge(t *testing.T) {
	m := content.Map{Boxes: []content.Box{content.B(-2, -2, -2, 2, 2, 2, content.Concrete)}}
	for g := 1; g < 6; g++ { // a gravity of its own (the world's never wraps: TestEdgeWrapNot)
		up := Up(g)
		for d := range 6 {
			t1 := GravDir(d) // the walk's direction (world)
			if t1 == up || t1 == GravDir(g) {
				continue
			}
			w := NewWorld(&m)
			b := NewBody(Vec3{float64(2 * up[0]), float64(2 * up[1]), float64(2 * up[2])})
			b.Grav, b.OnGround = g, true
			b.Yaw = yawAlong(ToFrame(g, t1))
			if !nearV(fwd(g, b.Yaw), t1) {
				t.Fatalf("g %d d %d: yaw %v looks along %v", g, d, b.Yaw, fwd(g, b.Yaw))
			}
			ev, ticks, ok := walkUntilWrap(w, b, 200)
			if !ok {
				t.Fatalf("gravity %d walking %v: no wrap (at %v, grav %d)", g, t1, b.Pos, b.Grav)
			}
			if b.Grav != GravOf(Vec3{-t1[0], -t1[1], -t1[2]}) || !b.OnGround {
				t.Fatalf("gravity %d walking %v: wrapped to %d (on ground %v)", g, t1, b.Grav, b.OnGround)
			}
			for a := range 3 { // the feet on the face walked to, below the edge
				if t1[a] != 0 && b.Pos[a] != 2*t1[a] || up[a] != 0 && !(b.Pos[a]*up[a] < 2-R+1e-9 && b.Pos[a]*up[a] > 2-R-0.2) {
					t.Fatalf("gravity %d walking %v: feet %v", g, t1, b.Pos)
				}
			}
			if !nearV(fwd(b.Grav, b.Yaw), GravDir(g)) || ev.Turn != math.Remainder(ev.Turn, 2*math.Pi) {
				t.Fatalf("gravity %d walking %v: the view looks along %v (turned %v), want %v", g, t1, fwd(b.Grav, b.Yaw), ev.Turn, GravDir(g))
			}
			v := Vec3(b.Vel)
			if dot := float64(v[0]*GravDir(g)[0]) + float64(v[1]*GravDir(g)[1]) + float64(v[2]*GravDir(g)[2]); dot < 3 || ticks > 60 {
				t.Fatalf("gravity %d walking %v: speed down the face %v after %d ticks", g, t1, v, ticks)
			}
			for range 20 { // and it walks on down the new face
				w.Step(b, Input{F: 1}, 0.01)
			}
			if !b.OnGround || b.Grav != GravOf(Vec3{-t1[0], -t1[1], -t1[2]}) {
				t.Fatalf("gravity %d walking %v: off the new face %v", g, t1, b.Pos)
			}
		}
	}
}

// de_hostile_takeover's case: on the outside of a hull wall (gravity into
// it), walking up over its top edge puts the body on top of the wall in the
// world's gravity, and walking on drops it onto the deck inside, where it
// stays: the world's gravity never wraps.
func TestEdgeWrapHullOntoDeck(t *testing.T) {
	m := content.Map{Boxes: []content.Box{
		content.B(0, -4, -10, 0.6, 6, 10, content.Metal),   // the hull wall, 0.6 thick, its top at 6
		content.B(0.6, 2, -10, 20, 2.5, 10, content.Metal), // the deck
	}}
	w := NewWorld(&m)
	b := NewBody(Vec3{0, 3, 0}) // on the wall's outer face
	b.Grav, b.OnGround = GravPX, true
	b.Yaw = yawAlong(ToFrame(GravPX, Vec3{0, 1, 0})) // up the wall
	if _, _, ok := walkUntilWrap(w, b, 300); !ok || b.Grav != GravDown || b.Pos[1] != 6 || !b.OnGround {
		t.Fatalf("over the top: grav %d at %v", b.Grav, b.Pos)
	}
	if !nearV(fwd(b.Grav, b.Yaw), Vec3{1, 0, 0}) {
		t.Fatalf("on top it looks along %v, not inboard", fwd(b.Grav, b.Yaw))
	}
	for range 150 {
		if ev := w.Step(b, Input{F: 1}, 0.01); ev.Wrapped {
			t.Fatalf("wrapped in the world's gravity at %v", b.Pos)
		}
	}
	if b.Grav != GravDown || b.Pos[1] != 2.5 || !b.OnGround || b.Pos[0] < 1 {
		t.Fatalf("not on the deck: grav %d at %v", b.Grav, b.Pos)
	}
}

// Where it doesn't wrap: a jump over the edge, a step down to a floor close
// below, no room on the face (a wall right beside it), and the world's own
// gravity (a ledge is a drop).
func TestEdgeWrapNot(t *testing.T) {
	cube := content.B(-2, -2, -2, 2, 2, 2, content.Concrete)
	start := func(boxes ...content.Box) (*World, *Body) {
		m := content.Map{Boxes: append([]content.Box{cube}, boxes...)}
		w := NewWorld(&m)
		b := NewBody(Vec3{0, 0, 2}) // on the +z face, gravity -z
		b.Grav, b.OnGround = GravNZ, true
		b.Yaw = yawAlong(ToFrame(GravNZ, Vec3{1, 0, 0})) // to the +x edge
		return w, b
	}
	w, b := start()
	for i := range 200 {
		in := Input{F: 1}
		if b.Pos[0] > 1.2 && i < 150 {
			in.Jump = true
		}
		if ev := w.Step(b, in, 0.01); ev.Wrapped {
			t.Fatalf("a jump over the edge wrapped at %v", b.Pos)
		}
		if b.Pos[0] > 1.2 {
			break
		}
	}
	w, b = start(content.B(2, -2, -2, 9, 2, 1.5, content.Concrete)) // a step down: a slab beyond the edge, its top 0.5 below the face
	for range 90 {
		if ev := w.Step(b, Input{F: 1}, 0.01); ev.Wrapped {
			t.Fatalf("stepped down and wrapped: %v", b.Pos)
		}
	}
	if b.Pos[2] != 1.5 || b.Grav != GravNZ || !b.OnGround {
		t.Fatalf("not on the slab below: %v grav %d", b.Pos, b.Grav)
	}
	w, b = start(content.B(2.5, -2, 2, 3, 2, 8, content.Concrete)) // a wall 0.5 from the +x face: no room for the hull there
	if _, _, ok := walkUntilWrap(w, b, 150); ok {
		t.Fatalf("wrapped into a wall: %v", b.Pos)
	}
	m := content.Map{Boxes: []content.Box{cube}}
	w = NewWorld(&m)
	b = NewBody(Vec3{0, 2, 0})
	b.OnGround, b.Yaw = true, -math.Pi/2
	if _, _, ok := walkUntilWrap(w, b, 150); ok || b.Grav != GravDown {
		t.Fatalf("the world's gravity wrapped: %v", b.Pos)
	}
}

// The generated JS wraps as the Go does, bit for bit (walks on every face
// of a cube over its edges, with the hook's boots on).
func TestEdgeWrapParityWithJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	m := content.Map{Boxes: []content.Box{content.B(-3, -3, -3, 3, 3, 3, content.Concrete), content.B(-40, -40, -40, 40, -39, 40, content.Concrete)}}
	var runs []run
	for g := 1; g < 6; g++ {
		up := Up(g)
		for d := range 6 {
			t1 := GravDir(d)
			if t1 == up || t1 == GravDir(g) {
				continue
			}
			rn := run{Spawn: Vec3{float64(3 * up[0]), float64(3 * up[1]), float64(3 * up[2])}, Yaw: yawAlong(ToFrame(g, t1)) + 0.3, Grav: g, Speed: 1}
			for i := range 160 {
				rn.Ticks = append(rn.Ticks, tick{F: 1, R: float64(i%40) / 80})
			}
			runs = append(runs, rn)
		}
	}
	parityRuns(t, node, m, runs, 0, 0, 0, MoveRules{})
	w := NewWorld(&m)
	wraps := 0
	for _, rn := range runs { // and they did wrap
		b := NewBody(rn.Spawn)
		b.Yaw, b.Grav = rn.Yaw, rn.Grav
		for _, tk := range rn.Ticks {
			if w.Step(b, Input{F: tk.F, R: tk.R, Speed: 1}, 0.01).Wrapped {
				wraps++
			}
		}
	}
	if wraps < len(runs) {
		t.Fatalf("%d wraps in %d runs", wraps, len(runs))
	}
}
