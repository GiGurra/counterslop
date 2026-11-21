package game

import (
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

func TestMatchResult(t *testing.T) {
	sc := func(v, a int) map[string]int { return map[string]int{content.Vibe: v, content.Artisan: a} }
	for _, c := range []struct {
		played, v, a int
		otOn         bool
		want         string
	}{
		{12, 6, 6, true, ""},
		{20, 13, 7, true, content.Vibe},     // first to 13
		{25, 12, 13, true, ""},              // 12-12 went to overtime: 13 isn't enough there
		{24, 12, 12, true, ""},              // tie: overtime
		{24, 12, 12, false, MatchDraw},      // no overtime: a draw
		{28, 16, 12, true, content.Vibe},    // OT1: first to 4 of 6
		{27, 14, 13, true, ""},              //
		{30, 15, 15, true, ""},              // OT1 tied: OT2
		{34, 15, 19, true, content.Artisan}, // OT2: 15+4
		{33, 17, 16, true, ""},              //
		{23, 13, 10, false, content.Vibe},   //
		{3, 2, 1, true, content.Vibe},       // mp_maxrounds 3: first to 2
	} {
		reg := 24
		if c.played == 3 {
			reg = 3
		}
		if got := matchResult(c.played, sc(c.v, c.a), reg, 6, c.otOn); got != c.want {
			t.Errorf("played %d at %d:%d (ot %v): got %q, want %q", c.played, c.v, c.a, c.otOn, got, c.want)
		}
	}
}

func TestHalftimeAndOvertimeRounds(t *testing.T) {
	for _, c := range []struct {
		played int
		want   bool
	}{{12, true}, {11, false}, {13, false}, {24, false}, {27, true}, {30, false}, {33, true}} {
		if got := halftimeAfter(c.played, 24, 6); got != c.want {
			t.Errorf("halftime after %d: got %v", c.played, got)
		}
	}
	for n, want := range map[int]int{1: 0, 24: 0, 25: 1, 30: 1, 31: 2, 37: 3} {
		if got := overtimeOf(n, 24, 6); got != want {
			t.Errorf("round %d: overtime %d, want %d", n, got, want)
		}
	}
}

// matchServer: one Vibe Coder (v) and one Artisan (a), a short match (first
// to 3: 4 rounds, halftime after 2), no waiting anywhere.
func matchServer(t *testing.T) (*Server, *Player, *Player) {
	s, v, a := roundServer(t)
	for k, val := range map[string]string{"mp_maxrounds": "4", "mp_overtime_maxrounds": "2", "mp_round_restart_delay": "0",
		"mp_halftime_duration": "0", "mp_match_restart_delay": "0", "mp_afk_rounds": "0", "mp_autoteambalance": "0"} {
		s.cvars[k] = val
	}
	return s, v, a
}

// winRound: the side `team` wins the live round by elimination.
func winRound(t *testing.T, s *Server, now *time.Time, team string) {
	t.Helper()
	ticks(s, 2, now, nil)
	if s.round.Phase != PhaseLive {
		t.Fatalf("phase %q, want live", s.round.Phase)
	}
	var killer *Player
	for _, p := range s.players {
		if p.Team == team && !p.Dead {
			killer = p
		}
	}
	for _, p := range s.players {
		if p.Team != team && (p.Team == content.Vibe || p.Team == content.Artisan) && !p.Dead {
			s.killBy(killer, p, "prompt", false)
		}
	}
	ticks(s, 1, now, nil)
	if s.round.Phase != PhaseOver && s.round.Phase != PhaseMatchOver {
		t.Fatalf("phase %q after the elimination", s.round.Phase)
	}
	*now = now.Add(10 * time.Millisecond)
	ticks(s, 1, now, nil) // the round-end delay (0) is over: halftime, match end or the next round
}

func TestMatchHalftimeSwapAndMatchEnd(t *testing.T) {
	s, v, a := matchServer(t)
	now := time.Now()
	winRound(t, s, &now, content.Vibe) // 1:0
	if s.round.Number != 2 || s.round.Score[content.Vibe] != 1 {
		t.Fatalf("round %d score %v", s.round.Number, s.round.Score)
	}
	v.Money = 9000
	winRound(t, s, &now, content.Vibe) // 2:0 -> halftime
	if s.round.Phase != PhaseHalftime {
		t.Fatalf("phase %q after 2 of 4 rounds, want halftime", s.round.Phase)
	}
	ticks(s, 1, &now, nil) // halftime (0 s) over: swap
	if v.Team != content.Artisan || a.Team != content.Vibe {
		t.Fatalf("teams %s / %s after halftime, want swapped", v.Team, a.Team)
	}
	if s.round.Score[content.Artisan] != 2 || s.round.Score[content.Vibe] != 0 || !s.round.Swapped {
		t.Fatalf("score %v swapped %v: the score follows the players", s.round.Score, s.round.Swapped)
	}
	if v.Money != s.econ().StartMoney || v.inv.Slots["primary"] != nil {
		t.Fatalf("after halftime: money %d, primary %v: want the pistol round", v.Money, v.inv.Slots["primary"])
	}
	if s.round.Bomb.Carrier != a.ID {
		t.Fatalf("the Big Diff goes to the new Vibe Coder (a), got %d", s.round.Bomb.Carrier)
	}
	winRound(t, s, &now, content.Artisan) // v's side: 3:0 -> match over
	if s.round.Phase != PhaseMatchOver || s.round.MatchWinner != content.Artisan {
		t.Fatalf("phase %q winner %q, want match over, won by v's side (now Artisans)", s.round.Phase, s.round.MatchWinner)
	}
	if s.vote == nil {
		t.Fatal("no map vote at match end")
	}
	if v.Points != 3*scoreKill || v.Kills != 3 {
		t.Fatalf("v: %d kills, %d points", v.Kills, v.Points)
	}
	// The end screen is over, the vote didn't change the map (it runs in advance): warmup, then a new match on the original sides.
	s.vote = nil
	now = now.Add(10 * time.Millisecond)
	ticks(s, 1, &now, nil)
	if s.round.Phase != PhaseWarmup {
		t.Fatalf("phase %q after the match, want warmup", s.round.Phase)
	}
	ticks(s, 2, &now, nil)
	if v.Team != content.Vibe || a.Team != content.Artisan || s.round.Number != 1 || v.Kills != 0 || v.Points != 0 {
		t.Fatalf("new match: teams %s/%s round %d kills %d points %d", v.Team, a.Team, s.round.Number, v.Kills, v.Points)
	}
}

func TestMatchOvertimeAndDraw(t *testing.T) {
	s, _, _ := matchServer(t)
	now := time.Now()
	// Each side's players win one round per half: 2:2 after regulation.
	winRound(t, s, &now, content.Vibe)
	winRound(t, s, &now, content.Artisan)
	ticks(s, 1, &now, nil) // halftime over
	winRound(t, s, &now, content.Vibe)
	winRound(t, s, &now, content.Artisan)
	if s.round.Phase == PhaseMatchOver || s.round.Overtime != 1 || s.round.Number != 5 {
		t.Fatalf("phase %q overtime %d round %d: want overtime 1, round 5", s.round.Phase, s.round.Overtime, s.round.Number)
	}
	// Overtime (2 rounds, first to 2 of them): one each -> halftime after 1, then a second overtime.
	winRound(t, s, &now, content.Vibe)
	if s.round.Phase != PhaseHalftime {
		t.Fatalf("phase %q, want the overtime halftime", s.round.Phase)
	}
	ticks(s, 1, &now, nil)
	winRound(t, s, &now, content.Vibe)
	if s.round.Overtime != 2 || s.round.Phase == PhaseMatchOver {
		t.Fatalf("overtime %d phase %q, want overtime 2", s.round.Overtime, s.round.Phase)
	}

	// mp_overtime_enable 0: 2:2 is a draw.
	s, _, _ = matchServer(t)
	s.cvars["mp_overtime_enable"] = "0"
	now = time.Now()
	winRound(t, s, &now, content.Vibe)
	winRound(t, s, &now, content.Artisan)
	ticks(s, 1, &now, nil)
	winRound(t, s, &now, content.Vibe)
	winRound(t, s, &now, content.Artisan)
	if s.round.Phase != PhaseMatchOver || s.round.MatchWinner != MatchDraw {
		t.Fatalf("phase %q winner %q, want a draw", s.round.Phase, s.round.MatchWinner)
	}
}

func TestScorePointsAndWarmupEnd(t *testing.T) {
	s := authServer(t)
	s.cvars["mp_respawn"] = "0"
	v, a := s.join("1.1.1.1"), s.join("2.2.2.2")
	v.ready, a.ready, v.Team, a.Team = true, true, content.Vibe, content.Artisan
	now := time.Now()
	ticks(s, 2, &now, nil)
	if s.round.Phase != PhaseWarmup || s.round.endsAt.IsZero() {
		t.Fatalf("phase %q: want a running warmup timer", s.round.Phase)
	}
	s.adminCommand(v, "mp_warmup_end", nil)
	ticks(s, 1, &now, nil)
	if s.round.Phase != PhaseFreeze || s.round.Number != 1 {
		t.Fatalf("phase %q round %d after mp_warmup_end", s.round.Phase, s.round.Number)
	}
	s.adminCommand(v, "mp_warmup_end", nil) // not in warmup: refused, nothing changes
	if s.warmupEnd {
		t.Fatal("mp_warmup_end outside warmup")
	}
	scoreKillPoints(v, a)
	scoreKillPoints(v, v)
	mate := &Player{Team: content.Vibe}
	scoreKillPoints(v, mate)
	scoreKillPoints(nil, a)
	if v.Points != scoreKill+scoreSuicide+scoreTeamKill {
		t.Fatalf("points %d", v.Points)
	}
	// restart mid-match: round 1 again.
	ticks(s, 3, &now, nil)
	s.adminCommand(v, "restart", nil)
	if s.round.Number != 1 || s.round.Phase != PhaseFreeze {
		t.Fatalf("restart: round %d phase %q", s.round.Number, s.round.Phase)
	}
}

func TestAssistsFromTheDamageLedger(t *testing.T) {
	s, v, a := matchServer(t)
	v2 := s.join("3.3.3.3")
	v2.ready, v2.Team = true, content.Vibe
	now := time.Now()
	ticks(s, 2, &now, nil)
	s.noteDamage(v2, a, assistDamage, a.HP) // enough for an assist
	s.noteDamage(v, a, 30, a.HP-assistDamage)
	s.killBy(v, a, "prompt", false)
	if v2.Assists != 1 || v2.Points != scoreAssist || v.Assists != 0 || v.Points != scoreKill {
		t.Fatalf("v2 %d assists %d pts, v %d assists %d pts", v2.Assists, v2.Points, v.Assists, v.Points)
	}
	b := s.join("4.4.4.4") // 40 damage: no assist
	b.ready, b.Team = true, content.Artisan
	s.noteDamage(v2, b, assistDamage-1, 100)
	s.killBy(v, b, "prompt", false)
	if v2.Assists != 1 {
		t.Fatalf("40 damage gave an assist")
	}
}
