// Package mapgen has layout helpers for building maps: enclosures, rooms,
// spawn sets. Prefer adding a helper here over hand-placing boxes in a map.
package mapgen

import (
	"math"
	"slices"

	c "github.com/gigurra/counterslop/internal/content"
)

// Builder accumulates boxes and spawns for a map.
type Builder struct {
	m    c.Map
	fill *fill // FillSpawns, resolved in Map
}

func New(name string) *Builder { return &Builder{m: c.Map{Name: name}} }

func (b *Builder) Add(boxes ...c.Box) *Builder { b.m.Boxes = append(b.m.Boxes, boxes...); return b }

// Water adds a water volume (its top is the surface; content.Map.Water).
func (b *Builder) Water(x0, y0, z0, x1, y1, z1 float64) *Builder {
	b.m.Water = append(b.m.Water, c.B(x0, y0, z0, x1, y1, z1, c.Water))
	return b
}

// Boat adds a boat's start (content.BoatSpawn): on the water at (x, y, z), heading yaw.
func (b *Builder) Boat(x, y, z, yaw float64) *Builder {
	b.m.Boats = append(b.m.Boats, c.BoatSpawn{Pos: [3]float64{x, y, z}, Yaw: yaw})
	return b
}

// Car adds a car's start (content.CarSpawn): type typ standing at (x, y, z), heading yaw.
func (b *Builder) Car(typ string, x, y, z, yaw float64) *Builder {
	b.m.Cars = append(b.m.Cars, c.CarSpawn{Pos: [3]float64{x, y, z}, Yaw: yaw, Type: typ})
	return b
}

// Mover adds moving solids (escalator treads: prefabs.EscalatorMover).
func (b *Builder) Mover(mv ...c.Mover) *Builder {
	b.m.Movers = append(b.m.Movers, mv...)
	return b
}

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

// Mode sets the game mode (content.ModeGPUDrop; the default is defusal).
func (b *Builder) Mode(mode string) *Builder {
	b.m.Mode = mode
	return b
}

// GPU puts a boxed GPU on a shelf spot (GPU Drop).
func (b *Builder) GPU(x, y, z, yaw float64) *Builder {
	b.m.GPUs = append(b.m.GPUs, c.GPUSpot{Pos: [3]float64{x, y, z}, Yaw: yaw})
	return b
}

// Exit adds an exit volume where a carrier secures a GPU (GPU Drop).
func (b *Builder) Exit(name string, x0, y0, z0, x1, y1, z1 float64) *Builder {
	b.m.Exits = append(b.m.Exits, c.Zone{Name: name, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}})
	return b
}

// Point adds a capture point (tickets mode, content.CapturePoint). A spawn
// point gets a ring of 8 spawn spots at 0.55 of its radius around the centre.
func (b *Builder) Point(p c.CapturePoint) *Builder {
	if p.Spawn && len(p.Spawns) == 0 {
		for i := range 8 {
			a := float64(i) * math.Pi / 4
			p.Spawns = append(p.Spawns, [3]float64{
				math.Round((p.Pos[0]+0.55*p.Radius*math.Sin(a))*100) / 100, p.Pos[1],
				math.Round((p.Pos[2]+0.55*p.Radius*math.Cos(a))*100) / 100})
		}
	}
	b.m.Points = append(b.m.Points, p)
	return b
}

// Light adds map lights (content.Point, content.Spot).
func (b *Builder) Light(l ...c.Light) *Builder {
	b.m.Lights = append(b.m.Lights, l...)
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

// NavLane adds a waypoint like Nav that is also a lane: attacker bots pick a
// lane each round and route through it (content.NavNode.Lane).
func (b *Builder) NavLane(name string, x, y, z float64, links ...string) *Builder {
	b.m.Nav = append(b.m.Nav, c.NavNode{Name: name, Pos: [3]float64{x, y, z}, Links: links, Lane: true})
	return b
}

// Meta sets the selector/vote card text and the recommended player count.
func (b *Builder) Meta(desc string, maxPlayers int) *Builder {
	b.m.Desc, b.m.MaxPlayers = desc, maxPlayers
	return b
}

// Fog sets the map's own distance fog (content.Map.Fog).
func (b *Builder) Fog(density float64, color [3]float64) *Builder {
	b.m.Fog = &c.Fog{Density: density, Color: color}
	return b
}

// Checkpoint adds a race track's next gate (Mode "race": the first is the
// start/finish line).
func (b *Builder) Checkpoint(cp c.Checkpoint) *Builder {
	b.m.Checkpoints = append(b.m.Checkpoints, cp)
	return b
}

// RaceLine appends points (x, y, z: on the road) to the bots' racing line
// (content.Map.RaceLine), in driving order.
func (b *Builder) RaceLine(pts ...[3]float64) *Builder {
	b.m.RaceLine = append(b.m.RaceLine, pts...)
	return b
}

// Pit adds a deadly volume (content.Map.Pits): falling in kills, the kill
// feed says name.
func (b *Builder) Pit(name string, x0, y0, z0, x1, y1, z1 float64) *Builder {
	b.m.Pits = append(b.m.Pits, c.Zone{Name: name, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}})
	return b
}

// GravZone adds a box with its own gravity (content.Map.GravZones; g and jump
// are fractions of the world's: g 0 is space).
func (b *Builder) GravZone(name string, x0, y0, z0, x1, y1, z1, g, jump float64) *Builder {
	b.m.GravZones = append(b.m.GravZones, c.GravZone{Name: name, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}, G: g, Jump: jump})
	return b
}

// JumpPad adds a jump pad (content.Map.Launchers with an Apex): a w x d pad
// on the floor at x, y, z throwing a body onto target, the arc's top apex m
// over the higher end. The pad's look is a box of the map's own.
func (b *Builder) JumpPad(name string, x, y, z, w, d float64, target [3]float64, apex float64) *Builder {
	b.m.Launchers = append(b.m.Launchers, c.Launcher{Name: name, Min: [3]float64{x - w/2, y - 0.1, z - d/2}, Max: [3]float64{x + w/2, y + 0.3, z + d/2}, Target: target, Apex: apex})
	return b
}

// Launcher adds a launch pad (content.Map.Launchers): a w x d pad on the floor
// at (x, y, z) (its centre) that throws a body standing on it at vel.
func (b *Builder) Launcher(name string, x, y, z, w, d float64, vel [3]float64) *Builder {
	b.m.Launchers = append(b.m.Launchers, c.Launcher{Name: name, Min: [3]float64{x - w/2, y - 0.1, z - d/2}, Max: [3]float64{x + w/2, y + 0.2, z + d/2}, Vel: vel})
	return b
}

// Ship is a vehicle built of movers with what rides along: zones and
// launchers carried by its movers and, if it keeps a timetable, a departure.
// Their Mover fields count from 1 within Movers; Builder.Ship rebases them.
type Ship struct {
	Movers    []c.Mover
	Zones     []c.GravZone
	Launchers []c.Launcher
	Departure *c.Departure
	Signs     []c.Sign // on the ship (Mover 1-based within Movers)
}

// Ship adds ship sh (prefabs.Rocket) to the map.
func (b *Builder) Ship(sh Ship) *Builder {
	base := len(b.m.Movers)
	b.m.Movers = append(b.m.Movers, sh.Movers...)
	for _, z := range sh.Zones {
		if z.Mover > 0 {
			z.Mover += base
		}
		b.m.GravZones = append(b.m.GravZones, z)
	}
	for _, l := range sh.Launchers {
		if l.Mover > 0 {
			l.Mover += base
		}
		b.m.Launchers = append(b.m.Launchers, l)
	}
	for _, g := range sh.Signs {
		if g.Mover > 0 {
			g.Mover += base
		}
		b.m.Signs = append(b.m.Signs, g)
	}
	if d := sh.Departure; d != nil {
		dep := *d
		dep.Mover += base
		b.m.Departures = append(b.m.Departures, dep)
	}
	return b
}

// TicketGate adds a "tickets please" gate (content.Map.TicketGates).
func (b *Builder) TicketGate(name string, x0, y0, z0, x1, y1, z1 float64) *Builder {
	b.m.TicketGates = append(b.m.TicketGates, c.Zone{Name: name, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}})
	return b
}

// Sign adds text to the map (content.Sign: drawn by the client only).
func (b *Builder) Sign(g c.Sign) *Builder {
	b.m.Signs = append(b.m.Signs, g)
	return b
}

// Scanner adds a walk-through security scanner (content.Map.Scanners: a gag).
func (b *Builder) Scanner(name string, x0, y0, z0, x1, y1, z1 float64) *Builder {
	b.m.Scanners = append(b.m.Scanners, c.Zone{Name: name, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}})
	return b
}

// VoiceSpot adds a spot where the client plays line (content.Map.VoiceSpots).
func (b *Builder) VoiceSpot(line string, x0, y0, z0, x1, y1, z1 float64) *Builder {
	b.m.VoiceSpots = append(b.m.VoiceSpots, c.Zone{Name: line, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}})
	return b
}

// Indoor sets the ambient kept where no sky is visible (content.Map.Indoor).
func (b *Builder) Indoor(a float64) *Builder {
	b.m.Indoor = a
	return b
}

// Sky sets the sky the client draws (content.Map.Sky).
func (b *Builder) Sky(sky string) *Builder {
	b.m.Sky = sky
	return b
}

// NavHint is a point a nav graph built from the geometry passes through
// (content.Map.NavHints): a doorway too narrow for the generator's links.
func (b *Builder) NavHint(x, y, z float64) *Builder {
	b.m.NavHints = append(b.m.NavHints, c.NavHint{Pos: [3]float64{x, y, z}})
	return b
}

// SkyZone gives the box x0..x1, y0..y1, z0..z1 its own sky (content.Map.SkyZones).
func (b *Builder) SkyZone(sky string, x0, y0, z0, x1, y1, z1 float64) *Builder {
	b.m.SkyZones = append(b.m.SkyZones, c.Zone{Name: sky, Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}})
	return b
}

// PitLane adds a pit lane: a car in it is repaired (content.Map.PitLanes).
func (b *Builder) PitLane(x0, y0, z0, x1, y1, z1 float64) *Builder {
	b.m.PitLanes = append(b.m.PitLanes, c.Zone{Name: "pit lane", Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}})
	return b
}

// Pickup puts a power-up on the track (content.Map.Pickups; kind one of
// content.PickupKinds), on the road at x, y, z.
func (b *Builder) Pickup(kind string, x, y, z float64) *Builder {
	b.m.Pickups = append(b.m.Pickups, c.Pickup{Kind: kind, Pos: [3]float64{x, y, z}})
	return b
}

// Cut sets the height above which a view from over the map leaves the boxes
// out (content.Map.Cut: an enclosed map's lid).
func (b *Builder) Cut(y float64) *Builder {
	b.m.Cut = &y
	return b
}

// Rule pins a server setting (a content.RuleCvars name) for this map.
func (b *Builder) Rule(cvar, value string) *Builder {
	if b.m.Rules == nil {
		b.m.Rules = map[string]string{}
	}
	b.m.Rules[cvar] = value
	return b
}

// Category puts the map in a selector category (content.Training, content.Competitive).
func (b *Builder) Category(cat string) *Builder {
	b.m.Category = cat
	return b
}

// Map is the built map (spawns filled in, see FillSpawns).
func (b *Builder) Map() c.Map {
	b.fillSpawns()
	return b.m
}

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

// Rect is an axis-aligned floor-plan rectangle: x0..x1 by z0..z1.
type Rect struct{ X0, Z0, X1, Z1 float64 }

// Fill returns solid rectangles covering everything inside bounds that no
// open rectangle covers: lay out a map as the walkable space (rooms,
// corridors, doorways as small open rects joining them) and let the rest be
// building. The open rects' edges cut bounds into a grid; solid cells merge
// greedily into as few rectangles as rows allow (along x, then down z).
func Fill(bounds Rect, open []Rect) []Rect {
	xs, zs := []float64{bounds.X0, bounds.X1}, []float64{bounds.Z0, bounds.Z1}
	for _, r := range open {
		xs = append(xs, max(bounds.X0, min(bounds.X1, r.X0)), max(bounds.X0, min(bounds.X1, r.X1)))
		zs = append(zs, max(bounds.Z0, min(bounds.Z1, r.Z0)), max(bounds.Z0, min(bounds.Z1, r.Z1)))
	}
	xs, zs = uniqSorted(xs), uniqSorted(zs)
	nx, nz := len(xs)-1, len(zs)-1
	solid := make([][]bool, nz)
	for j := range nz {
		solid[j] = make([]bool, nx)
		for i := range nx {
			cx, cz := (xs[i]+xs[i+1])/2, (zs[j]+zs[j+1])/2
			solid[j][i] = true
			for _, r := range open {
				if cx > r.X0 && cx < r.X1 && cz > r.Z0 && cz < r.Z1 {
					solid[j][i] = false
					break
				}
			}
		}
	}
	var out []Rect
	for j := range nz {
		for i := 0; i < nx; i++ {
			if !solid[j][i] {
				continue
			}
			i1 := i
			for i1+1 < nx && solid[j][i1+1] {
				i1++
			}
			j1 := j
			for j1+1 < nz && allTrue(solid[j1+1][i:i1+1]) {
				j1++
			}
			for jj := j; jj <= j1; jj++ {
				for ii := i; ii <= i1; ii++ {
					solid[jj][ii] = false
				}
			}
			out = append(out, Rect{xs[i], zs[j], xs[i1+1], zs[j1+1]})
			i = i1
		}
	}
	return out
}

func allTrue(b []bool) bool {
	for _, v := range b {
		if !v {
			return false
		}
	}
	return true
}

func uniqSorted(v []float64) []float64 {
	slices.Sort(v)
	return slices.Compact(v)
}
