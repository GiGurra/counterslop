// Package content holds level data shared with clients: boxes with a material
// name, spawns, and the map container. Maps are built in content/maps from
// prefabs (content/prefabs) and layout helpers (content/mapgen).
package content

import "fmt"

// Box is an axis-aligned solid block. Kind names a client material in
// web/assets/materials (sand, brick, crate, hazard, ...).
type Box struct {
	Min  [3]float64 `json:"min"`
	Max  [3]float64 `json:"max"`
	Kind string     `json:"kind"`
}

// Map is the level geometry, sent to clients on join so client-side
// collision matches what the server expects.
type Map struct {
	Name   string       `json:"name"`
	Boxes  []Box        `json:"boxes"`
	Spawns [][3]float64 `json:"spawns"`          // every spawn, any team
	Items  []Item       `json:"items,omitempty"` // weapons placed on the map (respawn after pickup)
	// TeamSpawns lists each team's spawns by team id (see Teams).
	TeamSpawns map[string][][3]float64 `json:"teamSpawns"`

	// Mode is the game mode (Modes): "" is Big Diff defusal (de_ maps, Sites
	// A and B), ModeGPUDrop is GPU Drop (gs_ maps, GPUs and Exits, no Sites).
	Mode string `json:"mode,omitempty"`
	// Sites are the bomb-plant volumes ("A", "B"); defusal only.
	Sites []Zone `json:"sites,omitempty"`
	// GPUs are the boxed GPUs on the stockroom shelves (GPU Drop: 4 by
	// default); a restocked GPU goes back to its spot.
	GPUs []GPUSpot `json:"gpus,omitempty"`
	// Exits are where a GPU carrier secures it (GPU Drop: 2 or more, far
	// apart; e.g. "dock", "entrance").
	Exits []Zone `json:"exits,omitempty"`
	// BuyZones are where each team may buy, by team id.
	BuyZones map[string][]Zone `json:"buyZones,omitempty"`
	// Nav is a waypoint graph over the walkable routes (spawns, chokes, sites):
	// the map tests walk it, and bots can path over it.
	Nav []NavNode `json:"nav,omitempty"`
	// Lights are the map's local lights (lamps, ceiling panels, screen glow); the
	// renderer adds them to the sun and the sky ambient (web/engine/lights.js).
	Lights []Light `json:"lights,omitempty"`

	// For the map selector and vote cards.
	Desc       string `json:"desc,omitempty"`
	MaxPlayers int    `json:"maxPlayers,omitempty"` // recommended total, both teams
	// Category groups the selector's cards (Categories, in order).
	Category string `json:"category"`
}

// Map categories, in the selector's order: small training maps, then full-size
// competitive ones.
const (
	Training    = "training"
	Competitive = "competitive"
)

var Categories = []string{Training, Competitive, GPUDrop}

// GPUDrop is the GPU Drop maps' category (gs_, Mode ModeGPUDrop), after the
// defusal ones.
const GPUDrop = "gpudrop"

// Game modes (Map.Mode). See docs/gpu-drop.md for GPU Drop's rules.
const (
	ModeDefusal = ""
	ModeGPUDrop = "gpudrop"
)

// GPUSpot is a boxed GPU's shelf spot: Pos is the box's floor point (where it
// stands), Yaw its facing.
type GPUSpot struct {
	Pos [3]float64 `json:"pos"`
	Yaw float64    `json:"yaw"`
}

// Site is a bomb site: a Zone the planter must stand in.
type Site = Zone

// Zone is a named axis-aligned volume (a bomb site, a buy zone).
type Zone struct {
	Name string     `json:"name"`
	Min  [3]float64 `json:"min"`
	Max  [3]float64 `json:"max"`
}

// Contains reports whether the point p (feet position) is inside the zone.
func (z Zone) Contains(p [3]float64) bool {
	for a := range 3 {
		if p[a] < z.Min[a] || p[a] > z.Max[a] {
			return false
		}
	}
	return true
}

// Center is the middle of the zone's floor (min y).
func (z Zone) Center() [3]float64 {
	return [3]float64{(z.Min[0] + z.Max[0]) / 2, z.Min[1], (z.Min[2] + z.Max[2]) / 2}
}

// NavNode is one waypoint; Links name the nodes reachable from it by walking
// (straight line, stairs included). Links are listed once and work both ways.
type NavNode struct {
	Name  string     `json:"name"`
	Pos   [3]float64 `json:"pos"`
	Links []string   `json:"links,omitempty"`
	// Lane marks a route choice: attacker bots each pick one lane node a
	// round (seeded) and walk spawn -> lane -> site, so they use more than the
	// shortest route (internal/game/ai_lanes.go).
	Lane bool `json:"lane,omitempty"`
}

// Team ids. Display names, colours and flavour text live in web/assets/teams.js.
const (
	Vibe    = "vibe"    // Vibe Coders, attackers
	Artisan = "artisan" // Artisans, defenders
	// Spectator is not a side: no body, can't shoot or be shot. New players
	// start here until they pick a team.
	Spectator = "spec"
)

// Teams in a fixed order.
var Teams = []string{Vibe, Artisan}

// B makes a box from min and max corners.
func B(x0, y0, z0, x1, y1, z1 float64, kind string) Box {
	return Box{Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}, Kind: kind}
}

// Material names known to the client (web/assets/materials).
const (
	Sand      = "sand"
	Brick     = "brick"
	Crate     = "crate"
	Hazard    = "hazard"
	Concrete  = "concrete"  // tiled floors, panelled walls
	Carpet    = "carpet"    // office carpet tiles
	Plaster   = "plaster"   // painted office walls with a skirting stripe
	Metal     = "metal"     // diamond-plate decks and steps
	Desk      = "desk"      // laminate desks, counters, whiteboards
	Container = "container" // corrugated shipping containers; colour from the footprint (prefabs.Container)
	Stucco    = "stucco"    // sun-baked render over sandstone blocks (old town walls)
	Rack      = "rack"      // server racks: perforated doors, blinking status LEDs
	Awning    = "awning"    // striped market cloth, its colour picked by where it hangs
	Lamp      = "lamp"      // light fixture: glowing diffuser in the hue of the Light under it

	// The hardware superstore (gs_ maps). Per-box looks are seeded by where the box stands.
	StoreTile    = "storetile"     // polished floor tiles, ceiling strips mirrored in them
	Shelf        = "shelf"         // gondola shelving stocked with products; narrow ends are SALE end caps
	Carton       = "carton"        // cardboard stock cartons (a big box reads as a stacked pallet)
	Checkout     = "checkout"      // checkout counter: blue sides, a lane light, a moving belt on top
	Screen       = "screen"        // monitors and TVs: big faces glow (code, a demo game or a sale ad)
	PCTower      = "pctower"       // gaming PC: glass side with RGB fans, mesh front
	Escalator    = "escalator"     // escalator steps, treads moving towards +x / +z
	EscalatorRev = "escalator_rev" // escalator steps, treads moving towards -x / -z
	Glass        = "glass"         // tinted glass panes and balustrades (opaque for now), steel frame
	Neon         = "neon"          // a sign with a neon word (SALE, GPU, OPEN, 9090, DEAL)
	LanCarpet    = "lancarpet"     // dark LAN cafe carpet with glowing arcade shapes
)

// Materials lists every material name above (the client must define each).
var Materials = []string{Sand, Brick, Crate, Hazard, Concrete, Carpet, Plaster, Metal, Desk, Container, Stucco, Rack, Awning, Lamp,
	StoreTile, Shelf, Carton, Checkout, Screen, PCTower, Escalator, EscalatorRev, Glass, Neon, LanCarpet}

// Item is a weapon lying on the map; Weapon is an id from web/assets/weapons/weapons.json.
type Item struct {
	Weapon string     `json:"weapon"`
	Pos    [3]float64 `json:"pos"`
}

// Light kinds.
const (
	PointLight = "point"
	SpotLight  = "spot"
)

// Light is a local light source on the map (purely visual: no gameplay reads it).
// Light reaches as far as Range, falling off smoothly to zero there, and only
// through open space: the client floods it cell by cell, so it never leaks
// through a wall into the next room. Shadow asks for a real shadow map; the
// client gives one to the r_light_shadows most important shadowed lights in view.
type Light struct {
	Kind      string     `json:"kind"`  // PointLight or SpotLight
	Pos       [3]float64 `json:"pos"`   // metres
	Color     [3]float64 `json:"color"` // linear RGB, 0..1
	Intensity float64    `json:"intensity"`
	Range     float64    `json:"range"`           // metres
	Dir       [3]float64 `json:"dir,omitempty"`   // spot: where it points (any length)
	Cone      float64    `json:"cone,omitempty"`  // spot: outer half-angle, degrees
	Inner     float64    `json:"inner,omitempty"` // spot: full-strength half-angle, degrees (0: 0.7 x Cone)
	Shadow    bool       `json:"shadow,omitempty"`
	// Flicker, when non-zero, seeds a flicker (a dying fluorescent tube, a neon sign).
	Flicker uint32 `json:"flicker,omitempty"`
}

// Point makes an unshadowed point light.
func Point(pos [3]float64, color [3]float64, intensity, rng float64) Light {
	return Light{Kind: PointLight, Pos: pos, Color: color, Intensity: intensity, Range: rng}
}

// Spot makes a spot light pointing along dir with an outer half-angle cone (degrees).
func Spot(pos, dir [3]float64, color [3]float64, intensity, rng, cone float64) Light {
	return Light{Kind: SpotLight, Pos: pos, Dir: dir, Color: color, Intensity: intensity, Range: rng, Cone: cone}
}

// Check reports what is wrong with a light, or nil.
func (l Light) Check() error {
	switch {
	case l.Kind != PointLight && l.Kind != SpotLight:
		return fmt.Errorf("kind %q is not point or spot", l.Kind)
	case l.Range <= 0 || l.Range > 40:
		return fmt.Errorf("range %v outside (0, 40] m", l.Range)
	case l.Intensity <= 0:
		return fmt.Errorf("intensity %v must be > 0", l.Intensity)
	}
	for _, c := range l.Color {
		if c < 0 || c > 1 {
			return fmt.Errorf("color %v outside 0..1", l.Color)
		}
	}
	if l.Kind == SpotLight {
		if l.Dir == ([3]float64{}) {
			return fmt.Errorf("spot without a dir")
		}
		if l.Cone <= 0 || l.Cone > 89 || l.Inner < 0 || l.Inner > l.Cone {
			return fmt.Errorf("spot cone %v / inner %v: want 0 < inner <= cone < 90", l.Cone, l.Inner)
		}
	}
	return nil
}
