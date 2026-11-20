package game

// Box is an axis-aligned solid block. Kind picks the shader palette on the
// client ("floor", "wall", "crate", "trim").
type Box struct {
	Min  [3]float64 `json:"min"`
	Max  [3]float64 `json:"max"`
	Kind string     `json:"kind"`
}

// Map is the level geometry, shared with clients on join so client-side
// collision matches what the server expects.
type Map struct {
	Name   string       `json:"name"`
	Boxes  []Box        `json:"boxes"`
	Spawns [][3]float64 `json:"spawns"`
}

func box(x0, y0, z0, x1, y1, z1 float64, kind string) Box {
	return Box{Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}, Kind: kind}
}

// DeSlop is the first (and so far only) map: a sandy courtyard with a
// mid-building, crates to peek from and a raised "A site" reached by steps.
func DeSlop() Map {
	const half = 32.0
	const wallH = 7.0
	const t = 1.0
	boxes := []Box{
		// floor slab
		box(-half, -1, -half, half, 0, half, "sand"),
		// outer walls
		box(-half-t, 0, -half-t, half+t, wallH, -half, "brick"),
		box(-half-t, 0, half, half+t, wallH, half+t, "brick"),
		box(-half-t, 0, -half, -half, wallH, half, "brick"),
		box(half, 0, -half, half+t, wallH, half, "brick"),

		// mid building with a doorway gap (two halves + lintel)
		box(-6, 0, -4, -1.5, 5, 4, "brick"),
		box(1.5, 0, -4, 6, 5, 4, "brick"),
		box(-1.5, 3, -4, 1.5, 5, 4, "brick"),

		// long corridor walls
		box(-20, 0, 10, -18, 5, 30, "brick"),
		box(18, 0, -30, 20, 5, -10, "brick"),

		// A site: raised platform with steps
		box(16, 0, 14, 31, 1.5, 31, "hazard"),
		box(14, 0, 18, 16, 0.5, 26, "hazard"),
		box(15, 0.5, 18, 16, 1.0, 26, "hazard"),

		// crates
		box(-12, 0, -14, -10, 2, -12, "crate"),
		box(-10, 0, -14, -8, 2, -12, "crate"),
		box(-11, 2, -14, -9, 4, -12, "crate"),
		box(8, 0, 10, 10, 2, 12, "crate"),
		box(22, 1.5, 22, 24, 3.5, 24, "crate"),
		box(-26, 0, -26, -23, 3, -23, "crate"),
		box(-4, 0, 16, -2, 1.5, 18, "crate"),
		box(4, 0, -18, 7, 2, -15, "crate"),

		// silly pillar
		box(-26, 0, 20, -24, 10, 22, "hazard"),
	}
	return Map{
		Name:  "de_slop",
		Boxes: boxes,
		Spawns: [][3]float64{
			{-26, 0, -10}, {-26, 0, 0}, {-22, 0, 10},
			{26, 0, -20}, {26, 0, 0}, {10, 0, 26},
		},
	}
}
