package game

import (
	"fmt"
	"math"
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
	s.Seed(1) // the same match every run (and so the same coverage)
	s.cvars["mp_freezetime"] = "1"
	s.cvars["mp_round_restart_delay"] = "1"
	s.cvars["mp_respawn"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["bot_difficulty"] = "3"
	s.Map.MaxPlayers = max(s.Map.MaxPlayers, 2*perTeam) // room for every bot asked for (the team cap)
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
	t.Parallel() // a long AI sweep on its own server: runs beside the other sweeps
	names := make([]string, 0, len(maps.All))
	for n := range maps.All {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, name := range names {
		if maps.All[name]().NoBomb() || maps.All[name]().Mode == content.ModeRace {
			continue // team deathmatch, no rounds (TestAIGrenadeFight plays he_merge_hell's); bots don't race yet (TestAIIdleOnARaceMap)
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := aiMatch(t, name, 2, 0)
			var st matchStats
			lastRound := 0
			kills := func() int {
				n := 0
				for _, p := range s.players {
					n += p.Kills
				}
				return n
			}
			before, prev := 0, 0            // kills of finished matches (a new match zeroes Player.Kills), the last count
			for range 6 * 60 * SimHz / 50 { // 6 minutes of play, in 0.5 s steps
				s.Step(50)
				if k := kills(); k < prev {
					before += prev
					prev = k
				} else {
					prev = k
				}
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
			st.kills = before + kills()
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

// A bot's route from its spawn to each site follows the nav graph: every two
// consecutive nav nodes on it are linked. The goal's node is picked among the
// nodes reachable from the start (de_standup's graph comes in two pieces; the
// Vibe carrier used to aim at the Artisans' B node through a wall and stood at
// the doorway all round).
func TestNavPathFollowsLinks(t *testing.T) {
	for _, name := range maps.Order {
		s := authServer(t)
		s.Map = maps.All[name]()
		node := map[Vec3]string{}
		linked := map[[2]string]bool{}
		for _, n := range s.Map.Nav {
			node[Vec3(n.Pos)] = n.Name
			for _, l := range n.Links {
				linked[[2]string{n.Name, l}], linked[[2]string{l, n.Name}] = true, true
			}
		}
		for team, spawns := range s.Map.TeamSpawns {
			for _, site := range s.Map.Sites {
				from, to := Vec3(spawns[0]), Vec3(site.Center())
				path := s.navPath(from, to)
				// Nodes reachable from the one nearest the spawn.
				start, bd := "", math.Inf(1)
				for _, n := range s.Map.Nav {
					if d := math.Hypot(n.Pos[0]-from[0], n.Pos[2]-from[2]); d < bd {
						start, bd = n.Name, d
					}
				}
				reach := map[string]bool{start: true}
				for q := []string{start}; len(q) > 0; q = q[1:] {
					for _, n := range s.Map.Nav {
						if !reach[n.Name] && linked[[2]string{q[0], n.Name}] {
							reach[n.Name] = true
							q = append(q, n.Name)
						}
					}
				}
				prev := ""
				for _, w := range path {
					n, ok := node[w]
					if !ok {
						continue // the goal itself
					}
					if !reach[n] {
						t.Errorf("%s %s: route goes to %s, unreachable from the spawn's node %s (path %v)", name, team, n, start, path)
					}
					if prev != "" && !linked[[2]string{prev, n}] {
						t.Errorf("%s %s: route jumps %s → %s, not linked (path %v)", name, team, prev, n, path)
					}
					prev = n
				}
			}
		}
	}
}

// A seeded match replays exactly: players are walked in ID order, not the
// players map's random order (which decided who drew from the seeded rng
// first, so CI saw a different de_standup match than anyone locally).
func TestAIMatchReplaysWithTheSameSeed(t *testing.T) {
	trace := func() string {
		s := aiMatch(t, "de_standup", 3, 20*SimHz)
		out := fmt.Sprintf("round %d %s score %v;", s.round.Number, s.round.Phase, s.round.Score)
		for _, p := range s.ordered() {
			out += fmt.Sprintf(" %d:%v/%d/%d/%d", p.ID, p.Pos, p.HP, p.Kills, p.Money)
		}
		return out
	}
	if a, b := trace(), trace(); a != b {
		t.Fatalf("same seed, different match:\n%s\n%s", a, b)
	}
}
