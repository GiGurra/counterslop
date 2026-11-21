package mapgen

import (
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

func TestBuilder(t *testing.T) {
	m := New("test").
		Add(c.B(0, 0, 0, 1, 1, 1, c.Crate), c.B(2, 0, 2, 3, 1, 3, c.Brick)).
		Add().
		TeamSpawns(c.Vibe, [3]float64{1, 0, 1}, [3]float64{2, 0, 2}).
		TeamSpawns(c.Artisan, [3]float64{-1, 0, -1}).
		TeamSpawns(c.Vibe, [3]float64{3, 0, 3}).
		Item("stub", 4, 0, 5).
		Map()
	if m.Name != "test" || len(m.Boxes) != 2 || m.Boxes[1].Kind != c.Brick {
		t.Fatalf("boxes %+v", m.Boxes)
	}
	if len(m.TeamSpawns[c.Vibe]) != 3 || len(m.TeamSpawns[c.Artisan]) != 1 {
		t.Fatalf("team spawns %+v", m.TeamSpawns)
	}
	// Team spawns are also general spawns, in the order added.
	want := [][3]float64{{1, 0, 1}, {2, 0, 2}, {-1, 0, -1}, {3, 0, 3}}
	if len(m.Spawns) != len(want) {
		t.Fatalf("spawns %v", m.Spawns)
	}
	for i := range want {
		if m.Spawns[i] != want[i] {
			t.Fatalf("spawn %d = %v, want %v", i, m.Spawns[i], want[i])
		}
	}
	if len(m.Items) != 1 || m.Items[0] != (c.Item{Weapon: "stub", Pos: [3]float64{4, 0, 5}}) {
		t.Fatalf("items %+v", m.Items)
	}
	if e := New("empty").Map(); e.TeamSpawns != nil || e.Boxes != nil {
		t.Fatalf("empty map %+v", e)
	}
}

func TestEnclosure(t *testing.T) {
	const half, h, th = 10.0, 5.0, 1.0
	boxes := Enclosure(half, h, th, c.Sand, c.Brick)
	if len(boxes) != 5 {
		t.Fatalf("%d boxes", len(boxes))
	}
	floor := boxes[0]
	if floor.Kind != c.Sand || floor.Max[1] != 0 || floor.Min != [3]float64{-half, -1, -half} || floor.Max != [3]float64{half, 0, half} {
		t.Fatalf("floor %+v", floor)
	}
	var lo, hi [3]float64
	for i, b := range boxes[1:] {
		if b.Kind != c.Brick || b.Min[1] != 0 || b.Max[1] != h {
			t.Fatalf("wall %d: %+v", i, b)
		}
		for a := range 3 {
			if b.Min[a] >= b.Max[a] {
				t.Fatalf("wall %d is inverted on axis %d: %+v", i, a, b)
			}
			lo[a], hi[a] = min(lo[a], b.Min[a]), max(hi[a], b.Max[a])
		}
		// Walls sit outside the floor: their inner face touches its edge.
		inner := b.Min[0] >= half || b.Max[0] <= -half || b.Min[2] >= half || b.Max[2] <= -half
		if !inner {
			t.Fatalf("wall %d overlaps the floor: %+v", i, b)
		}
	}
	if lo != [3]float64{-half - th, 0, -half - th} || hi != [3]float64{half + th, h, half + th} {
		t.Fatalf("walls span %v..%v", lo, hi)
	}
	// Every point just outside the floor edge is inside some wall (no gaps, corners included).
	for _, p := range [][2]float64{{0, -half - 0.5}, {0, half + 0.5}, {-half - 0.5, 0}, {half + 0.5, 0}, {half + 0.5, half + 0.5}, {-half - 0.5, -half - 0.5}} {
		in := false
		for _, b := range boxes[1:] {
			in = in || (p[0] >= b.Min[0] && p[0] <= b.Max[0] && p[1] >= b.Min[2] && p[1] <= b.Max[2])
		}
		if !in {
			t.Fatalf("gap in the walls at %v", p)
		}
	}
}

func TestNavLane(t *testing.T) {
	m := New("t").Nav("a", 0, 0, 0, "b").NavLane("b", 1, 0, 2, "c").Nav("c", 3, 0, 3).Map()
	if len(m.Nav) != 3 || m.Nav[0].Lane || !m.Nav[1].Lane || m.Nav[2].Lane {
		t.Fatalf("nav %+v", m.Nav)
	}
	if m.Nav[1].Pos != [3]float64{1, 0, 2} || len(m.Nav[1].Links) != 1 || m.Nav[1].Links[0] != "c" {
		t.Fatalf("lane node %+v", m.Nav[1])
	}
}
