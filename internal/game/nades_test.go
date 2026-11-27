package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// mp_loadout nades: the keyboard and one of every grenade, the team's own
// fire one, the HE in hand; four in all (the carry limit).
func TestNadeLoadout(t *testing.T) {
	r := registry(t)
	for _, team := range []string{content.Vibe, content.Artisan} {
		inv := r.NadeLoadout(team)
		if inv.Current != "he" || inv.Slots["melee"] == nil || inv.Slots["primary"] != nil || inv.Slots["sidearm"] != nil {
			t.Fatalf("%s: %+v", team, inv)
		}
		n := 0
		for _, slot := range []string{"he", "flash", "smoke", "fire"} {
			h := inv.Slots[slot]
			if h == nil || h.Ammo != 1 {
				t.Fatalf("%s: %s %+v", team, slot, h)
			}
			if w := r.Get(h.ID); w.Team != nil && *w.Team != team {
				t.Fatalf("%s carries the other side's %s", team, h.ID)
			}
			n++
		}
		if n > nadeCarryTotal {
			t.Fatalf("%d grenades, the limit is %d", n, nadeCarryTotal)
		}
	}
	if a, v := r.NadeLoadout(content.Artisan).Slots["fire"].ID, r.NadeLoadout(content.Vibe).Slots["fire"].ID; a == v {
		t.Fatalf("both sides got the same fire grenade %s", a)
	}
}

// mp_nade_refill: a grenade slot gone empty fills mp_nade_refill seconds
// later (not before, not without mp_loadout nades, never at 0); a spawn under
// a map's Rules gets the grenades and runs team deathmatch.
func TestNadeRefillAndRules(t *testing.T) {
	s, v, _ := buyServer(t)
	s.Map.Rules = map[string]string{"mp_loadout": "nades", "mp_nade_refill": "2", "mp_respawn": "1", "mp_buy_guns": "0"}
	if !s.deathmatch() {
		t.Fatal("the map's mp_respawn 1: deathmatch")
	}
	s.respawn(v)
	if v.inv.Slots["he"] == nil || v.inv.Slots["primary"] != nil {
		t.Fatalf("spawned with %+v", v.inv.Slots)
	}
	delete(v.inv.Slots, "he") // thrown (the last one leaves the slot, weapons_shared.go)
	v.inv.Slots["flash"].Ammo = 0
	for i := range 2*SimHz - 1 {
		s.tick++
		s.tickNadeRefill()
		if v.inv.Slots["he"] != nil {
			t.Fatalf("back after %d ticks", i+1)
		}
	}
	s.tick++
	s.tickNadeRefill()
	s.tick++
	s.tickNadeRefill()
	if h := v.inv.Slots["he"]; h == nil || h.Ammo != 1 || v.inv.Slots["flash"].Ammo != 1 {
		t.Fatalf("not refilled after 2 s: he %+v flash %+v", h, v.inv.Slots["flash"])
	}
	// No refill at 0, nor without the grenade loadout.
	for _, rules := range []map[string]string{{"mp_loadout": "nades", "mp_nade_refill": "0"}, {"mp_nade_refill": "2"}} {
		s.Map.Rules = rules
		delete(v.inv.Slots, "he")
		v.nadeGone = nil
		for range 3 * SimHz {
			s.tick++
			s.tickNadeRefill()
		}
		if v.inv.Slots["he"] != nil {
			t.Fatalf("refilled under %v", rules)
		}
	}
	// mp_buy_guns 0: no guns, grenades and gear still.
	s.Map.Rules = map[string]string{"mp_buy_guns": "0"}
	if why := s.purchase(v, "prompt"); why != "no guns on this map (mp_buy_guns)" {
		t.Fatalf("gun: %q", why)
	}
	if why := s.purchase(v, "mergeconflict"); why != "" {
		t.Fatalf("grenade: %q", why)
	}
	if why := s.purchase(v, "vest"); why != "" {
		t.Fatalf("vest: %q", why)
	}
}

// Bots with nothing but grenades (and the keyboard) fight with them: both
// sides lob HEs at the enemies they see, they come back (refill) and are
// thrown again, and people die of it.
func TestAIGrenadeFight(t *testing.T) {
	t.Parallel()
	s := aiMatch(t, "de_standup", 2, 0)
	s.Map.Rules = map[string]string{"mp_loadout": "nades", "mp_nade_refill": "3", "mp_respawn": "1", "mp_respawntime": "1"}
	s.round.Phase = PhaseWarmup
	for _, p := range s.ordered() {
		s.respawn(p)
	}
	he := map[string]int{}
	seen := map[int]bool{}
	for range 90 * SimHz / 10 {
		s.Step(10)
		for _, n := range s.nades {
			if !seen[n.ID] && n.by != nil {
				seen[n.ID] = true
				if n.Kind == "he" {
					he[n.by.Team]++
				}
			}
		}
	}
	deaths := 0
	for _, p := range s.ordered() {
		deaths += p.Deaths
		if p.inv != nil && (p.inv.Slots["primary"] != nil || p.inv.Slots["sidearm"] != nil) {
			t.Errorf("%s has a gun: %+v", p.Name, p.inv.Slots)
		}
	}
	t.Logf("HEs thrown %v, deaths %d", he, deaths)
	if he[content.Vibe] < 3 || he[content.Artisan] < 3 {
		t.Errorf("HEs thrown %v: both sides should keep lobbing (refilled)", he)
	}
	if deaths == 0 {
		t.Error("90 s of grenades and nobody died")
	}
}

// Up a ramp over the void (de_serverless: the spawn bridge up to the hub,
// 4 m over 8 m) is floor all the way, a step at a time; off its side isn't.
func TestFloorAlongUpARamp(t *testing.T) {
	s := authServer(t)
	s.Map = maps.All["de_serverless"]()
	for _, w := range [][2]Vec3{{{-24, 0, 0}, {-14, 3, 0}}, {{-17, 1.5, 0}, {-15.8, 2.1, 0}}, {{-10, 4, 0}, {-22, 0, 0}}} {
		if !s.floorAlong(w[0], w[1]) {
			t.Errorf("%v -> %v: no floor", w[0], w[1])
		}
	}
	if s.floorAlong(Vec3{-17, 1.5, 0}, Vec3{-17, 1.5, 4}) {
		t.Error("off the ramp's side: floor")
	}
}

// Bots on bridges (he_merge_hell): a straight walk over the pit has no floor
// (the corner cut that skipped a nav waypoint walked a bot off the base), the
// stuck sidestep only steps where there is floor, a respawned bot routes
// again from the spawn (its old route led over the pit), and with grenades
// only nobody lobs them into an empty site (its fire blocked a bridge).
func TestBotsKeepToTheBridges(t *testing.T) {
	s := authServer(t)
	s.Map = maps.All["he_merge_hell"]()
	if s.floorAlong(Vec3{-30, 0, -4}, Vec3{-8, 0, 11.2}) {
		t.Fatal("spawn -> north branch straight: floor all the way over the pit")
	}
	if !s.floorAlong(Vec3{-30, 0, 0}, Vec3{-6, 0, 0.6}) || !s.floorAlong(Vec3{-23, 0, 10.5}, Vec3{-8, 0, 11.2}) {
		t.Fatal("along a bridge: no floor")
	}
	s.Seed(1)
	a := s.addAI(content.Vibe)
	s.Step(1)
	put(s, a, Vec3{-6, 0, 10.4}) // on the north branch (z 9..12), facing along it (+x)
	a.ai.yaw = -math.Pi / 2
	put(s, a, Vec3{-6, 0, 9.3}) // at the branch's south edge: only the north side has floor
	for _, side := range []float64{1, -1} {
		got := s.sidestep(a, side)
		right := Vec3{math.Cos(a.ai.yaw), 0, -math.Sin(a.ai.yaw)}
		to := Vec3{a.Pos[0] + right[0]*got*1.2, 0, a.Pos[2] + right[2]*got*1.2}
		if got != 0 && (to[2] < 9 || to[2] > 12) {
			t.Fatalf("sidestep %v (asked %v) lands at %v: off the branch", got, side, to)
		}
	}
	a.ai.path, a.ai.goalKey = []Vec3{{0, 0, 9.6}}, "x"
	s.respawn(a)
	if a.ai.path != nil || a.ai.goalKey != "" {
		t.Fatalf("respawned with the old route %v", a.ai.path)
	}
	s.Map.Rules["mp_loadout"] = "nades"
	a.inv = s.Weapons.NadeLoadout(content.Vibe)
	put(s, a, Vec3{-12, 0, 10.4})
	if s.aiSiteNades(a) {
		t.Fatal("grenades only: lobbed one into an empty site")
	}
}

// An LGTM wall (he_merge_hell's padding) sends a grenade back at lgtmBounce,
// about twice what any other wall does (approved without reading).
func TestNadeBouncesHarderOffLGTM(t *testing.T) {
	back := func(kind string) float64 {
		s := authServer(t)
		for i := range s.Map.Boxes { // the west wall, x -32
			if b := &s.Map.Boxes[i]; b.Min[0] <= -32.5 && b.Max[0] >= -32 && b.Min[1] <= 1 && b.Max[1] >= 1 && b.Min[2] <= 0 && b.Max[2] >= 0 && kind != "" {
				b.Kind = kind
			}
		}
		p := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-20, 0, 0})
		n := nadeAt(s, p, "smoketest", Vec3{-30, 1, 0}, 0)
		n.resting, n.Vel = false, Vec3{-12, 0, 0}
		for range 40 {
			s.tickNades(0.01)
		}
		return n.Vel[0]
	}
	plain, lgtm := back(""), back(content.LGTM)
	if math.Abs(plain-12*nadeBounce) > 0.01 || math.Abs(lgtm-12*lgtmBounce) > 0.01 {
		t.Fatalf("back off the wall at %.2f (plain) and %.2f (LGTM): want %.2f and %.2f", plain, lgtm, 12*nadeBounce, 12*lgtmBounce)
	}
}

// A changelevel onto a grenades-only map hands everyone the grenade loadout
// (the reset gave the default guns: the Prompt on he_merge_hell).
func TestChangeLevelToNadesOnly(t *testing.T) {
	s, v, _ := buyServer(t)
	s.mu.Lock()
	ok := s.changeLevel("he_merge_hell")
	s.mu.Unlock()
	if !ok || v.inv.Slots["primary"] != nil || v.inv.Slots["he"] == nil || v.inv.Current != "he" {
		t.Fatalf("after changelevel: %v, slots %+v", ok, v.inv.Slots)
	}
}

// 2v2 bots on he_merge_hell for 3 minutes fight with grenades and stay out
// of main (aiEdgeGuard): chasing and strafing in a fight walked them off the
// bridges and jumped them over the rails, 4 falls in 3 minutes.
func TestBotsStayOutOfMain(t *testing.T) {
	s := aiMatch(t, "he_merge_hell", 2, 0)
	onFloor := map[*Player]bool{}
	falls, deaths := 0, 0
	for range 180 * SimHz {
		s.Step(1)
		for _, p := range s.players {
			if p.Dead || p.body == nil {
				continue
			}
			if p.Pos[1] > -0.5 {
				onFloor[p] = true
			} else if onFloor[p] {
				falls++
				t.Logf("%s fell in at %s", p.Name, v3(p.Pos))
				onFloor[p] = false
			}
		}
	}
	for _, p := range s.players {
		deaths += p.Deaths
	}
	t.Logf("%d deaths, %d in main", deaths, falls)
	if falls > 1 || deaths-falls < 4 {
		t.Fatalf("%d deaths, %d of them in main: want grenade kills and next to no falls", deaths, falls)
	}
}
