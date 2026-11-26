package maps

import (
	"slices"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// Labs are test maps for the engine: not in the rotation or the selector, loaded
// with `changelevel <name>` (rcon/admin) and by sim scenarios and the gallery.
var Labs = map[string]func() c.Map{
	"lab_bhop":   LabBhop,
	"lab_boots":  LabBoots,
	"lab_harbor": LabHarbor,
	"lab_kart":   LabKart,
	"lab_lights": LabLights,
	"lab_store":  LabStore,
	"lab_slopes": LabSlopes,
}

// LabNames lists the labs, sorted.
func LabNames() []string {
	out := make([]string, 0, len(Labs))
	for n := range Labs {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// Find is a playable map (built-in or custom) or a lab by name, or nil.
func Find(name string) func() c.Map {
	if b := All[name]; b != nil {
		return b
	}
	if b := Labs[name]; b != nil {
		return b
	}
	return customBuild(name)
}

// LabLights is the lighting test room: a roofed building of two rooms on a sunny
// yard. West: closed roof, coloured lamps, a ceiling of fluorescent panels (one
// dying), a crate stack. East: a skylight and a window band letting the sun in,
// shadow-casting spots on a crate and a pillar, a shadowed bare bulb. The yard
// has a street lamp.
func LabLights() c.Map {
	const (
		wallH = 4.0
		t     = 0.3
		roofT = 0.3
	)
	warm := [3]float64{1, 0.92, 0.78}
	cool := [3]float64{0.85, 0.93, 1}
	b := mapgen.New("lab_lights").
		Add(mapgen.Arena(-24, -18, 24, 18, 2.5, 1, c.Concrete, c.Brick)...).
		// The building: x -20..20, z -14..2, a wall between the rooms with a door.
		Add(prefabs.WallGaps(true, -14.3, -20.3, 20.3, t, 0, wallH, c.Plaster)...).
		Add(prefabs.WallGaps(false, -20.3, -14, 2, t, 0, wallH, c.Plaster)...).
		Add(prefabs.WallGaps(false, 20, -14, 2, t, 0, wallH, c.Plaster)...).
		Add(prefabs.WallGaps(false, -0.15, -14, 2, t, 0, wallH, c.Plaster, prefabs.Gap{From: -3, To: -1, H: 2.5})...).
		// South wall: a door into each room, and a window band (sill 1.2 m, lintel 2.6 m) in the east.
		Add(prefabs.WallGaps(true, 2, -20.3, 20.3, t, 0, wallH, c.Plaster,
			prefabs.Gap{From: -12, To: -10, H: 2.5}, prefabs.Gap{From: 4, To: 15}, prefabs.Gap{From: 17, To: 19, H: 2.5})...).
		Add(c.B(4, 0, 2, 15, 1.2, 2+t, c.Plaster), c.B(4, 2.6, 2, 15, wallH, 2+t, c.Plaster)).
		// Roof: closed over the west room; a 4 x 4 m skylight in the east one.
		Add(prefabs.Slab(-20.3, -14.3, 0, 2.3, wallH+roofT, roofT, c.Concrete),
			prefabs.Slab(0, -14.3, 8, 2.3, wallH+roofT, roofT, c.Concrete),
			prefabs.Slab(12, -14.3, 20.3, 2.3, wallH+roofT, roofT, c.Concrete),
			prefabs.Slab(8, -14.3, 12, -9, wallH+roofT, roofT, c.Concrete),
			prefabs.Slab(8, -5, 12, 2.3, wallH+roofT, roofT, c.Concrete)).
		// West room: carpet, a crate stack under the coloured lamps.
		Add(c.B(-20, 0, -14, -0.15, 0.02, 2, c.Carpet)).
		Add(prefabs.CratePyramid(-11, -8, 1.2)...).
		// East room: a crate for the display spot, a pillar between the bulb and the wall.
		Add(prefabs.Crate(4.4, 0, -9, 1.2, 1.2, 1.2), prefabs.Pillar(13, -11.5, 0.6, wallH, c.Concrete)).
		Add(prefabs.Crate(9.4, 0, -7.6, 1.2, 1.2, 1.2)).
		// The street lamp in the yard.
		Add(c.B(-0.1, 0, 9.9, 0.1, 3.4, 10.1, c.Metal), c.B(-0.35, 3.4, 9.75, 0.35, 3.55, 10.25, c.Lamp))
	// Fluorescent panels across the west ceiling: lamp boxes just under the roof,
	// each with a wide spot straight down (a panel lights the room, not the ceiling);
	// the one at (-16, -2) is dying.
	for _, x := range []float64{-16, -6} {
		for _, z := range []float64{-11, -2} {
			b.Add(c.B(x-0.6, wallH-0.08, z-0.3, x+0.6, wallH, z+0.3, c.Lamp))
			l := c.Spot([3]float64{x, wallH - 0.1, z}, [3]float64{0, -1, 0}, cool, 14, 9, 80)
			l.Inner = 35
			if x == -16 && z == -2 {
				l.Flicker = 7
			}
			b.Light(l)
		}
	}
	// Coloured lamps over the crate stack.
	for _, cl := range []struct {
		x, z float64
		col  [3]float64
	}{{-14, -8, [3]float64{1, 0.15, 0.1}}, {-11, -5, [3]float64{0.15, 1, 0.25}}, {-8, -8, [3]float64{0.2, 0.35, 1}}} {
		b.Add(c.B(cl.x-0.15, 2.9, cl.z-0.15, cl.x+0.15, 3.0, cl.z+0.15, c.Lamp))
		b.Light(c.Point([3]float64{cl.x, 2.85, cl.z}, cl.col, 6, 6))
	}
	// East: two shadowed display spots, a shadowed bare bulb behind the pillar.
	spotA := c.Spot([3]float64{3, 3.8, -6}, [3]float64{0.5, -1, -1}, warm, 30, 10, 28)
	spotA.Shadow = true
	spotB := c.Spot([3]float64{17, 3.8, -2}, [3]float64{-0.9, -1, -0.8}, [3]float64{1, 0.6, 0.85}, 30, 12, 22)
	spotB.Shadow = true
	bulb := c.Point([3]float64{15, 2.2, -12.5}, warm, 6, 8)
	bulb.Shadow = true
	b.Add(c.B(14.9, 2.25, -12.6, 15.1, 2.4, -12.4, c.Lamp))
	b.Light(spotA, spotB, bulb)
	// The street lamp, and a green exit sign glow over the east door.
	b.Light(c.Point([3]float64{0, 3.35, 10}, warm, 10, 10))
	b.Add(c.B(17.5, 2.6, 2.3, 18.5, 2.9, 2.4, c.Lamp))
	b.Light(c.Point([3]float64{18, 2.75, 2.6}, [3]float64{0.2, 1, 0.35}, 2, 4))
	return b.
		TeamSpawns(c.Vibe, [3]float64{-16, 0, 12}, [3]float64{-14, 0, 12}).
		TeamSpawns(c.Artisan, [3]float64{14, 0, 12}, [3]float64{16, 0, 12}).
		BuyAroundSpawns(2).
		Meta("Lighting lab: coloured lamps, fluorescent panels, shadowed spots, a skylight and a window.", 4).
		Category(c.Training).
		Map()
}
