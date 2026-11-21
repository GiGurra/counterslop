package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// Kill feed edges (QA pass): the headshot and noscope flags through the real
// shot path, a team kill, a suicide by your own HE.

// shootDead fires `weapon` at a victim 6 m away (head or body) until it dies
// and returns the kill message the shooter got.
func shootDead(t *testing.T, weapon string, scoped, head bool, victimTeam string) (map[string]any, *Player, *Player) {
	t.Helper()
	s := authServer(t)
	s.cvars["weapon_spread"] = "0"
	s.cvars["mp_friendlyfire"] = "1"
	a := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-14, 0, 6})
	b := cmdPlayer(t, s, "2.2.2.2", victimTeam, Vec3{-14, 0, 0})
	a.inv.Slots["primary"] = s.Weapons.newHeld(weapon)
	a.inv.Current = "primary"
	r := &runner{s: s, p: a}
	for range DeployTicks + 5 {
		r.cmd(Cmd{})
	}
	pitch := math.Atan2(0.9-1.62, 6) // body
	if head {
		pitch = math.Atan2(1.58-1.62, 6)
	}
	msgs(a)
	btn := BtnFire
	if scoped {
		btn |= BtnScope
	}
	for i := 0; i < 600 && !b.Dead; i++ {
		c := Cmd{Pitch: pitch}
		if i%2 == 0 { // tap: semi-autos fire on the press
			c.B = btn
		} else if scoped {
			c.B = BtnScope
		}
		r.cmd(c)
	}
	if !b.Dead {
		t.Fatalf("%s: victim not dead (hp %d)", weapon, b.HP)
	}
	k := ofType(a, "kill")
	if len(k) != 1 {
		t.Fatalf("%s: %d kill messages, want 1", weapon, len(k))
	}
	return k[0], a, b
}

func TestKillFeedFlags(t *testing.T) {
	cases := []struct {
		weapon       string
		scoped, head bool
		hs, ns       bool
	}{
		{"codereview", false, true, true, true}, // a sniper fired unscoped: noscope
		{"codereview", true, true, true, false},
		{"codereview", true, false, false, false},
		{"prompt", false, true, true, false}, // a rifle has no scope: never "noscope"
		{"prompt", false, false, false, false},
	}
	for _, c := range cases {
		k, _, _ := shootDead(t, c.weapon, c.scoped, c.head, content.Artisan)
		if (k["hs"] == true) != c.hs || (k["ns"] == true) != c.ns || k["w"] != c.weapon {
			t.Errorf("%s scoped=%v head=%v: kill %v, want hs %v ns %v", c.weapon, c.scoped, c.head, k, c.hs, c.ns)
		}
	}
}

// CS: a team kill costs a frag (and money, see economy_test); it is no kill.
func TestTeamKillIsNoKill(t *testing.T) {
	k, a, b := shootDead(t, "prompt", false, true, content.Vibe)
	if k["k"] != float64(a.ID) || k["v"] != float64(b.ID) {
		t.Fatalf("feed %v", k)
	}
	if a.Kills != -1 || b.Deaths != 1 {
		t.Fatalf("team kill: killer kills %d (want -1), victim deaths %d", a.Kills, b.Deaths)
	}
}

// Your own HE: the feed names you as killer and victim, no kill for it, one death.
func TestSuicideByOwnHE(t *testing.T) {
	s := authServer(t)
	alice := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-14, 0, 0})
	bob := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-14, 0, 30})
	alice.HP = 5
	msgs(bob)
	nadeAt(s, alice, "mergeconflict", Vec3{-14.5, 0.1, 0}, 1.6)
	s.tickNades(0.01)
	if !alice.Dead || alice.Kills != 0 || alice.Deaths != 1 {
		t.Fatalf("dead %v kills %d deaths %d", alice.Dead, alice.Kills, alice.Deaths)
	}
	k := ofType(bob, "kill")
	if len(k) != 1 || k[0]["k"] != float64(alice.ID) || k[0]["v"] != float64(alice.ID) || k[0]["w"] != "mergeconflict" {
		t.Fatalf("feed %v", k)
	}
}
