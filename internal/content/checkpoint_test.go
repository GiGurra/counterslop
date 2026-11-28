package content

import "testing"

// Crossed: the straight path a -> b touches the gate, however thin the gate
// and however long the step; a point is In.
func TestCheckpointCrossed(t *testing.T) {
	g := Checkpoint{Min: [3]float64{-1, -1, 0}, Max: [3]float64{1, 6, 24}}
	for _, c := range []struct {
		a, b [3]float64
		want bool
	}{
		{[3]float64{4, 0, 9}, [3]float64{-6, 0, 9}, true},    // right through, both ends outside
		{[3]float64{-6, 0, 9}, [3]float64{4, 0, 9}, true},    // backwards too (order is the race's business)
		{[3]float64{4, 0, 9}, [3]float64{2, 0, 9}, false},    // short of it
		{[3]float64{4, 0, 30}, [3]float64{-6, 0, 30}, false}, // beside it
		{[3]float64{4, 0, -1}, [3]float64{-4, 0, 2}, true},   // diagonally into its corner
		{[3]float64{4, 0, -3}, [3]float64{-4, 0, -1}, false}, // diagonally past its corner
		{[3]float64{4, 8, 9}, [3]float64{-6, 8, 9}, false},   // over it
		{[3]float64{0, 0, 9}, [3]float64{0, 0, 9}, true},     // a point inside: In
		{[3]float64{3, 0, 9}, [3]float64{3, 0, 9}, false},    // a point outside
		{[3]float64{0.5, 0, 9}, [3]float64{9, 0, 9}, true},   // leaving it
	} {
		if got := g.Crossed(c.a, c.b); got != c.want {
			t.Errorf("%v -> %v: %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
