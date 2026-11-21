package game

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

func TestBoltActionCyclesThenCoolsDown(t *testing.T) {
	r := registry(t)
	inv := r.Loadout("")
	inv.Slots["primary"] = r.newHeld("codereview")
	w := r.Get("codereview")
	tick := int64(1000)
	if got, why := r.fire(inv, tick); got == nil {
		t.Fatalf("first shot: %s", why)
	}
	cycle, interval := Ticks(w.Cycle), Ticks(w.Interval)
	if cycle >= interval {
		t.Fatalf("test assumes cycle (%d) < interval (%d)", cycle, interval)
	}
	for _, tc := range []struct {
		at      int64
		release bool
		want    string // "" = fires
	}{
		{tick + 1, false, "cycling"},
		{tick + cycle - 1, true, "cycling"}, // releasing doesn't skip the bolt
		{tick + cycle, false, "cooldown"},   // bolt worked, but the interval isn't over
		{tick + cycle + 1, false, "semi"},   // that press was used
		{tick + interval - 1, true, "cooldown"},
		{tick + interval, true, ""},
		{tick + interval + 1, false, "cycling"},
	} {
		if tc.release {
			inv.release()
		}
		got, why := r.fire(inv, tc.at)
		if (got == nil) == (tc.want == "") || why != tc.want {
			t.Fatalf("tick +%d: %v %q, want %q", tc.at-tick, got != nil, why, tc.want)
		}
	}
	if inv.Slots["primary"].Ammo != w.Mag-2 {
		t.Fatalf("ammo %d", inv.Slots["primary"].Ammo)
	}
}

func TestMeleeNeverRunsOut(t *testing.T) {
	r := registry(t)
	inv := r.Loadout("")
	r.switchTo(inv, "melee", 0)
	w := r.Get("keyboard")
	tick := int64(DeployTicks)
	for i := range 50 {
		if got, why := r.fire(inv, tick); got == nil {
			t.Fatalf("swing %d: %s", i, why)
		}
		tick += Ticks(w.Interval)
	}
	r.reload(inv, tick) // nothing to reload
	if inv.reloadEnd != 0 || inv.Slots["melee"].Ammo != 0 {
		t.Fatalf("melee reload: end %d ammo %d", inv.reloadEnd, inv.Slots["melee"].Ammo)
	}
}

func TestReloadNoOps(t *testing.T) {
	r := registry(t)
	for _, tc := range []struct {
		name  string
		setup func(inv *Inventory)
		want  bool // a reload starts
	}{
		{"full mag", func(*Inventory) {}, false},
		{"partial mag", func(inv *Inventory) { inv.Slots["primary"].Ammo = 3 }, true},
		{"no reserve", func(inv *Inventory) { inv.Slots["primary"].Ammo, inv.Slots["primary"].Reserve = 3, 0 }, false},
		{"no weapon in hand", func(inv *Inventory) { delete(inv.Slots, "primary") }, false},
	} {
		inv := r.Loadout("vibe")
		tc.setup(inv)
		r.reload(inv, 100)
		if started := inv.reloadEnd != 0; started != tc.want {
			t.Errorf("%s: reload started=%v", tc.name, started)
		}
	}
	// A second R mid-reload doesn't restart it.
	inv := r.Loadout("vibe")
	inv.Slots["primary"].Ammo = 0
	r.reload(inv, 100)
	end := inv.reloadEnd
	r.reload(inv, 150)
	if inv.reloadEnd != end {
		t.Fatalf("reload restarted: %d -> %d", end, inv.reloadEnd)
	}
	// Partial reserve: take what's left.
	inv.Slots["primary"].Reserve = 7
	r.settle(inv, end)
	if h := inv.Slots["primary"]; h.Ammo != 7 || h.Reserve != 0 {
		t.Fatalf("partial reserve: %+v", h)
	}
	// Switching away cancels a reload.
	inv.Slots["primary"].Ammo, inv.Slots["primary"].Reserve = 0, 30
	r.reload(inv, 200)
	r.switchTo(inv, "sidearm", 210)
	if inv.reloadEnd != 0 || inv.nextShot != 210+DeployTicks {
		t.Fatalf("after switch: reloadEnd %d nextShot %d", inv.reloadEnd, inv.nextShot)
	}
	if r.switchTo(inv, "sidearm", 220) || r.switchTo(inv, "grenade", 220) {
		t.Fatal("switch to the current or a missing slot")
	}
	// A reload that ends after the weapon was dropped settles into nothing.
	inv.reloadEnd = 300
	delete(inv.Slots, "sidearm")
	r.settle(inv, 300)
	if _, why := r.fire(inv, 301); why != "no weapon" || inv.reloadEnd != 0 {
		t.Fatalf("empty hands: %q end %d", why, inv.reloadEnd)
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := LoadWeapons([]byte("{")); err == nil {
		t.Fatal("bad weapons.json accepted")
	}
	r := registry(t)
	if r.Get("bfg") != nil || r.newHeld("bfg") != nil {
		t.Fatal("unknown weapon")
	}
	if Ticks(0.1) != 10 || Ticks(0.101) != 11 || Ticks(1.45) != 145 || Ticks(0) != 0 {
		t.Fatalf("Ticks: %d %d %d", Ticks(0.1), Ticks(0.101), Ticks(1.45))
	}
	before := len(PlayerModel.Parts)
	for _, b := range []string{"{", `{"parts": []}`, `{"parts": [{"team": 5}]}`} {
		if err := LoadModel([]byte(b)); err == nil {
			t.Errorf("LoadModel(%s) accepted", b)
		}
	}
	if len(PlayerModel.Parts) != before {
		t.Fatal("a failed LoadModel replaced the model")
	}
}

func TestTeamSet(t *testing.T) {
	for _, tc := range []struct {
		json  string
		team  string
		match bool
	}{
		{`"vibe"`, "vibe", true},
		{`"vibe"`, "artisan", false},
		{`["", "spec"]`, "", true},
		{`["", "spec"]`, "spec", true},
		{`["", "spec"]`, "vibe", false},
	} {
		var ts teamSet
		if err := json.Unmarshal([]byte(tc.json), &ts); err != nil {
			t.Fatal(err)
		}
		if ts.matches(tc.team) != tc.match {
			t.Errorf("%s matches %q: %v", tc.json, tc.team, !tc.match)
		}
	}
	if (teamSet(nil)).matches("anything") != true {
		t.Fatal("absent team set should match everyone")
	}
}

func TestLerpAngle(t *testing.T) {
	for _, tc := range []struct{ a, b, k, want float64 }{
		{0, 1, 0.5, 0.5},
		{3, -3, 1, 3 + (2*math.Pi - 6)},
		{-3, 3, 1, -3 - (2*math.Pi - 6)},
		{0, 4 * math.Pi, 0.5, 0},
		{0, -4 * math.Pi, 0.5, 0},
	} {
		if got := lerpAngle(tc.a, tc.b, tc.k); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("lerpAngle(%v, %v, %v) = %v, want %v", tc.a, tc.b, tc.k, got, tc.want)
		}
	}
}

func TestItemsCapAndMapRespawn(t *testing.T) {
	s := authServer(t)
	for i := range maxItems + 5 {
		s.spawnItem(*s.Weapons.newHeld("stub"), Vec3{float64(i), 0, 30}, -1)
	}
	if len(s.items) != maxItems || s.items[0].ID != 6 {
		t.Fatalf("%d items, oldest id %d", len(s.items), s.items[0].ID)
	}
	s.items = nil
	// The map's Code Review: placed when due, taken, back after mapItemRespawn.
	now := time.Now()
	s.itemRespawn[0] = time.Time{}
	s.tickItems(now)
	if len(s.items) != 1 {
		t.Fatalf("map item not placed: %d", len(s.items))
	}
	p := s.join("1.1.1.1")
	p.Team = content.Vibe
	delete(p.inv.Slots, "primary")
	bot := s.addBot()
	dead := s.join("2.2.2.2")
	delete(dead.inv.Slots, "primary")
	dead.Dead = true
	bot.Pos, dead.Pos = s.items[0].Pos, s.items[0].Pos
	s.tickItems(now)
	if len(s.items) != 1 {
		t.Fatal("a bot or a dead player picked up the item")
	}
	p.Pos = s.items[0].Pos
	p.Pos[1] += 2 // too far above it
	s.tickItems(now)
	if len(s.items) != 1 {
		t.Fatal("picked up from 2 m above")
	}
	p.Pos[1] -= 2
	s.tickItems(now)
	if h := p.inv.Slots["primary"]; h == nil || h.ID != "codereview" || p.inv.Current != "primary" || len(s.items) != 0 {
		t.Fatalf("pickup: %+v items %d", h, len(s.items))
	}
	if at := s.itemRespawn[0]; !at.Equal(now.Add(mapItemRespawn)) {
		t.Fatalf("respawn at %v, want %v", at, now.Add(mapItemRespawn))
	}
	s.tickItems(now.Add(mapItemRespawn - time.Millisecond))
	if len(s.items) != 0 {
		t.Fatal("respawned early")
	}
	s.tickItems(now.Add(mapItemRespawn + time.Millisecond))
	if len(s.items) != 1 || s.items[0].mapSlot != 0 {
		t.Fatal("map item didn't come back")
	}
	// Unknown weapons on the floor are never picked up.
	s.items = []*Item{{ID: 99, Weapon: "bfg", Pos: p.Pos, mapSlot: -1}}
	s.tickItems(now)
	if len(s.items) != 1 {
		t.Fatal("unknown weapon picked up")
	}
}

// The thrower can grab their own weapon again by stepping away and back.
func TestThrowGuardClearsWhenSteppingAway(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	s.drop(p)
	it := s.items[0]
	p.Pos = it.Pos
	now := time.Now()
	s.tickItems(now) // near: guard holds
	p.Pos = Vec3{it.Pos[0] + 5, it.Pos[1], it.Pos[2]}
	s.tickItems(now) // stepped away: guard cleared
	if it.thrownBy != 0 {
		t.Fatal("guard not cleared after stepping away")
	}
	p.Pos = it.Pos
	s.tickItems(now)
	if p.inv.Slots["primary"] == nil {
		t.Fatal("couldn't re-grab after stepping away and back")
	}
	// Melee can't be dropped; neither can empty hands.
	p.inv.Current = "melee"
	s.drop(p)
	p.inv.Current = "nothing"
	s.drop(p)
	if len(s.items) != 0 || p.inv.Slots["melee"] == nil {
		t.Fatalf("dropped melee or nothing: %d items", len(s.items))
	}
}

// drop throws the weapon 1.4 m along the view and draws the next slot.
func TestDropPlacesInFrontAndDrawsNext(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	p.Pos, p.Yaw = Vec3{0, 0, 20}, math.Pi/2 // facing -X
	p.cmdTicks = 500
	msgs(p)
	s.drop(p)
	it := s.items[0]
	if math.Abs(it.Pos[0]+1.4) > 1e-9 || math.Abs(it.Pos[2]-20) > 1e-9 || it.held.Ammo != 30 {
		t.Fatalf("item %+v", it)
	}
	if p.inv.Current != "sidearm" || p.inv.nextShot != 500+DeployTicks || len(ofType(p, "inv")) != 1 {
		t.Fatalf("after drop: cur %s nextShot %d", p.inv.Current, p.inv.nextShot)
	}
}
