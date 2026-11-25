package game

import (
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
