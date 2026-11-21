package main

import "testing"

// A caseState without connected or seq made lockstep skip WaitCommand and
// step the server without the client's command (tk_2dc008e7: "tick 1
// onGround: twin sims disagree" from a broken caseState). It must be an error.
func TestCaseStateKeys(t *testing.T) {
	for _, c := range []struct {
		st map[string]any
		ok bool
	}{
		{map[string]any{"connected": true, "seq": 3.0}, true},
		{map[string]any{"connected": false, "seq": 0.0}, true},
		{map[string]any{"seq": 3.0, "shout": ""}, false},
		{map[string]any{"connected": true}, false},
	} {
		if err := caseStateKeys(c.st); (err == nil) != c.ok {
			t.Errorf("%v: err %v, want ok=%v", c.st, err, c.ok)
		}
	}
}
