package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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

// Lockstep is the default; "realtime": true is only for scenarios whose
// subject is wall time itself: frame pacing and the loop's own timing, render
// benches (on the virtual clock every frame takes 0 ms), audio, the latency
// readout (GPU fences, input-to-photon). The live loop (__cs.live and the
// probes) and fake link faults (net_fake*) run on the virtual clock in
// lockstep, so they are not wall time.
func TestRealtimeOnlyForWallTime(t *testing.T) {
	wall := regexp.MustCompile(`"call":\s*"(bench|sprayLoad)"|"(pacedTaps|expectPacing|audio[A-Za-z]*)"|"latency\.`)
	files, _ := filepath.Glob("../../sim/scenarios/*.json")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		var sc Scenario
		if err := json.Unmarshal(b, &sc); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		uses := wall.FindString(string(b))
		switch {
		case uses != "" && !sc.Realtime:
			t.Errorf("%s uses %s (wall time) but runs lockstep: add \"realtime\": true", filepath.Base(f), uses)
		case uses == "" && sc.Realtime:
			t.Errorf("%s is realtime but uses nothing wall-time: let it run lockstep (deterministic)", filepath.Base(f))
		}
	}
}
