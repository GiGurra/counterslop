package rng

import "testing"

// Values produced by web/gen/rng.js mulberry32 for the same seeds.
func TestMatchesJS(t *testing.T) {
	cases := map[uint32][]float64{
		1337: {0.1844118325971067, 0.18998925131745636, 0.8104719922412187, 0.6437488221563399, 0.430774615611881},
		0:    {0.26642920868471265, 0.0003297457005828619, 0.2232720274478197},
	}
	for seed, want := range cases {
		r := New(seed)
		for i, w := range want {
			if got := r.Float(); got != w {
				t.Fatalf("seed %d #%d: got %v, want %v", seed, i, got, w)
			}
		}
	}
}
