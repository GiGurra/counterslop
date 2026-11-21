package game

import (
	"os"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

func econServer(t *testing.T) (*Server, *Player, *Player) {
	t.Helper()
	s := authServer(t)
	b, err := os.ReadFile("../../web/assets/economy.json")
	if err != nil {
		t.Fatal(err)
	}
	if s.Economy, err = LoadEconomy(b); err != nil {
		t.Fatal(err)
	}
	v, a := s.join("1.1.1.1"), s.join("2.2.2.2")
	v.ready, a.ready = true, true
	v.Team, a.Team = content.Vibe, content.Artisan
	s.round.Phase = PhaseLive
	return s, v, a
}

func TestStartMoneyAndWarmupPaysNothing(t *testing.T) {
	s, v, a := econServer(t)
	if v.Money != 800 {
		t.Fatalf("start money %d", v.Money)
	}
	s.round.Phase = PhaseWarmup
	s.moneyOnKill(v, a, "prompt")
	if v.Money != 800 {
		t.Fatalf("warmup kill paid: %d", v.Money)
	}
}

func TestKillRewardsByWeaponClass(t *testing.T) {
	for _, tc := range []struct {
		weapon string
		want   int
	}{{"prompt", 300}, {"stub", 300}, {"codereview", 100}, {"keyboard", 1500}, {"bigdiff", 0}} {
		s, v, a := econServer(t)
		drain(v)
		s.moneyOnKill(v, a, tc.weapon)
		if got := v.Money - 800; got != tc.want {
			t.Errorf("%s kill paid %d, want %d", tc.weapon, got, tc.want)
		}
		if tc.want > 0 && !strings.Contains(drain(v), `"t":"money"`) {
			t.Errorf("%s: no money message", tc.weapon)
		}
	}
}

func TestTeamKillPenaltyAndClamp(t *testing.T) {
	s, v, _ := econServer(t)
	mate := s.join("3.3.3.3")
	mate.ready, mate.Team = true, content.Vibe
	s.moneyOnKill(v, mate, "prompt")
	if v.Money != 500 {
		t.Fatalf("team kill: %d, want 500", v.Money)
	}
	v.Money = 15900
	s.moneyOnKill(v, s.players[2], "keyboard") // a teammate again: -300
	v.Money = 15900
	s.pay(v, 1500, "test")
	if v.Money != 16000 {
		t.Fatalf("max money: %d", v.Money)
	}
	v.Money = 100
	s.pay(v, -300, "test")
	if v.Money != 0 {
		t.Fatalf("money below zero: %d", v.Money)
	}
}

func TestLossBonusStreak(t *testing.T) {
	s, v, a := econServer(t)
	// Artisans lose five in a row: 1400, 1900, 2400, 2900, 3400, then capped.
	for i, want := range []int{1400, 1900, 2400, 2900, 3400, 3400} {
		a.Money = 0
		s.moneyOnRoundEnd(content.Vibe, "elimination", false)
		if a.Money != want {
			t.Fatalf("loss %d: bonus %d, want %d", i+1, a.Money, want)
		}
	}
	// A win takes the streak down by one: the next loss pays 2900.
	s.moneyOnRoundEnd(content.Artisan, "elimination", false)
	a.Money = 0
	s.moneyOnRoundEnd(content.Vibe, "elimination", false)
	if a.Money != 3400 {
		// streak was 5 -> win -> 4 -> loss -> 5: 3400
		t.Fatalf("after win then loss: %d, want 3400", a.Money)
	}
	v.Money = 0
	s.moneyOnRoundEnd(content.Vibe, "exploded", true)
	if v.Money != 3500 {
		t.Fatalf("exploded win: %d", v.Money)
	}
}

func TestPlantedLossBonusAndNoSaveBonus(t *testing.T) {
	s, v, _ := econServer(t)
	v.Money = 0
	s.moneyOnRoundEnd(content.Artisan, "defused", true)
	if v.Money != 1400+800 {
		t.Fatalf("lost after a plant: %d, want 2200", v.Money)
	}
	// Time ran out without a plant: a Vibe Coder who survived gets nothing.
	v.Money = 0
	s.moneyOnRoundEnd(content.Artisan, "time", false)
	if v.Money != 0 {
		t.Fatalf("saved and still paid: %d", v.Money)
	}
	v.Dead = true
	s.moneyOnRoundEnd(content.Artisan, "time", false)
	if v.Money == 0 {
		t.Fatal("a dead Vibe Coder should get the loss bonus")
	}
}

func TestPlantDefuseRewards(t *testing.T) {
	s, v, a := econServer(t)
	s.moneyOnPlant(v)
	s.moneyOnDefuse(a)
	if v.Money != 1100 || a.Money != 1100 {
		t.Fatalf("plant %d defuse %d", v.Money, a.Money)
	}
}
