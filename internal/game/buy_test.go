package game

import (
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// buyServer: one Vibe Coder and one Artisan in freeze time, standing in
// their buy zones (no zones on the test map = buy anywhere).
func buyServer(t *testing.T) (*Server, *Player, *Player) {
	t.Helper()
	s, v, a := econServer(t)
	s.Weapons = registry(t)
	s.round.Phase, s.round.Number = PhaseFreeze, 1
	s.round.buyUntil = s.clock().Add(time.Minute)
	for _, p := range []*Player{v, a} {
		p.inv = s.Weapons.PistolLoadout()
		p.Money = 16000
	}
	return s, v, a
}

func TestBuyWeaponTeamRulesAndMoney(t *testing.T) {
	s, v, a := buyServer(t)
	if why := s.purchase(v, "prompt"); why != "" || v.Money != 16000-2700 || v.inv.Slots["primary"].ID != "prompt" || v.inv.Current != "primary" {
		t.Fatalf("prompt: %q money %d inv %+v", why, v.Money, v.inv.Slots["primary"])
	}
	if why := s.purchase(v, "prompt"); why == "" {
		t.Fatal("bought the same gun twice")
	}
	if why := s.purchase(a, "prompt"); why == "" {
		t.Fatal("an Artisan bought the Vibe Coders' rifle")
	}
	// Buying into a taken slot drops the old gun on the ground.
	items := len(s.items)
	if why := s.purchase(v, "codereview"); why != "" || len(s.items) != items+1 || s.items[len(s.items)-1].Weapon != "prompt" {
		t.Fatalf("swap buy: %q, items %d", why, len(s.items))
	}
	a.Money = 100
	if why := s.purchase(a, "linter"); why != "can't afford it" || a.Money != 100 {
		t.Fatalf("unaffordable: %q %d", why, a.Money)
	}
	if why := s.purchase(a, "keyboard"); why == "" {
		t.Fatal("bought melee")
	}
}

func TestBuyGear(t *testing.T) {
	s, v, a := buyServer(t)
	if s.purchase(v, "vest") != "" || v.Armor != 100 || v.Money != 16000-650 {
		t.Fatalf("vest: armor %d money %d", v.Armor, v.Money)
	}
	if s.purchase(v, "helmet") != "" || !v.Helmet || v.Money != 16000-650-350 {
		t.Fatalf("helmet on top of a vest costs 350: money %d", v.Money)
	}
	if s.purchase(v, "kit") == "" {
		t.Fatal("a Vibe Coder bought a Rollback Kit")
	}
	if s.purchase(a, "kit") != "" || !a.Kit {
		t.Fatal("kit")
	}
}

func TestCanBuyTimeAndZone(t *testing.T) {
	s, v, _ := buyServer(t)
	s.Map.BuyZones = map[string][]content.Zone{content.Vibe: {{Min: [3]float64{-1, -1, -1}, Max: [3]float64{1, 3, 1}}}}
	v.Pos = Vec3{0, 0, 0}
	if ok, why := s.canBuy(v); !ok {
		t.Fatalf("in zone, freeze: %s", why)
	}
	v.Pos = Vec3{10, 0, 0}
	if ok, _ := s.canBuy(v); ok {
		t.Fatal("bought outside the buy zone")
	}
	v.Pos = Vec3{0, 0, 0}
	s.round.Phase, s.round.buyUntil = PhaseLive, s.clock().Add(-time.Second)
	if ok, _ := s.canBuy(v); ok {
		t.Fatal("bought after buy time")
	}
	s.round.Phase = PhaseWarmup
	before := v.Money
	if ok, _ := s.canBuy(v); !ok || s.purchase(v, "prompt") != "" || v.Money != before {
		t.Fatal("warmup: buy anywhere, free")
	}
}

func TestArmorAbsorbs(t *testing.T) {
	w := &Weapon{ArmorPen: 0.75}
	p := &Player{Armor: 100}
	if got := absorb(p, w, "legs", 40); got != 40 || p.Armor != 100 {
		t.Fatalf("legs are unarmored: %v armor %d", got, p.Armor)
	}
	if got := absorb(p, w, "head", 100); got != 100 {
		t.Fatalf("head without helmet: %v", got)
	}
	if got := absorb(p, w, "body", 40); got != 30 || p.Armor != 95 {
		t.Fatalf("body: %v armor %d, want 30 and 95", got, p.Armor)
	}
	p.Helmet = true
	if got := absorb(p, w, "head", 100); got != 75 {
		t.Fatalf("helmet: %v", got)
	}
	p.Armor = 2
	if got := absorb(p, &Weapon{}, "body", 40); got != 36 || p.Armor != 0 || p.Helmet {
		t.Fatalf("armor runs out: %v armor %d helmet %v", got, p.Armor, p.Helmet)
	}
}

func TestSurvivorsKeepWeaponsDeadStartOver(t *testing.T) {
	s, v, a := buyServer(t)
	s.purchase(v, "prompt")
	s.purchase(v, "vest")
	s.purchase(a, "linter")
	a.Dead = true
	s.round.Phase = PhaseOver
	s.startRound(s.clock())
	if v.inv.Slots["primary"] == nil || v.inv.Slots["primary"].ID != "prompt" || v.Armor != 100 {
		t.Fatalf("survivor lost their gear: %+v armor %d", v.inv.Slots["primary"], v.Armor)
	}
	if a.inv.Slots["primary"] != nil {
		t.Fatalf("the dead kept their rifle: %+v", a.inv.Slots["primary"])
	}
}

// Warmup gear is free and doesn't count: the first real round starts everyone
// on the pistol loadout, survivor or not.
func TestFirstRoundStartsFromPistolsAfterWarmup(t *testing.T) {
	s, v, _ := buyServer(t)
	s.round.Phase, s.round.Number = PhaseWarmup, 0
	v.inv = s.Weapons.Loadout("") // the warmup rifle
	v.Armor = 100
	s.startRound(s.clock())
	if v.inv.Slots["primary"] != nil || v.Armor != 0 || v.inv.Current != "sidearm" {
		t.Fatalf("round 1 kept warmup gear: %+v armor %d", v.inv.Slots["primary"], v.Armor)
	}
}
