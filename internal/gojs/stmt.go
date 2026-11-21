package gojs

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

func (t *tr) stmts(l []ast.Stmt) {
	for _, s := range l {
		t.stmt(s)
	}
}

func (t *tr) stmt(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.DeclStmt:
		t.localDecl(s.Decl.(*ast.GenDecl))
	case *ast.ExprStmt:
		t.line("%s;", t.expr(s.X))
	case *ast.IncDecStmt:
		t.line("%s;", t.incDec(s))
	case *ast.AssignStmt:
		t.assign(s)
	case *ast.ReturnStmt:
		switch len(s.Results) {
		case 0:
			t.line("return;")
		case 1:
			t.line("return %s;", t.returnExpr(s.Results[0]))
		default:
			var parts []string
			for _, r := range s.Results {
				parts = append(parts, t.returnExpr(r))
			}
			t.line("return [%s];", strings.Join(parts, ", "))
		}
	case *ast.BlockStmt:
		t.line("{")
		t.ind++
		t.stmts(s.List)
		t.ind--
		t.line("}")
	case *ast.IfStmt:
		if s.Init != nil {
			t.line("{")
			t.ind++
			t.stmt(s.Init)
		}
		t.ifChain(s, "")
		if s.Init != nil {
			t.ind--
			t.line("}")
		}
	case *ast.ForStmt:
		t.forStmt(s, "")
	case *ast.RangeStmt:
		t.rangeStmt(s, "")
	case *ast.SwitchStmt:
		t.switchStmt(s, "")
	case *ast.LabeledStmt:
		l := s.Label.Name
		switch inner := s.Stmt.(type) {
		case *ast.ForStmt:
			t.forStmt(inner, l)
		case *ast.RangeStmt:
			t.rangeStmt(inner, l)
		case *ast.SwitchStmt:
			t.switchStmt(inner, l)
		default:
			t.errf(s, "labels are only supported on for/range/switch")
		}
	case *ast.BranchStmt:
		lbl := ""
		if s.Label != nil {
			lbl = " " + s.Label.Name
		}
		switch s.Tok {
		case token.BREAK:
			if lbl == "" && len(t.brk) > 0 && t.brk[len(t.brk)-1] != "" {
				lbl = " " + t.brk[len(t.brk)-1]
			}
			t.line("break%s;", lbl)
		case token.CONTINUE:
			t.line("continue%s;", lbl)
		default:
			t.errf(s, "%s is not in the subset", s.Tok)
		}
	case *ast.EmptyStmt:
	default:
		t.errf(s, "%T is not in the subset", s)
	}
}

func (t *tr) localDecl(g *ast.GenDecl) {
	for _, s := range g.Specs {
		switch s := s.(type) {
		case *ast.TypeSpec:
			t.errf(s, "local type declarations are not in the subset")
		case *ast.ValueSpec:
			if len(s.Values) > 0 && len(s.Values) != len(s.Names) {
				t.errf(s, "multi-value var initialisers are not in the subset")
				continue
			}
			for i, n := range s.Names {
				obj := t.info.Defs[n]
				if n.Name == "_" {
					continue
				}
				name := t.declare(obj)
				if c, ok := obj.(*types.Const); ok {
					t.line("const %s = %s;", name, t.constLit(c.Val(), c.Type()))
					continue
				}
				val := t.zero(obj.Type(), s)
				if len(s.Values) > 0 {
					val = t.valueExpr(s.Values[i])
				}
				t.line("let %s = %s;", name, val)
			}
		}
	}
}

func (t *tr) ifChain(s *ast.IfStmt, prefix string) {
	t.line("%sif (%s) {", prefix, t.expr(s.Cond))
	t.ind++
	t.stmts(s.Body.List)
	t.ind--
	switch e := s.Else.(type) {
	case nil:
		t.line("}")
	case *ast.IfStmt:
		if e.Init != nil {
			t.line("} else {")
			t.ind++
			t.stmt(e)
			t.ind--
			t.line("}")
			return
		}
		t.ifChain(e, "} else ")
	case *ast.BlockStmt:
		t.line("} else {")
		t.ind++
		t.stmts(e.List)
		t.ind--
		t.line("}")
	}
}

func labelPrefix(label string) string {
	if label == "" {
		return ""
	}
	return label + ": "
}

func (t *tr) forStmt(s *ast.ForStmt, label string) {
	lbl := labelPrefix(label)
	t.brk = append(t.brk, "")
	defer func() { t.brk = t.brk[:len(t.brk)-1] }()
	if s.Init == nil && s.Post == nil {
		cond := "true"
		if s.Cond != nil {
			cond = t.expr(s.Cond)
		}
		t.line("%swhile (%s) {", lbl, cond)
	} else {
		init, post, cond := "", "", ""
		if s.Init != nil {
			init = t.simpleStmt(s.Init)
		}
		if s.Post != nil {
			post = t.simpleStmt(s.Post)
		}
		if s.Cond != nil {
			cond = t.expr(s.Cond)
		}
		t.line("%sfor (%s; %s; %s) {", lbl, init, cond, post)
	}
	t.ind++
	t.stmts(s.Body.List)
	t.ind--
	t.line("}")
}

// simpleStmt renders a for-clause init/post statement without the semicolon.
func (t *tr) simpleStmt(s ast.Stmt) string {
	switch s := s.(type) {
	case *ast.IncDecStmt:
		return t.incDec(s)
	case *ast.AssignStmt:
		if len(s.Lhs) == 1 && len(s.Rhs) == 1 {
			if s.Tok == token.DEFINE {
				return "let " + t.declare(t.info.Defs[s.Lhs[0].(*ast.Ident)]) + " = " + t.valueExpr(s.Rhs[0])
			}
			if s.Tok == token.ASSIGN {
				t.checkWritable(s.Lhs[0])
				return t.lvalue(s.Lhs[0]) + " = " + t.valueExpr(s.Rhs[0])
			}
			return t.opAssign(s)
		}
	}
	t.errf(s, "only single assignments and ++/-- are supported in for clauses")
	return ""
}

func (t *tr) rangeStmt(s *ast.RangeStmt, label string) {
	lbl := labelPrefix(label)
	t.brk = append(t.brk, "")
	defer func() { t.brk = t.brk[:len(t.brk)-1] }()
	if s.Tok == token.ASSIGN {
		t.errf(s, "range with = is not in the subset (use :=)")
		return
	}
	xt := t.info.Types[s.X].Type
	declKey := func() string {
		if id, ok := s.Key.(*ast.Ident); ok && id.Name != "_" {
			return t.declare(t.info.Defs[id])
		}
		return t.temp()
	}
	value := func(x, key string) {
		if id, ok := s.Value.(*ast.Ident); ok && id.Name != "_" {
			o := t.info.Defs[id]
			t.ind++
			t.line("const %s = %s[%s];", t.declare(o), x, key)
			t.ind--
			if isAggregate(o.Type()) {
				t.readonly[o] = "range value"
			}
		}
	}
	if isInt(xt) {
		key := declKey()
		n := t.expr(s.X)
		if !isSimple(s.X) {
			c := t.temp()
			t.line("%sfor (let %s = 0, %s = %s; %s < %s; %s++) {", lbl, key, c, n, key, c, key)
		} else {
			t.line("%sfor (let %s = 0; %s < %s; %s++) {", lbl, key, key, n, key)
		}
	} else {
		x := t.exprPrec(s.X, precMember)
		if !isSimple(s.X) {
			c := t.temp()
			t.line("const %s = %s;", c, t.expr(s.X))
			x = c
		}
		switch u := xt.Underlying().(type) {
		case *types.Map:
			if !isString(u.Key()) && !isInt(u.Key()) {
				t.errf(s, "maps need string or integer keys in the subset")
			}
			kv := t.temp()
			t.line("%sfor (const %s of Object.keys(%s ?? {})) {", lbl, kv, x)
			if id, ok := s.Key.(*ast.Ident); ok && id.Name != "_" {
				k := t.declare(t.info.Defs[id])
				if isInt(u.Key()) {
					t.line("  const %s = Number(%s);", k, kv)
				} else {
					t.line("  const %s = %s;", k, kv)
				}
			}
			value(x, kv)
		case *types.Array:
			key := declKey()
			t.line("%sfor (let %s = 0; %s < %d; %s++) {", lbl, key, key, u.Len(), key)
			value(x, key)
		case *types.Pointer:
			a, ok := u.Elem().Underlying().(*types.Array)
			if !ok {
				t.errf(s, "range over %s is not in the subset", xt)
				return
			}
			key := declKey()
			t.line("%sfor (let %s = 0; %s < %d; %s++) {", lbl, key, key, a.Len(), key)
			value(x, key)
		case *types.Slice:
			key := declKey()
			n := t.temp()
			t.line("%sfor (let %s = 0, %s = %s?.length ?? 0; %s < %s; %s++) {", lbl, key, n, x, key, n, key)
			value(x, key)
		default:
			t.errf(s, "range over %s is not in the subset", xt)
		}
	}
	t.ind++
	t.stmts(s.Body.List)
	t.ind--
	t.line("}")
}

func (t *tr) switchStmt(s *ast.SwitchStmt, label string) {
	if label == "" {
		t.tmp++
		label = fmt.Sprintf("$sw%d", t.tmp)
	}
	t.line("%s: {", label)
	t.ind++
	t.brk = append(t.brk, label)
	defer func() { t.brk = t.brk[:len(t.brk)-1] }()
	if s.Init != nil {
		t.stmt(s.Init)
	}
	tag := ""
	if s.Tag != nil {
		tag = t.temp()
		t.line("const %s = %s;", tag, t.expr(s.Tag))
	}
	var def *ast.CaseClause
	first := true
	for _, c := range s.Body.List {
		cc := c.(*ast.CaseClause)
		if cc.List == nil {
			def = cc
			continue
		}
		var conds []string
		for _, e := range cc.List {
			if tag != "" {
				conds = append(conds, tag+" === "+t.exprPrec(e, precEq+1))
			} else {
				conds = append(conds, t.exprPrec(e, precOr+1))
			}
		}
		kw := "if"
		if !first {
			kw = "} else if"
		}
		first = false
		t.line("%s (%s) {", kw, strings.Join(conds, " || "))
		t.caseBody(cc)
	}
	if def != nil {
		if first {
			t.line("{")
		} else {
			t.line("} else {")
		}
		t.caseBody(def)
	}
	if !first || def != nil {
		t.line("}")
	}
	t.ind--
	t.line("}")
}

func (t *tr) caseBody(cc *ast.CaseClause) {
	t.ind++
	for _, s := range cc.Body {
		if b, ok := s.(*ast.BranchStmt); ok && b.Tok == token.FALLTHROUGH {
			t.errf(b, "fallthrough is not in the subset")
		}
	}
	t.stmts(cc.Body)
	t.ind--
}

// lvalueRead renders an lvalue for reading inside a compound assignment
// (a missing map key reads as the zero value).
func (t *tr) lvalueRead(l ast.Expr) func(int) string {
	if ix, ok := ast.Unparen(l).(*ast.IndexExpr); ok {
		if _, isMap := t.info.Types[ix.X].Type.Underlying().(*types.Map); isMap {
			return func(p int) string { return t.mapGet(ix) }
		}
	}
	return func(p int) string { return t.exprPrec(l, p) }
}

func (t *tr) incDec(s *ast.IncDecStmt) string {
	t.checkWritable(s.X)
	op := token.ADD
	if s.Tok == token.DEC {
		op = token.SUB
	}
	ty := t.info.Types[s.X].Type
	if bits, _ := intKind(ty); bits > 0 || t.isMapIndex(s.X) {
		one := func(int) string { return "1" }
		return t.lvalue(s.X) + " = " + t.arith(op, ty, t.lvalueRead(s.X), one, s, precAssign)
	}
	return t.lvalue(s.X) + s.Tok.String()
}

// lvalue renders an assignment target (a map entry is m[k], not the
// zero-defaulting read).
func (t *tr) lvalue(e ast.Expr) string {
	if ix, ok := ast.Unparen(e).(*ast.IndexExpr); ok && t.isMapIndex(ix) {
		return t.exprPrec(ix.X, precMember) + "[" + t.expr(ix.Index) + "]"
	}
	return t.expr(e)
}

func (t *tr) isMapIndex(e ast.Expr) bool {
	if ix, ok := ast.Unparen(e).(*ast.IndexExpr); ok {
		_, m := t.info.Types[ix.X].Type.Underlying().(*types.Map)
		return m
	}
	return false
}

var opAssignTok = map[token.Token]token.Token{
	token.ADD_ASSIGN: token.ADD, token.SUB_ASSIGN: token.SUB, token.MUL_ASSIGN: token.MUL,
	token.QUO_ASSIGN: token.QUO, token.REM_ASSIGN: token.REM, token.AND_ASSIGN: token.AND,
	token.OR_ASSIGN: token.OR, token.XOR_ASSIGN: token.XOR, token.SHL_ASSIGN: token.SHL,
	token.SHR_ASSIGN: token.SHR, token.AND_NOT_ASSIGN: token.AND_NOT,
}

// opAssign renders x op= y. Plain float/int arithmetic keeps the JS
// compound operator; anything with Go-specific semantics (integer division,
// sized-integer wrap-around, bit ops, map entries) becomes x = x op y.
func (t *tr) opAssign(s *ast.AssignStmt) string {
	l := s.Lhs[0]
	t.checkWritable(l)
	op := opAssignTok[s.Tok]
	ty := t.info.Types[l].Type
	if op == token.ADD || op == token.SUB {
		t.checkFusable(s, s.Rhs[0])
	}
	bits, _ := intKind(ty)
	simple := (op == token.ADD || op == token.SUB || op == token.MUL || op == token.REM || op == token.QUO && isFloat(ty)) && bits == 0 && !t.isMapIndex(l)
	if simple {
		return t.lvalue(l) + " " + s.Tok.String() + " " + t.exprPrec(s.Rhs[0], precAssign)
	}
	r := func(p int) string { return t.exprPrec(s.Rhs[0], p) }
	return t.lvalue(l) + " = " + t.arith(op, ty, t.lvalueRead(l), r, s, precAssign)
}

func (t *tr) assign(s *ast.AssignStmt) {
	if s.Tok != token.ASSIGN && s.Tok != token.DEFINE {
		t.line("%s;", t.opAssign(s))
		return
	}
	if len(s.Rhs) == 1 && len(s.Lhs) > 1 {
		tmp := t.temp()
		r := ast.Unparen(s.Rhs[0])
		if ix, ok := r.(*ast.IndexExpr); ok && t.isMapIndex(ix) { // v, ok := m[k]
			t.helpers["$get2"] = true
			t.line("const %s = $get2(%s, %s, %s);", tmp, t.expr(ix.X), t.expr(ix.Index), t.zero(t.info.Types[ix.X].Type.Underlying().(*types.Map).Elem(), ix))
		} else if _, ok := r.(*ast.CallExpr); ok {
			t.line("const %s = %s;", tmp, t.expr(r))
		} else {
			t.errf(s, "this multi-value form (%T) is not in the subset", r)
		}
		for i, l := range s.Lhs {
			t.assignOne(s, l, fmt.Sprintf("%s[%d]", tmp, i))
		}
		return
	}
	if len(s.Lhs) == 1 {
		if push, ok := t.appendInPlace(s.Lhs[0], s.Rhs[0]); ok {
			t.line("%s;", push)
			return
		}
		t.assignOne(s, s.Lhs[0], t.valueExpr(s.Rhs[0]))
		return
	}
	// Parallel assignment: evaluate every right-hand side first unless none
	// of them can see what the left-hand sides change. Locals can only be
	// seen by name (or by a closure: calls count); fields, elements and
	// package vars can also be seen by any call.
	independent := true
	exposed := false
	lhsObjs := map[types.Object]bool{}
	for _, l := range s.Lhs {
		id, ok := l.(*ast.Ident)
		if !ok {
			exposed = true
			continue
		}
		o := t.info.Uses[id]
		if o == nil {
			o = t.info.Defs[id]
		}
		if o != nil {
			lhsObjs[o] = true
			if !t.locals[o] && s.Tok == token.ASSIGN {
				exposed = true
			}
		}
	}
	for _, r := range s.Rhs {
		ast.Inspect(r, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				if lhsObjs[t.info.Uses[n]] {
					independent = false
				}
			case *ast.CallExpr:
				if exposed && !t.info.Types[n.Fun].IsType() {
					independent = false
				}
			case *ast.SelectorExpr, *ast.IndexExpr, *ast.StarExpr:
				if exposed {
					independent = false // may alias an lvalue
				}
			case *ast.FuncLit:
				independent = false
			}
			return true
		})
	}
	if independent {
		for i, l := range s.Lhs {
			t.assignOne(s, l, t.valueExpr(s.Rhs[i]))
		}
		return
	}
	var tmps []string
	for _, r := range s.Rhs {
		tmp := t.temp()
		t.line("const %s = %s;", tmp, t.valueExpr(r))
		tmps = append(tmps, tmp)
	}
	for i, l := range s.Lhs {
		t.assignOne(s, l, tmps[i])
	}
}

// appendInPlace renders `s = append(s, a, b...)` as a push onto s.
func (t *tr) appendInPlace(l, r ast.Expr) (string, bool) {
	call, ok := ast.Unparen(r).(*ast.CallExpr)
	if !ok || !t.isBuiltin(call.Fun, "append") || len(call.Args) == 0 {
		return "", false
	}
	if exprSrc(t.fset, l) != exprSrc(t.fset, call.Args[0]) || !isSimple(l) {
		return "", false
	}
	t.checkWritable(l)
	args := t.appendArgs(call)
	if len(args) == 0 {
		return t.expr(l), true
	}
	return "(" + t.expr(l) + " ??= []).push(" + strings.Join(args, ", ") + ")", true
}

func (t *tr) assignOne(s *ast.AssignStmt, l ast.Expr, val string) {
	if id, ok := l.(*ast.Ident); ok {
		if id.Name == "_" {
			t.line("%s;", val)
			return
		}
		if s.Tok == token.DEFINE {
			if o := t.info.Defs[id]; o != nil {
				t.line("let %s = %s;", t.declare(o), val)
				return
			}
		}
	}
	if st, ok := ast.Unparen(l).(*ast.StarExpr); ok { // *p = v: overwrite the pointee in place
		switch t.info.Types[st].Type.Underlying().(type) {
		case *types.Struct:
			t.line("Object.assign(%s, %s);", t.expr(st.X), val)
		case *types.Array:
			t.line("%s.splice(0, Infinity, ...%s);", t.exprPrec(st.X, precMember), val)
		default:
			t.errf(l, "*p = v is only in the subset for pointers to structs and arrays")
		}
		return
	}
	t.checkWritable(l)
	t.line("%s = %s;", t.lvalue(l), val)
}

// rootObj is the variable an lvalue writes into (x in x.a[i].b), if any.
func (t *tr) rootObj(e ast.Expr) types.Object {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			if o := t.info.Uses[x]; o != nil {
				return o
			}
			return t.info.Defs[x]
		case *ast.SelectorExpr:
			if tv := t.info.Types[x.X]; tv.Type != nil {
				if _, ptr := tv.Type.Underlying().(*types.Pointer); ptr {
					return nil // through a pointer: someone else's value
				}
			}
			e = x.X
		case *ast.IndexExpr:
			switch t.info.Types[x.X].Type.Underlying().(type) {
			case *types.Slice, *types.Map:
				return nil
			}
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return nil
		}
	}
}

func (t *tr) checkWritable(l ast.Expr) {
	o := t.rootObj(l)
	if o == nil {
		return
	}
	if o.Pkg() != nil && o.Pkg() != t.pkg && o.Parent() == o.Pkg().Scope() {
		t.errf(l, "assigning another package's variable is not in the subset (ES module bindings are read-only)")
	}
	if why, ro := t.readonly[o]; ro {
		// Assigning the whole variable is fine: it rebinds the JS name.
		if id, ok := l.(*ast.Ident); ok && t.info.Uses[id] == o && !isAggregate(o.Type()) {
			return
		}
		t.errf(l, "%s %s is shared by reference in JS: copy it to a local before changing it", why, o.Name())
	}
}
