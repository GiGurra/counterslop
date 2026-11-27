package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The sharded cases jobs (CI: -cases -shard i/n per codec) each write their
// part; -cases-merge does the registry check over them all, and must notice a
// case that ran in no shard or in two, and a shard without the registry.
func TestMergeCaseResults(t *testing.T) {
	dir := t.TempDir()
	prev := *casesDir
	*casesDir = filepath.Join(dir, "cases")
	defer func() { *casesDir = prev }()
	_ = os.MkdirAll(*casesDir, 0o755)
	for _, n := range []string{"a", "b", "c"} {
		_ = os.WriteFile(filepath.Join(*casesDir, n+".json"), []byte(`{"name":"`+n+`"}`), 0o644)
	}
	_ = os.WriteFile(filepath.Join(*casesDir, "go-only.json"), []byte(`{"name":"g","levels":["go"]}`), 0o644)
	reg := registry{Actions: []string{"+attack"}}
	part := func(name string, r registry, cases ...string) string {
		var res []caseResult
		for _, c := range cases {
			res = append(res, caseResult{Name: c})
		}
		p := filepath.Join(dir, name)
		writeCaseResults(p, r, res)
		return p
	}
	p1, p2 := part("1.json", reg, "a", "c"), part("2.json", reg, "b")
	res, got, _, problems := mergeCaseResults([]string{p1, p2})
	if len(problems) > 0 || len(res) != 3 || res[0].Name != "a" || res[2].Name != "c" || len(got.entries()) != 1 {
		t.Fatalf("a clean split: %v problems, %d results, registry %v", problems, len(res), got.entries())
	}
	_, _, _, problems = mergeCaseResults([]string{p1})
	if len(problems) != 1 || !strings.Contains(problems[0], "case b ran in no shard") {
		t.Errorf("a missing case: %v", problems)
	}
	_, _, _, problems = mergeCaseResults([]string{p1, p2, part("3.json", reg, "a")})
	if len(problems) != 1 || !strings.Contains(problems[0], "case a ran in two shards") {
		t.Errorf("a doubled case: %v", problems)
	}
	_, _, _, problems = mergeCaseResults([]string{p1, part("4.json", registry{}, "b")})
	if len(problems) != 1 || !strings.Contains(problems[0], "no registry") {
		t.Errorf("a shard without the registry: %v", problems)
	}
	_, _, _, problems = mergeCaseResults([]string{p1, part("5.json", registry{Actions: []string{"+jump"}}, "b")})
	if len(problems) != 1 || !strings.Contains(problems[0], "different registry") {
		t.Errorf("a shard with another registry: %v", problems)
	}
}
