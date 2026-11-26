package sim

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// Noclip flies along the view through a wall, up on jump, slower walking.
func TestNoclip(t *testing.T) {
	m := content.Map{Boxes: []content.Box{content.B(-20, -1, -20, 20, 0, 20, content.Concrete), content.B(-20, 0, -4, 20, 4, -3, content.Concrete)}}
	w := NewWorld(&m)
	s := NewBody(Vec3{0, 0, 0})
	s.Noclip = true
	for range 100 { // 1 s ahead (yaw 0: -z), through the wall at z -3
		w.Step(s, Input{F: 1}, 0.01)
	}
	if math.Abs(s.Pos[2]+NoclipSpeed) > 1e-6 || s.Pos[1] != 0 || s.OnGround {
		t.Fatalf("flew to %v", s.Pos)
	}
	for range 50 {
		w.Step(s, Input{F: 1, Pitch: math.Pi / 2, Walk: true}, 0.01) // straight up, slow
	}
	if math.Abs(s.Pos[1]-0.5*NoclipSpeed*noclipWalk) > 1e-6 {
		t.Fatalf("up to %v", s.Pos)
	}
	y := s.Pos[1]
	w.Step(s, Input{Crouch: true}, 0.1)
	if math.Abs(s.Pos[1]-(y-0.1*NoclipSpeed)) > 1e-6 {
		t.Fatalf("crouch: %v", s.Pos)
	}
}
