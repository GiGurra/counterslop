package game

import (
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// Who the Big Diff's blast kills are credited to (QA pass). The planter gets
// the enemies it kills (as in CS); a Vibe teammate in the blast is no team
// kill (no frag lost, no penalty); and once the planter has left, the blast
// is a world kill: no victim is shown killing themselves.
func TestBlastKillCredit(t *testing.T) {
	setup := func(t *testing.T) (*Server, *Player, *Player, *Player, Vec3) {
		s, v, a := roundServer(t)
		v2 := s.join("3.3.3.3")
		v2.ready, v2.Team = true, content.Vibe
		s.round.Phase = PhasePlanted
		s.round.Bomb.Carrier = v.ID
		at := Vec3{0, 0, 0}
		v.Pos = Vec3{40, 0, 40} // planter out of range
		a.Pos, v2.Pos = Vec3{1, 0, 0}, Vec3{0, 0, 1}
		for _, p := range []*Player{v, a, v2} {
			p.Money = 1000
			msgs(p)
		}
		return s, v, a, v2, at
	}

	t.Run("planter here", func(t *testing.T) {
		s, v, a, v2, at := setup(t)
		s.blast(at)
		if !a.Dead || !v2.Dead {
			t.Fatalf("both in the blast should die: a %v v2 %v", a.Dead, v2.Dead)
		}
		if v.Kills != 1 {
			t.Errorf("planter kills %d, want 1 (the enemy; the teammate is no team kill)", v.Kills)
		}
		if v.Money < 1000 {
			t.Errorf("planter money %d: charged a team-kill penalty for the blast", v.Money)
		}
	})

	t.Run("planter gone", func(t *testing.T) {
		s, v, a, v2, at := setup(t)
		delete(s.players, v.ID) // kicked / disconnected after planting
		s.blast(at)
		for _, k := range ofType(a, "kill") {
			if k["k"] == k["v"] {
				t.Errorf("blast kill shown as a suicide: %v", k)
			}
			if k["k"] != float64(0) {
				t.Errorf("planter gone: want a world kill (k 0), got %v", k)
			}
		}
		if a.Kills != 0 || v2.Kills != 0 || a.Deaths != 1 || v2.Deaths != 1 {
			t.Errorf("kills a %d v2 %d, deaths a %d v2 %d", a.Kills, v2.Kills, a.Deaths, v2.Deaths)
		}
	})
}
