package prefabs

import (
	"math"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// top: the highest solid at (x, z), and whether any box covers it.
func top(boxes []c.Box, x, z float64) (float64, bool) {
	h, any := math.Inf(-1), false
	for _, b := range boxes {
		if x > b.Min[0] && x < b.Max[0] && z > b.Min[2] && z < b.Max[2] {
			h, any = math.Max(h, b.Max[1]), true
		}
	}
	return h, any
}

func TestEscalator(t *testing.T) {
	// Rising 5 m to a floor edge at z 10, descending towards -z, treads x 0..1.2.
	e := Escalator(true, 10, 0, 1.2, 0, 5, -1, c.Metal, c.Desk)
	if l := EscalatorLen(5); math.Abs(l-9) > 1e-9 {
		t.Fatalf("length %.2f, want 9", l)
	}
	prev := 0.0
	for z := 10 - EscalatorLen(5) + EscRun/2; z < 10; z += EscRun {
		h, _ := top(e, 0.6, z)
		if h-prev > EscRise+1e-9 || h <= prev {
			t.Fatalf("tread at z %.2f is %.2f after %.2f: want steps of %.2f", z, h, prev, EscRise)
		}
		prev = h
	}
	if math.Abs(prev-5) > 1e-9 {
		t.Fatalf("top tread %.2f, want 5", prev)
	}
	// Balustrades on both sides stand 1 m above the treads beside them.
	for _, x := range []float64{-EscRail / 2, 1.2 + EscRail/2} {
		for _, z := range []float64{1.2, 5, 9.8} {
			h, _ := top(e, x, z)
			tread, _ := top(e, 0.6, z)
			if math.Abs(h-tread-1) > 1e-9 {
				t.Fatalf("balustrade at x %.2f z %.2f: %.2f over a %.2f tread", x, z, h, tread)
			}
		}
	}
}

func TestStairwell(t *testing.T) {
	s := Stairwell(0, 0, []float64{-5, 0, 5}, []bool{true, false, true}, 5, c.Concrete, c.Plaster)
	// West landings at each floor above the bottom, mid landings halfway.
	for _, w := range []struct{ x, z, y float64 }{{1, 3, 0}, {1, 3, 5}, {8, 3, -2.5}, {8, 3, 2.5}} {
		found := false
		for _, b := range s {
			if b.Max[1] == w.y && w.x > b.Min[0] && w.x < b.Max[0] && w.z > b.Min[2] && w.z < b.Max[2] {
				found = true
			}
		}
		if !found {
			t.Errorf("no landing at %v", w)
		}
	}
	// Each flight's steps rise at most 0.5 (walkable): lane A rises east,
	// lane B rises west (the top storey's flights, seen from above).
	for _, lane := range []struct{ z, dir float64 }{{2, 1}, {5.5, -1}} {
		prev := math.NaN()
		for i := range 5 {
			x := 2.6 + 0.8*float64(i)
			if lane.dir < 0 {
				x = 5.8 - 0.8*float64(i)
			}
			h, ok := top(s, x, lane.z)
			if !ok {
				t.Fatalf("no step at x %.1f in lane z %.1f", x, lane.z)
			}
			if !math.IsNaN(prev) && (h <= prev || h-prev > 0.5+1e-9) {
				t.Fatalf("lane z %.1f: step %.2f after %.2f", lane.z, h, prev)
			}
			prev = h
		}
	}
	// A door at the bottom and top levels, none at the middle.
	door := func(y float64) bool {
		for _, b := range s {
			if b.Min[2] == 0 && b.Max[2] == 0.2 && 1.2 > b.Min[0] && 1.2 < b.Max[0] && y+1 > b.Min[1] && y+1 < b.Max[1] {
				return false
			}
		}
		return true
	}
	if !door(-5) || door(0) || !door(5) {
		t.Fatalf("doors at -5/0/5: %v %v %v, want true false true", door(-5), door(0), door(5))
	}
}

func TestCatwalkAndRoof(t *testing.T) {
	cw := Catwalk(-10, 0, 10, 2, 5, c.Metal, c.Metal)
	if h, _ := top(cw, 0, 1); h != 5 {
		t.Fatalf("deck %.2f, want 5", h)
	}
	if h, _ := top(cw, 0, 0.05); h != 6 {
		t.Fatalf("railing %.2f, want 6", h)
	}
	r := Roof(mapgen.Rect{X0: -10, Z0: -10, X1: 10, Z1: 10}, []mapgen.Rect{{X0: -2, Z0: -2, X1: 2, Z1: 2}}, 10, 0.5, c.Concrete)
	if _, ok := top(r, 0, 0); ok {
		t.Fatal("the skylight is roofed over")
	}
	if h, ok := top(r, 5, 5); !ok || h != 10.5 {
		t.Fatalf("roof at (5, 5): %.2f %v", h, ok)
	}
}
