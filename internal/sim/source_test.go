package sim

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// floor1km is a 1 km floor at y = 0.
func floor1km() *World {
	m := content.Map{Boxes: []content.Box{{Min: [3]float64{-500, -1, -500}, Max: [3]float64{500, 0, 500}, Kind: "sand"}}}
	return NewWorld(&m)
}

func hspeed(b *Body) float64 { return math.Hypot(b.Vel[0], b.Vel[2]) }

// strafeYaw is the yaw that, with only R = dir held, wishes along the body's
// velocity turned by just under 90 degrees toward dir: the speed along the
// wish direction is half airCap, so every air tick adds speed.
func strafeYaw(b *Body, dir float64) float64 {
	sp := hspeed(b)
	vx, vz := b.Vel[0]/sp, b.Vel[2]/sp
	a := math.Acos(math.Min(1, airCap/2/sp)) * dir
	// wish = v rotated by -a (right of the velocity for dir > 0, looking down y)
	wx := vx*math.Cos(a) + vz*math.Sin(a)
	wz := -vx*math.Sin(a) + vz*math.Cos(a)
	// R = dir: wish = dir * (cos yaw, -sin yaw)
	return math.Atan2(-wz*dir, wx*dir)
}

func runFwd(w *World, b *Body, ticks int, in Input) {
	for range ticks {
		w.Step(b, in, 0.01)
	}
}

func TestSourceGroundSpeedAndFriction(t *testing.T) {
	w := floor1km()
	w.Move = MoveRules{Model: MoveSource}
	b := NewBody(Vec3{0, 0, 0})
	runFwd(w, b, 5, Input{})
	runFwd(w, b, 200, Input{F: 1})
	if sp := hspeed(b); math.Abs(sp-Run) > 1e-6 {
		t.Fatalf("ground run speed %.6f, want %v", sp, Run)
	}
	// Source accelerates more slowly than classic: not at full speed in 0.1 s.
	b2 := NewBody(Vec3{0, 0, 0})
	runFwd(w, b2, 5, Input{})
	runFwd(w, b2, 10, Input{F: 1})
	if sp := hspeed(b2); sp > Run*0.6 || sp < Run*0.3 {
		t.Fatalf("after 0.1 s: %.3f m/s, want 30-60%% of run", sp)
	}
	// Let go: friction stops the body within half a second.
	runFwd(w, b, 50, Input{})
	if sp := hspeed(b); sp != 0 {
		t.Fatalf("still sliding at %.4f m/s after letting go", sp)
	}
}

// Air strafing: holding a strafe key while turning with the velocity gains
// speed in the source model, not in classic.
func TestSourceAirStrafeGainsSpeed(t *testing.T) {
	for _, model := range []int{MoveClassic, MoveSource} {
		w := floor1km()
		w.Move = MoveRules{Model: model}
		b := NewBody(Vec3{0, 0, 0})
		runFwd(w, b, 5, Input{})
		runFwd(w, b, 200, Input{F: 1})
		w.Step(b, Input{F: 1, Jump: true}, 0.01)
		start := hspeed(b)
		for !b.OnGround {
			b.Yaw = strafeYaw(b, 1)
			w.Step(b, Input{R: 1}, 0.01)
		}
		gain := hspeed(b) - start
		if model == MoveSource && gain < 1 {
			t.Fatalf("source: one strafed jump gained %.3f m/s, want > 1", gain)
		}
		if model == MoveClassic && hspeed(b) > Run+1e-9 {
			t.Fatalf("classic: %.3f m/s after a strafed jump, more than running", hspeed(b))
		}
	}
}

// A bunny-hop chain: auto bhop (holding jump) plus air strafing gets faster
// with every hop, as the jump on the landing tick skips friction; the same
// strafing without hopping (friction on the ground) does not.
func TestSourceBhopChain(t *testing.T) {
	w := floor1km()
	w.Move = MoveRules{Model: MoveSource, AutoBhop: true}
	b := NewBody(Vec3{0, 0, 0})
	runFwd(w, b, 5, Input{})
	runFwd(w, b, 200, Input{F: 1})
	var hops []float64
	dir := 1.0
	for tick := 0; tick < 1200 && len(hops) < 8; tick++ {
		was := b.OnGround
		if !b.OnGround {
			b.Yaw = strafeYaw(b, dir)
		}
		ev := w.Step(b, Input{R: dir, JumpHeld: true, Jump: tick == 0}, 0.01)
		if ev.Jumped {
			if !was {
				t.Fatalf("jumped while airborne")
			}
			hops = append(hops, hspeed(b))
			dir = -dir // strafe the other way each hop
		}
	}
	if len(hops) < 8 {
		t.Fatalf("only %d hops in 12 s", len(hops))
	}
	for i := 1; i < len(hops); i++ {
		if hops[i] <= hops[i-1] {
			t.Fatalf("hop %d slower than hop %d: %v", i, i-1, hops)
		}
	}
	t.Logf("hop speeds: %.2f", hops)
	if hops[len(hops)-1] < Run*1.5 {
		t.Fatalf("8 hops reach %.2f m/s, want > 1.5x run: %v", hops[len(hops)-1], hops)
	}
	// Without auto bhop, holding jump jumps once.
	w.Move.AutoBhop = false
	b = NewBody(Vec3{0, 0, 0})
	runFwd(w, b, 5, Input{})
	jumps := 0
	for tick := range 300 {
		if w.Step(b, Input{F: 1, JumpHeld: true, Jump: tick == 0}, 0.01).Jumped {
			jumps++
		}
	}
	if jumps != 1 {
		t.Fatalf("held jump without auto bhop: %d jumps, want 1", jumps)
	}
}

// sv_airaccelerate scales how fast air strafing turns speed into gain.
func TestSourceAirAccelerate(t *testing.T) {
	gain := func(aa float64) float64 {
		w := floor1km()
		w.Move = MoveRules{Model: MoveSource, AirAccel: aa}
		b := NewBody(Vec3{0, 0, 0})
		runFwd(w, b, 5, Input{})
		runFwd(w, b, 200, Input{F: 1})
		w.Step(b, Input{F: 1, Jump: true}, 0.01)
		start := hspeed(b)
		for range 20 {
			b.Yaw = strafeYaw(b, 1)
			w.Step(b, Input{R: 1}, 0.01)
		}
		return hspeed(b) - start
	}
	if g0, g12 := gain(0), gain(DefaultAirAccel); g0 != g12 {
		t.Fatalf("airAccel 0 gained %v, default %v: 0 must mean the default", g0, g12)
	}
	if g1, g100 := gain(1), gain(100); !(g100 > g1) {
		t.Fatalf("airAccel 100 gained %v, 1 gained %v", g100, g1)
	}
}
