package game

import (
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

func TestAFKPlayerGoesToSpectatorsAfterIdleRounds(t *testing.T) {
	s, v, a := roundServer(t)
	s.cvars["mp_afk_rounds"] = "2"
	v.lastLook, a.lastLook = [2]float64{1, 0}, [2]float64{1, 0}
	now := time.Now()
	ticks(s, 2, &now, map[*Player]Cmd{v: {Yaw: 1}, a: {Yaw: 1}}) // round 1
	idle := map[*Player]Cmd{v: {Yaw: 1}, a: {Yaw: 1}}
	end := func(look float64) {
		ticks(s, 1, &now, map[*Player]Cmd{v: {Yaw: look}, a: {Yaw: 1}}) // v moves the mouse; a never does
		ticks(s, 20, &now, idle)
		s.endRound(now, content.Artisan, "time")
		now = now.Add(10 * time.Second)
		ticks(s, 2, &now, idle) // the next round starts
	}
	end(2)
	if a.Team != content.Artisan || a.idleRounds != 1 || v.idleRounds != 0 {
		t.Fatalf("after one round: a %s idle %d, v idle %d", a.Team, a.idleRounds, v.idleRounds)
	}
	end(3)
	if a.Team != content.Spectator || v.Team != content.Vibe {
		t.Fatalf("after two idle rounds: a %s, v %s; want a in spectators", a.Team, v.Team)
	}
	var said bool
	for _, m := range msgs(v) {
		if m["t"] == "notice" && m["text"] != nil {
			said = said || len(m["text"].(string)) > 0 && m["text"].(string)[:len(a.Name)] == a.Name
		}
	}
	if !said {
		t.Fatal("nobody was told")
	}
}

func TestAFKCountsBuyingAndChatAndSkipsLateJoiners(t *testing.T) {
	s, v, a := roundServer(t)
	s.cvars["mp_afk_rounds"] = "1"
	now := time.Now()
	ticks(s, 2, &now, nil)
	s.handle(a, inbound{T: "say", Text: "gl"})
	late := s.join("3.3.3.3")
	late.ready, late.Team = true, content.Vibe // joined mid-round: didn't play it from the start
	s.endRound(now, content.Vibe, "time")
	now = now.Add(10 * time.Second)
	ticks(s, 2, &now, nil)
	if a.Team != content.Artisan || late.Team != content.Vibe {
		t.Fatalf("a (chatted) %s, late joiner %s: neither is AFK", a.Team, late.Team)
	}
	if v.Team != content.Spectator {
		t.Fatalf("v did nothing for a round with mp_afk_rounds 1: %s", v.Team)
	}
	// Off on test servers by default; 0 = never.
	s2, v2, _ := roundServer(t)
	s2.Test = true
	if s2.afkRounds() != 0 || s2.autoBalance() {
		t.Fatal("auto is on on a test server")
	}
	s2.cvars["mp_afk_rounds"] = "0"
	v2.idleRounds = 9
	s2.moveAFK()
	if v2.Team != content.Vibe {
		t.Fatal("mp_afk_rounds 0 moved someone")
	}
	s2.Test = false
	s2.cvars["mp_afk_rounds"] = "auto"
	if s2.afkRounds() != afkRoundsDefault || !s2.autoBalance() {
		t.Fatal("auto is off on a real server")
	}
}

func TestAutobalanceMovesNewestPlayersBotsFirst(t *testing.T) {
	s, v, a := roundServer(t)
	s.cvars["mp_autoteambalance"] = "1"
	base := time.Now()
	v.joinedAt, a.joinedAt = base, base
	var vs []*Player
	for i, ip := range []string{"3.3.3.3", "4.4.4.4", "5.5.5.5"} {
		p := s.join(ip)
		p.ready, p.Team, p.joinedAt = true, content.Vibe, base.Add(time.Duration(i+1)*time.Minute)
		vs = append(vs, p)
	}
	// 4 Vibe Coders vs 1 Artisan: the newest moves (3 vs 2).
	s.balanceTeams()
	if vs[2].Team != content.Artisan || vs[1].Team != content.Vibe || vs[0].Team != content.Vibe || v.Team != content.Vibe {
		t.Fatalf("teams %s %s %s %s", v.Team, vs[0].Team, vs[1].Team, vs[2].Team)
	}
	// 3 vs 2: balanced enough, nobody moves.
	s.balanceTeams()
	if vs[1].Team != content.Vibe {
		t.Fatal("moved within one")
	}
	// A playing bot goes before a newer human.
	bot := s.addAI(content.Vibe)
	bot.joinedAt = base
	p := s.join("6.6.6.6")
	p.ready, p.Team, p.joinedAt = true, content.Vibe, base.Add(time.Hour)
	s.balanceTeams() // 5 vs 2 -> 4 vs 3
	if bot.Team != content.Artisan || p.Team != content.Vibe {
		t.Fatalf("bot %s, newest human %s: the bot should move", bot.Team, p.Team)
	}
	s.cvars["mp_autoteambalance"] = "0"
	q := s.join("7.7.7.7")
	q.ready, q.Team = true, content.Vibe
	r := s.join("8.8.8.8") // 6 vs 3 with balance off
	r.ready, r.Team = true, content.Vibe
	s.balanceTeams()
	if q.Team != content.Vibe || r.Team != content.Vibe {
		t.Fatal("mp_autoteambalance 0 moved someone")
	}
}
