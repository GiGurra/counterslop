// Command counterslop serves the browser client and runs the game server.
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/GiGurra/boa/pkg/boa"
	"github.com/gigurra/counterslop/internal/app"
	"github.com/gigurra/counterslop/internal/mcp"
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
	NetCodec          string `descr:"snapshot wire codec: delta (binary, the default) or json (the fallback; docs/replication.md)" default:"delta" env:"COUNTERSLOP_NET_CODEC"`
	Replays           bool   `descr:"record every round as a server replay under --replay-dir (off by default; tv_autorecord 0 pauses it; docs/replays.md)" optional:"true" env:"COUNTERSLOP_REPLAYS"`
	ReplayDir         string `descr:"where --replays records (one directory per match)" default:"replays" env:"COUNTERSLOP_REPLAY_DIR"`
	ReplayMaxMB       int    `descr:"the replays may take this many MB on disk; the oldest rounds are deleted beyond it" default:"2048" env:"COUNTERSLOP_REPLAY_MAX_MB"`
	MapsDir           string `descr:"load custom maps (<name>.json, docs/custom-maps.md) from this directory, again whenever its files change; a broken map is refused with the reason (logged)" optional:"true" env:"COUNTERSLOP_MAPS_DIR"`
	Bots              int    `descr:"target-practice bots to start with (admins can add/kick more)" optional:"true" env:"COUNTERSLOP_BOTS"`
	AdminPasswordFile string `descr:"file holding the admin password (else $COUNTERSLOP_ADMIN_PASSWORD; neither = everyone is admin)" optional:"true" env:"COUNTERSLOP_ADMIN_PASSWORD_FILE"`
	// The password itself only comes from the environment (never a flag: it would show in ps).
	AdminPasswordEnv string `descr:"admin password" boa:"noflag" env:"COUNTERSLOP_ADMIN_PASSWORD" optional:"true"`
	// The server MCP (docs/mcp.md): off unless --mcp-addr is given. Its secrets
	// come only from the environment, like the admin password.
	MCPAddr         string `descr:"serve the admin MCP here (a port alone binds 127.0.0.1; name a host to expose it, which needs auth); empty = no MCP" optional:"true" env:"COUNTERSLOP_MCP_ADDR"`
	MCPDebug        bool   `descr:"add the MCP's debugger tools (pause/step, set any state, puppets) without --test; never in prod" optional:"true" env:"COUNTERSLOP_MCP_DEBUG"`
	MCPClientID     string `descr:"OAuth client id for the MCP's client-credentials token endpoint (secret: $COUNTERSLOP_MCP_CLIENT_SECRET)" optional:"true" env:"COUNTERSLOP_MCP_CLIENT_ID"`
	MCPToken        string `descr:"MCP bearer token" boa:"noflag" env:"COUNTERSLOP_MCP_TOKEN" optional:"true"`
	MCPPathSecret   string `descr:"MCP secret path segment (/mcp/<secret>)" boa:"noflag" env:"COUNTERSLOP_MCP_PATH_SECRET" optional:"true"`
	MCPClientSecret string `descr:"MCP OAuth client secret" boa:"noflag" env:"COUNTERSLOP_MCP_CLIENT_SECRET" optional:"true"`
}

// mcpAuth is the MCP's auth from the params, checked for its listen address
// (nil: no MCP).
func mcpAuth(p Params) (*mcp.Auth, string, error) {
	if p.MCPAddr == "" {
		return nil, "", nil
	}
	a := &mcp.Auth{Token: p.MCPToken, PathSecret: p.MCPPathSecret, ClientID: p.MCPClientID, ClientSecret: p.MCPClientSecret}
	addr := mcp.ListenAddr(p.MCPAddr)
	if err := a.Check(addr); err != nil {
		return nil, "", err
	}
	return a, addr, nil
}

// parseParams parses the command line (env vars from the process environment).
// ran is false when it only printed help.
func parseParams(args []string) (p Params, ran bool, err error) {
	err = boa.CmdT[Params]{
		Use:         "counterslop",
		Short:       "counterslop: the browser parody shooter's game server and client",
		Args:        cobra.NoArgs,
		SubCmds:     []*cobra.Command{mapCmd(os.Stdout)},
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
	mauth, maddr, err := mcpAuth(p)
	if err != nil {
		slog.Error("starting the MCP", "err", err)
		os.Exit(1)
	}
	a, err := app.New(app.Options{MCP: mauth, MCPDebug: p.MCPDebug, Version: version, Dev: p.Dev, Test: p.Test, Bots: p.Bots, AdminPassword: adminPw, FriendlyFire: p.Friendlyfire, NetCodec: p.NetCodec, ReplayDir: p.replayDir(), ReplayMaxMB: p.ReplayMaxMB, MapsDir: p.MapsDir})
	if err != nil {
		slog.Error("starting", "err", err)
		os.Exit(1)
	}
	go a.Server.Run(make(chan struct{}))
	if p.MapsDir != "" {
		go a.WatchMaps(time.NewTicker(2 * time.Second).C) // hot reload: a mapper saves, the map is back in 2 s
	}
	if a.MCP != nil {
		ways := []string{}
		for name, on := range map[string]bool{"token": mauth.Token != "", "path secret": mauth.PathSecret != "", "oauth client credentials": mauth.ClientID != ""} {
			if on {
				ways = append(ways, name)
			}
		}
		slices.Sort(ways)
		slog.Info("admin MCP listening", "addr", maddr, "loopback", mcp.Loopback(maddr), "auth", ways, "debug", p.MCPDebug || p.Test) // never the secrets
		go func() {
			if err := http.ListenAndServe(maddr, a.MCP); err != nil {
				slog.Error("MCP stopped", "err", err)
				os.Exit(1)
			}
		}()
	}

	slog.Info("counterslop listening", "addr", p.Addr, "version", version, "build", a.Build, "map", a.Server.Map.Name, "test", p.Test, "codec", p.NetCodec, "replays", a.Server.ReplayDir(), "admin", map[bool]string{true: "open (no password: everyone is admin)", false: "password"}[adminPw == ""])
	if err := http.ListenAndServe(p.Addr, a.Handler); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// replayDir is where the server records replays ("" = off): only with
// --replays, which is opt in.
func (p Params) replayDir() string {
	if !p.Replays {
		return ""
	}
	return p.ReplayDir
}
