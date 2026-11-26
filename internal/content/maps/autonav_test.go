package maps

import (
	"os"
	"testing"
	"time"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/navgen"
)

// autoNavMaps: built-in maps whose hand nav graph navgen must replace: with
// it deleted and generated from the geometry, every team walks from every
// spawn to every objective along it (TestWalkToSites' check, with the real
// movement code).
var autoNavMaps = []string{"de_slop", "de_monorepo", "de_standup", "de_pipeline", "de_legacy", "de_runway", "de_rackmount", "surf_scope_creep", "he_merge_hell"}

func TestAutoNavWalkToSites(t *testing.T) {
	for _, name := range autoNavMaps {
		if errs := autoNavWalk(name); len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("%s (auto nav): %s", name, e)
			}
		}
	}
}

// The rest of the rotation, for the record: how far auto nav gets there
// (NAVGEN_ALL=1 go test ./internal/content/maps -run TestAutoNavEveryMap -v;
// a minute: gs_megabyte's misses each walk a while).
func TestAutoNavEveryMap(t *testing.T) {
	if os.Getenv("NAVGEN_ALL") == "" {
		t.Skip("a report: NAVGEN_ALL=1")
	}
	for _, name := range Order {
		t0 := time.Now() // wall: a report, no game time
		errs := autoNavWalk(name)
		t.Logf("%s: %d problems (%v)", name, len(errs), time.Since(t0).Round(time.Millisecond)) // wall: a report
		for i, e := range errs {
			if i < 3 {
				t.Logf("  %s", e)
			}
		}
	}
}

func autoNavWalk(name string) []string {
	m := All[name]()
	m.Nav = nil
	m.Nav = navgen.Build(&m)
	if len(m.Nav) == 0 {
		return []string{"no nav graph"}
	}
	var errs []string
	for _, team := range c.Teams {
		for si, spawn := range m.TeamSpawns[team] {
			for _, site := range m.Objectives() {
				route := navPath(m, team, site)
				if route == nil {
					errs = append(errs, "no route from "+team+" to "+site.Name)
					continue
				}
				for _, model := range []string{"classic", "source"} {
					end, err := walkIn(m, spawn, route, moveModels[model])
					if err != nil {
						errs = append(errs, model+" "+team+" spawn "+string(rune('0'+si))+" -> "+site.Name+": "+err.Error())
					} else if !site.Contains(end) {
						errs = append(errs, model+" "+team+" spawn "+string(rune('0'+si))+" -> "+site.Name+": ended outside the site")
					}
				}
			}
		}
	}
	return errs
}
