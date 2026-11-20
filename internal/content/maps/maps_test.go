package maps

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// de_slop must stay exactly what clients have been playing (testdata is the
// /map.json from before the prefab refactor). Regenerate deliberately when
// the map is meant to change: UPDATE=1 go test ./internal/content/maps
func TestDeSlopGolden(t *testing.T) {
	got, _ := json.MarshalIndent(DeSlop(), "", " ")
	if os.Getenv("UPDATE") != "" {
		_ = os.WriteFile("testdata/de_slop.json", append(got, '\n'), 0o644)
	}
	want, err := os.ReadFile("testdata/de_slop.json")
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	_ = json.Unmarshal(got, &a)
	_ = json.Unmarshal(want, &b)
	ga, _ := json.Marshal(a)
	gb, _ := json.Marshal(b)
	if !bytes.Equal(ga, gb) {
		t.Fatalf("de_slop changed:\n got %s\nwant %s", ga, gb)
	}
}
