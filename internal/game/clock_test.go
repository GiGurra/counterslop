package game

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGameClockOnly: game code reads time from the game clock (s.clock(),
// the tick's now), never the wall clock, so a stepped (lockstep) server plays
// the same however loaded the machine is. The one wall-clock read is
// wallNow (the ticker, socket deadlines, traffic stats, harness timeouts).
func TestGameClockOnly(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && fn.Name.Name == "wallNow" {
				continue
			}
			ast.Inspect(d, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since" || sel.Sel.Name == "Until") {
					t.Errorf("%s: time.%s: use s.clock() (game time) or, for non-gameplay, wallNow()", fset.Position(sel.Pos()), sel.Sel.Name)
				}
				return true
			})
		}
	}
}

// A lockstep server keeps silent clients: their clocks (and keepalive pings)
// stop between harness steps, however long a step takes on a slow machine.
func TestIdleLimitInLockstep(t *testing.T) {
	s := authServer(t)
	if s.idleLimit() != idleTimeout {
		t.Fatalf("idle limit %v, want %v", s.idleLimit(), idleTimeout)
	}
	s.SetLockstep(true)
	if s.idleLimit() < 10*idleTimeout {
		t.Fatalf("lockstep idle limit %v: silent clients dropped while the harness works", s.idleLimit())
	}
	s.SetLockstep(false)
	if s.idleLimit() != idleTimeout {
		t.Fatal("idle limit not restored")
	}
}

// The wall clock (wallNow) only where game time can't be: socket deadlines
// (hang guards), the production ticker (and, through Server.started, the
// clock between Run's start and its first tick), and the harness's own waits.
// A test server that hasn't ticked reads gameEpoch (TestClockBeforeFirstTick). Anything else (gameplay, stats, ordering) reads
// s.clock(): a lockstep test then plays the same however slow the machine.
func TestWallNowOnlyWhereAllowed(t *testing.T) {
	allowed := map[string]string{
		"HandleWS":    "the socket's read deadline (idle drop)",
		"writeLoop":   "the socket's write deadline",
		"SetLockstep": "moves waiting reads' deadlines",
		"WaitCommand": "the harness's hang guard",
		"Run":         "the production ticker, and the game clock until its first tick (Server.started)",
		"wallNow":     "the definition",
		"profClock":   "the tick profiler (sv_prof): a report of how long each step took, never gameplay",
	}
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || allowed[fn.Name.Name] != "" {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "wallNow" {
						t.Errorf("%s: wallNow() in %s: use s.clock() (game time); the wall clock only as a hang guard (add it to this test's list with the reason)", fset.Position(c.Pos()), fn.Name.Name)
					}
				}
				return true
			})
		}
	}
}

// A server that hasn't ticked has a fixed game clock in tests (gameEpoch),
// never the wall clock: a test that builds a world and compares two moments
// before the first tick sees the same time however slow the machine
// (TestSnapMsgMatchesSnapBytes failed in CI on the old wall-clock fallback).
func TestClockBeforeFirstTick(t *testing.T) {
	s := NewServer()
	a := s.clock()
	for wallNow().Equal(a) { // let the wall clock move on
	}
	if b := s.clock(); !a.Equal(gameEpoch) || !b.Equal(a) {
		t.Fatalf("clock before the first tick: %v then %v, want gameEpoch %v both times", a, b, gameEpoch)
	}
	// Run's start is the clock until its first tick (production joins).
	s.started = gameEpoch.Add(time.Hour)
	if !s.clock().Equal(s.started) {
		t.Fatalf("clock %v, want Run's start %v", s.clock(), s.started)
	}
	s.lastTickAt = gameEpoch.Add(2 * time.Hour)
	if !s.clock().Equal(s.lastTickAt) {
		t.Fatalf("clock %v, want the last tick %v", s.clock(), s.lastTickAt)
	}
}
