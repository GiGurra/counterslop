package game

import (
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// laneMap: a 40x40 arena with a block in the middle; attackers start south,
// the site is north, and the way round is west or east (both lanes).
func laneMap(lanes bool) content.Map {
	b := mapgen.New("lanes")
	b.Add(mapgen.Arena(-20, -20, 20, 20, 5, 1, content.Concrete, content.Brick)...)
	b.Add(content.B(-12, 0, -10, 12, 4, 10, content.Brick))
	b.TeamSpawns(content.Vibe, [3]float64{0, 0, -16})
	b.TeamSpawns(content.Artisan, [3]float64{0, 0, 16})
	b.Site("A", -4, -0.5, 13, 4, 3, 19)
	b.Site("B", -19, -0.5, -19, -16, 3, -16)
	nav := b.Nav
	if lanes {
		nav = b.NavLane
	}
	b.Nav("vibe", 0, 0, -16, "sw", "se").Nav("artisan", 0, 0, 16).Nav("A", 0, 0, 16, "nw", "ne")
	nav("west", -16, 0, 0, "sw", "nw")
	nav("east", 16, 0, 0, "se", "ne")
	b.Nav("sw", -16, 0, -16).Nav("se", 16, 0, -16).Nav("nw", -16, 0, 16).Nav("ne", 16, 0, 16)
	return b.Map()
}

func laneBot(t *testing.T, m content.Map, seed uint64) (*Server, *Player) {
	t.Helper()
	s := authServer(t)
	s.Map = m
	var p *Player
	for range seed { // a bot's rng is seeded from its id: the n-th bot added
		p = s.addAI(content.Vibe)
	}
	p.Pos = Vec3{0, 0, -16}
	s.round.Phase, s.round.Number = PhaseLive, 1
	p.ai.site = "A"
	return s, p
}

func passes(path []Vec3, x, z float64) bool {
	for _, v := range path {
		if v[0] == x && v[2] == z {
			return true
		}
	}
	return false
}

// Attackers spread over the lanes (each bot its own seeded pick), and each route goes through its lane.
func TestAttackersPickLanes(t *testing.T) {
	used := map[float64]int{}
	for seed := uint64(1); seed <= 12; seed++ {
		s, p := laneBot(t, laneMap(true), seed)
		goal := s.mustSite("A")
		path := s.attackPath(p, "site:A", goal)
		if !p.ai.lane.on {
			t.Fatalf("seed %d: no lane picked", seed)
		}
		x := p.ai.lane.at[0]
		used[x]++
		if !passes(path, x, 0) {
			t.Fatalf("seed %d: lane %v but the path %v doesn't go through it", seed, p.ai.lane.at, path)
		}
		if last := path[len(path)-1]; last != goal {
			t.Fatalf("seed %d: path ends at %v, want the site %v", seed, last, goal)
		}
		// Same round, a repath (stuck): still the same lane.
		if s.attackPath(p, "site:A", goal); p.ai.lane.at[0] != x {
			t.Fatalf("seed %d: repath changed the lane", seed)
		}
	}
	if used[-16] == 0 || used[16] == 0 {
		t.Fatalf("lanes used %v, want both", used)
	}
}

// The lane is done once the bot is there (a repath goes straight on to the
// site, not back to it), or once the Big Diff drops.
func TestLaneDroppedOnArrivalOrBombDrop(t *testing.T) {
	s, p := laneBot(t, laneMap(true), 3)
	goal := s.mustSite("A")
	s.attackPath(p, "site:A", goal)
	p.Pos = p.ai.lane.at
	s.laneProgress(p)
	if p.ai.lane.on {
		t.Fatal("lane still on after arriving")
	}
	if path := s.attackPath(p, "site:A", goal); passes(path, p.Pos[0], -16) {
		t.Fatalf("after the lane the route goes back south: %v", path)
	}

	s, p = laneBot(t, laneMap(true), 3)
	s.attackPath(p, "site:A", goal)
	s.round.Bomb.State = BombDropped
	s.laneProgress(p)
	if p.ai.lane.on {
		t.Fatal("lane still on after the Big Diff dropped")
	}
	// A new round picks again.
	s.round.Bomb.State, s.round.Number = BombCarried, 2
	s.laneProgress(p)
	s.attackPath(p, "site:A", goal)
	if !p.ai.lane.on || p.ai.lane.round != 2 {
		t.Fatalf("round 2 lane %+v", p.ai.lane)
	}
}

// A lane far off the way (more than aiLaneDetour longer) is never taken, and
// defenders and warmup roaming ignore lanes.
func TestLaneLimits(t *testing.T) {
	s, p := laneBot(t, laneMap(true), 1)
	// B is in the south-west corner: both lanes are detours past the limit.
	if at, ok := s.pickLane(p, s.mustSite("B")); ok {
		t.Fatalf("picked lane %v for the site next to spawn", at)
	}
	p.Team = content.Artisan
	if s.attackPath(p, "site:A", s.mustSite("A")); p.ai.lane.on {
		t.Fatal("a defender took a lane")
	}
	p.Team = content.Vibe
	s.round.Phase = PhaseWarmup
	if s.attackPath(p, "site:A", s.mustSite("A")); p.ai.lane.on {
		t.Fatal("warmup roaming took a lane")
	}
}

// A map without lane nodes: the plain shortest route, and no rng drawn (so
// the existing maps' bot matches play exactly as before).
func TestNoLanesNoChange(t *testing.T) {
	_, p := laneBot(t, laneMap(false), 5)
	before := p.ai.r.Float()
	s2, p2 := laneBot(t, laneMap(false), 5)
	goal := s2.mustSite("A")
	got := s2.attackPath(p2, "site:A", goal)
	want := s2.navPath(p2.Pos, goal)
	if len(got) != len(want) {
		t.Fatalf("path %v, want %v", got, want)
	}
	if p2.ai.r.Float() != before {
		t.Fatal("attackPath drew from the bot's rng on a map without lanes")
	}
}
