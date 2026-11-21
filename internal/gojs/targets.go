package gojs

//go:generate go run ../../cmd/gojs -all

// Target is one package translated for the client. Paths are relative to the
// module root; Imports map Go packages to JS modules relative to Out.
type Target struct {
	Pkg     string
	Out     string
	Imports map[string]string
}

const modPath = "github.com/gigurra/counterslop/"

// Targets is every generated module (`go generate ./internal/gojs` rewrites
// them, TestGeneratedUpToDate fails when one is stale).
var Targets = []Target{
	{Pkg: "internal/detmath", Out: "web/gen/detmath.gen.js"},
	{Pkg: "internal/sim", Out: "web/engine/sim.gen.js",
		Imports: map[string]string{modPath + "internal/detmath": "../gen/detmath.gen.js"}},
}

// Generate translates target tg with the module root at root.
func Generate(root string, tg Target) ([]byte, error) {
	return Translate(Config{Dir: root + "/" + tg.Pkg, Imports: tg.Imports, Header: "Source: " + tg.Pkg + ". Regenerate: go generate ./internal/gojs"})
}
