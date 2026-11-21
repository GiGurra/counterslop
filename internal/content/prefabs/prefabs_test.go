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
