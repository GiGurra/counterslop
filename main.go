// Command counterslop serves the browser client and runs the game server.
package main

import (
	"encoding/json"
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/gigurra/counterslop/internal/game"
	"github.com/gigurra/counterslop/web"
)

// version is set at build time: go build -ldflags "-X main.version=<sha>".
var version = "dev"

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dev := flag.String("dev", "", "serve the client from this directory instead of the embedded copy (live edits)")
	flag.Parse()

	srv := game.NewServer()
	go srv.Run(make(chan struct{}))

	var static http.Handler = http.FileServerFS(web.FS)
	if *dev != "" {
		static = http.FileServer(http.Dir(*dev))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.HandleWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(version + "\n")) })
	mux.HandleFunc("/map.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(srv.Map)
	})
	mux.Handle("/", noCache(static))

	slog.Info("counterslop listening", "addr", *addr, "version", version, "map", srv.Map.Name)
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
