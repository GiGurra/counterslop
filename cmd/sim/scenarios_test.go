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

// Lockstep is the default; "realtime": true is for scenarios whose subject is
// wall time (a live client loop, frame pacing, audio, fake lag or packet
// faults), and those must say so: in lockstep a live loop has no server
// ticking under it and a dropped command stalls the run.
func TestRealtimeOnlyForWallTime(t *testing.T) {
	// "live" only as the call (the round phase "live" is lockstep-safe), the
	// pacing calls, the audio state and calls, and the fake-network cvars.
	wall := regexp.MustCompile(`"call":\s*"live"|"(pacedTaps|expectPacing|audio[A-Za-z]*)"|net_fake(lag|loss|dup|jitter)`)
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
