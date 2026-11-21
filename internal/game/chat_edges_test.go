package game

import (
	"strings"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// Adversarial chat/radio edges (QA pass): spectators, dead team chat,
// round end and restart, the bucket refill, radio while dead.

func TestChatSpectatorsTalkToGhostsOnlyWhileARoundIsOn(t *testing.T) {
	s, v, v2, a := chatServer(t)
	spec := s.join("4.4.4.4")
	spec.ready, spec.Team = true, content.Spectator
	drain(spec)
	s.round.Phase = PhaseLive
	s.say(spec, "nice clutch", false)
	if strings.Contains(drain(v), "clutch") || strings.Contains(drain(a), "clutch") {
		t.Fatal("the living heard a spectator mid-round")
	}
	v2.Dead = true
	drain(v2)
	s.say(spec, "nice clutch 2", false)
	if !strings.Contains(drain(v2), "clutch 2") {
		t.Fatal("a dead player should hear spectators")
	}
	// The living reach spectators (ghosts hear everyone).
	s.say(a, "eco round", false)
	if !strings.Contains(drain(spec), "eco round") {
		t.Fatal("spectator missed the living")
	}
	// Spectator team chat stays with spectators.
	s.say(spec, "spec only", true)
	if strings.Contains(drain(v2), "spec only") {
		t.Fatal("spectator team chat reached a dead player of a team")
	}
}

func TestChatDeadTeamChatOnlyToDeadTeammates(t *testing.T) {
	s, v, v2, a := chatServer(t)
	s.round.Phase = PhaseLive
	v.Dead, a.Dead = true, true
	s.say(v, "he's low, one shot", true)
	if strings.Contains(drain(a), "low") {
		t.Fatal("dead team chat reached a dead enemy")
	}
	if strings.Contains(drain(v2), "low") {
		t.Fatal("dead team chat reached a living teammate (ghosting)")
	}
}

func TestChatDeadRuleAcrossRoundEndAndRestart(t *testing.T) {
	s, v, v2, _ := chatServer(t)
	v.Dead = true
	s.round.Phase = PhaseOver // round decided, v still dead: still a ghost
	s.say(v, "after the end", false)
	if strings.Contains(drain(v2), "after the end") {
		t.Fatal("round over: the dead reached the living before the next round")
	}
	// Next round: v respawned, everyone hears him again.
	s.round.Phase, v.Dead = PhaseFreeze, false
	s.say(v, "new round", false)
	if !strings.Contains(drain(v2), "new round") {
		t.Fatal("a respawned player is muted")
	}
	// Back to warmup: the dead talk to everyone.
	s.round.Phase, v.Dead = PhaseWarmup, true
	s.say(v, "warmup", false)
	if !strings.Contains(drain(v2), "warmup") {
		t.Fatal("warmup: dead rule still on")
	}
}

func TestChatBucketRefills(t *testing.T) {
	s, v, _, _ := chatServer(t)
	now := time.Unix(1000, 0)
	for i := range chatBurst {
		if !s.chatAllowed(v, now) {
			t.Fatalf("message %d of the burst refused", i+1)
		}
	}
	if s.chatAllowed(v, now) {
		t.Fatal("over the burst allowed")
	}
	if s.chatAllowed(v, now.Add(time.Second)) {
		t.Fatal("refilled too fast (one per 2 s)")
	}
	if !s.chatAllowed(v, now.Add(3*time.Second)) {
		t.Fatal("no refill after 2 s")
	}
}

// CS: the dead can't use the radio (a ping on a corpse misleads), in
// warmup's respawn wait too.
func TestRadioRefusedWhileDead(t *testing.T) {
	s, v, v2, _ := chatServer(t)
	for _, ph := range []string{PhaseLive, PhaseOver, PhaseWarmup} {
		s.round.Phase = ph
		v.Dead = true
		s.radio(v, "need_backup")
		if strings.Contains(drain(v2), "need_backup") {
			t.Errorf("%s: a dead player's radio got through", ph)
		}
	}
	v.Dead = false
	s.radio(v, "need_backup")
	if !strings.Contains(drain(v2), "need_backup") {
		t.Fatal("alive radio refused")
	}
}
