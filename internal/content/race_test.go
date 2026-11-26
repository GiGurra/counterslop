package content

import (
	"strings"
	"testing"
)

// rcMap: a minimal valid race map.
func rcMap() Map {
	gate := func(x float64) Checkpoint {
		return Checkpoint{Name: "g", Min: [3]float64{x, -1, -6}, Max: [3]float64{x + 2, 5, 6}, Spawn: [3]float64{x - 4, 0, 0}}
	}
	return Map{Name: "rc_x", Mode: ModeRace, Category: Racing,
		Checkpoints: []Checkpoint{gate(0), gate(40), gate(80)},
		Cars:        []CarSpawn{{Type: "buggy"}, {Type: "buggy"}},
		Pits:        []Zone{{Name: "void", Min: [3]float64{-99, -99, -99}, Max: [3]float64{99, -10, 99}}}}
}

func TestRaceModeProblems(t *testing.T) {
	if p := rcMap().ModeProblems(); len(p) != 0 {
		t.Fatalf("a valid race map: %v", p)
	}
	lab := rcMap()
	lab.Name, lab.Category = "lab_rc", Training
	if p := lab.ModeProblems(); len(p) != 0 {
		t.Fatalf("a race lab: %v", p)
	}
	for _, c := range []struct {
		want string
		edit func(*Map)
	}{
		{"named rc_", func(m *Map) { m.Name = "de_x" }},
		{"category", func(m *Map) { m.Category = Competitive }},
		{"no bomb sites", func(m *Map) { m.Sites = []Zone{{Name: "A"}} }},
		{"checkpoints", func(m *Map) { m.Checkpoints = m.Checkpoints[:2] }},
		{"empty on axis 1", func(m *Map) { m.Checkpoints[1].Max[1] = m.Checkpoints[1].Min[1] }},
		{"grid slots", func(m *Map) { m.Cars = m.Cars[:1] }},
		{"no pit", func(m *Map) { m.Pits = nil }},
	} {
		m := rcMap()
		m.Checkpoints = append([]Checkpoint(nil), m.Checkpoints...)
		c.edit(&m)
		if p := strings.Join(m.ModeProblems(), "; "); !strings.Contains(p, c.want) {
			t.Errorf("want a problem with %q, got %q", c.want, p)
		}
	}
	if !rcMap().Checkpoints[0].In([3]float64{1, 0, 0}) || rcMap().Checkpoints[0].In([3]float64{3, 0, 0}) {
		t.Fatal("Checkpoint.In")
	}
}
