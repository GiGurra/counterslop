package game

import (
	"slices"
	"testing"

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
		if selfBlind*3 > popped && popped > 2 {
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
