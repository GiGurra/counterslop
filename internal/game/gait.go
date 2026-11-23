package game

import (
	"encoding/json"
	"math"
)

// The gait on the wire: [phase, amp, air, crouch] as integers in 1/gaitQ
// (under 3 mm of foot reach per step, far under a hit box), trailing zeros
// left out (a player standing on the ground sends [phase] or nothing): the
// snapshot's "g" and a pose snapshot row's tail (web/gen/gait.js
// gaitFromWire). Lag compensation records the same quantised values
// (recordHistory), so a rewound shot meets the pose drawn.
const gaitQ = 50

func quant(v float64) int { return int(math.Round(v * gaitQ)) }

// wireInts is g as the integers sent, trailing zeros trimmed.
func (g Gait) wireInts() []int {
	a := []int{quant(g.Phase), quant(g.Amp), quant(g.Air), quant(g.Crouch)}
	for len(a) > 0 && a[len(a)-1] == 0 {
		a = a[:len(a)-1]
	}
	return a
}

// wire is g as a client decodes it.
func (g Gait) wire() Gait {
	return Gait{float64(quant(g.Phase)) / gaitQ, float64(quant(g.Amp)) / gaitQ, float64(quant(g.Air)) / gaitQ, float64(quant(g.Crouch)) / gaitQ}
}

func (g Gait) MarshalJSON() ([]byte, error) { return json.Marshal(g.wireInts()) }

func (g *Gait) UnmarshalJSON(b []byte) error {
	var a []int
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	a = append(a, 0, 0, 0, 0)
	*g = Gait{float64(a[0]) / gaitQ, float64(a[1]) / gaitQ, float64(a[2]) / gaitQ, float64(a[3]) / gaitQ}
	return nil
}
