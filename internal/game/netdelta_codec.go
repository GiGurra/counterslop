package game

import (
	"encoding/binary"
	"reflect"
	"slices"
)

// deltaCodec (--net-codec delta): binary snapshots that carry only what
// changed since the last snapshot sent on that connection (a websocket is
// reliable and ordered, so the client has it), laid out by netSchema.
//
// Message: 0xD5, uvarint tick, uvarint ticks back to the base (0: a full
// one, a delta from the zero state), uint32 LE checksum (FNV-1a of the whole decoded
// state, hashNV), then snapMsg as a struct delta (encNV).
//
// Full snapshots never burst the server's egress: at most fullsPerTick(n)
// go out per tick for n connections (1 up to 100). A connection without a
// base (it just joined, or its client asked: a missing base or a checksum
// mismatch) goes first, oldest request first; the others wait a tick or two
// (they have nothing to draw yet), so a wave of joins is spread out. The
// rest of the budget is the periodic refresh, about once a second each: the
// connections whose last full is oldest (and at least fullPeriod ticks old).
// Any full resets that age, so joins and leaves rebalance by themselves.
const deltaMagic = 0xD5

// CodecDelta names the delta codec.
const CodecDelta = "delta"

const fullPeriod = SimHz // ticks between a connection's periodic fulls (~1 s)

// fullsPerTick: the most full snapshots per tick for n connections: enough
// for each to get one per fullPeriod.
func fullsPerTick(n int) int { return max(1, (n+fullPeriod-1)/fullPeriod) }

// deltaConn is one connection's codec state.
type deltaConn struct {
	base      *nv         // the last snapshot enqueued to it (nil: it needs a full)
	baseWorld *deltaWorld // ... whose shared part was this tick's world
	baseTick  int64
	lastFull  int64 // tick of its last full
	waitFrom  int64 // tick it started needing a full (event queue order)
	fulls     int   // fulls sent (tests)
	deferred  int   // snapshots deferred a tick by pacing (tests)
}

// deltaWorld: one tick's recipient-independent part of the snapshot.
type deltaWorld struct {
	root nv // snapMsg with Spot and Me empty
	// enc caches the shared fields' encoding per base world (nil: a full),
	// so recipients with the same base cost one encode.
	enc map[*deltaWorld][][]byte
	// hash: the shared fields' canonical bytes (hashNV), per field; hashPre:
	// the checksum's FNV state over the fields before the first per-recipient
	// one (hashFrom), the same for everyone.
	hash     [][]byte
	hashPre  uint32
	hashFrom int
}

func fnvFeed(h uint32, bs []byte) uint32 {
	for _, c := range bs {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

type deltaCodec struct{}

func (deltaCodec) name() string { return CodecDelta }

// The root's fields that differ per recipient.
var (
	rootSpot = rootField("spot")
	rootMe   = rootField("me")
)

func rootField(name string) int {
	for i, f := range netSchema.fields {
		if f.name == name {
			return i
		}
	}
	panic("netschema: no root field " + name)
}

func (deltaCodec) send(s *Server, list []*Player, _ bool) {
	list = slices.Clone(list)
	slices.SortFunc(list, func(a, b *Player) int { return a.ID - b.ID })
	m := snapMsg{T: "snap", Time: s.clock().UnixMilli(), Tick: s.tick, Players: list, Items: s.items, Nades: s.nades}
	r := s.roundMsgOf(s.clock())
	m.Round = &r
	w := &deltaWorld{root: valueNV(netSchema, reflect.ValueOf(m), false), enc: map[*deltaWorld][][]byte{}}
	w.hash = make([][]byte, len(netSchema.fields))
	w.hashPre, w.hashFrom = 2166136261, -1
	for j, f := range netSchema.fields {
		if j == rootSpot || j == rootMe {
			if w.hashFrom < 0 {
				w.hashFrom = j
			}
			continue
		}
		w.hash[j] = hashNV(nil, f.node, &w.root.kids[j])
		if w.hashFrom < 0 {
			w.hashPre = fnvFeed(w.hashPre, w.hash[j])
		}
	}
	spotted := s.spotted()

	var conns []*Player
	for _, p := range s.players {
		if !p.Bot && p.send != nil {
			conns = append(conns, p)
		}
	}
	slices.SortFunc(conns, func(a, b *Player) int { return a.ID - b.ID })
	full := s.pickFulls(conns)

	type out struct {
		p    *Player
		b    []byte
		root *nv
		full bool
	}
	var outs []out
	for _, p := range conns {
		d := &p.delta
		isFull := full[p]
		if d.base == nil && !isFull {
			continue // waiting for its full
		}
		root := nv{kids: slices.Clone(w.root.kids)}
		var spot *[]int
		if ids, ok := spotted[p.Team]; ok {
			ids = slices.Clone(ids)
			slices.Sort(ids) // a set (in no particular order): sorted, it only changes when it changes
			spot = &ids
		}
		root.kids[rootSpot] = valueNV(netSchema.fields[rootSpot].node, reflect.ValueOf(spot), true)
		root.kids[rootMe] = valueNV(netSchema.fields[rootMe].node, reflect.ValueOf(s.meStateOf(p)), true)

		var base *nv
		var bw *deltaWorld
		if !isFull {
			base, bw = d.base, d.baseWorld
		}
		outs = append(outs, out{p, deltaEncode(w, &root, base, bw, s.tick, d.baseTick), &root, isFull})
	}
	// Pacing: a tick whose snapshots would go over the budget (a round
	// restart or a wave of joins changes everyone's state at once) defers
	// some connections' snapshot to the next tick; theirs then covers both
	// (a delta from their last one). Fulls (counted already) go first, then
	// the connections longest without one; none waits more than paceMaxSkip
	// ticks, so a burst drains over a few ticks instead of in one.
	slices.SortStableFunc(outs, func(a, b out) int {
		if a.full != b.full {
			if a.full {
				return -1
			}
			return 1
		}
		return int(a.p.delta.baseTick - b.p.delta.baseTick) // the longest without one first
	})
	sent, demand, deltas := 0, 0, 0
	for _, o := range outs {
		if !o.full {
			demand += len(o.b)
			deltas++
		}
	}
	// Deltas only: fulls have their own budget, a count (with few connections
	// one full is more than a tick of deltas; it must not hold them back).
	budget := max(paceFactor*s.deltaPace*float64(deltas), paceFloor)
	for _, o := range outs {
		d := &o.p.delta
		if !o.full && d.base != nil && s.tick-d.baseTick <= paceMaxSkip && budget > 0 && float64(sent+len(o.b)) > budget {
			d.deferred++
			continue
		}
		if trySend(o.p, o.b) {
			if !o.full {
				sent += len(o.b)
			}
			d.base, d.baseWorld, d.baseTick = o.root, w, s.tick
			if o.full {
				d.lastFull = s.tick
				d.fulls++
			}
		}
	}
	// The mean follows what the deltas would cost, not what went out (else
	// deferring would shrink the budget and defer more).
	// Per connection, so joins and leaves don't move the budget off.
	if deltas > 0 {
		per := float64(demand) / float64(deltas)
		if s.deltaPace == 0 {
			s.deltaPace = per
		} else {
			s.deltaPace += paceAlpha * (per - s.deltaPace)
		}
	}
}

// Pacing: a tick sends at most paceFactor times the running mean of what a
// delta costs per connection (an EWMA with paceAlpha per tick, ~1 s), times
// the connections getting one.
const (
	paceFactor  = 1.2
	paceAlpha   = 0.01
	paceMaxSkip = 3 // ticks a connection may go without a snapshot (then it goes, budget or not)
	// paceFloor: a tick may always send this much (400 KB/s): a small game's
	// deltas are tiny and swing with every join or first step, which is no
	// burst worth a late snapshot.
	paceFloor = 4096
)

// pickFulls decides which connections get a full snapshot this tick.
func (s *Server) pickFulls(conns []*Player) map[*Player]bool {
	full := map[*Player]bool{}
	var waiting, have []*Player
	for _, p := range conns {
		if p.delta.base == nil {
			if p.delta.waitFrom == 0 {
				p.delta.waitFrom = s.tick + 1 // +1: 0 means "not waiting"
			}
			waiting = append(waiting, p)
		} else {
			have = append(have, p)
		}
	}
	budget := fullsPerTick(len(conns))
	slices.SortStableFunc(waiting, func(a, b *Player) int { return int(a.delta.waitFrom - b.delta.waitFrom) })
	for _, p := range waiting[:min(len(waiting), budget)] {
		full[p] = true
		p.delta.waitFrom = 0
	}
	budget -= len(full)
	slices.SortStableFunc(have, func(a, b *Player) int { return int(a.delta.lastFull - b.delta.lastFull) })
	for _, p := range have[:min(len(have), budget)] {
		if s.tick-p.delta.lastFull >= fullPeriod {
			full[p] = true
		}
	}
	return full
}

// requestFull: the client lacks a base (or its state failed the checksum).
func (p *Player) requestFull() { p.delta.base, p.delta.baseWorld = nil, nil }

// deltaEncode builds one message: root (this recipient's snapshot) as a
// delta from base (nil: from the zero state, a full one).
func deltaEncode(w *deltaWorld, root, base *nv, bw *deltaWorld, tick, baseTick int64) []byte {
	b := make([]byte, 0, 256)
	b = append(b, deltaMagic)
	b = binary.AppendUvarint(b, uint64(tick))
	if base == nil {
		b = binary.AppendUvarint(b, 0)
	} else {
		b = binary.AppendUvarint(b, uint64(tick-baseTick))
	}
	// checksum of root: the shared fields' cached bytes, the others hashed here
	h := w.hashPre
	for j := w.hashFrom; j < len(netSchema.fields); j++ {
		if w.hash[j] != nil {
			h = fnvFeed(h, w.hash[j])
		} else {
			h = fnvFeed(h, hashNV(nil, netSchema.fields[j].node, &root.kids[j]))
		}
	}
	b = binary.LittleEndian.AppendUint32(b, h)

	zero := zeroRoot
	if base == nil {
		base, bw = &zero, nil
	}
	shared, ok := w.enc[bw]
	if !ok {
		shared = make([][]byte, len(netSchema.fields))
		for j, f := range netSchema.fields {
			if j == rootSpot || j == rootMe || eqNV(&w.root.kids[j], &base.kids[j]) {
				continue
			}
			shared[j] = encNV(nil, f.node, &w.root.kids[j], &base.kids[j], f.hot)
		}
		w.enc[bw] = shared
	}
	var mask uint64
	var own [2][]byte
	for j, f := range netSchema.fields {
		switch j {
		case rootSpot, rootMe:
			if !eqNV(&root.kids[j], &base.kids[j]) {
				mask |= 1 << j
				e := encNV(nil, f.node, &root.kids[j], &base.kids[j], f.hot)
				if j == rootSpot {
					own[0] = e
				} else {
					own[1] = e
				}
			}
		default:
			if shared[j] != nil {
				mask |= 1 << j
			}
		}
	}
	b = binary.AppendUvarint(b, mask)
	for j := range netSchema.fields {
		if mask&(1<<j) == 0 {
			continue
		}
		switch j {
		case rootSpot:
			b = append(b, own[0]...)
		case rootMe:
			b = append(b, own[1]...)
		default:
			b = append(b, shared[j]...)
		}
	}
	return b
}

var zeroRoot = zeroNV(netSchema)
