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
