package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every sim scenario asserts something (an expect* call), not just snapshots:
// a picture nobody looks at catches nothing.
func TestEveryScenarioAsserts(t *testing.T) {
	files, _ := filepath.Glob("../../sim/scenarios/*.json")
	if len(files) == 0 {
		t.Fatal("no scenarios")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"call": "expect`) && !strings.Contains(string(b), `"call":"expect`) {
			t.Errorf("%s has no expect* step: assert what it shows (see web/game/testapi.js expectState)", filepath.Base(f))
		}
	}
}
