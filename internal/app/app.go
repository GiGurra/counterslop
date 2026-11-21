// Package app wires the game server and the client assets into one HTTP
// handler. Used by the counterslop binary and by the test harness (cmd/sim).
package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/game"
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
	// FriendlyFire starts the server with mp_friendlyfire 1 (admins can change it).
	FriendlyFire bool
}

type App struct {
	Handler http.Handler
	Server  *game.Server
	Build   string
}

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
	if o.FriendlyFire {
		srv.SetCvar("mp_friendlyfire", "1")
	}

	static := http.FileServerFS(assets)
	index := func(w http.ResponseWriter, r *http.Request) { serveIndex(w, r, assets, build, o.Test) }

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
	catalog, err := json.Marshal(game.MapCatalog())
	if err != nil {
		return nil, err
	}
	mux.HandleFunc("/maps.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(catalog)
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
	mux.Handle("/", noCache(withETags(static, etags)))

	return &App{Handler: mux, Server: srv, Build: build}, nil
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
	if test {
		b = bytes.Replace(b, []byte(`content="__TEST__"`), []byte(`content="1"`), 1)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}
