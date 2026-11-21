// Package mapgen has layout helpers for building maps: enclosures, rooms,
// spawn sets. Prefer adding a helper here over hand-placing boxes in a map.
package mapgen

import c "github.com/gigurra/counterslop/internal/content"

// Builder accumulates boxes and spawns for a map.
type Builder struct{ m c.Map }

func New(name string) *Builder { return &Builder{m: c.Map{Name: name}} }

func (b *Builder) Add(boxes ...c.Box) *Builder { b.m.Boxes = append(b.m.Boxes, boxes...); return b }

// TeamSpawns adds spawns for one team (they also count as general spawns).
func (b *Builder) TeamSpawns(team string, s ...[3]float64) *Builder {
	if b.m.TeamSpawns == nil {
		b.m.TeamSpawns = map[string][][3]float64{}
	}
	b.m.TeamSpawns[team] = append(b.m.TeamSpawns[team], s...)
	b.m.Spawns = append(b.m.Spawns, s...)
	return b
}

// Item places a weapon pickup.
func (b *Builder) Item(weapon string, x, y, z float64) *Builder {
	b.m.Items = append(b.m.Items, c.Item{Weapon: weapon, Pos: [3]float64{x, y, z}})
	return b
}

// Site adds a bomb site volume.
func (b *Builder) Site(name string, x0, y0, z0, x1, y1, z1 float64) *Builder {
	b.m.Sites = append(b.m.Sites, c.Zone{Name: name, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}})
	return b
}

// BuyZone adds a buy volume for team.
func (b *Builder) BuyZone(team string, x0, y0, z0, x1, y1, z1 float64) *Builder {
	if b.m.BuyZones == nil {
		b.m.BuyZones = map[string][]c.Zone{}
	}
	b.m.BuyZones[team] = append(b.m.BuyZones[team], c.Zone{Name: team, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}})
	return b
}

// BuyAroundSpawns adds one buy zone per team: the box around its spawns, pad
// metres wider on every side and 3 m tall.
func (b *Builder) BuyAroundSpawns(pad float64) *Builder {
	for _, team := range c.Teams {
		sp := b.m.TeamSpawns[team]
		if len(sp) == 0 {
			continue
		}
		lo, hi := sp[0], sp[0]
		for _, s := range sp {
			for a := range 3 {
				lo[a], hi[a] = min(lo[a], s[a]), max(hi[a], s[a])
			}
		}
		b.BuyZone(team, lo[0]-pad, lo[1]-0.5, lo[2]-pad, hi[0]+pad, hi[1]+3, hi[2]+pad)
	}
	return b
}

// Nav adds a waypoint linked (both ways) to the named, earlier or later, nodes.
func (b *Builder) Nav(name string, x, y, z float64, links ...string) *Builder {
	b.m.Nav = append(b.m.Nav, c.NavNode{Name: name, Pos: [3]float64{x, y, z}, Links: links})
	return b
}

// Meta sets the selector/vote card text and the recommended player count.
func (b *Builder) Meta(desc string, maxPlayers int) *Builder {
	b.m.Desc, b.m.MaxPlayers = desc, maxPlayers
	return b
}

// Category puts the map in a selector category (content.Training, content.Competitive).
func (b *Builder) Category(cat string) *Builder {
	b.m.Category = cat
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

// Arena is a rectangular play area: a floor slab over x0..x1, z0..z1 (top at
// y=0) and four walls of thickness t and height h outside it.
func Arena(x0, z0, x1, z1, h, t float64, floor, wall string) []c.Box {
	return []c.Box{
		c.B(x0, -1, z0, x1, 0, z1, floor),
		c.B(x0-t, 0, z0-t, x1+t, h, z0, wall),
		c.B(x0-t, 0, z1, x1+t, h, z1+t, wall),
		c.B(x0-t, 0, z0, x0, h, z1, wall),
		c.B(x1, 0, z0, x1+t, h, z1, wall),
	}
}
