// Command counterslop serves the browser client and runs the game server.
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/GiGurra/boa/pkg/boa"
	"github.com/gigurra/counterslop/internal/app"
	"github.com/spf13/cobra"
)

// version is set at build time: go build -ldflags "-X main.version=<sha>".
var version = "dev"

// Params is the command line, declared for boa (cobra/pflag underneath): long flags
// take two dashes (--addr, --bots). There are no one-letter flags, so an old
// single-dash call (-addr) fails as an unknown shorthand instead of misparsing.
// A flag on the command line beats its env var.
type Params struct {
	Addr              string `descr:"listen address" default:":8080" env:"COUNTERSLOP_ADDR"`
	Friendlyfire      bool   `descr:"mp_friendlyfire: shots hit teammates" optional:"true" env:"COUNTERSLOP_FRIENDLYFIRE"`
	Dev               string `descr:"serve the client from this directory instead of the embedded copy (live edits)" optional:"true" env:"COUNTERSLOP_DEV"`
	Test              bool   `descr:"enable test-harness hooks (window.__cs, /debug/state, no speed checks); never in prod" optional:"true"`
	NetCodec          string `descr:"snapshot wire codec: json or delta (docs/replication.md)" default:"json" env:"COUNTERSLOP_NET_CODEC"`
	Bots              int    `descr:"target-practice bots to start with (admins can add/kick more)" optional:"true" env:"COUNTERSLOP_BOTS"`
	AdminPasswordFile string `descr:"file holding the admin password (else $COUNTERSLOP_ADMIN_PASSWORD; neither = everyone is admin)" optional:"true" env:"COUNTERSLOP_ADMIN_PASSWORD_FILE"`
	// The password itself only comes from the environment (never a flag: it would show in ps).
	AdminPasswordEnv string `descr:"admin password" boa:"noflag" env:"COUNTERSLOP_ADMIN_PASSWORD" optional:"true"`
}

// parseParams parses the command line (env vars from the process environment).
// ran is false when it only printed help.
func parseParams(args []string) (p Params, ran bool, err error) {
	err = boa.CmdT[Params]{
		Use:         "counterslop",
		Short:       "counterslop: the browser parody shooter's game server and client",
		Args:        cobra.NoArgs,
		ParamEnrich: boa.ParamEnricherCombine(boa.ParamEnricherName, boa.ParamEnricherBool), // no auto one-letter flags
		RunFunc:     func(params *Params, _ *cobra.Command, _ []string) { p, ran = *params, true },
	}.RunArgsE(args)
	return p, ran, err
}

// usageError explains a bad command line; an old single-dash long flag gets a hint.
func usageError(err error) string {
	msg := fmt.Sprintf("counterslop: %v (see --help)", err)
	if strings.Contains(err.Error(), "shorthand") {
		msg += "\nlong flags take two dashes: --addr, --bots, --dev, ..."
	}
	return msg
}

// adminPassword: the file flag wins, then $COUNTERSLOP_ADMIN_PASSWORD; "" = everyone is admin.
func adminPassword(p Params) (string, error) {
	if p.AdminPasswordFile == "" {
		return p.AdminPasswordEnv, nil
	}
	b, err := os.ReadFile(p.AdminPasswordFile)
	if err != nil {
		return "", fmt.Errorf("reading admin password file: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

func main() {
	p, ran, err := parseParams(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, usageError(err))
		os.Exit(2)
	}
	if !ran {
		return // --help
	}
	adminPw, err := adminPassword(p)
	if err != nil {
		slog.Error("starting", "err", err)
		os.Exit(1)
	}
	a, err := app.New(app.Options{Version: version, Dev: p.Dev, Test: p.Test, Bots: p.Bots, AdminPassword: adminPw, FriendlyFire: p.Friendlyfire, NetCodec: p.NetCodec})
	if err != nil {
		slog.Error("starting", "err", err)
		os.Exit(1)
	}
	go a.Server.Run(make(chan struct{}))

	slog.Info("counterslop listening", "addr", p.Addr, "version", version, "build", a.Build, "map", a.Server.Map.Name, "test", p.Test, "codec", p.NetCodec, "admin", map[bool]string{true: "open (no password: everyone is admin)", false: "password"}[adminPw == ""])
	if err := http.ListenAndServe(p.Addr, a.Handler); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
