package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearEnv unsets every COUNTERSLOP_* variable for the test (t.Setenv restores them).
func clearEnv(t *testing.T) {
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "COUNTERSLOP_") {
			t.Setenv(k, "")
			os.Unsetenv(k)
		}
	}
}

func mustParse(t *testing.T, args ...string) Params {
	t.Helper()
	p, ran, err := parseParams(args)
	if err != nil || !ran {
		t.Fatalf("parseParams(%q): ran=%v err=%v", args, ran, err)
	}
	return p
}

func TestParamsDefaults(t *testing.T) {
	clearEnv(t)
	if got, want := mustParse(t), (Params{Addr: ":8080", NetCodec: "delta", ReplayDir: "replays", ReplayMaxMB: 2048}); got != want {
		t.Fatalf("defaults: got %+v, want %+v", got, want)
	}
}

func TestParamsEachFlag(t *testing.T) {
	clearEnv(t)
	got := mustParse(t, "--addr", "127.0.0.1:9", "--friendlyfire", "--dev", "web", "--test", "--bots", "3", "--admin-password-file", "/pw", "--net-codec", "json", "--replays", "--replay-dir", "/r", "--replay-max-mb", "5", "--maps-dir", "/m")
	want := Params{Addr: "127.0.0.1:9", Friendlyfire: true, Dev: "web", Test: true, Bots: 3, AdminPasswordFile: "/pw", NetCodec: "json", Replays: true, ReplayDir: "/r", ReplayMaxMB: 5, MapsDir: "/m"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if p := mustParse(t, "--addr=:1", "--bots=2"); p.Addr != ":1" || p.Bots != 2 {
		t.Fatalf("--flag=value: %+v", p)
	}
}

func TestParamsEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("COUNTERSLOP_ADDR", ":7")
	t.Setenv("COUNTERSLOP_BOTS", "4")
	t.Setenv("COUNTERSLOP_NET_CODEC", "jsonish")
	t.Setenv("COUNTERSLOP_ADMIN_PASSWORD", "hunter2")
	p := mustParse(t)
	if p.Addr != ":7" || p.Bots != 4 || p.AdminPasswordEnv != "hunter2" || p.NetCodec != "jsonish" {
		t.Fatalf("env: %+v", p)
	}
	if p := mustParse(t, "--addr", ":8"); p.Addr != ":8" { // the flag beats the env var
		t.Fatalf("flag over env: %+v", p)
	}
}

// Bad command lines fail instead of starting a server: unknown flags, stray
// arguments, a password as a flag, and the old single-dash spelling, which
// pflag would otherwise read as one-letter flags (-addr = -a ddr).
func TestParamsErrors(t *testing.T) {
	clearEnv(t)
	for _, args := range [][]string{
		{"--nope"}, {"stray"}, {"--admin-password-env", "x"}, {"--admin-password", "x"},
		{"-addr", "127.0.0.1:9", "-bots", "1"}, {"-addr", "127.0.0.1:9"}, {"-a", ":1"}, {"--bots", "many"},
	} {
		p, ran, err := parseParams(args)
		if err == nil || ran {
			t.Errorf("%q: want an error, got ran=%v %+v", args, ran, p)
		}
	}
	_, _, err := parseParams([]string{"-addr", ":1"})
	if msg := usageError(err); !strings.Contains(msg, "two dashes: --addr") {
		t.Errorf("single-dash hint missing: %q", msg)
	}
}

func TestParamsHelp(t *testing.T) {
	clearEnv(t)
	if _, ran, err := parseParams([]string{"--help"}); err != nil || ran {
		t.Fatalf("--help: ran=%v err=%v", ran, err)
	}
}

// The admin password: the file wins, then $COUNTERSLOP_ADMIN_PASSWORD, else "" (everyone is admin).
func TestAdminPassword(t *testing.T) {
	clearEnv(t)
	file := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(file, []byte("  from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		env  string
		args []string
		want string
	}{
		{"neither", "", nil, ""},
		{"env", "from-env", nil, "from-env"},
		{"file", "", []string{"--admin-password-file", file}, "from-file"},
		{"file beats env", "from-env", []string{"--admin-password-file", file}, "from-file"},
	}
	for _, c := range cases {
		os.Unsetenv("COUNTERSLOP_ADMIN_PASSWORD")
		if c.env != "" {
			t.Setenv("COUNTERSLOP_ADMIN_PASSWORD", c.env)
		}
		got, err := adminPassword(mustParse(t, c.args...))
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	if _, err := adminPassword(mustParse(t, "--admin-password-file", filepath.Join(t.TempDir(), "missing"))); err == nil {
		t.Error("a missing password file must fail startup")
	}
}

// The MCP: off without --mcp-addr; its secrets only from the environment
// (never flags: ps shows them); no way in = refused, on localhost too; a
// port alone binds the loopback interface.
func TestParamsMCP(t *testing.T) {
	clearEnv(t)
	if a, _, err := mcpAuth(mustParse(t)); a != nil || err != nil {
		t.Fatalf("no --mcp-addr: %v %v", a, err)
	}
	if _, _, err := mcpAuth(mustParse(t, "--mcp-addr", "8091")); err == nil || !strings.Contains(err.Error(), "needs auth") {
		t.Fatalf("no auth: %v", err)
	}
	if _, _, err := mcpAuth(mustParse(t, "--mcp-addr", "0.0.0.0:8091")); err == nil {
		t.Fatal("exposed without auth")
	}
	for _, args := range [][]string{{"--mcp-token", "x"}, {"--mcp-path-secret", "x"}, {"--mcp-client-secret", "x"}} {
		if _, ran, err := parseParams(args); err == nil || ran {
			t.Errorf("%q: a secret as a flag must not parse", args)
		}
	}
	t.Setenv("COUNTERSLOP_MCP_TOKEN", "0123456789abcdef-token")
	t.Setenv("COUNTERSLOP_MCP_PATH_SECRET", "0123456789abcdef-path")
	t.Setenv("COUNTERSLOP_MCP_CLIENT_SECRET", "0123456789abcdef-client")
	p := mustParse(t, "--mcp-addr", ":8091", "--mcp-client-id", "agents", "--mcp-debug")
	a, addr, err := mcpAuth(p)
	if err != nil || addr != "127.0.0.1:8091" || a.Token != "0123456789abcdef-token" || a.PathSecret != "0123456789abcdef-path" ||
		a.ClientID != "agents" || a.ClientSecret != "0123456789abcdef-client" || !p.MCPDebug {
		t.Fatalf("mcp params: %+v %q %v", a, addr, err)
	}
}

// counterslop map export / map check: a built-in map exported under a new
// name checks out; under its own name (or broken) it is refused, non-zero.
func TestMapExportAndCheck(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	if _, ran, err := parseParams([]string{"map", "export", "he_merge_hell", "--as", "he_merge_heaven", "--out", dir}); err != nil || ran {
		t.Fatalf("export: ran %v err %v", ran, err)
	}
	if _, _, err := parseParams([]string{"map", "export", "de_slop", "--out", dir}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := checkMaps(&out, []string{filepath.Join(dir, "he_merge_heaven.json")}); err != nil || !strings.Contains(out.String(), "ok (he_merge_heaven") {
		t.Fatalf("check: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := checkMaps(&out, []string{filepath.Join(dir, "de_slop.json"), filepath.Join(dir, "nope.json")}); err == nil || !strings.Contains(out.String(), "built-in map's name") {
		t.Fatalf("check of a built-in's name: %v\n%s", err, out.String())
	}
	if _, _, err := parseParams([]string{"map", "export", "de_nope", "--out", dir}); err == nil || !strings.Contains(err.Error(), `no map "de_nope"`) {
		t.Fatalf("export of an unknown map: %v", err)
	}
	out.Reset()
	if err := exportMaps(&out, "-", "", []string{"lab_harbor"}); err != nil || !strings.Contains(out.String(), `"boats"`) {
		t.Fatalf("export to stdout: %v", err)
	}
}

// Server replays are opt in: off by default, --replays / COUNTERSLOP_REPLAYS=true
// records into --replay-dir (default "replays").
func TestReplaysOptIn(t *testing.T) {
	clearEnv(t)
	if d := mustParse(t, "--replay-dir", "/r").replayDir(); d != "" {
		t.Fatalf("default: recording into %q", d)
	}
	if d := mustParse(t, "--replays").replayDir(); d != "replays" {
		t.Fatalf("--replays: %q", d)
	}
	if d := mustParse(t, "--replays", "--replay-dir", "/r").replayDir(); d != "/r" {
		t.Fatalf("--replays --replay-dir /r: %q", d)
	}
	t.Setenv("COUNTERSLOP_REPLAYS", "true")
	t.Setenv("COUNTERSLOP_REPLAY_DIR", "/e")
	if d := mustParse(t).replayDir(); d != "/e" {
		t.Fatalf("COUNTERSLOP_REPLAYS: %q", d)
	}
}
