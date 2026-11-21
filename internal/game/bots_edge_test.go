package game

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// Adversarial edges of playing bots inside a round (QA pass): quota changes,
// kicks with the Big Diff carried, all bots dead with a human alive, bots
// added in freeze time vs mid-round; and radar spotting edges.

// botRound: one human Vibe Coder (h) plus playing bots, rounds on, 1 s freeze.
func botRound(t *testing.T, vibeBots, artBots int) (*Server, *Player, time.Time) {
	t.Helper()
	s := authServer(t)
	s.cvars["mp_freezetime"] = "1"
	s.cvars["mp_respawn"] = "0"
	h := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{})
	for range vibeBots {
		s.addAI(content.Vibe)
	}
	for range artBots {
		s.addAI(content.Artisan)
	}
	now := s.lastTickAt
	ticks(s, 1, &now, nil) // warmup -> round 1 freeze
	if s.round.Phase != PhaseFreeze || s.round.Number != 1 {
		t.Fatalf("phase %q round %d, want round 1 freeze", s.round.Phase, s.round.Number)
	}
	return s, h, now
}

// botsOf: the playing bots on a side, oldest first.
func botsOf(s *Server, team string) []*Player {
	var out []*Player
	for _, p := range s.players {
		if p.ai != nil && p.Team == team {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b *Player) int { return a.ID - b.ID })
	return out
}

func TestBotAddedInFreezePlaysMidRoundWaits(t *testing.T) {
	s, _, now := botRound(t, 0, 1)
	early := s.addAI(content.Artisan) // freeze time: plays this round
	if early.Dead || early.body == nil {
		t.Fatalf("added in freeze time: dead %v body %v, want spawned", early.Dead, early.body != nil)
	}
	ticks(s, SimHz+1, &now, nil)
	if s.round.Phase != PhaseLive {
		t.Fatalf("phase %q, want live", s.round.Phase)
	}
	late := s.addAI("") // auto: the smaller side (Vibe: 1 human vs 2 bots)
	if late.Team != content.Vibe {
		t.Fatalf("auto bot joined %q, want the smaller Vibe side", late.Team)
	}
	if !late.Dead {
		t.Fatal("added mid-round: alive, want it to wait for the next round")
	}
	if _, alive := s.fighters(content.Vibe); alive != 1 {
		t.Fatalf("vibe alive %d, want only the human", alive)
	}
	// the waiting bot plays from the next round
	for _, b := range botsOf(s, content.Artisan) {
		s.killBy(b, b, "prompt", false)
	}
	ticks(s, 1, &now, nil) // elimination: round over
	now = now.Add(time.Duration(overSecs+1) * time.Second)
	ticks(s, 2, &now, nil)
	if s.round.Number != 2 || late.Dead {
		t.Fatalf("round %d, late bot dead %v: want it alive in round 2", s.round.Number, late.Dead)
	}
}

func TestBotQuotaRaisedMidRoundWaits(t *testing.T) {
	s, h, now := botRound(t, 1, 1)
	ticks(s, SimHz+1, &now, nil) // live
	before := map[int]bool{}
	for id := range s.players {
		before[id] = true
	}
	s.adminCommand(h, "bot_quota", []string{"5"}) // 1 human + 2 bots -> 4 bots
	s.fillQuota()
	n := 0
	for id, p := range s.players {
		if p.ai == nil {
			continue
		}
		n++
		if !before[id] && !p.Dead {
			t.Fatalf("%s joined mid-round alive", p.Name)
		}
	}
	if n != 4 {
		t.Fatalf("quota 5 with one human: %d bots", n)
	}
	if s.round.Phase != PhaseLive {
		t.Fatalf("phase %q, want the round to go on", s.round.Phase)
	}
}

func TestBotKickWithTheBigDiffDropsIt(t *testing.T) {
	s, h, now := botRound(t, 1, 1)
	ticks(s, SimHz+1, &now, nil)
	carrier := botsOf(s, content.Vibe)[0]
	s.round.Bomb.State, s.round.Bomb.Carrier = BombCarried, carrier.ID
	at := carrier.Pos
	s.kickAI(content.Vibe, 1) // what bot_quota / kick does to one bot
	if b := s.round.Bomb; b.State != BombDropped || b.Pos != at {
		t.Fatalf("carrier kicked: bomb %+v, want dropped at %v", b, at)
	}
	h.Pos, h.body.Pos = at, at
	s.pickUpBomb()
	if s.round.Bomb.State != BombCarried || s.round.Bomb.Carrier != h.ID {
		t.Fatalf("human standing on it: %+v, want picked up", s.round.Bomb)
	}
	// bot_kick removes every playing bot: no Artisans left, back to warmup
	s.adminCommand(h, "bot_kick", nil)
	ticks(s, 1, &now, nil)
	if s.round.Phase != PhaseWarmup {
		t.Fatalf("no Artisans left: phase %q, want warmup", s.round.Phase)
	}
}

func TestAllBotsDeadHumanAliveEndsTheRound(t *testing.T) {
	for _, side := range []string{content.Vibe, content.Artisan} {
		t.Run(side, func(t *testing.T) {
			s := authServer(t)
			s.cvars["mp_freezetime"] = "0"
			s.cvars["mp_respawn"] = "0"
			h := cmdPlayer(t, s, "1.1.1.1", side, Vec3{})
			enemy := content.Artisan
			if side == content.Artisan {
				enemy = content.Vibe
			}
			s.addAI(side)
			s.addAI(enemy)
			s.addAI(enemy)
			now := s.lastTickAt
			ticks(s, 2, &now, nil)
			if s.round.Phase != PhaseLive {
				t.Fatalf("phase %q", s.round.Phase)
			}
			for _, b := range botsOf(s, side) { // the human's own bot dies too
				s.killBy(b, b, "prompt", false)
			}
			for _, b := range botsOf(s, enemy) {
				s.killBy(h, b, "prompt", false)
			}
			ticks(s, 1, &now, nil)
			if s.round.Phase != PhaseOver || s.round.Winner != side || s.round.Reason != "elimination" {
				t.Fatalf("every bot dead, the %s human alive: phase %q winner %q reason %q", side, s.round.Phase, s.round.Winner, s.round.Reason)
			}
		})
	}
}

func TestSpottingEdges(t *testing.T) {
	s := authServer(t)
	alice := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-20, 0, 0})
	bob := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-12, 0, 0})
	spec := cmdPlayer(t, s, "3.3.3.3", content.Spectator, Vec3{-16, 0, 0})
	alice.Yaw, bob.Yaw = -math.Pi/2, -math.Pi/2 // alice sees bob's back
	snap := func(p *Player) map[string]any {
		msgs(p)
		s.broadcast()
		for _, m := range msgs(p) {
			if m["t"] == "snap" {
				return m
			}
		}
		t.Fatalf("no snapshot for %s", p.Name)
		return nil
	}
	spots := func(m map[string]any) []int {
		var ids []int
		b, _ := json.Marshal(m["spot"])
		_ = json.Unmarshal(b, &ids)
		return ids
	}
	if m := snap(spec); m["spot"] != nil {
		t.Fatalf("spectator's snapshot carries a team spot list: %v", m["spot"])
	}
	if got := s.spotted()[content.Vibe]; !slices.Equal(got, []int{bob.ID}) {
		t.Fatalf("vibe spots %v", got)
	}
	alice.Dead = true // a dead spotter sees nothing ...
	if got := s.spotted()[content.Vibe]; len(got) != 0 {
		t.Fatalf("dead alice still spots %v", got)
	}
	// ... but a dead viewer keeps their team's list (CS: you watch your team's radar)
	carol := cmdPlayer(t, s, "4.4.4.4", content.Vibe, Vec3{-22, 0, 0})
	carol.Yaw = -math.Pi / 2
	if got := spots(snap(alice)); !slices.Equal(got, []int{bob.ID}) {
		t.Fatalf("dead alice's snapshot: %v, want carol's spot of bob", got)
	}
	if got := spots(snap(bob)); len(got) != 0 {
		t.Fatalf("bob's snapshot leaks the other side's spots: %v", got)
	}
	alice.Dead = false
	bob.Dead = true // a dead enemy isn't spotted
	if got := s.spotted()[content.Vibe]; len(got) != 0 {
		t.Fatalf("dead bob spotted: %v", got)
	}
	bob.Dead = false
	bob.Team = content.Spectator // a spectator isn't an enemy
	if got := s.spotted()[content.Vibe]; len(got) != 0 {
		t.Fatalf("spectating bob spotted: %v", got)
	}
}
