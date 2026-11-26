package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"
)

// A heavy scenario ("callTimeout") gets its own hang guard on each call, says
// why, and runs alone (not beside other scenarios' browsers); the rest keep
// the default 60 s guard. The perf benches on gs_megabyte timed out in the
// nightly at 60 s under software GL with three other browsers beside them.
func TestCallTimeout(t *testing.T) {
	d := t.TempDir()
	old := *dir
	*dir = d
	defer func() { *dir = old }()
	write := func(name, js string) {
		if err := os.WriteFile(filepath.Join(d, name+".json"), []byte(js), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("plain", `{"description": "x", "steps": []}`)
	write("heavy", `{"description": "x", "callTimeout": 240, "callTimeoutReason": "slow GL", "steps": []}`)
	write("heavy-rt", `{"description": "x", "realtime": true, "callTimeout": 240, "callTimeoutReason": "slow GL", "steps": []}`)
	write("unexplained", `{"description": "x", "callTimeout": 240, "steps": []}`)

	if _, err := loadScenario("unexplained"); err == nil {
		t.Error("callTimeout without callTimeoutReason loaded")
	}
	lock, alone, real := phases([]string{"plain", "heavy", "heavy-rt", "unexplained"}, false)
	if !slices.Equal(lock, []string{"plain", "unexplained"}) || !slices.Equal(alone, []string{"heavy"}) || !slices.Equal(real, []string{"heavy-rt"}) {
		t.Errorf("phases: lock %v alone %v real %v", lock, alone, real)
	}

	b := &browser{}
	if b.guard() != cdpTimeout {
		t.Errorf("default guard %v, want %v", b.guard(), cdpTimeout)
	}
	b.callTimeout = 240 * time.Second
	if b.guard() != 240*time.Second {
		t.Errorf("guard %v, want 240s", b.guard())
	}
}

// The renderer benches on gs_megabyte take minutes of software GL on a CI
// runner: every scenario that benches says so with a callTimeout.
func TestBenchesHaveCallTimeout(t *testing.T) {
	bench := regexp.MustCompile(`"call":\s*"(bench|sprayLoad|profile)"`)
	files, _ := filepath.Glob("../../sim/scenarios/*.json")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		name := filepath.Base(f)
		var sc Scenario
		if err := json.Unmarshal(b, &sc); err != nil {
			t.Fatal(err)
		}
		if bench.Match(b) && (sc.CallTimeout == 0 || sc.CallTimeoutReason == "") {
			t.Errorf("%s benches the renderer but has no callTimeout (see Scenario.CallTimeout)", name)
		}
	}
}
