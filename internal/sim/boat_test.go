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
