package game

import (
	"slices"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// TestAIThrowsGrenades: in bots-only matches on every map, bots buy and throw
// HEs and flashes (Vibe Coders into their site, Artisans at enemies), and a
// bot that throws a flash has turned away from it.
func TestAIThrowsGrenades(t *testing.T) {
	names := make([]string, 0, len(maps.All))
	for n := range maps.All {
		names = append(names, n)
	}
	slices.Sort(names)
	total := map[string]int{}
	for _, name := range names {
		s := aiMatch(t, name, 2, 0)
		s.cvars["mp_startmoney"] = "4000"
		kinds := map[string]int{}
		seen, live := map[int]bool{}, map[int]*Nade{}
		popped, selfBlind, enemiesBlind := 0, 0, 0
		for range 4 * 60 * SimHz / 10 { // 4 minutes in 0.1 s steps
			s.Step(10)
			now := map[int]bool{}
			for _, n := range s.nades {
				now[n.ID] = true
				live[n.ID] = n
				if !seen[n.ID] {
					seen[n.ID] = true
					kinds[n.Kind]++
				}
			}
			for id, n := range live { // went off since the last step
				if now[id] {
					continue
				}
				delete(live, id)
				if n.Kind != "flash" || s.round.Phase != PhaseLive {
					continue
				}
				popped++
				if n.by.flashLeft(s.clock()) > 1.5 {
					selfBlind++
				}
				for _, q := range s.players {
					if q.Team != n.by.Team && q.flashLeft(s.clock()) > 1 {
						enemiesBlind++
					}
				}
			}
		}
		t.Logf("%s: thrown %v; flashes popped %d: thrower blinded %d, enemies blinded %d", name, kinds, popped, selfBlind, enemiesBlind)
		// Matches aren't fully reproducible: a loose bound here; the exact look-away
		// is TestAISiteThrowHEThenFlashThenLooksAway's (scripted).
		if selfBlind*2 > popped && popped > 4 {
			t.Errorf("%s: bots blind themselves with their own flashes (%d of %d)", name, selfBlind, popped)
		}
		for k, v := range kinds {
			total[k] += v
		}
	}
	if total["he"] == 0 || total["flash"] == 0 {
		t.Fatalf("bots threw %v over every map; want HEs and flashes", total)
	}
}

// aiNadeBot: one Vibe Coder bot on de_slop, its site A decided, carrying an HE
// and a flash, standing where it sees into A.
func aiNadeBot(t *testing.T, pos Vec3) (*Server, *Player) {
	t.Helper()
	s := authServer(t)
	s.Map = maps.All["de_slop"]()
	p := s.addAI(content.Vibe)
	s.Step(1)
	s.teleport(p, pos, 0, false, 0)
	p.ai.round, p.ai.site = s.round.Number, "A"
	p.inv.Slots["he"] = &Held{ID: "mergeconflict", Ammo: 1}
	p.inv.Slots["flash"] = &Held{ID: "stacktrace", Ammo: 1}
	return s, p
}

func TestAISiteThrowHEThenFlashThenLooksAway(t *testing.T) {
	s, p := aiNadeBot(t, Vec3{10, 0, 10})
	kinds := []string{}
	seen := map[int]bool{}
	averted := false
	for range 600 {
		s.Step(1)
		for _, n := range s.nades {
			if !seen[n.ID] {
				seen[n.ID] = true
				kinds = append(kinds, n.Kind)
			}
		}
		averted = averted || p.ai.nade.avert > p.cmdTicks
	}
	if !slices.Equal(kinds, []string{"he", "flash"}) {
		t.Fatalf("thrown %v, want the HE then the flash", kinds)
	}
	if !averted || p.flashLeft(s.clock()) > 0 {
		t.Fatalf("averted %v, still blind %.2f s", averted, p.flashLeft(s.clock()))
	}
	if p.inv.Current != "primary" || p.inv.Slots["he"] != nil || p.inv.Slots["flash"] != nil {
		t.Fatalf("after the throws: current %s, slots %v", p.inv.Current, p.inv.Slots)
	}
	if s.aiStartNade(p, "he", Vec3{20, 0, 20}, false) {
		t.Fatal("started a throw with no HE left")
	}
}

func TestAINadeHelpers(t *testing.T) {
	s, p := aiNadeBot(t, Vec3{10, 0, 10})
	o := s.eyeOf(p)
	if _, ok := s.lobPitch(o, Vec3{o[0], 0, o[2] - 15}); !ok {
		t.Fatal("15 m should be in reach")
	}
	if _, ok := s.lobPitch(o, Vec3{o[0], 0, o[2] - 200}); ok {
		t.Fatal("200 m is out of reach")
	}
	if s.aiStartNade(p, "smoke", Vec3{20, 0, 20}, false) {
		t.Fatal("no smoke carried")
	}
	if !s.aiStartNade(p, "he", Vec3{20, 0, 20}, false) || s.aiStartNade(p, "flash", Vec3{20, 0, 20}, false) {
		t.Fatal("one throw at a time")
	}
	// Drawn, then an enemy shows up: back to the rifle, nothing thrown.
	p.inv.Current = "he"
	var c Cmd
	s.aiNadeStop(p, &c)
	if c.W != "primary" || p.ai.aiNadeBusy(p.cmdTicks) {
		t.Fatalf("stop: switch %q, busy %v", c.W, p.ai.aiNadeBusy(p.cmdTicks))
	}
	// A throw that never happens gives up.
	s.aiStartNade(p, "he", Vec3{20, 0, 20}, false)
	p.ai.nade.started = p.cmdTicks - aiNadeTimeout - 1
	c = Cmd{}
	s.aiNade(p, &c)
	if p.ai.aiNadeBusy(p.cmdTicks) {
		t.Fatal("timed-out throw still busy")
	}
	if WeaponKind(nil) != "" || WeaponKind(s.Weapons.Get("prompt")) != "" || WeaponKind(s.Weapons.Get("stacktrace")) != "flash" {
		t.Fatal("WeaponKind")
	}
	if p.ai.nadeWeapon("nope", s) != "" || p.ai.nadeWeapon("he", s) != "mergeconflict" {
		t.Fatal("nadeWeapon")
	}
	bare := &Player{inv: &Inventory{Slots: map[string]*Held{}, Current: "he"}}
	if s.aiBestGun(bare) != "he" {
		t.Fatal("aiBestGun with nothing else keeps the current slot")
	}
	// The Artisans don't throw into sites; a Vibe Coder far from the site doesn't either.
	p.Team = content.Artisan
	if s.aiSiteNades(p) {
		t.Fatal("artisan site throw")
	}
	p.Team = content.Vibe
	s.teleport(p, Vec3{-28, 0, -28}, 0, false, 0)
	if s.aiSiteNades(p) {
		t.Fatal("site throw from across the map")
	}
}
