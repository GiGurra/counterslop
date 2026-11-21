package gojs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/detmath"
	"github.com/gigurra/counterslop/internal/gojs/internal/sample"
)

const root = "../.."

func node(t *testing.T) string {
	t.Helper()
	n, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	return n
}

// runNode runs an ES module script (src) in dir and returns its stdout.
func runNode(t *testing.T, dir, src string, stdin []byte) []byte {
	t.Helper()
	f := filepath.Join(dir, fmt.Sprintf("run-%d.mjs", rand.Int()))
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f)
	cmd := exec.Command(node(t), f)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// The committed .gen.js files are what the translator makes of today's Go.
func TestGeneratedUpToDate(t *testing.T) {
	for _, tg := range Targets {
		want, err := Generate(root, tg)
		if err != nil {
			t.Fatalf("%s: %v", tg.Pkg, err)
		}
		got, err := os.ReadFile(filepath.Join(root, tg.Out))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale: run go generate ./internal/gojs", tg.Out)
		}
	}
}

// Every construct with tricky semantics computes the same in Go and in the
// translated JS.
func TestSampleAgrees(t *testing.T) {
	js, err := Translate(Config{Dir: "internal/sample"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.mjs"), js, 0o644); err != nil {
		t.Fatal(err)
	}
	doc := `{"slots": {"p": {"id": "p", "ammo": 7}, "s": {"id": "s"}}, "cur": "p", "pos": [1, 2, 3], "main": {"ammo": 4}, "team": "red"}`
	var kit, bare sample.Kit
	if err := json.Unmarshal([]byte(`{"cur": "p"}`), &bare); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(doc), &kit); err != nil {
		t.Fatal(err)
	}
	want := map[string][]float64{
		"run": sample.Run(), "mulberry": sample.Mulberry(12345, 50), "sized": sample.Sized(),
		"collections": sample.Collections(), "describe": sample.Describe(kit),
		"zeros": sample.Zeros(kit), "zerosBare": sample.Zeros(bare),
	}
	src := `import * as s from "./sample.mjs";
process.stdout.write(JSON.stringify({ run: s.Run(), mulberry: s.Mulberry(12345, 50), sized: s.Sized(),
  collections: s.Collections(), describe: s.Describe(s.Kit_fromJSON(JSON.parse(` + "`" + doc + "`" + `))),
  zeros: s.Zeros(s.Kit_fromJSON(JSON.parse(` + "`" + doc + "`" + `))), zerosBare: s.Zeros(s.Kit_fromJSON({ cur: "p" })) }));`
	var got map[string][]float64
	if err := json.Unmarshal(runNode(t, dir, src, nil), &got); err != nil {
		t.Fatal(err)
	}
	for name, w := range want {
		g := got[name]
		if len(g) != len(w) {
			t.Errorf("%s: js %v, go %v", name, g, w)
			continue
		}
		for i := range w {
			if math.Float64bits(g[i]) != math.Float64bits(w[i]) {
				t.Errorf("%s[%d]: js %v, go %v", name, i, g[i], w[i])
			}
		}
	}
}

// detmath.gen.js returns the same bits as detmath on seeded inputs over the
// ranges the game uses and beyond.
func TestDetmathBitsJS(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	const n = 100000
	in := make([][2]float64, n)
	for i := range in {
		sc := []float64{1, 10, 1000, 1e9}[i%4]
		in[i] = [2]float64{(r.Float64()*2 - 1) * sc, (r.Float64()*2 - 1) * sc}
	}
	b, _ := json.Marshal(in)
	src := `import * as m from ` + jsPath(t, root+"/web/gen/detmath.gen.js") + `;
import { readFileSync } from "node:fs";
const inp = JSON.parse(readFileSync(0, "utf8"));
const dv = new DataView(new ArrayBuffer(8));
const hex = (x) => { dv.setFloat64(0, x); return dv.getBigUint64(0).toString(16); };
const out = inp.map(([a, b]) => [hex(m.Sin(a)), hex(m.Cos(a)), hex(m.Exp(a / 50)), hex(m.Hypot(a, b))]);
process.stdout.write(JSON.stringify(out));`
	var got [][4]string
	if err := json.Unmarshal(runNode(t, t.TempDir(), src, b), &got); err != nil {
		t.Fatal(err)
	}
	bad := 0
	for i, v := range in {
		want := [4]float64{detmath.Sin(v[0]), detmath.Cos(v[0]), detmath.Exp(v[0] / 50), detmath.Hypot(v[0], v[1])}
		for k, w := range want {
			if got[i][k] != fmt.Sprintf("%x", math.Float64bits(w)) {
				if bad++; bad < 5 {
					t.Errorf("input %v fn %d: js %s, go %x", v, k, got[i][k], math.Float64bits(w))
				}
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d results differ", bad, 4*n)
	}
}

func jsPath(t *testing.T, p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal("file://" + filepath.ToSlash(a))
	return string(b)
}

// No CPU may fuse a multiply-add in the shared packages: compile them for
// arm64 (whose compiler fuses a*b+c whenever it can) and look for FMADD.
// The translator's checkFusable makes this hold; this proves it.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for arm64")
	}
	for _, tg := range Targets {
		funcs, err := SharedFuncs(filepath.Join(root, tg.Pkg), modPath+tg.Pkg)
		if err != nil {
			t.Fatal(err)
		}
		shared := map[string]bool{}
		for _, f := range funcs {
			shared[f] = true
		}
		cmd := exec.Command("go", "build", "-gcflags=-S", "-o", os.DevNull, "./"+tg.Pkg)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOARCH=arm64", "GOOS=linux")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", tg.Pkg, err, out)
		}
		cur, checked := "", 0
		for _, l := range strings.Split(string(out), "\n") {
			if f := strings.Fields(l); len(f) > 1 && f[1] == "STEXT" {
				cur = f[0]
				if i := strings.Index(cur, ".func"); i > 0 { // a closure belongs to its function
					cur = cur[:i]
				}
				if shared[cur] {
					checked++
				}
				continue
			}
			if !shared[cur] {
				continue
			}
			for _, op := range []string{"\tFMADDD\t", "\tFMSUBD\t", "\tFNMADDD\t", "\tFNMSUBD\t"} {
				if strings.Contains(l, op) {
					t.Errorf("%s: fused multiply-add in shared %s: %s", tg.Pkg, cur, strings.TrimSpace(l))
				}
			}
		}
		if checked == 0 {
			t.Errorf("%s: no shared function found in the assembly (symbol names changed?)", tg.Pkg)
		}
	}
}

// What is outside the subset is an error with a position, not wrong JS.
func TestRejects(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"goroutine", `func f() { go f() }`, "GoStmt"},
		{"defer", `func f() { defer f() }`, "DeferStmt"},
		{"chan", `func f(c chan int) { c <- 1 }`, "SendStmt"},
		{"sin", `import "math"
func f(x float64) float64 { return math.Sin(x) }`, "use internal/detmath"},
		{"fma", `func f(a, b, c float64) float64 { return a*b + c }`, "write float64(a * b)"},
		{"fma-assign", `func f(a, b, c float64) float64 { c += a * b; return c }`, "write float64(a * b)"},
		{"mutate-param", `type T struct{ X float64 }
func f(t T) float64 { t.X = 1; return t.X }`, "value parameter t is shared by reference"},
		{"mutate-range", `type T struct{ X float64 }
func f(ts []T) { for _, t := range ts { t.X = 1 } }`, "range value t is shared by reference"},
		{"copy", `func f(a, b []int) { copy(a, b) }`, "builtin copy"},
		{"fallthrough", `func f(a int) int { switch a { case 1: fallthrough; default: }; return a }`, "fallthrough"},
		{"interface", `type I interface{ M() }`, "interfaces are not in the subset"},
		{"type-assert", `func f(v any) int { return v.(int) }`, "type assertions"},
		{"embedding", `type A struct{ X int }
type B struct{ A }`, "embedded fields"},
		{"generic", `func F[T any](x T) T { return x }`, "generic functions"},
		{"named-results", `func F() (a, b int) { a, b = 1, 2; return }`, "named results"},
		{"skip-used", `//gojs:skip
func server() int { return 1 }
func F() int { return server() }`, "server is //gojs:skip"},
		{"extern-type", `//gojs:extern ./x.js
type T struct{}`, "//gojs:extern is for functions"},
		{"custom-json", `type S []string
func (s *S) UnmarshalJSON(b []byte) error { return nil }
type T struct{ A S ` + "`json:\"a\"`" + ` }`, "custom UnmarshalJSON"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			src := "package p\n\n" + c.src + "\n"
			if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Translate(Config{Dir: dir})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestLowerFirst(t *testing.T) {
	for in, want := range map[string]string{"EyeH": "eyeH", "OnGround": "onGround", "F": "f", "ID": "id", "HTTPServer": "httpServer", "x": "x"} {
		if got := lowerFirst(in); got != want {
			t.Errorf("lowerFirst(%q) = %q, want %q", in, got, want)
		}
	}
}
