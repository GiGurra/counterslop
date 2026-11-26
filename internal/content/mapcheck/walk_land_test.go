package mapcheck

import (
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

// Down a long stair the classic model falls off each step and lags behind
// it: it reaches a node on the bottom step still in the air, above it. The
// height a waypoint is reached at is checked where the body lands (it came
// down the right stair), not mid-fall (de_stack's switchback stairs).
func TestWalkDownStairsChecksOnLanding(t *testing.T) {
	m := c.Map{Name: "downstairs", Boxes: []c.Box{c.B(-4, -1, -4, 4, 0, 30, c.Concrete)}}
	const steps, rise, tread = 12, 0.5, 0.8
	for i := range steps { // from z 0 (6 m up) down +z
		m.Boxes = append(m.Boxes, c.B(-2, 0, 0, 2, rise*float64(steps-i), tread*float64(i+1), c.Metal))
	}
	top := [3]float64{0, steps * rise, 0.3}
	foot := c.NavNode{Name: "foot", Pos: [3]float64{0, rise, tread*(steps-1) + 0.4}}
	for _, model := range []string{"classic", "source"} {
		if _, err := WalkIn(m, top, []c.NavNode{{Name: "top", Pos: top}, foot}, MoveModels[model]); err != nil {
			t.Errorf("%s: %v", model, err)
		}
	}
	// It is still checked: a node on another level fails where the body lands.
	far := c.NavNode{Name: "ledge", Pos: [3]float64{0, 3, tread*(steps-1) + 0.4}}
	if _, err := WalkIn(m, top, []c.NavNode{{Name: "top", Pos: top}, far}, MoveModels["classic"]); err == nil {
		t.Error("a node 2.5 m above the stair's foot counts as reached")
	}
}
