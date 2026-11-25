package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// LabBhop is the movement lab: a 120 m runway whose Rules pin source movement
// and auto bunny hopping, whatever the server runs (sim/cases bhop-*,
// strafe-*). Crate rows to hop over halfway and near the end, a step up to a
// raised deck at the far end.
func LabBhop() c.Map {
	return mapgen.New("lab_bhop").
		Add(mapgen.Arena(-10, -10, 110, 10, 4, 1, c.Concrete, c.Hazard)...).
		Add(prefabs.Crate(40, 0, -6, 1, 0.8, 1), prefabs.Crate(40, 0, 5, 1, 0.8, 1)).
		Add(prefabs.Crate(70, 0, -2, 1, 1, 4)).
		Add(c.B(95, 0, -10, 110, 0.5, 10, c.Metal)).
		TeamSpawns(c.Vibe, [3]float64{-6, 0, -4}, [3]float64{-6, 0, -2}).
		TeamSpawns(c.Artisan, [3]float64{-6, 0, 2}, [3]float64{-6, 0, 4}).
		BuyAroundSpawns(2).
		Rule("sv_movement", "source").
		Rule("sv_autobunnyhopping", "1").
		Meta("Movement lab: the map pins source movement and auto bunny hop. Strafe-jump down the runway.", 4).
		Category(c.Training).
		Map()
}
