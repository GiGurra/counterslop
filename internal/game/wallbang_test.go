package game

import (
	"math"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// wallRig: a 100 m sand floor (the map's shell all round) and one wall
// across +x at x = 0, its thickness th; the ray goes along +x at eye height.
func wallRig(kind string, th float64, more ...content.Box) ([]content.Box, [3]float64, [3]float64) {
	boxes := append([]content.Box{content.B(-50, -1, -50, 50, 0, 50, content.Sand), content.B(0, 0, -2, th, 3, 2, kind)}, more...)
	lo, hi := MapBounds(boxes)
	return boxes, lo, hi
}

// Materials and thickness (GitHub #9): wood, plaster, glass and thin metal
// go through easily; brick and concrete barely (only the sniper, thin ones);
// shotguns and melee never; a hollow prop (a crate) only costs its skins.
func TestWallTraceMaterials(t *testing.T) {
	r := registry(t)
	o, d := Vec3{-5, 1.5, 0}, Vec3{1, 0, 0}
	for _, tc := range []struct {
		kind string
		th   float64
		w    string
		keep float64 // the share left after the wall; 0: stopped
	}{
		{"crate", 2, "prompt", 0.85 * (1 - 0.1/1.4)}, // hollow: two 5 cm skins, however deep
		{"crate", 2, "stub", 0.85 * (1 - 0.1/0.5)},
		{"crate", 2, "tokenburner", 0}, // shotguns: no wallbangs
		{"crate", 2, "keyboard", 0},    // melee
		{"plaster", 0.2, "prompt", 0.85 * (1 - 0.3/1.4)},
		{"plaster", 1, "prompt", 0},                        // 1.5 wood m: over a rifle's 1.4
		{"plaster", 1, "codereview", 0.85 * (1 - 1.5/2.5)}, // the sniper still
		{"glass", 0.15, "autocomplete", 0.85 * (1 - 0.09/0.6)},
		{"metal", 0.05, "prompt", 0.85 * (1 - 0.15/1.4)},
		{"container", 2.4, "linter", 0.85 * (1 - 0.16/1.2)},
		{"brick", 0.3, "prompt", 0}, // brick: barely
		{"brick", 0.3, "codereview", 0.85 * (1 - 1.5/2.5)},
		{"brick", 1, "codereview", 0},
		{"concrete", 0.25, "prompt", 0},
		{"concrete", 0.25, "codereview", 0.85 * (1 - 1.75/2.5)},
		{"concrete", 1, "codereview", 0},
		{"stucco", 0.5, "codereview", 0.85 * (1 - 2/2.5)},
		{"sand", 0.2, "codereview", 0}, // floor materials: never
	} {
		boxes, lo, hi := wallRig(tc.kind, tc.th)
		w := r.Get(tc.w)
		walls := WallTrace(boxes, lo, hi, o, d, 100, w.WallPen)
		if len(walls) != 1 || walls[0].In != 5 {
			t.Fatalf("%s %.2f m %s: walls %+v", tc.kind, tc.th, tc.w, walls)
		}
		if math.Abs(walls[0].Keep-tc.keep) > 1e-9 {
			t.Errorf("%s %.2f m, %s: keep %.3f, want %.3f", tc.kind, tc.th, tc.w, walls[0].Keep, tc.keep)
		}
		if tc.keep > 0 && walls[0].Out != 5+tc.th {
			t.Errorf("%s: out at %v", tc.kind, walls[0].Out)
		}
		stop := WallStop(walls, 100)
		if (tc.keep == 0) != (stop == 5) {
			t.Errorf("%s %s: stops at %v", tc.kind, tc.w, stop)
		}
	}
}

// Never through a floor or a ceiling (a face entered from above or below),
// the map's outer shell, or more than three walls; the power left shrinks
// with each wall.
func TestWallTraceNever(t *testing.T) {
	const awp = 2.5
	// Down into a crate's top: the top face.
	boxes, lo, hi := wallRig("crate", 2)
	if w := WallTrace(boxes, lo, hi, Vec3{1, 5, 0}, Vec3{0, -1, 0}, 100, awp); len(w) != 1 || w[0].Keep != 0 || w[0].In != 2 {
		t.Fatalf("down into a crate: %+v", w)
	}
	// The floor.
	if w := WallTrace(boxes, lo, hi, Vec3{-5, 1.5, 0}, Vec3{0, -1, 0}, 100, awp); len(w) != 1 || w[0].Keep != 0 {
		t.Fatalf("the floor: %+v", w)
	}
	// The shell: a thin plaster wall that is the map's edge.
	shell := []content.Box{content.B(-50, -1, -50, 50, 0, 50, content.Sand), content.B(50, 0, -50, 50.1, 3, 50, content.Plaster)}
	lo, hi = MapBounds(shell)
	if w := WallTrace(shell, lo, hi, Vec3{40, 1.5, 0}, Vec3{1, 0, 0}, 100, awp); len(w) != 1 || w[0].Keep != 0 {
		t.Fatalf("the shell: %+v", w)
	}
	// Four glass panes: three pass, the fourth stops it.
	panes := []content.Box{content.B(1, 0, -2, 1.1, 3, 2, content.Glass), content.B(2, 0, -2, 2.1, 3, 2, content.Glass), content.B(3, 0, -2, 3.1, 3, 2, content.Glass)}
	boxes, lo, hi = wallRig("glass", 0.1, panes...)
	w := WallTrace(boxes, lo, hi, Vec3{-5, 1.5, 0}, Vec3{1, 0, 0}, 100, awp)
	if len(w) != 4 || w[2].Keep == 0 || w[3].Keep != 0 || w[3].In != 8 {
		t.Fatalf("four panes: %+v", w)
	}
	// The power left: two 0.2 m plaster walls; the second costs a bigger share of it.
	boxes, lo, hi = wallRig("plaster", 0.2, content.B(2, 0, -2, 2.2, 3, 2, content.Plaster))
	w = WallTrace(boxes, lo, hi, Vec3{-5, 1.5, 0}, Vec3{1, 0, 0}, 100, 1.4)
	if len(w) != 2 || !(w[1].Keep < w[0].Keep) || math.Abs(w[1].Keep-0.85*(1-0.3/1.1)) > 1e-9 {
		t.Fatalf("two walls: %+v", w)
	}
	if k := WallKeepAt(w, 10); math.Abs(k-w[0].Keep*w[1].Keep) > 1e-12 || WallKeepAt(w, 6) != w[0].Keep || WallKeepAt(w, 1) != 1 {
		t.Fatalf("keep at: %v", k)
	}
	// Overlapping boxes: the overlap is paid once.
	boxes, lo, hi = wallRig("plaster", 0.2, content.B(0.1, 0, -2, 0.3, 3, 2, content.Plaster))
	w = WallTrace(boxes, lo, hi, Vec3{-5, 1.5, 0}, Vec3{1, 0, 0}, 100, 1.4)
	if len(w) != 2 || w[1].In != 5.2 || math.Abs(w[1].Keep-0.85*(1-0.15/(1.4-0.3))) > 1e-9 {
		t.Fatalf("overlap: %+v", w)
	}
}

// A wallbang kill: the sniper through a 0.2 m drywall kills a player behind it
// (105 × 0.85 × (1 - 0.3/2.5) ≈ 79 to the chest); the AK
// through 0.3 m brick deals nothing; mp_wallbang 0 stops every bullet; the
// exits are relayed as "x" (exit holes for everyone else).
func TestWallbangShots(t *testing.T) {
	for _, tc := range []struct {
		name, wall, gun, wallbang string
		th                        float64
		hurt                      bool
	}{
		{"sniper through drywall", content.Plaster, "codereview", "1", 0.2, true},
		{"rifle through drywall", content.Plaster, "prompt", "1", 0.2, true},
		{"rifle stopped by brick", content.Brick, "prompt", "1", 0.3, false},
		{"mp_wallbang 0", content.Plaster, "codereview", "0", 0.2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := authServer(t)
			s.Map = content.Map{Name: "wb", Boxes: []content.Box{content.B(-50, -1, -50, 50, 0, 50, content.Sand), content.B(-20, 0, 1, -8, 3, 1+tc.th, tc.wall)},
				Spawns: [][3]float64{{-14, 0, 6}}}
			s.cvars["weapon_spread"], s.cvars["mp_wallbang"] = "0", tc.wallbang
			a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
			a.ready, b.ready = true, true
			a.Team, b.Team = content.Vibe, content.Artisan
			a.Pos, b.Pos = Vec3{-14, 0, 6}, Vec3{-14, 0, -4} // the wall between, 5 m in front of a
			s.queueCmds(a, []Cmd{{Seq: 1}})
			s.queueCmds(b, []Cmd{{Seq: 1}})
			stepSim(s)
			a.inv.Slots["primary"] = &Held{ID: tc.gun, Ammo: 5, Reserve: 30}
			a.inv.Current = "primary"
			for seq := 2; seq < 40; seq++ {
				s.queueCmds(a, []Cmd{{Seq: seq}})
				s.queueCmds(b, []Cmd{{Seq: seq}})
				stepSim(s)
			}
			drain(b)
			pitch := math.Atan2(1.0-1.62, 10) // b's chest
			s.queueCmds(a, []Cmd{{Seq: 40, Pitch: pitch, B: BtnFire}})
			s.queueCmds(b, []Cmd{{Seq: 40}})
			stepSim(s)
			if hurt := b.HP < maxHP; hurt != tc.hurt {
				t.Fatalf("hurt %v (hp %d), want %v", hurt, b.HP, tc.hurt)
			}
			if tc.hurt && tc.gun == "prompt" && b.HP > maxHP-15 {
				t.Fatalf("the rifle through drywall: hp %d, want a real hit", b.HP)
			}
			if out := drain(b); tc.hurt != strings.Contains(out, `"x":[[`) {
				t.Fatalf("exit relay %v: %.300s", tc.hurt, out)
			}
		})
	}
}
