//gojs:shared

package game

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// Wallbangs (GitHub #9): a bullet goes on through thin walls and props.
// Each map box it crosses costs "wood metres": the material's resistance
// (wallMat, wood = 1 per metre) × how far the ray goes inside it (a hollow
// prop, a crate or a car, only counts its two skins, however deep it is). A
// weapon gets through up to its wallPen wood metres in all (weapons.json
// `wallPen`; 0: shotguns, melee); each wall keeps wallKeep × the share of
// that power it leaves, so damage falls with the thickness × the material
// and a wall over the max stops the bullet. Never through a floor or a
// ceiling (entered through its top or bottom face), the map's outer shell, or
// more than wallMaxPasses walls. The server's shots (serverShot) and the
// client's own predicted holes trace the same walls (this file is generated
// to web/game/shared.gen.js). mp_wallbang 0: no wall lets a bullet through.

// wallKeep: the flat share of the damage a bullet keeps through any wall
// (on top of the thickness cost).
const wallKeep = 0.85

// wallMaxPasses: a bullet goes through at most this many walls.
const wallMaxPasses = 3

// wallMat is a material's resistance in wood metres per metre (wood = 1;
// < 0: never through), and its skin: > 0 for a hollow prop (a crate, a car,
// a desk): the ray pays at most two skins of it, however deep the box.
func wallMat(kind string) (float64, float64) {
	switch kind {
	case "crate", "carton": // planks, cardboard
		return 1, 0.05
	case "desk", "checkout": // laminate over a hollow frame
		return 1, 0.03
	case "container", "car", "rack", "shelf", "pctower": // sheet steel
		return 8, 0.01
	case "awning": // cloth
		return 0.3, 0
	case "glass":
		return 0.6, 0
	case "plaster": // drywall
		return 1.5, 0
	case "screen", "lamp", "neon":
		return 2, 0
	case "metal", "handrail", "escalator": // thin metal: decks, rails, treads
		return 3, 0
	case "hazard": // striped barriers
		return 3.5, 0
	case "stucco":
		return 4, 0
	case "brick":
		return 5, 0
	case "concrete":
		return 7, 0
	}
	return -1, 0 // floors (sand, carpet, storetile, ...) and anything unknown: never
}

// WallPass is one wall on a shot's ray: it went in at In and out at Out
// (metres from the eye) keeping Keep of what it had; Keep 0: it stopped the
// bullet at In (Out is In).
type WallPass struct {
	In   float64 `json:"in"`
	Out  float64 `json:"out"`
	Keep float64 `json:"keep"`
}

// rayBox is the ray o + t·d's entry and exit distance through the box and
// the axis it went in through (-1 if o is inside; t0 < 0: a miss).
func rayBox(o, d Vec3, mn, mx [3]float64) (float64, float64, int) {
	t0, t1, ax := 0.0, math.Inf(1), -1
	for a := 0; a < 3; a++ {
		if math.Abs(d[a]) < 1e-12 {
			if o[a] < mn[a] || o[a] > mx[a] {
				return -1, -1, -1
			}
			continue
		}
		ta, tb := (mn[a]-o[a])/d[a], (mx[a]-o[a])/d[a]
		if ta > tb {
			ta, tb = tb, ta
		}
		if ta > t0 {
			t0, ax = ta, a
		}
		t1 = math.Min(t1, tb)
		if t0 > t1 {
			return -1, -1, -1
		}
	}
	return t0, t1, ax
}

// wallBox: one box on the ray.
type wallBox struct {
	in, out float64
	ax      int
	i       int
}

// shellBox: the box reaches the map's outer bounds lo/hi across (x or z):
// the map's shell, never shot through.
func shellBox(b content.Box, lo, hi [3]float64) bool {
	return b.Min[0] <= lo[0] || b.Min[2] <= lo[2] || b.Max[0] >= hi[0] || b.Max[2] >= hi[2]
}

// MapBounds is the box around every one of boxes.
func MapBounds(boxes []content.Box) ([3]float64, [3]float64) {
	lo, hi := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}, [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i := range boxes {
		for a := 0; a < 3; a++ {
			lo[a] = math.Min(lo[a], boxes[i].Min[a])
			hi[a] = math.Max(hi[a], boxes[i].Max[a])
		}
	}
	return lo, hi
}

// WallTrace follows a shot's ray from o along d (unit) through the map's
// boxes up to rng metres with power wood metres to spend (the weapon's
// wallPen; 0: the first wall stops it). It returns the walls in order; the
// last one stops the bullet when its Keep is 0 (else the ray is clear to rng).
// lo/hi: MapBounds(boxes).
func WallTrace(boxes []content.Box, lo, hi [3]float64, o, d Vec3, rng, power float64) []WallPass {
	var on []wallBox
	for i := range boxes {
		in, out, ax := rayBox(o, d, boxes[i].Min, boxes[i].Max)
		if in < 0 || in >= rng {
			continue
		}
		on = append(on, wallBox{in: in, out: out, ax: ax, i: i})
	}
	for i := 1; i < len(on); i++ { // by entry, then box order (stable insertion: few boxes on a ray)
		for j := i; j > 0 && (on[j].in < on[j-1].in || on[j].in == on[j-1].in && on[j].i < on[j-1].i); j-- {
			on[j], on[j-1] = on[j-1], on[j]
		}
	}
	var walls []WallPass
	left, at := power, 0.0 // wood metres left; how far the bullet is (a box inside one already passed only counts beyond it)
	for _, w := range on {
		if w.out <= at {
			continue // inside a wall already passed
		}
		in := math.Max(w.in, at)
		b := boxes[w.i]
		k, skin := wallMat(b.Kind)
		if k < 0 || w.ax == 1 || w.ax < 0 || shellBox(b, lo, hi) || left <= 0 || len(walls) == wallMaxPasses {
			return append(walls, WallPass{In: in, Out: in, Keep: 0})
		}
		depth := w.out - in
		if skin > 0 {
			depth = math.Min(depth, 2*skin)
		}
		cost := float64(k * depth)
		if cost >= left {
			return append(walls, WallPass{In: in, Out: in, Keep: 0})
		}
		keep := float64(wallKeep * (1 - cost/left))
		left -= cost
		walls = append(walls, WallPass{In: in, Out: w.out, Keep: keep})
		at = w.out
	}
	return walls
}

// WallStop is where the walls stop the bullet: the stopping wall's In, or
// rng when it gets through all of them.
func WallStop(walls []WallPass, rng float64) float64 {
	if n := len(walls); n > 0 && walls[n-1].Keep == 0 {
		return walls[n-1].In
	}
	return rng
}

// WallKeepAt is the share of the damage the walls before t leave the bullet.
func WallKeepAt(walls []WallPass, t float64) float64 {
	k := 1.0
	for _, w := range walls {
		if w.Keep > 0 && w.Out <= t {
			k *= w.Keep
		}
	}
	return k
}
