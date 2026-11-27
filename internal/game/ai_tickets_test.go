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
	for range 90 * SimHz / 10 {
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

// cq_moonshot with bots: Earth's ride MVP-1 to the Moon, take points there
// from the Moon's, and both sides kill (a few departures' worth of play).
func TestAIMoonshot(t *testing.T) {
	t.Parallel() // a long AI run on its own server
	s := authServer(t)
	s.Map = maps.CqMoonshot()
	s.autoNav()
	s.Seed(11)
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["bot_difficulty"] = "1"
	for range 4 {
		s.addAI(content.Vibe)
		s.addAI(content.Artisan)
	}
	held := map[string]bool{}
	kills, last := map[string]int{}, map[*Player]int{} // over match restarts (they zero Player.Kills)
	for range 4 * 60 * SimHz / 50 {                    // 4 minutes
		s.Step(50)
		for i, pt := range s.round.Points {
			if pt.Own == content.Vibe && !s.Map.Points[i].Locked {
				held[s.Map.Points[i].ID] = true
			}
		}
		for _, p := range s.players {
			if p.Kills > last[p] {
				kills[p.Team] += p.Kills - last[p]
			}
			last[p] = p.Kills
		}
	}
	t.Logf("Earth held %v; kills %v; tickets %v", held, kills, s.round.Tickets)
	if len(held) == 0 || kills[content.Vibe] == 0 || kills[content.Artisan] == 0 {
		t.Fatalf("Earth held %v on the Moon, kills %v", held, kills)
	}
}
