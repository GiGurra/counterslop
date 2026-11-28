package sim

import (
	"fmt"
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
			{Name: "half", Min: [3]float64{-50, -5, -50}, Max: [3]float64{-30, 20, 50}, G: 0.5}, // no jump given: the world's
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
			if l.Mover > 0 {
				continue // MVP-1's drop pods: TestLabMoonRocketRide
			}
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

// rocketMap: a cabin (floor, walls, roof: path movers) docked at the origin
// for 5 s, up 50 m by 10 s, 40 m east by 15 s, then its floor opens (hidden
// 15..18) and a drop-pod launcher in it throws down; cycle 20 s. Space (0 g)
// over y 10; the cabin carries its own 1 g.
func rocketMap() content.Map {
	path := [][4]float64{{0, 0, 0, 0}, {5, 0, 0, 0}, {10, 0, 50, 0}, {15, 40, 50, 0}}
	mv := func(x0, y0, z0, x1, y1, z1 float64, hide [2]float64) content.Mover {
		return content.Mover{Kind: content.MoverPath, Mat: "metal", Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}, Period: 20, Path: path, Hide: hide}
	}
	return content.Map{
		Boxes: []content.Box{{Min: [3]float64{-20, -1, -20}, Max: [3]float64{-6, 0, 20}, Kind: "concrete"}},
		Movers: []content.Mover{
			mv(-4, -0.5, -3, 4, 0, 3, [2]float64{15, 18}), // the floor: a trapdoor
			mv(-4.5, -0.5, -3.5, -4, 3.5, 3.5, [2]float64{}), mv(4, -0.5, -3.5, 4.5, 3.5, 3.5, [2]float64{}),
			mv(-4, -0.5, -3.5, 4, 3.5, -3, [2]float64{}), mv(-4, -0.5, 3, 4, 3.5, 3.5, [2]float64{}),
			mv(-4.5, 3.5, -3.5, 4.5, 4, 3.5, [2]float64{}),
		},
		GravZones: []content.GravZone{
			{Name: "space", Min: [3]float64{-100, 10, -100}, Max: [3]float64{100, 200, 100}, G: 0},
			{Name: "cabin", Min: [3]float64{-4, -0.5, -3}, Max: [3]float64{4, 3.5, 3}, G: 1, Mover: 1},
		},
		Launchers: []content.Launcher{{Name: "pods", Min: [3]float64{-4, -1.5, -3}, Max: [3]float64{4, 1, 3}, Vel: [3]float64{0, -12, 0}, Mover: 1, From: 15, To: 18, Air: true}},
	}
}

// A body standing in the cabin rides it up and east through space (the
// cabin's own gravity keeps it on the floor), then the floor opens and the
// pod launcher throws it down, once.
func TestRocketRideAndDropPod(t *testing.T) {
	m := rocketMap()
	w := NewWorld(&m)
	b := NewBody(Vec3{1, 0, 0})
	launches := 0
	for tick := range 2000 {
		w.T = float64(tick) * 0.01
		ev := w.Step(b, Input{}, 0.01)
		if ev.Launched {
			launches++
		}
		switch tick {
		case 1490: // 14.9 s: carried to the top of the climb and east, standing
			if !b.OnGround || math.Abs(b.Pos[0]-41) > 0.05 || math.Abs(b.Pos[1]-50) > 0.01 {
				t.Fatalf("at 14.9 s: pos %v ground %v", b.Pos, b.OnGround)
			}
		case 1600: // 16 s: dropped through the trapdoor at the pods' speed, in space
			if b.OnGround || b.Pos[1] > 45 || b.Vel[1] > -12 || b.Vel[1] < -15 || launches != 1 {
				t.Fatalf("at 16 s: pos %v vel %v launches %d", b.Pos, b.Vel, launches)
			}
		}
	}
}

// The path is eased key to key and holds its last key; the cycle wraps.
func TestPathMover(t *testing.T) {
	m := rocketMap().Movers[0]
	for _, c := range []struct {
		t    float64
		want Vec3
	}{{0, Vec3{}}, {5, Vec3{}}, {7.5, Vec3{0, 25, 0}}, {10, Vec3{0, 50, 0}}, {12.5, Vec3{20, 50, 0}}, {19.9, Vec3{40, 50, 0}}, {20, Vec3{}}, {27.5, Vec3{0, 25, 0}}} {
		if o := MoverOffset(&m, c.t); math.Abs(o[0]-c.want[0])+math.Abs(o[1]-c.want[1])+math.Abs(o[2]-c.want[2]) > 1e-9 {
			t.Errorf("t %v: offset %v, want %v", c.t, o, c.want)
		}
	}
	if hidden(&m, 14.99) || !hidden(&m, 15) || !hidden(&m, 17.99) || hidden(&m, 18) {
		t.Error("the trapdoor's window")
	}
	wrap := m
	wrap.Hide = [2]float64{18, 2}
	if !hidden(&wrap, 19) || !hidden(&wrap, 21) || hidden(&wrap, 22.5) {
		t.Error("a window round the cycle's end")
	}
}

// lab_moon's MVP-1: a passenger standing in the aisle at departure rides up
// and across and is dropped onto the Moon's plateau; one still on the jet
// bridge when the door shuts stays on Earth. Both movement models.
func TestLabMoonRocketRide(t *testing.T) {
	m := maps.LabMoon()
	d := m.Departures[0]
	for _, mv := range []MoveRules{{}, {Model: MoveSource, AutoBhop: true}} {
		w := NewWorld(&m)
		w.Move = mv
		in := NewBody(d.Board)
		out := NewBody(Vec3{d.Door[0] - 2, d.Door[1], d.Door[2]})
		drops := 0
		for tick := range 4000 {
			w.T = float64(tick) / 100
			if w.Step(in, Input{}, 0.01).Launched {
				drops++
			}
			w.Step(out, Input{}, 0.01)
			if tick == 2590 && (!in.OnGround || math.Abs(in.Pos[0]-90) > 0.3 || math.Abs(in.Pos[1]-70) > 0.1) {
				t.Fatalf("model %d at 25.9 s: passenger at %v (ground %v), want standing at 90, 70", mv.Model, in.Pos, in.OnGround)
			}
		}
		if !in.OnGround || math.Abs(in.Pos[1]-moonTop) > 0.01 || in.Pos[0] < 80 || in.Pos[0] > 100 || drops != 1 {
			t.Errorf("model %d: passenger ended at %v (ground %v), %d drops; want on the Moon once", mv.Model, in.Pos, in.OnGround, drops)
		}
		if !out.OnGround || out.Pos[1] != 0 || out.Pos[0] > d.Door[0] {
			t.Errorf("model %d: the one left on the bridge ended at %v", mv.Model, out.Pos)
		}
	}
}

// GravityAt (the server's grenades): the innermost fixed zone scales the
// gravity; a zone a ship carries doesn't count (its cabin is somewhere else).
func TestGravityAt(t *testing.T) {
	zones := []content.GravZone{
		{Name: "space", Min: [3]float64{-50, 0, -50}, Max: [3]float64{50, 100, 50}, G: 0},
		{Name: "moon", Min: [3]float64{0, 0, 0}, Max: [3]float64{10, 10, 10}, G: 1.0 / 6},
		{Name: "cabin", Min: [3]float64{0, 0, 0}, Max: [3]float64{10, 10, 10}, G: 1, Mover: 1},
	}
	for _, c := range []struct {
		p    Vec3
		want float64
	}{{Vec3{5, 5, 5}, 22.0 / 6}, {Vec3{-20, 5, 5}, 0}, {Vec3{60, 5, 5}, 22}} {
		if got := GravityAt(zones, c.p, 22); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("at %v: %v, want %v", c.p, got, c.want)
		}
	}
}

// The movers' cycle clock and offsets at the edges: no period, no path, an
// escalator (no offset), a wrapping hide window.
func TestMoverClockEdges(t *testing.T) {
	m := content.Mover{Kind: content.MoverPath}
	if CycleT(&m, 12.5) != 0 || MoverOffset(&m, 3) != (Vec3{}) {
		t.Fatal("a path mover without a period or keys stays put")
	}
	if o := MoverOffset(&content.Mover{Kind: content.MoverEscalator, Period: 4}, 1); o != (Vec3{}) {
		t.Fatalf("an escalator's box has no offset: %v", o)
	}
	w := content.Mover{Kind: content.MoverPath, Period: 10, Hide: [2]float64{8, 2}}
	for _, c := range []struct {
		t    float64
		want bool
	}{{9, true}, {1, true}, {5, false}, {13, false}, {18.5, true}} {
		if hidden(&w, c.t) != c.want {
			t.Errorf("hide 8..2 (wrapping) at %v: %v", c.t, !c.want)
		}
	}
}

// cq_moonshot's MVP-1 drops a wave three ways: a passenger in the aft third
// of the cabin lands in the solar farm (B), one in the middle in the drop
// zone (A), one in the fore third by the rovers (C). Both movement models.
func TestMoonshotPodsFlank(t *testing.T) {
	m := maps.CqMoonshot()
	d := m.Departures[0]
	pts := map[string]Vec3{}
	for _, p := range m.Points {
		pts[p.ID] = p.Pos
	}
	for _, mv := range []MoveRules{{}, {Model: MoveSource, AutoBhop: true}} {
		w := NewWorld(&m)
		w.Move = mv
		riders := map[string]*Body{
			"B": NewBody(Vec3{d.Board[0] - 5, d.Board[1], d.Board[2]}),
			"A": NewBody(d.Board),
			"C": NewBody(Vec3{d.Board[0] + 5, d.Board[1], d.Board[2]}),
		}
		for tick := range 3400 { // to 34 s: down by 27.5 or so
			w.T = float64(tick) / 100
			for _, id := range []string{"A", "B", "C"} {
				w.Step(riders[id], Input{}, 0.01)
			}
		}
		for id, b := range riders {
			p := pts[id]
			if !b.OnGround || math.Abs(b.Pos[1]-p[1]) > 1.5 || math.Hypot(b.Pos[0]-p[0], b.Pos[2]-p[2]) > 12 {
				t.Errorf("model %d: the %s rider is at %v (ground %v), want within 12 m of %v", mv.Model, id, b.Pos, b.OnGround, p)
			}
		}
	}
}

// padMap: a floor at y 0 (x -10..10), a platform 30 m east and 6 m up
// (x 28..36, top at 6) over nothing, a jump pad at the origin aimed at the
// platform's middle with its top 4 m over it, and one back.
func padMap() content.Map {
	return content.Map{
		Boxes: []content.Box{{Min: [3]float64{-10, -1, -10}, Max: [3]float64{10, 0, 10}, Kind: "concrete"},
			{Min: [3]float64{28, 5, -4}, Max: [3]float64{36, 6, 4}, Kind: "concrete"}},
		Launchers: []content.Launcher{
			{Name: "out", Min: [3]float64{-1, -0.1, -1}, Max: [3]float64{1, 0.3, 1}, Target: [3]float64{32, 6, 0}, Apex: 4},
			{Name: "back", Min: [3]float64{33, 5.9, -1}, Max: [3]float64{35, 6.3, 1}, Target: [3]float64{0, 0, 5}, Apex: 2},
		},
	}
}

// flyPad: a body set down at from flies the pads (in rules mv, holding in);
// where it lands (on the ground after a throw) and how high it went.
func flyPad(t *testing.T, m content.Map, mv MoveRules, from Vec3, in Input) (Vec3, float64) {
	t.Helper()
	w := NewWorld(&m)
	w.Move = mv
	b := NewBody(from)
	launched, top := false, from[1]
	for range 1000 {
		ev := w.Step(b, in, 0.01)
		launched = launched || ev.Launched
		top = math.Max(top, b.Pos[1])
		if launched && b.OnGround && !ev.Launched {
			return b.Pos, top
		}
	}
	t.Fatalf("launched %v, never landed: %v", launched, b.Pos)
	return Vec3{}, 0
}

// A jump pad throws a body onto its target, the top of the arc Apex over the
// higher end, wherever on the pad it stands, in both movement models (and in
// a low-gravity zone, the arc is the zone's).
func TestJumpPadLandsOnTarget(t *testing.T) {
	for _, mv := range []MoveRules{{}, {Model: MoveSource, AirAccel: 12}} {
		for _, from := range []Vec3{{0, 0, 0}, {0.8, 0, -0.7}, {-0.9, 0, 0.9}} {
			at, top := flyPad(t, padMap(), mv, from, Input{})
			if math.Hypot(at[0]-32, at[2]) > 0.5 || math.Abs(at[1]-6) > 1e-6 || math.Abs(top-10) > 0.3 {
				t.Errorf("model %d from %v: landed %v, top %.2f (want 32 6 0, top 10)", mv.Model, from, at, top)
			}
		}
		at, _ := flyPad(t, padMap(), mv, Vec3{34, 6, 0}, Input{}) // and back down
		if math.Hypot(at[0], at[2]-5) > 0.5 || at[1] != 0 {
			t.Errorf("model %d: back landed %v", mv.Model, at)
		}
	}
	m := padMap()
	m.GravZones = []content.GravZone{{Name: "moon", Min: [3]float64{-50, -5, -50}, Max: [3]float64{50, 50, 50}, G: 1.0 / 6, Jump: 1}}
	if at, top := flyPad(t, m, MoveRules{}, Vec3{0, 0, 0}, Input{}); math.Hypot(at[0]-32, at[2]) > 0.5 || math.Abs(top-10) > 0.3 {
		t.Errorf("low gravity: landed %v top %.2f", at, top)
	}
}

// Holding a key in the air doesn't brake a throw faster than a run (it
// steers it): forward along the throw lands on the target as coasting does;
// back against it lands no shorter than a run's worth.
func TestAirKeysSteerAThrowDontBrakeIt(t *testing.T) {
	east := -math.Pi / 2 // yaw facing +x
	for _, in := range []Input{{F: 1}, {F: -1}} {
		b := NewBody(Vec3{0, 0, 0})
		b.Yaw = east
		m := padMap()
		w := NewWorld(&m)
		launched := false
		for range 1000 {
			ev := w.Step(b, in, 0.01)
			launched = launched || ev.Launched
			if launched && b.OnGround && !ev.Launched {
				break
			}
		}
		if b.Pos[0] < 28 || b.Pos[1] != 6 {
			t.Errorf("holding %v: landed %v, short of the platform", in.F, b.Pos)
		}
	}
	// At a run or slower the air control is as before: a jump's run is braked.
	m := padMap()
	w := NewWorld(&m)
	b := NewBody(Vec3{-8, 0, 0})
	b.Yaw = east
	settle(w, b, Input{}, 20)
	b.Vel = Vec3{Run, 0, 0}
	w.Step(b, Input{F: -1, Jump: true}, 0.01)
	for range 10 {
		w.Step(b, Input{F: -1}, 0.01)
	}
	if b.Vel[0] > Run-1 {
		t.Errorf("a jump at a run, held back: vel %v (want braked)", b.Vel)
	}
}

// PadVel: an arc from from to to, its top apex over the higher.
func TestPadVel(t *testing.T) {
	from, to := Vec3{0, 0, 0}, Vec3{20, 5, 10}
	v := PadVel(from, to, 3, Grav)
	tUp := v[1] / Grav
	if top := v[1] * v[1] / (2 * Grav); math.Abs(top-8) > 1e-9 {
		t.Errorf("top %v want 8", top)
	}
	tt := tUp + math.Sqrt(2*3/Grav)
	if x, y, z := v[0]*tt, v[1]*tt-Grav*tt*tt/2, v[2]*tt; math.Abs(x-20) > 1e-9 || math.Abs(y-5) > 1e-9 || math.Abs(z-10) > 1e-9 {
		t.Errorf("lands at %v %v %v", x, y, z)
	}
}

// de_serverless's pads: a body standing still on each is thrown, lands and
// comes to rest on a platform (the skid after a fast landing included), not
// off its far edge into the void, in both movement models (and a jump pad's
// lands it on its target first).
func TestServerlessPadsLandOnPlatforms(t *testing.T) {
	m := maps.DeServerless()
	for _, mv := range []MoveRules{{}, {Model: MoveSource, AirAccel: 12}} {
		for _, l := range m.Launchers {
			w := NewWorld(&m)
			w.Move = mv
			b := NewBody(Vec3{(l.Min[0] + l.Max[0]) / 2, l.Min[1] + 0.12, (l.Min[2] + l.Max[2]) / 2})
			launched, landed := false, Vec3{}
			for i := range 1500 {
				w.T = float64(i) / 100
				ev := w.Step(b, Input{}, 0.01)
				if ev.Launched {
					launched = true
				}
				if launched && b.OnGround && landed == (Vec3{}) {
					landed = b.Pos
				}
				if launched && b.OnGround && math.Hypot(b.Vel[0], b.Vel[2]) < 0.05 {
					break
				}
			}
			if !launched || !b.OnGround || b.Pos[1] < -1 {
				t.Errorf("model %d, %s: launched %v, ended at %.1f %.1f %.1f on the ground %v", mv.Model, l.Name, launched, b.Pos[0], b.Pos[1], b.Pos[2], b.OnGround)
				continue
			}
			if l.Apex > 0 && math.Hypot(landed[0]-l.Target[0], landed[2]-l.Target[2]) > 0.6 {
				t.Errorf("model %d, %s: landed at %.1f %.1f %.1f, target %v", mv.Model, l.Name, landed[0], landed[1], landed[2], l.Target)
			}
			t.Logf("model %d, %s: landed %.1f %.1f %.1f, at rest %.1f %.1f %.1f", mv.Model, l.Name, landed[0], landed[1], landed[2], b.Pos[0], b.Pos[1], b.Pos[2])
		}
	}
}

// cq_moonshot's flank pods: a passenger drops straight out through the open
// floor and veers only under the hull, from anywhere in its third, standing
// or in the air as the floor opens (a jump), so the cabin's side walls never
// stop its throw and it lands where its third's pods go (by the solar farm
// -z, the rovers +z). Before, the aft and fore pods threw sideways at once,
// in the cabin: out under the side wall (the operator saw that as being
// thrown out sideways), and a jumper, still at the walls' height, hit the
// wall, lost its 24 m/s and landed in the middle's drop zone.
func TestFlankPodsClearTheHull(t *testing.T) {
	m := maps.CqMoonshot()
	d := m.Departures[0]
	mv := &m.Movers[d.Mover-1]
	ej := mv.Hide[0]
	for _, rules := range []MoveRules{{}, {Model: MoveSource, AutoBhop: true}} {
		for _, at := range [][2]float64{{-7.5, 0}, {-5, 0}, {-3.6, 2.4}, {-3, -3.5}, {3, 3.5}, {4.9, 0}, {6.2, -2}, {7.4, -3.4}} {
			for _, jt := range []float64{-1, -0.3, -0.2, -0.1, -0.02, 0.05} {
				name := fmt.Sprintf("model%d/at%v/jump%v", rules.Model, at, jt)
				w := NewWorld(&m)
				w.Move = rules
				b := NewBody(Vec3{d.Board[0] + at[0], d.Board[1], d.Board[2] + at[1]})
				hull, vz := false, 0.0
				for tick := range int((ej + 1.5) * 100) {
					w.T = float64(tick) / 100
					in := Input{}
					if jt > -1 && math.Abs(w.T-(ej+jt)) < 0.005 {
						in.Jump, in.JumpHeld = true, true
					}
					w.Step(b, in, 0.01)
					t1 := w.T + 0.01
					if t1 < ej {
						continue
					}
					l := ToFrameLocal(m.Movers, d.Mover, t1, b.Pos)
					if math.Abs(l[2]-d.Board[2]) > 4-R+0.01 && l[1]+b.H > d.Board[1]-0.5 {
						hull = true // through the side walls' height: out of the cabin sideways
					}
					if math.Abs(b.Vel[2]) > math.Abs(vz) {
						vz = b.Vel[2]
					}
					if vz != 0 && math.Abs(b.Vel[2]) < 0.9*math.Abs(vz) && !b.OnGround {
						t.Errorf("%s: its throw stopped at %.2f s (vz %.1f, was %.1f) at %v in the cabin", name, t1, b.Vel[2], vz, l)
						break
					}
				}
				want := -1.0
				if at[0] > 0 {
					want = 1
				}
				if hull {
					t.Errorf("%s: out of the cabin sideways, through the walls' height", name)
				}
				if vz*want < 0.9*22 {
					t.Errorf("%s: thrown at %.1f m/s along z, want its third's flank (%+.0f)", name, vz, 22*want)
				}
			}
		}
	}
}
