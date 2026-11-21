package maps

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
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
	materials := map[string]bool{}
	for _, m := range c.Materials {
		if _, err := os.Stat("../../../web/assets/materials/" + m + ".js"); err != nil {
			t.Errorf("material %q has no web/assets/materials/%s.js", m, m)
		}
		materials[m] = true
	}
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
	if len(Order) != len(All) {
		t.Errorf("Order has %d maps, All %d", len(Order), len(All))
	}
	for _, n := range Order {
		if All[n] == nil {
			t.Errorf("Order lists unknown map %q", n)
		}
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
		var sites []string
		for _, z := range m.Sites {
			sites = append(sites, z.Name)
		}
		if fmt.Sprint(sites) != "[A B]" {
			t.Errorf("%s: sites %v, want [A B]", name, sites)
		}
		for _, team := range c.Teams {
			for _, s := range m.TeamSpawns[team] {
				in := false
				for _, z := range m.BuyZones[team] {
					in = in || z.Contains(s)
				}
				if !in {
					t.Errorf("%s: %s spawn %v is outside its buy zones", name, team, s)
				}
			}
		}
		if !slices.Contains(c.Categories, m.Category) {
			t.Errorf("%s: category %q is not one of %v (mapgen Builder.Category)", name, m.Category, c.Categories)
		}
		if m.Desc == "" || m.MaxPlayers < 2 {
			t.Errorf("%s: needs Meta (desc %q, maxPlayers %d)", name, m.Desc, m.MaxPlayers)
		}
		nodes := map[string]bool{}
		for _, n := range m.Nav {
			if nodes[n.Name] {
				t.Errorf("%s: nav node %q twice", name, n.Name)
			}
			nodes[n.Name] = true
			if solid(m, n.Pos) {
				t.Errorf("%s: nav node %s %v is inside a box", name, n.Name, n.Pos)
			}
		}
		for _, n := range m.Nav {
			for _, l := range n.Links {
				if !nodes[l] {
					t.Errorf("%s: nav node %s links to unknown %q", name, n.Name, l)
				}
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
