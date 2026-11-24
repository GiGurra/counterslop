package game

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"reflect"
	"slices"
	"strings"
)

// The delta codec's schema (docs/replication.md "The delta codec"): the
// snapshot's layout, compiled from the `net` tags of snapMsg and the types it
// carries. The encoder walks Go values with it; the client's decoder
// (web/engine/netdelta.js) reads the same layout from netschema.gen.js, which
// cmd/gojs -all writes from NetSchemaJS. One schema, two ends.

type nkind uint8

const (
	kInt    nkind = iota // any integer
	kBool                // a byte
	kStr                 // uvarint length + bytes
	kExact               // float64: its 8 bytes (hot: XOR the base's, a varint)
	kAngle               // float64 at 4 decimals: the integer of 1e-4 rad
	kVec3                // Vec3 in mm: three integers
	kExact3              // [3]float64 exact: a 3-bit mask + each changed one as kExact
	kGait                // Gait's quantised ints (trailing zeros trimmed): a byte (length<<4 | changed mask) + changed ones as deltas
	kStruct              // fields: a changed-field mask + each changed field
	kPtr                 // nil or the element
	kList                // a slice; of structs with an int "id": matched by id, else by index
	kMap                 // map[string]X, keys sorted
)

var nkindNames = [...]string{"int", "bool", "str", "exact", "angle", "vec3", "exact3", "gait", "struct", "ptr", "list", "map"}

type nfield struct {
	name      string // its JSON key ("" for an inlined embedded struct)
	index     int    // reflect field index
	omitempty bool
	hot       bool // numbers go as a delta from the base (hot) or whole (cold)
	inline    bool // an embedded *struct: its fields sit in the parent's JSON object
	node      *nnode
}

type nnode struct {
	kind    nkind
	elem    *nnode   // ptr, list, map
	fields  []nfield // struct
	idField int      // list of structs: the "id" field's index (-1: matched by index)
}

// netSchema is snapMsg's layout, compiled once.
var netSchema = compileNetSchema(reflect.TypeFor[snapMsg]())

func compileNetSchema(t reflect.Type) *nnode {
	return compileNode(t, "", map[reflect.Type]*nnode{})
}

func compileNode(t reflect.Type, prec string, seen map[reflect.Type]*nnode) *nnode {
	switch t {
	case reflect.TypeFor[Vec3]():
		return &nnode{kind: kVec3}
	case reflect.TypeFor[Gait]():
		return &nnode{kind: kGait}
	case reflect.TypeFor[[3]float64]():
		if prec != "exact" {
			panic(fmt.Sprintf("netschema: [3]float64 with precision %q", prec))
		}
		return &nnode{kind: kExact3}
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &nnode{kind: kInt}
	case reflect.Bool:
		return &nnode{kind: kBool}
	case reflect.String:
		return &nnode{kind: kStr}
	case reflect.Float64:
		switch prec {
		case "exact":
			return &nnode{kind: kExact}
		case "angle":
			return &nnode{kind: kAngle}
		}
		panic(fmt.Sprintf("netschema: float64 with precision %q", prec))
	case reflect.Pointer:
		return &nnode{kind: kPtr, elem: compileNode(t.Elem(), prec, seen)}
	case reflect.Slice:
		n := &nnode{kind: kList, elem: compileNode(t.Elem(), prec, seen), idField: -1}
		st := n.elem
		if st.kind == kPtr {
			st = st.elem
		}
		if st.kind == kStruct {
			for i, f := range st.fields {
				if f.name == "id" && f.node.kind == kInt {
					n.idField = i
				}
			}
		}
		return n
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			panic("netschema: maps need string keys: " + t.String())
		}
		return &nnode{kind: kMap, elem: compileNode(t.Elem(), prec, seen)}
	case reflect.Struct:
		if n, ok := seen[t]; ok {
			return n
		}
		n := &nnode{kind: kStruct, idField: -1}
		seen[t] = n
		for i := range t.NumField() {
			f := t.Field(i)
			tag, err := parseNetTag(f.Tag.Get("net"))
			if err != nil {
				panic(fmt.Sprintf("netschema: %s.%s: %v", t.Name(), f.Name, err))
			}
			if !tag.Sent() {
				continue
			}
			jname, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			nf := nfield{name: jname, index: i, omitempty: strings.Contains(","+opts+",", ",omitempty,"), hot: tag.Class == netHot}
			if f.Anonymous && jname == "" {
				nf.inline = true
			}
			nf.node = compileNode(f.Type, tag.Precision, seen)
			n.fields = append(n.fields, nf)
		}
		// Hot fields first: they change together most ticks, so a struct's
		// changed-field mask usually fits in one varint byte.
		slices.SortStableFunc(n.fields, func(a, b nfield) int {
			if a.hot == b.hot {
				return 0
			}
			if a.hot {
				return -1
			}
			return 1
		})
		if len(n.fields) > 63 {
			panic("netschema: more than 63 sent fields in " + t.Name())
		}
		return n
	}
	panic("netschema: unsupported type " + t.String())
}

// The schema as data for the client: nested {k, f: [{n, t, o, h, i}], e, id}.
func (n *nnode) descr(seen map[*nnode]bool) map[string]any {
	if seen[n] {
		panic("netschema: recursive types are not supported")
	}
	seen[n] = true
	defer delete(seen, n)
	d := map[string]any{"k": nkindNames[n.kind]}
	if n.elem != nil {
		d["e"] = n.elem.descr(seen)
	}
	if n.kind == kList && n.idField >= 0 {
		d["id"] = n.idField
	}
	if n.kind == kStruct {
		fs := []any{}
		for _, f := range n.fields {
			fd := map[string]any{"n": f.name, "t": f.node.descr(seen)}
			if f.omitempty {
				fd["o"] = 1
			}
			if f.hot {
				fd["h"] = 1
			}
			if f.inline {
				fd["i"] = 1
			}
			fs = append(fs, fd)
		}
		d["f"] = fs
	}
	return d
}

// netSchemaJSON is the schema descriptor; its hash goes in the welcome.
var netSchemaJSON = func() string {
	b, err := json.Marshal(netSchema.descr(map[*nnode]bool{}))
	if err != nil {
		panic(err)
	}
	return string(b)
}()

var netSchemaHash = func() string {
	h := fnv.New32a()
	h.Write([]byte(netSchemaJSON))
	return fmt.Sprintf("%08x", h.Sum32())
}()

// NetSchemaOut is where NetSchemaJS goes (relative to the module root).
const NetSchemaOut = "web/engine/netschema.gen.js"

// NetSchemaJS is web/engine/netschema.gen.js (cmd/gojs -all writes it, CI checks it).
func NetSchemaJS() []byte {
	return []byte("// Code generated by cmd/gojs -all from internal/game's net tags (netdelta.go). DO NOT EDIT.\n" +
		"// The delta codec's snapshot layout; the server sends NET_SCHEMA_HASH in its welcome.\n" +
		"export const NET_SCHEMA_HASH = \"" + netSchemaHash + "\";\n" +
		"export const NET_SCHEMA = " + netSchemaJSON + ";\n")
}

// nv is a value in the schema's terms: numbers quantised (exact floats as
// their bits), pointers/slices/maps present or not (i: 0 = nil).
type nv struct {
	i    int64
	s    string
	kids []nv     // struct fields, list elements, map values, vec3/exact3/gait parts, ptr target
	keys []string // map keys, sorted
}

func roundMM(v float64) int64 {
	x := float64(v * 1000)
	t := math.Trunc(x)
	if math.Abs(x-t) >= 0.5 { // = sim.RoundMM: halves away from zero
		if x < 0 {
			t--
		} else {
			t++
		}
	}
	return int64(t)
}

// zeroNV: n's zero value (what a full snapshot is a delta from).
func zeroNV(n *nnode) nv {
	switch n.kind {
	case kVec3, kExact3:
		return nv{kids: make([]nv, 3)}
	case kStruct:
		v := nv{kids: make([]nv, len(n.fields))}
		for i, f := range n.fields {
			v.kids[i] = zeroNV(f.node)
		}
		return v
	}
	return nv{}
}

// valueNV reads v (of n's type) into an nv. omitempty: an empty slice or map
// counts as nil (encoding/json leaves both out).
func valueNV(n *nnode, v reflect.Value, omitempty bool) nv {
	switch n.kind {
	case kInt:
		return nv{i: v.Int()}
	case kBool:
		if v.Bool() {
			return nv{i: 1}
		}
		return nv{}
	case kStr:
		return nv{s: v.String()}
	case kExact:
		return nv{i: int64(math.Float64bits(v.Float()))}
	case kAngle:
		return nv{i: int64(math.Round(v.Float() * 1e4))}
	case kVec3:
		return nv{kids: []nv{{i: roundMM(v.Index(0).Float())}, {i: roundMM(v.Index(1).Float())}, {i: roundMM(v.Index(2).Float())}}}
	case kExact3:
		k := make([]nv, 3)
		for j := range 3 {
			k[j].i = int64(math.Float64bits(v.Index(j).Float()))
		}
		return nv{kids: k}
	case kGait:
		w := v.Interface().(Gait).wireInts()
		k := make([]nv, len(w))
		for j, x := range w {
			k[j].i = int64(x)
		}
		return nv{kids: k}
	case kStruct:
		out := nv{kids: make([]nv, len(n.fields))}
		for i, f := range n.fields {
			out.kids[i] = valueNV(f.node, v.Field(f.index), f.omitempty)
		}
		return out
	case kPtr:
		if v.IsNil() {
			return nv{}
		}
		return nv{i: 1, kids: []nv{valueNV(n.elem, v.Elem(), false)}}
	case kList:
		if v.IsNil() || (omitempty && v.Len() == 0) {
			return nv{}
		}
		out := nv{i: 1, kids: make([]nv, v.Len())}
		for j := range v.Len() {
			e := v.Index(j)
			if n.elem.kind == kPtr && n.idField >= 0 { // a list of entities: the element itself
				if e.IsNil() {
					out.kids[j] = zeroNV(n.elem.elem)
				} else {
					out.kids[j] = valueNV(n.elem.elem, e.Elem(), false)
				}
				continue
			}
			out.kids[j] = valueNV(n.elem, e, false)
		}
		return out
	case kMap:
		if v.IsNil() || (omitempty && v.Len() == 0) {
			return nv{}
		}
		keys := make([]string, 0, v.Len())
		for _, k := range v.MapKeys() {
			keys = append(keys, k.String())
		}
		slices.Sort(keys)
		out := nv{i: 1, keys: keys, kids: make([]nv, len(keys))}
		for j, k := range keys {
			out.kids[j] = valueNV(n.elem, v.MapIndex(reflect.ValueOf(k).Convert(v.Type().Key())), false)
		}
		return out
	}
	panic("netschema: kind")
}

// entityNode: the struct an id-matched list's elements are (through the pointer).
func entityNode(n *nnode) *nnode {
	if n.elem.kind == kPtr {
		return n.elem.elem
	}
	return n.elem
}

func eqNV(a, b *nv) bool {
	if a.i != b.i || a.s != b.s || len(a.kids) != len(b.kids) || !slices.Equal(a.keys, b.keys) {
		return false
	}
	for j := range a.kids {
		if !eqNV(&a.kids[j], &b.kids[j]) {
			return false
		}
	}
	return true
}

func zz(v int64) uint64 { return uint64(v<<1) ^ uint64(v>>63) }

// putExact: a float64's bits. Hot: XOR the base's as a varint (a value
// that moved a little keeps its sign, exponent and top mantissa bits, so
// only the low bits differ: ~6 bytes instead of 8). Cold: the 8 bytes.
func putExact(b []byte, cur, base int64, hot bool) []byte {
	if hot {
		return binary.AppendUvarint(b, uint64(cur^base))
	}
	return binary.LittleEndian.AppendUint64(b, uint64(cur))
}

func putNum(b []byte, cur, base int64, hot bool) []byte {
	if hot {
		return binary.AppendUvarint(b, zz(cur-base))
	}
	return binary.AppendUvarint(b, zz(cur))
}

// encNV appends cur as a change from base (both of node n; base is the zero
// value for a full snapshot). Inside a struct only changed fields get here.
func encNV(b []byte, n *nnode, cur, base *nv, hot bool) []byte {
	switch n.kind {
	case kInt, kAngle:
		return putNum(b, cur.i, base.i, hot)
	case kBool:
		return append(b, byte(cur.i))
	case kStr:
		b = binary.AppendUvarint(b, uint64(len(cur.s)))
		return append(b, cur.s...)
	case kExact:
		return putExact(b, cur.i, base.i, hot)
	case kVec3:
		for j := range 3 {
			b = putNum(b, cur.kids[j].i, base.kids[j].i, hot)
		}
		return b
	case kExact3:
		var m byte
		for j := range 3 {
			if cur.kids[j].i != base.kids[j].i {
				m |= 1 << j
			}
		}
		b = append(b, m)
		for j := range 3 {
			if m&(1<<j) != 0 {
				b = putExact(b, cur.kids[j].i, base.kids[j].i, hot)
			}
		}
		return b
	case kGait:
		var m byte
		for j := range cur.kids {
			if cur.kids[j].i != gaitAt(base, j) {
				m |= 1 << j
			}
		}
		b = append(b, byte(len(cur.kids))<<4|m)
		for j := range cur.kids {
			if m&(1<<j) != 0 {
				b = binary.AppendUvarint(b, zz(cur.kids[j].i-gaitAt(base, j)))
			}
		}
		return b
	case kStruct:
		var mask uint64
		for j := range n.fields {
			if !eqNV(&cur.kids[j], &base.kids[j]) {
				mask |= 1 << j
			}
		}
		b = binary.AppendUvarint(b, mask)
		for j, f := range n.fields {
			if mask&(1<<j) != 0 {
				b = encNV(b, f.node, &cur.kids[j], &base.kids[j], f.hot)
			}
		}
		return b
	case kPtr:
		if cur.i == 0 {
			return append(b, 0)
		}
		b = append(b, 1)
		be := baseOrZero(n.elem, base)
		return encNV(b, n.elem, &cur.kids[0], be, hot)
	case kList:
		if cur.i == 0 {
			return append(b, 0)
		}
		if n.idField >= 0 {
			en := entityNode(n)
			sameIDs := base.i != 0 && len(base.kids) == len(cur.kids)
			for j := 0; sameIDs && j < len(cur.kids); j++ {
				sameIDs = cur.kids[j].kids[n.idField].i == base.kids[j].kids[n.idField].i
			}
			if sameIDs {
				b = append(b, 1)
				for j := range cur.kids {
					b = encNV(b, en, &cur.kids[j], &base.kids[j], hot)
				}
				return b
			}
			b = append(b, 3)
			b = binary.AppendUvarint(b, uint64(len(cur.kids)))
			var prev int64
			for j := range cur.kids {
				id := cur.kids[j].kids[n.idField].i
				b = binary.AppendUvarint(b, zz(id-prev))
				prev = id
			}
			byID := map[int64]*nv{}
			if base.i != 0 {
				for j := range base.kids {
					byID[base.kids[j].kids[n.idField].i] = &base.kids[j]
				}
			}
			for j := range cur.kids {
				be := byID[cur.kids[j].kids[n.idField].i]
				if be == nil {
					z := zeroNV(en)
					be = &z
				}
				b = encNV(b, en, &cur.kids[j], be, hot)
			}
			return b
		}
		b = append(b, 1)
		b = binary.AppendUvarint(b, uint64(len(cur.kids)))
		for j := range cur.kids {
			b = encNV(b, n.elem, &cur.kids[j], elemOrZero(n.elem, base, j), hot)
		}
		return b
	case kMap:
		if cur.i == 0 {
			return append(b, 0)
		}
		if base.i != 0 && slices.Equal(cur.keys, base.keys) {
			b = append(b, 1)
		} else {
			b = append(b, 3)
			b = binary.AppendUvarint(b, uint64(len(cur.keys)))
			for _, k := range cur.keys {
				b = binary.AppendUvarint(b, uint64(len(k)))
				b = append(b, k...)
			}
		}
		for j, k := range cur.keys {
			var be *nv
			if base.i != 0 {
				if x, ok := slices.BinarySearch(base.keys, k); ok {
					be = &base.kids[x]
				}
			}
			if be == nil {
				z := zeroNV(n.elem)
				be = &z
			}
			b = encNV(b, n.elem, &cur.kids[j], be, hot)
		}
		return b
	}
	panic("netschema: kind")
}

// gaitAt: part j of a gait value (0 past its trimmed end).
func gaitAt(v *nv, j int) int64 {
	if j < len(v.kids) {
		return v.kids[j].i
	}
	return 0
}

func baseOrZero(n *nnode, ptr *nv) *nv {
	if ptr.i != 0 {
		return &ptr.kids[0]
	}
	z := zeroNV(n)
	return &z
}

func elemOrZero(n *nnode, list *nv, j int) *nv {
	if list.i != 0 && j < len(list.kids) {
		return &list.kids[j]
	}
	z := zeroNV(n)
	return &z
}

// hashNV feeds v's canonical bytes to h (the checksum both ends compute
// over the whole decoded state).
func hashNV(h []byte, n *nnode, v *nv) []byte {
	switch n.kind {
	case kInt, kAngle:
		return binary.AppendUvarint(h, zz(v.i))
	case kBool:
		return append(h, byte(v.i))
	case kStr:
		h = binary.AppendUvarint(h, uint64(len(v.s)))
		return append(h, v.s...)
	case kExact:
		return binary.LittleEndian.AppendUint64(h, uint64(v.i))
	case kVec3, kGait:
		h = binary.AppendUvarint(h, uint64(len(v.kids)))
		for j := range v.kids {
			h = binary.AppendUvarint(h, zz(v.kids[j].i))
		}
		return h
	case kExact3:
		for j := range 3 {
			h = binary.LittleEndian.AppendUint64(h, uint64(v.kids[j].i))
		}
		return h
	case kStruct:
		for j, f := range n.fields {
			h = hashNV(h, f.node, &v.kids[j])
		}
		return h
	case kPtr:
		if v.i == 0 {
			return append(h, 0)
		}
		return hashNV(append(h, 1), n.elem, &v.kids[0])
	case kList, kMap:
		if v.i == 0 {
			return append(h, 0)
		}
		h = append(h, 1)
		h = binary.AppendUvarint(h, uint64(len(v.kids)))
		en := n.elem
		if n.kind == kList && n.idField >= 0 {
			en = entityNode(n)
		}
		for j := range v.kids {
			if n.kind == kMap {
				h = binary.AppendUvarint(h, uint64(len(v.keys[j])))
				h = append(h, v.keys[j]...)
			}
			h = hashNV(h, en, &v.kids[j])
		}
		return h
	}
	panic("netschema: kind")
}

// checksumNV: FNV-1a (32 bits) of the canonical bytes.
func checksumNV(n *nnode, v *nv) uint32 {
	h := uint32(2166136261)
	for _, c := range hashNV(nil, n, v) {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}
