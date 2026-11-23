package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// In warmup the bots run around and fight (a team deathmatch, respawning)
// on every map, also on the ones without bomb sites (GPU Drop: they stood
// still at spawn the whole warmup).
func TestAIWarmupRoams(t *testing.T) {
	t.Parallel() // a long AI sweep on its own server: runs beside the other sweeps
	for _, name := range maps.Order {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := authServer(t)
			s.Map = maps.All[name]()
			s.Seed(3)
			s.cvars["mp_warmup_time"] = "120"
			s.cvars["bot_difficulty"] = "3"
			for range 2 {
				s.addAI(content.Vibe)
				s.addAI(content.Artisan)
			}
			walked := map[*Player]float64{}
			last := map[*Player]Vec3{}
			for range 30 * 2 { // 30 s in 0.5 s steps
				s.Step(SimHz / 2)
				if s.round.Phase != PhaseWarmup {
					t.Fatalf("phase %q, want warmup", s.round.Phase)
				}
				for _, p := range s.ordered() {
					if q, ok := last[p]; ok && !p.Dead {
						if d := math.Hypot(p.Pos[0]-q[0], p.Pos[2]-q[2]); d < 5 { // not a respawn
							walked[p] += d
						}
					}
					last[p] = p.Pos
				}
			}
			kills := 0
			for _, p := range s.ordered() {
				kills += p.Kills
				if walked[p] < 15 {
					t.Errorf("%s bot %q walked %.1f m in 30 s of warmup", p.Team, p.Name, walked[p])
				}
			}
			t.Logf("%s: %d warmup kills", name, kills)
		})
	}
}
