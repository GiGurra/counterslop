package main

import (
	"strings"
	"testing"
	"time"
)

func TestFailRecap(t *testing.T) {
	if got := failRecap([]caseResult{{Name: "a"}, {Name: "b"}}); got != "" {
		t.Fatalf("all passed: %q", got)
	}
	long := strings.Repeat("x", 400)
	got := failRecap([]caseResult{
		{Name: "ok"},
		{Name: "afk-autobalance", Fails: []string{"tick 12: carol: command 40 never arrived (last 39)", "second"}, Took: 1500 * time.Millisecond},
		{Name: "walk", Fails: []string{long}},
	})
	for _, want := range []string{"FAILED 2 case(s): ", "afk-autobalance (1.5s): tick 12: carol: command 40 never arrived (last 39)", "walk (0s): " + strings.Repeat("x", 300) + "..."} {
		if !strings.Contains(got, want) {
			t.Fatalf("recap %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "second") || strings.Contains(got, "ok (") || strings.Contains(got, strings.Repeat("x", 301)) {
		t.Fatalf("recap %q has more than each failed case's first failure", got)
	}
}

func TestIDsInOrder(t *testing.T) {
	if err := idsInOrder([]string{"alice", "bob"}, map[string]string{"alice": "#3", "bob": "#7"}); err != nil {
		t.Fatal(err)
	}
	if idsInOrder([]string{"alice", "bob"}, map[string]string{"alice": "#7", "bob": "#3"}) == nil {
		t.Fatal("bob before alice passed")
	}
	if idsInOrder([]string{"alice"}, map[string]string{}) == nil {
		t.Fatal("a missing id passed")
	}
}
