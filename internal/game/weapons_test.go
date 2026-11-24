package game

import (
	"os"
	"testing"
)

func registry(t *testing.T) *Registry {
	t.Helper()
	b, err := os.ReadFile("../../web/assets/weapons/weapons.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := LoadWeapons(b)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRegistryLoads(t *testing.T) {
	r := registry(t)
	for _, id := range []string{"stub", "prompt", "linter", "codereview", "keyboard"} {
		w := r.Get(id)
		if w == nil || w.Damage <= 0 || w.Interval <= 0 || w.Range <= 0 {
			t.Fatalf("%s: bad entry %+v", id, w)
		}
	}
	if inv := r.Loadout("artisan"); inv.Slots["primary"].ID != "linter" || inv.Slots["sidearm"].ID != "stub" || inv.Slots["melee"].ID != "keyboard" {
		t.Fatalf("artisan loadout: %+v", inv.Slots)
	}
}

func TestFireRateAmmoReload(t *testing.T) {
	r := registry(t)
	inv := r.Loadout("vibe")
	tick := int64(1000)
	w := r.Get("prompt")
	if w.Action != "auto" {
		t.Fatalf("prompt action %q", w.Action)
	}
	if got, _ := r.fire(inv, tick); got == nil {
		t.Fatal("first shot refused")
	}
	if _, why := r.fire(inv, tick+Ticks(w.Interval)-1); why != "cooldown" {
		t.Fatalf("shot one tick early: %q, want cooldown", why)
	}
	for i := 1; i < w.Mag; i++ {
		tick += Ticks(w.Interval)
		if got, why := r.fire(inv, tick); got == nil {
			t.Fatalf("shot %d refused: %s", i, why)
		}
	}
	tick += 100
	if _, why := r.fire(inv, tick); why != "empty" {
		t.Fatalf("empty mag: %q", why)
	}
	r.reload(inv, tick)
	if _, why := r.fire(inv, tick+Ticks(w.Reload)-1); why != "reloading" {
		t.Fatalf("one tick before the reload ends: %q", why)
	}
	if got, why := r.fire(inv, tick+Ticks(w.Reload)); got == nil {
		t.Fatalf("refused the tick the reload ends: %s", why)
	}
	if h := inv.Slots["primary"]; h.Ammo != w.Mag-1 || h.Reserve != w.Reserve-w.Mag {
		t.Fatalf("ammo after reload: %+v", h)
	}
}

func TestSemiAutoAndDeploy(t *testing.T) {
	r := registry(t)
	inv := r.Loadout("vibe")
	tick := int64(1000)
	if !r.switchTo(inv, "sidearm", tick) {
		t.Fatal("no sidearm")
	}
	w := r.Get(inv.Slots["sidearm"].ID)
	if w.Action == "auto" {
		t.Skipf("sidearm %s is automatic", w.ID)
	}
	if _, why := r.fire(inv, tick+DeployTicks-1); why != "cooldown" {
		t.Fatalf("fired while drawing: %q", why)
	}
	inv.release()
	tick += DeployTicks
	if got, why := r.fire(inv, tick); got == nil {
		t.Fatalf("first press refused: %s", why)
	}
	if _, why := r.fire(inv, tick+100); why != "semi" {
		t.Fatalf("held trigger fired again: %q", why)
	}
	inv.release()
	if got, why := r.fire(inv, tick+101); got == nil {
		t.Fatalf("second press refused: %s", why)
	}
}

func TestDropAndPickup(t *testing.T) {
	s := NewServer()
	s.Weapons = registry(t)
	p := s.join("1.2.3.4")
	s.drop(p)
	if p.inv.Slots["primary"] != nil || p.inv.Current != "sidearm" || len(s.items) != 1 {
		t.Fatalf("after drop: cur=%s items=%d", p.inv.Current, len(s.items))
	}
	it := s.items[0]
	p.Pos = it.Pos
	s.tickItems(s.clock()) // thrower, inside the guard
	if p.inv.Slots["primary"] != nil {
		t.Fatal("picked up own drop instantly")
	}
	// Someone else can take it at once (hand-over).
	q := s.join("5.6.7.8")
	delete(q.inv.Slots, "primary")
	q.Pos = it.Pos
	s.tickItems(s.clock())
	if q.inv.Slots["primary"] == nil || q.inv.Slots["primary"].ID != "prompt" || len(s.items) != 0 {
		t.Fatal("teammate couldn't grab the thrown weapon at once")
	}
}

func TestThrowGuardExpires(t *testing.T) {
	s := NewServer()
	s.Weapons = registry(t)
	p := s.join("1.2.3.4")
	s.drop(p)
	p.Pos = s.items[0].Pos
	later := s.clock().Add(2 * throwGuard)
	s.tickItems(later) // guard over (cleared this tick)
	s.tickItems(later)
	if p.inv.Slots["primary"] == nil {
		t.Fatal("no re-pickup after the guard")
	}
}

func TestDropOntoAnotherWeaponSwaps(t *testing.T) {
	s := NewServer()
	s.Weapons = registry(t)
	p := s.join("1.2.3.4")
	review := s.Weapons.newHeld("codereview")
	s.spawnItem(*review, p.Pos, -1) // standing on a Code Review with a full primary slot
	s.tickItems(s.clock())
	if p.inv.Slots["primary"].ID != "prompt" {
		t.Fatal("picked up with the slot full")
	}
	s.drop(p) // throw the Prompt: the Code Review is picked up immediately
	if h := p.inv.Slots["primary"]; h == nil || h.ID != "codereview" || p.inv.Current != "primary" {
		t.Fatalf("after G on a pickup: %+v cur=%s", h, p.inv.Current)
	}
	if len(s.items) != 1 || s.items[0].Weapon != "prompt" {
		t.Fatalf("thrown Prompt should lie on the floor: %+v", s.items)
	}
}
