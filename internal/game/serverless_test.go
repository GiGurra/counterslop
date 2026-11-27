package game

import (
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// de_serverless with bots, 4 a side for 6 minutes: they fly the jump pads
// (the only way onto site A but the shuttle), plant on both sites and never
// walk off a platform into the void (a grenade may throw one: at most one).
// Everyone wears the Gravity Override (the map's rule).
func TestServerlessBots(t *testing.T) {
	t.Parallel()
	s := aiMatch(t, "de_serverless", 4, 0)
	falls, throws := 0, 0
	plants := map[string]bool{}
	down, fly := map[int]bool{}, map[int]bool{}
	boots := true
	for range 6 * 60 * SimHz / 10 {
		s.Step(10)
		if s.round.Bomb.State == BombPlanted {
			plants[s.round.Bomb.Site] = true
		}
		for _, p := range s.players {
			if p.body == nil || p.Dead {
				continue
			}
			boots = boots && p.body.Boots
			f := p.body.Pos[1] < -6
			if f && !down[p.ID] {
				falls++
				t.Logf("%s fell at %.0f %.0f (round %d)", p.Name, p.Pos[0], p.Pos[2], s.round.Number)
			}
			down[p.ID] = f
			th := !p.body.OnGround && p.body.Vel[1] > 9 // only a pad throws this hard up
			if th && !fly[p.ID] {
				throws++
			}
			fly[p.ID] = th
		}
	}
	t.Logf("rounds %d, score %v, pad throws %d, falls %d, planted at %v", s.round.Number, s.round.Score, throws, falls, plants)
	if !boots {
		t.Error("someone without the Gravity Override (mp_gravityboots 2)")
	}
	if throws < 10 {
		t.Errorf("%d pad throws in 6 minutes: the bots don't fly the pads", throws)
	}
	if falls > 1 {
		t.Errorf("%d falls into the void", falls)
	}
	if !plants["A"] || !plants["B"] {
		t.Errorf("planted at %v: want both sites", plants)
	}
	if len(s.Map.Nav) == 0 || s.cvar("mp_gravityboots") != "2" || s.Map.Category != content.Competitive {
		t.Fatalf("nav %d, boots rule %q", len(s.Map.Nav), s.cvar("mp_gravityboots"))
	}
}
