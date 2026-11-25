package game

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// tvRec is one record of a replay file.
type tvRec struct {
	kind byte
	tick int64
	data []byte // the snapshot message, or the event's JSON
}

// readReplay reads a replay file. A file still being written reads up to its
// last flush (live): complete is false then.
func readReplay(t *testing.T, path string) (h tvHeader, recs []tvRec, complete bool) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(gz)
	complete = err == nil
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("%s: %v", path, err)
	}
	r := bufio.NewReader(bytes.NewReader(raw))
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != tvMagic {
		t.Fatalf("%s: magic %q", path, magic)
	}
	n, _ := binary.ReadUvarint(r)
	hb := make([]byte, n)
	if _, err := io.ReadFull(r, hb); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(hb, &h); err != nil {
		t.Fatal(err)
	}
	for {
		k, err := r.ReadByte()
		if err == io.EOF {
			return
		}
		rec := tvRec{kind: k}
		switch k {
		case tvSnap:
		case tvEvent:
			tk, _ := binary.ReadUvarint(r)
			rec.tick = int64(tk)
		default:
			t.Fatalf("%s: record kind %q", path, k)
		}
		n, err := binary.ReadUvarint(r)
		if err != nil {
			t.Fatal(err)
		}
		rec.data = make([]byte, n)
		if _, err := io.ReadFull(r, rec.data); err != nil {
			t.Fatalf("%s: a cut record: %v", path, err)
		}
		if k == tvSnap {
			rec.tick, _ = snapTickBack(rec.data)
		}
		recs = append(recs, rec)
	}
}

// snapTickBack: a delta message's tick and ticks back to its base (0: full).
func snapTickBack(b []byte) (int64, int64) {
	r := &deltaReader{b: b}
	r.byte()
	tick := int64(r.uv())
	return tick, int64(r.uv())
}

// tvServer: a recording server (lockstep, game clock) with a Vibe Coder and
// an Artisan, no freeze or warmup, a 1 s round restart delay.
func tvServer(t *testing.T) (*Server, string, *Player, *Player) {
	s := authServer(t)
	dir := t.TempDir()
	if err := s.SetReplays(dir, 0); err != nil {
		t.Fatal(err)
	}
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_round_restart_delay"] = "1"
	v, a := s.join("1.1.1.1"), s.join("2.2.2.2")
	v.ready, a.ready = true, true
	v.Team, a.Team = content.Vibe, content.Artisan
	return s, dir, v, a
}

// tvRun steps n ticks, v running forward, and notes where every player was
// at each tick (what the replay's snapshots must say).
func tvRun(s *Server, n int, v *Player, at map[int64]map[int]Vec3) {
	for range n {
		s.queueCmds(v, []Cmd{{Seq: v.lastQueued + 1, F: 1, Yaw: 1}})
		s.Step(1)
		pos := map[int]Vec3{}
		for _, p := range s.players {
			pos[p.ID] = p.Pos
		}
		at[s.tick] = pos
	}
}

// Every round is a file of its own under the match's directory: a header
// (map, players, score, the schema), observer snapshots every tvEvery ticks
// that decode (checksums and all) to where everyone was, a keyframe every
// second, and the round's events (start, the kill, the end).
func TestReplayRecordsEachRound(t *testing.T) {
	s, dir, v, a := tvServer(t)
	at := map[int64]map[int]Vec3{}
	tvRun(s, 250, v, at)
	if s.round.Phase != PhaseLive || s.round.Number != 1 {
		t.Fatalf("round %d %s", s.round.Number, s.round.Phase)
	}
	s.mu.Lock()
	s.killBy(v, a, "prompt", true)
	s.mu.Unlock()
	tvRun(s, 200, v, at) // round over, then round 2 starts (1 s restart delay)
	if s.round.Number != 2 {
		t.Fatalf("round %d, want 2", s.round.Number)
	}
	tvRun(s, 150, v, at)
	s.tvSync()

	matches, _ := os.ReadDir(dir)
	if len(matches) != 1 || !strings.HasSuffix(matches[0].Name(), "-de_slop") {
		t.Fatalf("match dirs %v", matches)
	}
	mdir := filepath.Join(dir, matches[0].Name())
	h, recs, complete := readReplay(t, filepath.Join(mdir, "round-01"+tvExt))
	if !complete {
		t.Fatal("round 1's file is not complete after round 2 started")
	}
	if h.Format != tvFormat || h.Round != 1 || h.Match != matches[0].Name() || h.Map.Name != "de_slop" || len(h.Map.Boxes) == 0 ||
		h.SchemaHash != netSchemaHash || string(h.Schema) != netSchemaJSON || h.Every != tvEvery || h.KeyEvery != tvKeyEvery || h.SimHz != SimHz {
		t.Fatalf("header %+v", h)
	}
	if len(h.Players) != 2 || h.Players[0].ID != v.ID || h.Players[0].Team != content.Vibe || h.Score[content.Vibe] != 0 {
		t.Fatalf("header players %+v score %v", h.Players, h.Score)
	}

	var d deltaDecoder
	var snaps, keys int
	var prevSnap, prevKey int64 = -1, -1
	var evs []string
	for i, r := range recs {
		if r.kind == tvEvent {
			var e map[string]any
			if err := json.Unmarshal(r.data, &e); err != nil {
				t.Fatalf("event %d: %v", i, err)
			}
			ev, _ := e["ev"].(string)
			evs = append(evs, e["t"].(string)+"/"+ev)
			continue
		}
		tick, back := snapTickBack(r.data)
		if err := d.apply(r.data); err != nil {
			t.Fatalf("snapshot %d (tick %d): %v", snaps, tick, err)
		}
		if prevSnap >= 0 && tick-prevSnap != tvEvery {
			t.Fatalf("snapshot at tick %d after %d: want every %d", tick, prevSnap, tvEvery)
		}
		if snaps == 0 && back != 0 {
			t.Fatal("the first snapshot is not a keyframe")
		}
		if back == 0 {
			if prevKey >= 0 && (tick-prevKey < tvKeyEvery || tick-prevKey >= tvKeyEvery+tvEvery) {
				t.Fatalf("keyframes at %d and %d: want one a second", prevKey, tick)
			}
			prevKey = tick
			keys++
		} else if back != tvEvery {
			t.Fatalf("tick %d: a delta from %d back, want the snapshot before (%d)", tick, back, tvEvery)
		}
		prevSnap = tick
		snaps++
		snap := toJSON(netSchema, d.state).(map[string]any)
		if snap["me"] != nil || snap["spot"] != nil {
			t.Fatalf("tick %d: an observer snapshot with me/spot: %v %v", tick, snap["me"], snap["spot"])
		}
		players, _ := snap["players"].([]any)
		if len(players) != 2 {
			t.Fatalf("tick %d: %d players", tick, len(players))
		}
		for _, p := range players {
			pm := p.(map[string]any)
			want := at[tick][int(pm["id"].(float64))]
			got := pm["p"].([]any)
			for k := range 3 {
				if math.Abs(got[k].(float64)-want[k]) > 6e-4 {
					t.Fatalf("tick %d player %v at %v, the server had %v", tick, pm["id"], got, want)
				}
			}
		}
	}
	if snaps < 60 || keys < 2 {
		t.Fatalf("%d snapshots, %d keyframes", snaps, keys)
	}
	joined := strings.Join(evs, " ")
	if !strings.HasPrefix(joined, "round/start") || !strings.Contains(joined, "kill/") || !strings.Contains(joined, "round/end") {
		t.Fatalf("events %s", joined)
	}

	// Round 2 is still being played: its file reads up to the last keyframe.
	h2, recs2, complete2 := readReplay(t, filepath.Join(mdir, "round-02"+tvExt))
	if complete2 || h2.Round != 2 || h2.Score[content.Vibe] != 1 || len(recs2) == 0 {
		t.Fatalf("live round 2: complete %v header %+v, %d records", complete2, h2, len(recs2))
	}
	var d2 deltaDecoder
	for _, r := range recs2 {
		if r.kind == tvSnap {
			if err := d2.apply(r.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if d2.state == nil || d2.tick < s.tick-tvKeyEvery-tvEvery {
		t.Fatalf("live file reaches tick %d, the server is at %d", d2.tick, s.tick)
	}

	// A map change ends the round's file; the next round on it is a new match.
	s.mu.Lock()
	s.changeLevel("de_stack")
	s.mu.Unlock()
	s.tvSync()
	if _, _, complete := readReplay(t, filepath.Join(mdir, "round-02"+tvExt)); !complete {
		t.Fatal("round 2's file not closed by the map change")
	}
}

// tv_autorecord 0: no new round file (and the round being recorded ends).
func TestReplayAutorecordOff(t *testing.T) {
	s, dir, v, _ := tvServer(t)
	s.cvars["tv_autorecord"] = "0"
	at := map[int64]map[int]Vec3{}
	tvRun(s, 50, v, at)
	s.tvSync()
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Fatalf("recorded with tv_autorecord 0: %v", es)
	}
	s.cvars["tv_autorecord"] = "1"
	s.mu.Lock()
	s.tvRoundStart()
	s.mu.Unlock()
	tvRun(s, 10, v, at)
	if s.tv.file == "" {
		t.Fatal("not recording")
	}
	s.cvars["tv_autorecord"] = "0"
	tvRun(s, 10, v, at)
	if s.tv.file != "" {
		t.Fatal("still recording after tv_autorecord 0")
	}
	s.tvSync()
}

// No replay directory: no recorder, nothing recorded, no cost.
func TestReplayOffWithoutDir(t *testing.T) {
	s := authServer(t)
	if err := s.SetReplays("", 0); err != nil || s.tv != nil || s.ReplayDir() != "" {
		t.Fatal("a recorder without a directory")
	}
	s.tvFrame(nil)
	s.tvEventOut([]byte(`{}`))
	s.tvStop()
	s.tvSync()
}

// Retention: beyond the cap, the oldest rounds go first (match ids start with
// their time), then their empty match directories.
func TestReplayRetention(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string, n int) {
		p := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("20261125-100000-de_slop/round-01"+tvExt, 100)
	write("20261125-100000-de_slop/round-02"+tvExt, 100)
	write("20261125-110000-de_stack/round-01"+tvExt, 100)
	write("20261125-110000-de_stack/round-02"+tvExt, 100)
	write("20261125-110000-de_stack/notes.txt", 1000) // not a replay: not counted, not deleted
	tv := &tvRecorder{dir: dir, maxBytes: 250}
	tv.retain()
	left := []string{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			left = append(left, rel)
		}
		return nil
	})
	want := "20261125-110000-de_stack/notes.txt 20261125-110000-de_stack/round-01.cstv.gz 20261125-110000-de_stack/round-02.cstv.gz"
	if strings.Join(left, " ") != want {
		t.Fatalf("left %v", left)
	}
	if _, err := os.Stat(filepath.Join(dir, "20261125-100000-de_slop")); !os.IsNotExist(err) {
		t.Fatal("the empty match directory stays")
	}
	tv.retain() // within the cap: nothing more goes
	if _, err := os.Stat(filepath.Join(dir, "20261125-110000-de_stack/round-01"+tvExt)); err != nil {
		t.Fatal(err)
	}
}

// The writer falling behind never blocks the tick: the round's data stops
// (the file is cut short), file boundaries still go through.
func TestReplayQueueOverflowDrops(t *testing.T) {
	tv := &tvRecorder{ops: make(chan tvOp, 2), file: "x"}
	tv.send(tvOp{data: []byte{1}})
	tv.send(tvOp{data: []byte{2}})
	tv.send(tvOp{data: []byte{3}}) // full: dropped, not blocking
	if !tv.dropped || len(tv.ops) != 2 {
		t.Fatalf("dropped %v, queued %d", tv.dropped, len(tv.ops))
	}
	<-tv.ops
	tv.send(tvOp{data: []byte{4}}) // after a drop, the round's data stays out (no hole in the deltas)
	if len(tv.ops) != 1 {
		t.Fatal("data after a drop queued")
	}
	tv.send(tvOp{close: true})
	if len(tv.ops) != 2 {
		t.Fatal("the close was lost")
	}
}

// A unique match id: a second match in the same second on the same map gets a suffix.
func TestReplayMatchIDUnique(t *testing.T) {
	dir := t.TempDir()
	tv := &tvRecorder{dir: dir}
	now := gameEpoch
	a := tv.newMatchID(now, "de_slop")
	_ = os.Mkdir(filepath.Join(dir, a), 0o755)
	if b := tv.newMatchID(now, "de_slop"); b == a || !strings.HasPrefix(b, a) {
		t.Fatalf("ids %q %q", a, b)
	}
}

// A real match records small and decodes whole: 5v5 AI bots for a minute on
// de_standup (rounds, kills, grenades). The size bound is the budget a
// server's disk gets per match (docs/replays.md has the numbers).
func TestReplayAIMatchDecodesAndStaysSmall(t *testing.T) {
	s := authServer(t)
	s.Map = maps.All["de_standup"]()
	s.Seed(1)
	dir := t.TempDir()
	if err := s.SetReplays(dir, 0); err != nil {
		t.Fatal(err)
	}
	s.cvars["mp_freezetime"] = "1"
	s.cvars["mp_round_restart_delay"] = "1"
	s.cvars["mp_respawn"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.Map.MaxPlayers = 10
	for range 5 {
		s.addAI(content.Vibe)
		s.addAI(content.Artisan)
	}
	const secs = 60
	s.Step(secs * SimHz)
	s.tvSync()
	var total int64
	rounds, snaps, events := 0, 0, 0
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, _ error) error {
		if d.IsDir() {
			return nil
		}
		info, _ := d.Info()
		total += info.Size()
		rounds++
		_, recs, _ := readReplay(t, p)
		var dec deltaDecoder
		for _, r := range recs {
			if r.kind == tvEvent {
				events++
				continue
			}
			if err := dec.apply(r.data); err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			snaps++
		}
		return nil
	})
	perSec := float64(total) / secs
	t.Logf("5v5 AI, %d s: %d round files, %d snapshots, %d events, %d KB (%.1f KB/s)", secs, rounds, snaps, events, total>>10, perSec/1024)
	if rounds < 2 || snaps < secs*SimHz/tvEvery*8/10 || events < 20 {
		t.Fatalf("%d rounds, %d snapshots, %d events", rounds, snaps, events)
	}
	if perSec > 8<<10 { // measured 2.3 KB/s
		t.Fatalf("%.1f KB/s recorded, budget 8", perSec/1024)
	}
}

// TestReplayGolden: a short recorded round (lab_bhop, two players, a kill)
// decompressed, and what each snapshot decodes to
// (testdata/replay_golden.json). web/engine/replay.test.mjs reads the same
// bytes with the client's reader and must get the same.
//
//	UPDATE=1 go test ./internal/game -run TestReplayGolden   rewrites the file
func TestReplayGolden(t *testing.T) {
	s, dir, v, a := tvServer(t)
	s.Seed(1)
	s.Map = maps.LabBhop()
	at := map[int64]map[int]Vec3{}
	tvRun(s, 130, v, at)
	s.mu.Lock()
	s.killBy(v, a, "prompt", true)
	s.mu.Unlock()
	tvRun(s, 20, v, at)
	s.mu.Lock()
	s.tvStop()
	s.mu.Unlock()
	s.tvSync()
	var path string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			path = p
		}
		return nil
	})
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	gz, _ := gzip.NewReader(f)
	raw, err := io.ReadAll(gz)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	type snapW struct {
		Tick int64 `json:"tick"`
		Key  bool  `json:"key"`
		Want any   `json:"want"`
	}
	type evW struct {
		Tick int64           `json:"tick"`
		Msg  json.RawMessage `json:"msg"`
	}
	g := struct {
		Raw    []byte  `json:"raw"` // base64: the file's bytes, ungzipped
		Snaps  []snapW `json:"snaps"`
		Events []evW   `json:"events"`
	}{Raw: raw}
	_, recs, _ := readReplay(t, path)
	var d deltaDecoder
	for _, r := range recs {
		if r.kind == tvEvent {
			g.Events = append(g.Events, evW{r.tick, r.data})
			continue
		}
		if err := d.apply(r.data); err != nil {
			t.Fatal(err)
		}
		_, back := snapTickBack(r.data)
		g.Snaps = append(g.Snaps, snapW{r.tick, back == 0, toJSON(netSchema, d.state)})
	}
	if len(g.Snaps) < 40 || len(g.Events) < 2 {
		t.Fatalf("%d snapshots, %d events", len(g.Snaps), len(g.Events))
	}
	got, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	const golden = "testdata/replay_golden.json"
	if os.Getenv("UPDATE") != "" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (UPDATE=1 go test ./internal/game -run TestReplayGolden writes it)", err)
	}
	if string(want) != string(got) {
		t.Fatalf("%s is stale: the replay format, the codec or the schema changed (UPDATE=1 go test ./internal/game -run TestReplayGolden rewrites it)", golden)
	}
}
