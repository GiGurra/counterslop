package game

import (
	"github.com/gigurra/counterslop/internal/content/maps"
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

const botDt = 1.0 / SimHz

func TestBotWalksToItsTarget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		crouch bool
		speed  float64
	}{
		{"standing", false, botSpeed},
		{"crouched", true, botSpeed / 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer()
			b := s.addBot()
			b.Pos = Vec3{-14, 0, 6}
			b.Crouch = tc.crouch
			b.brain.holdCrouch = tc.crouch
			b.brain.crouchLeft = 0
			b.brain.target = Vec3{-14 + 3, 0, 6 + 4} // 5 m away along (+3, +4)
			s.tickBots(botDt)
			step := tc.speed * botDt
			want := Vec3{-14 + 0.6*step, 0, 6 + 0.8*step}
			for i := range 3 {
				if math.Abs(b.Pos[i]-want[i]) > 1e-12 {
					t.Fatalf("pos %v, want %v", b.Pos, want)
				}
			}
			if wantYaw := math.Atan2(-3, -4); math.Abs(b.Yaw-wantYaw) > 1e-9 {
				t.Fatalf("yaw %v, want %v (facing the target)", b.Yaw, wantYaw)
			}
		})
	}
}

func TestBotIdleFrozenAndHop(t *testing.T) {
	s := NewServer()
	b := s.addBot()
	b.Pos = Vec3{-14, 0, 6}
	b.brain.target = Vec3{-14, 0, 0}

	b.brain.frozen = true
	s.tickBots(botDt)
	if b.Pos != (Vec3{-14, 0, 6}) {
		t.Fatal("frozen bot moved")
	}
	b.brain.frozen = false

	b.brain.idle = 0.05
	for range 4 {
		s.tickBots(botDt)
	}
	if b.Pos != (Vec3{-14, 0, 6}) || b.brain.idle <= 0 {
		t.Fatalf("idle bot moved to %v (idle %v)", b.Pos, b.brain.idle)
	}

	// A hop: goes up, comes back down to the floor and stops there.
	b.brain.idle, b.brain.vy = 10, botJump
	peak := 0.0
	for range 200 {
		s.tickBots(botDt)
		peak = math.Max(peak, b.Pos[1])
	}
	wantPeak := botJump * botJump / (2 * botGrav)
	if math.Abs(peak-wantPeak) > 0.05 || b.Pos[1] != 0 || b.brain.vy != 0 {
		t.Fatalf("hop peak %.3f (want %.3f), y %v vy %v", peak, wantPeak, b.Pos[1], b.brain.vy)
	}

	// A timed crouch ends unless bot_crouch holds it.
	b.Crouch, b.brain.crouchLeft = true, 0.02
	for range 3 {
		s.tickBots(botDt)
	}
	if b.Crouch {
		t.Fatal("timed crouch didn't end")
	}
	b.Crouch, b.brain.crouchLeft, b.brain.holdCrouch = true, 0.02, true
	for range 3 {
		s.tickBots(botDt)
	}
	if !b.Crouch {
		t.Fatal("held crouch ended")
	}
}

// Bots wander forever without walking through walls or leaving the map, at
// walking speed, and never touch real players.
func TestBotsWanderInsideTheMap(t *testing.T) {
	s := NewServer()
	s.Seed(9) // a bot starts under de_slop's mid overhang (a box from y 3 up): not inside it
	p := s.join("1.1.1.1")
	p.Team = content.Vibe
	pPos := p.Pos
	var bots []*Player
	for range 4 {
		bots = append(bots, s.addBot())
	}
	inBox := func(v Vec3) bool {
		for _, bx := range s.Map.Boxes {
			if bx.Max[1] <= 0 {
				continue
			}
			if v[0] > bx.Min[0] && v[0] < bx.Max[0] && v[2] > bx.Min[2] && v[2] < bx.Max[2] && v[1] < bx.Max[1] && v[1]+1.8 > bx.Min[1] { // 1.8: a standing player's height
				return true
			}
		}
		return false
	}
	moved := map[int]bool{}
	for range 3000 {
		prev := make([]Vec3, len(bots))
		for i, b := range bots {
			prev[i] = b.Pos
		}
		s.tickBots(botDt)
		for i, b := range bots {
			if d := math.Hypot(b.Pos[0]-prev[i][0], b.Pos[2]-prev[i][2]); d > botSpeed*botDt+1e-9 {
				t.Fatalf("bot %d moved %.4f m in one tick", b.ID, d)
			} else if d > 0 {
				moved[b.ID] = true
			}
			if math.Abs(b.Pos[0]) > 32 || math.Abs(b.Pos[2]) > 32 || b.Pos[1] < 0 || inBox(b.Pos) {
				t.Fatalf("bot %d at %v: outside the map or inside a box", b.ID, b.Pos)
			}
		}
	}
	if len(moved) != len(bots) {
		t.Fatalf("only %d of %d bots ever moved", len(moved), len(bots))
	}
	if p.Pos != pPos {
		t.Fatal("tickBots moved a real player")
	}
	if n := s.kickBots(); n != 4 || len(s.players) != 1 {
		t.Fatalf("kicked %d, %d left", n, len(s.players))
	}
}

func TestSegmentClear(t *testing.T) {
	s := NewServer()
	for _, tc := range []struct {
		name string
		a, b Vec3
		r    float64
		ok   bool
	}{
		{"open floor", Vec3{-14, 0, 6}, Vec3{-14, 0, 0}, 0.5, true},
		{"into the mid building", Vec3{-10, 0, 0}, Vec3{10, 0, 0}, 0.5, false},
		// The doorway's lintel is above head height: walkable.
		{"through the doorway", Vec3{0, 0, -10}, Vec3{0, 0, 10}, 0.5, true},
		{"into a crate", Vec3{9, 0, 6}, Vec3{9, 0, 14}, 0.5, false},
		{"grazing a crate", Vec3{7.2, 0, 6}, Vec3{7.2, 0, 14}, 0.5, true},
		{"grazing a crate with a wider disc", Vec3{7.2, 0, 6}, Vec3{7.2, 0, 14}, 1, false},
		{"standing still", Vec3{-14, 0, 6}, Vec3{-14, 0, 6}, 0.5, true},
		{"out through the wall", Vec3{0, 0, 25}, Vec3{0, 0, 40}, 0.5, false},
	} {
		if got := s.segmentClear(tc.a, tc.b, tc.r); got != tc.ok {
			t.Errorf("%s: clear=%v, want %v", tc.name, got, tc.ok)
		}
	}
}

// Floors over floors (de_stack): the deck above the basement isn't in the way
// down there (bots used to pick nav nodes on the wrong floor and stand under
// their goal all round), the basement's walls still are.
func TestSegmentClearUnderADeck(t *testing.T) {
	s := NewServer()
	s.Map = maps.All["de_stack"]()
	if !s.segmentClear(Vec3{3, 0, -16}, Vec3{20, 0, -17}, 0.3) {
		t.Error("the basement's walk to B is blocked by the deck above it")
	}
	if s.segmentClear(Vec3{3, 0, -16}, Vec3{3, 0, -40}, 0.3) {
		t.Error("walked out through the basement wall")
	}
}

func TestRandomFloorPoint(t *testing.T) {
	s := NewServer()
	from := Vec3{-14, 0, 6}
	for range 200 {
		pt, ok := s.randomFloorPoint(from)
		d := math.Hypot(pt[0]-from[0], pt[2]-from[2])
		if !ok || pt[1] != 0 || d < 3-1e-9 || d > 12+1e-9 || !s.segmentClear(from, pt, 0.5) {
			t.Fatalf("point %v (ok %v, %.2f m away)", pt, ok, d)
		}
	}
	// Boxed in on every side: no point is reachable.
	s.Map.Boxes = append([]content.Box{},
		content.B(-1, 0, -1, 1, 1, -0.6, "brick"), content.B(-1, 0, 0.6, 1, 1, 1, "brick"),
		content.B(-1, 0, -1, -0.6, 1, 1, "brick"), content.B(0.6, 0, -1, 1, 1, 1, "brick"))
	if pt, ok := s.randomFloorPoint(Vec3{}); ok || pt != (Vec3{}) {
		t.Fatalf("boxed in: %v %v", pt, ok)
	}
	// One gap in the box: every point found leaves through it (+x side).
	s.Map.Boxes = s.Map.Boxes[:3]
	for range 50 {
		if pt, ok := s.randomFloorPoint(Vec3{}); ok && pt[0] < 0.6 {
			t.Fatalf("point %v not through the gap", pt)
		}
	}
}
