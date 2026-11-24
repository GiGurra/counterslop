package game

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// deltaWorldT: a server on the delta codec with n players on both teams
// (the last one spectating when spectator is set), an item and a grenade.
func deltaWorldT(t *testing.T, n int, spectator bool) (*Server, []*Player, time.Time) {
	t.Helper()
	s := authServer(t)
	s.Test = true
	if err := s.SetNetCodec(CodecDelta); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	s.lastTickAt = start
	var ps []*Player
	for i := range n {
		p := s.join(fmt.Sprintf("10.0.%d.1", i))
		p.Name, p.ready = fmt.Sprintf("p%02d", i), true
		p.Team = []string{content.Vibe, content.Artisan}[i%2]
		if spectator && i == n-1 {
			p.Team = content.Spectator
		}
		s.respawn(p)
		if p.Team != content.Spectator {
			p.inv = s.Weapons.Loadout(p.Team)
		}
		ps = append(ps, p)
	}
	s.items = append(s.items, &Item{ID: 7, Weapon: "lint", Pos: Vec3{1.23456, 0.5, -2.0001}})
	s.nades = append(s.nades, &Nade{ID: 3, Weapon: "nade", Kind: "frag", Pos: Vec3{1, 2, 3.33333}, Vel: Vec3{0.1, -9.81, 0}, State: "smoke", Left: 3})
	return s, ps, start
}

// wantSnapJSON: p's full snapshot as the json codec declares it (snapMsg),
// read back as generic JSON, players in id order.
func wantSnapJSON(t *testing.T, s *Server, p *Player) map[string]any {
	t.Helper()
	var list []*Player
	for _, q := range s.ordered() {
		if q.ready {
			list = append(list, q)
		}
	}
	m := snapMsg{T: "snap", Time: s.clock().UnixMilli(), Tick: s.tick, Players: list, Items: s.items, Nades: s.nades, Me: s.meStateOf(p)}
	if ids, ok := s.spotted()[p.Team]; ok {
		ids = slices.Clone(ids)
		slices.Sort(ids) // a set; the delta codec sends it sorted
		m.Spot = &ids
	}
	r := s.roundMsgOf(s.clock())
	m.Round = &r
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// deltaCheck drains every connection's messages through its decoder and
// checks the decoded state against the JSON snapshot. Returns bytes received.
func deltaCheck(t *testing.T, s *Server, ps []*Player, dec map[*Player]*deltaDecoder) (n, compared int) {
	t.Helper()
	for _, p := range ps {
		d := dec[p]
		got := false
		for len(p.send) > 0 {
			b := <-p.send
			if len(b) == 0 || b[0] != deltaMagic {
				continue
			}
			n += len(b)
			if err := d.apply(b); err != nil {
				t.Fatalf("tick %d, player %d: %v", s.tick, p.ID, err)
			}
			got = true
		}
		if !got || d.tick != s.tick {
			continue // waiting for its full
		}
		gotJ := toJSON(netSchema, d.state)
		want := wantSnapJSON(t, s, p)
		if !reflect.DeepEqual(gotJ, any(want)) {
			gb, _ := json.Marshal(gotJ)
			wb, _ := json.Marshal(want)
			t.Fatalf("tick %d, player %d: decoded\n%s\nwant (json codec)\n%s", s.tick, p.ID, gb, wb)
		}
		compared++
	}
	return n, compared
}

func TestDeltaDecodesToTheJSONSnapshot(t *testing.T) {
	s, ps, start := deltaWorldT(t, 6, true)
	dec := map[*Player]*deltaDecoder{}
	for _, p := range ps {
		dec[p] = &deltaDecoder{}
	}
	ps[0].Money, ps[0].Armor, ps[0].Helmet = 1234, 55, true
	compared := 0
	for tick := 1; tick <= 400; tick++ {
		for i, p := range ps {
			if p.Team == content.Spectator {
				continue
			}
			c := Cmd{Seq: tick, F: 1, R: math.Sin(float64(tick+i) * 0.1), Yaw: float64(i) + float64(tick)*0.013, Pitch: 0.1}
			if tick%40 < 5 {
				c.B |= BtnCrouch
			}
			if tick%57 == i {
				c.B |= BtnFire
			}
			s.queueCmds(p, []Cmd{c})
		}
		switch tick {
		case 50: // world changes: an item goes, a grenade moves, someone dies, renames, a new player
			s.items = nil
			s.nades[0].Pos[1] = 0.25
			ps[1].HP, ps[1].Dead = 0, true
			ps[2].Name = "renamed"
		case 120:
			p := s.join("10.9.9.9")
			p.ready, p.Team = true, content.Vibe
			s.respawn(p)
			p.inv = s.Weapons.Loadout(p.Team)
			ps = append(ps, p)
			dec[p] = &deltaDecoder{}
		case 200: // someone leaves
			s.leave(ps[3])
			ps = slices.Delete(ps, 3, 4)
		case 250:
			s.nades = nil
		}
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		s.broadcast()
		_, c := deltaCheck(t, s, ps, dec)
		compared += c
	}
	if compared < 400*5 {
		t.Errorf("compared %d decoded snapshots with the JSON ones, want about 400 per player", compared)
	}
	for _, p := range ps {
		if dec[p].state == nil {
			t.Errorf("player %d never got a snapshot", p.ID)
		}
		if p.delta.fulls < 3 {
			t.Errorf("player %d: %d fulls in 4 s, want the join one + about one a second", p.ID, p.delta.fulls)
		}
	}
}

// A client that lost its base, or whose state went wrong, notices (missing
// base, checksum) and asks for a full one, which puts it right.
func TestDeltaMissingBaseAndChecksum(t *testing.T) {
	s, ps, start := deltaWorldT(t, 4, false)
	p := ps[0]
	tick := 0
	step := func() []byte {
		tick++
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		s.broadcast()
		var mine []byte
		for _, q := range ps {
			for len(q.send) > 0 {
				if b := <-q.send; q == p && b[0] == deltaMagic {
					mine = b
				}
			}
		}
		return mine
	}
	d := &deltaDecoder{}
	for range 3 {
		if err := d.apply(step()); err != nil {
			t.Fatal(err)
		}
	}
	step() // lost on the way
	if err := d.apply(step()); err == nil {
		t.Fatal("applied a delta whose base was lost")
	}
	p.requestFull()
	b := step()
	if baseOf(b) != -1 {
		t.Fatalf("after the request: a delta from %d, want a full", baseOf(b))
	}
	if err := d.apply(b); err != nil {
		t.Fatalf("the full: %v", err)
	}
	if err := d.apply(step()); err != nil {
		t.Fatalf("the delta after it: %v", err)
	}
	// The client's state goes wrong (a decoder bug, say): the next delta
	// applies but its checksum doesn't match.
	bad := *d.state
	bad.kids = slices.Clone(bad.kids)
	bad.kids[rootField("time")].i += 7
	d.state = &bad
	if err := d.apply(step()); err == nil {
		t.Fatal("a wrong state passed the checksum")
	}
	// A corrupted checksum is caught too.
	b = step()
	b[len(varintHeader(b))] ^= 1
	if err := (&deltaDecoder{state: d.state, tick: d.tick}).apply(b); err == nil {
		t.Fatal("a wrong checksum passed")
	}
}

// varintHeader: magic, tick, base (the bytes before the checksum).
func varintHeader(b []byte) []byte {
	r := &deltaReader{b: b}
	r.byte()
	r.uv()
	r.uv()
	return b[:r.off]
}

// baseOf: the base tick a message is a delta from (-1: a full one).
func baseOf(b []byte) int64 {
	r := &deltaReader{b: b}
	r.byte()
	tick := int64(r.uv())
	if back := int64(r.uv()); back > 0 {
		return tick - back
	}
	return -1
}

// TestNetDeltaGolden: a recorded stream of one client's delta snapshots and
// what each decodes to (testdata/netdelta_golden.json). The Go decoder checks
// it here; web/engine/netdelta.test.mjs decodes the same bytes with the
// client's decoder and must get the same objects.
//
//	UPDATE=1 go test ./internal/game -run TestNetDeltaGolden   rewrites the file
func TestNetDeltaGolden(t *testing.T) {
	type golden struct {
		Hash string `json:"hash"`
		Msgs []struct {
			B    []byte `json:"b"` // base64
			Want any    `json:"want"`
		} `json:"msgs"`
	}
	var g golden
	g.Hash = netSchemaHash
	s, ps, start := deltaWorldT(t, 5, true)
	p := ps[0]
	ps[0].Money, ps[0].Armor, ps[0].Helmet, ps[0].Kit = 1234, 55, true, true
	d := &deltaDecoder{}
	for tick := 1; tick <= 50; tick++ {
		for i, q := range ps {
			if q.Team == content.Spectator {
				continue
			}
			c := Cmd{Seq: tick, F: 1, R: math.Sin(float64(tick+i) * 0.2), Yaw: float64(i) - float64(tick)*0.031, Pitch: -0.2 + float64(tick)*0.004}
			if tick%30 < 6 {
				c.B |= BtnCrouch
			}
			if tick%23 == i {
				c.B |= BtnFire
			}
			s.queueCmds(q, []Cmd{c})
		}
		switch tick {
		case 10:
			s.items = append(s.items, &Item{ID: 9, Weapon: "stub", Pos: Vec3{-3.25, 0, 7.125}})
			ps[2].HP, ps[2].Dead, ps[2].DeathTick, ps[2].FallYaw = 0, true, s.tick, -2.5
		case 20:
			q := s.join("10.7.7.7")
			q.ready, q.Team, q.Name = true, content.Artisan, "ünïcode ☃"
			s.respawn(q)
			q.inv = s.Weapons.Loadout(q.Team)
			ps = append(ps, q)
		case 30:
			s.leave(ps[1])
			ps = slices.Delete(ps, 1, 2)
		case 40:
			p.requestFull()
		}
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		s.broadcast()
		for _, q := range ps {
			for len(q.send) > 0 {
				b := <-q.send
				if q != p || b[0] != deltaMagic {
					continue
				}
				if err := d.apply(b); err != nil {
					t.Fatal(err)
				}
				g.Msgs = append(g.Msgs, struct {
					B    []byte `json:"b"`
					Want any    `json:"want"`
				}{b, toJSON(netSchema, d.state)})
			}
		}
	}
	got, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	const path = "testdata/netdelta_golden.json"
	if os.Getenv("UPDATE") != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (UPDATE=1 go test ./internal/game -run TestNetDeltaGolden writes it)", err)
	}
	if string(want) != string(got) {
		t.Fatalf("%s is stale: the codec or the schema changed (UPDATE=1 go test ./internal/game -run TestNetDeltaGolden rewrites it)", path)
	}
}

// web/engine/netschema.gen.js is what the net tags generate (cmd/gojs -all).
func TestNetSchemaJSUpToDate(t *testing.T) {
	got, err := os.ReadFile("../../" + NetSchemaOut)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(NetSchemaJS()) {
		t.Fatalf("%s is stale: run go generate ./internal/gojs", NetSchemaOut)
	}
}

// egress: what egressRun saw from tick from on, per tick: the bytes of
// snapshots queued to all clients, of them the fulls' bytes, and the fulls.
type egress struct {
	bytes, fullBytes []float64
	fulls            []int
}

// egressRun drives s for ticks with everyone moving, calls event(tick) before
// each tick. Every connection that has a base gets its delta every tick
// (deltas are never held back), and the client's drawn delay on a clean link
// (web/engine/net.js delayTicks) stays exactly 30 ms: it fails the test otherwise.
func egressRun(t *testing.T, s *Server, ps *[]*Player, start time.Time, ticks, from int, event func(tick int)) (e egress) {
	t.Helper()
	prevFulls := map[*Player]int{}
	// The drawn delay: 2 snapshot intervals + the p95 lateness (0 here) + 1
	// tick, the interval an EWMA (0.1) of the spacing.
	type view struct {
		prev     int64
		interval float64
	}
	views := map[*Player]*view{}
	var maxDelay float64
	missed := 0
	defer func() {
		t.Logf("drawn delay (net.js delayTicks on a clean link) over all clients and ticks: max %.1f ms", maxDelay)
		if maxDelay > 30+1e-9 {
			t.Errorf("the drawn delay reached %.1f ms: a client went a tick without a snapshot", maxDelay)
		}
		if missed > 0 {
			t.Errorf("%d times a connection with a base got no delta in a tick", missed)
		}
	}()
	for tick := 1; tick <= ticks; tick++ {
		if event != nil {
			event(tick)
		}
		for i, p := range *ps {
			if p.Team == content.Spectator || p.Bot {
				continue
			}
			s.queueCmds(p, []Cmd{{Seq: tick, F: 1, R: math.Sin(float64(tick+i) * 0.1), Yaw: float64(i) + float64(tick)*0.02, Pitch: 0.1}})
		}
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		based := map[*Player]bool{}
		for _, p := range *ps {
			based[p] = p.delta.base != nil
		}
		s.broadcast()
		bytes, fullBytes, nf := 0, 0, 0
		for _, p := range *ps {
			if p.send == nil {
				continue // a bot
			}
			got := false
			for len(p.send) > 0 {
				if b := <-p.send; b[0] == deltaMagic { // snapshots (events are JSON messages)
					got = true
					bytes += len(b)
					if baseOf(b) < 0 {
						fullBytes += len(b)
					}
					v := views[p]
					if v == nil {
						v = &view{prev: -1, interval: 1}
						views[p] = v
					}
					if v.prev >= 0 {
						v.interval += (float64(s.tick-v.prev) - v.interval) * 0.1
					}
					v.prev = s.tick
					if tick >= from {
						maxDelay = max(maxDelay, (2*v.interval+1)*1000/SimHz)
					}
				}
			}
			if based[p] && !got {
				missed++
			}
			nf += p.delta.fulls - prevFulls[p]
			prevFulls[p] = p.delta.fulls
		}
		if tick >= from {
			e.bytes = append(e.bytes, float64(bytes))
			e.fullBytes = append(e.fullBytes, float64(fullBytes))
			e.fulls = append(e.fulls, nf)
		}
	}
	return e
}

func peakMean(xs []float64) (peak, mean float64) {
	for _, x := range xs {
		peak = max(peak, x)
		mean += x
	}
	return peak, mean / float64(len(xs))
}

// fullsBound: the most a tick's fulls add to the server's egress, as a
// fraction of the mean per-tick egress (fulls are spread: no egress bursts).
// What a round restart or a join wave changes goes out as deltas at once
// (never held back); that burst is accepted.
const fullsBound = 0.6

// checkFullsSpread: at most fullsPerTick(n) fulls in a tick, and their bytes
// within fullsBound of the mean egress.
func checkFullsSpread(t *testing.T, e egress, n int, what string) {
	t.Helper()
	peak, mean := peakMean(e.bytes)
	fpeak, _ := peakMean(e.fullBytes)
	t.Logf("%s: per-tick egress peak %.0f B, mean %.0f B (%.2fx, deltas never held back); the fulls add at most %.0f B a tick (%.2fx the mean); fulls per tick max %d", what, peak, mean, peak/mean, fpeak, fpeak/mean, slices.Max(e.fulls))
	if fpeak/mean > fullsBound {
		t.Errorf("a tick's fulls added %.0f B, %.2fx the mean egress %.0f B: over %.1f", fpeak, fpeak/mean, mean, fullsBound)
	}
	if m := slices.Max(e.fulls); m > fullsPerTick(n) {
		t.Errorf("%d fulls in one tick, at most %d", m, fullsPerTick(n))
	}
}

// Fulls are spread: the periodic refreshes go out a few a tick, never all at
// once, also across a round restart (which changes everyone's state in one
// tick and goes out as deltas, at once). Every client gets its delta every tick.
func TestDeltaFullsSpreadAcrossRoundRestart(t *testing.T) {
	s, ps, start := deltaWorldT(t, 24, false)
	s.nades = nil
	e := egressRun(t, s, &ps, start, 1000, 101, func(tick int) {
		if tick == 500 {
			s.restart() // round restart: everyone respawned, money and weapons reset
		}
	})
	checkFullsSpread(t, e, 24, "12v12, 9 s incl. a round restart")
}

// One human and bots (the usual small game): the human's delta goes out every
// tick, across a round restart and a new item, and a full about once a second.
func TestDeltaOneHumanAndBotsEveryTick(t *testing.T) {
	s, ps, start := deltaWorldT(t, 1, false)
	s.nades = nil
	for range 9 {
		ps = append(ps, s.addBot())
	}
	human := ps[0]
	e := egressRun(t, s, &ps, start, 600, 2, func(tick int) {
		switch tick {
		case 200: // a gun lands where the human stands (the items list changes)
			s.items = append(s.items, &Item{ID: 99, Weapon: "prompt", Pos: human.Pos})
		case 400:
			s.restart()
		}
	})
	if human.delta.fulls < 5 || human.delta.fulls > 8 {
		t.Errorf("%d fulls in 6 s, want the first + about one a second", human.delta.fulls)
	}
	if slices.Max(e.fulls) > 1 {
		t.Errorf("%d fulls in a tick for one connection", slices.Max(e.fulls))
	}
}

// Joins, leaves and reconnect waves (10 clients in one tick) don't bunch the
// fulls: at most fullsPerTick a tick, their bytes bounded.
func TestDeltaFullsSpreadAcrossJoinWaves(t *testing.T) {
	s, ps, start := deltaWorldT(t, 20, false)
	s.nades = nil
	rng := rand.New(rand.NewPCG(7, 8))
	join := func(n int) {
		for range n {
			p := s.join(fmt.Sprintf("10.8.%d.%d", rng.IntN(250), rng.IntN(250)))
			p.ready, p.Team = true, []string{content.Vibe, content.Artisan}[rng.IntN(2)]
			s.respawn(p)
			p.inv = s.Weapons.Loadout(p.Team)
			ps = append(ps, p)
		}
	}
	leave := func(n int) {
		for range min(n, len(ps)-2) {
			i := rng.IntN(len(ps))
			s.leave(ps[i])
			ps = slices.Delete(ps, i, i+1)
		}
	}
	pending := 0
	e := egressRun(t, s, &ps, start, 3000, 201, func(tick int) {
		switch {
		case tick%500 == 250: // a reconnect wave: 10 drop and come back in the same tick
			leave(10)
			join(10)
		case tick%100 == 30: // a few join, as many leave half a second later (the mean is per tick: keep the head count)
			pending = 1 + rng.IntN(2)
			join(pending)
		case tick%100 == 80:
			leave(pending)
		}
	})
	checkFullsSpread(t, e, len(ps)+10, "30 s, 20-22 clients, joins and leaves, reconnect waves of 10")
	for _, p := range ps {
		if p.delta.base == nil {
			t.Errorf("player %d still waiting for its first full at the end", p.ID)
		}
	}
}
