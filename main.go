// Command counterslop serves the browser client and runs the game server.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gigurra/counterslop/internal/app"
)

// version is set at build time: go build -ldflags "-X main.version=<sha>".
var version = "dev"

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	ff := flag.Bool("friendlyfire", false, "mp_friendlyfire: shots hit teammates")
	dev := flag.String("dev", "", "serve the client from this directory instead of the embedded copy (live edits)")
	test := flag.Bool("test", false, "enable test-harness hooks (window.__cs, /debug/state, no speed checks); never in prod")
	bots := flag.Int("bots", 0, "target-practice bots to start with (admins can add/kick more)")
	pwFile := flag.String("admin-password-file", "", "file holding the admin password (else $COUNTERSLOP_ADMIN_PASSWORD; neither = everyone is admin)")
	flag.Parse()

	adminPw := os.Getenv("COUNTERSLOP_ADMIN_PASSWORD")
	if *pwFile != "" {
		b, err := os.ReadFile(*pwFile)
		if err != nil {
			slog.Error("reading admin password file", "err", err)
			os.Exit(1)
		}
		adminPw = strings.TrimSpace(string(b))
	}
	a, err := app.New(app.Options{Version: version, Dev: *dev, Test: *test, Bots: *bots, AdminPassword: adminPw, FriendlyFire: *ff})
	if err != nil {
		slog.Error("starting", "err", err)
		os.Exit(1)
	}
	go a.Server.Run(make(chan struct{}))

	slog.Info("counterslop listening", "addr", *addr, "version", version, "build", a.Build, "map", a.Server.Map.Name, "test", *test, "admin", map[bool]string{true: "open (no password: everyone is admin)", false: "password"}[adminPw == ""])
	if err := http.ListenAndServe(*addr, a.Handler); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
