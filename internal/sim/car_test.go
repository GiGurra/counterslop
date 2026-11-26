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

// carSpecs is web/assets/cars.json by id.
func carSpecs(t *testing.T) map[string]*CarSpec {
	t.Helper()
	b, err := os.ReadFile("../../web/assets/cars.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Cars []*CarSpec }
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	out := map[string]*CarSpec{}
	for _, c := range f.Cars {
		out[c.ID] = c
	}
	return out
}

var carTypes = []string{"buggy", "racecar", "tank", "hovercraft"}

// kartCar is a car of type typ on lab_kart at pos facing yaw.
func kartCar(t *testing.T, typ string, pos Vec3, yaw float64) (*World, *CarSpec, *Car) {
	t.Helper()
	m := maps.LabKart()
	sp := carSpecs(t)[typ]
	if sp == nil {
		t.Fatalf("no car type %q", typ)
	}
	return NewWorld(&m), sp, NewCar(sp, pos, yaw)
}

func carSteps(w *World, sp *CarSpec, c *Car, in CarInput, n int) {
	for range n {
		w.StepCar(sp, c, in, 0.01)
	}
}

const east = -math.Pi / 2 // a heading facing +x

// The tiers: top speed rises strictly with the tier, in small steps (it is a
// race: the dearer car is a little faster, not a lot).
func TestCarTiers(t *testing.T) {
	specs := carSpecs(t)
	if len(specs) != 4 {
		t.Fatalf("%d car types", len(specs))
	}
	for i, id := range carTypes {
		sp := specs[id]
		if sp == nil || sp.Tier != i+1 {
			t.Fatalf("%s: %+v", id, sp)
		}
		if sp.Front == "" || sp.Rear == "" || sp.HP <= 0 || sp.Drive == "" {
			t.Fatalf("%s: no built-in front weapon, rear item, HP or drive", id)
		}
		if i == 0 {
			continue
		}
		prev := specs[carTypes[i-1]]
		if d := sp.TopSpeed - prev.TopSpeed; d <= 0 || d > 2 {
			t.Fatalf("%s top speed %v after %s's %v: want 0 < step <= 2 m/s", id, sp.TopSpeed, prev.ID, prev.TopSpeed)
		}
		if sp.Price <= prev.Price {
			t.Fatalf("%s costs %d, %s %d", id, sp.Price, prev.ID, prev.Price)
		}
	}
	if specs["tank"].HP <= specs["buggy"].HP || specs["tank"].Drive != DriveTracks || specs["hovercraft"].Drive != DriveHover {
		t.Fatal("the tank has the most HP and tracks, the hovercraft hovers")
	}
}

// Full throttle on the flat, every type gets to its top speed along its
// heading and holds it there; let go, it rolls to a stop.
func TestCarTopSpeed(t *testing.T) {
	for _, typ := range carTypes {
		w, sp, c := kartCar(t, typ, Vec3{-80, 0, 22}, east)
		carSteps(w, sp, c, CarInput{Throttle: 1}, 400)
		f := CarForward(c)
		if f < sp.TopSpeed-0.1 || f > sp.TopSpeed+1e-9 || math.Abs(c.Vel[2]) > 1e-6 || c.Pos[1] != 0 || !c.OnGround {
			t.Fatalf("%s: %v m/s (top %v), vel %v pos %v", typ, f, sp.TopSpeed, c.Vel, c.Pos)
		}
		at := c.Pos[0]
		carSteps(w, sp, c, CarInput{}, 3000)
		if CarSpeed(c) > 0.01 {
			t.Fatalf("%s still rolling at %v", typ, CarSpeed(c))
		}
		if typ != "hovercraft" && c.Pos[0]-at > 200 {
			t.Fatalf("%s coasted %v m", typ, c.Pos[0]-at)
		}
	}
}

// Steering right turns it right (yaw falls) and it carves; standing still
// the wheels don't turn it, the tank's tracks pivot it on the spot.
func TestCarTurning(t *testing.T) {
	w, sp, c := kartCar(t, "buggy", Vec3{-80, 0, 24}, east)
	carSteps(w, sp, c, CarInput{Throttle: 1}, 100)
	carSteps(w, sp, c, CarInput{Throttle: 1, Steer: 1}, 50)
	if c.Yaw > east-0.5 {
		t.Fatalf("steering right: yaw %v", c.Yaw)
	}
	hx, hz := BoatHeading(c.Yaw)
	if lat := c.Vel[0]*-hz + c.Vel[2]*hx; math.Abs(lat) > 2 {
		t.Fatalf("skidding without the handbrake: %v sideways", lat)
	}
	for _, typ := range []string{"buggy", "racecar"} {
		w, sp, c := kartCar(t, typ, Vec3{-80, 0, 24}, east)
		carSteps(w, sp, c, CarInput{Steer: 1}, 100)
		if c.Yaw != east {
			t.Fatalf("%s turned standing: %v", typ, c.Yaw)
		}
	}
	w, sp, c = kartCar(t, "tank", Vec3{-80, 0, 24}, east)
	carSteps(w, sp, c, CarInput{Steer: -1}, 100) // left: yaw rises
	if c.Yaw < east+1 || CarSpeed(c) > 1e-9 || math.Abs(c.Pos[0]+80) > 1e-9 {
		t.Fatalf("tank pivot: yaw %v speed %v pos %v", c.Yaw, CarSpeed(c), c.Pos)
	}
}

// The handbrake in a turn at speed swaps grip for a slide: far more sideways
// speed than the same turn without it, and it swings round further.
func TestCarHandbrakeSlide(t *testing.T) {
	lat := func(hb bool) (float64, float64) {
		w, sp, c := kartCar(t, "buggy", Vec3{-85, 0, 24}, east)
		carSteps(w, sp, c, CarInput{Throttle: 1}, 150)
		carSteps(w, sp, c, CarInput{Throttle: 1, Steer: 1, Handbrake: hb}, 40)
		hx, hz := BoatHeading(c.Yaw)
		return math.Abs(c.Vel[0]*-hz + c.Vel[2]*hx), c.Yaw
	}
	grip, y0 := lat(false)
	slide, y1 := lat(true)
	if slide < 3*grip || slide < 3 || y1 > y0 {
		t.Fatalf("sideways %v gripping, %v sliding (yaw %v vs %v)", grip, slide, y0, y1)
	}
}

// Across the basin the hovercraft floats on the water at speed; a buggy goes
// in and bogs down.
func TestCarWater(t *testing.T) {
	w, sp, c := kartCar(t, "hovercraft", Vec3{40, 0, -35}, math.Pi/2) // facing -x, along the south straight
	minY, minV := 0.0, 1e9
	for range 500 {
		w.StepCar(sp, c, CarInput{Throttle: 1}, 0.01)
		if c.Pos[0] > -10 && c.Pos[0] < 10 {
			minY, minV = math.Min(minY, c.Pos[1]), math.Min(minV, CarSpeed(c))
		}
	}
	if c.Pos[0] > -20 || minY < -0.4-1e-9 || minV < 15 {
		t.Fatalf("hovercraft over the water: at %v, lowest %v, slowest %v m/s", c.Pos, minY, minV)
	}
	w, sp, c = kartCar(t, "buggy", Vec3{40, 0, -35}, math.Pi/2)
	carSteps(w, sp, c, CarInput{Throttle: 1}, 300)
	if !c.Wet || c.Pos[1] > -1 || CarSpeed(c) > 5 || c.Pos[0] < -14 {
		t.Fatalf("buggy in the water: wet %v pos %v speed %v", c.Wet, c.Pos, CarSpeed(c))
	}
}

// Up the jump at speed and off its 2.2 m high end: it flies (higher than
// the ramp's end, nose up on the way), and lands well past it.
func TestCarRampJump(t *testing.T) {
	w, sp, c := kartCar(t, "buggy", Vec3{-60, 0, 38}, east)
	top, air, pitch := 0.0, 0, 0.0
	landed := 0.0
	for range 500 {
		was := c.OnGround
		w.StepCar(sp, c, CarInput{Throttle: 1}, 0.01)
		top = math.Max(top, c.Pos[1])
		pitch = math.Max(pitch, c.Pitch)
		if !c.OnGround {
			air++
		}
		if !was && c.OnGround && landed == 0 && c.Pos[0] > -3 {
			landed = c.Pos[0]
		}
	}
	if top < 2.3 || air < 30 || landed < -2 || pitch < 0.1 {
		t.Fatalf("jump: top %v, %d steps in the air, landed at x %v, pitch up to %v", top, air, landed, pitch)
	}
}

// The east end's bank tilts the car: heading -z round it (the island on its
// left), the outside on its right is higher: it leans into the turn.
func TestCarBankedTurn(t *testing.T) {
	w, sp, c := kartCar(t, "racecar", Vec3{80, 1.78, 30}, 0) // facing -z on the bank (x 72..90 rises 4 m)
	c.Pos[1] = 4 * (80.0 - 72) / 18
	carSteps(w, sp, c, CarInput{Throttle: 1}, 30)
	if c.Roll > -0.1 || !c.OnGround || c.Contact != 15 {
		t.Fatalf("on the bank: roll %v ground %v contact %b pos %v", c.Roll, c.OnGround, c.Contact, c.Pos)
	}
}

// Driven at the island's wall it stops against it, not through it; at an
// angle it scrapes along it a little (the tyres grip: it doesn't slide far).
func TestCarWallHit(t *testing.T) {
	w, sp, c := kartCar(t, "tank", Vec3{0, 0, 30}, 0) // facing -z: the island's north face is z 18
	carSteps(w, sp, c, CarInput{Throttle: 1}, 300)
	if c.Pos[2] < 18+sp.Size[0]/2-1e-6 || CarSpeed(c) > 0.5 {
		t.Fatalf("through the wall: pos %v speed %v", c.Pos, CarSpeed(c))
	}
	w, sp, c = kartCar(t, "buggy", Vec3{0, 0, 30}, 0.4)
	carSteps(w, sp, c, CarInput{Throttle: 1}, 300)
	if c.Pos[2] < 18+sp.Size[0]/2-1e-6 || c.Pos[0] > -3 {
		t.Fatalf("not sliding along: pos %v", c.Pos)
	}
}

// Off the map's edge (nothing under it) it falls.
func TestCarFallsOffTheEdge(t *testing.T) {
	w, sp, c := kartCar(t, "buggy", Vec3{100, 0, 0}, east)
	carSteps(w, sp, c, CarInput{}, 100)
	if c.OnGround || c.Pos[1] > -5 {
		t.Fatalf("over nothing: %v", c.Pos)
	}
}

// Another car is solid: driven at one it stops against it.
func TestCarHitsCar(t *testing.T) {
	w, sp, c := kartCar(t, "buggy", Vec3{-80, 0, 24}, east)
	other := NewCar(sp, Vec3{-60, 0, 24}, 0) // across its path
	w.Cars = CarDiscs(sp, other, nil)
	carSteps(w, sp, c, CarInput{Throttle: 1}, 300)
	if c.Pos[0] > -60-sp.Size[0]+1e-6 {
		t.Fatalf("through the other car: %v", c.Pos)
	}
}

// Go and the generated JS step cars bit for bit: seeded throttle, steering
// and handbrake runs of every type around lab_kart (walls, the jump, the
// bank, the water, another car).
func TestCarParityWithJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	m := maps.LabKart()
	specs := carSpecs(t)
	r := rand.New(rand.NewPCG(11, 13))
	type step struct {
		T, S float64
		H    bool
	}
	type crun struct {
		Type  string   `json:"type"`
		Pos   Vec3     `json:"pos"`
		Yaw   float64  `json:"yaw"`
		Other *CarDisc `json:"other"`
		Steps []step   `json:"steps"`
	}
	var runs []crun
	for i := range 12 {
		rn := crun{Type: carTypes[i%4], Pos: Vec3{-80 + 14*float64(i), 0, -40 + 7*float64(i)}, Yaw: r.Float64() * 6.28}
		if i%3 == 0 {
			rn.Other = &CarDisc{X: rn.Pos[0] + 8, Z: rn.Pos[2], R: 1, Y0: 0, Y1: 1.5}
		}
		var cur step
		for k := range 2000 {
			if k%70 == 0 {
				cur = step{T: float64(r.IntN(3) - 1), S: float64(r.IntN(3)-1) * r.Float64(), H: r.IntN(5) == 0}
				if k < 300 {
					cur.T = 1
				}
			}
			rn.Steps = append(rn.Steps, cur)
		}
		runs = append(runs, rn)
	}
	sm := map[string]*CarSpec{}
	for k, v := range specs {
		sm[k] = v
	}
	cj, _ := json.Marshal(map[string]any{"map": m, "runs": runs, "specs": sm})
	f := filepath.Join(t.TempDir(), "cars.json")
	_ = os.WriteFile(f, cj, 0o644)
	cmd := exec.Command(node, "testdata/carparity.mjs", f)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var js [][][16]float64
	if err := json.Unmarshal(out, &js); err != nil {
		t.Fatal(err)
	}
	w := NewWorld(&m)
	air, wet, hits := 0, 0, 0
	for ri, rn := range runs {
		sp := specs[rn.Type]
		c := NewCar(sp, rn.Pos, rn.Yaw)
		w.Cars = nil
		if rn.Other != nil {
			w.Cars = []CarDisc{*rn.Other}
		}
		for k, s := range rn.Steps {
			v0 := CarSpeed(c)
			w.StepCar(sp, c, CarInput{Throttle: s.T, Steer: s.S, Handbrake: s.H}, 0.01)
			if v0 > 3 && CarSpeed(c) < v0*0.8 {
				hits++
			}
			b2f := func(b bool) float64 {
				if b {
					return 1
				}
				return 0
			}
			if !c.OnGround {
				air++
			}
			if c.Wet {
				wet++
			}
			g := [16]float64{c.Pos[0], c.Pos[1], c.Pos[2], c.Vel[0], c.Vel[1], c.Vel[2], c.Yaw, c.YawVel, c.Pitch, c.Roll, c.Steer, float64(c.Contact), c.Comp[0], c.Comp[3], b2f(c.OnGround), b2f(c.Wet)}
			if g != js[ri][k] {
				t.Fatalf("run %d (%s) step %d: go %v, js %v", ri, rn.Type, k, g, js[ri][k])
			}
		}
	}
	t.Logf("coverage: %d steps in the air, %d wet, %d hits", air, wet, hits)
	if hits < 5 || air < 20 {
		t.Fatalf("weak coverage: %d hits, %d steps in the air", hits, air)
	}
}
