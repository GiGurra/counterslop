// Command counterslop serves the browser client and runs the game server.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gigurra/counterslop/internal/game"
	"github.com/gigurra/counterslop/web"
)

// version is set at build time: go build -ldflags "-X main.version=<sha>".
var version = "dev"

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dev := flag.String("dev", "", "serve the client from this directory instead of the embedded copy (live edits)")
	flag.Parse()

	var assets fs.FS = web.FS
	if *dev != "" {
		assets = os.DirFS(*dev)
	}
	// build identifies the client assets: clients reload when the server they
	// (re)connect to has a different one. Hashing content (not the git SHA)
	// means server-only deploys don't force a reload.
	etags, build, err := hashAssets(assets)
	if err != nil {
		slog.Error("hashing client assets", "err", err)
		os.Exit(1)
	}
	if *dev != "" {
		build = "dev-" + strconv.FormatInt(time.Now().Unix(), 36) // edits on disk don't change it; restarts do
		etags = nil                                               // FileServer's Last-Modified handles revalidation
	}

	srv := game.NewServer()
	srv.Version, srv.Build = version, build
	go srv.Run(make(chan struct{}))

	static := http.FileServerFS(assets)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.HandleWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(version + "\n")) })
	mux.HandleFunc("/map.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(srv.Map)
	})
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) { serveIndex(w, r, assets, build) })
	mux.HandleFunc("/index.html", func(w http.ResponseWriter, r *http.Request) { serveIndex(w, r, assets, build) })
	mux.Handle("/", noCache(withETags(static, etags)))

	slog.Info("counterslop listening", "addr", *addr, "version", version, "build", build, "map", srv.Map.Name)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
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

// serveIndex stamps the asset build into the page so the client knows exactly
// which build it is running, even if a deploy lands mid-load.
func serveIndex(w http.ResponseWriter, _ *http.Request, fsys fs.FS, build string) {
	b, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "missing index.html", http.StatusInternalServerError)
		return
	}
	b = bytes.Replace(b, []byte("__BUILD__"), []byte(build), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}
