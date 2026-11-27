package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// raceKit builds a floating-roads race track (Slop n Roll Racing,
// docs/racing.md) out of axis-aligned pieces: roads (decks 1 m thick, the
// surface at y), low rails along their edges, pillars down into the void,
// gate arches with a banner, corner signs. The look is the track's own:
// its materials.
type raceKit struct {
	b                *mapgen.Builder
	road, rail, post string // the decks, the rails, pillars and arch posts
	banner           string // an arch's banner
}

const (
	kitRail = 0.7 // rail height: over a car's step (0.6), under its roof
	kitRT   = 0.4 // rail thickness
)

// deck: a road's deck, its surface at y.
func (k *raceKit) deck(x0, z0, x1, z1, y float64) { k.b.Add(c.B(x0, y-1, z0, x1, y, z1, k.road)) }

// railX: a rail along x (from x0 to x1) on the z = z edge (outside: side -1 / +1 along z), standing on y.
func (k *raceKit) railX(x0, x1, z, side, y float64) {
	if side < 0 {
		k.b.Add(c.B(x0, y, z-kitRT, x1, y+kitRail, z, k.rail))
	} else {
		k.b.Add(c.B(x0, y, z, x1, y+kitRail, z+kitRT, k.rail))
	}
}

// railZ: a rail along z (from z0 to z1) on the x = x edge.
func (k *raceKit) railZ(z0, z1, x, side, y float64) {
	if side < 0 {
		k.b.Add(c.B(x-kitRT, y, z0, x, y+kitRail, z1, k.rail))
	} else {
		k.b.Add(c.B(x, y, z0, x+kitRT, y+kitRail, z1, k.rail))
	}
}

// pillar: a 2 m square pillar from far down up to top, under (x, z).
func (k *raceKit) pillar(x, z, top float64) { k.b.Add(c.B(x-1, -30, z-1, x+1, top, z+1, k.post)) }

// archX: a gate's posts beside a road along x (z0..z1) at x, and its banner
// over it (y: the road's surface).
func (k *raceKit) archX(x, z0, z1, y float64) {
	k.b.Add(c.B(x-0.3, y+0.05, z0-1.1, x+0.3, y+5, z0-kitRT-0.05, k.post), c.B(x-0.3, y+0.05, z1+kitRT+0.05, x+0.3, y+5, z1+1.1, k.post),
		c.B(x-0.35, y+5, z0-1.15, x+0.35, y+5.6, z1+1.15, k.banner))
}

// archZ: the same across a road along z (x0..x1) at z.
func (k *raceKit) archZ(z, x0, x1, y float64) {
	k.b.Add(c.B(x0-1.1, y+0.05, z-0.3, x0-kitRT-0.05, y+5, z+0.3, k.post), c.B(x1+kitRT+0.05, y+0.05, z-0.3, x1+1.1, y+5, z+0.3, k.post),
		c.B(x0-1.15, y+5, z-0.35, x1+1.15, y+5.6, z+0.35, k.banner))
}

// sign: a board (x0..x1, z0..z1, one of them thin) 1..3 m over y in
// material mat, on a post down into the void.
func (k *raceKit) sign(x0, y, z0, x1, z1 float64, mat string) {
	cx, cz := (x0+x1)/2, (z0+z1)/2
	k.b.Add(c.B(x0, y+1, z0, x1, y+3, z1, mat), c.B(cx-0.3, -30, cz-0.3, cx+0.3, y+1, cz+0.3, k.post))
}

// gate: a checkpoint (the box a racer drives through) with its respawn spot
// and heading.
func (k *raceKit) gate(name string, x0, y0, z0, x1, y1, z1, sx, sy, sz, yaw float64) {
	k.b.Checkpoint(c.Checkpoint{Name: name, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}, Spawn: [3]float64{sx, sy, sz}, Yaw: yaw})
}
