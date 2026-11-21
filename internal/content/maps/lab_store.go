package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// LabStore is the store props test room (GPU Drop assets): one roofed store hall on a
// yard, every store prop and material under fluorescent panels and a skylight. West:
// gondola aisles and checkout lanes. Middle: demo kiosks under the skylight, price signs,
// carts by the door. East: the stockroom shelf, a TV wall, LAN desks on arcade carpet,
// and an escalator pair (up and down) to a mezzanine with more PCs.
func LabStore() c.Map {
	const (
		wallH = 5.0
		t     = 0.3
		roofT = 0.3
		y     = 0.02 // the store floor's top
		mezY  = 2.5
	)
	cool := [3]float64{0.88, 0.94, 1}
	b := mapgen.New("lab_store").
		Add(mapgen.Arena(-26, -20, 26, 18, 2.5, 1, c.Concrete, c.Brick)...).
		// The hall: x -20..20, z -14..6. Back and side walls plaster; the front has a door
		// in the middle and a glass shop window either side.
		Add(prefabs.WallGaps(true, -14.3, -20.3, 20.3, t, 0, wallH, c.Plaster)...).
		Add(prefabs.WallGaps(false, -20.3, -14, 6, t, 0, wallH, c.Plaster)...).
		Add(prefabs.WallGaps(false, 20, -14, 6, t, 0, wallH, c.Plaster)...).
		Add(prefabs.WallGaps(true, 6, -20.3, 20.3, t, 0, wallH, c.Plaster,
			prefabs.Gap{From: -16, To: -6}, prefabs.Gap{From: -2, To: 2, H: 3}, prefabs.Gap{From: 6, To: 16})...).
		Add(c.B(-16, 0, 6.05, -6, 0.5, 6.25, c.Plaster), c.B(-16, 0.5, 6.1, -6, 3.2, 6.2, c.Glass), c.B(-16, 3.2, 6, -6, wallH, 6.3, c.Plaster),
			c.B(6, 0, 6.05, 16, 0.5, 6.25, c.Plaster), c.B(6, 0.5, 6.1, 16, 3.2, 6.2, c.Glass), c.B(6, 3.2, 6, 16, wallH, 6.3, c.Plaster)).
		// A neon sign over the door.
		Add(c.B(-2.5, 3.4, 6.3, 2.5, 4.6, 6.45, c.Neon)).
		// Roof with a 8 x 6 m skylight over the kiosks.
		Add(prefabs.Slab(-20.3, -14.3, -4, 6.3, wallH+roofT, roofT, c.Concrete),
			prefabs.Slab(4, -14.3, 20.3, 6.3, wallH+roofT, roofT, c.Concrete),
			prefabs.Slab(-4, -14.3, 4, -8, wallH+roofT, roofT, c.Concrete),
			prefabs.Slab(-4, -2, 4, 6.3, wallH+roofT, roofT, c.Concrete)).
		// The store floor, and the LAN corner's carpet on it.
		Add(c.B(-20, 0, -14, 20, y, 6, c.StoreTile)).
		Add(c.B(6, y, -7.5, 19.5, y+0.02, 0.5, c.LanCarpet))
	// West: three gondola aisles, three checkout lanes towards the door.
	for _, z := range []float64{-12.5, -9.5, -6.5} {
		b.Add(prefabs.Gondola(-18, y, z, 10, true)...)
	}
	for _, x := range []float64{-17, -13.5, -10} {
		b.Add(prefabs.CheckoutLane(x, y, -2, false)...)
	}
	// Middle: kiosks under the skylight, price signs, carts by the door.
	for _, x := range []float64{-3, -0.5, 2} {
		b.Add(prefabs.Kiosk(x, y, -6)...)
	}
	b.Add(prefabs.PriceSign(-3, y, -2.5, true)...).Add(prefabs.PriceSign(2.3, y, -2.5, true)...)
	for i := range 4 {
		b.Add(prefabs.Cart(-4.5+float64(i)*1.4, y, 3.5, i)...)
	}
	// East: the stockroom shelf on the back wall, a TV wall on the east wall, LAN desks.
	b.Add(prefabs.StockShelf(6, y, -13.8, 10, true)...).
		Add(prefabs.TVWall(19.75, -12, 20, -2, y)...).
		Add(prefabs.LanDesk(7, y+0.02, -7, true, 8)...).
		Add(prefabs.LanDesk(7, y+0.02, -3.5, true, 8)...)
	// The escalator pair from the floor up to a mezzanine (x 8..18, z 1.5..5.5), its PCs.
	b.Add(prefabs.Slab(8, 1.5, 18, 5.5, mezY, 0.3, c.Metal)).
		Add(prefabs.Escalator(false, 8, 2.3, 1.0, y, mezY-y, -1, c.Escalator, c.Glass)...).
		Add(prefabs.Escalator(false, 8, 3.6, 1.0, y, mezY-y, -1, c.Escalator, c.Glass)...).
		Mover(prefabs.EscalatorMover(false, 8, 2.3, 1.0, y, mezY-y, -1, 0.75, c.Escalator),
			prefabs.EscalatorMover(false, 8, 3.6, 1.0, y, mezY-y, -1, -0.75, c.Escalator)).
		Add(prefabs.LanDesk(11, mezY, 3.5, true, 5)...)
	// Fluorescent panels over the hall (not under the skylight): lamp boxes under the
	// roof, wide spots straight down.
	for _, x := range []float64{-16, -10, 10, 16} {
		for _, z := range []float64{-11, -5, 1} {
			if x > 0 && z == 1 {
				continue // over the mezzanine: its own lamps below
			}
			b.Add(c.B(x-0.6, wallH-0.08, z-0.3, x+0.6, wallH, z+0.3, c.Lamp))
			l := c.Spot([3]float64{x, wallH - 0.1, z}, [3]float64{0, -1, 0}, cool, 14, 9, 80)
			l.Inner = 35
			b.Light(l)
		}
	}
	// The mezzanine's PCs glow under two magenta/cyan bulbs; display spots on the kiosks
	// and the TV wall.
	b.Add(c.B(12.85, 4.9, 3.35, 13.15, 5.0, 3.65, c.Lamp), c.B(15.85, 4.9, 3.35, 16.15, 5.0, 3.65, c.Lamp))
	b.Light(c.Point([3]float64{13, 4.85, 3.5}, [3]float64{1, 0.2, 0.85}, 6, 6), c.Point([3]float64{16, 4.85, 3.5}, [3]float64{0.2, 0.85, 1}, 6, 6))
	kiosks := c.Spot([3]float64{0, 4.9, -9}, [3]float64{0, -1, 0.6}, [3]float64{1, 0.92, 0.8}, 30, 10, 30)
	kiosks.Shadow = true
	tv := c.Spot([3]float64{15, 4.8, -7}, [3]float64{1, -0.6, 0}, [3]float64{0.9, 0.95, 1}, 30, 10, 28)
	b.Light(kiosks, tv)
	return b.
		TeamSpawns(c.Vibe, [3]float64{-10, 0, 13}, [3]float64{-8, 0, 13}).
		TeamSpawns(c.Artisan, [3]float64{8, 0, 13}, [3]float64{10, 0, 13}).
		BuyAroundSpawns(2).
		Meta("Store lab: every GPU Drop prop and material in one lit hall, with an escalator pair to a mezzanine.", 4).
		Category(c.Training).
		Map()
}
