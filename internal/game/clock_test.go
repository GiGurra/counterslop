package game

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
