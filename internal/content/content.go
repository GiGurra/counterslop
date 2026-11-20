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
	// Sites are the bomb sites ("The Big Diff" is planted inside one).
	Sites []Site `json:"sites,omitempty"`
}

// Site is a bomb site: an axis-aligned volume the planter must stand in.
type Site struct {
	Name string     `json:"name"`
	Min  [3]float64 `json:"min"`
	Max  [3]float64 `json:"max"`
}

// Contains reports whether p is inside the site volume.
func (s Site) Contains(p [3]float64) bool {
	for a := range 3 {
		if p[a] < s.Min[a] || p[a] > s.Max[a] {
			return false
		}
	}
	return true
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
	Sand   = "sand"
	Brick  = "brick"
	Crate  = "crate"
	Hazard = "hazard"
)

// Item is a weapon lying on the map; Weapon is an id from web/assets/weapons/weapons.json.
type Item struct {
	Weapon string     `json:"weapon"`
	Pos    [3]float64 `json:"pos"`
}
