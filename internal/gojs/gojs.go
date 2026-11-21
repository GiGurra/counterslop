// Package gojs translates the deterministic subset of Go that the server and
// the client share (the movement sim, weapon rules, hit model, command
// protocol...) into readable ES modules, so that code is written once, in Go.
// docs/shared-logic.md has the why and the measurements; this is the what.
//
// What gets translated: a whole package, or, when any of its files carries a
// `//gojs:shared` line above the package clause, only those files (so a
// server package like internal/game can share its rules files and keep the
// rest server-only). Shared code may not use declarations that aren't
// translated; the error names them.
//
// Per-declaration directives (in the doc comment):
//
//	//gojs:skip              not translated (server-only helper in a shared file)
//	//gojs:extern ./x.js     not translated: imported from a hand-written JS module
//	                         instead (for a hand-optimised hot path; the module
//	                         must export the same name, T_method for methods, and
//	                         should have a parity test against the Go)
//
// The subset (anything else is an error with its position):
//
//   - consts (emitted as their exact folded value), vars, funcs, methods (become
//     functions T_name(recv, ...)), closures, variadics, multiple results
//     (returned as arrays), named struct/basic/slice/map/func types;
//   - bool, string (ASCII: ==, <, +, len, indexing, a few strings.* calls),
//     integers (int/int64 within ±2^53; int32/uint32/int16/uint16/int8/uint8
//     with Go's wrap-around), float64, arrays, slices, maps with string or
//     integer keys, structs, pointers to structs/arrays, funcs;
//   - if/for/range (int, slice, array, map)/switch/labels/break/continue,
//     := = op= ++ --, panic;
//   - len, min, max, append, make, new, delete, slicing, math.{Abs,Sqrt,Floor,
//     Ceil,Trunc,Min,Max,Mod,Inf,NaN}, slices.{Contains,Index}, calls into other
//     translated packages.
//   - not: goroutines, channels, defer, interfaces, type switches, embedding,
//     generics, copy, math.Sin & co (V8 differs in the last bit: use
//     internal/detmath), and float products directly inside + or - (write
//     float64(a*b): Go may fuse them into an FMA on arm64, JS never does).
//
// Semantics kept: Go's exact constant arithmetic, integer division and
// float->int conversion truncate, sized integers wrap, struct/array values are
// copied on assignment, multiple assignment evaluates every right-hand side
// first, switch cases don't fall through and `break` leaves the switch,
// reading a missing map key gives the zero value.
//
// Where JS differs and the subset relies on discipline instead:
//   - struct/array parameters and range values are passed by reference: the
//     translator rejects code that assigns into one;
//   - a slice expression s[a:b] is a copy, and append never shares a backing
//     array: don't write through subslices (Go code that relies on aliasing is
//     rejected only when visible to the translator);
//   - map iteration order is JS insertion order (Go's is random: don't rely on it).
//
// Data from JSON: every struct type with json tags gets T_fromJSON(v), which
// fills the fields JSON left out with Go's zero values (JS objects parsed from
// JSON just lack them).
package gojs

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Config says what to translate and how imports map to JS modules.
type Config struct {
	Dir     string            // package directory
	Imports map[string]string // Go import path -> JS module specifier (relative to the output)
	Header  string            // a comment line under the "generated" line
}

type directives struct {
	skip   bool
	extern string
}

// Translate returns the ES module for the package in cfg.Dir.
func Translate(cfg Config) ([]byte, error) {
	fset := token.NewFileSet()
	ents, err := os.ReadDir(cfg.Dir)
	if err != nil {
		return nil, err
	}
	var files []*ast.File
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(cfg.Dir, n), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		if !buildable(f) {
			continue
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s: no Go files", cfg.Dir)
	}
	sort.Slice(files, func(i, j int) bool { return fset.File(files[i].Pos()).Name() < fset.File(files[j].Pos()).Name() })
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Implicits:  map[ast.Node]types.Object{},
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check(files[0].Name.Name, fset, files, info)
	if err != nil {
		return nil, err
	}
	t := &tr{
		fset: fset, info: info, pkg: pkg, cfg: cfg, out: &bytes.Buffer{},
		used: map[string]bool{}, clones: map[*types.Named]bool{}, fromJSON: map[*types.Named]bool{},
		helpers: map[string]bool{}, emitted: map[types.Object]bool{}, dirs: map[types.Object]directives{},
		aliases: map[string]string{}, reservedTop: map[string]bool{},
	}
	t.selectDecls(files)
	t.emit()
	if len(t.errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(t.errs, "\n"))
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by cmd/gojs from %s. DO NOT EDIT.\n", pkg.Path())
	if cfg.Header != "" {
		fmt.Fprintf(&out, "// %s\n", cfg.Header)
	}
	var imps []string
	for p := range t.used {
		imps = append(imps, p)
	}
	sort.Strings(imps)
	for _, p := range imps {
		fmt.Fprintf(&out, "import * as %s from %q;\n", t.aliases[p], cfg.Imports[p])
	}
	for _, l := range t.externs {
		out.WriteString(l + "\n")
	}
	out.WriteString(t.out.String())
	return out.Bytes(), nil
}

// buildable: skip files whose //go:build line excludes them on a normal
// (non-wasm) build, e.g. the wasm benchmark.
func buildable(f *ast.File) bool {
	for _, cg := range f.Comments {
		if cg.Pos() > f.Package {
			break
		}
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "//go:build ") && strings.Contains(c.Text, "wasm") && !strings.Contains(c.Text, "!wasm") {
				return false
			}
		}
	}
	return true
}

type tr struct {
	fset    *token.FileSet
	info    *types.Info
	pkg     *types.Package
	cfg     Config
	out     *bytes.Buffer
	ind     int
	errs    []string
	used    map[string]bool // imported packages referenced at runtime
	aliases map[string]string
	externs []string

	files       []*ast.File // the translated files
	emitted     map[types.Object]bool
	dirs        map[types.Object]directives
	clones      map[*types.Named]bool
	fromJSON    map[*types.Named]bool
	helpers     map[string]bool
	reservedTop map[string]bool // package-level names and import aliases: locals avoid them

	// per function
	names    map[types.Object]string
	taken    map[string]bool
	readonly map[types.Object]string // value params / range values: why they can't be assigned
	locals   map[types.Object]bool
	tmp      int
	brk      []string // break target per enclosing breakable stmt ("" = native)
}

func (t *tr) errf(n ast.Node, format string, a ...any) {
	pos := "?"
	if n != nil {
		pos = t.fset.Position(n.Pos()).String()
	}
	t.errs = append(t.errs, fmt.Sprintf("%s: %s", pos, fmt.Sprintf(format, a...)))
}

func (t *tr) line(format string, a ...any) {
	t.out.WriteString(strings.Repeat("  ", t.ind))
	fmt.Fprintf(t.out, format, a...)
	t.out.WriteByte('\n')
}

func (t *tr) pos(n ast.Node) string {
	p := t.fset.Position(n.Pos())
	return fmt.Sprintf("%s:%d", filepath.Base(p.Filename), p.Line)
}

func parseDirectives(cgs ...*ast.CommentGroup) directives {
	var d directives
	for _, cg := range cgs {
		if cg == nil {
			continue
		}
		for _, c := range cg.List {
			switch {
			case c.Text == "//gojs:skip":
				d.skip = true
			case strings.HasPrefix(c.Text, "//gojs:extern "):
				d.extern = strings.TrimSpace(strings.TrimPrefix(c.Text, "//gojs:extern "))
			}
		}
	}
	return d
}

func sharedFile(f *ast.File) bool {
	for _, cg := range f.Comments {
		if cg.Pos() > f.Package {
			break
		}
		for _, c := range cg.List {
			if c.Text == "//gojs:shared" {
				return true
			}
		}
	}
	return false
}

// selectDecls picks the files and declarations to translate.
func (t *tr) selectDecls(files []*ast.File) {
	partial := false
	for _, f := range files {
		if sharedFile(f) {
			partial = true
		}
	}
	for _, f := range files {
		if !partial || sharedFile(f) {
			t.files = append(t.files, f)
		}
	}
	for _, f := range t.files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				o := t.info.Defs[d.Name]
				dd := parseDirectives(d.Doc)
				if d.Recv != nil && (d.Name.Name == "UnmarshalJSON" || d.Name.Name == "MarshalJSON") {
					dd.skip = true // JSON plumbing: the client gets T_fromJSON instead
				}
				t.dirs[o] = dd
				if !dd.skip {
					t.emitted[o] = true
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					var ids []*ast.Ident
					var doc *ast.CommentGroup
					switch s := s.(type) {
					case *ast.TypeSpec:
						ids, doc = []*ast.Ident{s.Name}, s.Doc
					case *ast.ValueSpec:
						ids, doc = s.Names, s.Doc
					}
					dd := parseDirectives(d.Doc, doc)
					if dd.extern != "" {
						t.errf(s, "//gojs:extern is for functions and methods only")
					}
					for _, id := range ids {
						if o := t.info.Defs[id]; o != nil && id.Name != "_" {
							t.dirs[o] = dd
							if !dd.skip {
								t.emitted[o] = true
							}
						}
					}
				}
			}
		}
	}
	for _, n := range t.pkg.Scope().Names() {
		t.reservedTop[n] = true
	}
	for _, f := range t.files {
		for _, im := range f.Imports {
			p := strings.Trim(im.Path.Value, `"`)
			a := filepath.Base(p)
			for i := 2; t.reservedTop[a] && t.aliases[p] != a; i++ {
				a = fmt.Sprintf("%s%d", filepath.Base(p), i)
			}
			if _, ok := t.aliases[p]; !ok {
				t.aliases[p] = a
				t.reservedTop[a] = true
			}
			if im.Name != nil {
				t.reservedTop[im.Name.Name] = true
			}
		}
	}
}

// emit writes the module body: consts, types, vars (in Go's init order),
// funcs, then generated helpers.
func (t *tr) emit() {
	for _, f := range t.files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.CONST {
				t.constDecl(g)
			}
		}
	}
	for _, f := range t.files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.TYPE {
				for _, s := range g.Specs {
					if ts := s.(*ast.TypeSpec); t.emitted[t.info.Defs[ts.Name]] {
						t.typeSpec(ts)
					}
				}
			}
		}
	}
	t.packageVars()
	for _, f := range t.files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				o := t.info.Defs[fd.Name]
				if dd := t.dirs[o]; dd.extern != "" {
					name := t.funcJSName(o.(*types.Func))
					t.externs = append(t.externs, fmt.Sprintf("import { %s } from %q;", name, dd.extern))
					if ast.IsExported(fd.Name.Name) {
						t.externs = append(t.externs, fmt.Sprintf("export { %s };", name))
					}
					continue
				}
				if t.emitted[o] {
					t.funcDecl(fd)
				}
			}
		}
	}
	t.emitFromJSON()
	t.emitClones()
	t.emitHelpers()
}

func exportKw(name string) string {
	if ast.IsExported(name) {
		return "export "
	}
	return ""
}

func (t *tr) constDecl(g *ast.GenDecl) {
	for _, s := range g.Specs {
		vs := s.(*ast.ValueSpec)
		for i, n := range vs.Names {
			c, ok := t.info.Defs[n].(*types.Const)
			if !ok || n.Name == "_" || !t.emitted[c] {
				continue
			}
			src := ""
			if i < len(vs.Values) && !isLiteral(vs.Values[i]) {
				src = " // " + exprSrc(t.fset, vs.Values[i])
			}
			t.line("%sconst %s = %s;%s", exportKw(n.Name), n.Name, t.constLit(c.Val(), c.Type()), src)
		}
	}
}

// packageVars: uninitialised vars first (initialisers may read them), then
// the initialised ones in Go's initialisation order.
func (t *tr) packageVars() {
	t.resetFunc()
	inited := map[types.Object]bool{}
	for _, in := range t.info.InitOrder {
		for _, v := range in.Lhs {
			inited[v] = true
		}
	}
	for _, f := range t.files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
				for _, s := range g.Specs {
					for _, n := range s.(*ast.ValueSpec).Names {
						o := t.info.Defs[n]
						if o != nil && t.emitted[o] && !inited[o] {
							t.line("%slet %s = %s;", exportKw(n.Name), n.Name, t.zero(o.Type(), n))
						}
					}
				}
			}
		}
	}
	for _, in := range t.info.InitOrder {
		if len(in.Lhs) != 1 {
			t.errf(in.Rhs, "multi-value var initialisers are not in the subset")
			continue
		}
		v := in.Lhs[0]
		if !t.emitted[v] {
			continue
		}
		t.line("%slet %s = %s;", exportKw(v.Name()), v.Name(), t.valueExpr(in.Rhs))
	}
}

func (t *tr) resetFunc() {
	t.names, t.taken, t.readonly, t.locals, t.tmp, t.brk = map[types.Object]string{}, map[string]bool{}, map[types.Object]string{}, map[types.Object]bool{}, 0, nil
}

func (t *tr) typeSpec(s *ast.TypeSpec) {
	obj := t.info.Defs[s.Name].(*types.TypeName)
	if s.Assign.IsValid() {
		return // alias: types only
	}
	if s.TypeParams != nil {
		t.errf(s, "generic types are not in the subset")
		return
	}
	switch u := obj.Type().Underlying().(type) {
	case *types.Struct:
		var fs []string
		for i := range u.NumFields() {
			f := u.Field(i)
			if f.Embedded() {
				t.errf(s, "embedded fields are not in the subset")
			}
			fs = append(fs, fmt.Sprintf("%s: %s", t.fieldName(u, i), jsDocType(f.Type())))
		}
		t.line("")
		t.line("/** @typedef {{%s}} %s */", strings.Join(fs, ", "), s.Name.Name)
		if hasJSONTags(u) {
			t.fromJSON[obj.Type().(*types.Named)] = true
		}
	case *types.Interface:
		t.errf(s, "interfaces are not in the subset")
	default:
		t.line("")
		t.line("/** @typedef {%s} %s */", jsDocType(u), s.Name.Name)
	}
}

func (t *tr) funcJSName(fn *types.Func) string {
	sig := fn.Type().(*types.Signature)
	if sig.Recv() == nil {
		return fn.Name()
	}
	return recvNamed(sig.Recv().Type()).Obj().Name() + "_" + fn.Name()
}

func recvNamed(rt types.Type) *types.Named {
	if p, ok := rt.(*types.Pointer); ok {
		rt = p.Elem()
	}
	return rt.(*types.Named)
}

func (t *tr) funcDecl(d *ast.FuncDecl) {
	t.resetFunc()
	obj := t.info.Defs[d.Name].(*types.Func)
	sig := obj.Type().(*types.Signature)
	if d.Type.TypeParams != nil {
		t.errf(d, "generic functions are not in the subset")
		return
	}
	var params []string
	if r := sig.Recv(); r != nil {
		if r.Name() == "" || r.Name() == "_" {
			params = append(params, "$recv")
		} else {
			params = append(params, t.declare(r))
		}
		if _, ptr := r.Type().(*types.Pointer); !ptr && isAggregate(r.Type()) {
			t.readonly[r] = "value receiver"
		}
	}
	params = append(params, t.params(sig)...)
	if d.Body == nil {
		t.errf(d, "functions without bodies are not in the subset")
		return
	}
	t.line("")
	if d.Doc != nil {
		if txt := strings.TrimSpace(d.Doc.Text()); txt != "" {
			for _, l := range strings.Split(txt, "\n") {
				t.line("// %s", strings.TrimRight(l, " "))
			}
		}
	}
	t.line("// %s", t.pos(d))
	t.line("%sfunction %s(%s) {", exportKw(d.Name.Name), t.funcJSName(obj), strings.Join(params, ", "))
	t.ind++
	t.stmts(d.Body.List)
	t.ind--
	t.line("}")
}

func (t *tr) params(sig *types.Signature) []string {
	var ps []string
	for i := range sig.Params().Len() {
		p := sig.Params().At(i)
		n := fmt.Sprintf("$p%d", i)
		if p.Name() != "" && p.Name() != "_" {
			n = t.declare(p)
		}
		if sig.Variadic() && i == sig.Params().Len()-1 {
			n = "..." + n
		} else if isAggregate(p.Type()) {
			t.readonly[p] = "value parameter"
		}
		ps = append(ps, n)
	}
	return ps
}

// importUsedAtRuntime: does any func or var (not a type, const or field) of
// package p get used?
func (t *tr) needImport(p string, at ast.Node) string {
	if _, ok := t.cfg.Imports[p]; !ok {
		t.errf(at, "package %s has no JS module (translate it too: add it to gojs.Targets)", p)
	}
	t.used[p] = true
	a, ok := t.aliases[p]
	if !ok {
		a = filepath.Base(p)
		t.aliases[p] = a
	}
	return a
}

// ref is the JS name of a package-level object of this or another package,
// checking that shared code only uses translated declarations.
func (t *tr) ref(o types.Object, at ast.Node) string {
	if o.Pkg() != t.pkg {
		return t.needImport(o.Pkg().Path(), at) + "." + t.objJSName(o)
	}
	if !t.emitted[o] && t.dirs[o].extern == "" {
		if t.dirs[o].skip {
			t.errf(at, "%s is //gojs:skip (server-only) but shared code uses it", o.Name())
		} else {
			t.errf(at, "%s is not in a //gojs:shared file: shared code can't use it", o.Name())
		}
	}
	return t.objJSName(o)
}

func (t *tr) objJSName(o types.Object) string {
	if fn, ok := o.(*types.Func); ok {
		return t.funcJSName(fn)
	}
	return o.Name()
}
