package sim

import (
	"math"
	"os/exec"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// boxRoom: a closed 20 m cube (floor y 0, walls at ±10, ceiling y 10) and a
// wedge on the floor.
func boxRoom() content.Map {
	return content.Map{Boxes: []content.Box{
		{Min: [3]float64{-10, -1, -10}, Max: [3]float64{10, 0, 10}}, {Min: [3]float64{-10, 10, -10}, Max: [3]float64{10, 11, 10}},
		{Min: [3]float64{-11, 0, -10}, Max: [3]float64{-10, 10, 10}}, {Min: [3]float64{10, 0, -10}, Max: [3]float64{11, 10, 10}},
		{Min: [3]float64{-10, 0, -11}, Max: [3]float64{10, 10, -10}}, {Min: [3]float64{-10, 0, 10}, Max: [3]float64{10, 10, 11}},
		{Min: [3]float64{-6, 0, 4}, Max: [3]float64{-2, 2, 8}, Ramp: content.RampPX},
	}}
}

func hookWorld() (*World, *Body) {
	m := boxRoom()
	w := NewWorld(&m)
	w.Move.Hook = DefaultHook()
	b := NewBody(Vec3{0, 0, 0})
	b.OnGround, b.Boots, b.Energy = true, true, 100
	return w, b
}

// runHook steps b n commands of input in (the hook press only on the first).
func runHook(w *World, b *Body, in Input, n int) (evs []int) {
	for i := range n {
		x := in
		x.HookPress = in.HookPress && i == 0
		evs = append(evs, w.Step(b, x, 0.01).Hook)
	}
	return evs
}

// Fire at the +x wall: it latches at the hit point, pulls the body there
// (lifting it off the floor), and touching the wall flips its gravity onto
// it: standing on the wall, the hook let go.
func TestHookPullsAndFlipsOntoTheFace(t *testing.T) {
	w, b := hookWorld()
	b.Yaw, b.Pitch = -math.Pi/2, 0.3 // toward +x, a little up
	evs := runHook(w, b, Input{Hook: true, HookPress: true}, 1)
	if evs[0] != HookFired || !b.Hook.On || b.Hook.N != (Vec3{-1, 0, 0}) || math.Abs(b.Hook.At[0]-10) > 1e-9 || math.Abs(b.Energy-84.8) > 1e-9 { // the shot, and the first tick's drain
		t.Fatalf("fire: ev %v hook %+v energy %v", evs, b.Hook, b.Energy)
	}
	var flipped int
	for i := range 200 {
		ev := w.Step(b, Input{Hook: true}, 0.01).Hook
		if i == 5 && b.OnGround {
			t.Fatal("not lifted off")
		}
		if ev == HookFlipped {
			flipped = i
			break
		}
	}
	if flipped == 0 || b.Grav != GravPX || !b.OnGround || b.Hook.On || b.Pos[0] != 10 || b.Vel[0] != 0 {
		t.Fatalf("flip at %d: grav %d pos %v vel %v hook %+v", flipped, b.Grav, b.Pos, b.Vel, b.Hook)
	}
	if b.Energy >= 84.8 || b.Energy <= 0 {
		t.Fatalf("energy %v", b.Energy)
	}
	// standing on the wall: it stays there
	runHook(w, b, Input{}, 50)
	if b.Grav != GravPX || b.Pos[0] != 10 || !b.OnGround {
		t.Fatalf("on the wall: %v %v", b.Pos, b.OnGround)
	}
	// Hook the floor and come back down. Its view is in its frame (gravity
	// +x: frame (v1, -v0, v2)): world (-1, -0.5, 0) ahead and down to the
	// floor is the frame's (-0.5, 1, 0): yaw 90°, pitch atan(2).
	b.Yaw, b.Pitch = math.Pi/2, math.Atan(2)
	if evs := runHook(w, b, Input{Hook: true, HookPress: true}, 1); evs[0] != HookFired || b.Hook.N != (Vec3{0, 1, 0}) {
		t.Fatalf("floor hook: %v %+v", evs, b.Hook)
	}
	for range 300 {
		if w.Step(b, Input{Hook: true}, 0.01).Hook == HookFlipped {
			break
		}
	}
	if b.Grav != GravDown || b.Pos[1] != 0 || !b.OnGround {
		t.Fatalf("back on the floor: grav %d pos %v", b.Grav, b.Pos)
	}
}

// Letting go keeps the momentum; holding drains the energy; out of energy
// the hook lets go; a miss costs a shot; energy recharges unhooked, capped.
func TestHookEnergyAndRelease(t *testing.T) {
	w, b := hookWorld()
	b.Yaw, b.Pitch = -math.Pi/2, 0.5
	runHook(w, b, Input{Hook: true, HookPress: true}, 20)
	v := b.Vel
	if ev := w.Step(b, Input{}, 0.01).Hook; ev != HookReleased || b.Hook.On {
		t.Fatalf("release: %v", ev)
	}
	if math.Abs(b.Vel[0]-v[0]) > 1e-9 || b.Vel[0] < 5 {
		t.Fatalf("momentum: %v -> %v", v, b.Vel)
	}
	// out of energy
	w2, b2 := hookWorld()
	b2.Yaw, b2.Pitch, b2.Energy = -math.Pi/2, 0.5, 16
	evs := runHook(w2, b2, Input{Hook: true, HookPress: true}, 10)
	if evs[0] != HookFired || b2.Hook.On || b2.Energy > 1 { // (recharging again since)
		t.Fatalf("out of energy: %v %+v %v", evs, b2.Hook, b2.Energy)
	}
	released := false
	for _, e := range evs {
		released = released || e == HookReleased
	}
	if !released {
		t.Fatal("no release event")
	}
	// not enough for a shot: nothing happens
	b2.Energy = 10
	if ev := w2.Step(b2, Input{Hook: true, HookPress: true}, 0.01).Hook; ev != 0 || b2.Hook.On {
		t.Fatalf("shot without energy: %v", ev)
	}
	// a miss (range 2 m in a 20 m room) costs a shot
	w3, b3 := hookWorld()
	w3.Move.Hook.Range = 2
	b3.Yaw = -math.Pi / 2
	if ev := w3.Step(b3, Input{Hook: true, HookPress: true}, 0.01).Hook; ev != HookMissed || b3.Hook.On || b3.Energy != 85 {
		t.Fatalf("miss: %v %v", ev, b3.Energy)
	}
	// recharge, capped
	for range 200 {
		w3.Step(b3, Input{}, 0.01)
	}
	if b3.Energy != 100 {
		t.Fatalf("recharge %v", b3.Energy)
	}
	// without the boots: no hook at all
	w4, b4 := hookWorld()
	b4.Boots, b4.Yaw = false, -math.Pi/2
	if ev := w4.Step(b4, Input{Hook: true, HookPress: true}, 0.01).Hook; ev != 0 || b4.Hook.On {
		t.Fatalf("no boots: %v", ev)
	}
}

// A wedge can be hooked (and pulls) but never flips gravity (v1); the swing:
// hooked to the ceiling, the rope never gets longer.
func TestHookWedgeAndRope(t *testing.T) {
	w, b := hookWorld()
	b.Pos = Vec3{-8, 0, 6}
	b.Yaw, b.Pitch = -math.Pi/2, -0.2 // toward the wedge (+x), a little down
	if evs := runHook(w, b, Input{Hook: true, HookPress: true}, 1); evs[0] != HookFired || b.Hook.Flip {
		t.Fatalf("wedge: %v %+v", evs, b.Hook)
	}
	for range 150 {
		w.Step(b, Input{Hook: true}, 0.01)
	}
	if b.Grav != GravDown {
		t.Fatal("flipped onto a wedge")
	}
	w2, b2 := hookWorld()
	b2.Pos, b2.Pitch = Vec3{0, 3, 0}, 1.2 // the ceiling, ahead and up
	b2.OnGround = false
	runHook(w2, b2, Input{Hook: true, HookPress: true}, 1)
	rope := b2.Hook.Rope
	for range 30 {
		w2.Step(b2, Input{Hook: true}, 0.01)
		if b2.Hook.Rope > rope {
			t.Fatalf("the rope got longer: %v -> %v", rope, b2.Hook.Rope)
		}
		rope = b2.Hook.Rope
	}
}

// Pulled into a corner with no room to stand on the face: no flip.
func TestHookNoRoomNoFlip(t *testing.T) {
	m := boxRoom()
	m.Boxes = append(m.Boxes, content.Box{Min: [3]float64{8, 0, -10}, Max: [3]float64{8.5, 10, 10}}) // a slab 1.5 m off the +x wall
	w := NewWorld(&m)
	w.Move.Hook = DefaultHook()
	b := NewBody(Vec3{0, 0, 0})
	b.Boots, b.Energy, b.OnGround = true, 100, true
	b.Yaw = -math.Pi / 2
	runHook(w, b, Input{Hook: true, HookPress: true}, 200)
	if b.Grav != GravDown {
		t.Fatalf("flipped with no room: %v", b.Pos)
	}
}

// The generated JS runs the hook as Go does, bit for bit: random runs with
// the boots on lab_boots and de_slop, the hook pressed and held at varied
// pitches (latching, pulling, swinging, flipping onto walls and back,
// running out of energy).
func TestHookParityWithJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	for _, m := range []content.Map{maps.LabBoots(), maps.DeSlop()} {
		spawns := make([]Vec3, len(m.Spawns))
		for i, sp := range m.Spawns {
			spawns[i] = sp
		}
		runs := randomRuns(spawns)
		for i := range runs {
			runs[i].Boots = true
			for k := range runs[i].Ticks {
				tk := &runs[i].Ticks[k]
				ph := k % 160 // every 1.6 s: press, hold 1.1 s, let go
				tk.Hook, tk.Press = ph < 110, ph == 0
			}
		}
		parityRuns(t, node, m, runs, 10, 2, 0, MoveRules{Hook: DefaultHook()})
		if hookEvents[HookFired] < 20 || hookEvents[HookFlipped] < 3 || hookEvents[HookReleased] < 10 {
			t.Fatalf("%s: weak coverage: hook events %v", m.Name, hookEvents)
		}
	}
}
