package game

import (
	"math"
	"slices"
)

// Bullet penetration through bodies (CS-like): a shot's ray goes through
// every player it crosses before a wall, nearest first. Each body takes the
// damage left (× its hit part's multiplier, falloff to it, then armor), and
// what goes on is that remaining damage × the weapon's `penetration`. The
// bullet stops after penMaxBodies bodies, once the damage left is below 1,
// or at the first body when penetration is 0 (shotgun pellets, melee).
// Teammates under mp_friendlyfire 0 take nothing but still slow the bullet
// (CS:GO). Walls (wallbang_shared.go) on the way cut it too: a body behind
// them takes the damage × WallKeepAt.

// penMaxBodies: a bullet hits at most this many bodies.
const penMaxBodies = 3

// bodyHit: one body on a shot's ray, t metres from the eye.
type bodyHit struct {
	q    *Player
	part Hitbox
	t    float64
}

// penStep: what one body on the ray takes.
type penStep struct {
	bodyHit
	dmg     float64 // before armor, rounded
	through int     // bodies the bullet went through before this one
	spared  bool    // a teammate under mp_friendlyfire 0: no damage
}

// penetrate orders the bodies by distance and works out each one's damage.
// stopped: the bullet ended in a body (it didn't reach the wall behind).
func penetrate(w *Weapon, bodies []bodyHit, spared func(*Player) bool) (steps []penStep, stopped bool) {
	return penetrateWalls(w, bodies, nil, spared)
}

// penetrateWalls is penetrate through walls: the walls the ray goes through
// before each body cut its damage (WallKeepAt); bodies beyond the wall that
// stops the bullet are already left out by the caller.
func penetrateWalls(w *Weapon, bodies []bodyHit, walls []WallPass, spared func(*Player) bool) (steps []penStep, stopped bool) {
	slices.SortStableFunc(bodies, func(a, b bodyHit) int {
		switch {
		case a.t < b.t:
			return -1
		case a.t > b.t:
			return 1
		}
		return a.q.ID - b.q.ID
	})
	keep := 1.0 // fraction of the weapon's damage still in the bullet
	for i, b := range bodies {
		wk := WallKeepAt(walls, b.t)
		raw := w.DamageAt(b.part.Mult, b.t) * keep * wk
		if (i > 0 || wk < 1) && raw < 1 {
			return steps, true // spent
		}
		steps = append(steps, penStep{bodyHit: b, dmg: math.Round(raw), through: i, spared: spared != nil && spared(b.q)})
		keep *= penOf(w)
		if keep <= 0 || len(steps) == penMaxBodies {
			return steps, true
		}
	}
	return steps, false
}

// penOf is the weapon's penetration, clamped to [0, 1].
func penOf(w *Weapon) float64 {
	if !(w.Pen > 0) { // 0, negative or NaN
		return 0
	}
	return min(w.Pen, 1)
}
