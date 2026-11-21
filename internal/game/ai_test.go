package game

import (
	"slices"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// aiMatch plays n ticks of a bots-only match on a map (10 ms each, the
// server's own loop via Step) and returns the server.
func aiMatch(t *testing.T, mapName string, perTeam, ticks int) *Server {
	t.Helper()
	s := authServer(t)
	s.Map = maps.All[mapName]()
	s.cvars["mp_freezetime"] = "1"
	s.cvars["mp_round_restart_delay"] = "1"
	s.cvars["mp_respawn"] = "0"
	s.cvars["bot_difficulty"] = "3"
	for range perTeam {
		s.addAI(content.Vibe)
		s.addAI(content.Artisan)
	}
	s.Step(ticks)
	return s
}

type matchStats struct {
	rounds, kills, plants int
	reasons               []string
}

func TestAIPlaysRounds(t *testing.T) {
	names := make([]string, 0, len(maps.All))
	for n := range maps.All {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			s := aiMatch(t, name, 2, 0)
			var st matchStats
			lastRound := 0
			for range 6 * 60 * SimHz / 50 { // 6 minutes of play, in 0.5 s steps
				s.Step(50)
				if s.round.Number != lastRound {
					lastRound = s.round.Number
				}
				if s.round.Phase == PhaseOver && (len(st.reasons) < s.round.Number) {
					st.reasons = append(st.reasons, s.round.Reason)
				}
				if s.round.Bomb.State == BombPlanted && (st.plants == 0 || st.plants < s.round.Number && st.reasons != nil && len(st.reasons) < s.round.Number) {
					st.plants = max(st.plants, 1)
				}
			}
			for _, p := range s.players {
				st.kills += p.Kills
			}
			t.Logf("%s: %d rounds, ends %v, kills %d, planted %v, score %v", name, len(st.reasons), st.reasons, st.kills, st.plants > 0, s.round.Score)
			if len(st.reasons) < 3 {
				t.Fatalf("only %d rounds finished in 6 minutes: %v", len(st.reasons), st.reasons)
			}
			if st.kills == 0 {
				t.Fatal("nobody killed anybody")
			}
			// Stand-offs (both teams holding at spawn, shooting across the map)
			// run the clock out: at most a quarter of the rounds may time out.
			timeouts := 0
			for _, r := range st.reasons {
				if r == "time" {
					timeouts++
				}
			}
			if timeouts*4 > len(st.reasons) {
				t.Fatalf("%d of %d rounds timed out: the bots don't push", timeouts, len(st.reasons))
			}
			if !slices.ContainsFunc(st.reasons, func(r string) bool { return r != "time" }) {
				t.Fatalf("every round timed out: the bots never fight or plant: %v", st.reasons)
			}
		})
	}
}

func TestAIBuysAndKeepsMoneyRules(t *testing.T) {
	s := aiMatch(t, "de_slop", 1, 0)
	s.Step(200) // warmup -> round 1 freeze: bots buy
	for _, p := range s.players {
		if p.ai == nil {
			continue
		}
		if p.Money > s.econ().StartMoney || p.Money < 0 {
			t.Fatalf("%s money %d", p.Name, p.Money)
		}
		if p.Armor == 0 && p.Money >= 650 {
			t.Fatalf("%s didn't buy armor with $%d", p.Name, p.Money)
		}
	}
}

func TestBotQuotaFills(t *testing.T) {
	s := authServer(t)
	h := s.join("1.1.1.1")
	h.ready, h.Team = true, content.Vibe
	s.cvars["bot_quota"] = "4"
	s.Step(SimHz + 1)
	ai, teams := 0, map[string]int{}
	for _, p := range s.players {
		teams[p.Team]++
		if p.ai != nil {
			ai++
		}
	}
	if ai != 3 || teams[content.Vibe] != 2 || teams[content.Artisan] != 2 {
		t.Fatalf("quota 4 with one human: %d bots, teams %v", ai, teams)
	}
	s.cvars["bot_quota"] = "2"
	s.Step(SimHz)
	ai = 0
	for _, p := range s.players {
		if p.ai != nil {
			ai++
		}
	}
	if ai != 1 {
		t.Fatalf("quota 2: %d bots", ai)
	}
}

func TestNavPathReachesSites(t *testing.T) {
	s := authServer(t)
	for _, name := range []string{"A", "B"} {
		z := s.siteZone(name)
		goal := s.mustSite(name)
		path := s.navPath(Vec3(s.Map.Spawns[0]), goal)
		if z == nil || len(path) < 2 || path[len(path)-1] != goal {
			t.Fatalf("path to %s: %v", name, path)
		}
	}
}
