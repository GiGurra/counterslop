package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
)

func get(t *testing.T, h http.Handler, path string, hdr ...string) (*http.Response, string) {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	res := w.Result()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func newApp(t *testing.T, o Options) *App {
	t.Helper()
	a, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestEndpoints(t *testing.T) {
	a := newApp(t, Options{Version: "abc123", Bots: -1})
	if len(a.Build) != 12 || strings.HasPrefix(a.Build, "dev-") {
		t.Fatalf("build %q, want a 12-hex content hash", a.Build)
	}
	for _, tc := range []struct {
		path, body string
		code       int
	}{
		{"/healthz", "ok\n", 200},
		{"/version", "abc123\n", 200},
		{"/debug/state", "", 404}, // test mode only
	} {
		res, body := get(t, a.Handler, tc.path)
		if res.StatusCode != tc.code || (tc.body != "" && body != tc.body) {
			t.Errorf("%s: %d %q", tc.path, res.StatusCode, body)
		}
	}
	res, body := get(t, a.Handler, "/map.json")
	var m struct {
		Name  string
		Boxes []any
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil || m.Name != "de_slop" || len(m.Boxes) == 0 || res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("/map.json: %v %s", err, body)
	}
	// ?name= serves any map or lab (the gallery's lighting lab), 404 for unknown names.
	_, body = get(t, a.Handler, "/map.json?name=lab_lights")
	var lab struct {
		Name   string
		Lights []any
	}
	if err := json.Unmarshal([]byte(body), &lab); err != nil || lab.Name != "lab_lights" || len(lab.Lights) == 0 {
		t.Fatalf("/map.json?name=lab_lights: %v %.200s", err, body)
	}
	if res, _ := get(t, a.Handler, "/map.json?name=de_nope"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown map: %d", res.StatusCode)
	}
	res, body = get(t, a.Handler, "/maps.json")
	var cat []struct {
		Name string
		Size [2]float64
	}
	if err := json.Unmarshal([]byte(body), &cat); err != nil || len(cat) < 4 || cat[0].Name != "de_slop" || cat[0].Size[0] <= 0 || res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("/maps.json: %v %.200s", err, body)
	}
	res, _ = get(t, a.Handler, "/dev/assets")
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/dev/assets.html" {
		t.Fatalf("/dev/assets: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	if a.Server.Weapons == nil || a.Server.Weapons.Get("prompt") == nil || a.Server.Version != "abc123" || a.Server.Bots != -1 {
		t.Fatal("server not configured")
	}
}

func TestIndexIsStamped(t *testing.T) {
	for _, test := range []bool{false, true} {
		a := newApp(t, Options{Test: test})
		for _, path := range []string{"/", "/index.html"} {
			res, body := get(t, a.Handler, path)
			if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
				t.Fatalf("%s: %d %v", path, res.StatusCode, res.Header)
			}
			if !strings.Contains(body, `content="`+a.Build+`"`) || strings.Contains(body, "__BUILD__") {
				t.Fatalf("%s: build not stamped", path)
			}
			if strings.Contains(body, `content="__TEST__"`) == test || strings.Contains(body, `name="cs-test" content="1"`) != test {
				t.Fatalf("%s test=%v: test flag stamping wrong", path, test)
			}
		}
		res, body := get(t, a.Handler, "/debug/state")
		if test && (res.StatusCode != 200 || !strings.Contains(body, `"map":"de_slop"`)) {
			t.Fatalf("/debug/state in test mode: %d %s", res.StatusCode, body)
		}
	}
}

func TestStaticFilesHaveETags(t *testing.T) {
	a := newApp(t, Options{})
	res, body := get(t, a.Handler, "/assets/weapons/weapons.json")
	tag := res.Header.Get("ETag")
	if res.StatusCode != 200 || !strings.Contains(body, `"weapons"`) || len(tag) != 18 || res.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("static: %d etag %q cache %q", res.StatusCode, tag, res.Header.Get("Cache-Control"))
	}
	res, _ = get(t, a.Handler, "/assets/weapons/weapons.json", "If-None-Match", tag)
	if res.StatusCode != http.StatusNotModified {
		t.Fatalf("revalidation: %d", res.StatusCode)
	}
	res, _ = get(t, a.Handler, "/assets/weapons/weapons.json", "If-None-Match", `"stale"`)
	if res.StatusCode != 200 {
		t.Fatalf("stale etag: %d", res.StatusCode)
	}
	res, _ = get(t, a.Handler, "/no/such/file.js")
	if res.StatusCode != 404 || res.Header.Get("ETag") != "" {
		t.Fatalf("missing file: %d %q", res.StatusCode, res.Header.Get("ETag"))
	}
	// Same content, same build: the hash is stable.
	if b := newApp(t, Options{}).Build; b != a.Build {
		t.Fatalf("build changed between identical apps: %s vs %s", a.Build, b)
	}
}

func TestDevModeServesFromDisk(t *testing.T) {
	a := newApp(t, Options{Dev: "../../web"})
	if !strings.HasPrefix(a.Build, "dev-") {
		t.Fatalf("dev build %q", a.Build)
	}
	res, _ := get(t, a.Handler, "/assets/weapons/weapons.json")
	if res.StatusCode != 200 || res.Header.Get("ETag") != "" {
		t.Fatalf("dev static: %d etag %q", res.StatusCode, res.Header.Get("ETag"))
	}
}

func TestNewFailsWithoutAssets(t *testing.T) {
	dir := t.TempDir()
	write := func(p, s string) {
		t.Helper()
		_ = os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	steps := []struct{ path, content string }{
		{"index.html", "x"},                   // no weapons.json yet
		{"assets/weapons/weapons.json", "{"},  // invalid
		{"assets/weapons/weapons.json", `{}`}, // valid; no player.json yet
		{"assets/models/player.json", "nope"}, // invalid
	}
	for _, s := range steps {
		write(s.path, s.content)
		if _, err := New(Options{Dev: dir}); err == nil {
			t.Fatalf("after writing %s: New succeeded", s.path)
		}
	}
	if _, err := New(Options{Dev: filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("missing dev dir accepted")
	}
}

func TestServeIndexWithoutIndex(t *testing.T) {
	w := httptest.NewRecorder()
	serveIndex(w, httptest.NewRequest("GET", "/", nil), fstest.MapFS{}, "b", false)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code %d", w.Code)
	}
}

func TestHashAssets(t *testing.T) {
	fs1 := fstest.MapFS{"a.js": {Data: []byte("1")}, "d/b.css": {Data: []byte("2")}}
	fs2 := fstest.MapFS{"a.js": {Data: []byte("1")}, "d/b.css": {Data: []byte("3")}}
	e1, b1, err := hashAssets(fs1)
	if err != nil {
		t.Fatal(err)
	}
	e2, b2, _ := hashAssets(fs2)
	if len(e1) != 2 || e1["/a.js"] != e2["/a.js"] || e1["/d/b.css"] == e2["/d/b.css"] || b1 == b2 {
		t.Fatalf("etags %v %v builds %s %s", e1, e2, b1, b2)
	}
}

// -friendlyfire reaches the game server: a client sees mp_friendlyfire 1.
func TestFriendlyFireOption(t *testing.T) {
	for _, ff := range []bool{false, true} {
		a := newApp(t, Options{FriendlyFire: ff})
		hs := httptest.NewServer(a.Handler)
		c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(hs.URL, "http")+"/ws", nil)
		if err != nil {
			hs.Close()
			t.Fatal(err)
		}
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		var got string
		for got == "" {
			var m struct {
				T     string
				Cvars map[string]string
			}
			if err := c.ReadJSON(&m); err != nil {
				t.Fatal(err)
			}
			if m.T == "cvars" {
				got = m.Cvars["mp_friendlyfire"]
			}
		}
		c.Close()
		hs.Close()
		if want := map[bool]string{false: "0", true: "1"}[ff]; got != want {
			t.Fatalf("FriendlyFire=%v: mp_friendlyfire %q", ff, got)
		}
	}
}
