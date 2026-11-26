// Package maprender draws a map as a flat picture: from above (top), from the
// side (side: x across, y up) or from the front (front: z across, y up), with
// its boxes and wedges shaded by height or depth, the zones (sites, buy zones,
// pits, exits, water) outlined, spawns, items, GPUs and the nav graph marked,
// a coordinate grid with labels, and whatever else the caller marks or
// highlights (the map editor's selections, where each editor aims). It is for
// an agent editing a map over MCP (docs/map-editor.md) and for tests: plain
// pixels, no fonts or assets (its labels use a tiny bitmap font drawn here).
package maprender

import (
	"bytes"
	"cmp"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"math"
	"slices"
	"strings"

	"github.com/gigurra/counterslop/internal/content"
)

// Views.
const (
	Top   = "top"   // from above: x right, z down
	Side  = "side"  // from -z: x right, y up
	Front = "front" // from -x: z right, y up
)

// Views lists them.
var Views = []string{Top, Side, Front}

// Mark is a point to show: a crosshair with a label (an editor's aim point).
type Mark struct {
	Pos   [3]float64
	Color color.RGBA
	Label string
}

// Highlight outlines a volume (a selected box, zone, or a small one around a
// spawn, item or nav node) in a colour; Thin for a hover.
type Highlight struct {
	Min, Max [3]float64
	Color    color.RGBA
	Thin     bool
}

// Options say what to draw.
type Options struct {
	View string // Top (default), Side, Front
	// Min and Max, when set, are the region to draw (world metres): only what
	// is in it, framed by it. For a top view of one floor, a y range: Min[1],
	// Max[1]. Default: the whole map (a map's Cut leaves out its lid from above).
	Min, Max *[3]float64
	Size     int  // the longer side in pixels (default 800; 128-2048)
	Labels   bool // box indices, nav node names
	Nav      bool // the nav graph
	Marks    []Mark
	Lights   []Highlight
}

// Frame says how the picture maps to the world.
type Frame struct {
	View   string     `json:"view"`
	Width  int        `json:"width"`
	Height int        `json:"height"`
	Scale  float64    `json:"pxPerMetre"`
	Across string     `json:"across"` // the world axis to the right
	Down   string     `json:"down"`   // the world axis downwards ("-y": y up)
	Min    [3]float64 `json:"min"`    // the region drawn
	Max    [3]float64 `json:"max"`
	Grid   float64    `json:"gridMetres"` // grid line spacing
}

// Pixel is where world point p is in the picture.
func (f Frame) Pixel(p [3]float64) (x, y int) {
	ax, _ := axesOf(f.View)
	c := canvas{ax: ax, lo: f.Min, hi: f.Max, scale: f.Scale}
	return c.px(p)
}

// Caption says it in words, for whoever reads the picture.
func (f Frame) Caption() string {
	vert := map[string]string{"z": "z grows downwards", "-y": "y up"}[f.Down]
	return fmt.Sprintf("%s view, %dx%d px, %.2f px/m: %s grows to the right, %s; region min %v max %v; grid every %g m (labels at the edges). "+
		"Boxes shaded by %s; outlines: sites orange (A/B), buy zones in the team colour, pits red, exits green, water blue; "+
		"spawns: vibe magenta, artisan orange dots; items yellow diamonds; GPUs green; nav grey.",
		f.View, f.Width, f.Height, f.Scale, f.Across, vert, round2(f.Min), round2(f.Max), f.Grid,
		map[bool]string{true: "height (lighter is higher)", false: "nearness (lighter is nearer)"}[f.View == Top])
}

func round2(v [3]float64) [3]float64 {
	for i := range v {
		v[i] = math.Round(v[i]*100) / 100
	}
	return v
}

// axes: the world axis across (u), down the picture (v, flipped for y) and
// along the view (d; nearer is drawn later: higher y from above, lower z or
// x from the side or front).
type axes struct {
	u, v, d   int
	flipV     bool
	nearFirst bool // d: a smaller value is nearer
}

func axesOf(view string) (axes, error) {
	switch view {
	case "", Top:
		return axes{u: 0, v: 2, d: 1}, nil
	case Side:
		return axes{u: 0, v: 1, d: 2, flipV: true, nearFirst: true}, nil
	case Front:
		return axes{u: 2, v: 1, d: 0, flipV: true, nearFirst: true}, nil
	}
	return axes{}, fmt.Errorf("view %q (%s)", view, strings.Join(Views, ", "))
}

var axisName = [3]string{"x", "y", "z"}

// Render draws m as o says, as a PNG.
func Render(m content.Map, o Options) ([]byte, Frame, error) {
	img, f, err := Draw(m, o)
	if err != nil {
		return nil, f, err
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, f, err
	}
	return b.Bytes(), f, nil
}

// Draw draws m as o says.
func Draw(m content.Map, o Options) (*image.RGBA, Frame, error) {
	ax, err := axesOf(o.View)
	if err != nil {
		return nil, Frame{}, err
	}
	if o.View == "" {
		o.View = Top
	}
	lo, hi := bounds(m, o, ax)
	size := o.Size
	if size == 0 {
		size = 800
	}
	size = min(max(size, 128), 2048)
	ext := max(hi[ax.u]-lo[ax.u], hi[ax.v]-lo[ax.v], 1)
	scale := float64(size) / ext
	w := max(int(math.Ceil((hi[ax.u]-lo[ax.u])*scale)), 16)
	h := max(int(math.Ceil((hi[ax.v]-lo[ax.v])*scale)), 16)
	c := &canvas{img: image.NewRGBA(image.Rect(0, 0, w, h)), ax: ax, lo: lo, hi: hi, scale: scale}
	f := Frame{View: o.View, Width: w, Height: h, Scale: scale, Across: axisName[ax.u], Down: map[bool]string{true: "-y", false: axisName[ax.v]}[ax.flipV],
		Min: lo, Max: hi}
	c.fill(color.RGBA{24, 26, 32, 255})
	f.Grid = c.grid(false)
	c.boxes(m, o)
	for _, wb := range m.Water {
		c.zone(wb.Min, wb.Max, color.RGBA{60, 140, 255, 255}, "", true)
	}
	for _, z := range m.Pits {
		c.zone(z.Min, z.Max, color.RGBA{255, 60, 60, 255}, z.Name, false)
	}
	for _, z := range m.Exits {
		c.zone(z.Min, z.Max, color.RGBA{80, 220, 120, 255}, z.Name, false)
	}
	for _, t := range sortedKeys(m.BuyZones) {
		for _, z := range m.BuyZones[t] {
			c.zone(z.Min, z.Max, teamColor(t), "", false)
		}
	}
	for _, z := range m.Sites {
		c.zone(z.Min, z.Max, color.RGBA{255, 170, 40, 255}, z.Name, false)
	}
	if o.Nav {
		c.nav(m.Nav, o.Labels)
	}
	team := map[[3]float64]string{}
	for _, t := range sortedKeys(m.TeamSpawns) {
		for _, p := range m.TeamSpawns[t] {
			team[p] = t
		}
	}
	for _, p := range m.Spawns {
		if team[p] == "" {
			c.dot(p, 3, color.RGBA{170, 170, 170, 255})
		}
	}
	for _, t := range sortedKeys(m.TeamSpawns) {
		for _, p := range m.TeamSpawns[t] {
			c.dot(p, 4, teamColor(t))
		}
	}
	for _, it := range m.Items {
		c.diamond(it.Pos, 4, color.RGBA{255, 230, 60, 255})
	}
	for _, g := range m.GPUs {
		c.square(g.Pos, 3, color.RGBA{90, 255, 120, 255})
	}
	for _, l := range o.Lights {
		c.outline(l.Min, l.Max, l.Color, !l.Thin)
	}
	for _, mk := range o.Marks {
		c.cross(mk.Pos, mk.Color, mk.Label)
	}
	c.grid(true) // the labels on top of it all
	return c.img, f, nil
}

// bounds: the region to draw.
func bounds(m content.Map, o Options, ax axes) (lo, hi [3]float64) {
	first := true
	grow := func(a, b [3]float64) {
		for i := range 3 {
			if first || a[i] < lo[i] {
				lo[i] = a[i]
			}
			if first || b[i] > hi[i] {
				hi[i] = b[i]
			}
		}
		first = false
	}
	for _, b := range m.Boxes {
		grow(b.Min, b.Max)
	}
	for _, p := range m.Spawns {
		grow(p, p)
	}
	for _, z := range m.Sites {
		grow(z.Min, z.Max)
	}
	if first {
		lo, hi = [3]float64{-16, -1, -16}, [3]float64{16, 4, 16}
	}
	for i := range 3 { // a margin for the outlines and labels
		lo[i] -= 1
		hi[i] += 1
	}
	if m.Cut != nil && ax.d == 1 && hi[1] > *m.Cut {
		hi[1] = *m.Cut // from above: the lid off
	}
	if o.Min != nil {
		lo = *o.Min
	}
	if o.Max != nil {
		hi = *o.Max
	}
	for i := range 3 {
		if hi[i] < lo[i] {
			lo[i], hi[i] = hi[i], lo[i]
		}
	}
	return lo, hi
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

func teamColor(t string) color.RGBA {
	switch t {
	case content.Vibe:
		return color.RGBA{230, 70, 230, 255}
	case content.Artisan:
		return color.RGBA{255, 140, 30, 255}
	}
	return color.RGBA{200, 200, 200, 255}
}

// materialColor: a material's colour in the picture, roughly as it looks in game.
func materialColor(kind string) color.RGBA {
	switch kind {
	case content.Concrete, "plaster", "stucco":
		return color.RGBA{150, 152, 158, 255}
	case "sand":
		return color.RGBA{196, 170, 120, 255}
	case "brick":
		return color.RGBA{170, 90, 70, 255}
	case "crate", "desk", "carton", "shelf":
		return color.RGBA{160, 115, 70, 255}
	case "metal", "rack", "container", "escalator", "handrail", "pctower":
		return color.RGBA{120, 135, 150, 255}
	case "hazard":
		return color.RGBA{220, 190, 40, 255}
	case "carpet", "lancarpet":
		return color.RGBA{130, 60, 70, 255}
	case "glass", "screen":
		return color.RGBA{150, 200, 230, 255}
	case "water":
		return color.RGBA{60, 120, 200, 255}
	case "neon":
		return color.RGBA{255, 80, 220, 255}
	case "pcb", "ram":
		return color.RGBA{40, 140, 80, 255}
	case "lgtm":
		return color.RGBA{90, 200, 90, 255}
	case "deprecated":
		return color.RGBA{200, 80, 80, 255}
	}
	h := fnv.New32a()
	h.Write([]byte(kind))
	v := h.Sum32()
	return color.RGBA{uint8(90 + v%110), uint8(90 + (v>>8)%110), uint8(90 + (v>>16)%110), 255}
}

type canvas struct {
	img    *image.RGBA
	ax     axes
	lo, hi [3]float64
	scale  float64
}

// px: a world point's pixel.
func (c *canvas) px(p [3]float64) (int, int) {
	x := (p[c.ax.u] - c.lo[c.ax.u]) * c.scale
	y := (p[c.ax.v] - c.lo[c.ax.v]) * c.scale
	if c.ax.flipV {
		y = (c.hi[c.ax.v] - p[c.ax.v]) * c.scale
	}
	return int(math.Floor(x)), int(math.Floor(y))
}

// world: a pixel centre's (u, v) in world metres.
func (c *canvas) world(x, y int) (float64, float64) {
	u := c.lo[c.ax.u] + (float64(x)+0.5)/c.scale
	v := c.lo[c.ax.v] + (float64(y)+0.5)/c.scale
	if c.ax.flipV {
		v = c.hi[c.ax.v] - (float64(y)+0.5)/c.scale
	}
	return u, v
}

func (c *canvas) set(x, y int, col color.RGBA) {
	if image.Pt(x, y).In(c.img.Rect) {
		c.img.SetRGBA(x, y, col)
	}
}

func (c *canvas) blend(x, y int, col color.RGBA, a float64) {
	if !image.Pt(x, y).In(c.img.Rect) {
		return
	}
	o := c.img.RGBAAt(x, y)
	mix := func(p, q uint8) uint8 { return uint8(float64(p)*(1-a) + float64(q)*a) }
	c.img.SetRGBA(x, y, color.RGBA{mix(o.R, col.R), mix(o.G, col.G), mix(o.B, col.B), 255})
}

func (c *canvas) fill(col color.RGBA) {
	for i := 0; i < len(c.img.Pix); i += 4 {
		c.img.Pix[i], c.img.Pix[i+1], c.img.Pix[i+2], c.img.Pix[i+3] = col.R, col.G, col.B, 255
	}
}

// grid: lines every step metres (a power of two giving 6-16 lines), or
// (labels) their coordinates at the left and top edges. Returns the step.
func (c *canvas) grid(labels bool) float64 {
	ext := max(c.hi[c.ax.u]-c.lo[c.ax.u], c.hi[c.ax.v]-c.lo[c.ax.v])
	step := 1.0
	for ext/step > 16 {
		step *= 2
	}
	line := color.RGBA{44, 48, 58, 255}
	axis := color.RGBA{70, 76, 92, 255}
	for k := math.Ceil(c.lo[c.ax.u] / step); k*step <= c.hi[c.ax.u]; k++ {
		var p [3]float64
		p[c.ax.u] = k * step
		x, _ := c.px(p)
		col := line
		if k == 0 {
			col = axis
		}
		if labels {
			c.text(x+2, 2, fmtM(k*step), color.RGBA{150, 156, 170, 255})
			continue
		}
		for y := range c.img.Rect.Dy() {
			c.set(x, y, col)
		}
	}
	for k := math.Ceil(c.lo[c.ax.v] / step); k*step <= c.hi[c.ax.v]; k++ {
		var p [3]float64
		p[c.ax.v] = k * step
		_, y := c.px(p)
		col := line
		if k == 0 {
			col = axis
		}
		if labels {
			c.text(2, y+2, fmtM(k*step), color.RGBA{150, 156, 170, 255})
			continue
		}
		for x := range c.img.Rect.Dx() {
			c.set(x, y, col)
		}
	}
	return step
}

func fmtM(v float64) string { return strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0") }

// boxes: far ones first, each filled in its material's colour shaded by
// height (from above) or nearness, outlined darker; wedges by their slope.
// From the side and front they are see-through (a faint fill, a solid
// outline): the outer walls would hide the whole map.
func (c *canvas) boxes(m content.Map, o Options) {
	type item struct {
		i int
		b content.Box
	}
	var bs []item
	for i, b := range m.Boxes {
		if c.inRegion(b.Min, b.Max) {
			bs = append(bs, item{i, b})
		}
	}
	d := c.ax.d
	slices.SortStableFunc(bs, func(a, b item) int {
		if c.ax.nearFirst { // far (larger) first
			return cmp.Compare(b.b.Min[d]+b.b.Max[d], a.b.Min[d]+a.b.Max[d])
		}
		return cmp.Compare(a.b.Max[d], b.b.Max[d]) // low tops first
	})
	dlo, dhi := c.lo[d], c.hi[d]
	shade := func(depth float64) float64 { // 0.45 far/low .. 1 near/high
		t := (depth - dlo) / max(dhi-dlo, 1e-9)
		if c.ax.nearFirst {
			t = 1 - t
		}
		return 0.45 + 0.55*min(max(t, 0), 1)
	}
	for _, it := range bs {
		b := it.b
		base := materialColor(b.Kind)
		x0, y0 := c.px(b.Min)
		x1, y1 := c.px(b.Max)
		x0, x1 = min(x0, x1), max(x0, x1)
		y0, y1 = min(y0, y1), max(y0, y1)
		x1, y1 = max(x1, x0+1), max(y1, y0+1)
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				u, v := c.world(x, y)
				depth, in := c.boxAt(b, u, v)
				if !in {
					continue
				}
				k := shade(depth)
				col := color.RGBA{uint8(float64(base.R) * k), uint8(float64(base.G) * k), uint8(float64(base.B) * k), 255}
				edge := x == x0 || y == y0 || x == x1-1 || y == y1-1
				switch {
				case c.ax.nearFirst && edge:
					c.set(x, y, col)
				case c.ax.nearFirst:
					c.blend(x, y, col, 0.3)
				case edge:
					c.set(x, y, color.RGBA{col.R / 2, col.G / 2, col.B / 2, 255})
				default:
					c.set(x, y, col)
				}
			}
		}
		if o.Labels && x1-x0 >= 14 && y1-y0 >= 9 {
			s := fmt.Sprint(it.i)
			c.text((x0+x1)/2-textWidth(s)/2, (y0+y1)/2-glyphH, s, color.RGBA{255, 255, 255, 255})
		}
	}
}

// boxAt: whether box b covers the picture's (u, v), and its depth there (a
// wedge's surface height from above; its near face from the side).
func (c *canvas) boxAt(b content.Box, u, v float64) (float64, bool) {
	near := b.Min[c.ax.d]
	if !c.ax.nearFirst {
		near = b.Max[c.ax.d]
	}
	if b.Ramp == 0 {
		return near, true
	}
	axis, up := 0, true
	switch b.Ramp {
	case content.RampNX:
		up = false
	case content.RampPZ:
		axis = 2
	case content.RampNZ:
		axis, up = 2, false
	}
	t := func(w float64) float64 { // 0 at the low side, 1 at the high
		f := (w - b.Min[axis]) / max(b.Max[axis]-b.Min[axis], 1e-9)
		if !up {
			f = 1 - f
		}
		return min(max(f, 0), 1)
	}
	switch {
	case c.ax.d == 1: // from above: the slope's height
		w := u
		if axis == c.ax.v {
			w = v
		}
		return b.Min[1] + (b.Max[1]-b.Min[1])*t(w), true
	case axis == c.ax.u: // the slope across the picture: a triangle
		return near, v <= b.Min[1]+(b.Max[1]-b.Min[1])*t(u)
	}
	return near, true // end on: its high side's silhouette
}

func (c *canvas) inRegion(lo, hi [3]float64) bool {
	for i := range 3 {
		if hi[i] < c.lo[i] || lo[i] > c.hi[i] {
			return false
		}
	}
	return true
}

// zone: an outline (filled faintly for water) with its name in a corner.
func (c *canvas) zone(lo, hi [3]float64, col color.RGBA, name string, wash bool) {
	if !c.inRegion(lo, hi) {
		return
	}
	x0, y0, x1, y1 := c.rect(lo, hi)
	if wash {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				c.blend(x, y, col, 0.25)
			}
		}
	}
	c.outline(lo, hi, col, true)
	if name != "" {
		c.text(x0+3, y0+3, strings.ToUpper(name), col)
	}
}

func (c *canvas) rect(lo, hi [3]float64) (x0, y0, x1, y1 int) {
	x0, y0 = c.px(lo)
	x1, y1 = c.px(hi)
	return min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1)
}

// outline: a rectangle round the volume (2 px thick, or 1).
func (c *canvas) outline(lo, hi [3]float64, col color.RGBA, thick bool) {
	x0, y0, x1, y1 := c.rect(lo, hi)
	w := 1
	if thick {
		w = 2
	}
	for k := range w {
		for x := x0 - k; x <= x1+k; x++ {
			c.set(x, y0-k, col)
			c.set(x, y1+k, col)
		}
		for y := y0 - k; y <= y1+k; y++ {
			c.set(x0-k, y, col)
			c.set(x1+k, y, col)
		}
	}
}

func (c *canvas) dot(p [3]float64, r int, col color.RGBA) {
	if !c.inRegion(p, p) {
		return
	}
	x, y := c.px(p)
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if d := dx*dx + dy*dy; d <= r*r {
				if d > (r-1)*(r-1) {
					c.set(x+dx, y+dy, color.RGBA{0, 0, 0, 255})
				} else {
					c.set(x+dx, y+dy, col)
				}
			}
		}
	}
}

func (c *canvas) diamond(p [3]float64, r int, col color.RGBA) {
	if !c.inRegion(p, p) {
		return
	}
	x, y := c.px(p)
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if abs(dx)+abs(dy) <= r {
				c.set(x+dx, y+dy, col)
			}
		}
	}
}

func (c *canvas) square(p [3]float64, r int, col color.RGBA) {
	if !c.inRegion(p, p) {
		return
	}
	x, y := c.px(p)
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			c.set(x+dx, y+dy, col)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// nav: links as thin lines, nodes as dots (named with labels).
func (c *canvas) nav(nodes []content.NavNode, labels bool) {
	at := map[string][3]float64{}
	for _, n := range nodes {
		at[n.Name] = n.Pos
	}
	col := color.RGBA{200, 200, 210, 255}
	for _, n := range nodes {
		for _, l := range n.Links {
			if q, ok := at[l]; ok && n.Name < l {
				c.line(n.Pos, q, col, 0.55)
			}
		}
	}
	for _, n := range nodes {
		c.dot(n.Pos, 2, col)
		if labels && c.inRegion(n.Pos, n.Pos) {
			x, y := c.px(n.Pos)
			c.text(x+4, y-2, strings.ToUpper(n.Name), col)
		}
	}
}

// line: a to b, blended in at alpha.
func (c *canvas) line(a, b [3]float64, col color.RGBA, alpha float64) {
	x0, y0 := c.px(a)
	x1, y1 := c.px(b)
	n := max(abs(x1-x0), abs(y1-y0), 1)
	for k := 0; k <= n; k++ {
		t := float64(k) / float64(n)
		c.blend(int(math.Round(float64(x0)+t*float64(x1-x0))), int(math.Round(float64(y0)+t*float64(y1-y0))), col, alpha)
	}
}

// cross: an aim point: a crosshair with a ring and a label.
func (c *canvas) cross(p [3]float64, col color.RGBA, label string) {
	x, y := c.px(p)
	for k := -9; k <= 9; k++ {
		if abs(k) > 2 {
			c.set(x+k, y, col)
			c.set(x, y+k, col)
		}
	}
	for a := 0; a < 48; a++ {
		t := float64(a) / 48 * 2 * math.Pi
		c.set(x+int(math.Round(6*math.Cos(t))), y+int(math.Round(6*math.Sin(t))), col)
	}
	if label != "" {
		c.text(x+10, y-12, strings.ToUpper(label), col)
	}
}
