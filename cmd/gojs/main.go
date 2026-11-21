// Command gojs translates a Go package written in the gojs subset
// (internal/gojs) into an ES module for the client.
//
//	go run ./cmd/gojs -all            # every gojs.Targets entry (go generate ./internal/gojs)
//	go run ./cmd/gojs -pkg internal/sim -import <go path>=<js module>   # one package to stdout
//
// The generated files are committed; internal/gojs's TestGeneratedUpToDate
// fails when one is stale.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gigurra/counterslop/internal/gojs"
)

type imports map[string]string

func (m imports) String() string { return fmt.Sprint(map[string]string(m)) }
func (m imports) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("want path=module, got %q", v)
	}
	m[k] = val
	return nil
}

func main() {
	imps := imports{}
	pkg := flag.String("pkg", "", "package directory to translate")
	out := flag.String("out", "", "output .js file (default stdout)")
	flag.Var(imps, "import", "Go import path=JS module specifier (repeatable)")
	all := flag.Bool("all", false, "regenerate every target in gojs.Targets (run from the module root or internal/gojs)")
	flag.Parse()
	if *all {
		root := findRoot()
		for _, tg := range gojs.Targets {
			js, err := gojs.Generate(root, tg)
			if err == nil {
				err = os.WriteFile(filepath.Join(root, tg.Out), js, 0o644)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Printf("%s -> %s (%d bytes)\n", tg.Pkg, tg.Out, len(js))
		}
		return
	}
	if *pkg == "" {
		flag.Usage()
		os.Exit(2)
	}
	js, err := gojs.Translate(gojs.Config{Dir: *pkg, Imports: imps, Header: "go run ./cmd/gojs " + strings.Join(os.Args[1:], " ")})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *out == "" {
		os.Stdout.Write(js)
		return
	}
	if err := os.WriteFile(*out, js, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func findRoot() string {
	d, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		p := filepath.Dir(d)
		if p == d {
			fmt.Fprintln(os.Stderr, "no go.mod above the working directory")
			os.Exit(1)
		}
		d = p
	}
}
