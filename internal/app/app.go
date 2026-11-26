// Package app wires the game server and the client assets into one HTTP
// handler. Used by the counterslop binary and by the test harness (cmd/sim).
package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gigurra/counterslop/internal/content/mapcheck"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/game"
	"github.com/gigurra/counterslop/internal/mcp"
	"github.com/gigurra/counterslop/web"
)

type Options struct {
	Version string // git SHA, shown at /version
	Dev     string // serve the client from this directory instead of the embedded copy
	// Test enables the harness hooks: the client's window.__cs test API (with
	// ?test=1), /debug/state, and no movement speed checks (so scenarios can
	// teleport players). Never set in prod.
	Test bool
	// Bots: -1 disables target-practice bots entirely (prod); N >= 0 allows the
	// bot console commands and starts N bots.
	Bots int
	// AdminPassword unlocks admin commands; empty = everyone is admin.
	AdminPassword string
	// NetCodec: how snapshots go on the wire (game.NetCodecs; "" = json).
	NetCodec string
	// FriendlyFire starts the server with mp_friendlyfire 1 (admins can change it).
	FriendlyFire bool
	// ReplayDir: record every round as a replay there ("" = off); ReplayMaxMB caps
	// what the replays take on disk (0 = the default).
	ReplayDir   string
	ReplayMaxMB int
	// MCP: serve the server MCP (docs/mcp.md) behind this auth (App.MCP);
	// nil = no MCP. MCPDebug adds the debugger tools (always with Test).
	MCP      *mcp.Auth
	MCPDebug bool
	// MapsDir: load custom maps (<name>.json, docs/custom-maps.md) from here,
	// and again when its files change (WatchMaps). "" = built-in maps only.
	MapsDir string
}

type App struct {
	Handler http.Handler
	Server  *game.Server
	Build   string // the client build at startup (what /b/<build>/ serves as current)
	// MCP is the server MCP's handler (its own listener: main.go --mcp-addr); nil = none.
	MCP http.Handler

	current atomic.Pointer[string] // the build served now; only Redeploy (test mode) changes it

	mapsDir  string
	mapsMu   sync.Mutex // one (re)load at a time
	mapsSeen string     // maps.Fingerprint of the last load

	catMu  sync.Mutex
	cat    []byte // /maps.json, built for catGen
	catGen int
}

// BuildPrefix is where the page loads its client from: /b/<build>/... serves
// the same files as /..., but a new build means new URLs, so no browser or CDN
// cache (whatever its TTL) can hand a new page old modules. Every module import
// below main.js and the import map is relative, so the whole client follows.
func BuildPrefix(build string) string { return "/b/" + build + "/" }

func New(o Options) (*App, error) {
	var assets fs.FS = web.FS
	if o.Dev != "" {
		assets = os.DirFS(o.Dev)
	}
	// build identifies the client assets: clients reload when the server they
	// (re)connect to has a different one. Hashing content (not the git SHA)
	// means server-only deploys don't force a reload.
	etags, build, err := hashAssets(assets)
	if err != nil {
		return nil, err
	}
	if o.Dev != "" {
		build = "dev-" + strconv.FormatInt(time.Now().Unix(), 36) // edits on disk don't change it; restarts do
		etags = nil                                               // FileServer's Last-Modified handles revalidation
	}

	srv := game.NewServer()
	wj, err := fs.ReadFile(assets, "assets/weapons/weapons.json")
	if err != nil {
		return nil, err
	}
	if srv.Weapons, err = game.LoadWeapons(wj); err != nil {
		return nil, err
	}
	ej, err := fs.ReadFile(assets, "assets/economy.json")
	if err != nil {
		return nil, err
	}
	if srv.Economy, err = game.LoadEconomy(ej); err != nil {
		return nil, err
	}
	pj, err := fs.ReadFile(assets, "assets/models/player.json")
	if err != nil {
		return nil, err
	}
	if err := game.LoadModel(pj); err != nil {
		return nil, err
	}
	srv.Version, srv.Build, srv.Test, srv.Bots, srv.AdminPassword = o.Version, build, o.Test, o.Bots, o.AdminPassword
	if err := srv.SetNetCodec(o.NetCodec); err != nil {
		return nil, err
	}
	if err := srv.SetReplays(o.ReplayDir, o.ReplayMaxMB); err != nil {
		return nil, err
	}
	var mcpHandler http.Handler
	if o.MCP != nil {
		mcpHandler = o.MCP.Handler(srv.MCP(o.MCPDebug || o.Test))
	}
	if o.FriendlyFire {
		srv.SetCvar("mp_friendlyfire", "1")
	}

	a := &App{Server: srv, Build: build, MCP: mcpHandler, mapsDir: o.MapsDir}
	srv.MapsDir = o.MapsDir // map_save writes there; then the custom maps load again
	srv.OnMapSaved = func() {
		if _, err := a.MapsChanged(); err != nil {
			slog.Warn("reloading custom maps after map_save", "err", err)
		}
	}
	if o.MapsDir != "" {
		if _, _, err := a.ReloadMaps(); err != nil {
			return nil, err
		}
	}
	a.current.Store(&build)
	static := withETags(http.FileServerFS(assets), etags)
	index := func(w http.ResponseWriter, r *http.Request) { serveIndex(w, r, assets, *a.current.Load(), o.Test) }

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.HandleWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(o.Version + "\n")) })
	// The current map, or ?name= any map or lab (the asset gallery's lighting lab).
	mux.HandleFunc("/map.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if n := r.URL.Query().Get("name"); n != "" {
			build := maps.Find(n)
			if build == nil {
				http.Error(w, "unknown map", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(build())
			return
		}
		_ = json.NewEncoder(w).Encode(srv.Map)
	})
	mux.HandleFunc("GET /replays", srv.HandleReplaysPage)
	mux.HandleFunc("GET /replays.json", srv.HandleReplaysJSON)
	mux.HandleFunc("GET /replays/{match}/{file}", srv.HandleReplayFile)
	mux.HandleFunc("/maps.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(a.catalog())
	})
	if o.Test {
		mux.HandleFunc("/debug/state", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(srv.DebugState())
		})
	}
	mux.HandleFunc("/{$}", index)
	mux.HandleFunc("/index.html", index)
	mux.HandleFunc("/dev/assets", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dev/assets.html", http.StatusFound)
	})
	mux.HandleFunc("/b/{build}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		path := r.PathValue("path")
		if path == "" || path == "index.html" {
			index(w, r)
			return
		}
		switch {
		case o.Dev != "":
			w.Header().Set("Cache-Control", "no-cache") // live edits keep the URL
		case r.PathValue("build") == *a.current.Load():
			// Content-hashed build: these bytes never change under this URL.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			// A page from another build (a deploy landed mid-load): serve the
			// current file so it boots and sees the new build on connect (and
			// reloads), but let nobody cache it under the old build's URL.
			w.Header().Set("Cache-Control", "no-store")
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path, r2.URL.RawPath = "/"+path, ""
		static.ServeHTTP(w, r2)
	})
	mux.Handle("/", noCache(static))

	a.Handler = mux
	return a, nil
}

// Redeploy makes the app serve as a new client build, as a deploy would: new
// pages get the new build's URLs and every connected client is dropped and
// meets the new build when it reconnects. Test harness only (cmd/sim).
func (a *App) Redeploy(build string) {
	a.current.Store(&build)
	a.Server.Redeploy(build)
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

// hashAssets returns a strong ETag per file and one hash over all of them.
func hashAssets(fsys fs.FS) (map[string]string, string, error) {
	etags := map[string]string{}
	all := sha256.New()
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		etags["/"+p] = `"` + hex.EncodeToString(sum[:8]) + `"`
		all.Write([]byte(p))
		all.Write(sum[:])
		return nil
	})
	return etags, hex.EncodeToString(all.Sum(nil)[:6]), err
}

// withETags sets the file's ETag before the FileServer runs, which makes it
// answer If-None-Match with 304 (embed.FS has no modtimes to go by).
func withETags(h http.Handler, etags map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tag, ok := etags[r.URL.Path]; ok {
			w.Header().Set("ETag", tag)
		}
		h.ServeHTTP(w, r)
	})
}

// serveIndex stamps the asset build (and test mode) into the page so the client
// knows exactly which build it is running, even if a deploy lands mid-load.
func serveIndex(w http.ResponseWriter, _ *http.Request, fsys fs.FS, build string, test bool) {
	b, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "missing index.html", http.StatusInternalServerError)
		return
	}
	b = bytes.Replace(b, []byte("__BUILD__"), []byte(build), 1)
	pre := strings.TrimSuffix(BuildPrefix(build), "/")
	b = []byte(strings.NewReplacer(
		`src="/`, `src="`+pre+`/`, // the entry module
		`href="/`, `href="`+pre+`/`, // the stylesheet
		`": "/`, `": "`+pre+`/`, // import map entries
	).Replace(string(b)))
	if test {
		b = bytes.Replace(b, []byte(`content="__TEST__"`), []byte(`content="1"`), 1)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

// catalog is /maps.json: every playable map, built again when the custom
// maps change. (Plain data: it always marshals.)
func (a *App) catalog() []byte {
	a.catMu.Lock()
	defer a.catMu.Unlock()
	if g := maps.Gen(); a.cat == nil || a.catGen != g {
		a.cat, _ = json.Marshal(game.MapCatalog())
		a.catGen = g
	}
	return a.cat
}

// ReloadMaps reads the custom maps from the maps dir again: the ones that
// pass the built-in maps' checks replace the custom set, each other file is
// refused with its reason (logged), and the server reloads the custom map it
// is playing if its file changed.
func (a *App) ReloadMaps() (loaded []string, refused map[string]string, err error) {
	a.mapsMu.Lock()
	defer a.mapsMu.Unlock()
	return a.reloadMaps()
}

func (a *App) reloadMaps() (loaded []string, refused map[string]string, err error) {
	seen := maps.Fingerprint(a.mapsDir)
	weapon := func(id string) bool { return a.Server.Weapons != nil && a.Server.Weapons.Get(id) != nil }
	ok, refused, err := mapcheck.LoadDir(a.mapsDir, weapon, maps.Builtin)
	if err != nil {
		return nil, nil, err
	}
	a.mapsSeen = seen
	maps.SetCustom(ok)
	for n := range ok {
		loaded = append(loaded, n)
	}
	slices.Sort(loaded)
	for f, why := range refused {
		slog.Warn("custom map refused", "file", f, "why", why)
	}
	slog.Info("custom maps", "dir", a.mapsDir, "loaded", loaded, "refused", len(refused))
	a.Server.MapsReloaded(refused)
	return loaded, refused, nil
}

// MapsChanged: the maps dir's files changed since the last load; reload then.
// Returns whether it reloaded. WatchMaps calls it on a timer; tests call it.
func (a *App) MapsChanged() (bool, error) {
	if a.mapsDir == "" {
		return false, nil
	}
	a.mapsMu.Lock()
	defer a.mapsMu.Unlock()
	if maps.Fingerprint(a.mapsDir) == a.mapsSeen {
		return false, nil
	}
	_, _, err := a.reloadMaps()
	return true, err
}

// WatchMaps hot-reloads the custom maps: on every tick, MapsChanged (main.go
// runs it on a wall-clock ticker; nothing in it reads a clock).
func (a *App) WatchMaps(tick <-chan time.Time) {
	for range tick {
		if _, err := a.MapsChanged(); err != nil {
			slog.Warn("reloading custom maps", "err", err)
		}
	}
}
