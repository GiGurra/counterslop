package game

import (
	"bytes"
	"compress/flate"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// The bandwidth harness: what one client receives per snapshot on a busy
// scripted match, with each codec, raw and deflated (per message, as the
// websocket's permessage-deflate sends it), and where the delta codec's
// bytes go (by field, entities folded: "players[].p" is every player's
// position). The observer is one of the bots (its own state is a player's
// in the match); bots get no snapshots, so the harness encodes them itself,
// as the codecs would.
//
//	NETBW=1 go test ./internal/game -run TestNetBandwidthReport -v    (make netbw)
//	NETBW=race,moonshot NETBW_SECS=60 ...                             pick matches, longer
//
// Without NETBW it runs each match for a few seconds as a smoke check.

// bwMatch is a scripted match: build sets up the server (bots, cvars) and
// returns the observer; warm is how long to run before measuring.
type bwMatch struct {
	name  string
	build func(t *testing.T) (*Server, *Player)
	warm  int // ticks
}

var bwMatches = []bwMatch{
	{"race", bwRace, 6 * SimHz},         // rc_merge_train, 8 bot cars racing (power-ups, gear)
	{"moonshot", bwMoonshot, 2 * SimHz}, // cq_moonshot 8v8 bots (MVP-1, the rocket)
	{"slop5v5", bwSlop, 2 * SimHz},      // de_slop 5v5 bots (a classic round)
}

func bwRace(t *testing.T) (*Server, *Player) {
	s := authServer(t)
	s.Map = maps.Find("rc_merge_train")()
	s.Seed(1)
	s.cvars["mp_respawn"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_race_laps"] = "3"
	s.cvars["bot_difficulty"] = "3"
	for range 4 {
		s.addAI(content.Vibe)
		s.addAI(content.Artisan)
	}
	return s, s.ordered()[0]
}

func bwMoonshot(t *testing.T) (*Server, *Player) {
	s := authServer(t)
	s.Map = maps.CqMoonshot()
	s.autoNav()
	s.Seed(1)
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["bot_difficulty"] = "1"
	for range 8 {
		s.addAI(content.Vibe)
		s.addAI(content.Artisan)
	}
	return s, s.ordered()[0]
}

func bwSlop(t *testing.T) (*Server, *Player) {
	s := authServer(t)
	s.Map = maps.Find("de_slop")()
	s.autoNav()
	s.Seed(1)
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["bot_difficulty"] = "2"
	for range 5 {
		s.addAI(content.Vibe)
		s.addAI(content.Artisan)
	}
	return s, s.ordered()[0]
}

// bwResult is one match's numbers.
type bwResult struct {
	snaps                              int
	jsonRaw, jsonDefl, delRaw, delDefl int
	delFulls, delFullBytes             int
	byPath                             map[string]int // delta bytes by field path (inclusive)
	players, cars, boats, gear         int            // at the end (context)
}

func deflated(zw *flate.Writer, buf *bytes.Buffer, b []byte) int {
	buf.Reset()
	zw.Reset(buf)
	zw.Write(b)
	zw.Flush()
	return buf.Len()
}

// netBandwidth runs m for warm + ticks ticks and measures the last ticks.
func netBandwidth(t *testing.T, m bwMatch, ticks int) bwResult {
	s, obs := m.build(t)
	s.Step(m.warm)
	r := bwResult{byPath: map[string]int{}}
	var buf bytes.Buffer
	zw, _ := flate.NewWriter(&buf, flate.BestSpeed)
	// NETBW_ACKLAG: the delta's base is the snapshot this many ticks back (the
	// client's newest ack: about its round trip in ticks; 1 = the last one sent).
	lag := 1
	if v := os.Getenv("NETBW_ACKLAG"); v != "" {
		fmt.Sscan(v, &lag)
	}
	type sent struct {
		root *nv
		w    *deltaWorld
		tick int64
	}
	var hist []sent
	for i := range ticks {
		s.Step(1)
		var list []*Player
		for _, p := range s.ordered() {
			if p.ready {
				list = append(list, p)
			}
		}
		// json: a full one every fullEvery ticks, pose snapshots between (jsonCodec.send)
		var jb []byte
		if s.tick%fullEvery == 0 {
			jb = s.jsonFullSnap(list)(obs)
		} else {
			jb = poseBytes(s.clock().UnixMilli(), s.tick, s.posesJSON(list), s.meBytes(obs))
		}
		r.jsonRaw += len(jb)
		r.jsonDefl += deflated(zw, &buf, jb)
		// delta: a full about once a second (pickFulls), deltas between
		w := s.deltaWorldOf(list)
		root := s.deltaRoot(w, obs, s.spotted())
		full := i%fullPeriod == 0
		var b0 *nv
		var bw0 *deltaWorld
		var baseTick int64
		if len(hist) > 0 && !full {
			h := hist[max(0, len(hist)-lag)]
			b0, bw0, baseTick = h.root, h.w, h.tick
		}
		full = b0 == nil
		db := deltaEncode(w, &root, b0, bw0, s.tick, baseTick, false)
		r.delRaw += len(db)
		r.delDefl += deflated(zw, &buf, db)
		if full {
			r.delFulls++
			r.delFullBytes += len(db)
		} else {
			bwAttribute(netSchema, &root, b0, false, "", r.byPath)
		}
		hist = append(hist, sent{&root, w, s.tick})
		if len(hist) > 128 {
			hist = hist[1:]
		}
		r.snaps++
	}
	r.players, r.cars, r.boats, r.gear = len(s.players), len(s.carsSent()), len(s.boats), len(s.gear)
	return r
}

// bwAttribute adds what encNV writes for cur (from base) to acc, by field
// path, inclusive: "players" is the whole list, "players[].p" its positions.
func bwAttribute(n *nnode, cur, base *nv, hot bool, path string, acc map[string]int) {
	if base == nil {
		z := zeroNV(n)
		base = &z
	}
	switch n.kind {
	case kStruct:
		for j, f := range n.fields {
			if eqNV(&cur.kids[j], &base.kids[j]) {
				continue
			}
			p := f.name
			if f.inline { // an embedded struct (roundMsg's *roundState): its fields sit in the parent
				p = "(embedded)"
			}
			if path != "" {
				p = path + "." + p
			}
			acc[p] += len(encNV(nil, f.node, &cur.kids[j], &base.kids[j], f.hot))
			bwAttribute(f.node, &cur.kids[j], &base.kids[j], f.hot, p, acc)
		}
	case kPtr:
		if cur.i != 0 {
			bwAttribute(n.elem, &cur.kids[0], baseOrZero(n.elem, base), hot, path, acc)
		}
	case kList:
		if cur.i == 0 {
			return
		}
		en := n.elem
		byID := map[int64]*nv{}
		if n.idField >= 0 {
			en = entityNode(n)
			if base.i != 0 {
				for j := range base.kids {
					byID[base.kids[j].kids[n.idField].i] = &base.kids[j]
				}
			}
		}
		for j := range cur.kids {
			var be *nv
			if n.idField >= 0 {
				be = byID[cur.kids[j].kids[n.idField].i]
			} else {
				be = elemOrZero(n.elem, base, j)
			}
			bwAttribute(en, &cur.kids[j], be, hot, path+"[]", acc)
		}
	case kMap:
		for j, k := range cur.keys {
			var be *nv
			if base.i != 0 {
				if x, ok := slices.BinarySearch(base.keys, k); ok {
					be = &base.kids[x]
				}
			}
			bwAttribute(n.elem, &cur.kids[j], be, hot, path+"{}", acc)
		}
	}
}

func TestNetBandwidthReport(t *testing.T) {
	want, secs := os.Getenv("NETBW"), 20
	if want == "" {
		secs = 2 // a smoke run: every match builds and measures
	}
	if v := os.Getenv("NETBW_SECS"); v != "" {
		fmt.Sscan(v, &secs)
	}
	var out strings.Builder
	fmt.Fprintf(&out, "\n%-9s %5s | %8s %8s | %8s %8s %6s | %6s %6s\n", "match", "secs", "json B", "defl B", "delta B", "defl B", "full B", "json/s", "dlt/s")
	for _, m := range bwMatches {
		if want != "" && want != "1" && !slices.Contains(strings.Split(want, ","), m.name) {
			continue
		}
		r := netBandwidth(t, m, secs*SimHz)
		n := float64(r.snaps)
		kbs := func(b int) float64 { return float64(b) / 1024 / float64(secs) }
		fmt.Fprintf(&out, "%-9s %5d | %8.0f %8.0f | %8.0f %8.0f %6.0f | %5.1fK %5.1fK   (%d players, %d cars, %d boats, %d gear)\n",
			m.name, secs, float64(r.jsonRaw)/n, float64(r.jsonDefl)/n, float64(r.delRaw)/n, float64(r.delDefl)/n,
			float64(r.delFullBytes)/float64(max(r.delFulls, 1)), kbs(r.jsonDefl), kbs(r.delRaw), r.players, r.cars, r.boats, r.gear)
		if want == "" {
			if r.delRaw == 0 || r.jsonRaw == 0 {
				t.Errorf("%s: nothing measured", m.name)
			}
			continue
		}
		type kv struct {
			k string
			v int
		}
		var top []kv
		for k, v := range r.byPath {
			if strings.Count(k, ".") <= 1 {
				top = append(top, kv{k, v})
			}
		}
		slices.SortFunc(top, func(a, b kv) int { return b.v - a.v })
		deltas := n - float64(r.delFulls)
		hdr := float64(r.delRaw-r.delFullBytes) / deltas
		fmt.Fprintf(&out, "  delta snapshots (%.0f B mean, fulls excluded), bytes by field per snapshot:\n", hdr)
		for _, e := range top[:min(len(top), 24)] {
			fmt.Fprintf(&out, "    %-24s %7.1f\n", e.k, float64(e.v)/deltas)
		}
	}
	t.Log(out.String())
}
