package sim

import (
	"math"
	"os/exec"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// The six frames are rotations (right-handed, quarter turns), each gravity
// is its frame's -y, and the round trip is exact.
func TestGravityFrames(t *testing.T) {
	cross := func(a, b Vec3) Vec3 {
		return Vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
	}
	seen := map[Vec3]bool{}
	for g := range 6 {
		x, y, z := FromFrame(g, Vec3{1, 0, 0}), FromFrame(g, Vec3{0, 1, 0}), FromFrame(g, Vec3{0, 0, 1})
		if cross(x, y) != z {
			t.Errorf("frame %d isn't a rotation: %v %v %v", g, x, y, z)
		}
		if FromFrame(g, Vec3{0, -1, 0}) != GravDir(g) || Up(g) != y || GravOf(GravDir(g)) != g {
			t.Errorf("frame %d: down %v, gravity %v", g, FromFrame(g, Vec3{0, -1, 0}), GravDir(g))
		}
		seen[GravDir(g)] = true
		v := Vec3{1.25, -3.5, 7.125}
		if ToFrame(g, FromFrame(g, v)) != v || FromFrame(g, ToFrame(g, v)) != v {
			t.Errorf("frame %d: round trip", g)
		}
		for h := range 6 { // a gravity seen from another frame and back
			if FromFrame(g, GravDir(GravIn(g, h))) != GravDir(h) {
				t.Errorf("GravIn(%d, %d)", g, h)
			}
		}
		b := content.Box{Min: [3]float64{1, 2, 3}, Max: [3]float64{4, 6, 8}, Kind: "k", Ramp: 2}
		var l, back content.Box
		BoxToFrame(g, &b, &l)
		BoxFromFrame(g, &l, &back)
		if back.Min != b.Min || back.Max != b.Max || back.Ramp != 0 || back.Kind != "k" {
			t.Errorf("frame %d: box %v -> %v -> %v", g, b, l, back)
		}
	}
	if len(seen) != 6 || GravOf(Vec3{1, 1, 0}) != GravDown {
		t.Fatal("six gravities")
	}
}

// turned: map m turned by gravity g's frame (every box FromFrame; wedges
// become their boxes): what a body with gravity g sees as m.
func turned(m content.Map, g int) content.Map {
	out := m
	out.Boxes = make([]content.Box, len(m.Boxes))
	for i := range m.Boxes {
		BoxFromFrame(g, &m.Boxes[i], &out.Boxes[i])
	}
	out.Movers = nil
	return out
}

// A body with gravity g in a map turned by g moves exactly as a body with
// the world's gravity in the map itself, turned: same code, exact rotations.
// Peers (some with gravities of their own) are turned along.
func TestGravityRotatedRunsMatchTurned(t *testing.T) {
	m := turned(maps.DeSlop(), GravDown) // its wedges as boxes, like any rotated frame sees them
	runs := randomRuns([]Vec3{m.Spawns[0], m.Spawns[3], m.Spawns[6]})
	for g := 1; g < 6; g++ {
		tm := turned(m, g)
		ref, rot := NewWorld(&m), NewWorld(&tm)
		for ri, rn := range runs {
			rn.Peers[0].G = GravPX  // a peer standing on a wall: its hull turned
			rn.Peers[1].Core = true // and a core
			ref.Peers = rn.Peers
			var tp []Peer
			for _, p := range rn.Peers {
				tp = append(tp, Peer{Pos: FromFrame(g, p.Pos), H: p.H, Lo: p.Lo, Core: p.Core, Drawn: p.Drawn, G: GravOf(FromFrame(g, GravDir(p.G)))})
			}
			rot.Peers = tp
			a, b := NewBody(rn.Spawn), NewBody(FromFrame(g, rn.Spawn))
			a.Yaw, b.Yaw, b.Grav = rn.Yaw, rn.Yaw, g
			for ti, tk := range rn.Ticks[:600] {
				if tk.Yaw != nil {
					a.Yaw, b.Yaw = *tk.Yaw, *tk.Yaw
				}
				in := Input{F: tk.F, R: tk.R, Walk: tk.Walk, Jump: tk.Jump, Crouch: tk.Crouch, Speed: rn.Speed}
				ea, eb := ref.Step(a, in, 0.01), rot.Step(b, in, 0.01)
				if FromFrame(g, a.Pos) != b.Pos || FromFrame(g, a.Vel) != b.Vel || a.OnGround != b.OnGround || a.Crouched != b.Crouched || ea != eb {
					t.Fatalf("gravity %d run %d tick %d: turned %v %v %v, rotated %v %v %v", g, ri, ti, FromFrame(g, a.Pos), FromFrame(g, a.Vel), a.OnGround, b.Pos, b.Vel, b.OnGround)
				}
			}
		}
	}
}

// In a closed room, a body with each gravity falls onto that wall and stands
// on it; walking moves it along the wall.
func TestGravityStandsOnEveryWall(t *testing.T) {
	room := content.Map{Boxes: []content.Box{
		{Min: [3]float64{-10, -1, -10}, Max: [3]float64{10, 0, 10}}, {Min: [3]float64{-10, 10, -10}, Max: [3]float64{10, 11, 10}},
		{Min: [3]float64{-11, 0, -10}, Max: [3]float64{-10, 10, 10}}, {Min: [3]float64{10, 0, -10}, Max: [3]float64{11, 10, 10}},
		{Min: [3]float64{-10, 0, -11}, Max: [3]float64{10, 10, -10}}, {Min: [3]float64{-10, 0, 10}, Max: [3]float64{10, 10, 11}},
	}}
	w := NewWorld(&room)
	for g := range 6 {
		b := NewBody(Vec3{0, 5, 0})
		b.Grav = g
		for range 300 {
			w.Step(b, Input{}, 0.01)
		}
		d := GravDir(g)
		wall := 10.0
		if g == GravDown {
			wall = 0
		} else if g == GravUp {
			wall = 10
		}
		axis := 0
		for a := range 3 {
			if d[a] != 0 {
				axis = a
			}
		}
		feet := b.Pos[axis]
		if !b.OnGround || (g != GravDown && math.Abs(math.Abs(feet)-wall) > 1e-9) || (g == GravDown && feet != 0) {
			t.Fatalf("gravity %d: feet %v on ground %v", g, b.Pos, b.OnGround)
		}
		p0 := b.Pos
		for range 100 {
			w.Step(b, Input{F: 1}, 0.01)
		}
		if b.Pos[axis] != p0[axis] || math.Hypot(math.Hypot(b.Pos[0]-p0[0], b.Pos[1]-p0[1]), b.Pos[2]-p0[2]) < 4 || !b.OnGround {
			t.Fatalf("gravity %d: walked %v -> %v", g, p0, b.Pos)
		}
		// jump: away from the wall, back down onto it
		w.Step(b, Input{Jump: true}, 0.01)
		if b.OnGround || float64(b.Vel[0]*-d[0])+float64(b.Vel[1]*-d[1])+float64(b.Vel[2]*-d[2]) <= 0 {
			t.Fatalf("gravity %d: jump %v", g, b.Vel)
		}
	}
}

// The generated JS steps rotated bodies as Go does, bit for bit: de_slop
// turned by each gravity, its runs turned along.
func TestGravityParityWithJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	m := turned(maps.DeSlop(), GravDown)
	for _, g := range []int{GravUp, GravNX, GravPZ} {
		tm := turned(m, g)
		runs := randomRuns([]Vec3{m.Spawns[0], m.Spawns[5]})
		for i := range runs {
			runs[i].Spawn, runs[i].Grav = FromFrame(g, runs[i].Spawn), g
			for k := range runs[i].Peers {
				runs[i].Peers[k].Pos = FromFrame(g, runs[i].Peers[k].Pos)
				runs[i].Peers[k].G = g
			}
		}
		parityRuns(t, node, tm, runs, 10, 2, 50, MoveRules{})
	}
}

// gs_megabyte's escalators in a turned frame: their boxes at the command's
// time collide (no riding, v1), the same in the generated JS. A body that
// never stepped turned touches nobody.
func TestGravityMoversParityWithJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	mb := maps.GsMegabyte()
	var esc []Vec3
	for _, mv := range mb.Movers {
		x := (mv.Min[0] + mv.Max[0]) / 2
		esc = append(esc, Vec3{x, mv.Min[1] + mv.Rise, mv.Min[2] + 0.3}, Vec3{x, mv.Max[1], mv.Max[2] - 0.3})
	}
	runs := randomRuns(esc)
	for i := range runs {
		runs[i].Grav = GravNZ
		for k := range runs[i].Peers {
			runs[i].Peers[k].G = GravNZ
			runs[i].Peers[k].Core = k == 3
		}
	}
	parityRuns(t, node, mb, runs, 10, 2, 20, MoveRules{}) // fewer touches: the peers stand on the escalators' ends
	w := NewWorld(&mb)
	b := NewBody(esc[0])
	b.Grav = GravUp
	if w.Touching(b, 1) {
		t.Fatal("touching before any turned step")
	}
}
