package game

import (
	"bufio"
	"compress/gzip"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Replays over HTTP (docs/replays.md, part 2):
//
//	GET /replays.json                 the matches, newest first, each with its rounds
//	GET /replays/{match}/{file}       one round's file (a live round: up to its last keyframe)
//	GET /replays                      the same list as a plain page with download links
//
// tv_access (cvar): public (default) lets anyone list and download; admin
// asks for the admin password (HTTP Basic auth, any user name; rate limited
// per IP like admin_login) or the token admins get in their cvars message
// (?token=, for the client's Replays screen). Without an admin password
// everyone is admin, so admin is the same as public there.

// ReplayRound is one round file in the list.
type ReplayRound struct {
	Round    int            `json:"round"`
	File     string         `json:"file"` // round-NN.cstv.gz
	URL      string         `json:"url"`  // /replays/<match>/<file>
	Size     int64          `json:"size"` // bytes (compressed), as of now for a live round
	Modified int64          `json:"modified"`
	Live     bool           `json:"live,omitempty"`  // being recorded: the file grows every second
	Score    map[string]int `json:"score,omitempty"` // the score before the round
	Players  int            `json:"players"`         // on the server as it started
	Started  int64          `json:"started"`         // unix ms
}

// ReplayMatch is one match directory: its rounds in order.
type ReplayMatch struct {
	ID      string        `json:"id"`
	Map     string        `json:"map"`
	Started int64         `json:"started"` // unix ms (its first round's start)
	Version string        `json:"version,omitempty"`
	Live    bool          `json:"live,omitempty"`
	Rounds  []ReplayRound `json:"rounds"`
}

var (
	tvMatchRe = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[a-z0-9_]+(-[0-9]+)?$`)
	tvRoundRe = regexp.MustCompile(`^round-([0-9]{2,})\.cstv\.gz$`)
)

// tvSummary is what the list shows of a file's header (read once per file: a
// header doesn't change after it is written).
type tvSummary struct {
	Map     string
	Version string
	Started int64
	Score   map[string]int
	Players int
}

// tvHeaders caches the files' header summaries by path.
type tvHeaders struct {
	mu sync.Mutex
	m  map[string]tvSummary
}

// readHeader reads a replay file's header (only as much of the stream as it takes).
func readHeader(path string) (tvHeader, error) {
	var h tvHeader
	f, err := os.Open(path)
	if err != nil {
		return h, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return h, err
	}
	r := bufio.NewReader(gz)
	magic := make([]byte, len(tvMagic))
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != tvMagic {
		return h, errors.New("not a replay")
	}
	n, err := binary.ReadUvarint(r)
	if err != nil || n > 64<<20 {
		return h, errors.New("bad header length")
	}
	hb := make([]byte, n)
	if _, err := io.ReadFull(r, hb); err != nil {
		return h, err
	}
	return h, json.Unmarshal(hb, &h)
}

func (c *tvHeaders) get(path string) (tvSummary, bool) {
	c.mu.Lock()
	sum, ok := c.m[path]
	c.mu.Unlock()
	if ok {
		return sum, true
	}
	h, err := readHeader(path)
	if err != nil {
		return tvSummary{}, false // a file just opened (no header flushed yet), or not ours
	}
	sum = tvSummary{Map: h.Map.Name, Version: h.Version, Started: h.Started, Score: h.Score, Players: len(h.Players)}
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]tvSummary{}
	}
	c.m[path] = sum
	c.mu.Unlock()
	return sum, true
}

// prune forgets the files that are gone (retention deleted them).
func (c *tvHeaders) prune(keep map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for p := range c.m {
		if !keep[p] {
			delete(c.m, p)
		}
	}
}

// ReplayList lists the recorded matches, newest first, each with its rounds
// in order (nil when the server isn't recording).
func (s *Server) ReplayList() ([]ReplayMatch, error) {
	if s.tv == nil {
		return nil, nil
	}
	s.mu.Lock()
	live := s.tv.file
	s.mu.Unlock()
	dirs, err := os.ReadDir(s.tv.dir)
	if err != nil {
		return nil, err
	}
	out := []ReplayMatch{}
	seen := map[string]bool{}
	for _, d := range dirs {
		if !d.IsDir() || !tvMatchRe.MatchString(d.Name()) {
			continue
		}
		m := ReplayMatch{ID: d.Name(), Rounds: []ReplayRound{}}
		files, err := os.ReadDir(filepath.Join(s.tv.dir, d.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			sub := tvRoundRe.FindStringSubmatch(f.Name())
			if sub == nil || f.IsDir() {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			path := filepath.Join(s.tv.dir, d.Name(), f.Name())
			n, _ := strconv.Atoi(sub[1])
			rr := ReplayRound{Round: n, File: f.Name(), URL: "/replays/" + d.Name() + "/" + f.Name(), Size: info.Size(),
				Modified: info.ModTime().UnixMilli(), Live: path == live}
			if sum, ok := s.tv.headers.get(path); ok {
				seen[path] = true
				rr.Score, rr.Players, rr.Started = sum.Score, sum.Players, sum.Started
				if m.Map == "" || rr.Started < m.Started {
					m.Map, m.Started, m.Version = sum.Map, sum.Started, sum.Version
				}
			} else if !rr.Live {
				continue // unreadable and not being written: not a replay
			}
			m.Live = m.Live || rr.Live
			m.Rounds = append(m.Rounds, rr)
		}
		if len(m.Rounds) == 0 {
			continue
		}
		slices.SortFunc(m.Rounds, func(a, b ReplayRound) int { return a.Round - b.Round })
		out = append(out, m)
	}
	s.tv.headers.prune(seen)
	// newest first: match ids start with their start time
	slices.SortFunc(out, func(a, b ReplayMatch) int { return strings.Compare(b.ID, a.ID) })
	return out, nil
}

// newTVToken is a random token for the admins' replay downloads.
func newTVToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// replayAllowed: tv_access lets this request list and download. Basic auth
// with the admin password, or the admins' token. Failed passwords count
// towards the IP's login backoff.
func (s *Server) replayAllowed(r *http.Request) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cvar("tv_access") != "admin" || s.openAdmin() {
		return true
	}
	if tok := r.URL.Query().Get("token"); tok != "" && s.tv != nil &&
		subtle.ConstantTimeCompare([]byte(tok), []byte(s.tv.token)) == 1 {
		return true
	}
	_, pw, ok := r.BasicAuth()
	if !ok {
		return false
	}
	ip, now := clientIP(r), s.clock()
	f := s.loginFails[ip]
	if f == nil {
		f = &loginFails{}
		s.loginFails[ip] = f
	}
	if now.Before(f.blockedUntil) {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(pw), []byte(s.AdminPassword)) == 1 {
		f.times = nil
		return true
	}
	kept := f.times[:0]
	for _, t := range f.times {
		if now.Sub(t) < loginFailWindow {
			kept = append(kept, t)
		}
	}
	f.times = append(kept, now)
	slog.Warn("replay download: bad admin password", "ip", ip, "recent", len(f.times))
	if len(f.times) >= loginFailLimit {
		f.blockedUntil = now.Add(loginBackoff)
		f.times = nil
	}
	return false
}

// replayGate answers a request tv_access refuses (and says so); true: go on.
func (s *Server) replayGate(w http.ResponseWriter, r *http.Request) bool {
	if s.tv == nil {
		http.Error(w, "this server doesn't record replays (--replay-dir)", http.StatusNotFound)
		return false
	}
	if !s.replayAllowed(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="counterslop replays (admin password)"`)
		http.Error(w, "replays are for admins on this server (tv_access admin): log in with the admin password", http.StatusUnauthorized)
		return false
	}
	return true
}

// HandleReplaysJSON serves /replays.json.
func (s *Server) HandleReplaysJSON(w http.ResponseWriter, r *http.Request) {
	if !s.replayGate(w, r) {
		return
	}
	list, err := s.ReplayList()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"matches": list})
}

// HandleReplayFile serves /replays/{match}/{file}: a closed round as it is,
// a live one as far as it is written (it reads up to its last keyframe).
func (s *Server) HandleReplayFile(w http.ResponseWriter, r *http.Request) {
	if !s.replayGate(w, r) {
		return
	}
	match, file := r.PathValue("match"), r.PathValue("file")
	if !tvMatchRe.MatchString(match) || !tvRoundRe.MatchString(file) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.tv.dir, match, file)
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	live := path == s.tv.file
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/gzip") // the bytes as stored: no Content-Encoding, nobody unzips it on the way
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s"`, match, file))
	if live {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Replay-Live", "1")
		// What is on disk now (the writer appends; a copy of a fixed length, not a range server).
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		_, _ = io.CopyN(w, f, info.Size())
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

var replaysPage = template.Must(template.New("replays").Funcs(template.FuncMap{
	"kb":   func(n int64) string { return fmt.Sprintf("%.0f KB", float64(n)/1024) },
	"when": func(ms int64) string { return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04 UTC") },
	"score": func(sc map[string]int) string {
		if sc == nil {
			return ""
		}
		return fmt.Sprintf("%d : %d", sc["vibe"], sc["artisan"])
	},
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>counterslop replays</title>
<style>
body{font:15px/1.4 system-ui,sans-serif;margin:0 auto;max-width:860px;padding:16px;background:#14161a;color:#e8e8e8}
h1{font-size:22px}h2{font-size:17px;margin:24px 0 6px}a{color:#7fd4ff}
table{border-collapse:collapse;width:100%}td,th{padding:4px 8px;text-align:left;border-bottom:1px solid #2a2e35}
.live{color:#ff5a5a;font-weight:600}.dim{color:#9aa0a8}
</style></head><body>
<h1>Replays</h1>
<p class="dim">Every round is recorded (GOTV style); a live round downloads up to its last second.</p>
{{if not .Matches}}<p>No replays yet: they start with the first round after warmup.</p>{{end}}
{{range .Matches}}<h2>{{.Map}} <span class="dim">· {{when .Started}} · {{.ID}}</span>{{if .Live}} <span class="live">LIVE</span>{{end}}</h2>
<table><tr><th>Round</th><th>Score before</th><th>Players</th><th>Size</th><th></th></tr>
{{range .Rounds}}<tr><td>{{.Round}}{{if .Live}} <span class="live">live</span>{{end}}</td><td>{{score .Score}}</td><td>{{.Players}}</td><td>{{kb .Size}}</td><td><a href="{{.URL}}{{if $.Token}}?token={{$.Token}}{{end}}" download>download</a></td></tr>
{{end}}</table>{{end}}
</body></html>
`))

// HandleReplaysPage serves /replays: the list as a page with download links.
func (s *Server) HandleReplaysPage(w http.ResponseWriter, r *http.Request) {
	if !s.replayGate(w, r) {
		return
	}
	list, err := s.ReplayList()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = replaysPage.Execute(w, map[string]any{"Matches": list, "Token": r.URL.Query().Get("token")})
}
