package game

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// stack joins n humans to the Vibe Coders and reports how many got in.
func stack(t *testing.T, s *Server, n int) int {
	t.Helper()
	in := 0
	for range n {
		if _, m := joinTeam(t, s, content.Vibe); m["t"] == "spawn" {
			in++
		}
	}
	return in
}

func TestLimitTeamsDefaultIsOneOver(t *testing.T) {
	s := capServer(t, 16)
	if s.cvars["mp_limitteams"] != "1" {
		t.Fatalf("default mp_limitteams %q", s.cvars["mp_limitteams"])
	}
	// As before mp_limitteams: 1 vs 0 is fine, 2 vs 0 is refused.
	if got := stack(t, s, 3); got != 1 {
		t.Fatalf("%d joined one empty side's opponent, want 1", got)
	}
}

func TestLimitTeamsN(t *testing.T) {
	for _, c := range []struct {
		v    string
		want int
	}{{"1", 1}, {"2", 2}, {"3", 3}, {"2.7", 2}, {"0", 8}, {"-1", 8}} {
		s := capServer(t, 16)
		s.cvars["mp_limitteams"] = c.v
		if got := stack(t, s, 8); got != c.want {
			t.Errorf("mp_limitteams %s: %d joined one side against 0, want %d", c.v, got, c.want)
		}
	}
}

func TestLimitTeamsZeroStillHonoursTheMapCap(t *testing.T) {
	s := capServer(t, 2)
	s.cvars["mp_limitteams"] = "0"
	if got := stack(t, s, 4); got != 2 {
		t.Fatalf("%d joined a 2-per-team side", got)
	}
}

func TestLimitTeamsAutoPicksTheSmallerSide(t *testing.T) {
	for _, v := range []string{"0", "1", "3"} {
		s := capServer(t, 16)
		s.cvars["mp_limitteams"] = v
		joinTeam(t, s, content.Vibe)
		joinTeam(t, s, content.Artisan)
		joinTeam(t, s, content.Vibe)
		if p, m := joinTeam(t, s, "auto"); m["t"] != "spawn" || p.Team != content.Artisan {
			t.Fatalf("mp_limitteams %s: auto put the 4th player on %s (%v)", v, p.Team, m)
		}
	}
}

func TestLimitTeamsSwitchingSides(t *testing.T) {
	// 2 vs 2; one artisan switching makes it 3 vs 1: a difference of 2.
	for _, c := range []struct {
		v  string
		ok bool
	}{{"1", false}, {"2", true}, {"0", true}} {
		s := capServer(t, 16)
		s.cvars["mp_limitteams"] = c.v
		joinTeam(t, s, content.Vibe)
		a, _ := joinTeam(t, s, content.Artisan)
		joinTeam(t, s, content.Vibe)
		joinTeam(t, s, content.Artisan)
		s.handle(a, inbound{T: "team", Team: content.Vibe})
		if got := a.Team == content.Vibe; got != c.ok {
			t.Errorf("mp_limitteams %s: switch to 3 vs 1 allowed=%v, want %v", c.v, got, c.ok)
		}
	}
}

// adminFlag is the admin flag of p's latest cvars message, and whether it
// carried the cvar help.
func adminFlag(t *testing.T, p *Player) (admin, help bool) {
	t.Helper()
	c := ofType(p, "cvars")
	if len(c) == 0 {
		t.Fatal("no cvars message")
	}
	m := c[len(c)-1]
	_, help = m["help"].(map[string]any)
	return m["admin"] == true, help
}

func TestCvarsTellEachPlayerWhetherTheyAreAdmin(t *testing.T) {
	s := authServer(t)
	s.AdminPassword = "pw"
	p, o := s.join("1.1.1.1"), s.join("2.2.2.2")
	s.motd(p)
	if a, h := adminFlag(t, p); a || h {
		t.Fatalf("welcome: admin %v help %v before login", a, h)
	}
	s.handle(p, inbound{T: "admin_login", Pw: "pw"})
	if a, h := adminFlag(t, p); !a || !h {
		t.Fatalf("after login: admin %v help %v", a, h)
	}
	// A change goes to everyone, each with their own flag.
	msgs(o)
	s.handle(p, inbound{T: "rcon", Cmd: "mp_limitteams", Args: []string{"0"}})
	if a, h := adminFlag(t, o); a || h {
		t.Fatalf("non-admin got admin %v help %v", a, h)
	}
	if a, _ := adminFlag(t, p); !a {
		t.Fatal("admin lost the flag on a change")
	}
	s.handle(p, inbound{T: "admin_logout"})
	if a, h := adminFlag(t, p); a || h {
		t.Fatalf("after logout: admin %v help %v", a, h)
	}
	// Open server (no password): everyone is admin.
	s.SetAdminPassword("")
	if a, h := adminFlag(t, o); !a || !h {
		t.Fatalf("open server: admin %v help %v", a, h)
	}
}

// The Server settings screen (web/ui/serverrules.js) lists server cvars by
// name: each must be one the server has, with help for the screen's hint.
func TestServerSettingsScreenCvarsExist(t *testing.T) {
	b, err := os.ReadFile("../../web/ui/serverrules.js")
	if err != nil {
		t.Fatal(err)
	}
	names := regexp.MustCompile(`cvar: "([a-z_0-9]+)"`).FindAllStringSubmatch(string(b), -1)
	if len(names) < 10 {
		t.Fatalf("found %d cvars in serverrules.js", len(names))
	}
	for _, m := range names {
		if _, ok := defaultCvars[m[1]]; !ok {
			t.Errorf("%s: not a server cvar", m[1])
		}
		if cvarHelp[m[1]][1] == "" {
			t.Errorf("%s: no cvarHelp", m[1])
		}
	}
}

// The shared vectors (testdata/teamlimit_cases.json): the server's teamFull
// answers every row as the team menu's teamOpen (web/assets/teams.test.mjs).
func TestTeamLimitVectors(t *testing.T) {
	var f struct {
		Cases []struct {
			Vibe, Artisan, Limit int
			Team                 string
			Open                 bool
		}
	}
	b, err := os.ReadFile("testdata/teamlimit_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range f.Cases {
		s := capServer(t, 16)
		s.cvars["mp_limitteams"] = "0"
		for range c.Vibe {
			joinTeam(t, s, content.Vibe)
		}
		for range c.Artisan {
			joinTeam(t, s, content.Artisan)
		}
		s.cvars["mp_limitteams"] = strconv.Itoa(c.Limit)
		p := s.join("127.0.0.1")
		if got := !s.teamFull(c.Team, p); got != c.Open {
			t.Errorf("%d vs %d, join %s, limit %d: open %v, want %v", c.Vibe, c.Artisan, c.Team, c.Limit, got, c.Open)
		}
	}
}

// The join limit is state: every snapshot's round carries it (lim; 0 = none,
// a negative cvar too), so the team menu and the server agree on what is open.
func TestTeamLimitInSnapshotRound(t *testing.T) {
	s := capServer(t, 16)
	for _, c := range []struct {
		v    string
		want int
	}{{"1", 1}, {"0", 0}, {"3", 3}, {"2.7", 2}, {"-1", 0}} {
		s.cvars["mp_limitteams"] = c.v
		var m map[string]any
		if err := json.Unmarshal(s.roundJSON(s.clock()), &m); err != nil {
			t.Fatal(err)
		}
		if got, ok := m["lim"].(float64); !ok || int(got) != c.want {
			t.Errorf("mp_limitteams %s: round lim %v, want %d", c.v, m["lim"], c.want)
		}
	}
}

// A refusal over the limit says the limit (the client's feed line names it).
func TestTeamFullSaysTheLimit(t *testing.T) {
	s := capServer(t, 16)
	s.cvars["mp_limitteams"] = "1"
	joinTeam(t, s, content.Vibe)
	if _, m := joinTeam(t, s, content.Vibe); m["t"] != "teamfull" || m["limit"] != 1.0 || m["cap"] != nil {
		t.Fatalf("refusal %v, want teamfull with limit 1", m)
	}
}
