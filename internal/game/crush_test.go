package game

import (
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// A floor, a wall at x 2..2.5 and a slab sliding along +x into it at speed v.
func crushMap(v float64) content.Map {
	return content.Map{Name: "crusher", Boxes: []content.Box{content.B(-30, -1, -30, 30, 0, 30, content.Concrete), content.B(2, 0, -5, 2.5, 3, 5, content.Concrete)},
		Spawns: [][3]float64{{1.6, 0, 0}},
		Movers: []content.Mover{{Kind: content.MoverSlide, Mat: content.Metal, Min: [3]float64{-3, 0, -1}, Max: [3]float64{-2, 2, 1}, Move: [3]float64{4, 0, 0}, Period: 8 / v}}}
}

// A mover pushing a player into a wall crushes them: a slow one hurts a
// little each command and kills in a fraction of a second, a fast one kills
// at once; the kill feed says "crush", with no killer. The body never goes
// into the wall.
func TestMoverCrushesAPlayer(t *testing.T) {
	for _, tc := range []struct {
		v        float64
		minTicks int // commands from the first damage to the kill, at least
	}{{0.5, 10}, {20, 0}} {
		s := authServer(t)
		s.Map = crushMap(tc.v)
		p := s.join("1.1.1.1")
		p.ready, p.Team = true, content.Vibe
		p.body = sim.NewBody(sim.Vec3{1.6, 0, 0})
		p.Pos, p.HP = Vec3(p.body.Pos), 100
		msgs(p)
		seq, first, killed := 0, -1, -1
		for i := range int(8/tc.v*SimHz) + 10 {
			seq++
			s.queueCmds(p, []Cmd{{Seq: seq}})
			stepSim(s)
			if p.body.Pos[0] > 2-sim.R+1e-9 {
				t.Fatalf("v %v tick %d: pushed into the wall: %v", tc.v, i, p.body.Pos)
			}
			if p.HP < 100 && first < 0 {
				first = i
			}
			if p.Dead {
				killed = i
				break
			}
		}
		if killed < 0 {
			t.Fatalf("v %v: not crushed (hp %d at %v)", tc.v, p.HP, p.body.Pos)
		}
		if killed-first < tc.minTicks || tc.minTicks == 0 && killed != first {
			t.Fatalf("v %v: hurt at command %d, dead at %d, want at least %d between (0: at once)", tc.v, first, killed, tc.minTicks)
		}
		k := ofType(p, "kill")
		if len(k) != 1 || k[0]["w"] != "crush" || k[0]["k"] != float64(0) {
			t.Fatalf("v %v: kill messages %v, want one by the world with crush", tc.v, k)
		}
	}
}

func TestCrushDamage(t *testing.T) {
	for _, tc := range []struct {
		d    float64
		want int
	}{{0, 0}, {1e-6, 1}, {0.005, 3}, {0.01, 5}, {0.049, 25}, {0.05, 1 << 20}, {2, 1 << 20}} {
		if got := crushDamage(tc.d); got != tc.want {
			t.Fatalf("crushDamage(%v) = %d, want %d", tc.d, got, tc.want)
		}
	}
}
