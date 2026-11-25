package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

func nadeAt(s *Server, by *Player, id string, pos Vec3, age float64) *Nade {
	w := s.Weapons.Get(id)
	n := &Nade{ID: len(s.nades) + 1, Weapon: id, Kind: w.Grenade.Kind, Pos: pos, State: "fly", by: by, w: w, age: age, resting: true}
	s.nades = append(s.nades, n)
	return n
}

func TestThrowEmptiesTheSlotAndDrawsTheBestWeapon(t *testing.T) {
	s := authServer(t)
	p := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-20, 0, 0})
	p.inv.Slots["he"] = &Held{ID: "mergeconflict", Ammo: 1}
	r := &runner{s: s, p: p}
	r.cmd(Cmd{W: "he", Yaw: -math.Pi / 2})
	for range DeployTicks {
		r.cmd(Cmd{Yaw: -math.Pi / 2})
	}
	r.cmd(Cmd{B: BtnFire, Yaw: -math.Pi / 2}) // the pin
	if len(s.nades) != 0 || p.inv.Slots["he"] == nil {
		t.Fatalf("thrown on the press: nades %+v", s.nades)
	}
	r.cmd(Cmd{Yaw: -math.Pi / 2}) // let go: the throw
	if len(s.nades) != 1 || s.nades[0].Kind != "he" || s.nades[0].Vel[0] < 10 {
		t.Fatalf("nades %+v", s.nades)
	}
	if p.inv.Slots["he"] != nil || p.inv.Current != "primary" {
		t.Fatalf("after the throw: slots %v, current %s", p.inv.Slots, p.inv.Current)
	}
	if w := weaponStateOf(p); w.Next < DeployTicks-1 {
		t.Fatalf("the drawn rifle should be deploying: next %d", w.Next)
	}
}

func TestNadeBouncesOffAWallAndSettles(t *testing.T) {
	s := authServer(t)
	p := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-20, 0, 0})
	n := nadeAt(s, p, "smoketest", Vec3{-30, 1, 0}, 0)
	n.resting, n.Vel = false, Vec3{-12, 0, 0} // the west wall is at x = -32
	for range 40 {
		s.tickNades(0.01)
	}
	if n.Vel[0] <= 0 || n.Pos[0] > -29 && n.Pos[0] < -31.95 {
		t.Fatalf("no bounce: pos %v vel %v", n.Pos, n.Vel)
	}
	for range 300 {
		s.tickNades(0.01)
	}
	if n.State != "smoke" || n.Pos[1] < 0 || n.Pos[1] > 0.2 {
		t.Fatalf("should have settled on the floor and popped: %+v", n)
	}
}

func TestHEDamagesInRadiusButNotThroughWalls(t *testing.T) {
	s := authServer(t)
	alice := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-28, 0, 0}) // 13 m away: out of her own HE
	bob := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-14, 0, 0})
	carol := cmdPlayer(t, s, "3.3.3.3", content.Artisan, Vec3{0, 0, 0}) // in the mid building's doorway
	far := cmdPlayer(t, s, "4.4.4.4", content.Artisan, Vec3{-14, 0, 20})
	nadeAt(s, alice, "mergeconflict", Vec3{-15, 0.1, 0}, 1.6)
	s.tickNades(0.01)
	if len(s.nades) != 0 {
		t.Fatalf("should have gone off: %d left", len(s.nades))
	}
	if bob.HP >= 50 || bob.HP <= 0 {
		t.Fatalf("bob next to it: hp %d", bob.HP)
	}
	nadeAt(s, alice, "mergeconflict", Vec3{-7.5, 0.1, 0}, 1.6) // outside mid's west wall (x = -6), carol inside
	s.tickNades(0.01)
	if carol.HP != maxHP || far.HP != maxHP {
		t.Fatalf("behind the wall %d, out of range %d: want untouched", carol.HP, far.HP)
	}
	if alice.HP != maxHP {
		t.Fatalf("alice is out of range: %d", alice.HP)
	}
}

func TestHEKillCountsForTheThrower(t *testing.T) {
	s := authServer(t)
	alice := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-20, 0, 0})
	bob := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-14, 0, 0})
	bob.HP = 10
	nadeAt(s, alice, "mergeconflict", Vec3{-14.5, 0.1, 0}, 1.6)
	s.tickNades(0.01)
	if !bob.Dead || alice.Kills != 1 {
		t.Fatalf("bob dead %v, alice kills %d", bob.Dead, alice.Kills)
	}
}

func TestFlashBlindsByFacing(t *testing.T) {
	s := authServer(t)
	alice := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-22, 0, 0})
	bob := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-12, 0, 0})
	carol := cmdPlayer(t, s, "3.3.3.3", content.Artisan, Vec3{0, 0, 0}) // behind the building's wall
	for _, p := range []*Player{alice, bob, carol} {
		p.Yaw, p.Pitch = -math.Pi/2, 0 // all looking east (+x)
	}
	nadeAt(s, alice, "stacktrace", Vec3{-17, 1.5, 0}, 1.5) // in front of alice, behind bob
	s.tickNades(0.01)
	now := s.clock()
	a, b, c := alice.flashLeft(now), bob.flashLeft(now), carol.flashLeft(now)
	if a < 3 || b <= 0.25 || b >= a/2 || c != 0 {
		t.Fatalf("blind: facing %.2f, back turned %.2f, behind a wall %.2f", a, b, c)
	}
	if alice.flashDur < a {
		t.Fatalf("flashDur %.2f < left %.2f", alice.flashDur, a)
	}
}

func TestSmokeStandsForItsDuration(t *testing.T) {
	s := authServer(t)
	p := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-20, 0, 0})
	n := nadeAt(s, p, "smoketest", Vec3{-20, nadeRadius, 5}, 0)
	for range 60 {
		s.tickNades(0.01)
	}
	if n.State != "smoke" || n.Left < 17 {
		t.Fatalf("smoke: %+v", n)
	}
	for range 1800 {
		s.tickNades(0.01)
	}
	if len(s.nades) != 0 {
		t.Fatalf("still standing after its duration: %+v", s.nades[0])
	}
}

func TestBuyGrenadeLimits(t *testing.T) {
	s, v, _ := buyServer(t)
	for i, want := range []string{"", "can't carry more"} {
		if why := s.purchase(v, "mergeconflict"); why != want {
			t.Fatalf("HE #%d: %q, want %q", i+1, why, want)
		}
	}
	for i, want := range []string{"", "", "can't carry more"} {
		if why := s.purchase(v, "stacktrace"); why != want {
			t.Fatalf("flash #%d: %q, want %q", i+1, why, want)
		}
	}
	if v.inv.Slots["flash"].Ammo != 2 || v.inv.Current == "flash" {
		t.Fatalf("flashes %+v, current %s (buying doesn't draw a grenade)", v.inv.Slots["flash"], v.inv.Current)
	}
	if why := s.purchase(v, "smoketest"); why != "" {
		t.Fatalf("smoke: %q", why)
	}
	v.inv.Slots["flash"].Ammo = 1 // four in all is the cap: drop to three, buy back to four
	if why := s.purchase(v, "stacktrace"); why != "" {
		t.Fatalf("4th grenade: %q", why)
	}
	v.inv.Slots["smoke"] = nil
	delete(v.inv.Slots, "smoke")
	if why := s.purchase(v, "smoketest"); why != "" {
		t.Fatalf("smoke again: %q", why)
	}
	if why := s.purchase(v, "smoketest"); why != "can't carry more" {
		t.Fatalf("second smoke: %q", why)
	}
	if s.grenadeCount(v.inv) != 4 {
		t.Fatalf("carrying %d", s.grenadeCount(v.inv))
	}
}

// CS throwing: held, the grenade stays in hand (pinned, no fuse running);
// attack is the full throw, attack + attack2 the medium one, attack2 the
// lob, each slower than the last; the fuse counts from the release.
func TestThrowOnReleaseStrengths(t *testing.T) {
	var speeds []float64
	for _, tc := range []struct{ b, pin int }{{BtnFire, ThrowFull}, {BtnFire | BtnAlt, ThrowFull | ThrowLob}, {BtnAlt, ThrowLob}} {
		b := tc.b
		s := authServer(t)
		p := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-20, 0, 0})
		p.inv.Slots["he"] = &Held{ID: "mergeconflict", Ammo: 1}
		r := &runner{s: s, p: p}
		r.cmd(Cmd{W: "he", Yaw: -math.Pi / 2})
		for range DeployTicks {
			r.cmd(Cmd{Yaw: -math.Pi / 2})
		}
		for range 300 { // 3 s held: longer than the fuse
			r.cmd(Cmd{B: b, Yaw: -math.Pi / 2})
			s.tickNades(1.0 / SimHz)
		}
		if len(s.nades) != 0 || p.inv.pin != tc.pin {
			t.Fatalf("b=%d held 3 s: nades %d pin %d", b, len(s.nades), p.inv.pin)
		}
		r.cmd(Cmd{Yaw: -math.Pi / 2})
		if len(s.nades) != 1 {
			t.Fatalf("b=%d: not thrown on release", b)
		}
		n := s.nades[0]
		speeds = append(speeds, math.Hypot(n.Vel[0], n.Vel[2]))
		for range int(n.w.Grenade.Fuse*SimHz) - 2 {
			s.tickNades(1.0 / SimHz)
		}
		if len(s.nades) != 1 {
			t.Fatalf("b=%d: went off before its fuse counted from the throw", b)
		}
		for range 4 {
			s.tickNades(1.0 / SimHz)
		}
		if len(s.nades) != 0 {
			t.Fatalf("b=%d: still live after its fuse", b)
		}
	}
	if !(speeds[0] > 18 && speeds[0] > speeds[1] && speeds[1] > speeds[2] && speeds[2] > 4) {
		t.Fatalf("horizontal launch speeds full/medium/lob %v, want ~19 > ~12 > ~6", speeds)
	}
}
