package main

import (
	"strings"
	"testing"
	"time"
)

// A stuck case fails (hang guard) and its browser is killed, which ends it;
// one that finishes in time comes back as it is.
func TestWithDeadline(t *testing.T) {
	if got := withDeadline(time.Second, time.Second, func() []string { return []string{"x"} }, func() { t.Error("killed a case that finished") }); len(got) != 1 || got[0] != "x" {
		t.Fatalf("in time: %v", got)
	}
	stop := make(chan struct{})
	killed := false
	got := withDeadline(20*time.Millisecond, time.Second, func() []string { <-stop; return []string{"page gone"} }, func() { killed = true; close(stop) })
	if !killed || len(got) != 2 || !strings.HasPrefix(got[0], "hung:") || got[1] != "page gone" {
		t.Fatalf("stuck: killed %v, fails %v", killed, got)
	}
}
