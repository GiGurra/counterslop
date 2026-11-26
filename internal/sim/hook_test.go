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
// moverHooks counts parityRuns' hook events on movers (as hookEvents).
var moverHooks map[int]int

// moverHookRuns: for each slide mover, bodies on the floor aiming at its
// side face, its underside and its leading face (where it is at t 0), firing
// and holding the hook at a few points of its cycle.
func moverHookRuns(m *content.Map) []run {
	var runs []run
	for i := range m.Movers {
		mv := &m.Movers[i]
		if mv.Kind != content.MoverSlide {
			continue
		}
		var b content.Box
		offsetBox(mv, MoverOffset(mv, 0), &b)
		cx, cy, cz := (b.Min[0]+b.Max[0])/2, (b.Min[1]+b.Max[1])/2, (b.Min[2]+b.Max[2])/2
		lead := b.Min[2] - 6 // in front of its -z face (or +z, sliding that way)
		if mv.Move[2] > 0 {
			lead = b.Max[2] + 6
		}
		for _, aim := range [][2]Vec3{
			{{b.Min[0] - 5, 0, cz}, {b.Min[0], cy, cz}},        // its side
			{{cx, 0, lead}, {cx, cy, cz}},                      // its leading face
			{{cx + 1, 0, (cz + lead) / 2}, {cx, b.Min[1], cz}}, // its underside
		} {
			dx, dy, dz := aim[1][0]-aim[0][0], aim[1][1]-(aim[0][1]+Eye), aim[1][2]-aim[0][2]
			yaw, pitch := math.Atan2(-dx, -dz), math.Atan2(dy, math.Hypot(dx, dz))
			rn := run{Spawn: aim[0], Yaw: yaw, Speed: 1, Boots: true}
			for k := range 720 {
				ph := k % 180 // every 1.8 s: press, hold 1.4 s, let go
				rn.Ticks = append(rn.Ticks, tick{Pitch: pitch, Hook: ph < 140, Press: ph == 0})
			}
			runs = append(runs, rn)
		}
	}
	return runs
}

func TestHookParityWithJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	for _, m := range []content.Map{maps.LabBoots(), maps.DeSlop(), maps.DeRackmount()} {
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
		runs = append(runs, moverHookRuns(&m)...)
		parityRuns(t, node, m, runs, 10, 2, 0, MoveRules{Hook: DefaultHook()})
		if hookEvents[HookFired] < 20 || hookEvents[HookFlipped] < 3 || hookEvents[HookReleased] < 10 {
			t.Fatalf("%s: weak coverage: hook events %v", m.Name, hookEvents)
		}
		if len(m.Movers) > 0 && (moverHooks[HookFired] < 10 || moverHooks[HookFlipped] < 3 || moverHooks[HookReleased] < 1) {
			t.Fatalf("%s: weak coverage: hook events on movers %v", m.Name, moverHooks)
		}
	}
}

// moverHookWorld: boxRoom with a platform (y 2..4.5, its +z face at z -5)
// in front of a body at the middle facing -z; the platform slides move
// every period (and back).
func moverHookWorld(move Vec3, period float64) (*clock, *Body) {
	m := boxRoom()
	m.Movers = []content.Mover{{Kind: content.MoverSlide, Min: [3]float64{-1, 2, -7}, Max: [3]float64{1, 4.5, -5}, Move: move, Period: period}}
	w := NewWorld(&m)
	w.Move.Hook = DefaultHook()
	b := NewBody(Vec3{0, 0, 0})
	b.OnGround, b.Boots, b.Energy = true, true, 100
	b.Yaw, b.Pitch = 0, 0.3 // toward -z, a little up: at the platform's face
	return &clock{w: w}, b
}

// The hook latches onto a moving platform (it went through movers: only the
// static boxes were traced), pulls the body to it, and touching its face (it
// slides along x: within the face's plane) flips the body onto it.
func TestHookHitsAMover(t *testing.T) {
	c, b := moverHookWorld(Vec3{4, 0, 0}, 8)
	if ev := c.step(b, Input{Hook: true, HookPress: true}).Hook; ev != HookFired || b.Hook.Mover != 1 || b.Hook.N != (Vec3{0, 0, 1}) || math.Abs(b.Hook.At[2]+5) > 1e-9 {
		t.Fatalf("fire: ev %v hook %+v, want latched on the platform's +z face at z -5", ev, b.Hook)
	}
	flipped := false
	for range 200 {
		if c.step(b, Input{Hook: true}).Hook == HookFlipped {
			flipped = true
			break
		}
	}
	if !flipped || b.Grav != GravNZ || !b.OnGround || b.Hook.On || math.Abs(b.Pos[2]+5) > 1e-9 {
		t.Fatalf("flipped %v: grav %d pos %v hook %+v, want standing on the platform's face", flipped, b.Grav, b.Pos, b.Hook)
	}
}

// The anchor rides with the mover: every command it is the same point of the
// platform's box (at the command's time), and the rope pulls toward it.
func TestHookAnchorFollowsTheMover(t *testing.T) {
	c, b := moverHookWorld(Vec3{4, 0, 0}, 8)
	c.step(b, Input{Hook: true, HookPress: true})
	off := Vec3{b.Hook.At[0] - b.Hook.Lo[0], b.Hook.At[1] - b.Hook.Lo[1], b.Hook.At[2] - b.Hook.Lo[2]}
	x0 := b.Hook.At[0]
	for range 20 {
		c.step(b, Input{Hook: true})
		box := MoverBoxes(c.w.Movers, float64(c.tick+1)/100)[0] // where it is at the step's end
		if !b.Hook.On || Vec3(box.Min) != b.Hook.Lo || Vec3(box.Max) != b.Hook.Hi {
			t.Fatalf("tick %d: hook %+v, want on the platform's box %v", c.tick, b.Hook, box)
		}
		for a := range 3 {
			if math.Abs(b.Hook.At[a]-b.Hook.Lo[a]-off[a]) > 1e-9 {
				t.Fatalf("tick %d: anchor %v slid on the platform (box %v, want offset %v)", c.tick, b.Hook.At, box, off)
			}
		}
	}
	if d := b.Hook.At[0] - x0; math.Abs(d-0.2) > 1e-6 {
		t.Fatalf("the anchor moved %v along x in 0.2 s, want the platform's 0.2", d)
	}
}

// hookFlipOnto fires at the platform, pulls in and flips onto its face; it
// fails the test if that doesn't happen within 2 s.
func hookFlipOnto(t *testing.T, c *clock, b *Body) {
	t.Helper()
	c.step(b, Input{Hook: true, HookPress: true})
	if !b.Hook.On || b.Hook.Mover != 1 {
		t.Fatalf("not latched: %+v", b.Hook)
	}
	for range 200 {
		ev := c.step(b, Input{Hook: true}).Hook
		if ev == HookFlipped {
			return
		}
		if ev == HookReleased {
			t.Fatalf("tick %d: let go instead of flipping onto the face (hook %+v)", c.tick, b.Hook)
		}
	}
	t.Fatalf("never flipped: pos %v hook %+v", b.Pos, b.Hook)
}

// A platform face moving along its normal (a sled's front sliding in and
// out) is stood on like any face: hooked, the body flips onto it and stays on
// it, carried out and back with it (standing still: the same place on it;
// a rotated frame rides movers as the world's does).
func TestHookMoverFaceAlongItsNormalFlips(t *testing.T) {
	c, b := moverHookWorld(Vec3{0, 0, 2}, 4) // +z face at z -5, out to -3 and back every 4 s
	hookFlipOnto(t, c, b)
	if b.Grav != GravNZ || !b.OnGround {
		t.Fatalf("grav %d onGround %v, want standing on the +z face", b.Grav, b.OnGround)
	}
	for range 50 { // the speed along the face it flipped with runs out
		c.step(b, Input{})
	}
	off := Vec3{b.Pos[0], b.Pos[1], 0}
	minZ, maxZ := 1e9, -1e9
	for range 400 { // a whole period: out and back
		c.step(b, Input{})
		box := MoverBoxes(c.w.Movers, float64(c.tick+1)/100)[0]
		if !b.OnGround || b.Grav != GravNZ || math.Abs(b.Pos[2]-box.Max[2]) > 1e-9 || math.Abs(b.Pos[0]-off[0]) > 1e-9 || math.Abs(b.Pos[1]-off[1]) > 1e-9 {
			t.Fatalf("tick %d: pos %v ground %v grav %d, want standing on the face at z %v", c.tick, b.Pos, b.OnGround, b.Grav, box.Max[2])
		}
		minZ, maxZ = math.Min(minZ, b.Pos[2]), math.Max(maxZ, b.Pos[2])
	}
	if maxZ-minZ < 1.9 {
		t.Fatalf("carried %v..%v along z, want the platform's 2 m", minZ, maxZ)
	}
	// Walking on it: along the face (x), still on it.
	b.Yaw = -math.Pi / 2 // in its frame
	x0 := b.Pos[0]
	for range 15 { // the platform is 2 m wide: not off its edge
		c.step(b, Input{F: 1})
	}
	box := MoverBoxes(c.w.Movers, float64(c.tick+1)/100)[0]
	if math.Abs(b.Pos[0]-x0) < 0.3 || !b.OnGround || math.Abs(b.Pos[2]-box.Max[2]) > 1e-9 {
		t.Fatalf("walking on the face: pos %v (from x %v), on %v", b.Pos, x0, box)
	}
}

// Standing on a mover face that slides within its own plane, the body is
// carried with it (as on a sled's top in the world's gravity).
func TestRotatedBodyCarriedAlongTheFace(t *testing.T) {
	c, b := moverHookWorld(Vec3{4, 0, 0}, 8)
	hookFlipOnto(t, c, b)
	for range 50 { // the speed along the face it flipped with runs out
		c.step(b, Input{})
	}
	box := MoverBoxes(c.w.Movers, float64(c.tick+1)/100)[0]
	rel := b.Pos[0] - box.Min[0]
	for range 150 {
		c.step(b, Input{})
	}
	box = MoverBoxes(c.w.Movers, float64(c.tick+1)/100)[0]
	if math.Abs(b.Pos[0]-box.Min[0]-rel) > 1e-6 || box.Min[0] < -0.5 || b.Grav != GravNZ || !b.OnGround {
		t.Fatalf("pos %v, platform %v: want carried with it (x %v from its edge)", b.Pos, box, rel)
	}
}

// An escalator's tread is hooked too, and the anchor rides the tread.
func TestHookOnAnEscalatorTread(t *testing.T) {
	m := content.Map{Boxes: []content.Box{content.B(-20, -1, -20, 20, 0, 20, "")},
		Movers: []content.Mover{{Kind: content.MoverEscalator, Min: [3]float64{-1, 0, -6}, Max: [3]float64{1, 3, -2}, Axis: 2, Up: -1, Rise: 0.25, Run: 0.4, Speed: 0.5, Flat: 1}}}
	w := NewWorld(&m)
	w.Move.Hook = DefaultHook()
	c := &clock{w: w}
	b := NewBody(Vec3{0, 0, 2})
	b.OnGround, b.Boots, b.Energy = true, true, 100
	b.Yaw, b.Pitch = 0, -0.2
	if ev := c.step(b, Input{Hook: true, HookPress: true}).Hook; ev != HookFired || b.Hook.Mover != 1 || b.Hook.U != math.Floor(b.Hook.U) {
		t.Fatalf("fire: ev %v hook %+v, want latched on a tread", ev, b.Hook)
	}
	at := b.Hook.At
	c.step(b, Input{Hook: true})
	if !b.Hook.On || b.Hook.At == at || math.Abs(b.Hook.At[2]-at[2]+0.005) > 1e-9 {
		t.Fatalf("the anchor %v -> %v, want carried 0.005 m along -z with the tread", at, b.Hook.At)
	}
}

// A body standing on a wall (a gravity of its own) is shoved by a mover
// sliding along the wall into it, as a mover shoves a body on the floor: it
// never ends a step inside the mover, and stays on its wall.
func TestRotatedBodyShovedByAMover(t *testing.T) {
	m := boxRoom()
	m.Movers = []content.Mover{{Kind: content.MoverSlide, Min: [3]float64{-4, 4, -10}, Max: [3]float64{-2, 6, -8.5}, Move: [3]float64{6, 0, 0}, Period: 4}}
	w := NewWorld(&m)
	c := &clock{w: w}
	b := NewBody(Vec3{0, 5, -10})
	b.Grav, b.OnGround = GravNZ, true
	for range 150 {
		c.step(b, Input{})
		box := MoverBoxes(w.Movers, float64(c.tick+1)/100)[0]
		var hull content.Box
		HullBox(b.Pos, b.H, b.Grav, &hull)
		if hull.Max[0] > box.Min[0]+1e-6 && hull.Min[0] < box.Max[0]-1e-6 && hull.Max[1] > box.Min[1]+1e-6 && hull.Min[1] < box.Max[1]-1e-6 && hull.Max[2] > box.Min[2]+1e-6 && hull.Min[2] < box.Max[2]-1e-6 {
			t.Fatalf("tick %d: hull %v inside the mover %v", c.tick, hull, box)
		}
		if b.Grav != GravNZ || !b.OnGround || b.Pos[2] != -10 {
			t.Fatalf("tick %d: pos %v grav %d ground %v, want on the wall", c.tick, b.Pos, b.Grav, b.OnGround)
		}
	}
	if b.Pos[0] < 1.9 {
		t.Fatalf("pos %v: want shoved along +x ahead of the mover", b.Pos)
	}
}
