package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestVClock runs vclock.test.mjs (the virtual clock lockstep pages run on).
func TestVClock(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("no node")
	}
	out, err := exec.Command("node", "--test", "vclock.test.mjs").CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// TestVClockOnlyInLockstep: the virtual clock is never served (it lives in
// cmd/sim, not web/) and the harness injects it only into lockstep pages.
func TestVClockOnlyInLockstep(t *testing.T) {
	if strings.Contains(vclockJS, "export ") || !strings.Contains(vclockJS, "__vclock") {
		t.Fatal("vclock.js should be a classic script defining __vclock")
	}
	if got := pageInitFor(false, 1); len(got.scripts) != 0 {
		t.Fatalf("realtime page gets scripts %d", len(got.scripts))
	}
	// A lockstep page also draws nothing unless a step asks (testapi draw).
	if got := pageInitFor(true, 6); len(got.scripts) != 2 || got.scripts[0] != vclockJS || got.scripts[1] != noGLJS || got.throttle != 6 {
		t.Fatalf("lockstep page init: %+v", got)
	}
}
