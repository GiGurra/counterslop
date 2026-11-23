package sim

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

func flat() *World {
	m := content.Map{Boxes: []content.Box{{Min: [3]float64{-50, -1, -50}, Max: [3]float64{50, 0, 50}, Kind: "concrete"}}}
	return NewWorld(&m)
}

func settle(w *World, b *Body, in Input, ticks int) {
	for range ticks {
		w.Step(b, in, 0.01)
	}
}

// Walking into a standing player stops at his hull (R + R apart).
func TestPeerBlocksSideways(t *testing.T) {
	w := flat()
	w.Peers = []Peer{{Pos: Vec3{0, 0, -3}, H: H}}
	b := NewBody(Vec3{0, 0, 0})
	b.OnGround = true
	settle(w, b, Input{F: 1}, 100) // yaw 0 walks -z
	if math.Abs(b.Pos[2]-(-3+2*R)) > 1e-9 {
		t.Fatalf("stopped at z=%v, want %v", b.Pos[2], -3+2*R)
	}
	if !w.Touching(b, 1e-6) {
		t.Fatal("not touching the peer")
	}
	w.Peers = nil
	settle(w, b, Input{F: 1}, 50)
	if b.Pos[2] > -3.5 {
		t.Fatalf("not solid any more, but still blocked at %v", b.Pos[2])
	}
}

// A crouched player is too tall to step onto (1.2 m > a step).
func TestPeerNoStepUp(t *testing.T) {
	w := flat()
	w.Peers = []Peer{{Pos: Vec3{0, 0, -2}, H: HCrouch}}
	b := NewBody(Vec3{0, 0, 0})
	b.OnGround = true
	settle(w, b, Input{F: 1}, 100)
	if b.Pos[1] != 0 || math.Abs(b.Pos[2]-(-2+2*R)) > 1e-9 {
		t.Fatalf("at %v: stepped onto or through a crouched player", b.Pos)
	}
}

// Dropped on a head, the body stands there; when the head goes, it falls (no carrying).
func TestStandOnHead(t *testing.T) {
	w := flat()
	w.Peers = []Peer{{Pos: Vec3{0, 0, 0}, H: HCrouch}}
	b := NewBody(Vec3{0.1, 2, 0})
	settle(w, b, Input{}, 100)
	if b.Pos[1] != HCrouch || !b.OnGround {
		t.Fatalf("at %v ground=%v, want standing on the head at %v", b.Pos, b.OnGround, HCrouch)
	}
	w.Peers[0].Pos[0] = 3 // the head walks away (and doesn't carry the body along)
	settle(w, b, Input{}, 100)
	if b.Pos[1] != 0 || !b.OnGround {
		t.Fatalf("at %v: did not fall off when the head left", b.Pos)
	}
}

// A body overlapping a peer at the start (a spawn on top of someone) is never
// stuck: both ignore each other until apart, then collide again.
func TestPeerOverlapEscape(t *testing.T) {
	w := flat()
	w.Peers = []Peer{{Pos: Vec3{0.2, 0, 0}, H: H}}
	b := NewBody(Vec3{0, 0, 0})
	b.OnGround = true
	settle(w, b, Input{R: 1}, 60) // strafe right (+x) straight through the peer
	if b.Pos[0] < 1.5 {
		t.Fatalf("stuck in the peer at %v", b.Pos)
	}
	settle(w, b, Input{R: -1}, 100) // back: now it blocks
	if math.Abs(b.Pos[0]-(0.2+2*R)) > 1e-9 {
		t.Fatalf("separated, walked back to %v: want blocked at %v", b.Pos[0], 0.2+2*R)
	}
}

// Jumping up into someone standing over you stops at his feet.
func TestPeerHeadBump(t *testing.T) {
	w := flat()
	w.Peers = []Peer{{Pos: Vec3{0, 2.2, 0}, H: H}}
	b := NewBody(Vec3{0, 0, 0})
	b.OnGround = true
	w.Step(b, Input{Jump: true}, 0.01)
	settle(w, b, Input{}, 20)
	if b.Pos[1]+b.H > 2.2+1e-9 {
		t.Fatalf("head at %v went into the feet above at 2.2", b.Pos[1]+b.H)
	}
}

// Uncrouching under someone standing on you is blocked.
func TestPeerBlocksUncrouch(t *testing.T) {
	w := flat()
	w.Peers = []Peer{{Pos: Vec3{0, HCrouch, 0}, H: H}}
	b := NewBody(Vec3{0, 0, 0})
	b.OnGround, b.Crouched, b.H = true, true, HCrouch
	settle(w, b, Input{Crouch: false}, 10)
	if !b.Crouched {
		t.Fatal("stood up into the player on its head")
	}
}

// RoundMM is the snapshot's math.Round(v*1000)/1000, and idempotent: a hull
// from a snapshot position is the server's hull, bit for bit.
func TestRoundMM(t *testing.T) {
	for _, v := range []float64{0, 1.0005, -1.0005, 2.3456789, -2.3456789, 0.0004999, -0.0005, 123.4565, 1e6 + 0.0015, -7.25e-4} {
		want := math.Round(v*1000) / 1000
		if got := RoundMM(v); got != want {
			t.Errorf("RoundMM(%v) = %v, want %v", v, got, want)
		}
		if RoundMM(want) != want {
			t.Errorf("RoundMM not idempotent at %v", want)
		}
	}
	r := rand.New(rand.NewPCG(3, 4))
	for range 200000 {
		v := (r.Float64() - 0.5) * 200
		want := math.Round(v*1000) / 1000
		if got := RoundMM(v); got != want || RoundMM(want) != want {
			t.Fatalf("RoundMM(%v) = %v, want %v (idempotent: %v)", v, got, want, RoundMM(want))
		}
	}
}

// Two bodies on exactly one spot (spawned there), both idle, stepped in turn
// against each other as the server does: pushed apart, opposite ways, within a
// second, and solid to each other afterwards.
func TestPeerPushApart(t *testing.T) {
	for _, off := range []float64{0, 0.3} {
		w := flat()
		a, b := NewBody(Vec3{1, 0, 1}), NewBody(Vec3{1 + off, 0, 1 + off/2})
		a.OnGround, b.OnGround = true, true
		ticks := 0
		for ; ticks < 100; ticks++ {
			w.Peers = []Peer{{Pos: b.Pos, H: b.H, Lo: false}} // a has ID 1, b ID 2
			w.Step(a, Input{}, 0.01)
			w.Peers = []Peer{{Pos: a.Pos, H: a.H, Lo: true}}
			w.Step(b, Input{}, 0.01)
			dx, dz := a.Pos[0]-b.Pos[0], a.Pos[2]-b.Pos[2]
			if math.Abs(dx) >= 2*R || math.Abs(dz) >= 2*R {
				break
			}
		}
		if ticks >= 100 {
			t.Fatalf("offset %v: still overlapping after 1 s: %v %v", off, a.Pos, b.Pos)
		}
		if a.Pos[0] >= b.Pos[0] || a.Vel != (Vec3{}) {
			t.Fatalf("offset %v: a %v vel %v, b %v: want a pushed to -x, velocity untouched", off, a.Pos, a.Vel, b.Pos)
		}
		w.Peers = []Peer{{Pos: b.Pos, H: b.H}}
		settle(w, a, Input{R: 1}, 50)              // walks back into b: blocked now
		if a.Pos[0] > RoundMM(b.Pos[0])-2*R+1e-9 { // the hull is at the rounded position
			t.Fatalf("offset %v: walked back into b: a %v b %v", off, a.Pos, b.Pos)
		}
	}
}

// Pushed out against a wall, the body stays out of the wall.
func TestPeerPushNotIntoWorld(t *testing.T) {
	m := content.Map{Boxes: []content.Box{
		{Min: [3]float64{-50, -1, -50}, Max: [3]float64{50, 0, 50}, Kind: "concrete"},
		{Min: [3]float64{-5, 0, -5}, Max: [3]float64{0, 3, 5}, Kind: "concrete"}, // wall at x<0
	}}
	w := NewWorld(&m)
	a := NewBody(Vec3{R, 0, 0})
	a.OnGround = true
	w.Peers = []Peer{{Pos: Vec3{R + 0.2, 0, 0}, H: H}}
	settle(w, a, Input{}, 100)
	if a.Pos[0] < R-1e-9 {
		t.Fatalf("pushed into the wall: %v", a.Pos)
	}
}

// boostWorld: flat ground and a 3 m crate (de_slop's at [-26..-23]^2): one
// crate-high, out of reach of a jump from the floor or from a crouched head.
func boostWorld() *World {
	m := content.Map{Boxes: []content.Box{
		{Min: [3]float64{-50, -1, -50}, Max: [3]float64{50, 0, 50}, Kind: "concrete"},
		{Min: [3]float64{-26, 0, -26}, Max: [3]float64{-23, 3, -23}, Kind: "crate"},
	}}
	return NewWorld(&m)
}

// boostTick steps both bodies one 10 ms command each, as the server does (a
// first, b ID 2 second), each colliding with the other where it is now.
func boostTick(w *World, a, b *Body, ia, ib Input) {
	w.Peers = []Peer{{Pos: b.Pos, H: b.H}}
	w.Step(a, ia, 0.01)
	w.Peers = []Peer{{Pos: a.Pos, H: a.H, Lo: true}}
	w.Step(b, ib, 0.01)
}

// boostSetup: a crouched next to the crate, b standing on her head.
func boostSetup(t *testing.T) (*World, *Body, *Body) {
	w := boostWorld()
	a := NewBody(Vec3{-26.6, 0, -24.5})
	b := NewBody(Vec3{-26.6, 2.5, -24.5})
	for range 60 {
		boostTick(w, a, b, Input{Crouch: true}, Input{})
	}
	if !a.Crouched || b.Pos[1] != HCrouch || !b.OnGround {
		t.Fatalf("setup: a crouched=%v, b at %v ground=%v", a.Crouched, b.Pos, b.OnGround)
	}
	return w, a, b
}

// riderClimb: b jumps, then holds crouch (a crouch-jump) and strafes onto the
// crate; ticks later, where b ended up.
func riderClimb(w *World, a, b *Body, ia Input) {
	boostTick(w, a, b, ia, Input{Jump: true})
	for range 100 {
		boostTick(w, a, b, ia, Input{R: 1, Crouch: true})
	}
}

// The CS boost (GoldSrc's PM_UnDuck only stands up where the standing hull
// fits): the crouched player can't stand while the rider is on her head, so the
// rider jumps, she releases crouch and stands in the gap, he lands on the 1.8 m
// head and crouch-jumps from there onto a ledge he can't reach otherwise.
func TestBoost(t *testing.T) {
	// From the crouched head the crate is out of reach.
	w, a, b := boostSetup(t)
	riderClimb(w, a, b, Input{Crouch: true})
	if b.Pos[1] > HCrouch+1e-9 {
		t.Fatalf("crouch-jump from a crouched head reached y=%v (the crate is 3 m)", b.Pos[1])
	}

	// The boost.
	w, a, b = boostSetup(t)
	boostTick(w, a, b, Input{}, Input{Jump: true}) // he jumps, she releases: blocked by his feet
	if !a.Crouched {
		t.Fatal("stood up into the rider's feet")
	}
	stood := 0
	for i := 1; i < 100 && !(b.OnGround && i > 5); i++ {
		boostTick(w, a, b, Input{}, Input{})
		if !a.Crouched && stood == 0 {
			stood = i
			if b.Pos[1] < H {
				t.Fatalf("stood up with the rider's feet at %v, under her head", b.Pos[1])
			}
		}
	}
	if stood == 0 || a.Pos[1] != 0 {
		t.Fatalf("never stood up (a at %v crouched=%v)", a.Pos, a.Crouched)
	}
	if b.Pos[1] != H || !b.OnGround {
		t.Fatalf("rider at %v ground=%v, want on the standing head at %v", b.Pos, b.OnGround, H)
	}
	riderClimb(w, a, b, Input{})
	if b.Pos[1] != 3 || !b.OnGround || b.Pos[0] < -26+R {
		t.Fatalf("rider at %v ground=%v, want on the crate (y=3)", b.Pos, b.OnGround)
	}
	if a.Pos[1] != 0 || a.Crouched {
		t.Fatalf("booster at %v crouched=%v", a.Pos, a.Crouched)
	}
}
