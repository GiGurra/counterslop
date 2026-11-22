package game

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// TestAIBalanceSweep (AI_SWEEP=1, ~a minute): 3v3 bot matches, 10 seeds x 6
// minutes per map at bot_difficulty 3; prints each map's attacker (Vibe Coder)
// round wins and timeouts, the rounds planted at each site and the rounds the
// attackers went for each site (with how many of those they won). AI_SWEEP_SEEDS / AI_SWEEP_PER_TEAM change the size.
// The target: attackers win 40-60% on every map, under 10% of rounds time out.
func TestAIBalanceSweep(t *testing.T) {
	if os.Getenv("AI_SWEEP") == "" {
		t.Skip("AI_SWEEP=1 to run")
	}
	seeds, perTeam := 10, 3
	if n, err := strconv.Atoi(os.Getenv("AI_SWEEP_SEEDS")); err == nil {
		seeds = n
	}
	if n, err := strconv.Atoi(os.Getenv("AI_SWEEP_PER_TEAM")); err == nil {
		perTeam = n
	}
	only := os.Getenv("AI_SWEEP_MAPS") // e.g. "de_hackathon,de_pipeline": just these
	for _, name := range maps.Order {
		if only != "" && !slices.Contains(strings.Split(only, ","), name) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			st := sweepMap(t, name, seeds, perTeam)
			fmt.Printf("%-12s rounds %3d  attackers %3d (%2.0f%%)  timeouts %2d (%2.0f%%)  %v  plants (won) %v  attacked (won) %v\n", name, st.rounds,
				st.wins, 100*float64(st.wins)/float64(max(st.rounds, 1)), st.reasons["time"], 100*float64(st.reasons["time"])/float64(max(st.rounds, 1)), st.reasons, st.sites, st.attacked)
		})
	}
}

type sweepStats struct {
	rounds, wins int
	reasons      map[string]int
	sites        map[string][2]int // per site: rounds planted there, and of those won by the attackers
	attacked     map[string][2]int // per site: rounds the attacker bots went for it, and of those won
}

func sweepMap(t *testing.T, name string, seeds, perTeam int) sweepStats {
	st := sweepStats{reasons: map[string]int{}, sites: map[string][2]int{}, attacked: map[string][2]int{}}
	for seed := 1; seed <= seeds; seed++ {
		s := authServer(t)
		s.Map = maps.All[name]()
		s.Seed(uint64(seed))
		s.cvars["mp_freezetime"] = "1"
		s.cvars["mp_round_restart_delay"] = "1"
		s.cvars["mp_respawn"] = "0"
		s.cvars["mp_warmup_time"] = "0" // the whole budget is rounds
		s.cvars["bot_difficulty"] = "3"
		for range perTeam {
			s.addAI(content.Vibe)
			s.addAI(content.Artisan)
		}
		over := false
		for range 6 * 60 * SimHz / 10 {
			s.Step(10)
			if s.round.Phase == PhaseOver && !over {
				st.rounds++
				st.reasons[s.round.Reason]++
				if s.round.Winner == content.Vibe {
					st.wins++
				}
				if a := s.aiAttack; a.round == s.round.Number && a.site != "" {
					c := st.attacked[a.site]
					c[0]++
					if s.round.Winner == content.Vibe {
						c[1]++
					}
					st.attacked[a.site] = c
				}
				if site := s.round.Bomb.Site; site != "" {
					c := st.sites[site]
					c[0]++
					if s.round.Winner == content.Vibe {
						c[1]++
					}
					st.sites[site] = c
				}
			}
			over = s.round.Phase == PhaseOver
		}
	}
	return st
}
