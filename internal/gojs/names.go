package gojs

import (
	"fmt"
	"go/types"
	"strconv"
	"strings"
	"unicode"
)

var jsReserved = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`break case catch class const continue debugger default delete do else
		enum export extends false finally for function if import in instanceof new null return super switch
		this throw true try typeof var void while with yield let static implements interface package private
		protected public await arguments eval undefined NaN Infinity Math Object Array Number String Boolean
		JSON Symbol Map Set Date Error`) {
		jsReserved[w] = true
	}
}

// declare gives a local object a JS name unique within the function (so Go
// shadowing like `x := x` never hits JS's temporal dead zone) that doesn't
// hide a package-level name or an import.
func (t *tr) declare(o types.Object) string {
	base := o.Name()
	if jsReserved[base] {
		base += "_"
	}
	n := base
	for i := 1; t.taken[n] || t.reservedTop[n]; i++ {
		n = fmt.Sprintf("%s$%d", base, i)
	}
	t.taken[n] = true
	t.names[o] = n
	t.locals[o] = true
	return n
}

func (t *tr) temp() string {
	t.tmp++
	n := fmt.Sprintf("$%d", t.tmp)
	t.taken[n] = true
	return n
}

// fieldName: the json tag if there is one (so JSON-fed objects line up),
// else the Go name with its leading capitals lowered (EyeH -> eyeH, ID -> id).
func (t *tr) fieldName(st *types.Struct, i int) string {
	if tag := jsonName(st, i); tag != "" {
		return tag
	}
	return lowerFirst(st.Field(i).Name())
}

func jsonName(st *types.Struct, i int) string {
	tag := strings.Split(reflectTag(st.Tag(i), "json"), ",")[0]
	if tag == "-" {
		return ""
	}
	return tag
}

func hasJSONTags(st *types.Struct) bool {
	for i := range st.NumFields() {
		if reflectTag(st.Tag(i), "json") != "" {
			return true
		}
	}
	return false
}

func reflectTag(tag, key string) string {
	for tag != "" {
		i := strings.IndexByte(tag, ':')
		if i < 0 {
			return ""
		}
		k := strings.TrimSpace(tag[:i])
		tag = tag[i+1:]
		v, err := strconv.QuotedPrefix(tag)
		if err != nil {
			return ""
		}
		tag = tag[len(v):]
		if k == key {
			s, _ := strconv.Unquote(v)
			return s
		}
	}
	return ""
}

func lowerFirst(s string) string {
	r := []rune(s)
	n := 0
	for n < len(r) && unicode.IsUpper(r[n]) {
		n++
	}
	if n > 1 && n < len(r) {
		n-- // HTTPServer -> httpServer
	}
	for i := range n {
		r[i] = unicode.ToLower(r[i])
	}
	return string(r)
}
