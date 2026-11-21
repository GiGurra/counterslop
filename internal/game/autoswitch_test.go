package game

import "testing"

// The operator's rule: the selected slot changes by itself only when you get a
// primary (buy or pick up) while holding no primary. Everything else (a
// pistol, armor, a kit, grenades, ammo, a primary that replaces your primary)
// leaves your hand alone.
func TestAutoSwitchOnlyForAFirstPrimary(t *testing.T) {
	cases := []struct {
		name    string
		hold    string // slot in hand before
		primary string // primary owned before ("" = none)
		buy     []string
		want    string
	}{
		{"first primary, from the knife", "melee", "", []string{"prompt"}, "primary"},
		{"first primary, from the pistol", "sidearm", "", []string{"codereview"}, "primary"},
		{"a pistol while holding the knife", "melee", "", []string{"hotfix"}, "melee"},
		{"a pistol while holding the primary", "primary", "prompt", []string{"rubberduck"}, "primary"},
		{"a pistol that replaces the held pistol", "sidearm", "", []string{"hotfix"}, "sidearm"},
		{"armor, helmet, drink", "melee", "", []string{"vest", "helmet", "drink"}, "melee"},
		{"a defuse kit (Artisans)", "sidearm", "", []string{"kit"}, "sidearm"},
		{"grenades", "sidearm", "", []string{"mergeconflict", "stacktrace", "smoketest"}, "sidearm"},
		{"ammo", "melee", "prompt", []string{"ammo1", "ammo2"}, "melee"},
		{"a primary that replaces the primary, pistol in hand", "sidearm", "prompt", []string{"codereview"}, "sidearm"},
		{"a primary that replaces the held primary", "primary", "prompt", []string{"codereview"}, "primary"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, v, a := buyServer(t)
			if c.buy[0] == "kit" {
				v = a
			}
			if c.primary != "" {
				v.inv.Slots["primary"] = s.Weapons.newHeld(c.primary)
			}
			for _, h := range v.inv.Slots {
				h.Reserve = 0 // room for ammo
			}
			v.inv.Current = c.hold
			for _, item := range c.buy {
				if why := s.purchase(v, item); why != "" {
					t.Fatalf("buy %s: %s", item, why)
				}
			}
			if v.inv.Current != c.want {
				t.Fatalf("holding %q after buying %v, want %q", v.inv.Current, c.buy, c.want)
			}
		})
	}
}

func TestAutoSwitchOnPickupOnlyForAFirstPrimary(t *testing.T) {
	for _, c := range []struct {
		name, hold, item, want string
		dropPistol             bool
	}{
		{"a primary with none", "melee", "prompt", "primary", false},
		{"a pistol with none, knife in hand", "melee", "hotfix", "melee", true},
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
