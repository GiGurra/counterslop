package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// defendBots: a live round on mapName with one Artisan bot playing site.
func defendBot(t *testing.T, mapName, site string, pos Vec3) (*Server, *Player) {
	t.Helper()
	s := authServer(t)
	s.Map = maps.All[mapName]()
	a := s.addAI(content.Artisan)
	s.Step(1)
	s.round.Phase, s.round.Number = PhaseLive, 1
	a.ai.round, a.ai.site = 1, site
	s.teleport(a, pos, 0, false, a.lastQueued)
	return s, a
}

func TestSiteHoldStaysAheadOfTheAttackers(t *testing.T) {
	s := authServer(t)
	s.Map = maps.All["de_standup"]()
	// B is next to the attackers' spawn: hold short of it, on the defenders' side.
	h, c := s.siteHold("B"), s.mustSite("B")
	if h == c || s.siteZone("B").Contains(h) {
		t.Fatalf("de_standup B hold %v: want a spot short of the site (middle %v)", h, c)
	}
	att := Vec3(s.Map.TeamSpawns[content.Vibe][0])
	def := Vec3(s.Map.TeamSpawns[content.Artisan][0])
	if pathLen(def, s.navPath(def, h))+aiHoldMargin > pathLen(att, s.navPath(att, h))+1 {
		t.Fatalf("hold %v isn't ahead of the attackers", h)
	}
	// A is the defenders' own: they hold on it.
	ha := s.siteHold("A")
	if !s.siteZone("A").Contains(Vec3{ha[0], ha[1] + 0.5, ha[2]}) {
		t.Fatalf("de_standup A hold %v is off the site", ha)
	}
	if s.siteHold("A") != ha || s.siteHold("B") != h {
		t.Fatal("not cached")
	}
	// Each site's entrance is outside it, on the attackers' route.
	for _, z := range s.Map.Sites {
		e, ok := s.siteEntrance(z.Name)
		if !ok || z.Contains(Vec3{e[0], z.Min[1] + 1, e[2]}) {
			t.Fatalf("%s entrance %v", z.Name, e)
		}
	}
}

func TestIntelRotateOncePerRoundWhenOursIsQuiet(t *testing.T) {
	s, a := defendBot(t, "de_monorepo", "A", Vec3{25, 0, 18})
	b := s.mustSite("B")
	if _, ok := s.intel(content.Artisan); ok || s.seenAt(content.Artisan, "B") != -1 {
		t.Fatal("intel before anyone saw anything")
	}
	s.noteIntel(content.Artisan, b)
	if s.nearSite(b) != "B" || s.nearSite(Vec3{-30, 0, 0}) != "" {
		t.Fatal("nearSite")
	}
	// Our site saw someone a moment ago: stay.
	s.noteIntel(content.Artisan, s.mustSite("A"))
	s.noteIntel(content.Artisan, b)
	s.aiRotate(a)
	if a.ai.site != "A" {
		t.Fatal("rotated away from a busy site")
	}
	// Quiet at ours, B called: go, once.
	s.tick += aiQuietTicks
	s.noteIntel(content.Artisan, b)
	s.aiRotate(a)
	if a.ai.site != "B" {
		t.Fatal("didn't rotate to the site under attack")
	}
	s.tick += aiQuietTicks
	s.noteIntel(content.Artisan, s.mustSite("A"))
	s.aiRotate(a)
	if a.ai.site != "B" {
		t.Fatal("rotated twice in one round")
	}
	// Old news is no news; a new round forgets it.
	s.tick += aiIntelTicks + 1
	if _, ok := s.intel(content.Artisan); ok {
		t.Fatal("stale intel")
	}
	s.noteIntel(content.Artisan, b)
	s.round.Number = 2
	if _, ok := s.intel(content.Artisan); ok {
		t.Fatal("last round's intel")
	}
}

func TestDefenderWatchesIntelThenTheEntrance(t *testing.T) {
	s, a := defendBot(t, "de_monorepo", "A", Vec3{25, 0, 18})
	a.ai.path = nil
	e, _ := s.siteEntrance("A")
	w, ok := s.aiWatch(a)
	if !ok || w[0] != e[0] || w[2] != e[2] {
		t.Fatalf("watch %v, want the entrance %v", w, e)
	}
	s.noteIntel(content.Artisan, Vec3{15, 0, 10})
	if w, _ = s.aiWatch(a); w[0] != 15 || w[2] != 10 {
		t.Fatalf("watch %v, want the fresh sighting", w)
	}
	// Turning to it.
	for range 40 {
		s.lookAt(a, w, 0.2)
	}
	o := s.eyeOf(a)
	want := math.Atan2(-(15 - o[0]), -(10 - o[2]))
	if math.Abs(angleDiff(want, a.ai.yaw)) > 0.05 {
		t.Fatalf("yaw %.2f, want %.2f", a.ai.yaw, want)
	}
	// On the way from the defenders' spawn: watch the open way toward the
	// attackers (mid), not where it walks.
	s2, a2 := defendBot(t, "de_monorepo", "A", Vec3{24, 0, 0})
	a2.ai.path = s2.navPath(a2.Pos, s2.siteHold("A"))
	n, ok := s2.threatNode(a2)
	if !ok || onPath(a2.ai.path, n) || s2.attackerDist()[n] >= s2.attackerDist()[Vec3{20, 0, 0}] {
		t.Fatalf("threat node %v (ok %v) on the way from %v", n, ok, a2.Pos)
	}
}

// put moves p (server body and position) now.
func put(s *Server, p *Player, at Vec3) {
	s.teleport(p, at, 0, false, p.lastQueued)
	p.Pos = at
}

func TestRetakeWaitsForATeammate(t *testing.T) {
	s, a := defendBot(t, "de_slop", "A", Vec3{20, 0, 10})
	put(s, a, Vec3{20, 0, 10})
	mate := s.addAI(content.Artisan)
	a.Dead, mate.Dead = false, false
	put(s, mate, Vec3{-20, 0, -20})
	s.round.Phase = PhasePlanted
	s.round.Bomb = bombState{State: BombPlanted, Pos: Vec3{22, 0, 22}}
	s.round.endsAt = s.now.Add(secs(40))
	if !s.aiWaitForRetake(a) {
		t.Fatal("went in alone with a teammate on the way")
	}
	put(s, mate, Vec3{21, 0, 12})
	if s.aiWaitForRetake(a) {
		t.Fatal("waited with a teammate next to it")
	}
	put(s, mate, Vec3{-20, 0, -20})
	s.round.endsAt = s.now.Add(secs(defuseSecs + 2))
	if s.aiWaitForRetake(a) {
		t.Fatal("waited with no time left")
	}
	s.round.endsAt = s.now.Add(secs(40))
	mate.Dead = true
	if s.aiWaitForRetake(a) {
		t.Fatal("waited for a dead teammate")
	}
	put(s, a, Vec3{-20, 0, 20})
	if s.aiWaitForRetake(a) {
		t.Fatal("waited far from the Big Diff")
	}
}

func TestFacing(t *testing.T) {
	s, a := defendBot(t, "de_slop", "A", Vec3{0, 0, 0})
	v := s.addAI(content.Vibe)
	s.teleport(v, Vec3{0, 0, 20}, 0, false, v.lastQueued) // yaw 0 looks toward -z: at a
	if !s.facing(v, a) {
		t.Fatal("looking at it")
	}
	s.teleport(v, Vec3{0, 0, 20}, math.Pi, false, v.lastQueued)
	if s.facing(v, a) {
		t.Fatal("back turned")
	}
}

// de_stack's floors: a bot under its goal (the site is on the roof above)
// routes up the stairs instead of standing under it all round.
func TestBotUnderItsGoalRoutesUpstairs(t *testing.T) {
	s, a := defendBot(t, "de_stack", "A", Vec3{21, 4, 20})
	s.round.Phase = PhaseWarmup // walk to the site, no fights
	a.ai.site = "A"
	for range 30 * SimHz {
		s.Step(1)
		if a.Pos[1] > 7 {
			return
		}
	}
	t.Fatalf("still at %v after 30 s: never got up to A", a.Pos)
}

// In a live round the attackers mostly go for the same site (a lurker now
// and then): split evenly over two sites, they lose both on a big map.
func TestAIAttackersShareASite(t *testing.T) {
	s := aiMatch(t, "de_legacy", 8, 3*SimHz)
	if !s.roundsOn() {
		t.Fatal("rounds off")
	}
	for round := 0; round < 5; round++ {
		s.round.Number++
		same, all := 0, 0
		for _, p := range s.ordered() {
			if p.ai != nil && p.Team == content.Vibe {
				all++
				if s.pickSite(p) == s.aiAttack.site {
					same++
				}
			}
		}
		if s.aiAttack.round != s.round.Number || same < all*5/8 {
			t.Fatalf("round %d: %d of %d attackers on the team's site %q", s.round.Number, same, all, s.aiAttack.site)
		}
	}
}

// A bot doesn't crouch for range behind a railing that then hides its target
// (de_cloudfarm: an attacker on B's catwalk crouched out of sight, stood,
// saw them, crouched again... until the round ran out; seed 2 timed out).
func TestAICrouchBehindRailingDoesNotRunTheClockOut(t *testing.T) {
	s := authServer(t)
	s.Map = maps.All["de_cloudfarm"]()
	s.Seed(2)
	s.cvars["mp_freezetime"] = "1"
	s.cvars["mp_round_restart_delay"] = "1"
	s.cvars["mp_respawn"] = "0"
	s.cvars["bot_difficulty"] = "3"
	for range 3 {
		s.addAI(content.Vibe)
		s.addAI(content.Artisan)
	}
	for range 4 * 60 * SimHz / 10 {
		s.Step(10)
		if s.round.Phase == PhaseOver && s.round.Reason == "time" {
			t.Fatalf("round %d timed out", s.round.Number)
		}
	}
}
