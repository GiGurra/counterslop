package maps

import (
	"math"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// LabRace is the race rules' test track (Mode "race", docs/racing.md): a
// flat 120 x 48 m plate over a void, driven anticlockwise seen from above:
// west along the north half, round the west end, east along the south half,
// round the east end. Four wide gates: the start/finish line across the
// north half at x 0, the west end, the south half at x 0, the east end.
// The grid (four buggies, two by two) is behind the line, facing west; off
// the plate's edge is the void (a pit): a wreck.
func LabRace() c.Map {
	west := math.Pi / 2 // facing -x
	b := mapgen.New("lab_race").Mode(c.ModeRace).
		Add(c.B(-60, -1, -24, 60, 0, 24, c.Concrete)).
		Pit("void", -300, -80, -300, 300, -6, 300)
	b.Checkpoint(c.Checkpoint{Name: "start", Min: [3]float64{-1, -1, 0}, Max: [3]float64{1, 6, 24}, Spawn: [3]float64{-4, 0, 12}, Yaw: west})
	b.Checkpoint(c.Checkpoint{Name: "west", Min: [3]float64{-60, -1, -24}, Max: [3]float64{-44, 6, 24}, Spawn: [3]float64{-50, 0, -12}, Yaw: -west})
	b.Checkpoint(c.Checkpoint{Name: "south", Min: [3]float64{-1, -1, -24}, Max: [3]float64{1, 6, 0}, Spawn: [3]float64{4, 0, -12}, Yaw: -west})
	b.Checkpoint(c.Checkpoint{Name: "east", Min: [3]float64{44, -1, -24}, Max: [3]float64{60, 6, 24}, Spawn: [3]float64{50, 0, 12}, Yaw: west})
	for _, at := range [][2]float64{{8, 9}, {8, 17}, {15, 9}, {15, 17}} {
		b.Car("buggy", at[0], 0, at[1], west)
	}
	return b.
		TeamSpawns(c.Vibe, [3]float64{28, 0, 9}, [3]float64{28, 0, 13}).
		TeamSpawns(c.Artisan, [3]float64{28, 0, 17}, [3]float64{28, 0, 21}).
		BuyAroundSpawns(2).
		Meta("Race lab: a flat oval over the void, four gates, four buggies on the grid.", 4).
		Category(c.Racing).
		Map()
}
