// Package mapgen has layout helpers for building maps: enclosures, rooms,
// spawn sets. Prefer adding a helper here over hand-placing boxes in a map.
package mapgen

import c "github.com/gigurra/counterslop/internal/content"

// Builder accumulates boxes and spawns for a map.
type Builder struct{ m c.Map }

func New(name string) *Builder { return &Builder{m: c.Map{Name: name}} }

func (b *Builder) Add(boxes ...c.Box) *Builder { b.m.Boxes = append(b.m.Boxes, boxes...); return b }

func (b *Builder) Spawns(s ...[3]float64) *Builder { b.m.Spawns = append(b.m.Spawns, s...); return b }

// Item places a weapon pickup.
func (b *Builder) Item(weapon string, x, y, z float64) *Builder {
	b.m.Items = append(b.m.Items, c.Item{Weapon: weapon, Pos: [3]float64{x, y, z}})
	return b
}

func (b *Builder) Map() c.Map { return b.m }

// Enclosure is a square arena: a floor slab from -half to half (top at y=0)
// and four walls of thickness t and height h around it.
func Enclosure(half, h, t float64, floor, wall string) []c.Box {
	return []c.Box{
		c.B(-half, -1, -half, half, 0, half, floor),
		c.B(-half-t, 0, -half-t, half+t, h, -half, wall),
		c.B(-half-t, 0, half, half+t, h, half+t, wall),
		c.B(-half-t, 0, -half, -half, h, half, wall),
		c.B(half, 0, -half, half+t, h, half, wall),
	}
}
