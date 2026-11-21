package game

import (
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// Adversarial economy edges (QA pass): what must NOT pay, charge or carry.

// The Big Diff's blast credits its kills to the planter so the kill feed has
// a name, but it is the bomb, not a team kill: CS pays nothing and fines
// nothing for C4 kills. A planter whose teammates stood too close lost $300
// each.
func TestBigDiffBlastIsNoTeamKill(t *testing.T) {
	s, v, a := econServer(t)
	mate := s.join("3.3.3.3")
	mate.ready, mate.Team = true, content.Vibe
	s.round.Bomb = bombState{State: BombPlanted, Carrier: v.ID}
	for _, p := range []*Player{a, mate} {
		p.Pos = Vec3{0, 0, 0}
	}
	v.Pos = Vec3{100, 0, 0} // the planter ran; a teammate and an Artisan didn't
	before := v.Money
	s.blast(Vec3{0, 0, 0})
	if !mate.Dead || !a.Dead {
		t.Fatalf("blast at point blank: mate dead %v artisan dead %v", mate.Dead, a.Dead)
	}
	if v.Money != before {
		t.Fatalf("planter's money %d -> %d after the blast (bomb kills pay and cost nothing)", before, v.Money)
	}
}

func TestBuyRefusals(t *testing.T) {
	s, v, a := buyServer(t)
	for _, item := range []string{"sell", "keyboard", "bigdiff", "", "linter"} { // no selling, no melee, no bomb, the other team's rifle
		before := v.Money
		if why := s.purchase(v, item); why == "" || v.Money != before {
			t.Errorf("buy %q: %q, money %d -> %d", item, why, before, v.Money)
		}
	}
	// Same weapon twice: no second charge.
	s.purchase(v, "prompt")
	before := v.Money
	if s.purchase(v, "prompt") == "" || v.Money != before {
		t.Fatal("bought the Prompt twice")
	}
	// Can't afford: nothing changes.
	a.Money = 2000
	if s.purchase(a, "linter") == "" || a.Money != 2000 || a.inv.Slots["primary"] != nil {
		t.Fatal("bought a rifle on $2000")
	}
	// Grenades: per type (1 HE, 2 flashes, 1 smoke), four in all.
	a.Money = 16000
	for _, n := range []string{"mergeconflict", "stacktrace", "stacktrace", "smoketest"} {
		if why := s.purchase(a, n); why != "" {
			t.Fatalf("grenade %s: %s", n, why)
		}
	}
	for _, n := range []string{"mergeconflict", "stacktrace", "smoketest"} {
		if s.purchase(a, n) == "" {
			t.Fatalf("a fifth grenade (%s)", n)
		}
	}
	if a.Money != 16000-300-200-200-300 {
		t.Fatalf("grenade money %d", a.Money)
	}
	// Helmet over a full vest is the 350 upgrade; a second helmet is refused.
	s.purchase(a, "vest")
	m := a.Money
	if s.purchase(a, "helmet") != "" || m-a.Money != 350 || s.purchase(a, "helmet") == "" {
		t.Fatalf("helmet upgrade: paid %d", m-a.Money)
	}
}

func TestCanBuyRefusals(t *testing.T) {
	s, v, _ := buyServer(t)
	cases := []struct {
		name string
		set  func()
	}{
		{"dead", func() { v.Dead = true }},
		{"spectator", func() { v.Team = content.Spectator }},
		{"round over", func() { s.round.Phase = PhaseOver }},
		{"bomb planted", func() { s.round.Phase = PhasePlanted }},
		{"live after buy time", func() { s.round.Phase, s.round.buyUntil = PhaseLive, s.clock().Add(-time.Millisecond) }},
	}
	for _, c := range cases {
		s.round.Phase, s.round.buyUntil, v.Dead, v.Team = PhaseFreeze, s.clock().Add(time.Minute), false, content.Vibe
		c.set()
		before := v.Money
		s.buy(v, "vest")
		if v.Money != before || v.Armor != 0 {
			t.Errorf("%s: bought (money %d -> %d, armor %d)", c.name, before, v.Money, v.Armor)
		}
	}
	s.round.Phase, s.round.buyUntil, v.Dead, v.Team = PhaseLive, s.clock().Add(time.Second), false, content.Vibe
	if ok, why := s.canBuy(v); !ok {
		t.Fatalf("live, within buy time: %s", why)
	}
}

func TestMoneyEdges(t *testing.T) {
	s, v, a := econServer(t)
	// Round win at the cap stays at the cap; a loss bonus too.
	v.Money, a.Money = 15000, 15900
	s.moneyOnRoundEnd(content.Vibe, "elimination", false)
	if v.Money != 16000 || a.Money != 16000 {
		t.Fatalf("cap: winner %d loser %d", v.Money, a.Money)
	}
	// A suicide (own HE) pays and costs nothing.
	v.Money = 1000
	s.moneyOnKill(v, v, "mergeconflict")
	if v.Money != 1000 {
		t.Fatalf("suicide moved money: %d", v.Money)
	}
	// A team kill with a grenade is still a team kill.
	mate := s.join("3.3.3.3")
	mate.ready, mate.Team = true, content.Vibe
	s.moneyOnKill(v, mate, "mergeconflict")
	if v.Money != 700 {
		t.Fatalf("grenade team kill: %d, want 700", v.Money)
	}
	// Spectators earn nothing at round end.
	spec := s.join("4.4.4.4")
	spec.ready, spec.Team, spec.Money = true, content.Spectator, 0
	s.moneyOnRoundEnd(content.Artisan, "elimination", false)
	if spec.Money != 0 {
		t.Fatalf("spectator paid %d", spec.Money)
	}
	// The winners' streak never goes below zero: a team that only won, then
	// loses once, gets the base bonus.
	s.lossStreak = map[string]int{}
	for range 3 {
		s.moneyOnRoundEnd(content.Vibe, "elimination", false)
	}
	v.Money = 0
	s.moneyOnRoundEnd(content.Artisan, "elimination", false)
	if v.Money != 1400 {
		t.Fatalf("first loss after a win streak: %d, want 1400", v.Money)
	}
}

// Survivors keep armor, helmet and shades into the next round; the dead lose
// all of it (kit included).
func TestCarryOverGear(t *testing.T) {
	s, v, a := buyServer(t)
	s.round.Number = 2
	for _, it := range []string{"vest", "helmet", "shades"} {
		s.purchase(v, it)
		s.purchase(a, it)
	}
	s.purchase(a, "kit")
	a.Dead = true
	s.round.Phase = PhaseOver
	s.startRound(s.clock())
	if v.Armor != 100 || !v.Helmet || !v.Shades {
		t.Fatalf("survivor lost gear: armor %d helmet %v shades %v", v.Armor, v.Helmet, v.Shades)
	}
	if a.Armor != 0 || a.Helmet || a.Kit || a.Shades {
		t.Fatalf("the dead kept gear: armor %d helmet %v kit %v shades %v", a.Armor, a.Helmet, a.Kit, a.Shades)
	}
}

// rcon restart ("restarted the match") in the middle of a round-based match
// keeps everyone's money, armor, loss streaks, round number and score, and
// hands out the full warmup loadout (a free rifle) while rounds are on.
// Handed to lead (economy/rounds owner): what a match restart should reset.
func TestRestartMidMatchResetsTheEconomy(t *testing.T) {
	s, v, _ := buyServer(t)
	s.round.Phase, s.round.Number = PhaseLive, 5
	s.round.Score = map[string]int{content.Vibe: 3, content.Artisan: 1}
	s.lossStreak = map[string]int{content.Artisan: 3}
	v.Money, v.Armor = 9000, 100
	s.restart()
	if st := s.econ().LossBonus.Start; v.Money != s.econ().StartMoney || v.Armor != 0 || s.lossStreak[content.Vibe] != st || s.lossStreak[content.Artisan] != st {
		t.Errorf("after restart: money %d armor %d streak %v", v.Money, v.Armor, s.lossStreak)
	}
	if s.round.Score[content.Vibe] != 0 || s.round.Number > 1 {
		t.Errorf("after restart: round %d score %v", s.round.Number, s.round.Score)
	}
	if s.roundsOn() && v.inv.Slots["primary"] != nil {
		t.Errorf("a free %s after a restart in rounds mode", v.inv.Slots["primary"].ID)
	}
}

// CS2: each team's loss counter starts at 1 at the start of the match, so the
// pistol-round loser gets 1900; after wins it can drop to 0 and pay 1400.
func TestLossBonusStartsAtOneEachMatch(t *testing.T) {
	s, _, a := econServer(t)
	s.resetMoney()
	a.Money = 0
	s.moneyOnRoundEnd(content.Vibe, "elimination", false)
	if a.Money != 1900 {
		t.Fatalf("pistol-round loss: %d, want 1900", a.Money)
	}
	for range 3 {
		s.moneyOnRoundEnd(content.Artisan, "elimination", false) // 2 -> 1 -> 0 -> 0
	}
	a.Money = 0
	s.moneyOnRoundEnd(content.Vibe, "elimination", false)
	if a.Money != 1400 {
		t.Fatalf("loss after three wins: %d, want 1400", a.Money)
	}
}

// CS: with the helmet on and the vest worn down, "kevlar + helmet" costs the
// vest, the same as buying the vest (both give 100 armor and the helmet).
func TestHelmetWithHelmetOnCostsTheVest(t *testing.T) {
	s, v, _ := buyServer(t)
	v.Armor, v.Helmet, v.Money = 40, true, 5000
	if why := s.purchase(v, "helmet"); why != "" {
		t.Fatal(why)
	}
	if v.Money != 5000-650 || v.Armor != 100 || !v.Helmet {
		t.Fatalf("money %d armor %d helmet %v, want 4350/100/true", v.Money, v.Armor, v.Helmet)
	}
}
