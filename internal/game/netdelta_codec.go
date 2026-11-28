package game

import (
	"encoding/binary"
	"reflect"
	"slices"
)

// deltaCodec (--net-codec delta): binary snapshots that carry only what
// changed since the newest snapshot the client acked (deltaConn), laid out
// by netSchema.
//
// Message: 0xD5 (0xD6: kept as a base, keepMagic), uvarint tick, uvarint
// ticks back to the base (0: a full one, a delta from the zero state; else
// the newest one decoded or a kept one), uint32 LE checksum (FNV-1a of the whole decoded
// state, hashNV), then snapMsg as a struct delta (encNV).
//
// Fulls are spread: no egress bursts. At most fullsPerTick(n) go out per
// tick for n connections (1 up to 100). A connection without a base (it
// just joined, or its client asked: a missing base or a checksum mismatch)
// goes first, oldest request first; the others wait a tick or two (they have
// nothing to draw yet), so a wave of joins is spread out. The rest of the
// tick's fulls are the periodic refresh, about once a second each: the
// connections whose last full is oldest (and at least fullPeriod ticks old).
// Any full resets that age, so joins and leaves rebalance by themselves.
const deltaMagic = 0xD5

// CodecDelta names the delta codec.
const CodecDelta = "delta"

const fullPeriod = SimHz // ticks between a connection's periodic fulls (~1 s)

// fullsPerTick: the most full snapshots per tick for n connections: enough
// for each to get one per fullPeriod.
func fullsPerTick(n int) int { return max(1, (n+fullPeriod-1)/fullPeriod) }

// baseRing: how many snapshots per connection are kept as bases (both
// ends). Not every snapshot is kept: one every stride ticks (deltaConn.stride),
// sized from how old its acks are when they arrive, so the ring spans about a
// round trip and a half; a kept one goes out with keepMagic, so the client
// keeps the same ones (the newest baseRing it got: a subset of the server's,
// so a base the server picks from its acks is one the client still has).
const baseRing = 8

// keepMagic: a snapshot kept as a base on both ends (deltaMagic: not kept).
const keepMagic = 0xD6

// deltaSent: a snapshot kept as a base on a connection.
type deltaSent struct {
	tick  int64
	root  *nv
	world *deltaWorld // its shared part
	full  bool
}

// deltaConn is one connection's codec state. Each delta is based on the
// newest kept snapshot the client acked (its command packets and "sack"
// messages carry the newest kept tick it decoded, ackSnap), never on one only
// sent: the network may lose, duplicate and reorder snapshots (and acks), and
// a lost one then costs that frame, nothing more. Until anything is acked
// (just joined, asked for a full, or its acked base left the ring) a full goes
// out and the deltas after it are based on it.
type deltaConn struct {
	sent     []deltaSent // the kept ones, in tick order, at most baseRing
	last     *nv         // the last snapshot sent (the next shares its unchanged parts)
	lastKept int64       // tick of the newest kept one
	acked    int64       // the newest acked tick in sent (0: none)
	lapsed   int64       // tick its acked base left the ring (only a later full is a base)
	lastFull int64       // tick of its last full
	waitFrom int64       // tick it started needing a full (event queue order)
	fulls    int         // fulls sent (tests)
	lapses   int         // fulls sent because the acked base had left the ring (tests)
	acks     int         // acks taken (tests)
	ackAge   int64       // how old (ticks) its acks are when they arrive: a decaying max
	heard    bool        // an ack came (until then: a stride for a ~100 ms round trip)
}

// stride: ticks between kept snapshots, so that baseRing of them span
// twice the acks' age (1 up to a 30 ms round trip, 2 at 50 ms, 4 at 100 ms,
// 7 at 200 ms).
func (d *deltaConn) stride() int64 {
	if !d.heard {
		return 4
	}
	return max(1, (2*d.ackAge+baseRing-2)/(baseRing-1))
}

// keep: the snapshot of tick now is kept as a base (a full always is).
func (d *deltaConn) keep(now int64, full bool) bool {
	return full || len(d.sent) == 0 || now-d.lastKept >= d.stride()
}

// record adds a kept snapshot; the oldest leaves the ring (and with it the
// acked base, if it was that one: the next snapshot is a full).
func (d *deltaConn) record(e deltaSent) {
	d.sent = append(d.sent, e)
	d.lastKept = e.tick
	if len(d.sent) > baseRing {
		if d.sent[0].tick == d.acked {
			d.acked, d.lapsed = 0, e.tick
		}
		d.sent = slices.Delete(d.sent, 0, 1)
	}
}

// baseFor: the base for a delta: the acked snapshot, else the newest full
// sent since it lost its base (nil: none, it needs a full).
func (d *deltaConn) baseFor() *deltaSent {
	for i := len(d.sent) - 1; i >= 0; i-- {
		e := &d.sent[i]
		if d.acked > 0 {
			if e.tick == d.acked {
				return e
			}
		} else if e.full && e.tick > d.lapsed {
			return e
		}
	}
	return nil
}

// shareNV makes cur use prev's storage wherever the two are equal (the
// JS decoder's structural sharing), so the snapshots kept as bases cost
// what changed between them, not a whole tree each. It reports whether
// they were equal. Values are never mutated once built, so sharing is safe.
func shareNV(cur, prev *nv) bool {
	same := cur.i == prev.i && cur.s == prev.s && slices.Equal(cur.keys, prev.keys)
	if len(cur.kids) != len(prev.kids) {
		return false
	}
	for j := range cur.kids {
		if !shareNV(&cur.kids[j], &prev.kids[j]) {
			same = false
		}
	}
	if len(cur.kids) > 0 && (same || allShared(cur.kids, prev.kids)) {
		cur.kids = prev.kids
	}
	if same {
		cur.keys = prev.keys
	}
	return same
}

// allShared: every kid of a is b's (by storage), so a's slice can be b's.
func allShared(a, b []nv) bool {
	for j := range a {
		if !sameNV(&a[j], &b[j]) {
			return false
		}
	}
	return true
}

func sameNV(a, b *nv) bool {
	return a.i == b.i && a.s == b.s && len(a.kids) == len(b.kids) && (len(a.kids) == 0 || &a.kids[0] == &b.kids[0]) &&
		len(a.keys) == len(b.keys) && (len(a.keys) == 0 || &a.keys[0] == &b.keys[0])
}

// hasBase: a delta can go out.
func (d *deltaConn) hasBase() bool { return d.baseFor() != nil }

// ackSnap: at tick now, the client says it decoded the kept snapshot of tick
// t (an ack may come late, twice or out of order: only a newer one still in
// the ring counts). Its age sizes the stride, even when it is too old to be
// a base: the next ones will be.
func (p *Player) ackSnap(now, t int64) {
	d := &p.delta
	if t <= d.acked || t > now {
		return
	}
	d.heard = true
	if age := now - t; age > d.ackAge {
		d.ackAge = age
	} else {
		d.ackAge -= (d.ackAge - age) / 64 // shrinks back over a few seconds of acks
	}
	for i := range d.sent {
		if d.sent[i].tick == t {
			d.acked = t
			d.acks++
			return
		}
	}
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

// deltaWorldOf is this tick's shared snapshot (no spot, no me) for the
// ready players in list: what every connection's snapshot starts from, and
// what a replay records (tv.go). Callers hold s.mu.
func (s *Server) deltaWorldOf(list []*Player) *deltaWorld {
	list = slices.Clone(list)
	slices.SortFunc(list, func(a, b *Player) int { return a.ID - b.ID })
	m := snapMsg{T: "snap", Time: s.clock().UnixMilli(), Tick: s.tick, Players: list, Items: s.items, Nades: s.nades, Boats: s.boats, Cars: s.carsSent(), Gear: s.gear}
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
	return w
}

func (deltaCodec) send(s *Server, list []*Player, _ bool) {
	w := s.deltaWorldOf(list)
	if s.deltaPrev != nil { // the rings keep worlds of several ticks: store each change once
		shareNV(&w.root, &s.deltaPrev.root)
	}
	s.deltaPrev = w
	spotted := s.spotted()

	var conns []*Player
	for _, p := range s.players {
		if !p.Bot && p.send != nil {
			conns = append(conns, p)
		}
	}
	slices.SortFunc(conns, func(a, b *Player) int { return a.ID - b.ID })
	full := s.pickFulls(conns)

	// Every connection with a base gets its delta every tick, never held back
	// (a late delta is state its player waits for); only fulls are spread
	// (pickFulls).
	for _, p := range conns {
		d := &p.delta
		isFull := full[p]
		var b *deltaSent
		if !isFull {
			if b = d.baseFor(); b == nil {
				continue // waiting for its full
			}
		}
		root := s.deltaRoot(w, p, spotted)
		if d.last != nil {
			shareNV(&root.kids[rootSpot], &d.last.kids[rootSpot])
			shareNV(&root.kids[rootMe], &d.last.kids[rootMe])
		}

		var base *nv
		var bw *deltaWorld
		var baseTick int64
		if b != nil {
			base, bw, baseTick = b.root, b.world, b.tick
		}
		keep := d.keep(s.tick, isFull)
		if trySend(p, deltaEncode(w, &root, base, bw, s.tick, baseTick, keep)) {
			d.last = &root
			if keep {
				d.record(deltaSent{tick: s.tick, root: &root, world: w, full: isFull})
			}
			p.snapSent = s.tick
			if isFull {
				if d.lapsed > 0 && d.lastFull < d.lapsed {
					d.lapses++
				}
				d.lastFull = s.tick
				d.fulls++
			}
		}
	}
	// w may stay a base (deltaConn.sent) for a while: its encode and
	// checksum caches are this tick's only.
	w.enc, w.hash = nil, nil
}

// deltaRoot is p's snapshot: this tick's world plus p's team's spotted
// enemies and p's own state. Callers hold s.mu.
func (s *Server) deltaRoot(w *deltaWorld, p *Player, spotted map[string][]int) nv {
	root := nv{kids: slices.Clone(w.root.kids)}
	var spot *[]int
	if ids, ok := spotted[p.Team]; ok {
		ids = slices.Clone(ids)
		slices.Sort(ids) // a set (in no particular order): sorted, it only changes when it changes
		spot = &ids
	}
	root.kids[rootSpot] = valueNV(netSchema.fields[rootSpot].node, reflect.ValueOf(spot), true)
	root.kids[rootMe] = valueNV(netSchema.fields[rootMe].node, reflect.ValueOf(s.meStateOf(p)), true)
	return root
}

// pickFulls decides which connections get a full snapshot this tick.
func (s *Server) pickFulls(conns []*Player) map[*Player]bool {
	full := map[*Player]bool{}
	var waiting, have []*Player
	for _, p := range conns {
		if !p.delta.hasBase() {
			if p.delta.waitFrom == 0 {
				p.delta.waitFrom = s.tick + 1 // +1: 0 means "not waiting"
			}
			waiting = append(waiting, p)
		} else {
			have = append(have, p)
		}
	}
	n := fullsPerTick(len(conns))
	if s.fullsNow { // the lockstep harness's setup snapshot (SendSetupSnapshot): everyone at once
		n = len(conns)
	}
	slices.SortStableFunc(waiting, func(a, b *Player) int { return int(a.delta.waitFrom - b.delta.waitFrom) })
	for _, p := range waiting[:min(len(waiting), n)] {
		full[p] = true
		p.delta.waitFrom = 0
	}
	n -= len(full)
	slices.SortStableFunc(have, func(a, b *Player) int { return int(a.delta.lastFull - b.delta.lastFull) })
	for _, p := range have[:min(len(have), n)] {
		if s.tick-p.delta.lastFull >= fullPeriod {
			full[p] = true
		}
	}
	return full
}

// requestFull: the client lacks a base (or its state failed the checksum):
// nothing sent before is a base any more.
func (p *Player) requestFull() { p.delta.sent, p.delta.acked, p.delta.lapsed = nil, 0, 0 }

// deltaEncode builds one message: root (this recipient's snapshot) as a
// delta from base (nil: from the zero state, a full one); keep: the client
// keeps it as a base (keepMagic).
func deltaEncode(w *deltaWorld, root, base *nv, bw *deltaWorld, tick, baseTick int64, keep bool) []byte {
	b := make([]byte, 0, 256)
	if keep {
		b = append(b, keepMagic)
	} else {
		b = append(b, deltaMagic)
	}
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

// isDeltaSnap: b is a delta-codec snapshot (kept as a base or not).
func isDeltaSnap(b []byte) bool { return len(b) > 0 && (b[0] == deltaMagic || b[0] == keepMagic) }
