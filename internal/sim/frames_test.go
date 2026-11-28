package sim

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// A passenger standing in MVP-1's aisle is in the cabin's frame from boarding
// to the drop, at the same place in it all the way up (to well under a
// millimetre, whatever the ship's speed); the one left on the jet bridge is in
// none. Each command's body is at the end of its command (w.T + dt).
func TestFrameOfRocketPassenger(t *testing.T) {
	m := maps.LabMoon()
	d := m.Departures[0]
	w := NewWorld(&m)
	in := NewBody(d.Board)
	out := NewBody(Vec3{d.Door[0] - 2, d.Door[1], d.Door[2]})
	var l0 Vec3
	framed, worst := 0, 0.0
	for tick := range 2590 { // to 25.9 s: over the Moon, before the floor opens
		w.T = float64(tick) / 100
		w.Step(in, Input{}, 0.01)
		w.Step(out, Input{}, 0.01)
		bt := w.T + 0.01
		if fr := FrameOf(m.Movers, m.GravZones, out, bt); fr != 0 {
			t.Fatalf("tick %d: the one on the bridge is in frame %d", tick, fr)
		}
		fr := FrameOf(m.Movers, m.GravZones, in, bt)
		if fr != d.Mover {
			t.Fatalf("tick %d: the passenger at %v is in frame %d, want the cabin's %d", tick, in.Pos, fr, d.Mover)
		}
		framed++
		l := ToFrameLocal(m.Movers, fr, bt, in.Pos)
		if tick == 0 {
			l0 = l
		}
		for a := range 3 {
			worst = math.Max(worst, math.Abs(l[a]-l0[a]))
		}
		if p := FromFrameLocal(m.Movers, fr, bt, l); math.Abs(p[1]-in.Pos[1]) > 1e-9 {
			t.Fatalf("tick %d: placed back at %v, the body at %v", tick, p, in.Pos)
		}
	}
	if worst > 1e-4 {
		t.Errorf("the passenger's place in the cabin moved %.6f m over the climb (standing still: want ~0)", worst)
	}
	t.Logf("%d ticks in the cabin's frame, place in it within %.2g m", framed, worst)
}

// On a lift's top a body is in its frame; jumping off its top, on an
// escalator tread, or under the lift, it is in none. A frame the map lacks is
// the world's.
func TestFrameOfLift(t *testing.T) {
	lift := content.Mover{Kind: content.MoverSlide, Mat: content.Metal, Min: [3]float64{-1, -0.5, -1}, Max: [3]float64{1, 0, 1}, Move: [3]float64{0, 3, 0}, Period: 6}
	esc := content.Mover{Kind: content.MoverEscalator, Mat: content.Escalator, Min: [3]float64{5, 0, -5}, Max: [3]float64{6.2, 5, 4}, Axis: 2, Up: 1, Rise: 0.25, Run: 0.45, Speed: 0.75, Flat: 2}
	movers := []content.Mover{esc, lift}
	at := 1.5 // half way up: the lift's top at 1.5
	b := NewBody(Vec3{0.2, 1.5, 0.3})
	b.OnGround = true
	if fr := FrameOf(movers, nil, b, at); fr != 2 {
		t.Fatalf("on the lift: frame %d, want 2", fr)
	}
	if l := ToFrameLocal(movers, 2, at, b.Pos); l != (Vec3{0.2, 0, 0.3}) {
		t.Errorf("on the lift: local %v, want its place on the top as built", l)
	}
	b.OnGround = false
	if fr := FrameOf(movers, nil, b, at); fr != 0 {
		t.Errorf("jumping: frame %d, want none", fr)
	}
	b.OnGround = true
	b.Pos = Vec3{0.2, 0.5, 0.3} // under it
	if fr := FrameOf(movers, nil, b, at); fr != 0 {
		t.Errorf("under the lift: frame %d, want none", fr)
	}
	tr := MoverBoxes(movers[:1], at)[5]
	b.Pos = Vec3{(tr.Min[0] + tr.Max[0]) / 2, tr.Max[1], (tr.Min[2] + tr.Max[2]) / 2}
	if fr := FrameOf(movers, nil, b, at); fr != 0 {
		t.Errorf("on an escalator tread: frame %d, want none", fr)
	}
	if o := FrameOffset(movers, 7, at); o != (Vec3{}) {
		t.Errorf("a frame the map lacks: offset %v, want the world's", o)
	}
	if FrameOf(nil, nil, b, at) != 0 {
		t.Error("no movers: no frame")
	}
}

// ClockJump: a rider keeps its place in its frame, a bystander its place in
// the world, and a jump into a time a mover box fills its hull waits.
func TestClockJump(t *testing.T) {
	m := maps.LabMoon()
	d := m.Departures[0]
	mv := &m.Movers[d.Mover-1]
	// A passenger standing in the aisle in the climb (17 s), its clock jumping to the cross (23 s).
	o17 := MoverOffset(mv, 17)
	b := NewBody(Vec3{d.Board[0] + 1 + o17[0], d.Board[1] + o17[1], d.Board[2] + o17[2]})
	b.OnGround = true
	at, ok := ClockJump(m.Movers, m.GravZones, b, 17, 23)
	o23 := MoverOffset(mv, 23)
	if want := (Vec3{d.Board[0] + 1 + o23[0], d.Board[1] + o23[1], d.Board[2] + o23[2]}); !ok || math.Abs(at[0]-want[0])+math.Abs(at[1]-want[1])+math.Abs(at[2]-want[2]) > 1e-9 {
		t.Fatalf("rider: %v %v, want %v (its place in the cabin, where the ship is at 23 s)", at, ok, want)
	}
	// Someone where the docked cabin's floor is, while the ship is away (falling through the empty dock), its clock jumping to a docked time: the floor fills its hull.
	away := NewBody(Vec3{d.Board[0] + 1, d.Board[1] - 0.3, d.Board[2]})
	if FrameOf(m.Movers, m.GravZones, away, 23) != 0 {
		t.Fatalf("in the frame of a ship that's away")
	}
	if _, ok := ClockJump(m.Movers, m.GravZones, away, 23, 5); ok {
		t.Fatalf("jumped into the docked cabin's floor")
	}
	// A bystander on the terminal's floor: stays put.
	by := NewBody(Vec3{-140, 0, 10})
	by.OnGround = true
	if at, ok := ClockJump(m.Movers, m.GravZones, by, 3, 23); !ok || at != by.Pos {
		t.Fatalf("bystander: %v %v", at, ok)
	}
}
