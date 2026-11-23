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
