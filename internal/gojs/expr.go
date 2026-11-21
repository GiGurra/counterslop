package gojs

import (
	"bytes"
	"go/ast"
	"go/constant"
	"go/format"
	"go/token"
	"go/types"
	"strings"
)

// JS precedences (higher binds tighter).
const (
	precAssign  = 2 // also arrow functions, ?:, and ?? (which can't mix with || or && unparenthesised)
	precOr      = 3
	precAnd     = 4
	precBitOr   = 5
	precBitXor  = 6
	precBitAnd  = 7
	precEq      = 8
	precRel     = 9
	precShift   = 10
	precAdd     = 11
	precMul     = 12
	precUnary   = 14
	precMember  = 17 // also calls
	precNullish = precAssign
)

func paren(s string, inner, outer int) string {
	if inner < outer {
		return "(" + s + ")"
	}
	return s
}

func isSimple(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident, *ast.BasicLit:
		return true
	case *ast.SelectorExpr:
		return isSimple(x.X)
	case *ast.ParenExpr:
		return isSimple(x.X)
	}
	return false
}

func (t *tr) expr(e ast.Expr) string { return t.exprPrec(e, 0) }

// valueExpr is an expression whose value is stored: struct/array values are
// copied unless they are fresh (literals, call results).
func (t *tr) valueExpr(e ast.Expr) string {
	ty := t.info.Types[e].Type
	if ty != nil && isAggregate(ty) && !t.isFresh(e) {
		return t.clone(ty, t.exprPrec(e, precMember), e)
	}
	return t.expr(e)
}

// returnExpr: returning a local by value needs no copy (it's going away).
func (t *tr) returnExpr(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		if o := t.info.Uses[id]; o != nil && t.locals[o] {
			if _, ro := t.readonly[o]; !ro {
				return t.expr(e)
			}
		}
	}
	return t.valueExpr(e)
}

func (t *tr) isFresh(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.CompositeLit, *ast.FuncLit:
		return true
	case *ast.CallExpr:
		if t.info.Types[x.Fun].IsType() { // a conversion is as fresh as its operand
			return t.isFresh(x.Args[0])
		}
		return true
	case *ast.ParenExpr:
		return t.isFresh(x.X)
	}
	return false
}

func (t *tr) exprPrec(e ast.Expr, outer int) string {
	tv := t.info.Types[e]
	if tv.Value != nil {
		// Package-level consts of this package keep their name (their
		// declaration holds the exact value); everything else is folded.
		if id, ok := e.(*ast.Ident); ok {
			if c, ok := t.info.Uses[id].(*types.Const); ok {
				if c.Pkg() == t.pkg && c.Parent() == t.pkg.Scope() && t.emitted[c] {
					return id.Name
				}
				if n := t.names[c]; n != "" {
					return n
				}
			}
		}
		lit := t.constLit(tv.Value, tv.Type)
		if !isLiteral(e) && !isIdent(e) {
			lit += " /* " + exprSrc(t.fset, e) + " */"
		}
		if strings.HasPrefix(lit, "-") {
			return paren(lit, precUnary, outer)
		}
		return lit
	}
	switch x := e.(type) {
	case *ast.ParenExpr:
		return t.exprPrec(x.X, outer)
	case *ast.Ident:
		if x.Name == "nil" {
			return "null"
		}
		o := t.info.Uses[x]
		if n, ok := t.names[o]; ok {
			return n
		}
		if o != nil && o.Pkg() == t.pkg && o.Parent() == t.pkg.Scope() {
			return t.ref(o, x)
		}
		t.errf(x, "unknown identifier %s", x.Name)
		return x.Name
	case *ast.SelectorExpr:
		return t.selector(x, outer)
	case *ast.IndexExpr:
		xt := t.info.Types[x.X].Type
		switch u := xt.Underlying().(type) {
		case *types.Map:
			return t.mapGet(x)
		case *types.Basic:
			if u.Info()&types.IsString != 0 {
				return t.exprPrec(x.X, precMember) + ".charCodeAt(" + t.expr(x.Index) + ")"
			}
		case *types.Signature:
			t.errf(x, "generic instantiation is not in the subset")
		}
		return t.exprPrec(x.X, precMember) + "[" + t.expr(x.Index) + "]"
	case *ast.SliceExpr:
		if x.Slice3 {
			t.errf(x, "3-index slices are not in the subset")
		}
		args := []string{"0"}
		if x.Low != nil {
			args[0] = t.expr(x.Low)
		}
		if x.High != nil {
			args = append(args, t.expr(x.High))
		} else if x.Low == nil {
			args = nil
		}
		return t.exprPrec(x.X, precMember) + ".slice(" + strings.Join(args, ", ") + ")"
	case *ast.StarExpr:
		return t.exprPrec(x.X, outer)
	case *ast.UnaryExpr:
		return t.unary(x, outer)
	case *ast.BinaryExpr:
		return t.binary(x, outer)
	case *ast.CallExpr:
		return t.call(x, outer)
	case *ast.CompositeLit:
		return t.composite(x)
	case *ast.FuncLit:
		return paren(t.funcLit(x), precAssign, outer)
	case *ast.TypeAssertExpr:
		t.errf(x, "type assertions are not in the subset")
		return "undefined"
	}
	t.errf(e, "%T is not in the subset", e)
	return "undefined"
}

func (t *tr) selector(x *ast.SelectorExpr, outer int) string {
	if sel, ok := t.info.Selections[x]; ok {
		switch sel.Kind() {
		case types.FieldVal:
			if len(sel.Index()) != 1 {
				t.errf(x, "promoted (embedded) fields are not in the subset")
			}
			st := derefStruct(sel.Recv())
			return t.exprPrec(x.X, precMember) + "." + t.fieldName(st, sel.Index()[len(sel.Index())-1])
		case types.MethodVal: // a method value: bind the receiver
			fn := sel.Obj().(*types.Func)
			return t.ref(fn, x) + ".bind(null, " + t.expr(x.X) + ")"
		case types.MethodExpr:
			return t.ref(sel.Obj(), x)
		}
	}
	if id, ok := x.X.(*ast.Ident); ok {
		if pn, ok := t.info.Uses[id].(*types.PkgName); ok {
			if p := pn.Imported().Path(); p == "math" || p == "strings" || p == "slices" {
				t.errf(x, "%s.%s is not in the subset as a value", p, x.Sel.Name)
				return "undefined"
			}
			return t.ref(t.info.Uses[x.Sel], x)
		}
	}
	t.errf(x, "unsupported selector")
	return "undefined"
}

func isLiteral(e ast.Expr) bool {
	e = ast.Unparen(e)
	if u, ok := e.(*ast.UnaryExpr); ok && (u.Op == token.SUB || u.Op == token.ADD) {
		e = ast.Unparen(u.X)
	}
	_, ok := e.(*ast.BasicLit)
	return ok
}

func isIdent(e ast.Expr) bool { _, ok := ast.Unparen(e).(*ast.Ident); return ok }

func isNil(e ast.Expr) bool { id, ok := ast.Unparen(e).(*ast.Ident); return ok && id.Name == "nil" }

func derefStruct(ty types.Type) *types.Struct {
	if p, ok := ty.Underlying().(*types.Pointer); ok {
		ty = p.Elem()
	}
	return ty.Underlying().(*types.Struct)
}

func (t *tr) mapGet(x *ast.IndexExpr) string {
	t.helpers["$get"] = true
	return "$get(" + t.expr(x.X) + ", " + t.expr(x.Index) + ", " + t.zero(t.info.Types[x].Type, x) + ")"
}

func (t *tr) unary(x *ast.UnaryExpr, outer int) string {
	ty := t.info.Types[x].Type
	switch x.Op {
	case token.AND:
		xt := t.info.Types[x.X].Type
		if !isAggregate(xt) {
			t.errf(x, "& of a non-struct, non-array value is not in the subset")
		}
		return t.exprPrec(x.X, outer)
	case token.NOT:
		return paren("!"+t.exprPrec(x.X, precUnary), precUnary, outer)
	case token.ADD:
		return t.exprPrec(x.X, outer)
	case token.SUB, token.XOR:
		op := "-"
		if x.Op == token.XOR {
			op = "~"
		}
		render := func(p int) string {
			inner := t.exprPrec(x.X, precUnary)
			if strings.HasPrefix(inner, "-") || strings.HasPrefix(inner, "~") {
				inner = "(" + inner + ")"
			}
			return paren(op+inner, precUnary, p)
		}
		if bits, _ := intKind(ty); bits > 0 {
			return wrap(ty, render, outer)
		}
		return render(outer)
	}
	t.errf(x, "unary %s is not in the subset", x.Op)
	return "undefined"
}

func (t *tr) binary(x *ast.BinaryExpr, outer int) string {
	lt := t.info.Types[x.X].Type
	switch x.Op {
	case token.LOR:
		return paren(t.exprPrec(x.X, precOr)+" || "+t.exprPrec(x.Y, precOr+1), precOr, outer)
	case token.LAND:
		return paren(t.exprPrec(x.X, precAnd)+" && "+t.exprPrec(x.Y, precAnd+1), precAnd, outer)
	case token.EQL, token.NEQ:
		if isAggregate(lt) {
			t.errf(x, "comparing structs/arrays is not in the subset")
		}
		if isNil(x.X) || isNil(x.Y) { // nil: null, or a field JSON left out (undefined)
			v := x.X
			if isNil(x.X) {
				v = x.Y
			}
			op := " == null"
			if x.Op == token.NEQ {
				op = " != null"
			}
			return paren(t.exprPrec(v, precEq)+op, precEq, outer)
		}
		op := " === "
		if x.Op == token.NEQ {
			op = " !== "
		}
		return paren(t.exprPrec(x.X, precEq)+op+t.exprPrec(x.Y, precEq+1), precEq, outer)
	case token.LSS, token.GTR, token.LEQ, token.GEQ:
		return paren(t.exprPrec(x.X, precRel)+" "+x.Op.String()+" "+t.exprPrec(x.Y, precRel+1), precRel, outer)
	}
	if x.Op == token.ADD || x.Op == token.SUB {
		t.checkFusable(x, x.X)
		t.checkFusable(x, x.Y)
	}
	if isString(lt) && x.Op != token.ADD {
		t.errf(x, "string %s is not in the subset", x.Op)
	}
	l := func(p int) string { return t.exprPrec(x.X, p) }
	r := func(p int) string { return t.exprPrec(x.Y, p) }
	return t.arith(x.Op, t.info.Types[x].Type, l, r, x, outer)
}

// arith renders l op r for numbers with Go's semantics for type ty.
func (t *tr) arith(op token.Token, ty types.Type, l, r func(int) string, at ast.Node, outer int) string {
	bits, signed := intKind(ty)
	sized := bits > 0
	infix := func(js string, p int) func(int) string {
		return func(o int) string { return paren(l(p)+" "+js+" "+r(p+1), p, o) }
	}
	switch op {
	case token.ADD, token.SUB:
		f := infix(op.String(), precAdd)
		if sized {
			return wrap(ty, f, outer)
		}
		return f(outer)
	case token.MUL:
		if bits == 32 {
			return wrap(ty, func(o int) string { return "Math.imul(" + l(precAssign+1) + ", " + r(precAssign+1) + ")" }, outer)
		}
		f := infix("*", precMul)
		if sized {
			return wrap(ty, f, outer)
		}
		return f(outer)
	case token.QUO:
		if isInt(ty) {
			q := func(o int) string { return "Math.trunc(" + l(precMul) + " / " + r(precMul+1) + ")" }
			if sized && signed {
				return wrap(ty, q, outer)
			}
			return q(outer)
		}
		return infix("/", precMul)(outer)
	case token.REM:
		return infix("%", precMul)(outer)
	case token.AND, token.OR, token.XOR, token.AND_NOT:
		if !isInt(ty) {
			t.errf(at, "bit operations need integers")
		}
		js, p := map[token.Token]string{token.AND: "&", token.OR: "|", token.XOR: "^", token.AND_NOT: "&"}[op], map[token.Token]int{token.AND: precBitAnd, token.OR: precBitOr, token.XOR: precBitXor, token.AND_NOT: precBitAnd}[op]
		rr := r
		if op == token.AND_NOT {
			rr = func(int) string { return "~" + r(precUnary) }
		}
		f := func(o int) string { return paren(l(p)+" "+js+" "+rr(p+1), p, o) }
		if bits == 32 && !signed {
			return wrap(ty, f, outer)
		}
		return f(outer)
	case token.SHL:
		f := infix("<<", precShift)
		if sized {
			return wrap(ty, f, outer)
		}
		return f(outer)
	case token.SHR:
		if sized && !signed {
			return infix(">>>", precShift)(outer)
		}
		return infix(">>", precShift)(outer)
	}
	t.errf(at, "operator %s is not in the subset", op)
	return "undefined"
}

// checkFusable rejects a float product used directly as an operand of + or
// -: the Go spec lets the compiler fuse it into one FMA (arm64 and GOAMD64=v3
// do), JS never does, so the two would differ in the last bit. float64(a*b)
// forces the rounding in Go and costs nothing in JS.
func (t *tr) checkFusable(at ast.Node, operand ast.Expr) {
	e := ast.Unparen(operand)
	if u, ok := e.(*ast.UnaryExpr); ok && (u.Op == token.SUB || u.Op == token.ADD) {
		e = ast.Unparen(u.X)
	}
	m, ok := e.(*ast.BinaryExpr)
	if !ok || m.Op != token.MUL || !isFloat(t.info.Types[m].Type) || t.info.Types[m].Value != nil {
		return
	}
	t.errf(m, "%s may be fused with the surrounding +/- on some CPUs (FMA), never in JS: write float64(%s)", exprSrc(t.fset, m), exprSrc(t.fset, m))
}

var mathExact = map[string]string{
	"Abs": "Math.abs", "Sqrt": "Math.sqrt", "Floor": "Math.floor", "Ceil": "Math.ceil",
	"Trunc": "Math.trunc", "Min": "Math.min", "Max": "Math.max",
}

func (t *tr) isBuiltin(fun ast.Expr, name string) bool {
	id, ok := ast.Unparen(fun).(*ast.Ident)
	if !ok {
		return false
	}
	b, ok := t.info.Uses[id].(*types.Builtin)
	return ok && b.Name() == name
}

func (t *tr) args(x *ast.CallExpr, from int) []string {
	var a []string
	for i := from; i < len(x.Args); i++ {
		s := t.exprPrec(x.Args[i], precAssign+1)
		if x.Ellipsis.IsValid() && i == len(x.Args)-1 {
			s = "...(" + t.expr(x.Args[i]) + " ?? [])"
		}
		a = append(a, s)
	}
	return a
}

func (t *tr) appendArgs(x *ast.CallExpr) []string {
	var a []string
	for i := 1; i < len(x.Args); i++ {
		if x.Ellipsis.IsValid() && i == len(x.Args)-1 {
			a = append(a, "...("+t.expr(x.Args[i])+" ?? [])")
			continue
		}
		a = append(a, t.valueExpr(x.Args[i]))
	}
	return a
}

func (t *tr) conversion(x *ast.CallExpr, outer int) string {
	to, from := t.info.Types[x.Fun].Type, t.info.Types[x.Args[0]].Type
	arg := func(p int) string { return t.exprPrec(x.Args[0], p) }
	switch {
	case isInt(to) && isFloat(from):
		trunc := func(int) string { return "Math.trunc(" + t.expr(x.Args[0]) + ")" }
		return wrap(to, trunc, outer)
	case isInt(to) && isInt(from):
		tb, ts := intKind(to)
		fb, fs := intKind(from)
		if tb == 0 || fb > 0 && fb < tb && (fs == ts || !fs) || fb == tb && fs == ts {
			return arg(outer) // widening, or same kind
		}
		return wrap(to, arg, outer)
	case isNumber(to) && isNumber(from):
		return arg(outer) // float64(a*b) rounds in Go; JS never fuses, so nothing to do
	case isString(to) && isString(from):
		return arg(outer)
	case types.Identical(to.Underlying(), from.Underlying()):
		return arg(outer)
	}
	t.errf(x, "conversion %s -> %s is not in the subset", from, to)
	return arg(outer)
}

func (t *tr) call(x *ast.CallExpr, outer int) string {
	if t.info.Types[x.Fun].IsType() {
		return t.conversion(x, outer)
	}
	if id, ok := ast.Unparen(x.Fun).(*ast.Ident); ok {
		if b, ok := t.info.Uses[id].(*types.Builtin); ok {
			return t.builtin(b.Name(), x, outer)
		}
	}
	if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok {
			if pn, ok := t.info.Uses[id].(*types.PkgName); ok {
				switch pn.Imported().Path() {
				case "math":
					return t.mathCall(sel.Sel.Name, x, outer)
				case "slices", "strings":
					return t.stdCall(pn.Imported().Path(), sel.Sel.Name, x, outer)
				}
			}
		}
		if s, ok := t.info.Selections[sel]; ok && s.Kind() == types.MethodVal {
			fn := s.Obj().(*types.Func)
			recv := t.exprPrec(sel.X, precAssign+1)
			return t.ref(fn, x) + "(" + strings.Join(append([]string{recv}, t.args(x, 0)...), ", ") + ")"
		}
	}
	if _, ok := t.info.Types[x.Fun].Type.Underlying().(*types.Signature); !ok {
		t.errf(x, "unsupported call")
	}
	return t.exprPrec(x.Fun, precMember) + "(" + strings.Join(t.args(x, 0), ", ") + ")"
}

func (t *tr) builtin(name string, x *ast.CallExpr, outer int) string {
	switch name {
	case "len":
		at := t.info.Types[x.Args[0]].Type
		switch at.Underlying().(type) {
		case *types.Map:
			return "Object.keys(" + t.expr(x.Args[0]) + ").length"
		case *types.Slice:
			return paren(t.exprPrec(x.Args[0], precMember)+"?.length ?? 0", precNullish, outer)
		}
		return t.exprPrec(x.Args[0], precMember) + ".length"
	case "min", "max":
		if isString(t.info.Types[x].Type) {
			t.errf(x, "min/max of strings is not in the subset")
		}
		return "Math." + name + "(" + strings.Join(t.args(x, 0), ", ") + ")"
	case "append":
		parts := append([]string{"...(" + t.expr(x.Args[0]) + " ?? [])"}, t.appendArgs(x)...)
		return "[" + strings.Join(parts, ", ") + "]"
	case "make":
		ty := t.info.Types[x].Type
		switch u := ty.Underlying().(type) {
		case *types.Map:
			return "{}"
		case *types.Slice:
			z := t.zero(u.Elem(), x)
			n := t.expr(x.Args[1])
			if isAggregate(u.Elem()) || strings.HasPrefix(z, "{") {
				return "Array.from({ length: " + n + " }, () => " + z + ")"
			}
			return "new Array(" + n + ").fill(" + z + ")"
		}
	case "new":
		el := t.info.Types[x.Args[0]].Type
		if !isAggregate(el) {
			t.errf(x, "new of a non-struct, non-array type is not in the subset")
		}
		return t.zero(el, x)
	case "delete":
		return "delete " + t.exprPrec(x.Args[0], precMember) + "[" + t.expr(x.Args[1]) + "]"
	case "panic":
		return "throw new Error(String(" + t.expr(x.Args[0]) + "))"
	}
	t.errf(x, "builtin %s is not in the subset", name)
	return "undefined"
}

func (t *tr) mathCall(name string, x *ast.CallExpr, outer int) string {
	if js, ok := mathExact[name]; ok {
		return js + "(" + strings.Join(t.args(x, 0), ", ") + ")"
	}
	switch name {
	case "Mod": // fmod: exact in both, JS % on floats is fmod
		return paren(t.exprPrec(x.Args[0], precMul)+" % "+t.exprPrec(x.Args[1], precMul+1), precMul, outer)
	case "Inf":
		if v := t.info.Types[x.Args[0]].Value; v != nil {
			if i, _ := constant.Int64Val(v); i >= 0 {
				return "Infinity"
			}
			return paren("-Infinity", precUnary, outer)
		}
	case "NaN":
		return "NaN"
	case "IsNaN":
		return "Number.isNaN(" + t.expr(x.Args[0]) + ")"
	}
	t.errf(x, "math.%s is not bit-identical between Go and JS (V8): use internal/detmath", name)
	return "undefined"
}

func (t *tr) stdCall(pkg, name string, x *ast.CallExpr, outer int) string {
	a := t.args(x, 0)
	recv := func() string { return t.exprPrec(x.Args[0], precMember) }
	switch pkg + "." + name {
	case "slices.Contains":
		return paren(recv()+"?.includes("+a[1]+") ?? false", precNullish, outer)
	case "slices.Index":
		return paren(recv()+"?.indexOf("+a[1]+") ?? -1", precNullish, outer)
	case "strings.HasPrefix":
		return recv() + ".startsWith(" + a[1] + ")"
	case "strings.HasSuffix":
		return recv() + ".endsWith(" + a[1] + ")"
	case "strings.Contains":
		return recv() + ".includes(" + a[1] + ")"
	case "strings.Index":
		return recv() + ".indexOf(" + a[1] + ")"
	case "strings.Repeat":
		return recv() + ".repeat(" + a[1] + ")"
	case "strings.Join":
		return paren("(" + t.expr(x.Args[0]) + " ?? []).join(" + a[1] + ")", precMember, outer)
	}
	t.errf(x, "%s.%s is not in the subset", pkg, name)
	return "undefined"
}

func (t *tr) composite(x *ast.CompositeLit) string {
	ty := t.info.Types[x].Type
	if p, ok := ty.Underlying().(*types.Pointer); ok { // elided &T{...} inside a literal
		ty = p.Elem()
	}
	switch u := ty.Underlying().(type) {
	case *types.Struct:
		vals := map[int]string{}
		for i, el := range x.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				name := kv.Key.(*ast.Ident).Name
				for j := range u.NumFields() {
					if u.Field(j).Name() == name {
						vals[j] = t.valueExpr(kv.Value)
					}
				}
			} else {
				vals[i] = t.valueExpr(el)
			}
		}
		var fs []string
		for i := range u.NumFields() {
			v, ok := vals[i]
			if !ok {
				v = t.zero(u.Field(i).Type(), x)
			}
			fs = append(fs, t.fieldName(u, i)+": "+v)
		}
		return "{ " + strings.Join(fs, ", ") + " }"
	case *types.Map:
		var fs []string
		for _, el := range x.Elts {
			kv := el.(*ast.KeyValueExpr)
			k := t.expr(kv.Key)
			if t.info.Types[kv.Key].Value == nil {
				k = "[" + k + "]"
			}
			fs = append(fs, k+": "+t.valueExpr(kv.Value))
		}
		if len(fs) == 0 {
			return "{}"
		}
		return "{ " + strings.Join(fs, ", ") + " }"
	case *types.Array, *types.Slice:
		var parts []string
		for _, el := range x.Elts {
			if _, ok := el.(*ast.KeyValueExpr); ok {
				t.errf(el, "keyed array elements are not in the subset")
				continue
			}
			parts = append(parts, t.valueExpr(el))
		}
		if a, ok := u.(*types.Array); ok {
			for int64(len(parts)) < a.Len() {
				parts = append(parts, t.zero(a.Elem(), x))
			}
		}
		if len(parts) > 4 || len(strings.Join(parts, ", ")) > 80 {
			ind := strings.Repeat("  ", t.ind+1)
			return "[\n" + ind + strings.Join(parts, ",\n"+ind) + ",\n" + strings.Repeat("  ", t.ind) + "]"
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	t.errf(x, "composite literal of %s is not in the subset", ty)
	return "undefined"
}

// funcLit renders a closure as an arrow function. JS closures capture
// variables by reference like Go's, and `let` loop variables are
// per-iteration like Go 1.22's.
func (t *tr) funcLit(x *ast.FuncLit) string {
	sig := t.info.Types[x].Type.(*types.Signature)
	saved, savedBrk := t.out, t.brk
	t.out, t.brk = &bytes.Buffer{}, nil
	params := t.params(sig)
	t.ind++
	t.stmts(x.Body.List)
	t.ind--
	body := t.out.String()
	t.out, t.brk = saved, savedBrk
	return "(" + strings.Join(params, ", ") + ") => {\n" + body + strings.Repeat("  ", t.ind) + "}"
}

func exprSrc(fset *token.FileSet, e ast.Expr) string {
	var b bytes.Buffer
	if err := format.Node(&b, fset, e); err != nil {
		return "?"
	}
	return strings.ReplaceAll(b.String(), "*/", "* /")
}
