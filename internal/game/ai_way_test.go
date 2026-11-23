package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/sim"
)

// wayMap: a 40x40 arena split at z=0 by a wall with a gap of width w at
// x=0 (corridor: the wall is `deep` m thick, a corridor that long), and
// nav nodes either side of the gap.
func wayMap(w, deep float64) content.Map {
	b := mapgen.New("way")
	b.Add(mapgen.Arena(-20, -20, 20, 20, 5, 1, content.Concrete, content.Brick)...)
	b.Add(content.B(-20, 0, -deep/2, -w/2, 3, deep/2, content.Brick), content.B(w/2, 0, -deep/2, 20, 3, deep/2, content.Brick))
	b.TeamSpawns(content.Vibe, [3]float64{-15, 0, -15})
	b.TeamSpawns(content.Artisan, [3]float64{15, 0, 15})
	b.Nav("s", 0, 0, -deep/2-1.5, "n").Nav("n", 0, 0, deep/2+1.5)
	return b.Map()
}

func wayServer(t *testing.T, m content.Map) *Server {
	t.Helper()
	s := authServer(t)
	s.Map = m
	s.Seed(1)
	return s
}

// wayBot: a bot of the team at pos that walks to goal and nothing else.
func wayBot(s *Server, team string, pos, goal Vec3) *Player {
	p := s.addAI(team)
	p.Pos = pos
	p.body.Pos = sim.Vec3(pos)
	g := goal
	p.ai.goTo = &g
	return p
}

// runUntilArrived steps until every bot is within 1.3 m of its goal; the
// ticks it took. top: the highest any bot's feet got (over a head: >0).
func runUntilArrived(s *Server, bots []*Player, max int) (ticks int, top float64) {
	for n := 0; n < max; n += 10 {
		s.Step(10)
		all := true
		for _, p := range bots {
			top = math.Max(top, p.Pos[1])
			if math.Hypot(p.Pos[0]-p.ai.goTo[0], p.Pos[2]-p.ai.goTo[2]) > 1.3 {
				all = false
			}
		}
		if all {
			return n + 10, top
		}
	}
	return -1, top
}

// Two bots meet head-on in a 1.4 m corridor (two hulls need 1.6: no room to
// pass; a 1 m one leaves the waypoint follower 0.1 m to line up): the higher ID
// backs out and lets the other through, and both arrive.
func TestBotsPassInCorridor(t *testing.T) {
	s := wayServer(t, wayMap(1.4, 8))
	a := wayBot(s, content.Vibe, Vec3{0, 0, -8}, Vec3{0, 0, 8})
	b := wayBot(s, content.Vibe, Vec3{0, 0, 8}, Vec3{0, 0, -8})
	backed, low := 0.0, b.Pos[2] // how far b (the higher ID) backed off from its furthest in
	for range 4 * SimHz / 10 {
		s.Step(10)
		low = math.Min(low, b.Pos[2])
		backed = math.Max(backed, b.Pos[2]-low)
	}
	n, top := runUntilArrived(s, []*Player{a, b}, 20*SimHz)
	if top > 0.05 {
		t.Fatalf("a bot went over the other's head (feet at %.2f)", top)
	}
	if backed < 0.5 {
		t.Fatalf("b (the higher ID) didn't back off out of the corridor: %.2f m", backed)
	}
	if n < 0 {
		t.Fatalf("not through in 20 s: %v -> %v, %v -> %v", a.Pos, *a.ai.goTo, b.Pos, *b.ai.goTo)
	}
	t.Logf("both through in %.1f s", float64(n)/SimHz)
	if n > 10*SimHz {
		t.Fatalf("took %.1f s", float64(n)/SimHz)
	}
}

// Five bots through one 1.2 m door: they queue and all get through.
func TestBotsThroughDoor(t *testing.T) {
	s := wayServer(t, wayMap(1.2, 0.4))
	var bots []*Player
	for i := range 5 {
		x := float64(i-2) * 1.5
		bots = append(bots, wayBot(s, content.Vibe, Vec3{x, 0, -6}, Vec3{x, 0, 6}))
	}
	n, _ := runUntilArrived(s, bots, 30*SimHz)
	if n < 0 {
		for _, p := range bots {
			t.Logf("%s at %v, goal %v", p.Name, p.Pos, *p.ai.goTo)
		}
		t.Fatal("not all through in 30 s")
	}
	t.Logf("all 5 through in %.1f s", float64(n)/SimHz)
}

// A bot walking up behind a human standing in its way goes round him.
func TestBotGoesRoundStandingHuman(t *testing.T) {
	s := wayServer(t, wayMap(20, 0.4)) // a wide gap: open floor
	h := &Player{ID: 99, Name: "human", ready: true, Team: content.Vibe, HP: maxHP, Pos: Vec3{0, 0, 0}}
	s.players[h.ID] = h
	p := wayBot(s, content.Vibe, Vec3{0, 0, -6}, Vec3{0, 0, 6})
	n, top := runUntilArrived(s, []*Player{p}, 10*SimHz)
	if top > 0.05 {
		t.Fatalf("went over the human's head (feet at %.2f), not round him", top)
	}
	if n < 0 {
		t.Fatalf("stuck behind the human: at %v", p.Pos)
	}
	t.Logf("round the human in %.1f s", float64(n)/SimHz)
}

// At a round start a team's bots leave one after another, aiWayStagger
// ticks apart in ID order, not in lockstep into the same doorway.
func TestBotsStaggerTheStart(t *testing.T) {
	s := aiMatch(t, "de_slop", 3, 0)
	for range 40 * SimHz {
		s.Step(1)
		if s.round.Phase == PhaseLive && !s.frozen() {
			break
		}
	}
	s.Step(5)
	if s.round.Phase != PhaseLive {
		t.Fatalf("phase %q, want live", s.round.Phase)
	}
	for _, team := range []string{content.Vibe, content.Artisan} {
		var starts []int64
		for _, p := range s.ordered() {
			if p.ai != nil && p.Team == team {
				if p.ai.way.round != s.round.Number {
					t.Fatalf("%s: no staggered start for round %d", p.Name, s.round.Number)
				}
				starts = append(starts, p.ai.way.waitTill-p.cmdTicks)
			}
		}
		for i := 1; i < len(starts); i++ {
			if d := starts[i] - starts[i-1]; d != aiWayStagger {
				t.Fatalf("%s starts %v: want %d ticks apart", team, starts, aiWayStagger)
			}
		}
	}
}
