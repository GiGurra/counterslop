package game

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

// The delta wire watch: a test sees, every tick, that what the delta codec
// would send each human decodes (checksum included), and that it decodes to
// exactly the json codec's full snapshot
// (the bytes jsonFullSnap puts on the wire). It runs beside the test, on its
// own codec state per player: nothing goes into p.send, so a test reading
// its players' messages sees what it always did.
//
//	wireWatch(t, s)                            one test's server, failing t
//	COUNTERSLOP_TEST_NET_CODEC=delta go test   every server of every test (the run lists each server's first difference)
//	COUNTERSLOP_TEST_NET_CODEC=off go test     none (wireWatch does nothing)
//	COUNTERSLOP_TEST_NET_CODEC_EVERY=1         compare with the json snapshot every tick, not every 7th
//
// It hooks simTick (testTickHook) and checks the state as of the tick's
// start, so a test driving simTick, advance or the ticks helper is covered.

type wireConn struct {
	base      *nv
	baseWorld *deltaWorld
	baseTick  int64
	dec       deltaDecoder
	n         int // snapshots checked
	bytes     int // delta bytes
}

type wireWatcher struct {
	t        testing.TB // nil: the env switch (panic on a difference)
	conns    map[*Player]*wireConn
	ticks    int
	compared int // snapshots compared with the json one
	shape    string
	bad      bool
}

var (
	wireMu       sync.Mutex
	wireWatchers = map[*Server]*wireWatcher{}
	wireAll      = os.Getenv("COUNTERSLOP_TEST_NET_CODEC") == CodecDelta
	wireEvery    = os.Getenv("COUNTERSLOP_TEST_NET_CODEC_EVERY") == "1"
	wireOff      = os.Getenv("COUNTERSLOP_TEST_NET_CODEC") == "off" // no watching at all (to time the tests without it)
)

// wireFails: the env switch's differences, one per server; TestMain
// (hitbox_test.go) reports them and fails the run (wireReport).
var wireFails []string

func init() { testTickHook = wireTick }

// wireReport prints the env switch's differences; false if there were any.
func wireReport() bool {
	if !wireAll {
		return true
	}
	wireMu.Lock()
	defer wireMu.Unlock()
	for _, f := range wireFails {
		fmt.Fprintf(os.Stderr, "DELTA WIRE: %s\n\n", f)
	}
	fmt.Fprintf(os.Stderr, "delta wire watch: %d servers, %d differ\n", len(wireWatchers), len(wireFails))
	return len(wireFails) == 0
}

// testName: the Test function on the stack (the env switch has no t).
func testName() string {
	pc := make([]uintptr, 64)
	fr := runtime.CallersFrames(pc[:runtime.Callers(2, pc)])
	for {
		f, more := fr.Next()
		if i := strings.LastIndex(f.Function, ".Test"); i >= 0 {
			return f.Function[i+1:]
		}
		if !more {
			return "?"
		}
	}
}

// wireCompareEvery: how often a wireWatch compares with the json snapshot.
const wireCompareEvery = 7

// worldShape: the lengths of the world's lists (players, items, cars...).
func worldShape(root *nv) string {
	var b []byte
	for j := range root.kids {
		b = fmt.Appendf(b, "%d,", len(root.kids[j].kids))
	}
	return string(b)
}

// wireWatch checks s's delta wire every tick until the test ends; it
// returns the watcher (checked counts for the test's own assertions).
func wireWatch(t testing.TB, s *Server) *wireWatcher {
	t.Helper()
	w := &wireWatcher{t: t, conns: map[*Player]*wireConn{}}
	if wireOff {
		return w
	}
	wireMu.Lock()
	wireWatchers[s] = w
	wireMu.Unlock()
	t.Cleanup(func() {
		wireMu.Lock()
		delete(wireWatchers, s)
		wireMu.Unlock()
	})
	return w
}

func wireTick(s *Server) {
	wireMu.Lock()
	w := wireWatchers[s]
	if w == nil && wireAll {
		w = &wireWatcher{conns: map[*Player]*wireConn{}}
		wireWatchers[s] = w // kept until the process ends: test servers are few
	}
	wireMu.Unlock()
	if w == nil || w.bad {
		return
	}
	if err := w.check(s); err != nil {
		w.bad = true // one report per server
		if w.t == nil {
			wireMu.Lock()
			wireFails = append(wireFails, fmt.Sprintf("%s: %v", testName(), err))
			wireMu.Unlock()
			return
		}
		w.t.Errorf("delta wire: %v", err)
	}
}

// check encodes this tick for every human (and a bot per team) as the delta codec does (a full
// first and every fullPeriod ticks, deltas between), decodes it and
// compares it with the json codec's full snapshot. The caller is the tick
// (simTick), which holds s.mu where the server runs for real.
func (w *wireWatcher) check(s *Server) error {
	if len(s.players) == 0 {
		return nil
	}
	w.ticks++
	var list []*Player
	for _, p := range s.ordered() {
		if p.ready {
			list = append(list, p)
		}
	}
	for _, p := range list { // as sendSnapshot
		if p.inv != nil && p.inv.Slots[p.inv.Current] != nil {
			h := p.inv.Slots[p.inv.Current]
			p.Weapon, p.Ammo, p.Reserve = h.ID, h.Ammo, h.Reserve
		}
	}
	world := s.deltaWorldOf(list)
	spotted := s.spotted()
	// Decoding (and its checksum) runs every tick; the json comparison, the
	// costly part, every wireCompareEvery ticks (every tick with
	// COUNTERSLOP_TEST_NET_CODEC_EVERY=1) and on any tick an entity list changes length
	// (a join or leave, a car parked or back, a grenade thrown).
	shape := worldShape(&world.root)
	compare := wireEvery || w.ticks%wireCompareEvery == 1 || shape != w.shape
	w.shape = shape
	var jsonSnap func(*Player) []byte
	if compare {
		jsonSnap = s.jsonFullSnap(list)
	}
	for p := range w.conns {
		if s.players[p.ID] != p {
			delete(w.conns, p) // left
		}
	}
	botSeen := map[string]bool{}
	for _, p := range s.ordered() {
		// Every human, and one bot per team: a server of bots only (the AI
		// tests) is watched from a player's seat in the match too.
		if p.Bot && botSeen[p.Team] {
			continue
		}
		if p.Bot {
			botSeen[p.Team] = true
		}
		c := w.conns[p]
		if c == nil {
			c = &wireConn{}
			w.conns[p] = c
		}
		root := s.deltaRoot(world, p, spotted)
		base := c.base
		if c.n%fullPeriod == 0 {
			base = nil
		}
		bw := c.baseWorld
		if base == nil {
			bw = nil
		}
		msg := deltaEncode(world, &root, base, bw, s.tick, c.baseTick)
		if err := c.dec.apply(msg); err != nil {
			return fmt.Errorf("tick %d, player %d %q: decode: %v", s.tick, p.ID, p.Name, err)
		}
		c.base, c.baseWorld, c.baseTick = &root, world, s.tick
		c.n++
		c.bytes += len(msg)
		if !compare {
			continue
		}
		w.compared++
		got := toJSON(netSchema, c.dec.state)
		var want map[string]any
		b := jsonSnap(p)
		if err := json.Unmarshal(b, &want); err != nil {
			return fmt.Errorf("tick %d: the json snapshot: %v", s.tick, err)
		}
		if spot, ok := want["spot"].([]any); ok {
			slices.SortFunc(spot, func(a, b any) int { return int(a.(float64) - b.(float64)) })
		}
		if !reflect.DeepEqual(got, any(want)) {
			gb, _ := json.Marshal(got)
			return fmt.Errorf("tick %d, player %d %q: delta decodes to\n%s\njson codec sends\n%s\nfirst difference: %s", s.tick, p.ID, p.Name, gb, b, jsonDiffPath("", got, any(want)))
		}
	}
	return nil
}

// jsonDiffPath: the path of the first difference between two decoded JSON values.
func jsonDiffPath(path string, a, b any) string {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: %v vs %v", path, a, b)
		}
		keys := map[string]bool{}
		for k := range av {
			keys[k] = true
		}
		for k := range bv {
			keys[k] = true
		}
		ks := make([]string, 0, len(keys))
		for k := range keys {
			ks = append(ks, k)
		}
		slices.Sort(ks)
		for _, k := range ks {
			x, okA := av[k]
			y, okB := bv[k]
			if okA != okB {
				return fmt.Sprintf("%s.%s: present in delta %v, in json %v (%v vs %v)", path, k, okA, okB, x, y)
			}
			if !reflect.DeepEqual(x, y) {
				return jsonDiffPath(path+"."+k, x, y)
			}
		}
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return fmt.Sprintf("%s: %d elements in delta, json %v", path, len(av), b)
		}
		for i := range av {
			if !reflect.DeepEqual(av[i], bv[i]) {
				return jsonDiffPath(fmt.Sprintf("%s[%d]", path, i), av[i], bv[i])
			}
		}
	}
	return fmt.Sprintf("%s: %v vs %v", path, a, b)
}
