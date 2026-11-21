package game

import (
	"math"
	"testing"
)

func TestRecoilIndexAndOffset(t *testing.T) {
	p := [][3]float64{{1, 0.01, 0}, {3, 0.03, -0.02}}
	for _, tc := range []struct {
		idx    float64
		up, rt float64
	}{{0, 0, 0}, {-1, 0, 0}, {0.5, 0.005, 0}, {1, 0.01, 0}, {2, 0.02, -0.01}, {3, 0.03, -0.02}, {9, 0.03, -0.02}} {
		o := RecoilOffset(p, tc.idx)
		if math.Abs(o[0]-tc.up) > 1e-12 || math.Abs(o[1]-tc.rt) > 1e-12 {
			t.Errorf("offset at %v = %v, want [%v %v]", tc.idx, o, tc.up, tc.rt)
		}
	}
	if o := RecoilOffset(nil, 5); o != [2]float64{} {
		t.Errorf("no pattern: %v", o)
	}
	// Recovery starts one interval after the shot, at recover per second.
	for _, tc := range []struct {
		tick int64
		want float64
	}{{105, 5}, {110, 5}, {130, 3}, {900, 0}} {
		if got := RecoilIndex(5, 100, tc.tick, 10, 10); math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("index at %d = %v, want %v", tc.tick, got, tc.want)
		}
	}
	// up raises the pitch, right turns right (yaw down); pitch clamped.
	if y, pt := recoilAngles(1, 0.2, [2]float64{0.1, 0.05}, 2); math.Abs(y-0.9) > 1e-12 || math.Abs(pt-0.4) > 1e-12 {
		t.Errorf("angles %v %v", y, pt)
	}
	if _, pt := recoilAngles(0, 1.55, [2]float64{0.1, 0}, 1); pt != 1.6 {
		t.Errorf("pitch not clamped: %v", pt)
	}
}

// A held spray moves the server's shots along the weapon's pattern; the view
// (the command's angles) stays put; weapon_recoil_scale scales it and
// weapon_spread 0 (exact shots) turns it off; a switch starts a fresh spray.
func TestServerShotsFollowThePattern(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	p.ready, p.Team = true, "vibe"
	s.respawn(p)
	w := s.Weapons.Get("prompt")
	inv := p.inv
	inv.Slots["primary"] = &Held{ID: "prompt", Ammo: 30, Reserve: 90}
	inv.Current = "primary"
	var idxs []float64
	for tick := int64(100); tick < 160; tick++ {
		if got, _ := s.Weapons.trigger(inv, tick, true); got != nil {
			idxs = append(idxs, inv.shotIdx)
		}
	}
	if len(idxs) != 6 || idxs[0] != 0 || idxs[5] != 5 {
		t.Fatalf("spray indices %v", idxs)
	}
	off := RecoilOffset(w.Recoil.Pattern, inv.shotIdx)
	if off[0] <= 0 {
		t.Fatalf("the sixth shot has no climb: %v", off)
	}
	for _, tc := range []struct {
		spread, scale string
		k             float64
	}{{"1", "1", 1}, {"1", "2", 2}, {"1", "0", 0}, {"0", "1", 0}, {"1", "junk", 1}} {
		s.cvars["weapon_spread"], s.cvars["weapon_recoil_scale"] = tc.spread, tc.scale
		yaw, pitch := recoilAngles(0.3, 0.1, off, s.recoilScale())
		if math.Abs(pitch-(0.1+off[0]*tc.k)) > 1e-12 || math.Abs(yaw-(0.3-off[1]*tc.k)) > 1e-12 {
			t.Errorf("spread %s scale %s: shot at %v %v", tc.spread, tc.scale, yaw, pitch)
		}
	}
	s.cvars["weapon_spread"], s.cvars["weapon_recoil_scale"] = "1", "1"
	// The weapon state carries the spray; a switch resets it.
	if ws := weaponStateOf(p); ws.Recoil != 6 {
		t.Fatalf("weapon state recoil %v", ws.Recoil)
	}
	s.Weapons.switchTo(inv, "sidearm", 200)
	if inv.recoil != 0 || weaponStateOf(p).Recoil != 0 {
		t.Fatalf("switch kept the spray: %v", inv.recoil)
	}
}

// Bots pull against the spray by their difficulty's share of it.
func TestBotPullsDown(t *testing.T) {
	s := authServer(t)
	p := s.addAI("vibe")
	inv := p.inv
	inv.Slots["primary"] = &Held{ID: "prompt", Ammo: 30, Reserve: 90}
	inv.Current = "primary"
	inv.recoil, inv.lastShot = 8, p.cmdTicks
	c := Cmd{Yaw: 1, Pitch: 0.2}
	s.aiPullDown(p, &c, 0.5)
	off := RecoilOffset(s.Weapons.Get("prompt").Recoil.Pattern, 8)
	if math.Abs(c.Pitch-(0.2-off[0]*0.5)) > 1e-12 || math.Abs(c.Yaw-(1+off[1]*0.5)) > 1e-12 {
		t.Fatalf("pulled to %v %v (offset %v)", c.Yaw, c.Pitch, off)
	}
	inv.Current = "melee"
	c2 := Cmd{Yaw: 1, Pitch: 0.2}
	s.aiPullDown(p, &c2, 1)
	if c2.Yaw != 1 || c2.Pitch != 0.2 {
		t.Fatalf("melee pulled: %+v", c2)
	}
}
