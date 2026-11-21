package game

import (
	"encoding/json"
	"os"
	"testing"
)

// TestBalanceBands: the time-to-kill table from the weapon data stays inside
// the target bands (testdata/balance_bands.json); every weapon has bands.
func TestBalanceBands(t *testing.T) {
	b, err := os.ReadFile("testdata/balance_bands.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	bands := map[string]map[string]Band{}
	for w, v := range raw {
		if w == "_doc" {
			continue
		}
		var m map[string]Band
		if err := json.Unmarshal(v, &m); err != nil {
			t.Fatalf("%s: %v", w, err)
		}
		bands[w] = m
	}
	table := BalanceTable(registry(t), PlayerModel.Mult)
	for _, line := range CheckBands(table, bands) {
		t.Error(line)
	}
}

func TestDamageAtFalloff(t *testing.T) {
	w := &Weapon{Damage: 100, Falloff: 0.9}
	if got := w.DamageAt(1, 0); got != 100 {
		t.Fatalf("no falloff at 0 m: %v", got)
	}
	if got := w.DamageAt(1, falloffMetres); got < 89.99 || got > 90.01 {
		t.Fatalf("one falloff step at 12.7 m: %v", got)
	}
	if got := (&Weapon{Damage: 10}).DamageAt(4, 1000); got != 40 {
		t.Fatalf("falloff 0 = none: %v", got)
	}
}

// TestShotsToKillArmor: the table uses the same absorb as a real hit.
func TestShotsToKillArmor(t *testing.T) {
	w := &Weapon{Damage: 36, ArmorPen: 0.775}
	if n := shotsToKill(w, 1, "none", "body", 0); n != 3 {
		t.Fatalf("unarmored body: %d", n)
	}
	if n := shotsToKill(w, 1, "vest", "body", 0); n != 4 {
		t.Fatalf("vest body: %d", n)
	}
	if n := shotsToKill(w, 4, "vest", "head", 0); n != 1 {
		t.Fatalf("vest doesn't cover the head: %d", n)
	}
	if n := shotsToKill(&Weapon{Damage: 10, Range: 2}, 1, "none", "body", 5); n != 0 {
		t.Fatalf("out of range: %d", n)
	}
}
