package game

import (
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
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
	s.respawn(v) // warmup: full money on every spawn, real prices
	v.Pos = Vec3{0, 0, 0}
	if v.Money != s.econ().MaxMoney {
		t.Fatalf("warmup spawn money %d, want %d", v.Money, s.econ().MaxMoney)
	}
	if ok, _ := s.canBuy(v); !ok || s.purchase(v, "vest") != "" || v.Money != s.econ().MaxMoney-650 {
		t.Fatalf("warmup: buy in your zone at the real price: money %d", v.Money)
	}
	v.Pos = Vec3{10, 0, 0}
	if ok, _ := s.canBuy(v); ok {
		t.Fatal("warmup: bought outside the buy zone (the menu belongs to the spawn)")
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

// A survivor whose reload is still running when the round ends gets it
// finished at round start: the held gun tops up from its own reserve, nothing
// more (no other gun, no free ammo); a gun not being reloaded stays as it was.
func TestRoundEndFinishesReloadInProgress(t *testing.T) {
	s, v, a := buyServer(t)
	s.purchase(v, "prompt")
	s.purchase(a, "linter")
	vp, ap := v.inv.Slots["primary"], a.inv.Slots["primary"]
	vp.Ammo, vp.Reserve = 7, 90
	v.inv.Slots["sidearm"].Ammo = 3
	ap.Ammo, ap.Reserve = 5, 20
	s.Weapons.reload(v.inv, v.cmdTicks) // v reloads, a doesn't
	if v.inv.reloadEnd == 0 {
		t.Fatal("no reload started")
	}
	s.round.Phase = PhaseOver
	s.startRound(s.clock())
	if vp := v.inv.Slots["primary"]; vp.Ammo != 30 || vp.Reserve != 67 || v.inv.reloadEnd != 0 {
		t.Fatalf("reloading survivor: ammo %d reserve %d reloadEnd %d, want 30/67/0", vp.Ammo, vp.Reserve, v.inv.reloadEnd)
	}
	if v.inv.Slots["sidearm"].Ammo != 3 {
		t.Fatalf("the pistol got reloaded too: %d", v.inv.Slots["sidearm"].Ammo)
	}
	if ap := a.inv.Slots["primary"]; ap.Ammo != 5 || ap.Reserve != 20 {
		t.Fatalf("not reloading: ammo %d reserve %d, want 5/20 unchanged", ap.Ammo, ap.Reserve)
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

func TestBuySunglassesCutFlashBlindness(t *testing.T) {
	s, v, a := buyServer(t)
	if why := s.purchase(v, "shades"); why != "" || !v.Shades || v.Money != 16000-200 {
		t.Fatalf("shades: %q shades %v money %d", why, v.Shades, v.Money)
	}
	if why := s.purchase(v, "shades"); why != "already wearing them" {
		t.Fatalf("second pair: %q", why)
	}
	// The same flash at the same spot, facing it: the sunglasses cut it to 40%.
	blind := func(p *Player) float64 {
		p.flashUntil, p.flashDur = time.Time{}, 0
		p.Pos, p.Yaw, p.Pitch, p.ready = Vec3{-20, 0, 0}, -math.Pi/2, 0, true
		p.body = sim.NewBody(sim.Vec3(p.Pos))
		other := a
		if p == a {
			other = v
		}
		other.Pos = Vec3{-20, 0, 30} // out of the way
		nadeAt(s, v, "stacktrace", Vec3{-17, 1.5, 0}, 1.5)
		s.tickNades(0.01)
		return p.flashLeft(s.clock())
	}
	with, without := blind(v), blind(a)
	if without < 2 || math.Abs(with-without*0.4) > 0.05 {
		t.Fatalf("blind %.2f with sunglasses, %.2f without", with, without)
	}
}

func TestBuyEnergyDrinkBoostsTheNextCommands(t *testing.T) {
	s, v, _ := buyServer(t)
	base := s.heldSpeed(v)
	if why := s.purchase(v, "drink"); why != "" || v.Money != 16000-300 {
		t.Fatalf("drink: %q money %d", why, v.Money)
	}
	if why := s.purchase(v, "drink"); why != "still buzzing" {
		t.Fatalf("a second drink: %q", why)
	}
	if w := weaponStateOf(v); w.Boost != 1000 {
		t.Fatalf("self-snapshot boost %d, want 1000 commands", w.Boost)
	}
	v.cmdTicks++ // the next command
	if got := s.heldSpeed(v); math.Abs(got-base*1.1) > 1e-9 {
		t.Fatalf("boosted speed %v, want %v", got, base*1.1)
	}
	v.cmdTicks += 999 // its last boosted command
	if got := s.heldSpeed(v); math.Abs(got-base*1.1) > 1e-9 {
		t.Fatalf("last boosted command: %v", got)
	}
	v.cmdTicks++
	if got := s.heldSpeed(v); got != base {
		t.Fatalf("worn off: %v, want %v", got, base)
	}
	if why := s.purchase(v, "drink"); why != "" {
		t.Fatalf("another once it wore off: %q", why)
	}
}

func TestSurvivorsKeepSunglassesNotTheDrink(t *testing.T) {
	s, v, a := buyServer(t)
	s.purchase(v, "shades")
	s.purchase(v, "drink")
	s.purchase(a, "shades")
	a.Dead = true
	s.round.Phase = PhaseOver
	s.startRound(s.clock())
	if !v.Shades || a.Shades {
		t.Fatalf("shades: survivor %v, dead %v", v.Shades, a.Shades)
	}
	if w := weaponStateOf(v); w.Boost != 0 {
		t.Fatalf("the drink carried into the next round: %d", w.Boost)
	}
}

// Ammo (CS 1.6's buyammo1 on ",", buyammo2 on "."): refills the gun's reserve
// to a new one's, for the economy's price; full, missing or broke is refused.
func TestBuyAmmoRefillsTheReserve(t *testing.T) {
	s, v, _ := buyServer(t)
	stub := v.inv.Slots["sidearm"]
	if why := s.purchase(v, "ammo2"); why != "ammo is full" || v.Money != 16000 {
		t.Fatalf("full reserve: %q, money %d", why, v.Money)
	}
	if why := s.purchase(v, "ammo1"); why != "no primary to load" {
		t.Fatalf("no primary: %q", why)
	}
	full := stub.Reserve
	stub.Reserve = 3
	if why := s.purchase(v, "ammo2"); why != "" || stub.Reserve != full || v.Money != 16000-s.econ().Ammo["sidearm"] {
		t.Fatalf("refill: %q, reserve %d/%d, money %d", why, stub.Reserve, full, v.Money)
	}
	stub.Reserve, v.Money = 0, 10
	if why := s.purchase(v, "ammo2"); why != "can't afford it" || stub.Reserve != 0 {
		t.Fatalf("broke: %q, reserve %d", why, stub.Reserve)
	}
	if s.econ().Ammo["primary"] <= 0 || s.econ().Ammo["sidearm"] <= 0 {
		t.Fatalf("ammo prices missing: %v", s.econ().Ammo)
	}
}

// The drink's countdown starts with the round: bought in freeze time, the
// frozen commands don't use it up (the HUD shows the full duration), and it
// lasts its full length from the first live command. Bought once the round is
// live, it counts from the purchase.
func TestEnergyDrinkBoughtInFreezeTimeStartsWithTheRound(t *testing.T) {
	s, v, _ := buyServer(t)
	v.body = sim.NewBody(sim.Vec3(v.Pos))
	w := &sim.World{Boxes: s.Map.Boxes}
	seq := 0
	run := func(n int) {
		for range n {
			seq++
			s.runCmd(w, v, Cmd{Seq: seq})
		}
	}
	run(10)
	base := s.heldSpeed(v)
	if why := s.purchase(v, "drink"); why != "" {
		t.Fatalf("drink: %q", why)
	}
	run(500) // still freeze time
	if b := weaponStateOf(v).Boost; b != 1000 {
		t.Fatalf("after 500 frozen commands the boost is %d, want the full 1000", b)
	}
	s.round.Phase = PhaseLive
	run(999)
	if b := weaponStateOf(v).Boost; b != 1 {
		t.Fatalf("999 live commands in: boost %d, want 1", b)
	}
	v.cmdTicks++ // the 1000th live command is still boosted
	if got := s.heldSpeed(v); math.Abs(got-base*1.1) > 1e-9 {
		t.Fatalf("1000th live command: speed %v, want %v", got, base*1.1)
	}
	v.cmdTicks--
	run(1)
	if b := weaponStateOf(v).Boost; b != 0 {
		t.Fatalf("after 1000 live commands: boost %d, want 0", b)
	}

	// Bought mid-round: it counts from the purchase.
	s2, v2, _ := buyServer(t)
	v2.body = sim.NewBody(sim.Vec3(v2.Pos))
	s2.round.Phase = PhaseLive
	if why := s2.purchase(v2, "drink"); why != "" {
		t.Fatalf("drink mid-round: %q", why)
	}
	for i := range 300 {
		s2.runCmd(w, v2, Cmd{Seq: i + 1})
	}
	if b := weaponStateOf(v2).Boost; b != 700 {
		t.Fatalf("bought live, 300 commands later: boost %d, want 700", b)
	}
}

// The Gravity Override: for sale only with mp_gravityboots 1 (the server's or
// the map's); bought, the body wears it with a full meter; a survivor keeps it
// into the next round (fresh meter, back in the world's gravity), the dead don't.
func TestBuyGravityOverride(t *testing.T) {
	s, v, a := buyServer(t)
	v.body, a.body = sim.NewBody(sim.Vec3(v.Pos)), sim.NewBody(sim.Vec3(a.Pos))
	if why := s.purchase(v, "boots"); why != "not on this server (mp_gravityboots)" || v.Boots || v.Money != 16000 {
		t.Fatalf("off: %q boots %v money %d", why, v.Boots, v.Money)
	}
	s.cvars["mp_gravityboots"] = "1"
	if why := s.purchase(v, "boots"); why != "" || !v.Boots || !v.body.Boots || v.body.Energy != sim.DefaultHook().Energy || v.Money != 16000-1000 {
		t.Fatalf("on: %q boots %v body %v energy %v money %d", why, v.Boots, v.body.Boots, v.body.Energy, v.Money)
	}
	if why := s.purchase(v, "boots"); why != "already wearing them" {
		t.Fatalf("second pair: %q", why)
	}
	s.purchase(a, "boots")
	v.body.Energy, v.body.Grav = 3, sim.GravPX // used, on a wall
	a.Dead = true
	s.round.Phase = PhaseOver
	s.startRound(s.clock())
	if !v.Boots || a.Boots {
		t.Fatalf("boots: survivor %v, dead %v", v.Boots, a.Boots)
	}
	if v.body == nil || !v.body.Boots || v.body.Energy != sim.DefaultHook().Energy || v.body.Grav != sim.GravDown {
		t.Fatalf("survivor's new body: %+v", v.body)
	}
	if a.body != nil && a.body.Boots {
		t.Fatal("the dead one's new body wears boots")
	}
	// A map may sell them on a server that doesn't.
	s.cvars["mp_gravityboots"] = "0"
	s.Map.Rules = map[string]string{"mp_gravityboots": "1"}
	if why := s.purchase(a, "boots"); why != "" || !a.Boots {
		t.Fatalf("map rule: %q", why)
	}
}

// Out of the world: below y -20 as ever, and (a body in a gravity of its own)
// past the map's top or sides; the reset body keeps the gear.
func TestOutOfWorldEveryWay(t *testing.T) {
	s, v, _ := buyServer(t)
	lo, hi := MapBounds(s.Map.Boxes)
	mid := sim.Vec3{(lo[0] + hi[0]) / 2, 1, (lo[2] + hi[2]) / 2}
	if s.outOfWorld(mid) {
		t.Fatal("the middle of the map is out")
	}
	for _, p := range []sim.Vec3{{mid[0], -21, mid[2]}, {mid[0], hi[1] + 31, mid[2]}, {lo[0] - 31, 1, mid[2]}, {mid[0], 1, hi[2] + 31}} {
		if !s.outOfWorld(p) {
			t.Fatalf("%v is in", p)
		}
	}
	// Falling "up" off the map, boots on: back at a spawn, boots still on.
	s.cvars["mp_gravityboots"] = "1"
	v.body = sim.NewBody(sim.Vec3(v.Pos))
	s.purchase(v, "boots")
	v.ready = true
	v.body.Pos, v.body.Grav, v.body.OnGround = sim.Vec3{mid[0], hi[1] + 29.9, mid[2]}, sim.GravUp, false
	v.body.Vel = sim.Vec3{0, 30, 0}
	s.runCmd(sim.NewWorld(&s.Map), v, Cmd{Seq: 1})
	if v.body.Grav != sim.GravDown || v.body.Pos[1] > hi[1] || !v.body.Boots {
		t.Fatalf("not reset: %+v", v.body)
	}
}

// A ship's path counts as part of the world: a passenger riding a path mover
// far above the map's boxes is not out of the world (lab_moon's MVP-1).
func TestOutOfWorldCountsMoverPaths(t *testing.T) {
	s := authServer(t)
	s.Map.Movers = append(s.Map.Movers, content.Mover{Kind: content.MoverPath, Mat: content.Metal, Min: [3]float64{0, 0, 0}, Max: [3]float64{4, 1, 4},
		Period: 10, Path: [][4]float64{{0, 0, 0, 0}, {5, 0, 150, 0}}})
	if s.outOfWorld(Vec3{2, 150, 2}) {
		t.Error("a rider at the top of the path is out of the world")
	}
	if !s.outOfWorld(Vec3{2, 400, 2}) {
		t.Error("far over the path is still out of the world")
	}
}
