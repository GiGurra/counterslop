package maps

import (
	"fmt"
	"math"
	"slices"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

// zFights lists pairs of boxes with a coplanar face pointing the same way that
// overlap with real area: the renderer draws both at the same depth, and they
// flicker (z-fighting) as the camera moves. A "3D lint" for map geometry.
func zFights(m c.Map) []string {
	const eps = 1e-6
	var out []string
	for i, a := range m.Boxes {
		for j := i + 1; j < len(m.Boxes); j++ {
			b := m.Boxes[j]
			for ax := range 3 {
				u, v := (ax+1)%3, (ax+2)%3
				ou := math.Min(a.Max[u], b.Max[u]) - math.Max(a.Min[u], b.Min[u])
				ov := math.Min(a.Max[v], b.Max[v]) - math.Max(a.Min[v], b.Min[v])
				if ou <= 0.01 || ov <= 0.01 {
					continue // the faces don't overlap (or only along an edge)
				}
				for _, hi := range []bool{false, true} {
					pa, pb := a.Min[ax], b.Min[ax]
					if hi {
						pa, pb = a.Max[ax], b.Max[ax]
					}
					if math.Abs(pa-pb) > eps {
						continue
					}
					if ax == 1 && !hi && pa <= 0 {
						continue // undersides at the floor are never drawn (boxMesh)
					}
					side := map[bool]string{false: "min", true: "max"}[hi]
					out = append(out, fmt.Sprintf("boxes %d %v-%v (%s) and %d %v-%v (%s): both %s %c faces at %g overlap %.2f x %.2f m",
						i, a.Min, a.Max, a.Kind, j, b.Min, b.Max, b.Kind, side, "xyz"[ax], pa, ou, ov))
				}
			}
		}
	}
	return out
}

func TestNoZFighting(t *testing.T) {
	names := make([]string, 0, len(All))
	for n := range All {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		for _, f := range zFights(All[n]()) {
			t.Errorf("%s: %s", n, f)
		}
	}
}
