package gojs

import (
	"path/filepath"
	"strings"
)

//go:generate go run ../../cmd/gojs -all

// Target is one package translated for the client. Paths are relative to the
// module root. Imports between targets are wired automatically.
type Target struct {
	Pkg string
	Out string
}

const modPath = "github.com/gigurra/counterslop/"

// Targets is every generated module (`go generate ./internal/gojs` rewrites
// them, TestGeneratedUpToDate fails when one is stale).
var Targets = []Target{
	{Pkg: "internal/detmath", Out: "web/gen/detmath.gen.js"},
	{Pkg: "internal/sim", Out: "web/engine/sim.gen.js"},
	{Pkg: "internal/content/rng", Out: "web/gen/rng.gen.js"},
}

// Generate translates target tg with the module root at root.
func Generate(root string, tg Target) ([]byte, error) {
	imports := map[string]string{}
	for _, o := range Targets {
		rel, err := filepath.Rel(filepath.Dir(tg.Out), o.Out)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(rel, ".") {
			rel = "./" + rel
		}
		imports[modPath+o.Pkg] = filepath.ToSlash(rel)
	}
	return Translate(Config{Dir: filepath.Join(root, tg.Pkg), Imports: imports, Header: "Source: " + tg.Pkg + ". Regenerate: go generate ./internal/gojs"})
}
