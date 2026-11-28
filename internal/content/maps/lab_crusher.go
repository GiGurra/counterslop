package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// LabCrusher is the movers lab (internal/sim push.go: movers push and
// crush): three lanes across a 40 x 32 m hall, each a slide mover.
//   - The pusher (z -12..-8): a 2 m block crossing the open floor at 4 m/s
//     and back: it pushes whoever stands in its way, and leaves them there.
//   - The ram (z -2..2): a block running at 8 m/s right up to the east wall:
//     whoever is still in front of it there is crushed (fast: dead at once).
//   - The press (z 8..12): a slab coming down from 3 m to 0.1 m off the
//     floor at 1 m/s: a slow crush (a few HP a command).
//
// Signs name the lanes on the west wall. sim/cases mover-push-crush plays on
// it, the scenario lab-crusher shows it.
func LabCrusher() c.Map {
	b := mapgen.New("lab_crusher").
		Add(mapgen.Arena(-20, -16, 20, 16, 6, 1, c.Concrete, c.Hazard)...).
		Mover(
			c.Mover{Kind: c.MoverSlide, Mat: c.Metal, Min: [3]float64{-16, 0, -12}, Max: [3]float64{-14, 2, -8}, Move: [3]float64{20, 0, 0}, Period: 10},
			c.Mover{Kind: c.MoverSlide, Mat: c.Metal, Min: [3]float64{-16, 0, -2}, Max: [3]float64{-14, 2.5, 2}, Move: [3]float64{34, 0, 0}, Period: 8.5},
			c.Mover{Kind: c.MoverSlide, Mat: c.Metal, Min: [3]float64{-2, 3, 8}, Max: [3]float64{2, 3.5, 12}, Move: [3]float64{0, -2.9, 0}, Period: 5.8},
		)
	for _, s := range []struct {
		text string
		z    float64
	}{{"PUSH", -10}, {"RAM", 0}, {"PRESS", 10}} {
		b.Sign(c.Sign{Text: s.text, Pos: [3]float64{-19.95, 3.6, s.z}, Face: c.SignPX, Size: 0.5, Mat: c.SignLed})
	}
	return b.
		TeamSpawns(c.Vibe, [3]float64{-18, 0, 14}, [3]float64{-16, 0, 14}).
		TeamSpawns(c.Artisan, [3]float64{16, 0, 14}, [3]float64{18, 0, 14}).
		BuyAroundSpawns(1.5).
		Meta("Movers lab: a block that pushes, a ram that crushes against the wall, a press that comes down slowly.", 4).
		Category(c.Training).
		Map()
}
