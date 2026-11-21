package maps

import (
	"encoding/json"
	"os"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

// Every registered map uses known materials and weapons, gives each team
// spawns, and keeps spawns and items out of solid boxes.
func TestAllMapsAreValid(t *testing.T) {
	wj, err := os.ReadFile("../../../web/assets/weapons/weapons.json")
	if err != nil {
		t.Fatal(err)
	}
	var reg struct {
		Weapons []struct{ ID string } `json:"weapons"`
	}
	if err := json.Unmarshal(wj, &reg); err != nil {
		t.Fatal(err)
	}
	weapons := map[string]bool{}
	for _, w := range reg.Weapons {
		weapons[w.ID] = true
	}
	materials := map[string]bool{c.Sand: true, c.Brick: true, c.Crate: true, c.Hazard: true}
	solid := func(m c.Map, p [3]float64) bool {
		const r = 0.4 // player radius
		for _, b := range m.Boxes {
			if p[0]+r > b.Min[0] && p[0]-r < b.Max[0] && p[2]+r > b.Min[2] && p[2]-r < b.Max[2] &&
				p[1] < b.Max[1] && p[1]+1.8 > b.Min[1] {
				return true
			}
		}
		return false
	}
	for name, build := range All {
		m := build()
		if m.Name != name {
			t.Errorf("map registered as %q is named %q", name, m.Name)
		}
		if len(m.Boxes) == 0 || len(m.Spawns) == 0 {
			t.Errorf("%s: %d boxes, %d spawns", name, len(m.Boxes), len(m.Spawns))
		}
		for i, b := range m.Boxes {
			if !materials[b.Kind] {
				t.Errorf("%s box %d: unknown material %q", name, i, b.Kind)
			}
			for a := range 3 {
				if b.Min[a] >= b.Max[a] {
					t.Errorf("%s box %d empty on axis %d: %+v", name, i, a, b)
				}
			}
		}
		for _, team := range c.Teams {
			if len(m.TeamSpawns[team]) == 0 {
				t.Errorf("%s: no spawns for %s", name, team)
			}
		}
		for _, s := range m.Spawns {
			if solid(m, s) {
				t.Errorf("%s: spawn %v is inside a box", name, s)
			}
		}
		for _, it := range m.Items {
			if !weapons[it.Weapon] {
				t.Errorf("%s: item %q is not in weapons.json", name, it.Weapon)
			}
			if solid(m, it.Pos) {
				t.Errorf("%s: item at %v is inside a box", name, it.Pos)
			}
		}
		// Building a map twice gives the same map (no hidden randomness).
		a, _ := json.Marshal(m)
		b, _ := json.Marshal(build())
		if string(a) != string(b) {
			t.Errorf("%s is not deterministic", name)
		}
	}
}
