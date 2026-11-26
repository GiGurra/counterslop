package maps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

// The custom map registry (mapcheck.LoadDir fills it from files): custom maps
// are found after the built-ins, each build is its own copy, a built-in's
// name stays the built-in's, and Gen moves on every set.
func TestCustomRegistry(t *testing.T) {
	m := DeSlop()
	m.Name = "de_registry"
	g := Gen()
	SetCustom(map[string]c.Map{"de_registry": m})
	defer SetCustom(nil)
	if Gen() == g || Find("de_registry") == nil || Playable("de_registry") == nil || !IsCustom("de_registry") {
		t.Fatal("the custom map isn't found")
	}
	if IsCustom("de_slop") || IsCustom("lab_bhop") || !Builtin("lab_bhop") || Builtin("de_registry") || Playable("lab_bhop") != nil {
		t.Fatal("built-ins and labs")
	}
	if n := Names(); len(n) != len(Order)+1 || n[len(n)-1] != "de_registry" || CustomNames()[0] != "de_registry" {
		t.Fatalf("names %v", n)
	}
	a := Find("de_registry")()
	a.Boxes[0].Kind = "changed"
	if Find("de_registry")().Boxes[0].Kind == "changed" {
		t.Fatal("builds share the map")
	}
	b, err := Export(m)
	var back c.Map
	if err != nil || json.Unmarshal(b, &back) != nil || back.Name != "de_registry" {
		t.Fatalf("export: %v", err)
	}
	dir := t.TempDir()
	if Fingerprint(dir) != "" {
		t.Fatal("an empty dir has a fingerprint")
	}
	_ = os.WriteFile(filepath.Join(dir, "x.json"), b, 0o644)
	if Fingerprint(dir) == "" {
		t.Fatal("no fingerprint")
	}
}
