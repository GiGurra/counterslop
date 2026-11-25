package maps

import (
	"encoding/json"
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
		for i, l := range m.Lights {
			if err := l.Check(); err != nil {
				t.Errorf("%s light %d: %v", name, i, err)
			}
		}
		for _, p := range m.ModeProblems() {
			t.Errorf("%s: %s", name, p)
		}
		for _, g := range m.GPUs {
			gb := c.GPUBox(g)
			for i, b := range m.Boxes {
				if gb.Min[0] < b.Max[0] && gb.Max[0] > b.Min[0] && gb.Min[1] < b.Max[1] && gb.Max[1] > b.Min[1] && gb.Min[2] < b.Max[2] && gb.Max[2] > b.Min[2] {
					t.Errorf("%s: the GPU at %v overlaps box %d %v-%v", name, g.Pos, i, b.Min, b.Max)
				}
			}
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

// Labs are test maps: known materials, valid lights, both teams' spawns clear of
// boxes and inside buy zones, deterministic, and never named like a playable map.
func TestLabsAreValid(t *testing.T) {
	known := map[string]bool{}
	for _, m := range c.Materials {
		known[m] = true
	}
	for name, build := range Labs {
		if All[name] != nil {
			t.Errorf("lab %s is also in All", name)
		}
		m := build()
		if m.Name != name {
			t.Errorf("lab %q is named %q", name, m.Name)
		}
		for i, b := range m.Boxes {
			if !known[b.Kind] {
				t.Errorf("%s box %d: unknown material %q", name, i, b.Kind)
			}
		}
		for i, l := range m.Lights {
			if err := l.Check(); err != nil {
				t.Errorf("%s light %d: %v", name, i, err)
			}
		}
		for _, team := range c.Teams {
			if len(m.TeamSpawns[team]) == 0 {
				t.Errorf("%s: no %s spawns", name, team)
			}
			for _, s := range m.TeamSpawns[team] {
				in := false
				for _, z := range m.BuyZones[team] {
					in = in || z.Contains(s)
				}
				if !in {
					t.Errorf("%s: %s spawn %v outside its buy zones", name, team, s)
				}
				for _, b := range m.Boxes {
					if s[0]+0.4 > b.Min[0] && s[0]-0.4 < b.Max[0] && s[2]+0.4 > b.Min[2] && s[2]-0.4 < b.Max[2] && s[1] < b.Max[1] && s[1]+1.8 > b.Min[1] {
						t.Errorf("%s: spawn %v inside box %+v", name, s, b)
					}
				}
			}
		}
		a, _ := json.Marshal(m)
		b, _ := json.Marshal(build())
		if string(a) != string(b) {
			t.Errorf("%s is not deterministic", name)
		}
	}
	if !slices.Equal(LabNames(), []string{"lab_bhop", "lab_lights", "lab_slopes", "lab_store"}) {
		t.Errorf("LabNames %v", LabNames())
	}
	if Find("lab_lights") == nil || Find("de_slop") == nil || Find("nope") != nil {
		t.Error("Find")
	}
}

// A map pins only the cvars it may (content.RuleCvars), and the values are ones
// the server takes (internal/game TestMapRulesAreValidCvars checks them
// against the cvars' own validation).
func TestMapRulesAreRuleCvars(t *testing.T) {
	names := append(slices.Clone(Order), LabNames()...)
	for _, name := range names {
		for k := range Find(name)().Rules {
			if !slices.Contains(c.RuleCvars, k) {
				t.Errorf("%s pins %q, not one of content.RuleCvars %v", name, k, c.RuleCvars)
			}
		}
	}
	if r := LabBhop().Rules; r["sv_movement"] != "source" || r["sv_autobunnyhopping"] != "1" {
		t.Errorf("lab_bhop rules %v", r)
	}
}
