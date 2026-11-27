package maps

import (
	"math"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// LabParkour is the on-foot race rules' test course (Map.OnFoot, point to
// point; docs/racing.md "Parkour"): a straight run east over a void. The
// start plate (the team spawns at its west end, the start gate across it at
// x 10), a 3 m gap at x 30..33 (a jump; falling in puts you back on the
// start gate), the middle gate at x 40, a car station (one buggy) at x 44,
// a 10 m gap at x 60..70 the buggy jumps off a ramp, the finish at x 90.
func LabParkour() c.Map {
	east := -math.Pi / 2 // facing +x
	b := mapgen.New("lab_parkour").Mode(c.ModeRace).OnFoot(true).
		Add(c.B(-4, -1, -8, 30, 0, 8, c.Concrete), c.B(33, -1, -8, 60, 0, 8, c.Concrete), c.B(70, -1, -8, 96, 0, 8, c.Concrete)).
		Add(c.W(52, 0, -3, 60, 1.2, 3, c.Metal, c.RampPX)).
		Pit("void", -300, -80, -300, 300, -6, 300)
	b.Checkpoint(c.Checkpoint{Name: "start", Min: [3]float64{10, -1, -8}, Max: [3]float64{11, 6, 8}, Spawn: [3]float64{12, 0, 0}, Yaw: east})
	b.Checkpoint(c.Checkpoint{Name: "middle", Min: [3]float64{40, -1, -8}, Max: [3]float64{41, 6, 8}, Spawn: [3]float64{42, 0, -4}, Yaw: east})
	b.Checkpoint(c.Checkpoint{Name: "finish", Min: [3]float64{90, -1, -8}, Max: [3]float64{91, 8, 8}, Spawn: [3]float64{92, 0, 0}, Yaw: east})
	b.Car("buggy", 44, 0, 4, east)
	return b.
		TeamSpawns(c.Vibe, [3]float64{0, 0, -3}, [3]float64{0, 0, -1}).
		TeamSpawns(c.Artisan, [3]float64{0, 0, 1}, [3]float64{0, 0, 3}).
		BuyAroundSpawns(2).
		Rule("mp_race_combat", "0").
		Meta("Parkour lab: a straight course on foot over the void, a jump, a buggy for the big gap.", 4).
		Category(c.Parkour).
		Map()
}
