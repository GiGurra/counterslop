package game

import (
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
