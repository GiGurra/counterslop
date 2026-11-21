package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every sim scenario asserts something (an expect* or waitState call), not just snapshots:
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
		js := strings.ReplaceAll(string(b), `"call":"`, `"call": "`)
		if !strings.Contains(js, `"call": "expect`) && !strings.Contains(js, `"call": "waitState"`) {
			t.Errorf("%s has no expect* / waitState step: assert what it shows (see web/game/testapi.js expectState)", filepath.Base(f))
		}
	}
}
