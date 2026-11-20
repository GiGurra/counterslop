package game

import (
	"os"
	"testing"
	"time"
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
	now := time.Unix(1000, 0)
	w := r.Get("prompt")
	if r.fire(inv, now) == nil {
		t.Fatal("first shot refused")
	}
	if r.fire(inv, now.Add(time.Duration(w.Interval*0.5*float64(time.Second)))) != nil {
		t.Fatal("shot at twice the fire rate accepted")
	}
	for i := 1; i < w.Mag; i++ {
		now = now.Add(time.Second)
		if r.fire(inv, now) == nil {
			t.Fatalf("shot %d refused", i)
		}
	}
	now = now.Add(time.Second)
	if r.fire(inv, now) != nil {
		t.Fatal("fired with an empty mag")
	}
	r.reload(inv, now)
	if r.fire(inv, now.Add(time.Second)) != nil {
		t.Fatal("fired mid-reload")
	}
	now = now.Add(time.Duration((w.Reload + 0.1) * float64(time.Second)))
	if r.fire(inv, now) == nil {
		t.Fatal("refused after reload")
	}
	if h := inv.Slots["primary"]; h.Ammo != w.Mag-1 || h.Reserve != w.Reserve-w.Mag {
		t.Fatalf("ammo after reload: %+v", h)
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
	s.tickItems(time.Now()) // still in the no-pickup window
	if p.inv.Slots["primary"] != nil {
		t.Fatal("picked up own drop instantly")
	}
	s.tickItems(time.Now().Add(2 * time.Second))
	if p.inv.Slots["primary"] == nil || len(s.items) != 0 {
		t.Fatal("no pickup")
	}
}
