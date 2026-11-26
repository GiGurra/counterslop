package maps

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// Every map must stay exactly what clients have been playing (testdata/<map>.json;
// de_slop's is the /map.json from before the prefab refactor, plus sites, buy
// zones and nav). Regenerate deliberately when a map is meant to change:
// UPDATE=1 go test ./internal/content/maps. lab_slopes pins the wedges'
// wire shape (Box.Ramp).
func TestMapsGolden(t *testing.T) {
	for _, name := range append(slices.Clone(Order), "lab_bhop", "lab_slopes", "lab_boots", "lab_harbor", "lab_kart") { // lab_bhop: sim/cases play on it (web/game/casemap.mjs)
		got, _ := json.MarshalIndent(Find(name)(), "", " ")
		file := "testdata/" + name + ".json"
		if os.Getenv("UPDATE") != "" {
			_ = os.WriteFile(file, append(got, '\n'), 0o644)
		}
		want, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var a, b any
		_ = json.Unmarshal(got, &a)
		_ = json.Unmarshal(want, &b)
		ga, _ := json.Marshal(a)
		gb, _ := json.Marshal(b)
		if !bytes.Equal(ga, gb) {
			t.Errorf("%s changed (UPDATE=1 go test ./internal/content/maps if on purpose):\n got %s\nwant %s", name, ga, gb)
		}
	}
}
