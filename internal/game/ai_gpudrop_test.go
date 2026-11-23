package game

import (
	"math"
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
	t.Parallel() // a long AI sweep on its own server: runs beside the other sweeps
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

// The staff don't all camp the stock: over two minutes, some of them hold
// forward posts (taking turns: more than one bot goes out), while someone
// stays near the GPUs most of the time.
func TestAIStaffMixItUp(t *testing.T) {
	t.Parallel() // a long AI sweep on its own server: runs beside the other sweeps
	s := megabyteBots(t, 1, 4, content.Vibe)
	nearStock := func(p *Player) bool {
		for _, g := range s.Map.GPUs {
			if math.Hypot(p.Pos[0]-g.Pos[0], p.Pos[2]-g.Pos[2]) < 10 && math.Abs(p.Pos[1]-g.Pos[1]) < 2 {
				return true
			}
		}
		return false
	}
	away := map[*Player]int{}
	guarded, samples := 0, 0
	for range 120 * 2 { // 2 minutes in 0.5 s steps
		s.Step(SimHz / 2)
		samples++
		home := false
		for _, p := range s.ordered() {
			if p.Team != content.Artisan || p.Dead {
				continue
			}
			if nearStock(p) {
				home = true
			} else {
				away[p]++
			}
		}
		if home {
			guarded++
		}
	}
	out := 0
	for p, n := range away {
		t.Logf("%s away from the stock %d of %d samples", p.Name, n, samples)
		if n >= samples/5 {
			out++
		}
	}
	if out < 2 {
		t.Fatalf("%d staff bots spent a fifth of the time away from the stock, want at least 2 (taking turns)", out)
	}
	if guarded*10 < samples*8 {
		t.Fatalf("someone at the stock only %d of %d samples", guarded, samples)
	}
}

// Every GPU spot on the GPU Drop maps has forward posts, none of them in the
// stock or at the scalpers' spawn; a lone staff bot stays home.
func TestStockPosts(t *testing.T) {
	for _, name := range []string{"gs_kiosk", "gs_megabyte"} {
		s := authServer(t)
		s.Map = maps.All[name]()
		for i, g := range s.Map.GPUs {
			posts := s.stockPosts(Vec3(g.Pos))
			if name == "gs_megabyte" && len(posts) < 3 { // gs_kiosk is small: the scalpers get everywhere first
				t.Errorf("%s gpu %d: posts %v, want 3 or more", name, i, posts)
			}
			for _, at := range posts {
				if s.nearGPU(at, aiPostOut) || s.nearSpawn(content.Vibe, at, aiPostSpawn) {
					t.Errorf("%s gpu %d: post %v in the stock or at their spawn", name, i, at)
				}
			}
		}
	}
	s := megabyteBots(t, 1, 1, content.Vibe)
	for range 4 {
		s.Step(aiStaffShift)
		for _, p := range s.ordered() {
			if p.ai != nil && p.Team == content.Artisan && p.ai.post.on {
				t.Fatalf("the only staff bot went out to %v", p.ai.post.at)
			}
		}
	}
}

// A bot walks down the east escalator from wherever it stands at its top:
// the lane is 1.2 m between handrails (0.4 m to spare), so heading straight
// for the foot from off the line ran it into a handrail's end, and it
// stayed there the rest of the round.
func TestAIDownTheEastEscalator(t *testing.T) {
	s := megabyteBots(t, 1, 1, content.Artisan) // the staff bot is parked
	var v *Player
	for _, p := range s.ordered() {
		if p.Team == content.Vibe {
			v = p
		}
	}
	for _, from := range []Vec3{{18.57, 5, -5.4}, {19.2, 5, -5.4}, {19.7, 5, -5.5}, {18.2, 5, -6}, {20.5, 5, -6.5}, {17.5, 5, -7.5}} {
		s.teleport(v, from, math.Pi, false, v.lastQueued)
		v.ai.path, v.ai.goalKey, v.ai.site = nil, "", "gpu2"
		v.ai.lane = aiLane{round: s.round.Number, site: "gpu2"} // no detour through the aisles
		for range 6 * SimHz / 10 {
			s.Step(10)
			if v.Pos[1] < 1 {
				break
			}
		}
		if v.Pos[1] > 1 {
			t.Errorf("from %v: still upstairs at %.2f after 6 s", from, v.Pos)
		}
	}
}
