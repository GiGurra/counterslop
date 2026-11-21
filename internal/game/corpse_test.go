package game

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// A death stamps the corpse: the tick it fell on and which way (away from the
// killer, else backwards), in the snapshot; a respawn clears it.
func TestDeathStampsTheCorpse(t *testing.T) {
	s, v, a := roundServer(t)
	now := time.Now()
	ticks(s, 2, &now, nil)
	v.Pos, a.Pos = Vec3{0, 0, 0}, Vec3{5, 0, 0} // the Artisan east of the killer
	s.killBy(v, a, "prompt", false)
	if a.DeathTick != s.tick {
		t.Fatalf("death tick %d, want %d", a.DeathTick, s.tick)
	}
	// Falls east (+x): forward of yaw y is (-sin y, -cos y).
	if fx, fz := -math.Sin(a.FallYaw), -math.Cos(a.FallYaw); math.Abs(fx-1) > 1e-9 || math.Abs(fz) > 1e-9 {
		t.Fatalf("falls toward (%.2f, %.2f), want away from the killer (1, 0)", fx, fz)
	}
	if a.Vanish {
		t.Fatal("a live round's corpse vanishes")
	}
	js, _ := json.Marshal(a)
	var m map[string]any
	_ = json.Unmarshal(js, &m)
	if m["dk"] != float64(s.tick) || m["fy"] == nil {
		t.Fatalf("snapshot %s: want dk and fy", js)
	}
	// No killer (the world) or a suicide: backwards.
	b := &Player{Yaw: 0.5}
	if math.Abs(fallYaw(nil, b)-(0.5+math.Pi)) > 1e-9 || fallYaw(b, b) != fallYaw(nil, b) {
		t.Fatalf("no killer falls at %.2f, want backwards %.2f", fallYaw(nil, b), 0.5+math.Pi)
	}
	// The next round: up again, no corpse.
	ticks(s, 1, &now, nil) // the round ends
	now = now.Add(overSecs * time.Second)
	ticks(s, 2, &now, nil)
	if a.Dead || a.DeathTick != 0 {
		t.Fatalf("after the respawn: dead %v death tick %d", a.Dead, a.DeathTick)
	}
}

// In warmup (and with timed respawns) the body goes at once.
func TestWarmupCorpseVanishes(t *testing.T) {
	s, v, a := roundServer(t)
	s.round.Phase = PhaseWarmup
	s.killBy(v, a, "prompt", false)
	if !a.Vanish {
		t.Fatal("a warmup corpse stays")
	}
}
