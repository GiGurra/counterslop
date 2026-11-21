package content

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestB(t *testing.T) {
	b := B(1, 2, 3, 4, 5, 6, Crate)
	if b.Min != [3]float64{1, 2, 3} || b.Max != [3]float64{4, 5, 6} || b.Kind != "crate" {
		t.Fatalf("B: %+v", b)
	}
	j, _ := json.Marshal(b)
	if string(j) != `{"min":[1,2,3],"max":[4,5,6],"kind":"crate"}` {
		t.Fatalf("json %s", j)
	}
}

// Every material name the server uses has a client material file.
func TestMaterialsExistOnTheClient(t *testing.T) {
	for _, m := range []string{Sand, Brick, Crate, Hazard} {
		if _, err := os.Stat(filepath.Join("../../web/assets/materials", m+".js")); err != nil {
			t.Errorf("material %q: %v", m, err)
		}
	}
}

func TestTeams(t *testing.T) {
	if !slices.Equal(Teams, []string{"vibe", "artisan"}) || slices.Contains(Teams, Spectator) {
		t.Fatalf("teams %v", Teams)
	}
}

func TestMapJSONOmitsEmptyItems(t *testing.T) {
	j, _ := json.Marshal(Map{Name: "x"})
	var m map[string]any
	_ = json.Unmarshal(j, &m)
	if _, has := m["items"]; has {
		t.Fatalf("items present: %s", j)
	}
	for _, k := range []string{"name", "boxes", "spawns", "teamSpawns"} {
		if _, has := m[k]; !has {
			t.Fatalf("%s missing: %s", k, j)
		}
	}
}

// Lights serialize with the names the client reads (web/engine/lights.js), and Check
// rejects lights the renderer can't use.
func TestLights(t *testing.T) {
	s := Spot([3]float64{1, 3, 2}, [3]float64{0, -1, 0}, [3]float64{1, 0.9, 0.8}, 2, 8, 30)
	s.Shadow, s.Flicker = true, 7
	j, _ := json.Marshal(s)
	want := `{"kind":"spot","pos":[1,3,2],"color":[1,0.9,0.8],"intensity":2,"range":8,"dir":[0,-1,0],"cone":30,"shadow":true,"flicker":7}`
	if string(j) != want {
		t.Fatalf("json %s\nwant %s", j, want)
	}
	if err := s.Check(); err != nil {
		t.Fatal(err)
	}
	p := Point([3]float64{0, 2, 0}, [3]float64{0.2, 1, 0.3}, 1, 5)
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	bad := []Light{
		{Kind: "area", Range: 5, Intensity: 1},
		{Kind: PointLight, Range: 0, Intensity: 1},
		{Kind: PointLight, Range: 5, Intensity: 0},
		{Kind: PointLight, Range: 5, Intensity: 1, Color: [3]float64{2, 0, 0}},
		{Kind: SpotLight, Range: 5, Intensity: 1, Cone: 30},
		{Kind: SpotLight, Range: 5, Intensity: 1, Dir: [3]float64{0, -1, 0}, Cone: 95},
		{Kind: SpotLight, Range: 5, Intensity: 1, Dir: [3]float64{0, -1, 0}, Cone: 30, Inner: 40},
	}
	for i, l := range bad {
		if l.Check() == nil {
			t.Errorf("bad light %d passed: %+v", i, l)
		}
	}
	// A map without lights keeps its JSON (and golden files) unchanged.
	m, _ := json.Marshal(Map{Name: "x"})
	if containsKey(m, "lights") {
		t.Fatalf("empty Lights serialized: %s", m)
	}
}

func containsKey(j []byte, k string) bool {
	var v map[string]any
	_ = json.Unmarshal(j, &v)
	_, ok := v[k]
	return ok
}
