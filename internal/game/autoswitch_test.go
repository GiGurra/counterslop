package game

import (
	"fmt"
	"testing"
)

// The operator's rule for the hand when a gun is bought or picked up:
//   - any primary goes to hand (a first one or a replacement);
//   - a pistol that replaces your pistol goes to hand, unless your primary is in hand;
//   - nothing else moves it: a first pistol, armor, a helmet, a kit, the drink,
//     grenades, ammo.
func TestAutoSwitchOnBuy(t *testing.T) {
	type buy struct {
		items []string
		want  func(hold string) string // the slot in hand after, given the one before
	}
	same := func(hold string) string { return hold }
	buys := map[string]buy{
		"a first primary":     {[]string{"prompt"}, func(string) string { return "primary" }},
		"a replacing primary": {[]string{"codereview"}, func(string) string { return "primary" }}, // owns the prompt
		"a replacing pistol":  {[]string{"hotfix"}, func(h string) string { return map[bool]string{true: "primary", false: "sidearm"}[h == "primary"] }},
		"armor and a helmet":  {[]string{"vest", "helmet"}, same},
		"the drink":           {[]string{"drink"}, same},
		"a grenade":           {[]string{"smoketest"}, same},
		"ammo for both guns":  {[]string{"ammo1", "ammo2"}, same},
	}
	for _, hold := range []string{"melee", "sidearm", "he", "primary"} {
		for name, b := range buys {
			if hold == "primary" && name == "a first primary" {
				continue // holding a primary means owning one
			}
			t.Run(fmt.Sprintf("%s in hand, %s", hold, name), func(t *testing.T) {
				s, v, _ := buyServer(t)
				v.inv.Slots["he"] = s.Weapons.newHeld("mergeconflict")
				if name != "a first primary" {
					v.inv.Slots["primary"] = s.Weapons.newHeld("prompt")
				}
				for _, h := range v.inv.Slots {
					h.Reserve = 0 // room for ammo
				}
				v.inv.Current = hold
				for _, item := range b.items {
					if why := s.purchase(v, item); why != "" {
						t.Fatalf("buy %s: %s", item, why)
					}
				}
				if want := b.want(hold); v.inv.Current != want {
					t.Fatalf("holding %q after buying %v, want %q", v.inv.Current, b.items, want)
				}
			})
		}
	}
}

func TestAutoSwitchOnKitAndFirstPistol(t *testing.T) {
	s, v, a := buyServer(t)
	a.inv.Current = "melee"
	if why := s.purchase(a, "kit"); why != "" || a.inv.Current != "melee" {
		t.Fatalf("kit: %q, holding %q", why, a.inv.Current)
	}
	delete(v.inv.Slots, "sidearm") // a first pistol (rare: you spawn with one)
	v.inv.Current = "melee"
	if why := s.purchase(v, "hotfix"); why != "" || v.inv.Current != "melee" {
		t.Fatalf("first pistol: %q, holding %q", why, v.inv.Current)
	}
}

func TestAutoSwitchOnPickup(t *testing.T) {
	for _, c := range []struct {
		name, hold, item, want string
		dropPistol             bool
	}{
		{"a primary, knife in hand", "melee", "prompt", "primary", false},
		{"a primary, pistol in hand", "sidearm", "codereview", "primary", false},
		{"a pistol into an empty slot, knife in hand", "melee", "hotfix", "melee", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, v, _ := buyServer(t)
			if c.dropPistol {
				delete(v.inv.Slots, "sidearm")
			}
			v.inv.Current = c.hold
			s.spawnItem(*s.Weapons.newHeld(c.item), v.Pos, -1)
			s.tickItems(s.clock())
			if v.inv.Slots[s.Weapons.Get(c.item).Slot] == nil {
				t.Fatalf("%s not picked up", c.item)
			}
			if v.inv.Current != c.want {
				t.Fatalf("holding %q after picking up %s, want %q", v.inv.Current, c.item, c.want)
			}
		})
	}
}
