package maprender

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// Every playable map draws in every view: a PNG of the frame's size, the
// longer side the size asked for.
func TestRenderEveryMap(t *testing.T) {
	for _, n := range maps.Names() {
		m := maps.Find(n)()
		for _, v := range Views {
			b, f, err := Render(m, Options{View: v, Size: 300, Labels: true, Nav: true})
			if err != nil {
				t.Fatalf("%s %s: %v", n, v, err)
			}
			img, err := png.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatalf("%s %s: %v", n, v, err)
			}
			if r := img.Bounds(); r.Dx() != f.Width || r.Dy() != f.Height || max(f.Width, f.Height) < 299 || max(f.Width, f.Height) > 302 {
				t.Fatalf("%s %s: %v vs %+v", n, v, r, f)
			}
			if !strings.Contains(f.Caption(), v+" view") {
				t.Fatalf("caption %q", f.Caption())
			}
		}
	}
}

func testMap() content.Map {
	return content.Map{
		Name: "t",
		Boxes: []content.Box{
			content.B(-10, -1, -10, 10, 0, 10, content.Concrete),
			content.B(-2, 0, -2, 2, 2, 2, "crate"),
			content.W(4, 0, -2, 8, 2, 2, content.Concrete, content.RampPX),
		},
		Spawns:     [][3]float64{{-8, 0, 0}, {8, 0, 8}},
		TeamSpawns: map[string][][3]float64{content.Vibe: {{-8, 0, 0}}},
		Sites:      []content.Zone{{Name: "A", Min: [3]float64{-9, 0, 5}, Max: [3]float64{-5, 2, 9}}},
		Items:      []content.Item{{Weapon: "x", Pos: [3]float64{0, 0, -6}}},
		Nav:        []content.NavNode{{Name: "a", Pos: [3]float64{-8, 0, -8}, Links: []string{"b"}}, {Name: "b", Pos: [3]float64{8, 0, -8}}},
	}
}

func TestRenderPixels(t *testing.T) {
	m := testMap()
	cyan := color.RGBA{0, 255, 255, 255}
	red := color.RGBA{255, 0, 0, 255}
	img, f, err := Draw(m, Options{Size: 400, Nav: true, Labels: true,
		Marks:  []Mark{{Pos: [3]float64{0, 2, -6}, Color: cyan, Label: "mcp"}},
		Lights: []Highlight{{Min: m.Boxes[1].Min, Max: m.Boxes[1].Max, Color: red}}})
	if err != nil {
		t.Fatal(err)
	}
	if f.View != Top || f.Across != "x" || f.Down != "z" || f.Min != [3]float64{-11, -2, -11} {
		t.Fatalf("frame %+v", f)
	}
	at := func(img *image.RGBA, f Frame, p [3]float64) color.RGBA {
		x, y := f.Pixel(p)
		return img.RGBAAt(x, y)
	}
	if c := at(img, f, [3]float64{0.5, 0, 0.5}); !(c.R > c.G && c.G > c.B) { // the crate: brown
		t.Fatalf("crate pixel %v", c)
	}
	if c := at(img, f, [3]float64{-2, 0, 0}); c != red { // its highlight on its edge
		t.Fatalf("highlight %v", c)
	}
	lo, hi := at(img, f, [3]float64{4.5, 0, 0}), at(img, f, [3]float64{7.5, 0, 0})
	if lo.R >= hi.R { // the wedge rises to +x: lighter there
		t.Fatalf("wedge shading %v %v", lo, hi)
	}
	if c := at(img, f, [3]float64{-8, 0, 0}); c != teamColor(content.Vibe) {
		t.Fatalf("vibe spawn %v", c)
	}
	if c := at(img, f, [3]float64{-9, 0, 7}); c != (color.RGBA{255, 170, 40, 255}) {
		t.Fatalf("site outline %v", c)
	}
	if c := at(img, f, [3]float64{0, 0, -6 + 8/f.Scale}); c != cyan { // the mark's crosshair
		t.Fatalf("mark %v", c)
	}
	// From the side: the wedge is a triangle (empty above its low end).
	side, sf, err := Draw(m, Options{View: Side, Size: 400})
	if err != nil {
		t.Fatal(err)
	}
	if sf.Down != "-y" || sf.Across != "x" {
		t.Fatalf("side frame %+v", sf)
	}
	above := func(x float64) color.RGBA { return at(side, sf, [3]float64{x, 1.6, 0}) }
	if bg := above(9.5); above(4.4) != bg || above(7.8) == bg {
		t.Fatalf("wedge from the side: %v %v (background %v)", above(4.4), above(7.8), bg)
	}
	// A region: only what is in it, framed by it.
	_, rf, _ := Draw(m, Options{Min: &[3]float64{0, -1, -4}, Max: &[3]float64{10, 3, 4}, Size: 200})
	if rf.Min != [3]float64{0, -1, -4} || rf.Width != 200 || rf.Height != 160 {
		t.Fatalf("region frame %+v", rf)
	}
	if _, _, err := Render(m, Options{View: "iso"}); err == nil {
		t.Fatal("a bad view rendered")
	}
}

// An empty map, a reversed region, odd materials and characters: no panics,
// deterministic colours.
func TestRenderOddities(t *testing.T) {
	if _, f, err := Draw(content.Map{}, Options{Size: 10}); err != nil || f.Width != 128 {
		t.Fatalf("empty: %+v %v", f, err)
	}
	if _, f, _ := Draw(testMap(), Options{Min: &[3]float64{5, 5, 5}, Max: &[3]float64{-5, -5, -5}, Size: 9000}); f.Min != [3]float64{-5, -5, -5} || f.Width != 2048 {
		t.Fatalf("reversed region: %+v", f)
	}
	if materialColor("unobtainium") != materialColor("unobtainium") || materialColor("unobtainium") == materialColor("crate") {
		t.Fatal("material colours")
	}
	for _, k := range content.Materials {
		_ = materialColor(k)
	}
	m := testMap()
	m.Boxes = append(m.Boxes, content.W(-8, 0, -8, -4, 2, -4, "metal", content.RampNZ), content.W(-8, 0, 4, -4, 2, 8, "x", content.RampPZ), content.W(-8, 0, -4, -4, 2, 0, "x", content.RampNX))
	m.Water = []content.Box{content.B(-10, -1, 9, 10, 0, 10, content.Water)}
	m.Pits = []content.Zone{{Name: "pit", Min: [3]float64{-1, -1, -9}, Max: [3]float64{1, 0, -8}}}
	m.Exits = []content.Zone{{Name: "dock", Min: [3]float64{8, 0, 8}, Max: [3]float64{9, 2, 9}}}
	m.BuyZones = map[string][]content.Zone{content.Artisan: {{Min: [3]float64{5, 0, 5}, Max: [3]float64{9, 2, 9}}}, "other": {{}}}
	m.GPUs = []content.GPUSpot{{Pos: [3]float64{1, 0, 1}}}
	cut := 1.0
	m.Cut = &cut
	for _, v := range Views {
		if _, _, err := Draw(m, Options{View: v, Labels: true, Nav: true, Marks: []Mark{{Pos: [3]float64{100, 0, 0}, Label: "é?"}}}); err != nil {
			t.Fatal(err)
		}
	}
}
