package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// GsKiosk is the small GPU Drop map: a corner computer shop. Four boxed GPUs
// sit on two low shelves in the stockroom (east, the staff's side), which has
// a door to the shop floor and gaps north and south. The scalpers come in
// from the street (west) and carry the boxes out the front entrance (west) or
// the loading dock (north). It is also the GPU Drop rules' test map
// (internal/game tests, sim/cases gpu-*).
func GsKiosk() c.Map {
	b := mapgen.New("gs_kiosk")
	b.Mode(c.ModeGPUDrop)
	b.Add(mapgen.Enclosure(24, 6, 1, c.Concrete, c.Plaster)...)

	// The stockroom: x 10..24, z -10.5..10.5; a door west (z -2..2) and gaps
	// north and south (x 15..18).
	b.Add(
		c.B(10, 0, -10.5, 10.5, 4, -2, c.Plaster), c.B(10, 0, 2, 10.5, 4, 10.5, c.Plaster),
		c.B(10.5, 0, 10, 15, 4, 10.5, c.Plaster), c.B(18, 0, 10, 24, 4, 10.5, c.Plaster),
		c.B(10.5, 0, -10.5, 15, 4, -10, c.Plaster), c.B(18, 0, -10.5, 24, 4, -10, c.Plaster),
	)
	// Two low stock shelves; the GPUs stand on their tops at the front lip.
	b.Add(c.B(20, 0, -6, 22, 0.9, -2, c.Metal), c.B(20, 0, 2, 22, 0.9, 6, c.Metal))
	for _, z := range []float64{-5, -3, 3, 5} {
		b.GPU(20.35, 0.9, z, 1.5708) // facing the aisle (-x), on the shelf's front lip
	}

	// The shop floor: a checkout counter and two gondolas.
	b.Add(c.B(-6, 0, -3, -3, 1.1, 3, c.Desk))
	b.Add(c.B(-4, 0, 14, 6, 1.8, 15.5, c.Metal), c.B(-4, 0, -15.5, 6, 1.8, -14, c.Metal))

	b.Exit("entrance", -24, 0, -4, -20, 3, 4)
	b.Exit("dock", 0, 0, 20, 8, 3, 24)

	b.TeamSpawns(c.Vibe, [3]float64{-20, 0, -8}, [3]float64{-20, 0, 8}, [3]float64{-17, 0, 0})
	b.TeamSpawns(c.Artisan, [3]float64{14, 0, -6}, [3]float64{14, 0, 6}, [3]float64{15, 0, 0})
	b.BuyZone(c.Vibe, -24, -0.5, -12, -14, 3, 12)
	b.BuyZone(c.Artisan, 10.5, -0.5, -10, 24, 3, 10)

	b.Nav("vibe", -19, 0, 0, "entrance", "wN", "wS").
		Nav("entrance", -22, 0, 0).
		Nav("wN", -8, 0, 7, "mN").
		Nav("wS", -8, 0, -7, "mS").
		Nav("mN", 6, 0, 7, "door", "nOut").
		Nav("mS", 6, 0, -7, "door", "sOut").
		Nav("door", 9, 0, 0, "stock").
		Nav("nOut", 8, 0, 13, "nGap", "dock").
		Nav("dock", 4, 0, 21.5).
		Nav("nGap", 16.5, 0, 12.5, "nIn").
		Nav("nIn", 16.5, 0, 8, "stock").
		Nav("sOut", 8, 0, -13, "sGap").
		Nav("sGap", 16.5, 0, -12.5, "sIn").
		Nav("sIn", 16.5, 0, -8, "stock").
		Nav("stock", 19.45, 0, 0, "gpu2", "gpu3").
		Nav("gpu2", 19.45, 0, -3, "gpu1").
		Nav("gpu1", 19.45, 0, -5).
		Nav("gpu3", 19.45, 0, 3, "gpu4").
		Nav("gpu4", 19.45, 0, 5).
		Nav("artisan", 15, 0, 0, "stock", "door")
	b.Meta("A corner computer shop: four boxed GPUs in the stockroom, out the front door or the loading dock.", 6)
	b.FillSpawns(false) // one spawn per player, in the same spawn area
	b.Category(c.GPUDrop)
	return b.Map()
}
