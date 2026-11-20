package game

import (
	"math"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

func TestHistoryAtInterpolatesLikeTheClient(t *testing.T) {
	var h History
	h.Record(3, map[int]PoseState{1: {Pos: Vec3{0, 0, 0}, Yaw: 3, Pitch: 0}})
	h.Record(6, map[int]PoseState{1: {Pos: Vec3{3, 0, 6}, Yaw: -3, Pitch: 0.3, Crouch: true}, 2: {Pos: Vec3{9, 9, 9}}})
	got := h.At(4)[1]
	if math.Abs(got.Pos[0]-1) > 1e-9 || math.Abs(got.Pos[2]-2) > 1e-9 || math.Abs(got.Pitch-0.1) > 1e-9 {
		t.Fatalf("At(4) = %+v", got)
	}
	// 3 → -3 the short way round: through π, not through 0.
	if want := 3 + (2*math.Pi-6)/3; math.Abs(got.Yaw-want) > 1e-9 {
		t.Fatalf("yaw %.4f, want %.4f", got.Yaw, want)
	}
	if !got.Crouch { // crouch comes from the later snapshot, as in net.js
		t.Fatal("crouch should come from the later snapshot")
	}
	if p := h.At(4)[2]; p.Pos != (Vec3{9, 9, 9}) {
		t.Fatalf("player only in the later snapshot: %+v", p)
	}
	if p := h.At(1)[1]; p.Pos != (Vec3{}) {
		t.Fatalf("before the history clamps to the oldest: %+v", p)
	}
	if p := h.At(99)[1]; p.Pos != (Vec3{3, 0, 6}) {
		t.Fatalf("after the history clamps to the newest: %+v", p)
	}
	for i := range 100 {
		h.Record(int64(9+3*i), nil)
	}
	if len(h.frames) != histLen {
		t.Fatalf("history kept %d frames, want %d", len(h.frames), histLen)
	}
}

// A target strafes across the shooter's view. The shooter aims where it saw
// the target at an older tick: with that view tick the shot hits, without it
// (or with the rewind capped by sv_maxunlag) it misses.
func TestShotIsTracedAtTheShootersViewTick(t *testing.T) {
	for _, tc := range []struct {
		name     string
		maxUnlag string
		vt       bool
		hit      bool
	}{
		{"rewound", "0.5", true, true},
		{"no view tick", "0.5", false, false},
		{"lag comp off", "0", true, false},
		{"beyond sv_maxunlag", "0.05", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := authServer(t)
			s.cvars["sv_maxunlag"] = tc.maxUnlag
			a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
			a.ready, b.ready = true, true
			a.Team, b.Team = content.Vibe, content.Artisan
			a.Pos, b.Pos = Vec3{-14, 0, 6}, Vec3{-14, 0, 0}
			// b strafes along +X at 7 m/s (as the server sees it after each tick).
			var seenTick int64
			seq := 1
			for i := range 30 {
				s.queueCmds(a, []Cmd{{Seq: seq}})
				seq++
				stepSim(s)
				b.Pos[0] = -14 + float64(i)*0.07
				if s.tick%snapEvery == 0 {
					s.recordHistory()
					if seenTick == 0 && i >= 10 {
						seenTick = s.tick // the snapshot a's screen shows when it fires
					}
				}
			}
			seen := s.history.At(float64(seenTick))[b.ID].Pos
			if b.Pos[0]-seen[0] < 1 {
				t.Fatalf("setup: b only moved %.2f m since the seen tick", b.Pos[0]-seen[0])
			}
			drain(a)
			// Aim at b's chest where a saw it.
			dx, dz := seen[0]-a.Pos[0], seen[2]-a.Pos[2]
			yaw := math.Atan2(-dx, -dz)
			pitch := math.Atan2(0.9-1.62, math.Hypot(dx, dz))
			c := Cmd{Seq: seq, Yaw: yaw, Pitch: pitch, B: BtnFire}
			if tc.vt {
				c.VT = float64(seenTick)
			}
			s.queueCmds(a, []Cmd{c})
			stepSim(s)
			out := drain(a)
			got := strings.Contains(out, `"t":"hit"`)
			if got != tc.hit {
				t.Fatalf("hit=%v, want %v (b moved from %.2f to %.2f)", got, tc.hit, seen[0], b.Pos[0])
			}
			if tc.hit && b.HP >= maxHP {
				t.Fatalf("rewound hit did no damage: hp %d", b.HP)
			}
		})
	}
}
