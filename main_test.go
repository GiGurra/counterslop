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
	if got, want := mustParse(t), (Params{Addr: ":8080", NetCodec: "json", ReplayDir: "replays", ReplayMaxMB: 2048}); got != want {
		t.Fatalf("defaults: got %+v, want %+v", got, want)
	}
}

func TestParamsEachFlag(t *testing.T) {
	clearEnv(t)
	got := mustParse(t, "--addr", "127.0.0.1:9", "--friendlyfire", "--dev", "web", "--test", "--bots", "3", "--admin-password-file", "/pw", "--net-codec", "json", "--replay-dir", "/r", "--replay-max-mb", "5")
	want := Params{Addr: "127.0.0.1:9", Friendlyfire: true, Dev: "web", Test: true, Bots: 3, AdminPasswordFile: "/pw", NetCodec: "json", ReplayDir: "/r", ReplayMaxMB: 5}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if p := mustParse(t, "--replay-dir="); p.ReplayDir != "" {
		t.Fatalf("--replay-dir= (recording off): %+v", p)
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
