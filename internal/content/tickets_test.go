package content

import (
	"strings"
	"testing"
)

func cqMap() Map {
	return Map{Name: "cq_test", Mode: ModeTickets, Category: Conquest, Points: []CapturePoint{
		{ID: "earth", Pos: [3]float64{-40, 0, 0}, Radius: 6, Owner: Vibe, Locked: true, Spawn: true, Spawns: [][3]float64{{-40, 0, 2}}},
		{ID: "A", Pos: [3]float64{0, 0, 0}, Radius: 5, Spawn: true, Spawns: [][3]float64{{1, 0, 0}}},
		{ID: "moon", Pos: [3]float64{40, 0, 0}, Radius: 6, Owner: Artisan, Locked: true, Spawn: true, Spawns: [][3]float64{{40, 0, -2}}},
	}}
}

func TestTicketModeProblems(t *testing.T) {
	if p := cqMap().ModeProblems(); len(p) != 0 {
		t.Fatalf("a valid tickets map: %v", p)
	}
	lab := cqMap()
	lab.Name, lab.Category = "lab_cq", Training
	if p := lab.ModeProblems(); len(p) != 0 {
		t.Fatalf("a tickets lab: %v", p)
	}
	for _, c := range []struct {
		want string
		edit func(*Map)
	}{
		{"named cq_", func(m *Map) { m.Name = "de_x" }},
		{"category", func(m *Map) { m.Category = Competitive }},
		{"no bomb sites", func(m *Map) { m.Sites = []Zone{{Name: "A"}} }},
		{"taken", func(m *Map) { m.Points[1].ID = "earth" }},
		{"radius", func(m *Map) { m.Points[1].Radius = 0 }},
		{"not a team", func(m *Map) { m.Points[1].Owner = "mars" }},
		{"a team's main", func(m *Map) { m.Points[0].Spawn = false }},
		{"mains for artisan", func(m *Map) { m.Points[2].Locked = false }},
		{"no capturable point", func(m *Map) { m.Points = append(m.Points[:1], m.Points[2]) }},
		{"without spawn spots", func(m *Map) { m.Points[1].Spawns = nil }},
		{"outside it", func(m *Map) { m.Points[1].Spawns = [][3]float64{{9, 0, 0}} }},
	} {
		m := cqMap()
		m.Points = append([]CapturePoint(nil), m.Points...)
		c.edit(&m)
		if p := strings.Join(m.ModeProblems(), "; "); !strings.Contains(p, c.want) {
			t.Errorf("want a problem with %q, got %q", c.want, p)
		}
	}
	de := Map{Name: "de_x", Category: Competitive, Sites: []Zone{{Name: "A"}, {Name: "B"}}, Points: cqMap().Points}
	if p := strings.Join(de.ModeProblems(), "; "); !strings.Contains(p, "capture points") {
		t.Errorf("defusal map with points: %q", p)
	}
}

func TestPointInAndObjectives(t *testing.T) {
	p := CapturePoint{ID: "A", Pos: [3]float64{10, 2, 0}, Radius: 5}
	for _, c := range []struct {
		at [3]float64
		in bool
	}{{[3]float64{10, 2, 0}, true}, {[3]float64{14.9, 2, 0}, true}, {[3]float64{14, 2, 4}, false},
		{[3]float64{10, 5, 0}, true}, {[3]float64{10, 5.1, 0}, false}, {[3]float64{10, -1.1, 0}, false}} {
		if got := p.In(c.at); got != c.in {
			t.Errorf("In(%v) = %v", c.at, got)
		}
	}
	if p.Label() != "A" || (CapturePoint{ID: "A", Name: "Crater"}).Label() != "Crater" {
		t.Error("Label")
	}
	o := cqMap().Objectives()
	if len(o) != 1 || o[0].Name != "A" || !cqMap().Points[1].In(o[0].Min) || !cqMap().Points[1].In(o[0].Center()) {
		t.Fatalf("objectives %+v: the capturable point only, inside it", o)
	}
}
