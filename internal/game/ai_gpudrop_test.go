package game

import (
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// megabyteBots: gs_megabyte with bot teams; parked players stand idle on the
// roof (out of sight of everything inside), so the other side plays alone.
func megabyteBots(t *testing.T, vibes, artisans int, park string) *Server {
	t.Helper()
	s := authServer(t)
	s.Map = maps.All["gs_megabyte"]()
	s.Seed(7)
	s.cvars["mp_freezetime"] = "1"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_respawn"] = "0"
	s.cvars["mp_roundtime"] = "300"
	s.cvars["bot_difficulty"] = "3"
	for range vibes {
		s.addAI(content.Vibe)
	}
	for range artisans {
		s.addAI(content.Artisan)
	}
	s.Step(2 * SimHz) // past the freeze
	for i, p := range s.ordered() {
		if p.Team == park {
			p.ai = nil // a player who doesn't play
			s.teleport(p, Vec3{40 - 3*float64(i), 10.5, -30}, 0, false, p.lastQueued)
		}
	}
	if s.round.Phase != PhaseLive || !s.gpuMode() {
		t.Fatalf("phase %q gpu mode %v", s.round.Phase, s.gpuMode())
	}
	return s
}

// Scalper bots go down to the stock, grab GPUs, carry them out an exit and
// win by securing them.
func TestAIScalpersGrabAndSecure(t *testing.T) {
	s := megabyteBots(t, 3, 1, content.Artisan)
	round, grabbed := s.round.Number, false
	for range 240 * SimHz / 10 {
		s.Step(10)
		for _, g := range s.round.GPUs {
			grabbed = grabbed || g.State == GPUCarried
		}
		if s.round.Number != round || s.round.Phase == PhaseOver {
			break
		}
	}
	if !grabbed || s.round.Winner != content.Vibe || s.round.Reason != "secured" {
		t.Fatalf("grabbed %v, round over: winner %q reason %q (secured %d of %d)", grabbed, s.round.Winner, s.round.Reason, s.round.Secured, s.round.ToWin)
	}
}

// A dropped GPU: the nearest staff bot goes and restocks it.
func TestAIStaffRestock(t *testing.T) {
	s := megabyteBots(t, 1, 2, content.Vibe)
	g := &s.round.GPUs[1]
	g.State, g.Pos = GPUDropped, Vec3{-20, -5, 28.5} // in the basement corridor
	for range 40 * SimHz / 10 {
		s.Step(10)
		if g.State == GPUShelf {
			if g.Pos != Vec3(s.Map.GPUs[1].Pos) {
				t.Fatalf("restocked to %v, want its shelf %v", g.Pos, s.Map.GPUs[1].Pos)
			}
			return
		}
	}
	t.Fatalf("GPU still %s at %v after 40 s", g.State, g.Pos)
}

// The scalpers' targets: the GPUs (named like the map's objectives), a team
// target with lurkers; staff hold a GPU spot each.
func TestAIGPUTargets(t *testing.T) {
	s := megabyteBots(t, 4, 2, "")
	if got := s.objectiveNames(); len(got) != 4 || got[0] != "gpu1" || got[3] != "gpu4" {
		t.Fatalf("objectives %v", got)
	}
	for _, p := range s.ordered() {
		if i := gpuIndex(p.ai.site); i < 0 || i >= 4 {
			t.Fatalf("%s bot plays %q, want a GPU", p.Team, p.ai.site)
		}
	}
	if gpuIndex("A") != -1 || gpuIndex("gpu0") != -1 || gpuIndex(gpuName(2)) != 2 {
		t.Fatal("gpuIndex")
	}
}
