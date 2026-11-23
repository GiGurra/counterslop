package game

import (
	"math"
	"math/rand"
	"testing"

	"github.com/gigurra/counterslop/internal/content/maps"
)

// TestMapClearMatchesMapDistance: mapClear (the line-of-sight fast path) says
// the same as mapDistance >= limit for random segments on every map, including
// ones that end just short of, on, or just past a wall.
func TestMapClearMatchesMapDistance(t *testing.T) {
	for name, build := range maps.All {
		m := build()
		lo, hi := Vec3{math.Inf(1), math.Inf(1), math.Inf(1)}, Vec3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
		for _, b := range m.Boxes {
			for a := 0; a < 3; a++ {
				lo[a], hi[a] = math.Min(lo[a], b.Min[a]), math.Max(hi[a], b.Max[a])
			}
		}
		r := rand.New(rand.NewSource(1))
		pt := func() Vec3 {
			return Vec3{lo[0] + r.Float64()*(hi[0]-lo[0]), lo[1] + r.Float64()*(hi[1]-lo[1]+2), lo[2] + r.Float64()*(hi[2]-lo[2])}
		}
		for i := 0; i < 2000; i++ {
			o, e := pt(), pt()
			d := Vec3{e[0] - o[0], e[1] - o[1], e[2] - o[2]}
			l := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
			if i%4 == 0 { // axis-aligned rays (the zero-direction branch)
				d[1], d[i/4%2*2] = 0, 0
				l = math.Sqrt(d[0]*d[0] + d[2]*d[2])
			}
			if l < 1e-6 {
				continue
			}
			d = Vec3{d[0] / l, d[1] / l, d[2] / l}
			hit := mapDistance(&m, o, d)
			limits := []float64{l, l - 0.2, 0, -1}
			if !math.IsInf(hit, 1) {
				limits = append(limits, hit, math.Nextafter(hit, 0), math.Nextafter(hit, math.Inf(1)), hit+0.5)
			}
			for _, lim := range limits {
				if got, want := mapClear(&m, o, d, lim), hit >= lim; got != want {
					t.Fatalf("%s: mapClear(%v, %v, %v) = %v, mapDistance %v", name, o, d, lim, got, hit)
				}
			}
		}
	}
}
