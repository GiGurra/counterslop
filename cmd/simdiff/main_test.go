package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// An older base that stops at the first case it can't load (a field it
// doesn't know: here "bad") loses every case after it in that run; run on
// its own, each of those still gets a trace, and only the case it can't
// load stays without one ("the base can't run it").
func TestRetryAlone(t *testing.T) {
	tmp := t.TempDir()
	tree, cases, out := filepath.Join(tmp, "tree"), filepath.Join(tmp, "cases"), filepath.Join(tmp, "trace")
	for _, d := range []string{filepath.Join(tree, "internal/game"), cases} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(tree, "internal/game/cases_test.go"), nil, 0o644)
	for name, body := range map[string]string{"a": `{}`, "b": `{"bad": true}`, "c": `{}`, "d": `{}`} {
		os.WriteFile(filepath.Join(cases, name+".json"), []byte(body), 0o644)
	}
	runs := 0
	oldBase := func(dir, cs, out, level string) { // the base: stops at a case it can't load
		runs++
		files, _ := filepath.Glob(filepath.Join(cs, "*.json"))
		sort.Strings(files)
		for _, f := range files {
			b, _ := os.ReadFile(f)
			if strings.Contains(string(b), "bad") {
				return
			}
			tr, _ := json.Marshal(trace{{"alice": {"pos": level}}})
			os.WriteFile(filepath.Join(out, strings.TrimSuffix(filepath.Base(f), ".json")+"."+level+".json"), tr, 0o644)
		}
	}
	head := map[string]trace{}
	for _, n := range []string{"a", "b", "c", "d"} {
		head[n+".go"], head[n+".js"] = trace{}, trace{}
	}
	base := traces(tree, cases, out, oldBase)
	if len(base) != 2 || base["c.go"] != nil { // the bug: c and d lost to b
		t.Fatalf("the batch run: %v", keys(base))
	}
	base = retryAlone(tree, cases, out, filepath.Join(tmp, "alone"), base, head, oldBase)
	if got := keys(base); strings.Join(got, " ") != "a.go a.js c.go c.js d.go d.js" {
		t.Fatalf("after the retry: %v", got)
	}
	if runs != 2+6 { // the batch at two levels, then b, c, d alone at each
		t.Fatalf("%d runs", runs)
	}
	if again := retryAlone(tree, cases, out, filepath.Join(tmp, "alone2"), map[string]trace{"a.go": {}}, map[string]trace{"a.go": {}}, oldBase); len(again) != 1 || runs != 8 {
		t.Fatal("nothing missing: nothing to run")
	}
	if traces(filepath.Join(tmp, "nowhere"), cases, out, oldBase) != nil {
		t.Fatal("a tree without cases_test.go has traces")
	}
}

func keys(m map[string]trace) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
