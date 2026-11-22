package game

import (
	"math"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// meleeDuel: a live round on de_slop, a Vibe bot at from facing an idle
// Artisan player (back turned) at to, with the bot's guns as given (nil: none).
func meleeDuel(t *testing.T, from, to Vec3, primary, sidearm *Held) (*Server, *Player, *Player) {
	t.Helper()
	s := authServer(t)
	s.Map = maps.All["de_slop"]()
	s.cvars["bot_difficulty"] = "3"
	p := s.addAI(content.Vibe)
	q := s.join("10.0.0.2")
	q.ready, q.Team = true, content.Artisan
	s.respawn(q)
	s.Step(1)
	s.round.Phase, s.round.Number = PhaseLive, 1
	p.ai.round, p.ai.site = 1, "A"
	yaw := math.Atan2(-(to[0] - from[0]), -(to[2] - from[2]))
	s.teleport(p, from, yaw, false, p.lastQueued)
	s.teleport(q, to, yaw, false, q.lastQueued)
	p.ai.yaw = yaw
	p.inv.Slots["primary"], p.inv.Slots["sidearm"] = primary, sidearm
	p.inv.Current = "primary"
	if primary == nil {
		p.inv.Current = "melee"
	}
	if _, ok := s.visible(p, q, math.Pi); !ok {
		t.Fatalf("%v can't see %v on de_slop", from, to)
	}
	return s, p, q
}

var (
	duelFrom = Vec3{-30, 0, 8}
	duelTo   = Vec3{-30, 0, -2}
)

// Every gun dry (no mag, no reserve) and an enemy at 10 m: the bot draws the
// keyboard, runs at them and lands a hit. It used to press reload forever.
func TestAIOutOfAmmoMeleesTheEnemy(t *testing.T) {
	s, p, q := meleeDuel(t, duelFrom, duelTo, &Held{ID: "prompt"}, &Held{ID: "stub"})
	for i := range 5 * SimHz {
		s.Step(1)
		if q.HP < maxHP {
			if p.inv.Current != "melee" {
				t.Fatalf("hit with %s", p.inv.Current)
			}
			t.Logf("keyboard hit after %.2f s", float64(i)/SimHz)
			return
		}
	}
	t.Fatalf("no hit in 5 s: bot holds %s at %v (enemy at %v)", p.inv.Current, p.Pos, q.Pos)
}

// A dry mag with reserve left is still a gun: reload, don't charge.
func TestAIWithReserveReloadsInsteadOfMelee(t *testing.T) {
	s, p, q := meleeDuel(t, duelFrom, duelTo, &Held{ID: "prompt", Reserve: 90}, &Held{ID: "stub"})
	for range 4 * SimHz {
		s.Step(1)
		if p.inv.Current == "melee" {
			t.Fatal("drew the keyboard with reserve ammo left")
		}
	}
	if h := p.inv.Slots["primary"]; h.Ammo == 0 {
		t.Fatalf("never reloaded: %+v", *h)
	}
	if q.HP == maxHP {
		t.Fatal("reloaded but never shot the enemy")
	}
}

// Keyboard in hand and an enemy out of reach: the bot closes in instead of
// standing there swinging at the air.
func TestAIWithKeyboardDoesNotSwingFromAfar(t *testing.T) {
	far := Vec3{-30, 0, -8} // 16 m: used to stand, crouch and "fire"
	s, p, _ := meleeDuel(t, duelFrom, far, nil, nil)
	start := math.Hypot(far[0]-p.Pos[0], far[2]-p.Pos[2])
	for range SimHz {
		s.Step(1)
		d := math.Hypot(far[0]-p.Pos[0], far[2]-p.Pos[2])
		if p.lastCmd.B&BtnFire != 0 && d > 3 {
			t.Fatalf("swung the keyboard %.1f m away", d)
		}
	}
	if d := math.Hypot(far[0]-p.Pos[0], far[2]-p.Pos[2]); d > start-3 {
		t.Fatalf("didn't close in: %.1f m -> %.1f m", start, d)
	}
}

// With no enemy in sight, a bot holding the keyboard (or a dry gun) puts
// a gun with ammo back in hand and goes on with its objective.
func TestAIArmsTheBestWeaponWhenSafe(t *testing.T) {
	s, p, q := meleeDuel(t, duelFrom, duelTo, &Held{ID: "prompt", Ammo: 30}, &Held{ID: "stub"})
	q.Dead = true
	p.inv.Current = "melee"
	s.Step(SimHz)
	if p.inv.Current != "primary" {
		t.Fatalf("holding %s, want the loaded primary", p.inv.Current)
	}
	if aiGunSlot(&Player{inv: &Inventory{Slots: map[string]*Held{"primary": {ID: "prompt"}, "sidearm": {ID: "stub", Reserve: 1}}}}) != "sidearm" {
		t.Fatal("a dry primary and a sidearm with reserve: want the sidearm")
	}
}

// The straight run is blocked (the body would clip a crate's corner the bot
// can see past): it takes the nav route instead and still lands a hit.
func TestAIChargeRoutesAroundAnObstacle(t *testing.T) {
	from, to := Vec3{-1.8, 0, 22}, Vec3{-1.8, 0, 10} // past the crate at x -4..-2, z 16..18
	s, p, q := meleeDuel(t, from, to, nil, nil)
	if s.segmentClear(from, to, 0.3) {
		t.Fatal("the straight run is clear: no obstacle to route around")
	}
	routed := false
	for range 8 * SimHz {
		s.Step(1)
		routed = routed || strings.HasPrefix(p.ai.goalKey, "melee:")
		if q.HP < maxHP {
			break
		}
	}
	if !routed || q.HP == maxHP {
		t.Fatalf("routed %v, enemy HP %d, bot at %v", routed, q.HP, p.Pos)
	}
}

// Keyboard only and the enemy beyond the charge range: no swinging, no
// standing there: the bot keeps to its objective.
func TestAIKeyboardIgnoresAFarEnemy(t *testing.T) {
	from, to := Vec3{-30, 0, 28}, Vec3{-30, 0, -2} // 30 m
	s, p, _ := meleeDuel(t, from, to, nil, nil)
	for range SimHz {
		s.Step(1)
		if p.lastCmd.B&BtnFire != 0 {
			t.Fatal("swung at an enemy 30 m away")
		}
	}
	if math.Hypot(p.Pos[0]-from[0], p.Pos[2]-from[2]) < 2 {
		t.Fatalf("stood still at %v", p.Pos)
	}
	if s.aiArm(&Player{inv: &Inventory{Slots: map[string]*Held{}}}, &Cmd{}) {
		t.Fatal("no weapons at all: no keyboard either")
	}
}
