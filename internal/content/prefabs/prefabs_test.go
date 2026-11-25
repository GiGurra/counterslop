package prefabs

import (
	"math"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

func vol(b c.Box) float64 {
	return (b.Max[0] - b.Min[0]) * (b.Max[1] - b.Min[1]) * (b.Max[2] - b.Min[2])
}

func valid(t *testing.T, boxes ...c.Box) {
	t.Helper()
	for i, b := range boxes {
		for a := range 3 {
			if !(b.Min[a] < b.Max[a]) {
				t.Fatalf("box %d inverted/empty on axis %d: %+v", i, a, b)
			}
		}
		switch b.Kind {
		case c.Sand, c.Brick, c.Crate, c.Hazard:
		default:
			t.Fatalf("box %d: unknown material %q", i, b.Kind)
		}
	}
}

func TestSimpleProps(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  c.Box
		want c.Box
	}{
		{"crate", Crate(1, 2, 3, 2, 1, 4), c.B(1, 2, 3, 3, 3, 7, c.Crate)},
		{"wall", Wall(-2, 5, 2, 6, 3, c.Brick), c.B(-2, 0, 5, 2, 3, 6, c.Brick)},
		{"pillar", Pillar(4, -4, 1.5, 9, c.Hazard), c.B(4, 0, -4, 5.5, 9, -2.5, c.Hazard)},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, tc.got, tc.want)
		}
		valid(t, tc.got)
	}
}

func TestCratePyramid(t *testing.T) {
	bs := CratePyramid(10, 20, 2)
	valid(t, bs...)
	want := []c.Box{
		c.B(10, 0, 20, 12, 2, 22, c.Crate),
		c.B(12, 0, 20, 14, 2, 22, c.Crate),
		c.B(11, 2, 20, 13, 4, 22, c.Crate),
	}
	if len(bs) != 3 {
		t.Fatalf("%d crates", len(bs))
	}
	for i := range want {
		if bs[i] != want[i] {
			t.Fatalf("crate %d: %+v, want %+v", i, bs[i], want[i])
		}
	}
}

func TestBuildingHasADoorway(t *testing.T) {
	const cx, cz, w, d, h, dw, dh = 5.0, -3.0, 12.0, 8.0, 5.0, 3.0, 3.0
	bs := Building(cx, cz, w, d, h, dw, dh, c.Brick)
	valid(t, bs...)
	total := 0.0
	for _, b := range bs {
		total += vol(b)
		if b.Min[2] != cz-d/2 || b.Max[2] != cz+d/2 || b.Max[1] != h {
			t.Fatalf("part %+v doesn't span the full depth and height", b)
		}
	}
	// Solid volume minus the doorway tunnel.
	if want := w*d*h - dw*dh*d; math.Abs(total-want) > 1e-9 {
		t.Fatalf("volume %v, want %v", total, want)
	}
	inside := func(x, y, z float64) bool {
		for _, b := range bs {
			if x > b.Min[0] && x < b.Max[0] && y > b.Min[1] && y < b.Max[1] && z > b.Min[2] && z < b.Max[2] {
				return true
			}
		}
		return false
	}
	for z := cz - d/2 + 0.1; z < cz+d/2; z += 0.5 {
		if inside(cx, 1, z) || inside(cx+dw/2-0.1, dh-0.1, z) {
			t.Fatalf("doorway blocked at z %v", z)
		}
		if !inside(cx, dh+0.1, z) || !inside(cx-dw/2-0.1, 1, z) || !inside(cx+dw/2+0.1, 1, z) {
			t.Fatalf("wall or lintel missing at z %v", z)
		}
	}
}

func TestStairs(t *testing.T) {
	const edge, z0, z1, run, rise = 16.0, 18.0, 26.0, 1.0, 0.5
	for n := range 5 {
		bs := StairsDownX(edge, z0, z1, run, rise, n, c.Hazard)
		if len(bs) != n {
			t.Fatalf("n=%d: %d steps", n, len(bs))
		}
		valid(t, bs...)
		for i, b := range bs {
			// Step i spans y [i·rise, (i+1)·rise], ends at the ledge, and
			// starts (n-i) runs out, so each step's tread is one run deep.
			if b.Min[1] != float64(i)*rise || b.Max[1] != float64(i+1)*rise || b.Max[0] != edge ||
				b.Min[0] != edge-run*float64(n-i) || b.Min[2] != z0 || b.Max[2] != z1 {
				t.Fatalf("n=%d step %d: %+v", n, i, b)
			}
		}
	}
}

// A container's footprint encodes its colour the way materials/container.js decodes it.
func TestContainerFootprint(t *testing.T) {
	decode := func(w float64) int {
		idx := (w - 2.4) / 0.05
		if w >= 9 {
			idx = (w - 12) / 0.2
		} else if w >= 4 {
			idx = (w - 6) / 0.1
		}
		return int(math.Floor(idx+0.5)) % 4
	}
	for colour := range 6 {
		for _, long := range []bool{false, true} {
			b := Container(1, 2.6, 3, true, long, colour)
			if b.Kind != c.Container || b.Max[1]-b.Min[1] != ContainerH || b.Min != [3]float64{1, 2.6, 3} {
				t.Fatalf("container %+v", b)
			}
			lx, wz := b.Max[0]-b.Min[0], b.Max[2]-b.Min[2]
			if decode(lx) != colour%4 || decode(wz) != colour%4 {
				t.Fatalf("colour %d long %v: length %.2f width %.2f decode %d %d", colour, long, lx, wz, decode(lx), decode(wz))
			}
			z := Container(0, 0, 0, false, long, colour)
			if math.Abs(z.Max[0]-z.Min[0]-wz) > 1e-9 || math.Abs(z.Max[2]-z.Min[2]-lx) > 1e-9 {
				t.Fatalf("along z %+v", z)
			}
		}
	}
}

// A stall: the counter on the floor, the poles at the front corners under the
// awning's outer edge, the awning overhead (walkable under), either way round.
func TestStall(t *testing.T) {
	for _, north := range []bool{false, true} {
		b := Stall(0, 0, 0, 2, 1, 1.5, north)
		counter, pole, awning := b[0], b[1], b[3]
		if counter.Min[1] != 0 || counter.Max[1] != 1 || awning.Min[1] < 2.4 {
			t.Fatalf("north %v: counter %v awning %v", north, counter, awning)
		}
		if pole.Max[1] != awning.Min[1] || pole.Min[2] < awning.Min[2] || pole.Max[2] > awning.Max[2] {
			t.Fatalf("north %v: pole %v not under the awning %v", north, pole, awning)
		}
		out := pole.Min[2] >= counter.Max[2]+1.4
		if !north {
			out = pole.Max[2] <= counter.Min[2]-1.4
		}
		if !out {
			t.Fatalf("north %v: pole %v not out in front of the counter %v", north, pole, counter)
		}
	}
}

func TestCar(t *testing.T) {
	for colour := range 4 {
		for _, alongX := range []bool{true, false} {
			bs := Car(10, -4, 20, alongX, colour)
			if len(bs) != 6 || bs[0].Kind != c.Car || bs[1].Kind != c.Glass {
				t.Fatalf("Car: %+v", bs)
			}
			body := bs[0]
			sx, sz := body.Max[0]-body.Min[0], body.Max[2]-body.Min[2]
			l, w := sx, sz
			if !alongX {
				l, w = sz, sx
			}
			// the material reads the colour back from the footprint
			if math.Abs(w-(1.8+0.02*float64(colour))) > 1e-9 || math.Abs(l-(4.2+0.1*float64(colour))) > 1e-9 {
				t.Errorf("colour %d alongX %v: body %.2f x %.2f", colour, alongX, l, w)
			}
			for _, b := range bs {
				if b.Min[1] < -4 || b.Max[1] > -4+1.5+1e-9 || b.Min[0] < body.Min[0] || b.Max[0] > body.Max[0] || b.Min[2] < body.Min[2] || b.Max[2] > body.Max[2] {
					t.Errorf("part %+v outside the car's footprint or height", b)
				}
			}
		}
	}
}

func TestRamps(t *testing.T) {
	if got, want := Ramp(0, 1, 4, 3, 0.5, 2, c.RampNX, c.Concrete), c.W(0, 0.5, 1, 4, 2.5, 3, c.Concrete, c.RampNX); got != want {
		t.Fatalf("Ramp: %+v, want %+v", got, want)
	}
	for _, alongX := range []bool{true, false} {
		r := SurfRamp(alongX, 5, -10, 10, 1, 4, 3, c.Metal)
		if len(r) != 2 || r[0].Max[1] != 5 || r[1].Max[1] != 5 || r[0].Min[1] != 1 {
			t.Fatalf("SurfRamp(%v): %+v", alongX, r)
		}
		ax := 2
		if !alongX {
			ax = 0
		}
		// Back to back at the centre line, rising towards it from both sides.
		if r[0].Max[ax] != 5 || r[1].Min[ax] != 5 || r[0].Min[ax] != 2 || r[1].Max[ax] != 8 {
			t.Fatalf("SurfRamp(%v) cross-section: %+v", alongX, r)
		}
		if r[0].Ramp == r[1].Ramp || r[0].Ramp == 0 || r[1].Ramp == 0 {
			t.Fatalf("SurfRamp(%v) directions: %d %d", alongX, r[0].Ramp, r[1].Ramp)
		}
	}
}
