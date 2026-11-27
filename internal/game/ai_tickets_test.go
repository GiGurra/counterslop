package game

import (
	"fmt"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// Bots on lab_conquest play the tickets mode through: they capture points
// (each side takes some), die and respawn, and one backlog runs out.
func TestAITicketsPlayThrough(t *testing.T) {
	t.Parallel() // a long AI run on its own server
	aiTicketsRun(t, 3)
}

func aiTicketsRun(t *testing.T, seed uint64) string {
	t.Helper()
	s := authServer(t)
	s.Map = maps.LabConquest()
	s.Seed(seed)
	s.cvars["mp_freezetime"] = "1"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_respawn"] = "0"
	s.cvars["mp_tickets"] = "25"
	s.cvars["bot_difficulty"] = "2"
	for range 3 {
		s.addAI(content.Vibe)
		s.addAI(content.Artisan)
	}
	s.Step(2 * SimHz)
	if s.round.Phase != PhaseLive || !s.ticketMode() {
		t.Fatalf("phase %q tickets mode %v", s.round.Phase, s.ticketMode())
	}
	took := map[string]bool{}
	respawned := false
	for range 900 * SimHz / 10 {
		s.Step(10)
		for _, p := range s.round.Points {
			if p.Own != "" {
				took[p.ID+":"+p.Own] = true
			}
		}
		for _, p := range s.players {
			respawned = respawned || (p.Deaths > 0 && !p.Dead)
		}
		if s.round.Phase != PhaseLive {
			break
		}
	}
	r := &s.round
	vibeTook, artTook := took["A:vibe"] || took["B:vibe"] || took["C:vibe"] || took["D:vibe"], took["E:artisan"] || took["B:artisan"] || took["C:artisan"] || took["D:artisan"]
	if !vibeTook || !artTook || !respawned {
		t.Errorf("captures %v (Earth %v, Moon %v), a respawn %v", took, vibeTook, artTook, respawned)
	}
	if r.Phase != PhaseOver || r.Reason != "tickets" || r.Tickets[content.Teams[0]]*r.Tickets[content.Teams[1]] != 0 {
		t.Fatalf("after the run: phase %s winner %q (%s), tickets %v", r.Phase, r.Winner, r.Reason, r.Tickets)
	}
	t.Logf("winner %s, tickets %v, captures %v", r.Winner, r.Tickets, took)
	return r.Winner
}

// On lab_moon the Moon's points are across the void: Earth's bots walk
// through the terminal to MVP-1's door, board while it is docked, ride it
// and are dropped on the Moon, where they fight for its points.
func TestAIBoardsTheRocket(t *testing.T) {
	t.Parallel() // a long AI run on its own server
	s := authServer(t)
	s.Map = maps.LabMoon()
	s.autoNav()
	s.Seed(5)
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_respawn"] = "0"
	s.cvars["bot_difficulty"] = "0"
	for range 2 {
		s.addAI(content.Vibe)
	}
	s.addAI(content.Artisan)
	moon := map[int]bool{}
	boarded := map[int]bool{}
	for range 150 * SimHz / 10 { // five departures: the Moon's bot defends the drop (one may be shot falling)
		s.Step(10)
		for _, p := range s.players {
			if p.Team != content.Vibe || p.Dead {
				continue
			}
			if s.aboard(p, &s.Map.Departures[0]) && !s.docked(&s.Map.Departures[0], 0) {
				boarded[p.ID] = true
			}
			if boarded[p.ID] && p.Pos[0] > 40 && p.body.OnGround && p.Pos[1] > 19.5 && p.Pos[1] < 21 {
				moon[p.ID] = true
			}
		}
	}
	if len(boarded) < 2 || len(moon) < 2 {
		t.Fatalf("Earth's bots: %d rode MVP-1, %d landed on the Moon (want 2 and 2)", len(boarded), len(moon))
	}
}

// cq_moonshot with bots is roughly even across team sizes (phase 5: Earth's
// bots used to lose their whole backlog in 4 minutes while the Moon camped the
// drop zone): 2v2, 4v4 and 8v8, two seeds each, 5 minutes a match. Nobody is
// wiped out, each side is ahead on points some of the time in every match,
// Earth takes a Moon point and both sides kill, and Earth leads on tickets in
// a quarter to three quarters of the matches. AI_MOONSHOT=1
// TestAIMoonshotSweep: four seeds, 8 minutes.
func TestAIMoonshot(t *testing.T) {
	type match struct {
		n    int
		seed uint64
		r    moonshotResult
	}
	var ms []*match
	for _, n := range []int{2, 4, 8} {
		for seed := uint64(1); seed <= 2; seed++ {
			ms = append(ms, &match{n: n, seed: seed})
		}
	}
	t.Run("matches", func(t *testing.T) {
		for _, m := range ms {
			t.Run(fmt.Sprintf("%dv%d-%d", m.n, m.n, m.seed), func(t *testing.T) {
				t.Parallel() // a long AI run on its own server
				m.r = moonshotMatch(t, m.n, m.seed, 5*60)
				r := m.r
				t.Logf("tickets %v; points ahead earth %.2f moon %.2f; earth held %v; kills %v", r.tickets, r.earthAhead, r.moon, r.earthHeld, r.kills)
				if r.winner != "" || r.earthAhead == 0 || r.moon == 0 {
					t.Errorf("one-sided: tickets %v, winner %q, ahead earth %.2f moon %.2f", r.tickets, r.winner, r.earthAhead, r.moon)
				}
				if len(r.earthHeld) == 0 || r.kills[content.Vibe] == 0 || r.kills[content.Artisan] == 0 {
					t.Errorf("Earth held %v on the Moon, kills %v", r.earthHeld, r.kills)
				}
			})
		}
	})
	earth := 0
	for _, m := range ms {
		if m.r.tickets[content.Vibe] > m.r.tickets[content.Artisan] {
			earth++
		}
	}
	t.Logf("Earth leads %d of %d matches", earth, len(ms))
	if earth*4 < len(ms) || earth*4 > 3*len(ms) {
		t.Errorf("Earth leads %d of %d matches: not even", earth, len(ms))
	}
}
