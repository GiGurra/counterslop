package content

import (
	"strings"
	"testing"
)

func gsMap() Map {
	return Map{Name: "gs_test", Mode: ModeGPUDrop, Category: GPUDrop,
		GPUs: []GPUSpot{{Pos: [3]float64{0, 0, 0}}, {Pos: [3]float64{1, 0, 0}}},
		Exits: []Zone{{Name: "dock", Min: [3]float64{30, 0, 0}, Max: [3]float64{34, 3, 4}},
			{Name: "entrance", Min: [3]float64{-34, 0, 0}, Max: [3]float64{-30, 3, 4}}}}
}

func TestModeProblems(t *testing.T) {
	if p := gsMap().ModeProblems(); len(p) != 0 {
		t.Fatalf("a valid GPU Drop map: %v", p)
	}
	de := Map{Name: "de_x", Category: Competitive, Sites: []Zone{{Name: "A"}, {Name: "B"}}}
	if p := de.ModeProblems(); len(p) != 0 {
		t.Fatalf("a valid defusal map: %v", p)
	}
	for _, c := range []struct {
		want string
		edit func(*Map)
	}{
		{"no bomb sites", func(m *Map) { m.Sites = []Zone{{Name: "A"}} }},
		{"category", func(m *Map) { m.Category = Competitive }},
		{"gs_", func(m *Map) { m.Name = "de_test" }},
		{"GPUs on the shelves", func(m *Map) { m.GPUs = m.GPUs[:1] }},
		{"exits, want", func(m *Map) { m.Exits = m.Exits[:1] }},
		{"apart", func(m *Map) { m.Exits[1].Min, m.Exits[1].Max = [3]float64{36, 0, 0}, [3]float64{40, 3, 4} }},
		{"taken", func(m *Map) { m.Exits[1].Name = "dock" }},
		{"stands in exit", func(m *Map) { m.GPUs[0].Pos = [3]float64{32, 1, 2} }},
		{"unknown mode", func(m *Map) { m.Mode = "hostage" }},
	} {
		m := gsMap()
		c.edit(&m)
		if p := strings.Join(m.ModeProblems(), "; "); !strings.Contains(p, c.want) {
			t.Errorf("want a problem with %q, got %q", c.want, p)
		}
	}
	de.GPUs = gsMap().GPUs
	if p := strings.Join(de.ModeProblems(), "; "); !strings.Contains(p, "GPUs or exits") {
		t.Errorf("defusal map with GPUs: %q", p)
	}
}

func TestObjectives(t *testing.T) {
	m := gsMap()
	o := m.Objectives()
	if len(o) != 4 || o[0].Name != "gpu1" || !o[0].Contains([3]float64{1.4, 0.5, -1.4}) || o[0].Contains([3]float64{1.6, 0, 0}) || o[3].Name != "entrance" {
		t.Fatalf("objectives %+v", o)
	}
	de := Map{Sites: []Zone{{Name: "A"}, {Name: "B"}}}
	if o := de.Objectives(); len(o) != 2 || o[1].Name != "B" {
		t.Fatalf("defusal objectives %+v", o)
	}
}
