package game

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/content/rng"
)

// TestAIDefenseSweep (AI_DEFMIX=1, a few minutes): 5v5 bot matches, 6 seeds x
// 4 minutes per bomb map; prints, per site, how many Artisans set up there
// (1 s into the round: rounds per defender count) and from how many 4 m
// cells its defenders held it 20 s in (those that didn't rotate; the
// busiest cell's count). The first seed keeps the bots' own rngs (a real
// server's). Operator report: de_hackathon's A had no defenders in most rounds.
func TestAIDefenseSweep(t *testing.T) {
	if os.Getenv("AI_DEFMIX") == "" {
		t.Skip("AI_DEFMIX=1 to run")
	}
	for _, name := range maps.Order {
		if len(maps.All[name]().Sites) < 2 {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			perSite := map[string]map[int]int{} // site -> defenders count -> rounds
			spots := map[string]map[string]int{}
			rounds, rotated := 0, 0
			for seed := 1; seed <= 6; seed++ {
				s := authServer(t)
				s.Map = maps.All[name]()
				s.Map.MaxPlayers = max(s.Map.MaxPlayers, 10)
				s.Seed(uint64(seed))
				s.cvars["mp_freezetime"] = "1"
				s.cvars["mp_round_restart_delay"] = "1"
				s.cvars["mp_warmup_time"] = "0"
				s.cvars["bot_difficulty"] = "3"
				s.cvars["mp_maxrounds"] = "1000"
				s.cvars["mp_respawn"] = "0"
				for range 5 {
					s.addAI(content.Vibe)
					s.addAI(content.Artisan)
				}
				if seed > 1 {
					for _, p := range s.ordered() {
						p.ai.r = rng.New(uint32(seed)*2654435761 + uint32(p.ID)*40503 + 1)
					}
				}
				lastSetup, lastSpots := -1, -1
				for range 4 * 60 * SimHz / 10 {
					s.Step(10)
					if s.round.Phase != PhaseLive {
						continue
					}
					live := s.cvarF("mp_roundtime", 115) - s.round.endsAt.Sub(s.now).Seconds()
					if live >= 1 && s.round.Number != lastSetup {
						lastSetup = s.round.Number
						rounds++
						cnt := map[string]int{}
						for _, z := range s.Map.Sites {
							cnt[z.Name] = 0
						}
						for _, p := range s.ordered() {
							if p.Team == content.Artisan && !p.Dead {
								cnt[p.ai.site]++
							}
						}
						for k, v := range cnt {
							if perSite[k] == nil {
								perSite[k] = map[int]int{}
							}
							perSite[k][v]++
						}
					}
					if live >= 20 && s.round.Number != lastSpots {
						lastSpots = s.round.Number
						for _, p := range s.ordered() {
							if p.Team != content.Artisan || p.Dead {
								continue
							}
							if p.ai.rotated == s.round.Number {
								rotated++
								continue
							}
							if spots[p.ai.site] == nil {
								spots[p.ai.site] = map[string]int{}
							}
							spots[p.ai.site][fmt.Sprintf("%.0f,%.0f", p.Pos[0]/4, p.Pos[2]/4)]++
						}
					}
				}
			}
			keys := []string{}
			for k := range perSite {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			fmt.Printf("%-13s rounds %d rotated %d", name, rounds, rotated)
			for _, k := range keys {
				top := 0
				for _, n := range spots[k] {
					top = max(top, n)
				}
				fmt.Printf("  %s: setup defenders->rounds %v; held from %d 4m cells (busiest %d)", k, perSite[k], len(spots[k]), top)
			}
			fmt.Println()
		})
	}
}
