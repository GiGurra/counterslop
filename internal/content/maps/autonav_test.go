package maps

import (
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/navgen"
)

// Every built-in map with its hand nav graph deleted and one built from its
// geometry instead (navgen): every team walks from every spawn to every
// objective along it (TestWalkToSites' check, the real movement code, both
// movement models). A map's own graph is only a hand-tuned choice of routes.
func TestAutoNavWalkToSites(t *testing.T) {
	for _, name := range Order {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, e := range autoNavWalk(name) {
				t.Errorf("%s (auto nav): %s", name, e)
			}
		})
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
