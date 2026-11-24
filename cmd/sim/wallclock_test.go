package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Tests don't depend on real-time clocks (the operator's rule; TESTING.md):
// the server ticks when the harness steps it and every page runs on a virtual
// clock. What is left on the wall clock in the harness (cmd/sim) and the
// page's test API (web/game/testapi.js) is a hang guard, a report of how long
// something took, the browser's own work (a shader compile, a reload) or a
// realtime-only scenario, and says so on the line: a "wall:" comment. A new
// real-time wait without one fails here.
func TestWallClockOnlyWhereMarked(t *testing.T) {
	check := func(path string, re *regexp.Regexp) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, l := range strings.Split(string(b), "\n") {
			if re.MatchString(l) && !strings.Contains(l, "wall:") {
				t.Errorf("%s:%d: real time without a \"wall:\" reason (hang guard, report, browser work, realtime only), or use game time: %s", path, i+1, strings.TrimSpace(l))
			}
		}
	}
	goWall := regexp.MustCompile(`\btime\.(Now|Sleep|After|Since|Until|NewTicker|NewTimer|Tick|AfterFunc)\(`)
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			check(f, goWall)
		}
	}
	// testapi.js: wallNow and wallSleep are its real clock; a bare setTimeout /
	// setInterval is the real one there too (globalThis.setTimeout is the
	// page's, virtual in lockstep).
	check("../../web/game/testapi.js", regexp.MustCompile(`\bwallNow\(|\bwallSleep\(|(^|[^.\w])set(Timeout|Interval)\(`))
}
