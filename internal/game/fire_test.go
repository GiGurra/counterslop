package game

import (
	"math"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// fireServer: an open floor (de_slop's west side) with alice (vibe) and bob
// (artisan) ready, alice's Hot Reload flying at `at` (dropping).
func fireServer(t *testing.T, at Vec3) (*Server, *Player, *Player, *Nade) {
	t.Helper()
	s := authServer(t)
	a := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-28, 0, 0})
	b := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-28, 0, 6})
	n := nadeAt(s, a, "hotreload", at, 0)
	n.resting, n.Vel = false, Vec3{0, -2, 0}
	return s, a, b, n
}

// It bursts where it first touches the floor: a pool of flames on a 0.7 m
// grid within 3 m, on the floor, and burns 7 s.
func TestFireBurstsOnTheFloor(t *testing.T) {
	s, _, _, n := fireServer(t, Vec3{-24, 1, 0})
	for i := 0; i < 100 && n.State != "fire"; i++ {
		s.tickNades(0.01)
	}
	if n.State != "fire" || n.age > 0.6 {
		t.Fatalf("no burst on landing: %+v (age %.2f)", n.State, n.age)
	}
	if len(n.Flames) < 50 || len(n.Flames) > 70 {
		t.Fatalf("%d flames, want a full 3 m pool (~57)", len(n.Flames))
	}
	for _, f := range n.Flames {
		if math.Abs(f[1]) > 1e-9 || math.Hypot(f[0]+24, f[2]) > 3 {
			t.Fatalf("a flame off the floor or out of reach: %v", f)
		}
	}
	for range 690 {
		s.tickNades(0.01)
	}
	if len(s.nades) != 1 {
		t.Fatalf("burnt out early: %+v", s.nades)
	}
	for range 20 {
		s.tickNades(0.01)
	}
	if len(s.nades) != 0 {
		t.Fatalf("still burning after 7 s: left %.2f", n.Left)
	}
}

// Flying for its whole fuse (2 s): it bursts in the air and the flames fall to
// the floor below; with no floor within 4 m it fizzles ("out").
func TestFireAirburst(t *testing.T) {
	s, _, _, n := fireServer(t, Vec3{-24, 2, 0})
	n.resting, n.age = true, 2 // hanging at 2 m: the fuse is up
	s.tickNades(0.01)
	if n.State != "fire" || n.Pos[1] != 0 {
		t.Fatalf("airburst at 2 m: %+v", n)
	}
	s, a, _, n := fireServer(t, Vec3{-24, 9, 0})
	n.resting, n.age = true, 2
	drain(a)
	s.tickNades(0.01)
	if len(s.nades) != 0 || !strings.Contains(drain(a), `"ev":"out"`) {
		t.Fatalf("9 m up should fizzle: %+v", s.nades)
	}
}

// Walls stop the spread: a brick wall 1 m east of the burst keeps every flame
// on its own side.
func TestFireWallsStopIt(t *testing.T) {
	s, _, _, n := fireServer(t, Vec3{-24, 0.5, 0})
	s.Map.Boxes = append(s.Map.Boxes, content.B(-23, 0, -5, -22.6, 3, 5, content.Brick))
	for i := 0; i < 100 && n.State != "fire"; i++ {
		s.tickNades(0.01)
	}
	if n.State != "fire" {
		t.Fatal("no fire")
	}
	west := 0
	for _, f := range n.Flames {
		if f[0] > -23 {
			t.Fatalf("a flame through the wall: %v", f)
		}
		west++
	}
	if west < 20 {
		t.Fatalf("only %d flames on the burst's side", west)
	}
}

// Standing in it: ~40 damage a second in 8-point pieces (armor or not), the
// thrower too; a teammate (mp_friendlyfire 0) and anyone outside take none;
// a low player dies to it, credited to the thrower's grenade.
func TestFireBurns(t *testing.T) {
	s, a, b, n := fireServer(t, Vec3{-24, 0.5, 0})
	c := cmdPlayer(t, s, "3.3.3.3", content.Vibe, Vec3{-24, 0, 1})
	for i := 0; i < 100 && n.State != "fire"; i++ {
		s.tickNades(0.01)
	}
	a.Pos, b.Pos, c.Pos = Vec3{-24.3, 0, 0.2}, Vec3{-23.6, 0, -0.4}, Vec3{-24, 0, 1}
	b.Armor = 100
	out := cmdPlayer(t, s, "4.4.4.4", content.Artisan, Vec3{-24, 0, 4})
	for range 100 {
		s.tickNades(0.01)
	}
	if b.HP != maxHP-40 || a.HP != maxHP-40 {
		t.Fatalf("a second in the fire: bob %d, alice (thrower) %d; want %d", b.HP, a.HP, maxHP-40)
	}
	if c.HP != maxHP || out.HP != maxHP {
		t.Fatalf("teammate %d, outside %d: want untouched", c.HP, out.HP)
	}
	b.HP = 10
	drain(a)
	for range 40 {
		s.tickNades(0.01)
	}
	if !b.Dead || !strings.Contains(drain(a), `"hotreload"`) {
		t.Fatalf("bob should have burned to death by the Hot Reload: dead %v hp %d", b.Dead, b.HP)
	}
}

// A Smoke Test puts it out: one popping over the pool takes the flames under
// its cloud (all of them here: the pool is gone); a fire bursting inside a
// standing smoke is out at once.
func TestSmokePutsOutFire(t *testing.T) {
	s, a, _, n := fireServer(t, Vec3{-24, 0.5, 0})
	for i := 0; i < 100 && n.State != "fire"; i++ {
		s.tickNades(0.01)
	}
	sm := nadeAt(s, a, "smoketest", Vec3{-24, 0, 1}, 1) // resting, old enough: pops on the next tick
	drain(a)
	s.tickNades(0.01)
	if sm.State != "smoke" || len(n.Flames) != 0 || !strings.Contains(drain(a), `"ev":"out"`) {
		t.Fatalf("smoke over the fire: %d flames left", len(n.Flames))
	}
	s.tickNades(0.01)
	for _, x := range s.nades {
		if x.State == "fire" {
			t.Fatal("the pool still stands")
		}
	}
	// A second fire into the standing smoke.
	n2 := nadeAt(s, a, "hotreload", Vec3{-23, 0.5, 0}, 0)
	n2.resting, n2.Vel = false, Vec3{0, -2, 0}
	for range 60 {
		s.tickNades(0.01)
	}
	if n2.State == "fire" || len(n2.Flames) != 0 {
		t.Fatalf("a fire inside a smoke: %+v", n2)
	}
	// Half under a smoke: only the covered half goes.
	s2, a2, _, f := fireServer(t, Vec3{-24, 0.5, 0})
	for i := 0; i < 100 && f.State != "fire"; i++ {
		s2.tickNades(0.01)
	}
	before := len(f.Flames)
	nadeAt(s2, a2, "smoketest", Vec3{-24 + 6.5, 0, 0}, 1) // its 4.5 m cloud reaches 2 m into the pool
	s2.tickNades(0.01)
	if len(f.Flames) == 0 || len(f.Flames) >= before {
		t.Fatalf("half covered: %d of %d flames left", len(f.Flames), before)
	}
}

// Buying: Hot Reload is the Vibe Coders' ($400), Burndown Chart the
// Artisans' ($500); one at a time, inside the four-grenade limit.
func TestFireGrenadeBuy(t *testing.T) {
	s := authServer(t)
	v := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-28, 0, 0})
	art := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-28, 0, 6})
	for _, p := range []*Player{v, art} {
		p.Money = 16000
	}
	if why := s.buyGrenade(v, s.Weapons.Get("hotreload")); why != "" || v.Money != 16000-400 {
		t.Fatalf("vibe hot reload: %q, money %d", why, v.Money)
	}
	if why := s.buyGrenade(v, s.Weapons.Get("hotreload")); why == "" {
		t.Fatal("a second Hot Reload")
	}
	if why := s.buyGrenade(art, s.Weapons.Get("burndown")); why != "" || art.Money != 16000-500 {
		t.Fatalf("artisan burndown: %q", why)
	}
	for _, id := range []string{"mergeconflict", "stacktrace", "stacktrace"} {
		if why := s.buyGrenade(v, s.Weapons.Get(id)); why != "" {
			t.Fatalf("%s: %s", id, why)
		}
	}
	if why := s.buyGrenade(v, s.Weapons.Get("smoketest")); why == "" {
		t.Fatal("a fifth grenade")
	}
	if w := s.Weapons.Get("hotreload"); w.Team == nil || *w.Team != content.Vibe || s.Weapons.Get("burndown").Team == nil || *s.Weapons.Get("burndown").Team != content.Artisan {
		t.Fatal("team-only")
	}
}

// Bots keep out of fire: about to walk into a pool they stop; standing in one
// they walk out, away from its middle.
func TestBotsAvoidFire(t *testing.T) {
	s, a, _, n := fireServer(t, Vec3{-24, 0.5, 0})
	for i := 0; i < 100 && n.State != "fire"; i++ {
		s.tickNades(0.01)
	}
	bot := s.addAI(content.Artisan)
	bot.body.Pos = [3]float64{-24, 0, -4.2} // 1.2 m off the pool's south edge, facing it (-z is yaw 0: north is +z → yaw π)
	bot.ai.yaw = math.Pi
	c := Cmd{F: 1}
	s.aiAvoidFire(bot, &c)
	if c.F != 0 || c.R != 0 {
		t.Fatalf("walked on into the fire: %+v", c)
	}
	c = Cmd{F: 1}
	bot.ai.yaw = 0 // walking away from it
	s.aiAvoidFire(bot, &c)
	if c.F != 1 {
		t.Fatalf("stopped walking away from it: %+v", c)
	}
	bot.body.Pos = [3]float64{-24, 0, -1} // in it, south of the middle
	c = Cmd{}
	s.aiAvoidFire(bot, &c)
	sy, cy := math.Sin(bot.ai.yaw), math.Cos(bot.ai.yaw)
	if dz := -cy*c.F - sy*c.R; dz > -0.9 {
		t.Fatalf("should walk out south: %+v (dz %.2f)", c, dz)
	}
	_ = a
}
