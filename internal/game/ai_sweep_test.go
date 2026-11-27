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
	"github.com/gigurra/counterslop/internal/content/rng"
)

// TestAIBalanceSweep (AI_SWEEP=1, ~a minute): 3v3 bot matches, 10 seeds x 6
// minutes per map at bot_difficulty 3; prints each map's attacker (Vibe Coder)
// round wins and timeouts, the rounds planted at each site and the rounds the
// attackers went for each site (with how many of those they won). AI_SWEEP_SEEDS / AI_SWEEP_PER_TEAM change the size, AI_SWEEP_SEED0=20 plays seeds 21.. instead;
// AI_SWEEP_SOLID=0 plays with mp_solid_players 0 (players walk through each other) to compare.
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
			first, _ := strconv.Atoi(os.Getenv("AI_SWEEP_SEED0")) // another set of seeds: the noise in these numbers
			st := sweepMap(t, name, first, seeds, perTeam, 6)
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

// sweepMap plays seeds first+1..first+seeds of map name, perTeam bots a side,
// minutes each.
func sweepMap(t *testing.T, name string, first, seeds, perTeam, minutes int) sweepStats {
	st := sweepStats{reasons: map[string]int{}, sites: map[string][2]int{}, attacked: map[string][2]int{}}
	for seed := first + 1; seed <= first+seeds; seed++ {
		s := authServer(t)
		s.Map = maps.All[name]()
		s.autoNav() // a map without a graph gets one, as on a changelevel (de_serverless)
		s.Seed(uint64(seed))
		s.cvars["mp_freezetime"] = "1"
		s.cvars["mp_round_restart_delay"] = "1"
		s.cvars["mp_respawn"] = "0"
		s.cvars["mp_warmup_time"] = "0" // the whole budget is rounds
		s.cvars["bot_difficulty"] = "3"
		s.cvars["mp_maxrounds"] = "1000" // one match: no rotating to the next map mid-sweep
		if v := os.Getenv("AI_SWEEP_SOLID"); v != "" {
			s.cvars["mp_solid_players"] = v
		}
		for range perTeam {
			s.addAI(content.Vibe)
			s.addAI(content.Artisan)
		}
		for _, p := range s.ordered() {
			// The bots' own rngs come from their ids: without this, every
			// seed plays the same match.
			p.ai.r = rng.New(uint32(seed)*2654435761 + uint32(p.ID)*40503 + 1)
		}
		over := false
		for range minutes * 60 * SimHz / 10 {
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
