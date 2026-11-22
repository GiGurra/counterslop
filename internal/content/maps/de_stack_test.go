package maps

import (
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

// de_stack's teams start at opposite ends of the stack (operator): the
// Artisans on the roof (y 8) but off site A, the Vibe Coders in the garage
// under the ground floor.
func TestDeStackSpawnLevels(t *testing.T) {
	m := DeStack()
	var a c.Zone
	for _, s := range m.Sites {
		if s.Name == "A" {
			a = s
		}
	}
	for _, s := range m.TeamSpawns[c.Artisan] {
		if s[1] != 8 || a.Contains([3]float64{s[0], s[1] + 0.5, s[2]}) {
			t.Errorf("artisan spawn %v: want on the roof (y 8), off site A", s)
		}
	}
	for _, s := range m.TeamSpawns[c.Vibe] {
		if s[1] >= 0 {
			t.Errorf("vibe spawn %v: want in the garage, below ground", s)
		}
	}
}
