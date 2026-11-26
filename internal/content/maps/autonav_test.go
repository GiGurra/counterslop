package maps

import (
	"fmt"
	"math"
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
		for _, lanes := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/lanes=%v", name, lanes), func(t *testing.T) {
				t.Parallel()
				for _, e := range autoNavWalk(name, lanes) {
					t.Errorf("%s (auto nav): %s", name, e)
				}
			})
		}
	}
}

// autoNavWalk: the walks on name's generated graph: from every spawn to every
// objective, or (lanes) through every lane to every objective.
func autoNavWalk(name string, lanes bool) []string {
	m := All[name]()
	m.Nav = nil
	m.Nav = navgen.Build(&m)
	if len(m.Nav) == 0 {
		return []string{"no nav graph"}
	}
	var errs []string
	for _, team := range c.Teams {
		if lanes {
			break
		}
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
	// Lanes: an attacker (a Vibe Coder) walks spawn -> lane -> objective for
	// every lane a bot may pick (internal/game pickLane: a detour within 1.6
	// times the straight route).
	sp := m.TeamSpawns[c.Vibe][0]
	for _, l := range m.Nav {
		if !lanes || !l.Lane {
			continue
		}
		at := c.Zone{Name: l.Name, Min: [3]float64{l.Pos[0] - 0.01, l.Pos[1] - 0.01, l.Pos[2] - 0.01}, Max: [3]float64{l.Pos[0] + 0.01, l.Pos[1] + 0.01, l.Pos[2] + 0.01}}
		toLane := navPath(m, c.Vibe, at)
		for _, site := range m.Objectives() {
			direct, onward := navPath(m, c.Vibe, site), navPath(m, l.Name, site)
			if toLane == nil || onward == nil || direct == nil {
				errs = append(errs, "lane "+l.Name+": no route through it to "+site.Name)
				continue
			}
			if routeLen(toLane)+routeLen(onward) > 1.6*routeLen(direct) {
				continue // the bots won't take it there
			}
			route := append(append([]c.NavNode{}, toLane...), onward[1:]...)
			for _, model := range []string{"classic", "source"} {
				end, err := walkIn(m, sp, route, moveModels[model])
				if err != nil {
					errs = append(errs, model+" vibe via lane "+l.Name+" -> "+site.Name+": "+err.Error())
				} else if !site.Contains(end) {
					errs = append(errs, model+" vibe via lane "+l.Name+" -> "+site.Name+": ended outside the site")
				}
			}
		}
	}
	return errs
}

func routeLen(r []c.NavNode) float64 {
	d := 0.0
	for i := 1; i < len(r); i++ {
		d += math.Hypot(r[i].Pos[0]-r[i-1].Pos[0], r[i].Pos[2]-r[i-1].Pos[2])
	}
	return d
}
