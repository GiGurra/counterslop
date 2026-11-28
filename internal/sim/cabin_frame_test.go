package sim

import (
	"fmt"
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// A ship's cabin is its passengers' frame: whatever they do aboard (stand,
// crouch, jump, walk the aisle, stand on a seat, strafe into the window band)
// they stay inside it from the doors shutting until the floor opens, however
// the ship accelerates. Before, only a body standing on the floor was carried:
// one in the air (a jump, or lifted off the floor when the climb eased out
// faster than 1 g: MVP-1 decelerates at 18 m/s² on lab_moon and 73 m/s² on
// cq_moonshot) kept its world velocity and the cabin's walls, floor and roof
// ran into it at 60-90 m/s: thrown out of the rocket mid-flight.
func TestCabinIsItsPassengersFrame(t *testing.T) {
	type rider struct {
		name string
		at   [3]float64 // from the Board spot
		in   func(tick int) Input
	}
	riders := []rider{
		{"stand", [3]float64{0, 0, 0}, func(int) Input { return Input{} }},
		{"crouch", [3]float64{1, 0, 0}, func(int) Input { return Input{Crouch: true} }},
		{"jump", [3]float64{-1, 0, 0}, func(tick int) Input { return Input{Jump: tick%40 == 0, JumpHeld: tick%40 == 0} }},
		{"bhop", [3]float64{2, 0, 0.3}, func(int) Input { return Input{Jump: true, JumpHeld: true, F: 1} }},
		{"walk", [3]float64{-2, 0, -0.3}, func(tick int) Input { return Input{F: []float64{1, -1}[(tick/150)%2]} }},
		{"seat", [3]float64{-5.2, 0.45, 2.4}, func(int) Input { return Input{} }},
		{"window", [3]float64{3, 0, 2}, func(tick int) Input { return Input{R: 1, Jump: tick%50 == 0} }},
		{"cornerFL", [3]float64{0, 0, 0}, func(int) Input { return Input{F: 1, R: -1} }},
		{"cornerFR", [3]float64{0, 0, 0}, func(int) Input { return Input{F: 1, R: 1} }},
		{"cornerBL", [3]float64{0, 0, 0}, func(int) Input { return Input{F: -1, R: -1} }},
		{"cornerBR", [3]float64{0, 0, 0}, func(int) Input { return Input{F: -1, R: 1} }},
		{"cornerjump", [3]float64{0, 0, 0}, func(tick int) Input { return Input{F: 1, R: -1, Jump: tick%70 == 0} }},
		{"crouchjump", [3]float64{4, 0, -2}, func(tick int) Input { return Input{R: -1, Crouch: true, Jump: tick%30 == 0} }},
	}
	for _, mp := range []struct {
		name string
		m    content.Map
	}{{"lab_moon", maps.LabMoon()}, {"cq_moonshot", maps.CqMoonshot()}} {
		d := mp.m.Departures[0]
		mv := &mp.m.Movers[d.Mover-1]
		for _, rules := range []MoveRules{{}, {Model: MoveSource, AutoBhop: true}} {
			for _, r := range riders {
				t.Run(fmt.Sprintf("%s/model%d/%s", mp.name, rules.Model, r.name), func(t *testing.T) {
					w := NewWorld(&mp.m)
					w.Move = rules
					b := NewBody(Vec3{d.Board[0] + r.at[0], d.Board[1] + r.at[1], d.Board[2] + r.at[2]})
					b.Yaw = -math.Pi / 2 // facing +x, along the aisle
					first, worst := -1.0, 0.0
					for tick := range int(mv.Hide[0] * 100) {
						w.T = float64(tick) / 100
						in := Input{}
						if w.T > d.Close-2 { // aboard and settled when the doors shut
							in = r.in(tick)
						}
						w.Step(b, in, 0.01)
						t1 := w.T + 0.01
						if t1 < d.Close+0.2 || t1 > mv.Hide[0]-0.05 {
							continue
						}
						l := ToFrameLocal(mp.m.Movers, d.Mover, t1, b.Pos)
						// Inside the cabin: x -8..8, z -4..4 round Board, feet on or over the floor, head under the roof.
						off := math.Max(math.Abs(l[0]-d.Board[0])-(8-R), math.Abs(l[2]-d.Board[2])-(4-R))
						off = math.Max(off, math.Max(d.Board[1]-l[1], l[1]+b.H-(d.Board[1]+3.2)))
						if off > 0.01 && first < 0 {
							first = t1
						}
						worst = math.Max(worst, off)
					}
					if first >= 0 {
						t.Errorf("out of the cabin at %.2f s into the cycle (%.0f m out at worst)", first, worst)
					}
				})
			}
		}
	}
}
