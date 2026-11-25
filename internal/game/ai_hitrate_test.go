package game

import (
	"fmt"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// botHitRate: the fraction of a bot's shots (at bot_difficulty diff, weapon
// w) that hit a player standing dist metres away in the open, over trials
// fresh engagements of 3 s each (seeded: the same every run).
func botHitRate(t *testing.T, diff int, w string, dist float64, trials int) float64 {
	t.Helper()
	b := mapgen.New("range")
	b.Add(mapgen.Arena(-40, -40, 40, 40, 5, 1, content.Concrete, content.Brick)...)
	m := b.Map()
	m.Spawns = [][3]float64{{-30, 0, 30}, {30, 0, 30}}
	shots, hits := 0, 0
	for trial := range trials {
		s := authServer(t)
		s.Map = m
		s.Seed(uint64(trial + 1))
		s.cvars["bot_difficulty"] = fmt.Sprint(diff)
		p := s.addAI(content.Vibe)
		q := s.join("10.0.0.2")
		q.ready, q.Team = true, content.Artisan
		s.respawn(q)
		s.Step(1)
		s.round.Phase, s.round.Number = PhaseLive, 1
		p.ai.round, p.ai.site = 1, "A"
		s.teleport(p, Vec3{0, 0, dist / 2}, 0, false, p.lastQueued)
		s.teleport(q, Vec3{0, 0, -dist / 2}, 3.14159, false, q.lastQueued)
		h := s.Weapons.newHeld(w)
		h.Ammo = 100000 // no reloads: every tick on target is a shot
		slot := s.Weapons.Get(w).Slot
		p.inv.Slots[slot], p.inv.Current = h, slot
		for range 3 * SimHz {
			q.HP = 100000 // never dies: count every hit
			ammo, hp := h.Ammo, q.HP
			s.Step(1)
			shots += ammo - h.Ammo
			if q.HP < hp {
				hits++
			}
		}
	}
	return float64(hits) / float64(max(1, shots))
}

// Bots' hit rates at their engagement ranges (strafing up close, standing at
// mid range, crouched past 18 m) stay where the 2026-11 balance pass (#14) left
// them: weapon spread changes move these, so a change that makes bots much
// easier or deadlier fails here (retune aiSkill's aim error instead).
func TestBotHitRateBands(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		diff     int
		w        string
		dist     float64
		min, max float64
	}{
		{1, "prompt", 6, 0.85, 1},   // strafing
		{1, "prompt", 22, 0.3, 0.6}, // aim error dominates at easy
		{1, "stub", 22, 0.85, 1},
		{3, "prompt", 6, 0.75, 1},     // strafing: moving costs (moveMin)
		{3, "prompt", 22, 0.9, 1},     // crouched, settled aim
		{3, "prompt", 32, 0.65, 0.95}, // spread and recoil show at range
		{3, "autocomplete", 32, 0.65, 0.95},
	} {
		if got := botHitRate(t, tc.diff, tc.w, tc.dist, 6); got < tc.min || got > tc.max {
			t.Errorf("bot_difficulty %d, %s at %.0f m: %.0f%% of shots hit, band [%.0f%%, %.0f%%]", tc.diff, tc.w, tc.dist, 100*got, 100*tc.min, 100*tc.max)
		}
	}
}
