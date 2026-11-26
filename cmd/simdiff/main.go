// Command simdiff shows what a change does to gameplay: it runs every sim/cases
// case (the head's cases, so both sides get the same inputs) on a base ref and
// on head, at the server level (go) and the client-predictor level (js), and
// reports per case which state fields changed, from which tick, and by how much.
//
//	go run ./cmd/simdiff               base HEAD~1, head = the working tree
//	go run ./cmd/simdiff main          what the working tree changes vs main
//	go run ./cmd/simdiff a1b2c3 HEAD   two commits
//	go run ./cmd/simdiff -md out.md    also write the report as markdown (CI summary)
//
// "No behaviour change" means every case produced the same per-tick state.
// A case the base can't load (a field or slot it predates) costs only itself:
// what the base's batch run lost is run again one case at a time (retryAlone),
// and only what still has no trace is listed as "the base can't run it".
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var mdOut = flag.String("md", "", "also write the report as markdown to this file")

type trace []map[string]map[string]any

func main() {
	flag.Parse()
	base, head := "HEAD~1", ""
	if flag.NArg() > 0 {
		base = flag.Arg(0)
	}
	if flag.NArg() > 1 {
		head = flag.Arg(1)
	}
	root := strings.TrimSpace(git(".", "rev-parse", "--show-toplevel"))
	tmp, err := os.MkdirTemp("", "simdiff")
	check(err)
	cleanups = append(cleanups, func() { os.RemoveAll(tmp) })
	defer cleanup()

	headDir := root
	if head != "" {
		headDir = worktree(root, tmp, "head", head)
	}
	baseDir := worktree(root, tmp, "base", base)
	cases := filepath.Join(headDir, "sim", "cases")
	headTr := traces(headDir, cases, filepath.Join(tmp, "head-trace"), runLevel)
	baseTr := traces(baseDir, cases, filepath.Join(tmp, "base-trace"), runLevel)
	if len(baseTr) > 0 { // a case the base stopped at (one it can't load) cost only itself
		baseTr = retryAlone(baseDir, cases, filepath.Join(tmp, "base-trace"), filepath.Join(tmp, "alone"), baseTr, headTr, runLevel)
	}

	label := head
	if label == "" {
		label = "working tree"
	}
	var md strings.Builder
	fmt.Fprintf(&md, "### Behaviour diff: %s → %s\n\n", base, label)
	if len(baseTr) == 0 {
		fmt.Fprintf(&md, "The base cannot write case traces (it predates sim/cases or simdiff): nothing to compare.\n")
	}
	names := make([]string, 0, len(headTr))
	for n := range headTr {
		names = append(names, n)
	}
	sort.Strings(names)
	changed, same := 0, 0
	for _, n := range names {
		b, ok := baseTr[n]
		if !ok {
			if len(baseTr) > 0 {
				fmt.Fprintf(&md, "- `%s`: no trace on the base (the base can't run it)\n", n)
			}
			continue
		}
		rows := diff(b, headTr[n])
		if len(rows) == 0 {
			same++
			continue
		}
		changed++
		fmt.Fprintf(&md, "\n**%s** (%s)\n\n| player.field | first tick | base | head | ticks differing | max Δ |\n|---|---|---|---|---|---|\n", n, levelOf(n))
		for _, r := range rows {
			fmt.Fprintf(&md, "| %s | %d | %s | %s | %d | %s |\n", r.field, r.first, r.base, r.head, r.count, r.delta)
		}
	}
	if len(baseTr) > 0 {
		if changed == 0 {
			fmt.Fprintf(&md, "\nNo behaviour change: all %d case traces are identical.\n", same)
		} else {
			fmt.Fprintf(&md, "\n%d case trace(s) changed, %d identical.\n", changed, same)
		}
	}
	fmt.Print(md.String())
	if *mdOut != "" {
		check(os.WriteFile(*mdOut, []byte(md.String()), 0o644))
	}
}

func levelOf(n string) string {
	if strings.HasSuffix(n, ".go") {
		return "server"
	}
	return "client predictor"
}

// worktree checks out ref in a temporary git worktree.
func worktree(root, tmp, name, ref string) string {
	dir := filepath.Join(tmp, name)
	git(root, "worktree", "add", "--detach", "-q", dir, ref)
	cleanups = append(cleanups, func() { exec.Command("git", "-C", root, "worktree", "remove", "--force", dir).Run() })
	return dir
}

// cleanups remove the temporary worktrees (also on a fatal error).
var cleanups []func()

func cleanup() {
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
	cleanups = nil
}

// A runner runs the cases in directory cases at one level ("go": the
// server, "js": the client predictor) in tree dir, writing a trace per case
// it gets through to out. Failing expectations are expected when behaviour
// changed: only the traces matter.
type runner func(dir, cases, out, level string)

func runLevel(dir, cases, out, level string) {
	env := append(os.Environ(), "CASES_DIR="+cases, "CASES_TRACE="+out)
	switch level {
	case "go":
		run(dir, env, "go", "test", "-count=1", "./internal/game", "-run", "^TestCases$")
	case "js":
		if _, err := os.Stat(filepath.Join(dir, "web/game/cases.test.mjs")); err == nil {
			run(dir, env, "node", "--test", "web/game/cases.test.mjs")
		}
	}
}

// traces runs the cases at both levels in tree dir and returns {"<case>.go": trace, "<case>.js": trace}.
func traces(dir, cases, out string, run runner) map[string]trace {
	check(os.MkdirAll(out, 0o755))
	if _, err := os.Stat(filepath.Join(dir, "internal/game/cases_test.go")); err != nil {
		return nil
	}
	run(dir, cases, out, "go")
	run(dir, cases, out, "js")
	return readTraces(out)
}

// retryAlone runs each case the head has a trace for and the base hasn't
// on its own (a directory with just that case) at that level: an older tree
// may stop at the first case it can't load (a field or slot it doesn't know)
// or crash on one, and lose every case after it. What still has no trace is
// a case the base can't run.
func retryAlone(dir, cases, out, tmp string, base, head map[string]trace, run runner) map[string]trace {
	names := make([]string, 0, len(head))
	for n := range head {
		if _, ok := base[n]; !ok {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return base
	}
	sort.Strings(names)
	for i, n := range names {
		name, level := strings.TrimSuffix(n, filepath.Ext(n)), strings.TrimPrefix(filepath.Ext(n), ".")
		one := filepath.Join(tmp, fmt.Sprint(i))
		check(os.MkdirAll(one, 0o755))
		b, err := os.ReadFile(filepath.Join(cases, name+".json"))
		if err != nil {
			continue
		}
		check(os.WriteFile(filepath.Join(one, name+".json"), b, 0o644))
		run(dir, one, out, level)
	}
	return readTraces(out)
}

// readTraces reads the traces written to out.
func readTraces(out string) map[string]trace {
	got := map[string]trace{}
	files, _ := filepath.Glob(filepath.Join(out, "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		check(err)
		var t trace
		check(json.Unmarshal(b, &t))
		got[strings.TrimSuffix(filepath.Base(f), ".json")] = t
	}
	return got
}

type row struct {
	field, base, head, delta string
	first, count             int
}

// diff lists every player.field whose per-tick value differs between traces.
func diff(a, b trace) []row {
	n := min(len(a), len(b))
	byField := map[string]*row{}
	maxD := map[string]float64{}
	for i := 0; i < n; i++ {
		for who, fa := range a[i] {
			fb := b[i][who]
			keys := map[string]bool{}
			for k := range fa {
				keys[k] = true
			}
			for k := range fb {
				keys[k] = true
			}
			for k := range keys {
				va, vb := fa[k], fb[k]
				if strings.HasPrefix(k, "_") || fmt.Sprint(va) == fmt.Sprint(vb) { // _: harness bookkeeping (_contact), not gameplay
					continue
				}
				key := who + "." + k
				r := byField[key]
				if r == nil {
					r = &row{field: key, first: i + 1, base: show(va), head: show(vb)}
					byField[key] = r
				}
				r.count++
				maxD[key] = math.Max(maxD[key], delta(va, vb))
			}
		}
	}
	if len(a) != len(b) {
		byField["(ticks)"] = &row{field: "(ticks)", first: n + 1, base: fmt.Sprint(len(a)), head: fmt.Sprint(len(b)), count: 1}
	}
	rows := make([]row, 0, len(byField))
	for k, r := range byField {
		if d := maxD[k]; d > 0 {
			r.delta = fmt.Sprintf("%.4g", d)
		}
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].first < rows[j].first || rows[i].first == rows[j].first && rows[i].field < rows[j].field
	})
	return rows
}

// delta: numeric magnitude of a change (vector: largest component), 0 if not numeric.
func delta(a, b any) float64 {
	switch av := a.(type) {
	case float64:
		if bv, ok := b.(float64); ok {
			return math.Abs(av - bv)
		}
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return 0
		}
		d := 0.0
		for i := range av {
			d = math.Max(d, delta(av[i], bv[i]))
		}
		return d
	}
	return 0
}

func show(v any) string {
	b, _ := json.Marshal(v)
	return "`" + string(b) + "`"
}

func git(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Stderr = dir, os.Stderr
	out, err := cmd.Output()
	check(err)
	return string(out)
}

func run(dir string, env []string, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	_ = cmd.Run() // output is not needed; traces are files
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "simdiff:", err)
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
		os.Exit(1)
	}
}
