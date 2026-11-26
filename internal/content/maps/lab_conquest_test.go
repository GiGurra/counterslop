package maps

import (
	"strings"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

// lab_conquest plays the tickets mode: its points pass the mode's checks,
// every spawn spot stands clear of the boxes, and both teams walk from every
// spawn to every capture point on the nav graph bots build from its geometry
// (it has no hand graph).
func TestLabConquest(t *testing.T) {
	m := LabConquest()
	if p := m.ModeProblems(); len(p) != 0 {
		t.Fatalf("mode problems: %s", strings.Join(p, "; "))
	}
	if len(m.Points) != 7 || len(m.Objectives()) != 5 || len(m.Nav) != 0 {
		t.Fatalf("%d points, %d capturable, %d nav nodes", len(m.Points), len(m.Objectives()), len(m.Nav))
	}
	for _, p := range m.Points {
		for _, s := range p.Spawns {
			for _, b := range m.Boxes {
				if s[0]+0.4 > b.Min[0] && s[0]-0.4 < b.Max[0] && s[2]+0.4 > b.Min[2] && s[2]-0.4 < b.Max[2] && s[1] < b.Max[1] && s[1]+1.8 > b.Min[1] {
					t.Errorf("point %s: spawn %v inside box %+v", p.ID, s, b)
				}
			}
		}
	}
	if got := m.TeamSpawns[c.Vibe]; len(got) != 8 || got[0][0] > -50 {
		t.Errorf("Earth's warmup spawns %v: its main's ring", got)
	}
	for _, e := range autoNavWalk("lab_conquest", false) {
		t.Error(e)
	}
}
