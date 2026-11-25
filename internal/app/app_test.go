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

	"github.com/gigurra/counterslop/internal/mcp"
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

// The page loads its client from /b/<build>/: a new build is new URLs, so no
// browser or CDN cache can serve a new page old modules.
func TestBuildStampedURLs(t *testing.T) {
	a := newApp(t, Options{})
	pre := "/b/" + a.Build + "/"
	_, page := get(t, a.Handler, "/")
	for _, want := range []string{`src="` + pre + `game/main.js"`, `href="` + pre + `style.css"`, `"preact": "` + pre + `third_party/preact/preact.module.js"`, `"htm": "` + pre + `third_party/`} {
		if !strings.Contains(page, want) {
			t.Errorf("index lacks %s", want)
		}
	}
	for _, old := range []string{`"/game/`, `"/third_party/`, `"/style.css"`} {
		if strings.Contains(page, old) {
			t.Errorf("index still loads %s unprefixed", old)
		}
	}

	// Prefixed: the current files, cacheable forever; unprefixed still works.
	for _, f := range []string{"game/main.js", "style.css", "third_party/preact/preact.module.js", "assets/weapons/weapons.json"} {
		res, body := get(t, a.Handler, pre+f)
		plain, want := get(t, a.Handler, "/"+f)
		if res.StatusCode != 200 || body != want || res.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("%s%s: %d cache %q, same body %v", pre, f, res.StatusCode, res.Header.Get("Cache-Control"), body == want)
		}
		if plain.StatusCode != 200 || plain.Header.Get("Cache-Control") != "no-cache" || plain.Header.Get("ETag") == "" {
			t.Errorf("/%s: %d %v", f, plain.StatusCode, plain.Header)
		}
	}
	if res, _ := get(t, a.Handler, pre+"no/such.js"); res.StatusCode != 404 {
		t.Errorf("missing prefixed file: %d", res.StatusCode)
	}
	for _, p := range []string{pre, pre + "index.html"} { // the page itself is never cached
		res, body := get(t, a.Handler, p)
		if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" || !strings.Contains(body, `src="`+pre+`game/main.js"`) {
			t.Errorf("%s: %d %v", p, res.StatusCode, res.Header)
		}
	}

	// A deploy: new pages get the new build's URLs; a page of the old build
	// still boots (and then reloads), but nothing caches it under the old URL.
	a.Redeploy("next")
	if _, page := get(t, a.Handler, "/"); !strings.Contains(page, `src="/b/next/game/main.js"`) || !strings.Contains(page, `content="next"`) {
		t.Fatal("index after redeploy doesn't load the new build")
	}
	if res, _ := get(t, a.Handler, "/b/next/game/main.js"); res.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("new build: cache %q", res.Header.Get("Cache-Control"))
	}
	if res, _ := get(t, a.Handler, pre+"game/main.js"); res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("old build after deploy: %d cache %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
}

// Dev mode serves the prefixed URLs from disk too, revalidated, so live edits show.
func TestDevModeBuildPrefix(t *testing.T) {
	a := newApp(t, Options{Dev: "../../web"})
	res, _ := get(t, a.Handler, "/b/"+a.Build+"/game/main.js")
	if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("dev prefixed: %d %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
}

// A stale client meeting a new server: Redeploy drops the connection (no kick
// notice, so the client reconnects) and the reconnect's welcome names the new
// build, which is what makes the client reload (web/game/build.js).
func TestRedeployDropsClientsOntoNewBuild(t *testing.T) {
	a := newApp(t, Options{})
	stop := make(chan struct{})
	defer close(stop)
	go a.Server.Run(stop)
	hs := httptest.NewServer(a.Handler)
	defer hs.Close()
	url := "ws" + strings.TrimPrefix(hs.URL, "http") + "/ws"
	welcome := func(c *websocket.Conn) map[string]any {
		t.Helper()
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		var m map[string]any
		if err := c.ReadJSON(&m); err != nil || m["t"] != "welcome" {
			t.Fatalf("welcome: %v %v", err, m)
		}
		return m
	}
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if w := welcome(c); w["build"] != a.Build {
		t.Fatalf("build %v, want %s", w["build"], a.Build)
	}
	a.Redeploy("next")
	for {
		var m map[string]any
		if err := c.ReadJSON(&m); err != nil {
			break // dropped
		}
		if m["t"] == "kicked" {
			t.Fatal("a redeploy must not look like a kick (the client wouldn't reconnect)")
		}
	}
	c2, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	if w := welcome(c2); w["build"] != "next" {
		t.Fatalf("after redeploy: build %v", w["build"])
	}
}

// The MCP is its own handler (main.go serves it on --mcp-addr), never on the
// game's: no MCP option, no handler; with one, its tools, debug ones only
// with MCPDebug or Test.
func TestMCPHandler(t *testing.T) {
	if a := newApp(t, Options{Version: "v", Bots: -1}); a.MCP != nil {
		t.Fatal("an MCP without the option")
	}
	tools := func(o Options) string {
		o.MCP = &mcp.Auth{Token: "0123456789abcdef"}
		a := newApp(t, o)
		if res, _ := get(t, a.Handler, "/mcp"); res.StatusCode != 404 && res.StatusCode != 200 {
			t.Fatalf("/mcp on the game handler: %d", res.StatusCode)
		}
		r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		r.Header.Set("Authorization", "Bearer 0123456789abcdef")
		w := httptest.NewRecorder()
		a.MCP.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("tools/list: %d %s", w.Code, w.Body)
		}
		return w.Body.String()
	}
	if l := tools(Options{Version: "v", Bots: -1}); !strings.Contains(l, `"name":"rcon"`) || strings.Contains(l, `"name":"step"`) {
		t.Fatal("admin MCP: no rcon, or debug tools")
	}
	if l := tools(Options{Version: "v", Bots: -1, MCPDebug: true}); !strings.Contains(l, `"name":"step"`) {
		t.Fatal("--mcp-debug without the debug tools")
	}
	if l := tools(Options{Version: "v", Bots: 0, Test: true}); !strings.Contains(l, `"name":"puppet_add"`) {
		t.Fatal("--test without the debug tools")
	}
}
