package prefabs

import (
	"math"
	"strings"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// Monument is a statue on a plinth: its boxes, the plaque's lettering and
// the spot in front of it where a voice line plays (content.Map.VoiceSpots).
type Monument struct {
	Boxes  []c.Box
	Signs  []c.Sign
	Lights []c.Light
	Spot   c.Zone
}

// facing maps a prop's local frame (s: to its left as you face it, h: up,
// f: towards the viewer it faces) to world boxes, for a prop at (x, y, z)
// facing face (content.SignPX ...: the side it is seen from).
type facing struct {
	x, y, z float64
	face    string
}

func (fr facing) box(s0, h0, f0, s1, h1, f1 float64, mat string) c.Box {
	var x0, z0, x1, z1 float64
	switch fr.face {
	case c.SignNX:
		x0, x1, z0, z1 = fr.x-f1, fr.x-f0, fr.z+s0, fr.z+s1
	case c.SignPZ:
		x0, x1, z0, z1 = fr.x+s0, fr.x+s1, fr.z+f0, fr.z+f1
	case c.SignNZ:
		x0, x1, z0, z1 = fr.x-s1, fr.x-s0, fr.z-f1, fr.z-f0
	default: // c.SignPX
		x0, x1, z0, z1 = fr.x+f0, fr.x+f1, fr.z-s1, fr.z-s0
	}
	return c.B(x0, fr.y+h0, z0, x1, fr.y+h1, z1, mat)
}

func (fr facing) at(s, h, f float64) [3]float64 {
	b := fr.box(s, h, f, s, h, f, "")
	return b.Min
}

// Facepalm is the figure alone: a weary engineer in a hoodie and jeans, one
// palm flat over the eyes, the other arm hanging with a coffee mug, the head
// bowed; height h (a person's 1.8 m scaled), standing at (x, y, z), facing
// face. All one material (a statue: c.Bronze).
func Facepalm(x, y, z, h float64, face, mat string) []c.Box {
	fr, k := facing{x, y, z, face}, h/1.8
	var out []c.Box
	// add takes a person's metres: s across (+ its left), up, f forward.
	add := func(s0, h0, f0, s1, h1, f1 float64) {
		out = append(out, fr.box(k*s0, k*h0, k*f0, k*s1, k*h1, k*f1, mat))
	}
	// Shoes and jeans (a slouch: weight on one leg, that foot turned out).
	add(-0.2, 0, -0.12, -0.06, 0.1, 0.17)
	add(0.05, 0, -0.1, 0.19, 0.1, 0.2)
	add(-0.19, 0.1, -0.09, -0.04, 0.88, 0.08)
	add(0.04, 0.1, -0.07, 0.19, 0.88, 0.1)
	// The hoodie, slumped: the body, a hunch over the upper back (shoulders
	// rolled forward), the kangaroo pocket, the cuffed hem, the hood bunched
	// behind the neck, its two drawstrings.
	add(-0.25, 0.86, -0.13, 0.25, 1.38, 0.13)
	add(-0.235, 1.2, -0.17, 0.235, 1.42, 0.06)
	add(-0.26, 0.84, -0.14, 0.26, 0.92, 0.14)
	add(-0.15, 0.94, 0.13, 0.15, 1.1, 0.16)
	add(-0.18, 1.32, -0.21, 0.18, 1.47, -0.08)
	add(-0.07, 1.16, 0.13, -0.05, 1.34, 0.15)
	add(0.05, 1.18, 0.13, 0.07, 1.34, 0.15)
	// A lanyard's badge (no ticket: it is the Monument to the Postmortem).
	add(-0.16, 1.08, 0.135, -0.085, 1.18, 0.155)
	// The head bowed deep, forward of the shoulders, into the hand; the hair
	// a mess.
	add(-0.06, 1.365, 0.02, 0.06, 1.43, 0.1)
	add(-0.11, 1.36, 0.03, 0.11, 1.6, 0.25)
	add(-0.12, 1.53, 0.01, 0.12, 1.645, 0.21)
	add(-0.05, 1.635, 0.05, 0.03, 1.68, 0.14)
	add(-0.13, 1.44, 0.06, -0.11, 1.53, 0.14) // ears
	add(0.11, 1.44, 0.06, 0.13, 1.53, 0.14)
	// The facepalm arm (its right, s < 0): the shoulder rolled forward, the
	// upper arm to the elbow held out low in front, the forearm rising across
	// the chest in two steps, a big palm flat over the eyes, the fingers
	// splayed over the brow and the hair, the thumb down the cheek.
	add(-0.32, 1.22, -0.06, -0.215, 1.36, 0.09)
	add(-0.315, 1.0, 0.0, -0.205, 1.24, 0.16)
	add(-0.3, 0.985, 0.15, -0.15, 1.12, 0.34) // the elbow
	add(-0.24, 1.1, 0.24, -0.11, 1.3, 0.37)
	add(-0.145, 1.28, 0.26, -0.035, 1.46, 0.36)
	add(-0.15, 1.42, 0.255, 0.13, 1.585, 0.325) // the palm, wider than the face
	add(-0.16, 1.57, 0.25, -0.115, 1.64, 0.31)  // the fingers, splayed
	add(-0.095, 1.58, 0.25, -0.05, 1.7, 0.31)
	add(-0.03, 1.58, 0.25, 0.015, 1.715, 0.31)
	add(0.035, 1.58, 0.25, 0.08, 1.705, 0.31)
	add(0.1, 1.575, 0.25, 0.145, 1.66, 0.31)
	add(0.12, 1.33, 0.24, 0.17, 1.44, 0.3) // the thumb, down the cheek
	// The other arm hangs limp from a dropped shoulder, the coffee mug in
	// its hand.
	add(0.215, 1.205, -0.06, 0.325, 1.34, 0.08)
	add(0.225, 0.8, -0.05, 0.315, 1.22, 0.07)
	add(0.235, 0.7, -0.04, 0.305, 0.8, 0.06)
	add(0.24, 0.58, 0.03, 0.355, 0.74, 0.15)  // the mug
	add(0.275, 0.61, 0.15, 0.315, 0.71, 0.18) // its handle
	return out
}

// FacepalmMonument is the Facepalm figure, h tall, on a plinth (plinthH high,
// 0.72 h square) at (x, y, z) facing face, a bronze plaque on the plinth's
// front with plaque in lit letters, two spotlights on it from lightY up in front
// (a monument is lit) and the spot in front of the plinth where the statue
// sighs (Map.VoiceSpots, the line "sigh").
func FacepalmMonument(x, y, z, h, plinthH, lightY float64, face, plaque string) Monument {
	fr := facing{x, y, z, face}
	w := h * 0.36 // the plinth's half width: the figure's stance with room
	m := Monument{}
	m.Boxes = append(m.Boxes,
		fr.box(-w-0.2, 0, -w-0.2, w+0.2, 0.3, w+0.2, c.Concrete), // the step
		fr.box(-w, 0.3, -w, w, plinthH, w, c.Concrete),
		fr.box(-w-0.15, plinthH, -w-0.15, w+0.15, plinthH+0.25, w+0.15, c.Concrete), // the cap
		fr.box(-w*0.6, plinthH*0.25, w, w*0.6, plinthH*0.8, w+0.06, c.Bronze),       // the plaque
	)
	m.Boxes = append(m.Boxes, Facepalm(x, y+plinthH+0.25, z, h, face, c.Bronze)...)
	long := 1
	for _, l := range strings.Split(plaque, "\n") {
		long = max(long, len([]rune(l)))
	}
	size := min(0.3, 0.9*2*w*0.6/(float64(long)*6/7))                                                                             // the longest line fills nine tenths of the plaque
	m.Signs = append(m.Signs, c.Sign{Text: plaque, Pos: fr.at(0, plinthH*0.525, w+0.06), Face: face, Size: size, Mat: c.SignLed}) // lit letters: readable in the hall's gloom
	spot := func(from, to [3]float64, power, cone float64) {
		d := [3]float64{to[0] - from[0], to[1] - from[1], to[2] - from[2]}
		n := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
		// Lights fall off as w²/(1+d²) (w = 1 - d²/range²; gen/glsl/lights.js):
		// with the range 1.6 times the distance, power is what reaches the aim.
		rng := 1.6 * n
		w := 1 - (n*n)/(rng*rng)
		l := c.Spot(from, [3]float64{d[0] / n, d[1] / n, d[2] / n}, [3]float64{1, 0.86, 0.66}, power*(1+n*n)/(w*w), rng, cone)
		l.Inner = cone * 0.6
		m.Lights = append(m.Lights, l)
	}
	for _, side := range []float64{-1, 1} { // the figure, from both front corners
		spot(fr.at(side*w*1.6, lightY-y, w+h*0.55), fr.at(0, plinthH+0.55*h, 0), 0.75, 36)
	}
	sb := fr.box(-w-1, 0, w+0.2, w+1, 2.5, w+0.2+3.5, "")
	m.Spot = c.Zone{Name: "sigh", Min: sb.Min, Max: sb.Max}
	return m
}

// CrestBanner is a banner on a pole: Earth's assault crest (content.Crest:
// the facepalm) w wide and bh tall, its top at height top, the pole planted
// at (x, y, z), the cloth facing face (both sides show the crest).
func CrestBanner(x, y, z, w, bh, top float64, face string) []c.Box {
	fr := facing{x, y, z, face}
	return []c.Box{
		fr.box(-0.08, 0.15, -0.08, 0.08, top+0.3, 0.08, c.Metal),      // the pole, on its foot
		fr.box(-0.1, top-0.08, -0.05, w+0.1, top+0.02, 0.05, c.Metal), // the crossbar
		fr.box(0.1, top-bh, -0.03, w+0.1, top-0.08, 0.03, c.Crest),    // the cloth
		fr.box(-0.4, 0, -0.4, 0.4, 0.15, 0.4, c.Metal),                // the foot
	}
}

// Place adds the monument to the map being built: its boxes, its lettering
// and its voice spot.
func (m Monument) Place(b *mapgen.Builder) {
	b.Add(m.Boxes...)
	for _, s := range m.Signs {
		b.Sign(s)
	}
	for _, l := range m.Lights {
		b.Light(l)
	}
	b.VoiceSpot(m.Spot.Name, m.Spot.Min[0], m.Spot.Min[1], m.Spot.Min[2], m.Spot.Max[0], m.Spot.Max[1], m.Spot.Max[2])
}
