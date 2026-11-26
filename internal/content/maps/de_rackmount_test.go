package maps

import (
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

// de_rackmount is built for the Gravity Override: its Rules sell it, its Cut
// takes off only the ceiling board (no cover hidden from a view from above),
// and the drive sleds slide clear of every static box (never into a wall or a
// RAM stick: a body on the panel is shoved by them, never crushed against one).
func TestDeRackmount(t *testing.T) {
	m := DeRackmount()
	if m.Rules["mp_gravityboots"] != "1" {
		t.Fatalf("rules %v: the boots are the point", m.Rules)
	}
	if m.Cut == nil {
		t.Fatal("no cut: the preview would show a closed box")
	}
	var lid []c.Box
	for _, b := range m.Boxes {
		if b.Min[1] >= *m.Cut {
			lid = append(lid, b)
		}
	}
	if len(lid) != 1 || lid[0].Kind != c.PCB {
		t.Fatalf("lid %v: want the ceiling board alone", lid)
	}
	if len(m.Movers) < 4 {
		t.Fatalf("%d sleds", len(m.Movers))
	}
	for i, mv := range m.Movers {
		if mv.Kind != c.MoverSlide || mv.Period <= 0 {
			t.Fatalf("mover %d: %+v", i, mv)
		}
		// the whole stroke: from Min..Max to Min+Move..Max+Move
		var lo, hi [3]float64
		for a := range 3 {
			lo[a], hi[a] = min(mv.Min[a], mv.Min[a]+mv.Move[a]), max(mv.Max[a], mv.Max[a]+mv.Move[a])
		}
		for _, b := range m.Boxes {
			if lo[0] < b.Max[0] && hi[0] > b.Min[0] && lo[1] < b.Max[1] && hi[1] > b.Min[1] && lo[2] < b.Max[2] && hi[2] > b.Min[2] {
				t.Errorf("sled %d's stroke %v-%v runs into %v-%v (%s)", i, lo, hi, b.Min, b.Max, b.Kind)
			}
		}
		if lo[1] < 3.5 {
			t.Errorf("sled %d at y %v: over the floor's heads, please", i, lo[1])
		}
	}
}
