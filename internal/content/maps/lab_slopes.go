package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// LabSlopes is the sloped-geometry test yard (wedges, internal/sim/wedge.go):
// four ramps of 15, 30, 45 and 60 degrees up to ledges (the last one too steep
// to walk: you slide back), a deck you walk off down a ramp, a surf ridge, and
// a ramp leaning on a wall to shoot and throw at.
func LabSlopes() c.Map {
	b := mapgen.New("lab_slopes").
		Add(mapgen.Arena(-30, -30, 30, 30, 4, 1, c.Concrete, c.Brick)...)
	// The ramp row: each 4 m run, 3 m wide, rising towards +x onto a 3 m ledge.
	for i, rise := range []float64{1.07, 2.31, 4, 6.93} {
		z0 := -26 + float64(i)*5
		b.Add(prefabs.Ramp(-24, z0, -20, z0+3, 0, rise, c.RampPX, c.Concrete),
			c.B(-20, 0, z0, -17, rise, z0+3, c.Brick))
	}
	// The deck: a 2 m platform with a 30 degree ramp down its east side (and
	// one down its north side along z), stairs-free.
	b.Add(c.B(-6, 0, -26, 0, 2, -18, c.Metal),
		prefabs.Ramp(0, -26, 3.46, -22, 0, 2, c.RampNX, c.Metal),
		prefabs.Ramp(-6, -18, -2, -14.54, 0, 2, c.RampNZ, c.Metal))
	// The surf ridge: 63 degree sides, 6 m high, 30 m long along z.
	b.Add(prefabs.SurfRamp(false, 14, -26, 4, 0, 6, 3, c.Metal)...)
	// A ramp against a wall: a target for bullets, grenades and sight lines.
	b.Add(c.B(-4, 0, 14, 4, 3, 15, c.Brick),
		prefabs.Ramp(-4, 10, 4, 14, 0, 2.5, c.RampPZ, c.Crate))
	return b.
		TeamSpawns(c.Vibe, [3]float64{-22, 0, 22}, [3]float64{-20, 0, 22}).
		TeamSpawns(c.Artisan, [3]float64{20, 0, 22}, [3]float64{22, 0, 22}).
		BuyAroundSpawns(2).
		Meta("Slopes lab: ramps from 15 to 60 degrees, a deck with ramps down, a surf ridge, a ramp to shoot at.", 4).
		Category(c.Training).
		Map()
}
