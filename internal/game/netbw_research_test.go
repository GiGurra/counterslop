package game

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// Bandwidth research (tk_105679ca): candidate savings measured as variants
// of the delta codec on the bandwidth harness's matches (netbw_test.go).
// Each variant transforms the observer's snapshot (nv) before it is
// encoded, keeps its own base and history, and is measured over the same
// ticks as the codec today, so the table reads "what this change would
// save". Nothing here changes the wire; prototypes go in their own branch.
//
//	NETBWR=race,moonshot NETBW_SECS=30 go test ./internal/game -run TestNetBandwidthResearch -v
//
// Without NETBWR the test is skipped.

// bwVariant is one candidate.
type bwVariant struct {
	name string
	note string
	// every: send a snapshot every this many ticks (default 1).
	every int
	// back: diff against the snapshot this many sends back (an acked base
	// one round trip old) instead of the previous one; 0 = previous.
	back int
	// tf transforms this tick's root (a deep copy) for the observer; prev is
	// the variant's previous sent root (nil at first); tick counts sends.
	tf func(s *Server, obs *Player, root, prev *nv, tick int)
	// stream: deflate the variant's messages as one stream (context
	// takeover) instead of per message.
	stream bool
	// dict: per-message deflate with a preset dictionary (the first full).
	dict bool
}

type bwVarState struct {
	hist  []*nv // sent roots, newest last (for back)
	bytes int
	defl  int
	sends int
	encNs time.Duration
	zw    *flate.Writer
	zbuf  bytes.Buffer
	dict  []byte
	nFull int
	fullB int
}

func cloneNV(v *nv) *nv {
	c := *v
	if v.kids != nil {
		c.kids = make([]nv, len(v.kids))
		for i := range v.kids {
			c.kids[i] = *cloneNV(&v.kids[i])
		}
	}
	return &c
}

// fieldIdx finds a struct field by JSON name, looking through inline
// (embedded) fields: it returns the path of field indices to it.
func fieldIdx(n *nnode, name string) []int {
	for i, f := range n.fields {
		if f.name == name {
			return []int{i}
		}
	}
	for i, f := range n.fields {
		if f.inline {
			if sub := fieldIdx(f.node.elem, name); sub != nil {
				return append([]int{i}, sub...)
			}
		}
	}
	return nil
}

// forPath calls fn on every value at path ("round.race[].wr"; "[]" goes
// into every list element, pointers are followed) under v (of node n).
func forPath(n *nnode, v *nv, path string, fn func(n *nnode, v *nv)) {
	for n.kind == kPtr {
		if v.i == 0 {
			return
		}
		n, v = n.elem, &v.kids[0]
	}
	if path == "" {
		fn(n, v)
		return
	}
	if strings.HasPrefix(path, "[]") {
		if n.kind != kList || v.i == 0 {
			return
		}
		en := n.elem
		if n.idField >= 0 {
			en = entityNode(n)
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(path, "[]"), ".")
		for j := range v.kids {
			forPath(en, &v.kids[j], rest, fn)
		}
		return
	}
	name, rest, _ := strings.Cut(path, ".")
	if i := strings.Index(name, "[]"); i >= 0 {
		name, rest = name[:i], path[i:]
	}
	idx := fieldIdx(n, name)
	if idx == nil {
		panic("bw: no field " + name + " in path " + path)
	}
	for k, i := range idx {
		f := n.fields[i]
		n, v = f.node, &v.kids[i]
		if k < len(idx)-1 { // an inline pointer
			if v.i == 0 {
				return
			}
			n, v = n.elem, &v.kids[0]
		}
	}
	forPath(n, v, rest, fn)
}

// samePlayer: the entry with id in a list of players (nil: none).
func samePlayer(list *nv, n *nnode, id int64) *nv {
	if list == nil || list.i == 0 {
		return nil
	}
	for j := range list.kids {
		if list.kids[j].kids[n.idField].i == id {
			return &list.kids[j]
		}
	}
	return nil
}

// Countdowns as deadlines: a float of seconds left that drops every tick
// becomes the tick it ends at (constant while it runs; cold). The value is
// what the client would be sent: the deadline in ticks.
func asDeadline(tick int) func(n *nnode, v *nv) {
	return func(n *nnode, v *nv) {
		if n.kind != kExact || v.i == 0 {
			return
		}
		left := math.Float64frombits(uint64(v.i))
		v.i = int64(tick) + int64(math.Round(left*SimHz))
	}
}

// asStart: a float of seconds since something (the race clock) becomes the
// tick it started at.
func asStart(tick int) func(n *nnode, v *nv) {
	return func(n *nnode, v *nv) {
		if n.kind != kExact || v.i == 0 {
			return
		}
		since := math.Float64frombits(uint64(v.i))
		v.i = int64(tick) - int64(math.Round(since*SimHz))
	}
}

var playersField = func() int { return rootField("players") }()

// players: the root's player list node and value.
func playersOf(root *nv) (*nnode, *nv) {
	return netSchema.fields[playersField].node, &root.kids[playersField]
}

// posOf: a player entry's position in metres.
func posOf(en *nnode, p *nv) [3]float64 {
	i := fieldIdx(en, "p")[0]
	k := p.kids[i].kids
	return [3]float64{float64(k[0].i) / 1000, float64(k[1].i) / 1000, float64(k[2].i) / 1000}
}

// holdFar: players farther than dist from the observer keep their previous
// values except every k-th send (a lower send rate for far entities).
func holdFar(dist float64, k int) func(s *Server, obs *Player, root, prev *nv, tick int) {
	return func(s *Server, obs *Player, root, prev *nv, tick int) {
		if prev == nil || tick%k == 0 {
			return
		}
		n, list := playersOf(root)
		_, plist := playersOf(prev)
		en := entityNode(n)
		for j := range list.kids {
			p := &list.kids[j]
			id := p.kids[n.idField].i
			if int(id) == obs.ID {
				continue
			}
			q := posOf(en, p)
			d := math.Hypot(q[0]-obs.Pos[0], q[2]-obs.Pos[2])
			if d < dist {
				continue
			}
			if old := samePlayer(plist, n, id); old != nil {
				*p = *cloneNV(old)
			}
		}
	}
}

// cullFar: players (not the observer, not its team) farther than dist and
// not spotted by its team are not sent at all (interest management).
func cullFar(dist float64) func(s *Server, obs *Player, root, prev *nv, tick int) {
	return func(s *Server, obs *Player, root, prev *nv, tick int) {
		n, list := playersOf(root)
		en := entityNode(n)
		spotted := s.spotted()[obs.Team]
		keep := list.kids[:0:0]
		for j := range list.kids {
			p := list.kids[j]
			id := int(p.kids[n.idField].i)
			q := s.players[id]
			if q == nil || id == obs.ID || q.Team == obs.Team || slices.Contains(spotted, id) {
				keep = append(keep, p)
				continue
			}
			pos := posOf(en, &p)
			if math.Hypot(pos[0]-obs.Pos[0], pos[2]-obs.Pos[2]) < dist {
				keep = append(keep, p)
			}
		}
		list.kids = keep
	}
}

// coarse: int values at path rounded to step (positions in cm: step 10 of mm).
func coarse(path string, step int64) func(s *Server, obs *Player, root, prev *nv, tick int) {
	return func(s *Server, obs *Player, root, prev *nv, tick int) {
		forPath(netSchema, root, path, func(n *nnode, v *nv) {
			r := func(x int64) int64 { return int64(math.Round(float64(x)/float64(step))) * step }
			switch n.kind {
			case kVec3:
				for j := range v.kids {
					v.kids[j].i = r(v.kids[j].i)
				}
			default:
				v.i = r(v.i)
			}
		})
	}
}

// exactAs: exact floats at path sent at a fixed precision instead (1/scale).
func exactAs(path string, scale float64) func(s *Server, obs *Player, root, prev *nv, tick int) {
	return func(s *Server, obs *Player, root, prev *nv, tick int) {
		forPath(netSchema, root, path, func(n *nnode, v *nv) {
			q := func(x *nv) {
				f := math.Float64frombits(uint64(x.i))
				x.i = int64(math.Round(f * scale))
			}
			switch n.kind {
			case kExact:
				q(v)
			case kExact3:
				for j := range v.kids {
					q(&v.kids[j])
				}
			case kStruct:
				for j, f := range n.fields {
					switch f.node.kind {
					case kExact:
						q(&v.kids[j])
					case kExact3:
						for k := range v.kids[j].kids {
							q(&v.kids[j].kids[k])
						}
					}
				}
			}
		})
	}
}

// holdPath: values at path keep their previous send's value except every k-th.
func holdPath(path string, k int) func(s *Server, obs *Player, root, prev *nv, tick int) {
	return func(s *Server, obs *Player, root, prev *nv, tick int) {
		if prev == nil || tick%k == 0 {
			return
		}
		var olds []nv
		forPath(netSchema, prev, path, func(_ *nnode, v *nv) { olds = append(olds, *cloneNV(v)) })
		i := 0
		forPath(netSchema, root, path, func(_ *nnode, v *nv) {
			if i < len(olds) && len(olds[i].kids) == len(v.kids) {
				*v = olds[i]
			}
			i++
		})
	}
}

func chain(fs ...func(s *Server, obs *Player, root, prev *nv, tick int)) func(s *Server, obs *Player, root, prev *nv, tick int) {
	return func(s *Server, obs *Player, root, prev *nv, tick int) {
		for _, f := range fs {
			f(s, obs, root, prev, tick)
		}
	}
}

// deadlines: every countdown and clock in the snapshot as a tick deadline / start.
func deadlines(s *Server, obs *Player, root, prev *nv, tick int) {
	for _, p := range []string{"round.left", "round.buy", "round.race[].wr", "nades[].left"} {
		forPath(netSchema, root, p, asDeadline(int(s.tick)))
	}
	forPath(netSchema, root, "round.rt", asStart(int(s.tick)))
}

var bwVariants = []bwVariant{
	{name: "today", note: "the delta codec as it is"},
	{name: "deadlines", note: "countdowns/clocks as tick deadlines (round left/buy/rt, race wr, nade left)", tf: deadlines},
	{name: "gait/3", note: "players' gait at 33 Hz", tf: holdPath("players[].g", 3)},
	{name: "pos cm", note: "others' positions in cm (players, cars)", tf: chain(coarse("players[].p", 10), coarse("cars[].p", 10))},
	{name: "far/3 >30m", note: "players over 30 m away at 33 Hz", tf: holdFar(30, 3)},
	{name: "cull >40m", note: "enemies over 40 m away and not spotted: not sent", tf: cullFar(40)},
	{name: "me.cb mm", note: "own car's state at mm/1e-4 instead of exact", tf: exactAs("me.cb", 1e4)},
	{name: "cars v/3", note: "cars' velocity, yaw rate, steer, flags at 33 Hz", tf: chain(holdPath("cars[].v", 3), holdPath("cars[].yv", 3), holdPath("cars[].st", 3), holdPath("cars[].f", 3))},
	{name: "50 Hz", note: "a snapshot every 2nd tick", every: 2},
	{name: "33 Hz", note: "a snapshot every 3rd tick", every: 3},
	{name: "acked 5", note: "base = the send 5 back (50 ms RTT: the acked-base fix)", back: 5},
	{name: "acked 10", note: "base = the send 10 back (100 ms RTT)", back: 10},
	{name: "acked 20", note: "base = the send 20 back (200 ms RTT)", back: 20},
	{name: "stream defl", note: "deflate with context takeover (one stream)", stream: true},
	{name: "dict defl", note: "per-message deflate, the first full as a preset dictionary", dict: true},
	{name: "combo", note: "deadlines + gait/3 + far/3 + pos cm", tf: chain(deadlines, holdPath("players[].g", 3), holdFar(30, 3), coarse("players[].p", 10), coarse("cars[].p", 10))},
}

func TestNetBandwidthResearch(t *testing.T) {
	want := os.Getenv("NETBWR")
	if want == "" {
		t.Skip("NETBWR=race,moonshot,... runs the bandwidth research variants")
	}
	secs := 20
	if v := os.Getenv("NETBW_SECS"); v != "" {
		fmt.Sscan(v, &secs)
	}
	var out strings.Builder
	for _, m := range bwMatches {
		if want != "1" && !slices.Contains(strings.Split(want, ","), m.name) {
			continue
		}
		s, obs := m.build(t)
		// a listener: a human spectator, to count the event messages (bots get none)
		ear := s.join("10.99.0.1")
		ear.Name, ear.ready, ear.Team = "ear", true, "spectator"
		s.Step(m.warm)
		evBytes, evMsgs := map[string]int{}, map[string]int{}
		drainEar := func() {
			for {
				select {
				case b := <-ear.send:
					if len(b) == 0 || b[0] == deltaMagic {
						continue
					}
					var h struct {
						T string `json:"t"`
					}
					_ = json.Unmarshal(b, &h)
					if h.T == "snap" {
						continue
					}
					evBytes[h.T] += len(b)
					evMsgs[h.T]++
				default:
					return
				}
			}
		}
		drainEar()
		clear(evBytes)
		clear(evMsgs)
		st := make([]*bwVarState, len(bwVariants))
		for i := range bwVariants {
			st[i] = &bwVarState{}
			st[i].zw, _ = flate.NewWriter(&st[i].zbuf, flate.BestSpeed)
		}
		var scratch bytes.Buffer
		zw, _ := flate.NewWriter(&scratch, flate.BestSpeed)
		ticks := secs * SimHz
		for i := range ticks {
			s.Step(1)
			drainEar()
			var list []*Player
			for _, p := range s.ordered() {
				if p.ready {
					list = append(list, p)
				}
			}
			w := s.deltaWorldOf(list)
			base := s.deltaRoot(w, obs, s.spotted())
			for vi, v := range bwVariants {
				vs := st[vi]
				every := max(v.every, 1)
				if i%every != 0 {
					continue
				}
				root := cloneNV(&base)
				var prev *nv
				if len(vs.hist) > 0 {
					prev = vs.hist[len(vs.hist)-1]
				}
				if v.tf != nil {
					v.tf(s, obs, root, prev, vs.sends)
				}
				// the base: the previous send (or back sends back); a full
				// about once a second as the codec does
				var b *nv
				full := vs.sends%(fullPeriod/every) == 0
				if !full {
					k := max(v.back, 1)
					if len(vs.hist) >= k {
						b = vs.hist[len(vs.hist)-k]
					}
				}
				t0 := time.Now() // wall: measuring encode cost
				bz := b
				if bz == nil {
					z := zeroNV(netSchema)
					bz = &z
				}
				msg := encNV(make([]byte, 0, 512), netSchema, root, bz, false)
				vs.encNs += time.Since(t0) // wall: measuring encode cost
				n := len(msg) + 9          // + the header: magic, tick, back, checksum
				vs.bytes += n
				if full {
					vs.nFull++
					vs.fullB += n
					if vs.dict == nil {
						vs.dict = slices.Clone(msg)
					}
				}
				switch {
				case v.stream:
					before := vs.zbuf.Len()
					vs.zw.Write(msg)
					vs.zw.Flush() // sync flush per message, as permessage-deflate with context takeover
					vs.defl += vs.zbuf.Len() - before + 9
				case v.dict:
					var db bytes.Buffer
					dw, _ := flate.NewWriterDict(&db, flate.BestSpeed, vs.dict)
					dw.Write(msg)
					dw.Flush()
					vs.defl += db.Len() + 9
				default:
					vs.defl += deflated(zw, &scratch, msg) + 9
				}
				vs.sends++
				vs.hist = append(vs.hist, root)
				if len(vs.hist) > 32 {
					vs.hist = vs.hist[1:]
				}
			}
		}
		fmt.Fprintf(&out, "\n%s (%d s, %d players): bytes per second per client\n", m.name, secs, len(s.players))
		fmt.Fprintf(&out, "  %-12s %9s %9s %7s %8s %8s  %s\n", "variant", "raw B/s", "defl B/s", "vs now", "B/send", "enc us", "")
		base := float64(st[0].bytes)
		for vi, v := range bwVariants {
			vs := st[vi]
			fmt.Fprintf(&out, "  %-12s %9.0f %9.0f %6.0f%% %8.0f %8.1f  %s\n", v.name, float64(vs.bytes)/float64(secs), float64(vs.defl)/float64(secs),
				100*(float64(vs.bytes)-base)/base, float64(vs.bytes)/float64(max(vs.sends, 1)), float64(vs.encNs.Microseconds())/float64(max(vs.sends, 1)), v.note)
		}
		var evKinds []string
		evTotal, evN := 0, 0
		for k := range evBytes {
			evKinds = append(evKinds, k)
			evTotal += evBytes[k]
			evN += evMsgs[k]
		}
		slices.SortFunc(evKinds, func(a, b string) int { return evBytes[b] - evBytes[a] })
		fmt.Fprintf(&out, "  events (a spectator, all json text messages): %.0f B/s in %.0f msgs/s:", float64(evTotal)/float64(secs), float64(evN)/float64(secs))
		for _, k := range evKinds[:min(len(evKinds), 8)] {
			fmt.Fprintf(&out, " %s %.0f B/s (%.1f/s)", k, float64(evBytes[k])/float64(secs), float64(evMsgs[k])/float64(secs))
		}
		fmt.Fprintln(&out)
		// welcome / maps.json: what a join costs
		wb, _ := json.Marshal(s.Map)
		fmt.Fprintf(&out, "  the map as sent in welcome / on a change: %d B json, %d B deflated\n", len(wb), deflated(zw, &scratch, wb))
	}
	t.Log(out.String())
}
