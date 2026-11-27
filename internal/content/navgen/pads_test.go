package navgen

import (
	"slices"
	"testing"

	"github.com/gigurra/counterslop/internal/content/maps"
)

// Every jump pad the floor walks onto, on every map in the rotation, is one
// bots walk onto too: its node in the generated graph has a link onto it
// from a node off it (de_serverless: B's pad to A, the only node inside B cut
// off from it by the props in front, so no bot ever took it).
func TestEveryPadWalkable(t *testing.T) {
	for _, name := range maps.Order {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := maps.Find(name)()
			m.Nav = nil
			g := ready(&m)
			if g == nil {
				return
			}
			nav := Build(&m)
			for i, p := range g.pads {
				walked := false // a reachable floor cell off the pad steps onto it
				for n := range g.y {
					if g.reach[n] && g.onPad[n] == 0 {
						g.neighbours(n, func(k int, _ float64) {
							if g.onPad[k] == i+1 {
								walked = true
							}
						})
					}
					if walked {
						break
					}
				}
				if !walked {
					continue // a pad no one walks onto (a decoration, out of reach)
				}
				onto := false
				for _, nd := range nav {
					if slices.Contains(nd.OneWay, "pad "+p.name) {
						onto = true
					}
				}
				if !onto {
					t.Errorf("%s: pad %q: the floor walks onto it, no nav node does", name, p.name)
				}
			}
		})
	}
}
