package maps

import "testing"

// MegaByte Mart's play areas are lit by shadow-casting lights (the client
// picks the r_light_shadows most important in view), and so are its display
// spots; back of house isn't.
func TestMegabyteShadowedLights(t *testing.T) {
	m := GsMegabyte()
	areas := []struct {
		name               string
		x0, y0, z0, x1, z1 float64 // the light's position: x, z box at about height y0
	}{
		{"stockroom", -55, -1, 5, -24, 39},
		{"checkouts", -40, 4.2, -36, 40, -30},
		{"gaming", -55, 4.2, -24, -22, 24},
		{"RGB Everything", 22, 4.2, -24, 55, 24},
		{"atrium (the 9090 tower)", -12, 4.2, -14, 12, 10},
		{"LAN cafe", -55, 9.5, -8, -24, 39},
		{"component aisles", 22, 9.5, -39, 55, 30},
		{"the dock", 22, -1, 5, 55, 39},
	}
	for _, a := range areas {
		n := 0
		for _, l := range m.Lights {
			if l.Shadow && l.Pos[0] > a.x0 && l.Pos[0] < a.x1 && l.Pos[2] > a.z0 && l.Pos[2] < a.z1 && l.Pos[1] > a.y0-0.5 && l.Pos[1] < a.y0+0.5 {
				n++
			}
		}
		if n == 0 {
			t.Errorf("%s: no shadow-casting light", a.name)
		}
	}
	for _, l := range m.Lights {
		if l.Shadow && l.Pos[2] > 25 && l.Pos[2] < 31 && l.Pos[1] > 4 && l.Pos[1] < 5 {
			t.Errorf("back of house light at %v casts shadows", l.Pos)
		}
	}
}
