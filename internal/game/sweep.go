package game

import (
	"iter"
	"math"
)

// Trigger volumes swept along each command's path. One server tick runs
// every command queued for a player (up to cmdBurst after a client hitch or
// a stalled socket), so a volume sampled once a tick at the final position
// can be carried through uncounted. simTick records where each command run
// this tick left the player (Player.path); a trigger tested after it (race
// gates, race power-ups, GPU Drop exits, ticket gates, floor weapons) tests
// every step of it. A move without a command (a respawn, setpos, a
// teleport: jumped) sweeps nothing on the way.

// steps yields the straight steps p moved this tick: one per command run
// (from where the first started), then where p is now as a point (a == b)
// when that is not where the last command left it. Without a command this
// tick: just the point.
func (p *Player) steps() iter.Seq2[Vec3, Vec3] {
	return func(yield func(a, b Vec3) bool) {
		path := p.path
		for k := 0; k+1 < len(path); k++ {
			if !yield(path[k], path[k+1]) {
				return
			}
		}
		if len(path) == 0 || path[len(path)-1] != p.Pos {
			yield(p.Pos, p.Pos)
		}
	}
}

// swept: some step p moved this tick hits (hit(a, b)).
func (p *Player) swept(hit func(a, b Vec3) bool) bool {
	for a, b := range p.steps() {
		if hit(a, b) {
			return true
		}
	}
	return false
}

// jumped: p was moved, not driven (a respawn, setpos, a teleport): nothing
// is swept along the jump (the path starts again where they are).
func (p *Player) jumped() { p.path = p.path[:0] }

// nearPath: the step a -> b passes within r of q across (the ground plane)
// and within h of it up or down there (a == b: the point). A cylinder
// trigger (a pickup) swept along a step.
func nearPath(q, a, b Vec3, r, h float64) bool {
	dx, dz := b[0]-a[0], b[2]-a[2]
	t := 0.0
	if l := dx*dx + dz*dz; l > 0 {
		t = min(1, max(0, ((q[0]-a[0])*dx+(q[2]-a[2])*dz)/l))
	}
	x, y, z := a[0]+dx*t, a[1]+(b[1]-a[1])*t, a[2]+dz*t
	return math.Hypot(x-q[0], z-q[2]) < r && math.Abs(y-q[1]) < h
}
