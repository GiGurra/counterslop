package game

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// Hitbox is one damage zone, relative to a player's feet. Axis-aligned (yaw is
// ignored); the x/z half-size is wide enough to cover the model at any yaw.
type Hitbox struct {
	Part string  `json:"part"` // head | body | legs
	Y0   float64 `json:"y0"`
	Y1   float64 `json:"y1"`
	Half float64 `json:"half"` // half-size in x and z
	Mult float64 `json:"mult"` // damage multiplier
}

// HitboxSet is sent to clients in welcome, so cl_showhitboxes draws exactly
// what the server tests against.
type HitboxSet struct {
	Stand  []Hitbox `json:"stand"`
	Crouch []Hitbox `json:"crouch"`
}

var Hitboxes = HitboxSet{
	Stand: []Hitbox{
		{Part: "head", Y0: 1.34, Y1: 1.84, Half: 0.27, Mult: 4},
		{Part: "body", Y0: 0.45, Y1: 1.34, Half: 0.38, Mult: 1},
		{Part: "legs", Y0: 0, Y1: 0.45, Half: 0.32, Mult: 0.75},
	},
	Crouch: []Hitbox{
		{Part: "head", Y0: 0.74, Y1: 1.24, Half: 0.27, Mult: 4},
		{Part: "body", Y0: 0.3, Y1: 0.74, Half: 0.38, Mult: 1},
		{Part: "legs", Y0: 0, Y1: 0.3, Half: 0.32, Mult: 0.75},
	},
}

// rayAABB returns the entry distance of ray o+t·d into the box, or -1.
func rayAABB(o, d, mn, mx Vec3) float64 {
	t0, t1 := 0.0, math.Inf(1)
	for a := 0; a < 3; a++ {
		if math.Abs(d[a]) < 1e-12 {
			if o[a] < mn[a] || o[a] > mx[a] {
				return -1
			}
			continue
		}
		ta, tb := (mn[a]-o[a])/d[a], (mx[a]-o[a])/d[a]
		if ta > tb {
			ta, tb = tb, ta
		}
		t0, t1 = math.Max(t0, ta), math.Min(t1, tb)
		if t0 > t1 {
			return -1
		}
	}
	return t0
}

// mapDistance is the distance to the first map box along the ray (or +Inf).
func mapDistance(m *content.Map, o, d Vec3) float64 {
	best := math.Inf(1)
	for _, b := range m.Boxes {
		if t := rayAABB(o, d, Vec3(b.Min), Vec3(b.Max)); t >= 0 && t < best {
			best = t
		}
	}
	return best
}

// hitPlayer tests the ray against a player's hitboxes; returns the nearest part hit.
func hitPlayer(p *Player, o, d Vec3) (Hitbox, float64, bool) {
	set := Hitboxes.Stand
	if p.Crouch {
		set = Hitboxes.Crouch
	}
	best, bestT, ok := Hitbox{}, math.Inf(1), false
	for _, h := range set {
		mn := Vec3{p.Pos[0] - h.Half, p.Pos[1] + h.Y0, p.Pos[2] - h.Half}
		mx := Vec3{p.Pos[0] + h.Half, p.Pos[1] + h.Y1, p.Pos[2] + h.Half}
		if t := rayAABB(o, d, mn, mx); t >= 0 && t < bestT {
			best, bestT, ok = h, t, true
		}
	}
	return best, bestT, ok
}
