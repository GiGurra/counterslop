package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/sim"
)

// A wedge stops bullets and sight on its slope, not on its bounding box: a
// ray just over the slope goes by, one into it stops where it meets it. A
// walkable slope is a floor (never shot through); a steep one is a wall, and
// the depth paid is the ray's real path through the wedge.
func TestWallTraceWedge(t *testing.T) {
	ramp := content.W(0, 0, -2, 4, 2, 2, content.Crate, content.RampPX) // 26.6 degrees, rising to x 4
	boxes, lo, hi := wallRig(content.Crate, 0.01)
	boxes = append(boxes[:1], ramp)
	// Level along +x at 1.2 m: in at x 2.4 (the slope), out at x 4.
	w := WallTrace(boxes, lo, hi, Vec3{-5, 1.2, 0}, Vec3{1, 0, 0}, 50, 0)
	if len(w) != 1 || math.Abs(w[0].In-7.4) > 1e-9 || w[0].Keep != 0 {
		t.Fatalf("into the slope: %+v", w)
	}
	// Along z just over the slope at x 1 (slope 0.5 m there): clear.
	if w := WallTrace(boxes, lo, hi, Vec3{1, 0.55, -10}, Vec3{0, 0, 1}, 50, 1); len(w) != 0 {
		t.Fatalf("over the slope: %+v", w)
	}
	// Down onto it from above: a floor, never through.
	if w := WallTrace(boxes, lo, hi, Vec3{1, 5, 0}, Vec3{0, -1, 0}, 50, 99); len(w) != 1 || w[0].Keep != 0 || math.Abs(w[0].In-4.5) > 1e-9 {
		t.Fatalf("down onto the slope: %+v", w)
	}
	// A steep crate wedge (63 degrees) is a wall: through its real depth
	// (level at 1 m: from the slope at x 0.5 to the back at x 1), two skins.
	steep := content.W(0, 0, -2, 1, 2, 2, content.Crate, content.RampPX)
	boxes[1] = steep
	w = WallTrace(boxes, lo, hi, Vec3{-5, 1, 0}, Vec3{1, 0, 0}, 50, 1.4)
	if len(w) != 1 || math.Abs(w[0].In-5.5) > 1e-9 || math.Abs(w[0].Out-6) > 1e-9 || w[0].Keep <= 0 {
		t.Fatalf("through the steep wedge: %+v", w)
	}
	m := content.Map{Boxes: boxes}
	if !mapClear(&m, Vec3{0.2, 1.2, -5}, Vec3{0, 0, 1}, 10) {
		t.Fatal("sight over the steep slope's low end is blocked")
	}
	if mapClear(&m, Vec3{0.8, 1.2, -5}, Vec3{0, 0, 1}, 10) {
		t.Fatal("sight through the steep wedge is clear")
	}
}

// A grenade dropped on a ramp lands on the slope (not its bounding box) and
// rolls off downhill.
func TestNadeOnARamp(t *testing.T) {
	s := authServer(t)
	s.Map = maps.LabSlopes()
	p := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-22, 0, 22})
	// lab_slopes' 30 degree ramp: x -24..-20 rising to 2.31 at z -21..-18.
	n := nadeAt(s, p, "smoketest", Vec3{-22, 3, -19.5}, 0)
	n.resting, n.Vel = false, Vec3{}
	b := &s.Map.Boxes[7]
	if !sim.IsWedge(b) || b.Max[1] != 2.31 {
		t.Fatalf("box 7 is %+v, want the 30 degree ramp", b)
	}
	minY, bounced := math.Inf(1), false
	for range 150 {
		s.tickNades(0.01)
		if n.Pos[0] > -24 && n.Pos[0] < -20 {
			minY = math.Min(minY, n.Pos[1]-sim.TopAt(b, n.Pos[0]+nadeRadius, n.Pos[2]))
		}
		bounced = bounced || n.Vel[0] < -0.5
	}
	if minY < nadeRadius-0.02 || n.Pos[1] > 2 {
		t.Fatalf("landed %v off the slope, now at %v", minY, n.Pos)
	}
	if !bounced || n.Pos[0] > -23 {
		t.Fatalf("didn't go off downhill (-x): at %v vel %v", n.Pos, n.Vel)
	}
}

// Bots walk straight over walkable ramps (segmentClear): up and down lab_slopes'
// 30 degree ramp (to just short of its ledge: a box ends a straight walk) is
// clear; up the 60 degree one, or across a ramp's tall side, is not.
func TestSegmentClearRamps(t *testing.T) {
	s := authServer(t)
	s.Map = maps.LabSlopes()
	for _, c := range []struct {
		a, b Vec3
		want bool
	}{
		{Vec3{-27, 0, -19.5}, Vec3{-20.6, 2.19, -19.5}, true}, // up the 30 degree ramp
		{Vec3{-20.6, 2.19, -19.5}, Vec3{-27, 0, -19.5}, true}, // and down
		{Vec3{-27, 0, -9.5}, Vec3{-20.6, 6.5, -9.5}, false},   // the 60 degree one: a wall
		{Vec3{-22, 0, -16}, Vec3{-22, 0, -23}, false},         // across the 30 degree ramp at floor level: its side is 1.1 m high there
		{Vec3{-25, 0, -16}, Vec3{-25, 0, -17.6}, true},        // along its foot, beside it
		{Vec3{5.5, 0, -24}, Vec3{0.6, 1.88, -24}, true},       // up the deck's east ramp
		{Vec3{-3, 1.88, -17.4}, Vec3{-3, 0, -13}, true},       // down its north one
		{Vec3{5.5, 0, -24}, Vec3{0.6, 0, -24}, false},         // into the east ramp at floor level
	} {
		if got := s.segmentClear(c.a, c.b, 0.3); got != c.want {
			t.Errorf("segmentClear %s -> %s = %v, want %v", v3(c.a), v3(c.b), got, c.want)
		}
	}
}
