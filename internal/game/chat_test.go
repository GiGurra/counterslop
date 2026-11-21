package game

import (
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

func TestSanitizeChat(t *testing.T) {
	for in, want := range map[string]string{
		"  hello   world \n":     "hello world",
		"bell\x07\x1b[31mred":    "bell[31mred",
		strings.Repeat("a", 300): strings.Repeat("a", chatMax),
		"\t\n ":                  "",
	} {
		if got := sanitizeChat(in); got != want {
			t.Errorf("sanitizeChat(%q) = %q, want %q", in, got, want)
		}
	}
}

func chatServer(t *testing.T) (*Server, *Player, *Player, *Player) {
	s := authServer(t)
	v, v2, a := s.join("1.1.1.1"), s.join("2.2.2.2"), s.join("3.3.3.3")
	for _, p := range []*Player{v, v2, a} {
		p.ready = true
		drain(p)
	}
	v.Team, v2.Team, a.Team = content.Vibe, content.Vibe, content.Artisan
	return s, v, v2, a
}

func TestChatAllTeamAndDeadRule(t *testing.T) {
	s, v, v2, a := chatServer(t)
	s.say(v, "gl hf", false)
	if !strings.Contains(drain(a), `"text":"gl hf"`) || !strings.Contains(drain(v2), "gl hf") || !strings.Contains(drain(v), "gl hf") {
		t.Fatal("all chat should reach everyone (sender included)")
	}
	s.say(v, "rush B", true)
	if strings.Contains(drain(a), "rush B") || !strings.Contains(drain(v2), `"tm":true`) {
		t.Fatal("team chat leaked or missed the team")
	}
	// A round is on and v is dead: only the dead hear him.
	s.round.Phase = PhaseLive
	v.Dead = true
	drain(v)
	s.say(v, "I was lagging", false)
	if strings.Contains(drain(a), "lagging") || strings.Contains(drain(v2), "lagging") {
		t.Fatal("the living heard the dead")
	}
	a.Dead = true
	s.say(v, "ggs", false)
	if !strings.Contains(drain(a), `"dead":true`) {
		t.Fatal("the dead should hear the dead")
	}
	// In warmup everyone hears everyone.
	s.round.Phase = PhaseWarmup
	s.say(v, "warmup talk", false)
	if !strings.Contains(drain(v2), "warmup talk") {
		t.Fatal("warmup: no dead rule")
	}
}

func TestChatRateLimit(t *testing.T) {
	s, v, v2, _ := chatServer(t)
	for i := range chatBurst + 3 {
		s.say(v, "spam"+string(rune('a'+i)), false)
	}
	if n := strings.Count(drain(v2), `"t":"chat"`); n != chatBurst {
		t.Fatalf("got %d messages through, want the burst of %d", n, chatBurst)
	}
}

func TestRadioGoesToTheLivingTeam(t *testing.T) {
	s, v, v2, a := chatServer(t)
	s.radio(v, "need_backup")
	if got := drain(v2); !strings.Contains(got, `"id":"need_backup"`) || !strings.Contains(got, `"p":`) {
		t.Fatalf("teammate: %s", got)
	}
	if strings.Contains(drain(a), "need_backup") {
		t.Fatal("radio reached the enemy")
	}
	s.radio(v, "<script>")
	if strings.Contains(drain(v2), "script") {
		t.Fatal("bad radio id relayed")
	}
}
