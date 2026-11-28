package game

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
)

// A Go twin of web/engine/netdelta.js's decoder, for tests: it checks that
// what the delta codec sends decodes to exactly the JSON snapshot.

type deltaReader struct {
	b   []byte
	off int
	err error
}

func (r *deltaReader) byte() byte {
	if r.off >= len(r.b) {
		r.err = fmt.Errorf("short message")
		return 0
	}
	c := r.b[r.off]
	r.off++
	return c
}

func (r *deltaReader) uv() uint64 {
	v, n := binary.Uvarint(r.b[min(r.off, len(r.b)):])
	if n <= 0 {
		r.err = fmt.Errorf("bad varint at %d", r.off)
		return 0
	}
	r.off += n
	return v
}

func unzz(u uint64) int64 { return int64(u>>1) ^ -int64(u&1) }

func (r *deltaReader) num(base int64, hot bool) int64 {
	if hot {
		return base + unzz(r.uv())
	}
	return unzz(r.uv())
}

func (r *deltaReader) u64() uint64 {
	if r.off+8 > len(r.b) {
		r.err = fmt.Errorf("short message")
		r.off = len(r.b)
		return 0
	}
	v := binary.LittleEndian.Uint64(r.b[r.off:])
	r.off += 8
	return v
}

func (r *deltaReader) exact(base int64, hot bool) int64 {
	if hot {
		return base ^ int64(r.uv())
	}
	return int64(r.u64())
}

func (r *deltaReader) str() string {
	n := int(r.uv())
	if r.off+n > len(r.b) {
		r.err = fmt.Errorf("short string")
		r.off = len(r.b)
		return ""
	}
	s := string(r.b[r.off : r.off+n])
	r.off += n
	return s
}

// decNV reads a value of n as a change from base.
func (r *deltaReader) decNV(n *nnode, base *nv, hot bool) nv {
	switch n.kind {
	case kInt, kAngle:
		return nv{i: r.num(base.i, hot)}
	case kBool:
		return nv{i: int64(r.byte())}
	case kStr:
		return nv{s: r.str()}
	case kExact:
		return nv{i: r.exact(base.i, hot)}
	case kVec3:
		k := make([]nv, 3)
		for j := range 3 {
			k[j].i = r.num(base.kids[j].i, hot)
		}
		return nv{kids: k}
	case kExact3:
		m := r.byte()
		k := slices.Clone(base.kids)
		for j := range 3 {
			if m&(1<<j) != 0 {
				k[j].i = r.exact(base.kids[j].i, hot)
			}
		}
		return nv{kids: k}
	case kGait:
		h := r.byte()
		k := make([]nv, h>>4)
		for j := range k {
			k[j].i = gaitAt(base, j)
			if h&(1<<j) != 0 {
				k[j].i += unzz(r.uv())
			}
		}
		return nv{kids: k}
	case kStruct:
		mask := r.uv()
		out := nv{kids: make([]nv, len(n.fields))}
		for j, f := range n.fields {
			if mask&(1<<j) != 0 {
				out.kids[j] = r.decNV(f.node, &base.kids[j], f.hot)
			} else {
				out.kids[j] = base.kids[j]
			}
		}
		return out
	case kPtr:
		if r.byte() == 0 {
			return nv{}
		}
		return nv{i: 1, kids: []nv{r.decNV(n.elem, baseOrZero(n.elem, base), hot)}}
	case kList:
		flags := r.byte()
		if flags == 0 {
			return nv{}
		}
		if n.idField >= 0 {
			en := entityNode(n)
			var bases []*nv
			if flags&2 == 0 {
				for j := range base.kids {
					bases = append(bases, &base.kids[j])
				}
			} else {
				byID := map[int64]*nv{}
				if base.i != 0 {
					for j := range base.kids {
						byID[base.kids[j].kids[n.idField].i] = &base.kids[j]
					}
				}
				cnt := int(r.uv())
				var prev int64
				for range cnt {
					prev += unzz(r.uv())
					b := byID[prev]
					if b == nil {
						z := zeroNV(en)
						b = &z
					}
					bases = append(bases, b)
				}
			}
			out := nv{i: 1, kids: make([]nv, len(bases))}
			for j, b := range bases {
				out.kids[j] = r.decNV(en, b, hot)
			}
			return out
		}
		l := int(r.uv())
		out := nv{i: 1, kids: make([]nv, l)}
		for j := range l {
			out.kids[j] = r.decNV(n.elem, elemOrZero(n.elem, base, j), hot)
		}
		return out
	case kMap:
		flags := r.byte()
		if flags == 0 {
			return nv{}
		}
		keys := base.keys
		if flags&2 != 0 {
			keys = make([]string, r.uv())
			for j := range keys {
				keys[j] = r.str()
			}
		}
		out := nv{i: 1, keys: keys, kids: make([]nv, len(keys))}
		for j, k := range keys {
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
			out.kids[j] = r.decNV(n.elem, be, hot)
		}
		return out
	}
	panic("kind")
}

// deltaDecoder: a client's codec state: the newest snapshot decoded (state,
// tick) and the ones before it in the base window (web/engine/netdelta.js).
type deltaDecoder struct {
	state *nv
	tick  int64
	ring  map[int64]*nv
}

// stale: a snapshot not newer than the newest decoded (a duplicate or a
// late one): dropped, not an error.
var errStale = fmt.Errorf("stale snapshot")

// errMissingBase: a delta from a snapshot the decoder doesn't have.
var errMissingBase = errors.New("missing base")

// apply decodes one message onto the decoder's state. It returns an error
// for a missing base or a checksum mismatch (the client then asks for a full).
func (d *deltaDecoder) apply(b []byte) error {
	r := &deltaReader{b: b}
	if r.byte() != deltaMagic {
		return fmt.Errorf("not a delta snapshot")
	}
	tick := int64(r.uv())
	baseTick := int64(-1)
	if back := int64(r.uv()); back > 0 {
		baseTick = tick - back
	}
	if r.off+4 > len(b) {
		return fmt.Errorf("short header")
	}
	sum := binary.LittleEndian.Uint32(b[r.off:])
	r.off += 4
	if d.state != nil && tick <= d.tick {
		return errStale
	}
	base := &zeroRoot
	if baseTick >= 0 {
		switch {
		case d.state != nil && d.tick == baseTick:
			base = d.state
		case d.ring[baseTick] != nil:
			base = d.ring[baseTick]
		default:
			return fmt.Errorf("%w %d (have %d)", errMissingBase, baseTick, d.tick)
		}
	}
	v := r.decNV(netSchema, base, false)
	if r.err != nil {
		return r.err
	}
	if r.off != len(b) {
		return fmt.Errorf("%d bytes left over", len(b)-r.off)
	}
	if got := checksumNV(netSchema, &v); got != sum {
		return fmt.Errorf("checksum %08x, want %08x", got, sum)
	}
	d.state, d.tick = &v, tick
	if d.ring == nil {
		d.ring = map[int64]*nv{}
	}
	d.ring[tick] = &v
	for k := range d.ring {
		if k <= tick-maxBaseWindow {
			delete(d.ring, k)
		}
	}
	return nil
}

// toJSON: v as encoding/json + json.Unmarshal into any would give it.
func toJSON(n *nnode, v *nv) any {
	switch n.kind {
	case kInt:
		return float64(v.i)
	case kBool:
		return v.i != 0
	case kStr:
		return v.s
	case kExact:
		return math.Float64frombits(uint64(v.i))
	case kAngle:
		return float64(v.i) / 1e4
	case kVec3:
		return []any{float64(v.kids[0].i) / 1000, float64(v.kids[1].i) / 1000, float64(v.kids[2].i) / 1000}
	case kExact3:
		return []any{math.Float64frombits(uint64(v.kids[0].i)), math.Float64frombits(uint64(v.kids[1].i)), math.Float64frombits(uint64(v.kids[2].i))}
	case kGait:
		out := []any{}
		for _, k := range v.kids {
			out = append(out, float64(k.i))
		}
		return out
	case kStruct:
		m := map[string]any{}
		structJSON(n, v, m)
		return m
	case kPtr:
		if v.i == 0 {
			return nil
		}
		return toJSON(n.elem, &v.kids[0])
	case kList:
		if v.i == 0 {
			return nil
		}
		en := n.elem
		if n.idField >= 0 {
			en = entityNode(n)
		}
		out := make([]any, len(v.kids))
		for j := range v.kids {
			out[j] = toJSON(en, &v.kids[j])
		}
		return out
	case kMap:
		if v.i == 0 {
			return nil
		}
		m := map[string]any{}
		for j, k := range v.keys {
			m[k] = toJSON(n.elem, &v.kids[j])
		}
		return m
	}
	panic("kind")
}

func structJSON(n *nnode, v *nv, m map[string]any) {
	for j, f := range n.fields {
		c := &v.kids[j]
		if f.inline {
			if c.i != 0 {
				structJSON(f.node.elem, &c.kids[0], m)
			}
			continue
		}
		if f.omitempty && emptyNV(f.node, c) {
			continue
		}
		m[f.name] = toJSON(f.node, c)
	}
}

// emptyNV: encoding/json's omitempty test.
func emptyNV(n *nnode, v *nv) bool {
	switch n.kind {
	case kInt, kBool, kAngle:
		return v.i == 0
	case kExact:
		return math.Float64frombits(uint64(v.i)) == 0
	case kStr:
		return v.s == ""
	case kPtr, kList, kMap:
		return v.i == 0 || len(v.kids) == 0 && n.kind != kPtr
	}
	return false // arrays of 3, structs
}
