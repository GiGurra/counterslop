package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

func penBodies(ts ...float64) []bodyHit {
	var out []bodyHit
	for i, t := range ts {
		out = append(out, bodyHit{q: &Player{ID: i + 1}, part: Hitbox{Part: "body", Mult: 1}, t: t})
	}
	return out
}

// Nearest first whatever the input order; each body takes what is left
// (× its part's multiplier), the rest goes on × penetration.
func TestPenetrateOrderAndDamage(t *testing.T) {
	w := &Weapon{Damage: 100, Pen: 0.5}
	b := penBodies(10, 3, 6)
	b[0].part.Mult = 1.25 // the far one: legs-ish
	b[2].part.Mult = 4    // the middle one: a head
	steps, stopped := penetrate(w, b, nil)
	if !stopped || len(steps) != 3 { // the third body is the last
		t.Fatalf("stopped %v, %d steps", stopped, len(steps))
	}
	want := []struct {
		id      int
		dmg     float64
		through int
	}{{2, 100, 0}, {3, 200, 1}, {1, 31, 2}} // 100; 50×4; round(25×1.25)
	for i, st := range steps {
		if st.q.ID != want[i].id || st.dmg != want[i].dmg || st.through != want[i].through {
			t.Fatalf("step %d: id %d dmg %g through %d, want %+v", i, st.q.ID, st.dmg, st.through, want[i])
		}
	}
}

// Falloff is to each body's own distance, on what is left.
func TestPenetrateFalloff(t *testing.T) {
	w := &Weapon{Damage: 100, Pen: 0.5, Falloff: 0.5}
	steps, _ := penetrate(w, penBodies(12.7, 25.4), nil)
	if steps[0].dmg != 50 || steps[1].dmg != 13 { // 100×0.5; 100×0.5×0.25 = 12.5 → 13
		t.Fatalf("damage %g, %g", steps[0].dmg, steps[1].dmg)
	}
}

func TestPenetrateCapsAtThreeBodies(t *testing.T) {
	steps, stopped := penetrate(&Weapon{Damage: 100, Pen: 1}, penBodies(1, 2, 3, 4, 5), nil)
	if len(steps) != penMaxBodies || !stopped {
		t.Fatalf("%d steps, stopped %v", len(steps), stopped)
	}
	// Exactly three bodies: the third still stops it (no wall impact behind).
	if _, stopped := penetrate(&Weapon{Damage: 100, Pen: 1}, penBodies(1, 2, 3), nil); !stopped {
		t.Fatal("went on past the third body")
	}
}

// Penetration 0 (shotgun pellets, melee, a weapon without the stat) stops at
// the first body; below 1 damage left the bullet is spent.
func TestPenetrateStops(t *testing.T) {
	for _, pen := range []float64{0, -1, math.NaN()} {
		steps, stopped := penetrate(&Weapon{Damage: 100, Pen: pen}, penBodies(1, 2), nil)
		if len(steps) != 1 || !stopped {
			t.Fatalf("pen %g: %d steps, stopped %v", pen, len(steps), stopped)
		}
	}
	steps, stopped := penetrate(&Weapon{Damage: 3, Pen: 0.2}, penBodies(1, 2), nil) // 3, then 0.6
	if len(steps) != 1 || !stopped {
		t.Fatalf("spent bullet: %d steps, stopped %v", len(steps), stopped)
	}
	if _, stopped := penetrate(&Weapon{Damage: 100, Pen: 0.5}, penBodies(1, 2), nil); stopped {
		t.Fatal("stopped with damage left and no third body")
	}
}

// A spared teammate (mp_friendlyfire 0) takes nothing but the bullet still
// loses damage through it.
func TestPenetrateSparedStillAbsorbs(t *testing.T) {
	b := penBodies(1, 2)
	steps, _ := penetrate(&Weapon{Damage: 100, Pen: 0.5}, b, func(q *Player) bool { return q.ID == 1 })
	if !steps[0].spared || steps[1].spared || steps[1].dmg != 50 || steps[1].through != 1 {
		t.Fatalf("steps %+v", steps)
	}
}

// penLine: a shooter at z=6 and two players in line behind each other at z=0
// and z=-3 (front's team; back is an Artisan), one shot at chest height, no spread.
func penLine(t *testing.T, weapon, frontTeam string, ff string, frontHP, backHP int) (a, front, back *Player) {
	t.Helper()
	s := authServer(t)
	s.cvars["weapon_spread"] = "0"
	s.cvars["mp_friendlyfire"] = ff
	a = cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-14, 0, 6})
	front = cmdPlayer(t, s, "2.2.2.2", frontTeam, Vec3{-14, 0, 0})
	back = cmdPlayer(t, s, "3.3.3.3", content.Artisan, Vec3{-14, 0, -3})
	front.HP, back.HP = frontHP, backHP
	w := s.Weapons.Get(weapon)
	a.inv.Slots[w.Slot] = s.Weapons.newHeld(weapon)
	a.inv.Current = w.Slot
	r := &runner{s: s, p: a}
	for range DeployTicks + 5 {
		r.cmd(Cmd{})
	}
	msgs(a)
	r.cmd(Cmd{Pitch: math.Atan2(1.2-1.62, 6), B: BtnFire})
	return a, front, back
}

// Through the real shot path: two enemies in line both get hit, the one
// behind with less (the Prompt keeps 70%).
func TestShotPenetratesBodies(t *testing.T) {
	a, front, back := penLine(t, "prompt", content.Artisan, "0", maxHP, maxHP)
	hits := ofType(a, "hit")
	if len(hits) != 2 || int(hits[0]["id"].(float64)) != front.ID || int(hits[1]["id"].(float64)) != back.ID {
		t.Fatalf("hits %v", hits)
	}
	df, db := maxHP-front.HP, maxHP-back.HP
	if df <= 0 || db <= 0 || db >= df {
		t.Fatalf("damage front %d, back %d", df, db)
	}
	if want := int(math.Round(float64(df) * 0.7)); math.Abs(float64(db-want)) > 1 {
		t.Fatalf("back took %d, want ≈ %d (70%% of %d)", db, want, df)
	}
}

// mp_friendlyfire 0: the teammate in front takes nothing but the enemy behind
// gets the reduced damage, as if the teammate were an enemy.
func TestShotThroughTeammateFriendlyFireOff(t *testing.T) {
	_, mate, back := penLine(t, "prompt", content.Vibe, "0", maxHP, maxHP)
	_, _, ref := penLine(t, "prompt", content.Artisan, "0", maxHP, maxHP)
	if mate.HP != maxHP {
		t.Fatalf("teammate hurt with mp_friendlyfire 0: hp %d", mate.HP)
	}
	if back.HP != ref.HP {
		t.Fatalf("enemy behind a teammate: hp %d, behind an enemy: %d", back.HP, ref.HP)
	}
	_, mate1, _ := penLine(t, "prompt", content.Vibe, "1", maxHP, maxHP)
	if mate1.HP == maxHP {
		t.Fatal("teammate not hurt with mp_friendlyfire 1")
	}
}

// Shotgun pellets stop in the first body.
func TestShotgunPelletsStopInFirstBody(t *testing.T) {
	_, front, back := penLine(t, "tokenburner", content.Artisan, "0", 1000, maxHP) // front survives every pellet

	if front.HP == 1000 || back.HP != maxHP {
		t.Fatalf("front hp %d, back hp %d", front.HP, back.HP)
	}
}

// A kill through another body carries the kill feed's penetrated flag; one
// on the first body doesn't.
func TestKillFeedPenetrated(t *testing.T) {
	a, _, back := penLine(t, "prompt", content.Artisan, "0", maxHP, 1)
	k := ofType(a, "kill")
	if !back.Dead || len(k) != 1 || k[0]["pn"] != true {
		t.Fatalf("dead %v, kills %v", back.Dead, k)
	}
	a, front, _ := penLine(t, "prompt", content.Artisan, "0", 1, maxHP)
	k = ofType(a, "kill")
	if !front.Dead || len(k) != 1 || k[0]["pn"] != nil {
		t.Fatalf("dead %v, kills %v", front.Dead, k)
	}
}
