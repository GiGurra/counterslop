package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"
)

// lossyLink: one client's unreliable datagram link, both ways, in ticks:
// each packet is lost with p loss, sent twice with p dup, and delayed
// lag + 0..jitter ticks on its own (so packets overtake each other). hitch:
// the client reads nothing from tick hitchAt for hitch ticks (a main-thread
// stall), then everything that came meanwhile at once.
type lossyLink struct {
	loss, dup        float64
	dropFirst        int // the first n snapshots are lost
	lossFrom         int // no loss before this tick
	lag, jitter      int
	hitchAt, hitch   int
	rng              *rand.Rand
	down             map[int][][]byte // arrival tick -> snapshots
	up               map[int][]int64  // arrival tick -> acks (the newest decoded tick)
	errs, fulls, ok  int
	stale, lostSnaps int
	bytes            int // snapshot bytes sent to it
	p                *Player
}

func (l *lossyLink) send(q map[int][][]byte, now int, b []byte) {
	l.bytes += len(b)
	if l.dropFirst > 0 || (l.rng.Float64() < l.loss && now >= l.lossFrom) {
		l.dropFirst--
		l.lostSnaps++
		return
	}
	n := 1
	if l.rng.Float64() < l.dup {
		n = 2
	}
	for range n {
		at := now + l.lag + l.rng.IntN(l.jitter+1)
		q[at] = append(q[at], b)
	}
}

func (l *lossyLink) ack(now int, t int64) {
	if l.rng.Float64() < l.loss && now >= l.lossFrom {
		return
	}
	at := now + l.lag + l.rng.IntN(l.jitter+1)
	l.up[at] = append(l.up[at], t)
}

// runLossy plays n players for ticks over lossy links; the client end
// decodes as web/engine/net.js does (a missing base: it acks what it has
// at once and asks for a full only if it isn't in step within 20 ticks; a
// bad checksum: it asks at once; again every 20 ticks), acks the newest tick
// it decoded
// every tick, and checks every snapshot it decodes against the json codec's
// for that tick.
func runLossy(t *testing.T, mk func(i int) *lossyLink, ticks int) []*lossyLink {
	return runLossyRec(t, mk, ticks, nil)
}

// runLossyRec: runLossy, calling rec with every snapshot client 0 reads, in
// the order it reads them, and what came of it (its decoded JSON, "stale"
// or the error).
func runLossyRec(t *testing.T, mk func(i int) *lossyLink, ticks int, rec func(b []byte, got any)) []*lossyLink {
	t.Helper()
	s, ps, start := deltaWorldT(t, 4, true)
	links := map[*Player]*lossyLink{}
	decs := map[*Player]*deltaDecoder{}
	asked, gap := map[*Player]int{}, map[*Player]int{}
	var out []*lossyLink
	for i, p := range ps {
		l := mk(i)
		l.down, l.up = map[int][][]byte{}, map[int][]int64{}
		links[p], decs[p] = l, &deltaDecoder{}
		l.p = p
		out = append(out, l)
	}
	want := map[*Player]map[int64]any{}
	for tick := 1; tick <= ticks; tick++ {
		for i, p := range ps {
			if p.inv != nil {
				s.queueCmds(p, []Cmd{{Seq: tick, F: 1, R: math.Sin(float64(tick+i) * 0.1), Yaw: float64(i) + float64(tick)*0.02, Pitch: 0.1}})
			}
		}
		if tick%97 == 0 { // the cold state moves too: money, an item
			ps[0].Money += 50
			s.items = append(s.items, &Item{ID: 100 + tick, Weapon: "lint", Pos: Vec3{float64(tick % 7), 0, 1}})
		}
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		// acks that arrive now reach the server before this tick's snapshots
		for _, p := range ps {
			for _, a := range links[p].up[tick] {
				if a < 0 {
					p.requestFull()
				} else {
					p.ackSnap(s.tick, a)
				}
			}
			delete(links[p].up, tick)
		}
		s.broadcast()
		for _, p := range ps {
			l, d := links[p], decs[p]
			if want[p] == nil {
				want[p] = map[int64]any{}
			}
			w := wantSnapJSON(t, s, p)
			want[p][s.tick] = w
			delete(want[p], s.tick-1000)
			for len(p.send) > 0 {
				if b := <-p.send; isDeltaSnap(b) {
					l.send(l.down, tick, b)
				}
			}
			if tick >= l.hitchAt && tick < l.hitchAt+l.hitch {
				continue // stalled: the snapshots wait in the socket
			}
			for _, at := range slices.Sorted(maps.Keys(l.down)) { // arrival order
				if at > tick {
					break
				}
				for _, b := range l.down[at] {
					err := d.apply(b)
					if rec != nil && p == ps[0] {
						switch err {
						case nil:
							rec(b, toJSON(netSchema, d.state))
						case errStale:
							rec(b, "stale")
						default:
							rec(b, err.Error())
						}
					}
					switch err {
					case nil:
						l.ok++
						gap[p] = 0
						got := toJSON(netSchema, d.state)
						if !reflect.DeepEqual(got, want[p][d.tick]) {
							gb, _ := json.Marshal(got)
							wb, _ := json.Marshal(want[p][d.tick])
							t.Fatalf("tick %d, player %d: decoded tick %d\n%s\nwant\n%s", tick, p.ID, d.tick, gb, wb)
						}
					case errStale:
						l.stale++
					default:
						l.errs++
						if errors.Is(err, errMissingBase) {
							if d.state != nil {
								l.ack(tick, d.kept) // at once (net.js SACK_MS)
							}
							if gap[p] == 0 {
								gap[p] = tick
							}
							if tick-gap[p] < 20 {
								continue
							}
						}
						if tick-asked[p] >= 20 { // net.js: once per gap, again after ~2 round trips
							asked[p] = tick
							l.fulls++
							at := tick + l.lag // the "full" message is reliable (not a datagram), but takes the lag
							l.up[at] = append(l.up[at], -1)
						}
					}
				}
				delete(l.down, at)
			}
			if d.state != nil {
				l.ack(tick, d.kept)
			}
		}
	}
	return out
}

// The standing rule: the network may lose, duplicate and reorder anything.
// Deltas are based on what the client acked, so a lost, duplicated or late
// snapshot costs that frame: no decode errors, no fulls asked for (before
// acked bases: every lost snapshot broke the chain until a full came, about
// 1250 errors and 71 fulls per client here).
func TestDeltaSurvivesLossDupReorder(t *testing.T) {
	links := runLossy(t, func(i int) *lossyLink {
		return &lossyLink{loss: 0.1, dup: 0.1, lag: 3 + i, jitter: 4, rng: rand.New(rand.NewPCG(uint64(i), 7))}
	}, 1500)
	for i, l := range links {
		t.Logf("client %d: %d decoded, %d stale (dups, late), %d lost, %d decode errors, %d fulls asked", i, l.ok, l.stale, l.lostSnaps, l.errs, l.fulls)
		if l.errs != 0 || l.fulls != 0 {
			t.Errorf("client %d: %d decode errors, %d fulls asked under 10%% loss/dup and reordering: want 0", i, l.errs, l.fulls)
		}
		if l.ok < 900 { // the rest: lost, or overtaken by a newer one (a late one is dropped)
			t.Errorf("client %d: only %d snapshots decoded of 1500", i, l.ok)
		}
	}
}

// Even the join's full may be lost: the deltas from it (nothing acked yet)
// can't be decoded, the client asks again once, and it is in step after
// that. A stall longer than the base window: nothing it acked is a base any
// more, so the server sends it a full by itself; still no decode error.
func TestDeltaLostFirstFullAndLongStall(t *testing.T) {
	links := runLossy(t, func(i int) *lossyLink {
		l := &lossyLink{lag: 2, rng: rand.New(rand.NewPCG(uint64(i), 9))}
		switch i {
		case 0:
			l.dropFirst = 1
		case 1:
			l.hitchAt, l.hitch = 200, 100
		}
		return l
	}, 600)
	for i, l := range links {
		t.Logf("client %d: %d decoded, %d decode errors, %d fulls asked", i, l.ok, l.errs, l.fulls)
		want := 0
		if i == 0 {
			want = 1
		}
		if l.fulls != want || (i > 0 && l.errs > 0) {
			t.Errorf("client %d: %d fulls asked, %d decode errors; want %d fulls asked", i, l.fulls, l.errs, want)
		}
	}
}

// testdata/netdelta_loss_golden.json: what a client read over a lossy link
// (10% loss and duplicates, reordering, acks late and lost too): every
// message in the order it arrived, and every 20th decoded snapshot (the rest
// "ok", "stale"). web/engine/netdelta.test.mjs decodes it with the client's
// decoder: deltas from older acked bases, out of order. UPDATE=1 rewrites it.
func TestNetDeltaLossGolden(t *testing.T) {
	type msg struct {
		B    []byte `json:"b"`
		Want any    `json:"want"`
	}
	var g struct {
		Hash string `json:"hash"`
		Msgs []msg  `json:"msgs"`
	}
	g.Hash = netSchemaHash
	n := 0
	runLossyRec(t, func(i int) *lossyLink {
		return &lossyLink{loss: 0.1, dup: 0.1, lag: 3, jitter: 4, hitchAt: 150, hitch: 30, rng: rand.New(rand.NewPCG(uint64(i), 11))}
	}, 300, func(b []byte, got any) {
		if _, ok := got.(string); !ok {
			if n++; n%20 != 1 {
				got = "ok"
			}
		}
		g.Msgs = append(g.Msgs, msg{b, got})
	})
	got, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	const path = "testdata/netdelta_loss_golden.json"
	if os.Getenv("UPDATE") != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (UPDATE=1 go test ./internal/game -run TestNetDeltaLossGolden writes it)", err)
	}
	if string(want) != string(got) {
		t.Fatalf("%s is stale: the codec or the schema changed (UPDATE=1 go test ./internal/game -run TestNetDeltaLossGolden rewrites it)", path)
	}
}

// The snapshots kept as bases share what didn't change with the tick
// before (shareNV), and still encode and decode exactly as unshared ones.
func TestDeltaBasesShareUnchangedParts(t *testing.T) {
	s, ps, start := deltaWorldT(t, 4, true)
	for tick := 1; tick <= 3; tick++ {
		s.queueCmds(ps[0], []Cmd{{Seq: tick, F: 1}}) // only p00 moves
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		s.broadcast()
		for _, p := range ps {
			for len(p.send) > 0 {
				<-p.send
			}
			if p.delta.acked == 0 && len(p.delta.sent) > 0 {
				p.ackSnap(s.tick, p.delta.sent[0].tick)
			}
		}
	}
	d := &ps[1].delta
	a, b := d.sent[len(d.sent)-2].root, d.sent[len(d.sent)-1].root
	pl := rootField("players")
	pa, pb := a.kids[pl].kids, b.kids[pl].kids
	if &pa[0].kids[0] == &pb[0].kids[0] {
		t.Error("the moving player's fields are shared: it moved")
	}
	for j := 1; j < len(pa); j++ {
		if !sameNV(&pa[j], &pb[j]) {
			t.Errorf("player %d didn't change but has its own storage", j)
		}
	}
	if !sameNV(&a.kids[rootField("items")], &b.kids[rootField("items")]) {
		t.Error("the items didn't change but have their own storage")
	}
	// sharing changes no value
	x := nv{i: 1, kids: []nv{{i: 2}, {s: "a", kids: []nv{{i: 3}}}}}
	y := nv{i: 1, kids: []nv{{i: 2}, {s: "b", kids: []nv{{i: 3}}}}}
	if shareNV(&y, &x) || y.kids[1].s != "b" || !sameNV(&y.kids[0], &x.kids[0]) || !sameNV(&y.kids[1].kids[0], &x.kids[1].kids[0]) {
		t.Errorf("shareNV: %+v from %+v", y, x)
	}
}

// A stall past the base window under loss and reordering: the server falls
// back to a full (nothing it has is acked any more), deltas based on it may
// overtake it or it may be lost. The client acks its state at once instead
// of asking for a full right away; asking at once made a loop (each full
// asked for wiped the bases and was overtaken by deltas based on it: 36
// fulls in 25 s live).
func TestDeltaStallUnderReorderNoFullsLoop(t *testing.T) {
	links := runLossy(t, func(i int) *lossyLink {
		return &lossyLink{loss: 0.2, dup: 0.1, lag: 6, jitter: 6, hitchAt: 200 + 50*i, hitch: 150 + 20*i, rng: rand.New(rand.NewPCG(uint64(i), 13))}
	}, 1200)
	for i, l := range links {
		t.Logf("client %d: %d decoded, %d decode errors, %d fulls asked", i, l.ok, l.errs, l.fulls)
		if l.fulls != 0 { // asking at once: 1-2 each here
			t.Errorf("client %d: %d fulls asked after one long stall: want none (its ack puts it in step)", i, l.fulls)
		}
	}
}

// A slow link (a 2 s round trip): the kept snapshots spread out (the stride
// grows with the age the acks arrive with) so the ring of baseRing still
// spans a round trip, and once it has (the loss starts at 5 s) every delta
// comes from a base the client acked.
func TestDeltaSlowLinkStrideGrows(t *testing.T) {
	links := runLossy(t, func(i int) *lossyLink {
		return &lossyLink{loss: 0.05, lossFrom: 500, lag: 100, jitter: 3, rng: rand.New(rand.NewPCG(uint64(i), 17))} // the loss from 5 s on: after its window has grown
	}, 1500)
	for i, l := range links {
		t.Logf("client %d: %d decoded, %d decode errors, %d fulls asked", i, l.ok, l.errs, l.fulls)
		if l.errs > 5 || l.fulls > 0 { // the errors: deltas that overtook the join's full (it comes right after)
			t.Errorf("client %d: %d decode errors, %d fulls asked on a slow link: want none asked", i, l.errs, l.fulls)
		}
	}
}

// NETRING=1 go test ./internal/game -run TestDeltaRingReport -v: how often
// the acked base leaves the ring of baseRing (a full goes out instead), and
// what it costs, per client over 60 s of 4 players moving, at a few round
// trips and losses (acks every tick, as command packets carry them).
func TestDeltaRingReport(t *testing.T) {
	if os.Getenv("NETRING") == "" {
		t.Skip("NETRING=1 prints the table")
	}
	secs := 60
	out := "\n rtt   loss | stride | fallback fulls/min | periodic fulls/min | decode errors/min | fulls asked/min | delta B/s\n"
	for _, rtt := range []int{50, 100, 200} {
		for _, loss := range []float64{0, 0.03} {
			links := runLossy(t, func(i int) *lossyLink {
				return &lossyLink{loss: loss, lag: rtt / 20, jitter: 1, rng: rand.New(rand.NewPCG(uint64(i), uint64(rtt)))}
			}, secs*SimHz)
			var lapses, fulls, errs, asked, bytes, stride float64
			for _, l := range links {
				lapses += float64(l.p.delta.lapses)
				fulls += float64(l.p.delta.fulls - l.p.delta.lapses - 1)
				errs += float64(l.errs)
				asked += float64(l.fulls)
				bytes += float64(l.bytes)
				stride += float64(l.p.delta.stride())
			}
			n, m := float64(len(links)), float64(secs)/60
			out += fmt.Sprintf("%4d ms %3.0f%% | %6.1f | %18.2f | %18.1f | %17.2f | %15.2f | %8.0f\n", rtt, loss*100, stride/n, lapses/n/m, fulls/n/m, errs/n/m, asked/n/m, bytes/n/float64(secs))
		}
	}
	t.Log(out)
}

// A 450 ms main-thread stall (no acks, nothing read): the acked base leaves
// the ring, so the server sends one full on its own and bases the deltas
// after it on that; the client reads the stalled ones in order and is in
// step after them. No full asked for.
func TestDeltaStallFallsBackToOneFull(t *testing.T) {
	links := runLossy(t, func(i int) *lossyLink {
		return &lossyLink{lag: 3 + i, jitter: 1, hitchAt: 300 + 100*i, hitch: 45, rng: rand.New(rand.NewPCG(uint64(i), 19))}
	}, 1000)
	for i, l := range links {
		t.Logf("client %d: %d decoded, %d decode errors, %d fulls asked, %d fallback fulls", i, l.ok, l.errs, l.fulls, l.p.delta.lapses)
		if l.errs != 0 || l.fulls != 0 || l.p.delta.lapses < 1 || l.p.delta.lapses > 2 {
			t.Errorf("client %d: %d decode errors, %d fulls asked, %d fallback fulls: want 0, 0, 1 (its join may lapse too)", i, l.errs, l.fulls, l.p.delta.lapses)
		}
	}
}

// A tick's world, once sent, drops its encode cache: the cache is keyed by
// the base's world, so keeping it chained every world since the start (a
// leak of ~68 KB a tick at 12v12, before acked bases), and replays the same.
func TestDeltaWorldsDoNotChain(t *testing.T) {
	s, ps, start := deltaWorldT(t, 4, true)
	for tick := 1; tick <= 20; tick++ {
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		s.broadcast()
		for _, p := range ps {
			for len(p.send) > 0 {
				<-p.send
			}
			if n := len(p.delta.sent); n > 0 {
				p.ackSnap(s.tick, p.delta.sent[n-1].tick)
			}
		}
	}
	if s.deltaPrev.enc != nil {
		t.Error("the last world sent still holds its encode cache")
	}
	for _, p := range ps {
		for _, e := range p.delta.sent {
			if e.world.enc != nil {
				t.Errorf("player %d: a kept world (tick %d) holds its encode cache", p.ID, e.tick)
			}
		}
	}
}
