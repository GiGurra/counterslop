package maps

import (
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

// de_hostile_takeover: its Rules sell the Gravity Override, dinghies float at
// the quay (Boats on by map), and the accommodation ladder takes a boarding
// party from its pontoon at the waterline up to the deck at B with the real
// movement code (the pier route to the sites is TestWalkToSites').
func TestDeHostileTakeover(t *testing.T) {
	m := DeHostileTakeover()
	if m.Rules["mp_gravityboots"] != "1" {
		t.Fatalf("rules %v: hooking the hull is the point", m.Rules)
	}
	if len(m.Boats) < 3 || len(m.Water) != 1 {
		t.Fatalf("%d boats, %d water volumes", len(m.Boats), len(m.Water))
	}
	for i, bt := range m.Boats {
		if bt.Pos[1] != m.Water[0].Max[1] {
			t.Errorf("boat %d at y %v: not on the surface %v", i, bt.Pos[1], m.Water[0].Max[1])
		}
	}
	up := []c.NavNode{{Name: "foot", Pos: [3]float64{5, 0, -9.2}}, {Name: "top", Pos: [3]float64{19.5, 8, -9.2}}, {Name: "deck", Pos: [3]float64{19.5, 8, -13}}}
	end, err := walk(m, [3]float64{2, 0, -9}, up)
	if err != nil {
		t.Fatal(err)
	}
	if end[1] < 7.9 || !m.Sites[1].Contains(end) {
		t.Fatalf("up the ladder: ended at %v, not on the deck at B", end)
	}
}
