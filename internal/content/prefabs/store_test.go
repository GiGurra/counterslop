package prefabs

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

// coplanar lists pairs of boxes whose same-facing faces lie in one plane and overlap
// with real area (they z-fight): the maps package's TestNoZFighting rule, here per prop.
func coplanar(boxes []c.Box) []string {
	var out []string
	for i, a := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			b := boxes[j]
			for ax := range 3 {
				u, v := (ax+1)%3, (ax+2)%3
				ou := math.Min(a.Max[u], b.Max[u]) - math.Max(a.Min[u], b.Min[u])
				ov := math.Min(a.Max[v], b.Max[v]) - math.Max(a.Min[v], b.Min[v])
				if ou <= 0.01 || ov <= 0.01 {
					continue
				}
				if math.Abs(a.Min[ax]-b.Min[ax]) < 1e-6 && !(ax == 1 && a.Min[1] <= 0) {
					out = append(out, fmt.Sprintf("%d (%s) and %d (%s): min %c faces at %g", i, a.Kind, j, b.Kind, "xyz"[ax], a.Min[ax]))
				}
				if math.Abs(a.Max[ax]-b.Max[ax]) < 1e-6 {
					out = append(out, fmt.Sprintf("%d (%s) and %d (%s): max %c faces at %g", i, a.Kind, j, b.Kind, "xyz"[ax], a.Max[ax]))
				}
			}
		}
	}
	return out
}

// bounds of a prop.
func bounds(boxes []c.Box) (mn, mx [3]float64) {
	mn, mx = boxes[0].Min, boxes[0].Max
	for _, b := range boxes {
		for a := range 3 {
			mn[a], mx[a] = math.Min(mn[a], b.Min[a]), math.Max(mx[a], b.Max[a])
		}
	}
	return
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// storeProp checks one prop: valid boxes in known materials, inside the footprint
// (x0, z0)-(x1, z1) and y..y+h, standing on y (unless it hangs: TV walls), and no
// z-fighting inside it or with a floor slab under it.
func storeProp(t *testing.T, name string, boxes []c.Box, x0, z0, x1, z1, y, h float64) {
	storePropAt(t, name, boxes, x0, z0, x1, z1, y, h, true)
}

func storePropAt(t *testing.T, name string, boxes []c.Box, x0, z0, x1, z1, y, h float64, stands bool) {
	t.Helper()
	if len(boxes) == 0 {
		t.Fatalf("%s: no boxes", name)
	}
	for i, b := range boxes {
		for a := range 3 {
			if !(b.Min[a] < b.Max[a]) {
				t.Errorf("%s: box %d empty on axis %d: %+v", name, i, a, b)
			}
		}
		if !slices.Contains(c.Materials, b.Kind) {
			t.Errorf("%s: box %d: unknown material %q", name, i, b.Kind)
		}
	}
	mn, mx := bounds(boxes)
	if mn[0] < x0-1e-6 || mn[2] < z0-1e-6 || mx[0] > x1+1e-6 || mx[2] > z1+1e-6 {
		t.Errorf("%s: footprint %v-%v, want inside (%g, %g)-(%g, %g)", name, mn, mx, x0, z0, x1, z1)
	}
	if (stands && !near(mn[1], y)) || mn[1] < y-1e-6 || mx[1] > y+h+1e-6 {
		t.Errorf("%s: y %g..%g, want from %g up to %g", name, mn[1], mx[1], y, y+h)
	}
	floor := c.B(x0-2, y-0.3, z0-2, x1+2, y, z1+2, c.StoreTile)
	for _, f := range coplanar(append([]c.Box{floor}, boxes...)) {
		t.Errorf("%s: z-fighting: %s", name, f)
	}
}

func TestStoreProps(t *testing.T) {
	const y = 4.5 // an upper floor: everything stands on y, not on 0
	for _, alongX := range []bool{true, false} {
		ax := map[bool]string{true: "x", false: "z"}[alongX]
		// fp turns a footprint (long along the axis) into world extents at (x, z).
		fp := func(x, z, l, d float64) (float64, float64, float64, float64) {
			if alongX {
				return x, z, x + l, z + d
			}
			return x, z, x + d, z + l
		}
		x0, z0, x1, z1 := fp(10, -3, 6, 1.2)
		storeProp(t, "gondola along "+ax, Gondola(10, y, -3, 6, alongX), x0, z0, x1, z1, y, GondolaH+0.45)
		x0, z0, x1, z1 = fp(10, -3, 5, 1.0)
		storeProp(t, "stock shelf along "+ax, StockShelf(10, y, -3, 5, alongX), x0, z0, x1, z1, y, StockH)
		x0, z0, x1, z1 = fp(10, -3, 4, 1.0)
		storeProp(t, "checkout along "+ax, CheckoutLane(10, y, -3, alongX), x0, z0, x1, z1, y, 2.75)
		x0, z0, x1, z1 = fp(10, -3, 4, 1.6)
		storeProp(t, "LAN desk along "+ax, LanDesk(10, y, -3, alongX, 4), x0, z0, x1, z1, y, 1.4)
		x0, z0, x1, z1 = fp(10, -3, 0.7, 0.08)
		storeProp(t, "price sign along "+ax, PriceSign(10, y, -3, alongX), x0, z0, x1, z1, y, 1.95)
	}
	for r := range 4 {
		w, d := 0.7, 1.1
		if r%2 == 1 {
			w, d = d, w
		}
		storeProp(t, fmt.Sprintf("cart yaw4 %d", r), Cart(2, y, 3, r), 2, 3, 2+w, 3+d, y, 1.0)
	}
	storeProp(t, "kiosk", Kiosk(2, y, 3), 2, 3, 3, 4, y, 1.3)
	storePropAt(t, "TV wall along x", TVWall(0, 5, 8, 5.25, y), 0, 5, 8, 5.25, y, 3.3, false)
	storePropAt(t, "TV wall along z", TVWall(-0.3, 0, 0, 6, y), -0.3, 0, 0, 6, y, 3.3, false)
}

func TestStorePropDetails(t *testing.T) {
	// The gondola's long faces must read as shelves (> 1.3 m), the short as end caps.
	g := Gondola(0, 0, 0, 3, true)[0]
	if g.Kind != c.Shelf || g.Max[0]-g.Min[0] <= 1.3 || g.Max[2]-g.Min[2] >= 1.3 {
		t.Errorf("gondola body %+v", g)
	}
	// The stock shelf's middle shelf is clear for the GPUs: nothing sits between its top
	// and the next shelf up.
	for _, b := range StockShelf(0, 0, 0, 4, true) {
		if b.Kind == c.Carton && b.Max[1] > StockMid && b.Min[1] < StockHigh {
			t.Errorf("a carton on the GPU shelf: %+v", b)
		}
	}
	// A cart's handle is at the side yaw4 says.
	handle := func(r int) c.Box {
		for _, b := range Cart(0, 0, 0, r) {
			if b.Kind == c.Neon {
				return b
			}
		}
		t.Fatal("no handle")
		return c.Box{}
	}
	if h := handle(0); h.Min[2] < 0.9 {
		t.Errorf("yaw4 0: handle at +z, got %+v", h)
	}
	if h := handle(1); h.Min[0] < 0.9 {
		t.Errorf("yaw4 1: handle at +x, got %+v", h)
	}
	if h := handle(2); h.Max[2] > 0.2 {
		t.Errorf("yaw4 2: handle at -z, got %+v", h)
	}
	if h := handle(3); h.Max[0] > 0.2 {
		t.Errorf("yaw4 3: handle at -x, got %+v", h)
	}
	// TV walls: as many 1.2 m screens as fit, two rows.
	n := 0
	for _, b := range TVWall(0, 0, 8, 0.2, 0) {
		if b.Kind == c.Screen {
			n++
		}
	}
	if n != 12 {
		t.Errorf("an 8 m TV wall: %d screens, want 2 rows of 6", n)
	}
	// One seat per metre at a LAN desk, each with a monitor and a tower.
	s := 0
	for _, b := range LanDesk(0, 0, 0, true, 5) {
		if b.Kind == c.Screen || b.Kind == c.PCTower {
			s++
		}
	}
	if s != 10 {
		t.Errorf("5 seats: %d screens + towers, want 10", s)
	}
}

// The gallery's store showroom (web/dev/storeprops.json): every store prop on a store
// floor, so /dev/assets shows them with the real materials. A golden file:
// UPDATE=1 go test ./internal/content/prefabs rewrites it.
func showroom() []c.Box {
	const z = -21.0 // behind the gallery's material row, on its own floor
	out := []c.Box{c.B(-22, -1, z-1, 22, 0, -12.5, c.StoreTile)}
	out = append(out, Gondola(-20, 0, z, 5, true)...)
	out = append(out, Gondola(-20, 0, z+2.5, 5, true)...)
	out = append(out, StockShelf(-13.5, 0, z, 5, true)...)
	out = append(out, CheckoutLane(-7, 0, z, false)...)
	out = append(out, CheckoutLane(-5, 0, z, false)...)
	out = append(out, LanDesk(-2, 0, z, true, 4)...)
	out = append(out, TVWall(3, z-0.3, 11, z, 0)...)
	out = append(out, c.B(3, 0, z-0.6, 11, 3.6, z-0.3, c.Plaster)) // the wall it hangs on
	out = append(out, Kiosk(4, 0, z+1.5)...)
	out = append(out, Kiosk(6, 0, z+1.5)...)
	out = append(out, PriceSign(8.5, 0, z+2, true)...)
	for r := range 4 {
		out = append(out, Cart(12+float64(r)*1.6, 0, z+0.5, r)...)
	}
	// an escalator stub: two flights of steps
	for i := range 4 {
		h := 0.25 * float64(i+1)
		out = append(out, c.B(19, 0, z+float64(i)*0.4, 20, h, z+float64(i+1)*0.4, c.Escalator))
		out = append(out, c.B(20.2, 0, z+float64(i)*0.4, 21.2, h, z+float64(i+1)*0.4, c.Escalator))
	}
	out = append(out, c.B(18.9, 0, z, 18.98, 2.0, z+1.6, c.Glass), c.B(21.22, 0, z, 21.3, 2.0, z+1.6, c.Glass))
	out = append(out, c.B(-2, 0, z+2.2, 2, 0.02, z+4, c.LanCarpet))
	return out
}

func TestShowroomGolden(t *testing.T) {
	boxes := showroom()
	for _, f := range coplanar(boxes) {
		t.Errorf("showroom z-fighting: %s", f)
	}
	got, err := json.Marshal(map[string]any{"boxes": boxes})
	if err != nil {
		t.Fatal(err)
	}
	const path = "../../../web/dev/storeprops.json"
	if os.Getenv("UPDATE") != "" {
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got)+"\n" {
		t.Errorf("%s is stale: UPDATE=1 go test ./internal/content/prefabs", path)
	}
}
