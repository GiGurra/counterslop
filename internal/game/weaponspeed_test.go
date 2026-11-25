package game

import (
	"slices"
	"testing"
)

// Every weapon in weapons.json has a weapon_speed_<id> cvar (Server settings:
// Weapon speeds), and every such cvar names a weapon.
func TestWeaponSpeedCvarsCoverRegistry(t *testing.T) {
	r := registry(t)
	ids := []string{}
	for _, w := range r.Weapons {
		ids = append(ids, w.ID)
	}
	a, b := slices.Clone(ids), slices.Clone(speedWeapons)
	slices.Sort(a)
	slices.Sort(b)
	if !slices.Equal(a, b) {
		t.Fatalf("speedWeapons %v, weapons.json %v", b, a)
	}
	for _, id := range ids {
		if defaultCvars[speedCvarPrefix+id] != "0" || cvarHelp[speedCvarPrefix+id][1] == "" {
			t.Errorf("%s: cvar or help missing", id)
		}
	}
}

// GitHub #12: an admin's override changes the held weapon's move speed (and
// the weapon's Speed, which the moving-accuracy rule reads); 0 puts the
// weapons.json value back; values are clamped to 0.3..1.5; junk is refused.
func TestWeaponSpeedOverride(t *testing.T) {
	s := NewServer()
	s.Weapons = registry(t)
	p := s.join("1.1.1.1") // open admin
	p.inv.Current = "primary"
	base := s.Weapons.BaseSpeed("prompt")
	if base == 0 || s.heldSpeed(p) != base {
		t.Fatalf("prompt default %v, held %v", base, s.heldSpeed(p))
	}
	s.adminCommand(p, "weapon_speed_prompt", []string{"0.5"})
	if s.cvars["weapon_speed_prompt"] != "0.5" || s.heldSpeed(p) != 0.5 || s.Weapons.Get("prompt").Speed != 0.5 {
		t.Fatalf("override: cvar %q held %v", s.cvars["weapon_speed_prompt"], s.heldSpeed(p))
	}
	if s.Weapons.Get("linter").Speed != s.Weapons.BaseSpeed("linter") {
		t.Fatal("another weapon changed")
	}
	s.adminCommand(p, "weapon_speed_prompt", []string{"9"})
	if s.cvars["weapon_speed_prompt"] != "1.5" || s.heldSpeed(p) != 1.5 {
		t.Fatalf("clamp high: %q", s.cvars["weapon_speed_prompt"])
	}
	s.adminCommand(p, "weapon_speed_prompt", []string{"0.01"})
	if s.cvars["weapon_speed_prompt"] != "0.3" {
		t.Fatalf("clamp low: %q", s.cvars["weapon_speed_prompt"])
	}
	s.adminCommand(p, "weapon_speed_prompt", []string{"fast"})
	if s.cvars["weapon_speed_prompt"] != "0.3" {
		t.Fatalf("junk taken: %q", s.cvars["weapon_speed_prompt"])
	}
	s.adminCommand(p, "weapon_speed_prompt", []string{"0"})
	if s.cvars["weapon_speed_prompt"] != "0" || s.heldSpeed(p) != base || s.Weapons.Get("prompt").Speed != base {
		t.Fatalf("reset: cvar %q held %v want %v", s.cvars["weapon_speed_prompt"], s.heldSpeed(p), base)
	}
}

func TestRegistrySetSpeed(t *testing.T) {
	r := registry(t)
	if r.SetSpeed("nope", 1) {
		t.Fatal("unknown id accepted")
	}
	base := r.Get("codereview").Speed
	r.SetSpeed("codereview", 1.2)
	if r.Get("codereview").Speed != 1.2 || r.BaseSpeed("codereview") != base {
		t.Fatal("set")
	}
	r.SetSpeed("codereview", 0)
	if r.Get("codereview").Speed != base {
		t.Fatal("reset")
	}
}
