package prefabs

import (
	"math"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// The monument at any size and facing: no two of its boxes z-fight, the
// figure stands on the plinth's cap and fits under h + the plinth, the
// plaque's lettering is on the plaque's face, read from the front, and the
// sigh spot is in front of the plinth.
func TestFacepalmMonument(t *testing.T) {
	for _, face := range []string{c.SignPX, c.SignNX, c.SignPZ, c.SignNZ} {
		for _, h := range []float64{1.8, 7.5} {
			m := FacepalmMonument(10, 2, -5, h, 1.8, 13, face, "MONUMENT TO THE POSTMORTEM\nIN MEMORIAM")
			for _, f := range coplanar(m.Boxes) {
				t.Errorf("%s h %g: %s", face, h, f)
			}
			top, low := 0.0, math.Inf(1)
			for _, b := range m.Boxes[4:] {
				top, low = max(top, b.Max[1]), min(low, b.Min[1])
			}
			if math.Abs(low-(2+1.8+0.25)) > 1e-9 || top > 2+1.8+0.25+h+1e-9 || top < 2+1.8+0.25+0.9*h {
				t.Errorf("%s h %g: the figure spans y %g..%g", face, h, low, top)
			}
			if len(m.Signs) != 1 || m.Signs[0].Face != face {
				t.Errorf("%s: plaque %+v", face, m.Signs)
			}
			if len(m.Lights) != 2 || m.Lights[0].Pos[1] != 13 || m.Lights[0].Dir[1] >= 0 {
				t.Errorf("%s: lights %+v", face, m.Lights)
			}
			if m.Spot.Name != "sigh" || m.Spot.Contains(m.Signs[0].Pos) {
				t.Errorf("%s: the spot %+v covers the plaque", face, m.Spot)
			}
			b := mapgen.New("t")
			m.Place(b)
			if mm := b.Map(); len(mm.VoiceSpots) != 1 || len(mm.Signs) != 1 || len(mm.Boxes) != len(m.Boxes) {
				t.Errorf("%s: placed %d spots %d signs %d boxes", face, len(mm.VoiceSpots), len(mm.Signs), len(mm.Boxes))
			}
		}
	}
	for _, f := range coplanar(CrestBanner(0, 30, 0, 2, 3, 6, c.SignPX)) { // off the ground: undersides count
		t.Errorf("banner: %s", f)
	}
}
