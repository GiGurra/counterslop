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
	Spawns [][3]float64 `json:"spawns"`
}

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
