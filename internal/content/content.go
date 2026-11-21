// Package content holds level data shared with clients: boxes with a material
// name, spawns, and the map container. Maps are built in content/maps from
// prefabs (content/prefabs) and layout helpers (content/mapgen).
package content

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

	// Sites are the bomb-plant volumes ("A", "B").
	Sites []Zone `json:"sites,omitempty"`
	// BuyZones are where each team may buy, by team id.
	BuyZones map[string][]Zone `json:"buyZones,omitempty"`
	// Nav is a waypoint graph over the walkable routes (spawns, chokes, sites):
	// the map tests walk it, and bots can path over it.
	Nav []NavNode `json:"nav,omitempty"`

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

var Categories = []string{Training, Competitive}

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
	Sand     = "sand"
	Brick    = "brick"
	Crate    = "crate"
	Hazard   = "hazard"
	Concrete = "concrete" // tiled floors, panelled walls
	Carpet   = "carpet"   // office carpet tiles
	Plaster  = "plaster"  // painted office walls with a skirting stripe
	Metal    = "metal"    // diamond-plate decks and steps
	Desk     = "desk"     // laminate desks, counters, whiteboards
)

// Materials lists every material name above (the client must define each).
var Materials = []string{Sand, Brick, Crate, Hazard, Concrete, Carpet, Plaster, Metal, Desk}

// Item is a weapon lying on the map; Weapon is an id from web/assets/weapons/weapons.json.
type Item struct {
	Weapon string     `json:"weapon"`
	Pos    [3]float64 `json:"pos"`
}
