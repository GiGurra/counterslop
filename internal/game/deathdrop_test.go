package game

import (
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/sim"
)

// liveRound: a round server with its round live (bodies stay, guns drop).
func liveRound(t *testing.T) (*Server, *Player, *Player, *time.Time) {
	t.Helper()
	s, v, a := roundServer(t)
	s.cvars["mp_respawn"] = "0"
	now := time.Now()
	ticks(s, 2, &now, nil)
	if s.round.Phase != PhaseLive {
		t.Fatalf("phase %q, want live", s.round.Phase)
	}
	return s, v, a, &now
}

func droppedItems(s *Server) []*Item {
	var out []*Item
	for _, it := range s.items {
		if it.mapSlot < 0 {
			out = append(out, it)
		}
	}
	return out
}

func TestDeathDropsThePrimaryWithItsAmmo(t *testing.T) {
	s, v, a, _ := liveRound(t)
	v.inv.Slots["primary"] = &Held{ID: "codereview", Ammo: 3, Reserve: 17}
	v.inv.Current = "sidearm" // not in hand: drops all the same
	s.kill(a, v, s.Weapons.Get("prompt"), false)
	got := droppedItems(s)
	if len(got) != 1 || got[0].Weapon != "codereview" || got[0].held.Ammo != 3 || got[0].held.Reserve != 17 {
		t.Fatalf("dropped %+v, want the Code Review with 3/17", got)
	}
	if d := math.Hypot(got[0].Pos[0]-v.Pos[0], got[0].Pos[2]-v.Pos[2]); d > deathDropToss+1e-9 {
		t.Errorf("dropped %.2f m from the body, want within %.1f", d, deathDropToss)
	}
	if v.inv.Slots["primary"] != nil {
		t.Errorf("the dead still carry the primary")
	}
	if v.inv.Slots["sidearm"] == nil || v.inv.Slots["melee"] == nil {
		t.Errorf("pistol or keyboard dropped too: %v", v.inv.Slots)
	}
}

func TestDeathWithOnlyAPistolDropsNothing(t *testing.T) {
	s, v, a, _ := liveRound(t)
	delete(v.inv.Slots, "primary")
	v.inv.Current = "sidearm"
	s.kill(a, v, s.Weapons.Get("prompt"), false)
	if got := droppedItems(s); len(got) != 0 {
		t.Fatalf("dropped %v, want nothing", got)
	}
}

func TestDeathInWarmupOrDeathmatchDropsNothing(t *testing.T) {
	t.Run("deathmatch", func(t *testing.T) {
		s, v, a, _ := liveRound(t)
		s.cvars["mp_respawn"] = "1"
		v.inv.Slots["primary"] = s.Weapons.newHeld("prompt")
		s.kill(a, v, s.Weapons.Get("prompt"), false)
		if got := droppedItems(s); len(got) != 0 {
			t.Fatalf("dropped %v in deathmatch, want nothing", got)
		}
	})
	t.Run("warmup", func(t *testing.T) {
		s, v, a := roundServer(t)
		s.cvars["mp_respawn"] = "0"
		s.cvars["mp_warmup_time"] = "60"
		s.round = newRound()
		now := time.Now()
		ticks(s, 2, &now, nil)
		if s.round.Phase != PhaseWarmup {
			t.Fatalf("phase %q, want warmup", s.round.Phase)
		}
		v.inv.Slots["primary"] = s.Weapons.newHeld("prompt")
		s.kill(a, v, s.Weapons.Get("prompt"), false)
		if got := droppedItems(s); len(got) != 0 {
			t.Fatalf("dropped %v in warmup, want nothing", got)
		}
	})
}

func TestDeathDropIsPickedUpAndClearedNextRound(t *testing.T) {
	s, v, a, now := liveRound(t)
	v.inv.Slots["primary"] = &Held{ID: "codereview", Ammo: 4, Reserve: 9}
	delete(a.inv.Slots, "primary")
	s.kill(a, v, s.Weapons.Get("prompt"), false)
	it := droppedItems(s)[0]
	a.Pos = it.Pos
	s.tickItems(*now)
	if h := a.inv.Slots["primary"]; h == nil || h.ID != "codereview" || h.Ammo != 4 || h.Reserve != 9 {
		t.Fatalf("picked up %+v, want the Code Review with 4/9", h)
	}
	if len(droppedItems(s)) != 0 {
		t.Fatalf("still on the ground after the pickup")
	}

	// Another death drop, left lying: the next round clears it.
	a.inv.Slots["primary"] = s.Weapons.newHeld("prompt")
	s.cvars["mp_round_restart_delay"] = "0"
	s.kill(v, a, s.Weapons.Get("prompt"), false)
	if len(droppedItems(s)) != 1 {
		t.Fatalf("dropped %v, want the Prompt", droppedOnGround(s))
	}
	s.endRound(*now, v.Team, "elimination")
	ticks(s, 2, now, nil)
	if s.round.Number != 2 {
		t.Fatalf("round %d, want 2", s.round.Number)
	}
	if got := droppedOnGround(s); len(got) != 0 {
		t.Errorf("death drops still on the ground in round 2: %v", got)
	}
}

// dropMap: a floor with a wall at x = 2 and a crate top 1 m up at z 5..7.
func dropMap() content.Map {
	b := mapgen.New("drop")
	b.Add(mapgen.Arena(-20, -20, 20, 20, 5, 1, content.Concrete, content.Brick)...)
	b.Add(content.B(2, 0, -5, 3, 3, 5, content.Brick), content.B(-3, 0, 5, 3, 1, 7, content.Brick))
	return b.Map()
}

func TestDropSpotStaysOutOfWallsAndOnTheFloor(t *testing.T) {
	s := authServer(t)
	s.Map = dropMap()
	// Running into the wall: stops short of it.
	at := s.dropSpot(Vec3{1, 0, 0}, Vec3{8, 0, 0})
	if at[0]+deathDropClear > 2+1e-9 || at[1] != 0 {
		t.Errorf("into a wall: %v, want short of x=2 on the floor", at)
	}
	// Mid-air over the crate: lands on its top, not floating.
	at = s.dropSpot(Vec3{0, 1.2, 6}, Vec3{})
	if at[1] != 1 {
		t.Errorf("mid-air over the crate: %v, want y=1", at)
	}
	// A big velocity: tossed at most deathDropToss.
	at = s.dropSpot(Vec3{-10, 0, -10}, Vec3{0, 0, -50})
	if d := math.Hypot(at[0]+10, at[2]+10); math.Abs(d-deathDropToss) > 1e-9 || at[1] != 0 {
		t.Errorf("fast: %v (%.2f m), want %v m along z on the floor", at, d, deathDropToss)
	}
}

// lootServer: a live round on an open arena with one bot of each team, far
// apart behind a wall (they don't fight).
func lootServer(t *testing.T) (*Server, *Player) {
	t.Helper()
	s := wayServer(t, wayMap(0, 1))
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_respawn"] = "0"
	bot := s.addAI(content.Vibe)
	other := s.addAI(content.Artisan)
	s.Step(3)
	if s.round.Phase != PhaseLive {
		t.Fatalf("phase %q, want live", s.round.Phase)
	}
	for _, p := range []*Player{bot, other} {
		p.Dead, p.HP = false, maxHP
	}
	other.Pos, other.body.Pos = Vec3{15, 0, 15}, sim.Vec3{15, 0, 15}
	bot.Pos, bot.body.Pos = Vec3{-10, 0, -10}, sim.Vec3{-10, 0, -10}
	return s, bot
}

func TestBotPicksUpADroppedPrimaryNearby(t *testing.T) {
	s, bot := lootServer(t)
	delete(bot.inv.Slots, "primary")
	bot.inv.Current = "sidearm"
	s.spawnItem(Held{ID: "codereview", Ammo: 5, Reserve: 10}, Vec3{-10, 0, -6}, -1)
	for range 4 * SimHz {
		s.Step(1)
		if h := bot.inv.Slots["primary"]; h != nil {
			if h.ID != "codereview" {
				t.Fatalf("picked up %q", h.ID)
			}
			return
		}
	}
	t.Fatalf("bot at %v never picked up the Code Review at (-10,-6)", bot.Pos)
}

func TestBotSwapsACheaperPrimaryForABetterOne(t *testing.T) {
	s, bot := lootServer(t)
	bot.inv.Slots["primary"] = s.Weapons.newHeld("autocomplete")
	s.spawnItem(Held{ID: "codereview", Ammo: 5, Reserve: 10}, Vec3{-7, 0, -10}, -1)
	for range 4 * SimHz {
		s.Step(1)
		if h := bot.inv.Slots["primary"]; h.ID == "codereview" {
			if got := droppedOnGround(s); len(got) != 1 || got[0] != "autocomplete" {
				t.Errorf("on the ground %v, want the Autocomplete left there", got)
			}
			return
		}
	}
	t.Fatalf("bot at %v kept its %q", bot.Pos, bot.inv.Slots["primary"].ID)
}

func TestBotIgnoresAWorseOrFarPrimary(t *testing.T) {
	s, bot := lootServer(t)
	bot.inv.Slots["primary"] = s.Weapons.newHeld("codereview")
	s.spawnItem(*s.Weapons.newHeld("autocomplete"), Vec3{-10, 0, -8}, -1)
	if it := s.aiLoot(bot); it != nil {
		t.Errorf("bot with a Code Review wants the Autocomplete")
	}
	delete(bot.inv.Slots, "primary")
	s.items = nil
	s.spawnItem(*s.Weapons.newHeld("prompt"), Vec3{-10, 0, -10 + aiLootRange + 1}, -1)
	if it := s.aiLoot(bot); it != nil {
		t.Errorf("bot detours %v m for a gun", aiLootRange+1)
	}
}

// A gun the map places (de_slop's Code Review) is a player's pickup, not loot:
// bots go only for dropped ones. Taking it re-armed bots that bot_ammo 0 0 had
// left with the keyboard (scenario ai-melee), and the map's copy never came
// back (aiSwap doesn't schedule the respawn).
func TestBotIgnoresAMapPlacedPrimary(t *testing.T) {
	s, bot := lootServer(t)
	delete(bot.inv.Slots, "primary")
	bot.inv.Current = "sidearm"
	s.items = nil
	s.spawnItem(Held{ID: "codereview", Ammo: 5, Reserve: 10}, Vec3{-10, 0, -8}, 0)
	if it := s.aiLoot(bot); it != nil {
		t.Fatalf("bot goes for the map's own %s", it.Weapon)
	}
}
