package maps

import (
	"math"
	"slices"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// coplanar: a and b have a face pointing the same way within tol of each
// other's plane, overlapping with real area (they would z-fight).
func coplanar(a, b c.Box, tol float64) (int, bool) {
	for ax := range 3 {
		u, v := (ax+1)%3, (ax+2)%3
		ou := math.Min(a.Max[u], b.Max[u]) - math.Max(a.Min[u], b.Min[u])
		ov := math.Min(a.Max[v], b.Max[v]) - math.Max(a.Min[v], b.Min[v])
		if ou <= 0.01 || ov <= 0.01 {
			continue
		}
		if math.Abs(a.Min[ax]-b.Min[ax]) <= tol || math.Abs(a.Max[ax]-b.Max[ax]) <= tol {
			return ax, true
		}
	}
	return 0, false
}

// TestMoversNoZFighting: at every phase of the treads' cycle, no drawn mover
// box has a face within 1 mm of a static box's (the escalators' landings
// flickered against the floor where the treads run level).
func TestMoversNoZFighting(t *testing.T) {
	names := make([]string, 0, len(All))
	for n := range All {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		m := All[n]()
		if len(m.Movers) == 0 {
			continue
		}
		var static []c.Box
		for i := range m.Boxes {
			if !sim.Replaced(&m.Boxes[i], m.Movers) {
				static = append(static, m.Boxes[i])
			}
		}
		bad := 0
		for k := range 64 {
			tm := float64(k) * 0.6 / 64 // a tread period (0.45 m at 0.75 m/s) and more
			for _, d := range sim.DrawnMoverBoxes(m.Movers, tm) {
				for _, s := range static {
					if ax, ok := coplanar(d, s, 0.001); ok && bad < 20 {
						bad++
						t.Errorf("%s t=%.3f: tread %v-%v and %s box %v-%v share a %c face", n, tm, d.Min, d.Max, s.Kind, s.Min, s.Max, "xyz"[ax])
					}
				}
			}
		}
	}
}
