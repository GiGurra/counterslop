package mapcheck

import (
	"math"
	"strings"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// Every built-in map passes the checks a custom map is loaded with
// (Problems): among them, every team walks from every one of its spawns to
// every objective along the nav graph with the real movement code, in both
// movement models (WalkProblems).
func TestWalkToSites(t *testing.T) {
	for _, name := range maps.Order {
		if p := Problems(maps.All[name](), nil); len(p) > 0 {
			t.Errorf("%s: %v", name, p)
		}
	}
}

// A broken map is refused with the reason.
func TestProblemsSaySo(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*c.Map)
		want string
	}{
		{"material", func(m *c.Map) { m.Boxes[0].Kind = "unobtainium" }, `unknown material "unobtainium"`},
		{"empty box", func(m *c.Map) { m.Boxes[0].Max[1] = m.Boxes[0].Min[1] }, "box 0 is empty on axis 1"},
		{"name", func(m *c.Map) { m.Name = "De Slop!" }, "name"},
		{"buy zone", func(m *c.Map) { m.BuyZones = nil }, "outside its buy zones"},
		{"no sites", func(m *c.Map) { m.Sites = nil }, "site"},
		{"nav", func(m *c.Map) { m.Nav = nil }, `no nav route from "vibe"`},
		{"rule", func(m *c.Map) { m.Rules = map[string]string{"rcon_password": "x"} }, `rule "rcon_password"`},
		{"too big", func(m *c.Map) { m.Boxes = make([]c.Box, MaxBoxes+1) }, "boxes"},
		{"far", func(m *c.Map) { m.Boxes[0].Max[0] = 1e6 }, "m out"},
		{"walled in", func(m *c.Map) {
			// A wall across the middle of the longest leg of the route to A.
			r := NavPath(*m, c.Vibe, m.Sites[0])
			i := 0
			for k := 1; k+1 < len(r); k++ {
				if dist2(r[k].Pos, r[k+1].Pos) > dist2(r[i].Pos, r[i+1].Pos) {
					i = k
				}
			}
			a, b := r[i].Pos, r[i+1].Pos
			mx, mz := (a[0]+b[0])/2, (a[2]+b[2])/2
			m.Boxes = append(m.Boxes, c.B(mx-0.25, a[1], mz-0.25, mx+0.25, a[1]+8, mz+0.25, c.Concrete))
			for d := 0.5; d < 4; d += 0.5 { // 8 m across the leg
				nx, nz := -(b[2] - a[2]), b[0]-a[0]
				l := math.Hypot(nx, nz)
				for _, sg := range []float64{-1, 1} {
					x, z := mx+sg*d*nx/l, mz+sg*d*nz/l
					m.Boxes = append(m.Boxes, c.B(x-0.25, a[1], z-0.25, x+0.25, a[1]+8, z+0.25, c.Concrete))
				}
			}
		}, "stuck"},
	} {
		m := maps.DeSlop()
		tc.edit(&m)
		p := Problems(m, nil)
		if !strings.Contains(strings.Join(p, "; "), tc.want) {
			t.Errorf("%s: %v, want %q", tc.name, p, tc.want)
		}
	}
}

func dist2(a, b [3]float64) float64 { return (a[0]-b[0])*(a[0]-b[0]) + (a[2]-b[2])*(a[2]-b[2]) }
