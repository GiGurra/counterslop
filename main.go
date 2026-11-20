// Command counterslop serves the browser client and runs the game server.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/gigurra/counterslop/internal/app"
)

// version is set at build time: go build -ldflags "-X main.version=<sha>".
var version = "dev"

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dev := flag.String("dev", "", "serve the client from this directory instead of the embedded copy (live edits)")
	test := flag.Bool("test", false, "enable test-harness hooks (window.__cs, /debug/state, no speed checks); never in prod")
	bots := flag.Int("bots", -1, "target-practice bots: -1 disabled (prod), N >= 0 allows bot_add/bot_kick and starts N bots")
	flag.Parse()

	a, err := app.New(app.Options{Version: version, Dev: *dev, Test: *test, Bots: *bots})
	if err != nil {
		slog.Error("starting", "err", err)
		os.Exit(1)
	}
	go a.Server.Run(make(chan struct{}))

	slog.Info("counterslop listening", "addr", *addr, "version", version, "build", a.Build, "map", a.Server.Map.Name, "test", *test)
	if err := http.ListenAndServe(*addr, a.Handler); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
