package game

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// replayMux routes the replay handlers as internal/app does.
func replayMux(s *Server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /replays", s.HandleReplaysPage)
	mux.HandleFunc("GET /replays.json", s.HandleReplaysJSON)
	mux.HandleFunc("GET /replays/{match}/{file}", s.HandleReplayFile)
	return mux
}

func get(t *testing.T, h http.Handler, url string, edit ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", url, nil)
	for _, e := range edit {
		e(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// tvTwoRounds plays round 1 to a kill and 1.5 s into round 2 (still being recorded).
func tvTwoRounds(t *testing.T) (*Server, string) {
	t.Helper()
	s, dir, v, a := tvServer(t)
	at := map[int64]map[int]Vec3{}
	tvRun(s, 250, v, at)
	s.mu.Lock()
	s.killBy(v, a, "prompt", true)
	s.mu.Unlock()
	tvRun(s, 200, v, at)
	tvRun(s, 150, v, at)
	s.tvSync()
	if s.round.Number != 2 {
		t.Fatalf("round %d, want 2", s.round.Number)
	}
	return s, dir
}

// /replays.json: the match with its rounds in order, the header's map,
// score and players, round 2 live; each round downloads as the file on disk
// (round 2 as far as it is written, and it reads as a replay up to its last
// keyframe), and the page lists the same with links.
func TestReplayListAndDownload(t *testing.T) {
	s, dir := tvTwoRounds(t)
	mux := replayMux(s)
	w := get(t, mux, "/replays.json")
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	var list struct{ Matches []ReplayMatch }
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Matches) != 1 {
		t.Fatalf("matches %+v", list.Matches)
	}
	m := list.Matches[0]
	if m.Map != "de_slop" || !m.Live || len(m.Rounds) != 2 || m.Started == 0 || !strings.HasSuffix(m.ID, "-de_slop") {
		t.Fatalf("match %+v", m)
	}
	r1, r2 := m.Rounds[0], m.Rounds[1]
	if r1.Round != 1 || r1.Live || r2.Round != 2 || !r2.Live || r1.Players != 2 || r2.Score["vibe"] != 1 || r1.Score["vibe"] != 0 {
		t.Fatalf("rounds %+v", m.Rounds)
	}
	for _, r := range m.Rounds {
		w := get(t, mux, r.URL)
		disk, _ := os.ReadFile(filepath.Join(dir, m.ID, r.File))
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), disk) || w.Header().Get("Content-Type") != "application/gzip" ||
			w.Header().Get("Content-Encoding") != "" || !strings.Contains(w.Header().Get("Content-Disposition"), m.ID+"-"+r.File) {
			t.Fatalf("%s: %d, %d bytes (disk %d), headers %v", r.URL, w.Code, w.Body.Len(), len(disk), w.Header())
		}
		if r.Live != (w.Header().Get("X-Replay-Live") == "1") || r.Live != (w.Header().Get("Cache-Control") == "no-store") {
			t.Fatalf("%s: live %v, headers %v", r.URL, r.Live, w.Header())
		}
		got := filepath.Join(t.TempDir(), r.File)
		_ = os.WriteFile(got, w.Body.Bytes(), 0o644)
		h, recs, _ := readReplay(t, got)
		if h.Round != r.Round || len(recs) < 10 {
			t.Fatalf("%s downloaded: round %d, %d records", r.URL, h.Round, len(recs))
		}
	}
	page := get(t, mux, "/replays")
	if page.Code != 200 || !strings.Contains(page.Body.String(), r2.URL) || !strings.Contains(page.Body.String(), "LIVE") {
		t.Fatalf("page: %d %s", page.Code, page.Body)
	}
	// Not ours: other names, traversal, files that aren't there.
	for _, u := range []string{"/replays/" + m.ID + "/round-09.cstv.gz", "/replays/" + m.ID + "/notes.txt", "/replays/..%2F..%2Fetc/round-01.cstv.gz", "/replays/x/round-01.cstv.gz"} {
		if w := get(t, mux, u); w.Code != 404 {
			t.Fatalf("%s: %d", u, w.Code)
		}
	}
}

// A finished match drops off the live mark; retention deleting a round
// drops it from the list; a stray file in the directory isn't listed.
func TestReplayListSkipsStrays(t *testing.T) {
	s, dir := tvTwoRounds(t)
	list, _ := s.ReplayList()
	_ = os.WriteFile(filepath.Join(dir, list[0].ID, "round-07"+tvExt), []byte("junk"), 0o644)
	_ = os.Mkdir(filepath.Join(dir, "lost+found"), 0o755)
	_ = os.Remove(filepath.Join(dir, list[0].ID, "round-01"+tvExt))
	s.mu.Lock()
	s.tvStop()
	s.mu.Unlock()
	s.tvSync()
	list, err := s.ReplayList()
	if err != nil || len(list) != 1 || len(list[0].Rounds) != 1 || list[0].Rounds[0].Round != 2 || list[0].Live || list[0].Rounds[0].Live {
		t.Fatalf("list %+v %v", list, err)
	}
}

// tv_access admin: the list and the files want the admin password (Basic
// auth) or the admins' token; repeated bad passwords lock the IP out a while.
// Without an admin password everyone is admin.
func TestReplayAccessAdmin(t *testing.T) {
	s, _ := tvTwoRounds(t)
	mux := replayMux(s)
	list, _ := s.ReplayList()
	file := list[0].Rounds[0].URL
	s.cvars["tv_access"] = "admin"
	if w := get(t, mux, "/replays.json"); w.Code != 200 {
		t.Fatalf("no admin password: %d", w.Code)
	}
	s.AdminPassword = "hunter2"
	for _, u := range []string{"/replays.json", "/replays", file} {
		if w := get(t, mux, u); w.Code != 401 || !strings.Contains(w.Header().Get("WWW-Authenticate"), "Basic") {
			t.Fatalf("%s without a password: %d", u, w.Code)
		}
		if w := get(t, mux, u, func(r *http.Request) { r.SetBasicAuth("", "hunter2") }); w.Code != 200 {
			t.Fatalf("%s with the password: %d", u, w.Code)
		}
		if w := get(t, mux, u+"?token="+s.tv.token); w.Code != 200 {
			t.Fatalf("%s with the token: %d", u, w.Code)
		}
	}
	if w := get(t, mux, file+"?token=nope"); w.Code != 401 {
		t.Fatalf("a wrong token: %d", w.Code)
	}
	for range loginFailLimit {
		get(t, mux, "/replays.json", func(r *http.Request) { r.SetBasicAuth("", "guess") })
	}
	if w := get(t, mux, "/replays.json", func(r *http.Request) { r.SetBasicAuth("", "hunter2") }); w.Code != 401 {
		t.Fatalf("locked out IP with the right password: %d", w.Code)
	}
	s.cvars["tv_access"] = "public"
	if w := get(t, mux, file); w.Code != 200 {
		t.Fatalf("public: %d", w.Code)
	}
	// The token reaches admins (the client's Replays screen), nobody else.
	p := s.join("3.3.3.3")
	s.mu.Lock()
	var c map[string]any
	_ = json.Unmarshal(s.cvarsMsg(p), &c)
	if _, ok := c["tvToken"]; ok {
		t.Fatal("a non-admin got the replay token")
	}
	p.admin = true
	_ = json.Unmarshal(s.cvarsMsg(p), &c)
	s.mu.Unlock()
	if c["tvToken"] != s.tv.token {
		t.Fatalf("an admin's cvars: %v", c["tvToken"])
	}
}

// No recorder: the endpoints say so.
func TestReplayEndpointsOff(t *testing.T) {
	s := authServer(t)
	mux := replayMux(s)
	w := get(t, mux, "/replays.json")
	b, _ := io.ReadAll(w.Body)
	if w.Code != 404 || !strings.Contains(string(b), "replay-dir") {
		t.Fatalf("%d %s", w.Code, b)
	}
}
