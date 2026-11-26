package sim

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gigurra/counterslop/internal/content/maps"
)

func harborBoat() (*World, *Boat) {
	m := maps.LabHarbor()
	sp := m.Boats[0]
	return NewWorld(&m), &Boat{Pos: sp.Pos, Yaw: sp.Yaw}
}

func boatSteps(w *World, b *Boat, in BoatInput, n int) {
	for range n {
		w.StepBoat(b, in, 0.01)
	}
}

// Full ahead it gets up to speed along its heading, on the surface; let go,
// the water stops it.
func TestBoatThrustAndDrift(t *testing.T) {
	w, b := harborBoat()
	b.Yaw = -math.Pi / 2 // bows to +x, south of the pier
	b.Pos = Vec3{-25, -0.6, -20}
	boatSteps(w, b, BoatInput{F: 1}, 300)
	if b.Vel[0] < 9 || math.Abs(b.Vel[2]) > 1e-9 || b.Pos[1] != -0.6 {
		t.Fatalf("full ahead: vel %v pos %v", b.Vel, b.Pos)
	}
	boatSteps(w, b, BoatInput{}, 1500)
	if sp := math.Hypot(b.Vel[0], b.Vel[2]); sp > 0.1 {
		t.Fatalf("still going at %v", sp)
	}
}

// The tiller turns it right (R > 0): its yaw falls, and it carves a circle
// instead of skidding (little sideways speed).
func TestBoatTurns(t *testing.T) {
	w, b := harborBoat()
	b.Pos = Vec3{0, -0.6, -15}
	b.Yaw = 0
	boatSteps(w, b, BoatInput{F: 1}, 100)
	boatSteps(w, b, BoatInput{F: 1, R: 1}, 60)
	if b.Yaw > -0.5 {
		t.Fatalf("tiller right: yaw %v", b.Yaw)
	}
	hx, hz := BoatHeading(b.Yaw)
	if side := b.Vel[0]*hz - b.Vel[2]*hx; math.Abs(side) > 1.5 {
		t.Fatalf("skidding: %v sideways", side)
	}
}

// Driven at the ship's hull it comes to rest against it (bow disc touching),
// and stays there under thrust; at an angle it slides along it.
func TestBoatRestsAgainstHull(t *testing.T) {
	w, b := harborBoat()
	b.Pos = Vec3{0, -0.6, -15}
	b.Yaw = 0 // bows to -z: the hull at z -30
	boatSteps(w, b, BoatInput{F: 1}, 600)
	want := -30 + BoatDisc + BoatDiscOff
	if math.Abs(b.Pos[2]-want) > 1e-6 || math.Abs(b.Vel[2]) > 1e-9 || math.Abs(b.Pos[0]) > 1e-9 {
		t.Fatalf("at the hull: pos %v vel %v, want z %v", b.Pos, b.Vel, want)
	}
	b.Yaw = 0.4 // angled: slides along it towards -x
	x0 := b.Pos[0]
	boatSteps(w, b, BoatInput{F: 1}, 100)
	if b.Pos[0] >= x0-0.5 || b.Pos[2] < -30+BoatDisc-1e-9 {
		t.Fatalf("not sliding along: %v", b.Pos)
	}
}

// The pier stops it too (a box from the floor up through the surface).
func TestBoatHitsPier(t *testing.T) {
	w, b := harborBoat()
	b.Pos = Vec3{-10, -0.6, 0}
	b.Yaw = -math.Pi / 2 // bows to +x: the pier at x -2..2
	boatSteps(w, b, BoatInput{F: 1}, 400)
	if b.Pos[0] > -2-BoatDisc+1e-9 {
		t.Fatalf("through the pier: %v", b.Pos)
	}
}

// Go and the generated JS step boats bit for bit (seeded tiller and
// throttle runs around lab_harbor, into its walls).
func TestBoatParityWithJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	m := maps.LabHarbor()
	r := rand.New(rand.NewPCG(7, 9))
	type step struct{ F, R float64 }
	type brun struct {
		Boat  Boat   `json:"boat"`
		Steps []step `json:"steps"`
	}
	var runs []brun
	for i := range 6 {
		rn := brun{Boat: Boat{Pos: Vec3{-20 + 8*float64(i), -0.6, -20 + 4*float64(i%3)}, Yaw: r.Float64() * 6.28}}
		var cur step
		for k := range 1500 {
			if k%60 == 0 {
				cur = step{F: float64(r.IntN(3) - 1), R: float64(r.IntN(3)-1) * r.Float64()}
			}
			rn.Steps = append(rn.Steps, cur)
		}
		runs = append(runs, rn)
	}
	c, _ := json.Marshal(map[string]any{"map": m, "runs": runs})
	f := filepath.Join(t.TempDir(), "boats.json")
	_ = os.WriteFile(f, c, 0o644)
	cmd := exec.Command(node, "testdata/boatparity.mjs", f)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var js [][][7]float64
	if err := json.Unmarshal(out, &js); err != nil {
		t.Fatal(err)
	}
	w := NewWorld(&m)
	hit := 0
	for ri, rn := range runs {
		b := rn.Boat
		for k, s := range rn.Steps {
			v0 := math.Hypot(b.Vel[0], b.Vel[2])
			w.StepBoat(&b, BoatInput{F: s.F, R: s.R}, 0.01)
			if v0 > 1 && math.Hypot(b.Vel[0], b.Vel[2]) < v0*0.8 {
				hit++
			}
			g := [7]float64{b.Pos[0], b.Pos[1], b.Pos[2], b.Vel[0], b.Vel[2], b.Yaw, b.YawVel}
			if g != js[ri][k] {
				t.Fatalf("run %d step %d: go %v, js %v", ri, k, g, js[ri][k])
			}
		}
	}
	if hit < 5 {
		t.Fatalf("weak coverage: %d hits on walls", hit)
	}
}

// A boat put with a disc's centre inside a box (a teleport, a spawn on the
// pier) is pushed out of the box's nearest side, clear of it.
func TestBoatOutOfABox(t *testing.T) {
	w, b := harborBoat()
	b.Pos = Vec3{1.5, -0.6, 0} // the pier is x -2..2: both discs' centres in it, nearest its +x side
	b.Yaw = 0                  // along z
	b.Vel = Vec3{-1, 0, 0}
	w.StepBoat(b, BoatInput{}, 0.01)
	if b.Pos[0] < 2+BoatDisc-1e-9 || b.Vel[0] < 0 {
		t.Fatalf("still in the pier: pos %v vel %v", b.Pos, b.Vel)
	}
}

// The deck's frame: ToDeck and DeckWorld undo each other, x is starboard and
// z aft, and the hull's outline is the two discs' stadium.
func TestDeckFrame(t *testing.T) {
	b := &Boat{Pos: Vec3{5, -0.6, 3}, Yaw: -math.Pi / 2} // bow to +x: starboard is +z, aft -x
	if p := DeckWorld(b, Vec3{1, 0.1, 2}); math.Abs(p[0]-3) > 1e-12 || math.Abs(p[1]+0.5) > 1e-12 || math.Abs(p[2]-4) > 1e-12 {
		t.Fatalf("deck (1, 0.1, 2) at %v", p)
	}
	for _, l := range []Vec3{{0.3, 0.1, -1.2}, {-0.7, 2, 0.4}} {
		if g := ToDeck(b, DeckWorld(b, l)); math.Abs(g[0]-l[0]) > 1e-12 || math.Abs(g[1]-l[1]) > 1e-12 || math.Abs(g[2]-l[2]) > 1e-12 {
			t.Fatalf("round trip %v -> %v", l, g)
		}
	}
	b.Vel = Vec3{2, 0, 0}
	if v := DeckVel(b, Vec3{1, 0, 0}); math.Abs(v[0]-2) > 1e-12 || math.Abs(v[2]-1) > 1e-12 {
		t.Fatalf("walking to starboard on a boat going ahead: %v", v)
	}
	for _, c := range []struct {
		x, z float64
		on   bool
	}{{0, 0, true}, {0.8, 0, true}, {0.9, 0, false}, {0, 1.75, true}, {0, -1.85, false}, {0.7, 1.6, false}} {
		if OnHull(c.x, c.z) != c.on {
			t.Fatalf("OnHull(%v, %v) = %v", c.x, c.z, !c.on)
		}
	}
}

// LandOnBoat: feet coming down onto the floor (or walking on from up to
// deckStep under it) over the hull land; rising, from deep in the water,
// beside the hull, flying or hooked don't.
func TestLandOnBoat(t *testing.T) {
	b := &Boat{Pos: Vec3{0, -0.6, 0}, Vel: Vec3{0, 0, -3}}
	floor := -0.6 + DeckTop
	for _, c := range []struct {
		name   string
		pos    Vec3
		y0, vy float64
		land   bool
	}{
		{"falling onto it", Vec3{0.2, floor - 0.05, 0.5}, floor + 0.1, -2, true},
		{"walking on from a pontoon", Vec3{0, floor - 0.2, 1.5}, floor - 0.2, 0, true},
		{"rising through it", Vec3{0, floor - 0.05, 0}, floor - 0.2, 3, false},
		{"swimming under it", Vec3{0, floor - 1.2, 0}, floor - 1.1, -0.5, false},
		{"beside the hull", Vec3{1, floor - 0.05, 0}, floor + 0.1, -2, false},
		{"over the bow", Vec3{0, floor - 0.05, -1.9}, floor + 0.1, -2, false},
	} {
		s := NewBody(c.pos)
		s.Vel = Vec3{1, c.vy, -3}
		var d Deck
		if got := LandOnBoat(b, s, c.y0, &d); got != c.land {
			t.Fatalf("%s: landed %v", c.name, got)
		}
		if c.land && (d.Pos[1] != DeckTop || d.Vel != (Vec3{1, 0, 0}) || !s.OnGround || s.Pos[1] != floor) {
			t.Fatalf("%s: deck %+v body %+v", c.name, d, s)
		}
	}
	s := NewBody(Vec3{0, floor - 0.05, 0})
	s.Noclip = true
	if LandOnBoat(b, s, floor+0.1, &Deck{}) {
		t.Fatal("a flying editor landed")
	}
	s.Noclip, s.Hook.On = false, true
	if LandOnBoat(b, s, floor+0.1, &Deck{}) {
		t.Fatal("hooked, landed")
	}
}

// StepDeck: walking is along the view relative to the boat (forward to the
// bow whatever the boat's heading), a jump lands on the floor again, and
// the body is where that is on the boat, moving with it.
func TestStepDeck(t *testing.T) {
	w, b := harborBoat()
	b.Yaw = 1.0
	b.Vel = Vec3{-4, 0, 1}
	s := NewBody(Vec3{})
	s.OnGround, s.Yaw = true, 1.0 // facing the bow
	d := Deck{Pos: Vec3{0, DeckTop, 1}}
	for range 20 {
		w.StepDeck(b, s, &d, Input{F: 1}, 0.01)
	}
	if d.Pos[2] > 0.2 || math.Abs(d.Pos[0]) > 1e-9 || d.Pos[1] != DeckTop || !s.OnGround {
		t.Fatalf("walked forward: deck %+v", d)
	}
	if p := DeckWorld(b, d.Pos); s.Pos != p || s.Vel != DeckVel(b, d.Vel) {
		t.Fatalf("body %v, deck at %v", s.Pos, p)
	}
	for range 10 { // stop
		w.StepDeck(b, s, &d, Input{}, 0.01)
	}
	w.StepDeck(b, s, &d, Input{Jump: true}, 0.01)
	if d.Vel[1] <= 0 || s.OnGround {
		t.Fatalf("no jump: %+v", d)
	}
	for range 80 {
		w.StepDeck(b, s, &d, Input{}, 0.01)
	}
	if d.Pos[1] != DeckTop || !s.OnGround {
		t.Fatalf("not back on the floor: %+v", d)
	}
	for range 40 {
		w.StepDeck(b, s, &d, Input{R: 1}, 0.01)
	}
	if OnHull(d.Pos[0], d.Pos[2]) {
		t.Fatalf("still on the hull after walking to starboard: %+v", d)
	}
}

// Go and the generated JS move bodies on decks bit for bit (seeded walking,
// jumping and crouching on boats that are driven about meanwhile).
func TestDeckParityWithJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	m := maps.LabHarbor()
	r := rand.New(rand.NewPCG(3, 5))
	type step struct {
		F, R, Yaw, BF, BR float64
		Jump, Crouch      bool
	}
	type drun struct {
		Boat  Boat   `json:"boat"`
		Deck  Deck   `json:"deck"`
		Steps []step `json:"steps"`
	}
	var runs []drun
	for i := range 6 {
		rn := drun{Boat: Boat{Pos: Vec3{-20 + 8*float64(i), -0.6, -20}, Yaw: r.Float64() * 6.28}, Deck: Deck{Pos: Vec3{0, DeckTop, 0}}}
		var cur step
		for k := range 800 {
			if k%25 == 0 {
				cur = step{F: float64(r.IntN(3)-1) * 0.4, R: float64(r.IntN(3)-1) * 0.3, Yaw: r.Float64() * 6.28, BF: float64(r.IntN(3) - 1), BR: float64(r.IntN(3) - 1), Crouch: r.IntN(4) == 0}
			}
			c := cur
			c.Jump = r.IntN(40) == 0
			rn.Steps = append(rn.Steps, c)
		}
		runs = append(runs, rn)
	}
	c, _ := json.Marshal(map[string]any{"map": m, "runs": runs})
	f := filepath.Join(t.TempDir(), "decks.json")
	_ = os.WriteFile(f, c, 0o644)
	cmd := exec.Command(node, "testdata/deckparity.mjs", f)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var js [][][11]float64
	if err := json.Unmarshal(out, &js); err != nil {
		t.Fatal(err)
	}
	w := NewWorld(&m)
	total, jumps := 0, 0
	for ri, rn := range runs {
		b, d, s := rn.Boat, rn.Deck, NewBody(Vec3{})
		s.OnGround = true
		for k, st := range rn.Steps {
			s.Yaw = st.Yaw
			ev := w.StepDeck(&b, s, &d, Input{F: st.F, R: st.R, Jump: st.Jump, Crouch: st.Crouch, Speed: 1}, 0.01)
			if !OnHull(d.Pos[0], d.Pos[2]) { // walked off: back to the middle (as the JS)
				d, s.OnGround = Deck{Pos: Vec3{0, DeckTop, 0}}, true
			}
			if ev.Jumped {
				jumps++
			}
			w.StepBoat(&b, BoatInput{F: st.BF, R: st.BR}, 0.01)
			g := [11]float64{d.Pos[0], d.Pos[1], d.Pos[2], d.Vel[0], d.Vel[1], d.Vel[2], s.Pos[0], s.Pos[1], s.Pos[2], b2f(s.OnGround), b2f(s.Crouched)}
			if g != js[ri][k] {
				t.Fatalf("run %d step %d: go %v, js %v", ri, k, g, js[ri][k])
			}
			total++
		}
	}
	if total < 4000 || jumps < 20 {
		t.Fatalf("weak coverage: %d steps, %d jumps", total, jumps)
	}
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
