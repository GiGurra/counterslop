package gojs

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"sort"
	"strconv"
	"strings"
)

func isAggregate(ty types.Type) bool {
	switch ty.Underlying().(type) {
	case *types.Struct, *types.Array:
		return true
	}
	return false
}

func basicInfo(ty types.Type) types.BasicInfo {
	if b, ok := ty.Underlying().(*types.Basic); ok {
		return b.Info()
	}
	return 0
}

func isInt(ty types.Type) bool    { return basicInfo(ty)&types.IsInteger != 0 }
func isFloat(ty types.Type) bool  { return basicInfo(ty)&types.IsFloat != 0 }
func isString(ty types.Type) bool { return basicInfo(ty)&types.IsString != 0 }
func isNumber(ty types.Type) bool { return basicInfo(ty)&types.IsNumeric != 0 }

// intKind: the width and signedness of a sized integer type that wraps in
// Go; 0 bits for int/int64/uint/uint64 (plain JS numbers, |v| < 2^53).
func intKind(ty types.Type) (bits int, signed bool) {
	b, ok := ty.Underlying().(*types.Basic)
	if !ok {
		return 0, false
	}
	switch b.Kind() {
	case types.Int8:
		return 8, true
	case types.Int16:
		return 16, true
	case types.Int32:
		return 32, true
	case types.Uint8:
		return 8, false
	case types.Uint16:
		return 16, false
	case types.Uint32:
		return 32, false
	case types.Int, types.Int64, types.Uint, types.Uint64, types.Uintptr, types.UntypedInt, types.UntypedRune:
		return 0, b.Info()&types.IsUnsigned == 0
	}
	return 0, false
}

// wrap renders s (already rendered at the precedence the wrapping operator
// needs) reduced to the sized integer type ty, at the caller's precedence.
func wrap(ty types.Type, render func(prec int) string, outer int) string {
	bits, signed := intKind(ty)
	switch {
	case bits == 32 && !signed:
		return paren(render(precShift)+" >>> 0", precShift, outer)
	case bits == 32:
		return paren(render(precBitOr)+" | 0", precBitOr, outer)
	case bits == 16 && !signed:
		return paren(render(precBitAnd)+" & 0xffff", precBitAnd, outer)
	case bits == 8 && !signed:
		return paren(render(precBitAnd)+" & 0xff", precBitAnd, outer)
	case bits == 16:
		return paren(render(precShift)+" << 16 >> 16", precShift, outer)
	case bits == 8:
		return paren(render(precShift)+" << 24 >> 24", precShift, outer)
	}
	return render(outer)
}

func (t *tr) zero(ty types.Type, at ast.Node) string {
	switch u := ty.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			return "false"
		case u.Info()&types.IsString != 0:
			return `""`
		case u.Info()&types.IsNumeric != 0:
			return "0"
		}
	case *types.Array:
		z := t.zero(u.Elem(), at)
		if u.Len() <= 16 {
			parts := make([]string, u.Len())
			for i := range parts {
				parts[i] = z
			}
			return "[" + strings.Join(parts, ", ") + "]"
		}
		return fmt.Sprintf("Array.from({ length: %d }, () => %s)", u.Len(), z)
	case *types.Slice, *types.Pointer, *types.Signature:
		return "null"
	case *types.Map:
		return "{}"
	case *types.Struct:
		var fs []string
		for i := range u.NumFields() {
			fs = append(fs, fmt.Sprintf("%s: %s", t.fieldName(u, i), t.zero(u.Field(i).Type(), at)))
		}
		return "{ " + strings.Join(fs, ", ") + " }"
	}
	t.errf(at, "no zero value for %s in the subset", ty)
	return "undefined"
}

func jsDocType(ty types.Type) string {
	if n, ok := ty.(*types.Named); ok {
		return n.Obj().Name()
	}
	switch u := ty.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			return "boolean"
		case u.Info()&types.IsString != 0:
			return "string"
		}
		return "number"
	case *types.Array:
		return jsDocType(u.Elem()) + "[]"
	case *types.Slice:
		return "?" + jsDocType(u.Elem()) + "[]"
	case *types.Pointer:
		return "?" + jsDocType(u.Elem())
	case *types.Map:
		return "Object<string, " + jsDocType(u.Elem()) + ">"
	case *types.Signature:
		return "Function"
	}
	return "any"
}

// clone copies an aggregate value (Go assignment semantics).
func (t *tr) clone(ty types.Type, s string, at ast.Node) string {
	switch u := ty.Underlying().(type) {
	case *types.Array:
		if isAggregate(u.Elem()) {
			n, ok := u.Elem().(*types.Named)
			if !ok {
				t.errf(at, "arrays of anonymous aggregates are not in the subset")
				return s
			}
			t.clones[n] = true
			return fmt.Sprintf("%s.map($clone_%s)", s, n.Obj().Name())
		}
		return s + ".slice()"
	case *types.Struct:
		n, ok := ty.(*types.Named)
		if !ok {
			t.errf(at, "anonymous structs are not in the subset")
			return s
		}
		t.clones[n] = true
		return fmt.Sprintf("$clone_%s(%s)", n.Obj().Name(), s)
	}
	return s
}

func sortedNamed(m map[*types.Named]bool, done map[*types.Named]bool) []*types.Named {
	var ns []*types.Named
	for n := range m {
		if !done[n] {
			ns = append(ns, n)
		}
	}
	sort.Slice(ns, func(i, j int) bool { return ns[i].Obj().Name() < ns[j].Obj().Name() })
	return ns
}

func (t *tr) emitClones() {
	done := map[*types.Named]bool{}
	for len(done) < len(t.clones) {
		for _, n := range sortedNamed(t.clones, done) {
			done[n] = true
			st := n.Underlying().(*types.Struct)
			var fs []string
			for i := range st.NumFields() {
				fn := t.fieldName(st, i)
				v := "v." + fn
				if ft := st.Field(i).Type(); isAggregate(ft) {
					v = t.clone(ft, v, nil)
				}
				fs = append(fs, fmt.Sprintf("%s: %s", fn, v))
			}
			t.line("")
			t.line("function $clone_%s(v) {", n.Obj().Name())
			t.line("  return { %s };", strings.Join(fs, ", "))
			t.line("}")
		}
	}
}

// fromJSONName is the JS function that builds a full T from a JSON-parsed
// object (missing fields get Go's zero values, as json.Unmarshal leaves them).
func (t *tr) fromJSONName(n *types.Named, at ast.Node) string {
	if n.Obj().Pkg() != t.pkg {
		return t.needImport(n.Obj().Pkg().Path(), at) + "." + n.Obj().Name() + "_fromJSON"
	}
	t.fromJSON[n] = true
	return n.Obj().Name() + "_fromJSON"
}

func hasUnmarshal(ty types.Type) bool {
	ms := types.NewMethodSet(types.NewPointer(ty))
	return ms.Lookup(nil, "UnmarshalJSON") != nil
}

// fromJSONExpr converts the JSON value s to type ty.
func (t *tr) fromJSONExpr(ty types.Type, s string, at ast.Node) string {
	if n, ok := ty.(*types.Named); ok && hasUnmarshal(n) {
		if n.Obj().Pkg() != t.pkg {
			return t.needImport(n.Obj().Pkg().Path(), at) + "." + n.Obj().Name() + "_fromJSON(" + s + ")"
		}
		// A custom UnmarshalJSON: the package provides T_fromJSON itself
		// (a function in the subset, or //gojs:extern).
		if o := t.pkg.Scope().Lookup(n.Obj().Name() + "_fromJSON"); o == nil {
			t.errf(at, "%s has a custom UnmarshalJSON: write a func %s_fromJSON for the client", n.Obj().Name(), n.Obj().Name())
		}
		return n.Obj().Name() + "_fromJSON(" + s + ")"
	}
	switch u := ty.Underlying().(type) {
	case *types.Basic:
		return s + " ?? " + t.zero(ty, at)
	case *types.Struct:
		n, ok := ty.(*types.Named)
		if !ok {
			t.errf(at, "anonymous structs are not in the subset")
			return s
		}
		return t.fromJSONName(n, at) + "(" + s + ")"
	case *types.Pointer:
		if isAggregate(u.Elem()) || hasUnmarshal(u.Elem()) {
			return s + " == null ? null : " + t.fromJSONExpr(u.Elem(), s, at)
		}
		return s + " ?? null"
	case *types.Array:
		if needsConvert(u.Elem()) {
			return "(" + s + " ?? " + t.zero(ty, at) + ").map((e) => " + t.fromJSONExpr(u.Elem(), "e", at) + ")"
		}
		return s + " ?? " + t.zero(ty, at)
	case *types.Slice:
		if needsConvert(u.Elem()) {
			return s + " == null ? null : " + s + ".map((e) => " + t.fromJSONExpr(u.Elem(), "e", at) + ")"
		}
		return s + " ?? null"
	case *types.Map:
		if needsConvert(u.Elem()) {
			return "Object.fromEntries(Object.entries(" + s + " ?? {}).map(([k, e]) => [k, " + t.fromJSONExpr(u.Elem(), "e", at) + "]))"
		}
		return s + " ?? {}"
	}
	t.errf(at, "no JSON conversion for %s", ty)
	return s
}

// needsConvert: does a JSON value of this type need more than `?? zero`?
func needsConvert(ty types.Type) bool {
	if n, ok := ty.(*types.Named); ok && hasUnmarshal(n) {
		return true
	}
	switch u := ty.Underlying().(type) {
	case *types.Struct:
		return true
	case *types.Pointer:
		return needsConvert(u.Elem())
	case *types.Array:
		return needsConvert(u.Elem())
	case *types.Slice:
		return needsConvert(u.Elem())
	case *types.Map:
		return needsConvert(u.Elem())
	}
	return false
}

func (t *tr) emitFromJSON() {
	done := map[*types.Named]bool{}
	for len(done) < len(t.fromJSON) {
		for _, n := range sortedNamed(t.fromJSON, done) {
			done[n] = true
			st := n.Underlying().(*types.Struct)
			var fs []string
			for i := range st.NumFields() {
				f := st.Field(i)
				name := t.fieldName(st, i)
				key := jsonName(st, i)
				if key == "" && f.Exported() && reflectTag(st.Tag(i), "json") != "-" {
					key = f.Name() // encoding/json's default key
				}
				if key == "" {
					fs = append(fs, fmt.Sprintf("%s: %s", name, t.zero(f.Type(), nil)))
					continue
				}
				fs = append(fs, fmt.Sprintf("%s: %s", name, t.fromJSONExpr(f.Type(), "v"+jsProp(key), nil)))
			}
			t.line("")
			t.line("// %s_fromJSON builds a %s from parsed JSON (missing fields get Go's zero values).", n.Obj().Name(), n.Obj().Name())
			t.line("export function %s_fromJSON(v) {", n.Obj().Name())
			t.line("  v ??= {};")
			t.line("  return {")
			for _, f := range fs {
				t.line("    %s,", f)
			}
			t.line("  };")
			t.line("}")
		}
	}
}

func jsProp(key string) string {
	ok := key != ""
	for i, r := range key {
		if !(r == '_' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			ok = false
		}
	}
	if ok {
		return "." + key
	}
	b, _ := json.Marshal(key)
	return "[" + string(b) + "]"
}

var helperSrc = map[string]string{
	"$get":  "function $get(m, k, z) { return Object.hasOwn(m, k) ? m[k] : z; }",
	"$get2": "function $get2(m, k, z) { return Object.hasOwn(m, k) ? [m[k], true] : [z, false]; }",
}

func (t *tr) emitHelpers() {
	var hs []string
	for h := range t.helpers {
		hs = append(hs, h)
	}
	sort.Strings(hs)
	if len(hs) > 0 {
		t.line("")
	}
	for _, h := range hs {
		t.line("%s", helperSrc[h])
	}
}

func (t *tr) constLit(v constant.Value, ty types.Type) string {
	switch v.Kind() {
	case constant.Bool:
		return strconv.FormatBool(constant.BoolVal(v))
	case constant.String:
		b, _ := json.Marshal(constant.StringVal(v))
		return string(b)
	case constant.Int:
		if !isFloat(ty) {
			if i, ok := constant.Int64Val(v); ok && i <= 1<<53 && i >= -(1<<53) {
				if bits, signed := intKind(ty); bits > 0 && !signed && i > 9 {
					return "0x" + strconv.FormatInt(i, 16)
				}
				return strconv.FormatInt(i, 10)
			}
			return v.ExactString() + " /* beyond 2^53! */"
		}
		fallthrough
	case constant.Float:
		f, _ := constant.Float64Val(v)
		if f == float64(int64(f)) && f < 1e15 && f > -1e15 {
			return strconv.FormatInt(int64(f), 10)
		}
		s := strconv.FormatFloat(f, 'g', -1, 64)
		if i := strings.Index(s, "e"); i >= 0 { // 1e-04 -> 1e-4
			mant, exp := s[:i], s[i+1:]
			sign := ""
			if exp[0] == '-' || exp[0] == '+' {
				if exp[0] == '-' {
					sign = "-"
				}
				exp = exp[1:]
			}
			s = mant + "e" + sign + strings.TrimLeft(exp, "0")
		}
		return s
	}
	return "undefined"
}
