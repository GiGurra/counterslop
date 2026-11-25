package game

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// Replays (GOTV style): the server records every round as the observer's view
// of the game, one file per round, while it plays (docs/replays.md).
//
// A file is a gzip stream (flushed every keyframe, so a round still being
// played reads up to its last second) of:
//
//	"CSTV" | uvarint header length | header JSON (tvHeader)
//	records: 'S' | uvarint length | a delta-codec snapshot (0xD5 ..., netdelta_codec.go)
//	         'E' | uvarint tick | uvarint length | an event message's JSON
//
// Snapshots are the delta codec's messages with no spot and no me (what a
// spectator of the whole game sees), every tvEvery ticks (~33 Hz), each a
// delta from the one before, and a keyframe (a full one, base 0) every
// tvKeyEvery ticks: a player seeks to the keyframe before a time and decodes
// forward. The header carries the snapshot schema itself, so a replay
// decodes with its own layout after the schema changes.
//
// Events are the optional messages the clients got (shots, hits, kills,
// grenades, round events): sounds and the kill feed. Everything that matters
// is in the snapshots, as on the wire.
const (
	tvMagic        = "CSTV"
	tvFormat       = 1
	tvEvery        = 3     // ticks between recorded snapshots (~33 Hz)
	tvKeyEvery     = SimHz // ticks between keyframes (1 s)
	tvExt          = ".cstv.gz"
	tvSnap         = 'S'
	tvEvent        = 'E'
	tvQueue        = 4096 // writes queued to the writer goroutine before the recorder drops a round
	tvDefaultMaxMB = 2048
)

// tvHeader opens every replay file.
type tvHeader struct {
	Format     int             `json:"format"`
	Version    string          `json:"version"` // the server binary that recorded it
	Match      string          `json:"match"`   // the match's id (= its directory)
	Round      int             `json:"round"`   // the round number (1-based, overtime counts on)
	Map        content.Map     `json:"map"`     // the whole map: a replay plays without the server
	SchemaHash string          `json:"schemaHash"`
	Schema     json.RawMessage `json:"schema"` // the snapshot layout (NET_SCHEMA): decodes after the schema changes
	SimHz      int             `json:"simHz"`
	Every      int             `json:"every"`    // ticks between snapshots
	KeyEvery   int             `json:"keyEvery"` // ticks between keyframes
	StartTick  int64           `json:"startTick"`
	Started    int64           `json:"started"` // unix ms (the server's game clock)
	Players    []tvPlayer      `json:"players"` // who was on the server as the round started
	Score      map[string]int  `json:"score"`   // the score before the round
}

type tvPlayer struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Team string `json:"team"`
	Bot  bool   `json:"bot,omitempty"`
}

// tvRecorder is the server's recorder. Encoding happens on the tick (it
// reads the state, under s.mu); the gzip and the disk on its own goroutine.
type tvRecorder struct {
	dir      string
	maxBytes int64
	ops      chan tvOp

	// the round being recorded (file == "": none)
	file      string
	match     string
	matchMap  string // the map the match id was made on
	base      *nv
	baseWorld *deltaWorld
	baseTick  int64
	lastSnap  int64
	lastKey   int64
	dropped   bool // the queue overflowed: the round's file is cut short
}

type tvOp struct {
	open  string // a new file at this path (after closing the current one)
	data  []byte // bytes for the current file
	flush bool
	close bool
	done  chan struct{} // closed when every op before it is done (sync)
}

// SetReplays turns the recorder on: round files go under dir (one directory
// per match), the oldest deleted once they hold more than maxMB. Before Run.
func (s *Server) SetReplays(dir string, maxMB int) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("replay dir: %w", err)
	}
	if maxMB <= 0 {
		maxMB = tvDefaultMaxMB
	}
	tv := &tvRecorder{dir: dir, maxBytes: int64(maxMB) << 20, ops: make(chan tvOp, tvQueue)}
	go tv.writer()
	s.tv = tv
	return nil
}

// ReplayDir is where replays go ("" = not recording).
func (s *Server) ReplayDir() string {
	if s.tv == nil {
		return ""
	}
	return s.tv.dir
}

// tvOn: tv_autorecord (default 1). Callers hold s.mu.
func (s *Server) tvOn() bool { return s.tv != nil && s.cvar("tv_autorecord") != "0" }

// tvRoundStart closes the round being recorded and starts the next one's
// file. A new match (round 1, or another map) gets a new directory. Callers
// hold s.mu.
func (s *Server) tvRoundStart() {
	tv := s.tv
	if tv == nil {
		return
	}
	tv.stop()
	if !s.tvOn() {
		return
	}
	now := s.clock()
	r := &s.round
	if tv.match == "" || r.Number <= 1 || tv.matchMap != s.Map.Name {
		tv.match, tv.matchMap = tv.newMatchID(now, s.Map.Name), s.Map.Name
	}
	h := tvHeader{Format: tvFormat, Version: s.Version, Match: tv.match, Round: r.Number, Map: s.Map,
		SchemaHash: netSchemaHash, Schema: json.RawMessage(netSchemaJSON), SimHz: SimHz, Every: tvEvery, KeyEvery: tvKeyEvery,
		StartTick: s.tick, Started: now.UnixMilli(), Score: maps.Clone(r.Score)}
	for _, p := range s.ordered() {
		h.Players = append(h.Players, tvPlayer{ID: p.ID, Name: p.Name, Team: p.Team, Bot: p.Bot})
	}
	hb, err := json.Marshal(h)
	if err != nil {
		slog.Error("replay header", "err", err)
		return
	}
	b := append([]byte(tvMagic), binary.AppendUvarint(nil, uint64(len(hb)))...)
	tv.file = filepath.Join(tv.dir, tv.match, fmt.Sprintf("round-%02d%s", r.Number, tvExt))
	tv.base, tv.baseWorld, tv.lastSnap, tv.lastKey, tv.dropped = nil, nil, -tvEvery, -tvKeyEvery, false
	tv.send(tvOp{open: tv.file})
	tv.send(tvOp{data: append(b, hb...)})
}

// newMatchID: the match's start time (UTC, the game clock) and map, unique
// in the replay directory.
func (tv *tvRecorder) newMatchID(now time.Time, mapName string) string {
	id := now.UTC().Format("20060102-150405") + "-" + mapName
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(tv.dir, id)); os.IsNotExist(err) {
			return id
		}
		id = fmt.Sprintf("%s-%s-%d", now.UTC().Format("20060102-150405"), mapName, i)
	}
}

// stop ends the current round's file, if any.
func (tv *tvRecorder) stop() {
	if tv.file == "" {
		return
	}
	tv.send(tvOp{close: true})
	tv.file, tv.base, tv.baseWorld = "", nil, nil
}

// tvStop ends the recording (a map change, warmup, tv_autorecord 0). Callers hold s.mu.
func (s *Server) tvStop() {
	if s.tv != nil {
		s.tv.stop()
	}
}

// tvFrame records this tick's observer snapshot when one is due. list: the
// ready players, as the codecs get them. Callers hold s.mu.
func (s *Server) tvFrame(list []*Player) {
	tv := s.tv
	if tv == nil || tv.file == "" {
		return
	}
	if s.round.Phase == PhaseWarmup || !s.tvOn() {
		tv.stop()
		return
	}
	if s.tick-tv.lastSnap < tvEvery {
		return
	}
	w := s.deltaWorldOf(list)
	base, bw := tv.base, tv.baseWorld
	key := base == nil || s.tick-tv.lastKey >= tvKeyEvery
	if key {
		base, bw = nil, nil
	}
	msg := deltaEncode(w, &w.root, base, bw, s.tick, tv.baseTick)
	b := append([]byte{tvSnap}, binary.AppendUvarint(nil, uint64(len(msg)))...)
	tv.send(tvOp{data: append(b, msg...), flush: key})
	tv.base, tv.baseWorld, tv.baseTick, tv.lastSnap = &w.root, w, s.tick, s.tick
	if key {
		tv.lastKey = s.tick
	}
}

// tvEventOut records an event message (JSON) everyone got. Callers hold s.mu.
func (s *Server) tvEventOut(msg []byte) {
	tv := s.tv
	if tv == nil || tv.file == "" {
		return
	}
	b := []byte{tvEvent}
	b = binary.AppendUvarint(b, uint64(s.tick))
	b = binary.AppendUvarint(b, uint64(len(msg)))
	tv.send(tvOp{data: append(b, msg...)})
}

// send queues an op for the writer, never blocking the tick: if the disk
// can't keep up, the round's file is cut where the queue overflowed.
func (tv *tvRecorder) send(op tvOp) {
	if tv.dropped && op.open == "" && !op.close {
		return
	}
	select {
	case tv.ops <- op:
	default:
		if !tv.dropped {
			slog.Warn("replay: the writer is behind, cutting the round short", "file", tv.file)
		}
		tv.dropped = true
		if op.close || op.open != "" {
			tv.ops <- op // a file boundary: wait for the writer
		}
	}
}

// tvSync waits until the writer has done everything queued (tests).
func (s *Server) tvSync() {
	if s.tv == nil {
		return
	}
	done := make(chan struct{})
	s.tv.ops <- tvOp{done: done}
	<-done
}

// writer owns the files: gzip, flushes, closes, and retention after each
// closed round.
func (tv *tvRecorder) writer() {
	var f *os.File
	var bw *bufio.Writer
	var gz *gzip.Writer
	closeCur := func() {
		if f == nil {
			return
		}
		err := gz.Close()
		if err == nil {
			err = bw.Flush()
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			slog.Error("replay: closing", "file", f.Name(), "err", err)
		}
		f, bw, gz = nil, nil, nil
		tv.retain()
	}
	for op := range tv.ops {
		switch {
		case op.done != nil:
			close(op.done)
		case op.open != "":
			closeCur()
			if err := os.MkdirAll(filepath.Dir(op.open), 0o755); err != nil {
				slog.Error("replay: dir", "err", err)
				continue
			}
			nf, err := os.Create(op.open)
			if err != nil {
				slog.Error("replay: create", "err", err)
				continue
			}
			f, bw = nf, bufio.NewWriterSize(nf, 64<<10)
			gz, _ = gzip.NewWriterLevel(bw, gzip.DefaultCompression)
		case op.close:
			closeCur()
		case f != nil:
			if _, err := gz.Write(op.data); err != nil {
				slog.Error("replay: write", "err", err)
			}
			if op.flush { // a keyframe: what is written so far reads as a replay (a live round)
				_ = gz.Flush()
				_ = bw.Flush()
			}
		}
	}
}

// tvFile is a replay file on disk.
type tvFile struct {
	path string
	size int64
	mod  time.Time
}

// retain deletes the oldest round files (and then empty match directories)
// until the replays take at most maxBytes. Runs on the writer, between files.
func (tv *tvRecorder) retain() {
	var files []tvFile
	var total int64
	_ = filepath.WalkDir(tv.dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, tvExt) {
			return nil
		}
		if info, err := d.Info(); err == nil {
			files = append(files, tvFile{p, info.Size(), info.ModTime()})
			total += info.Size()
		}
		return nil
	})
	if total <= tv.maxBytes {
		return
	}
	// oldest first: by path (match ids start with their time, rounds count up), mtime as a tiebreak
	slices.SortFunc(files, func(a, b tvFile) int {
		if c := strings.Compare(a.path, b.path); c != 0 {
			return c
		}
		return a.mod.Compare(b.mod)
	})
	for _, f := range files {
		if total <= tv.maxBytes {
			break
		}
		if err := os.Remove(f.path); err == nil {
			total -= f.size
			_ = os.Remove(filepath.Dir(f.path)) // fails unless the match directory is empty now
		}
	}
}

// tvHit records a hit the shooter's client was told about (the hit marker),
// with the shooter's id ("by"): only the shooter got it live. Callers hold s.mu.
func (s *Server) tvHit(shooter *Player, msg []byte) {
	if s.tv == nil || s.tv.file == "" || len(msg) < 2 {
		return
	}
	s.tvEventOut(append(fmt.Appendf(nil, `{"by":%d,`, shooter.ID), msg[1:]...))
}
