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
		H, B bool
	}
	type crun struct {
		Type   string   `json:"type"`
		Pos    Vec3     `json:"pos"`
		Yaw    float64  `json:"yaw"`
		Other  *CarDisc `json:"other"`
		Boost  float64  `json:"boost"`
		Slip   float64  `json:"slip"`
		Engine int      `json:"engine"`
		Tyres  int      `json:"tyres"`
		Steps  []step   `json:"steps"`
	}
	var runs []crun
	for i := range 12 {
		rn := crun{Type: carTypes[i%4], Pos: Vec3{-80 + 14*float64(i), 0, -40 + 7*float64(i)}, Yaw: r.Float64() * 6.28, Boost: float64(i%3) * 2, Slip: float64(i % 2 * 3), Engine: i % 4, Tyres: (i / 4) % 4}
		if i%3 == 0 {
			rn.Other = &CarDisc{X: rn.Pos[0] + 8, Z: rn.Pos[2], R: 1, Y0: 0, Y1: 1.5}
		}
		var cur step
		for k := range 2000 {
			if k%70 == 0 {
				cur = step{T: float64(r.IntN(3) - 1), S: float64(r.IntN(3)-1) * r.Float64(), H: r.IntN(5) == 0, B: r.IntN(3) == 0}
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
	var js [][][18]float64
	if err := json.Unmarshal(out, &js); err != nil {
		t.Fatal(err)
	}
	w := NewWorld(&m)
	air, wet, hits := 0, 0, 0
	for ri, rn := range runs {
		sp := specs[rn.Type]
		c := NewCar(sp, rn.Pos, rn.Yaw)
		c.Boost, c.Slip, c.Engine, c.Tyres = rn.Boost, rn.Slip, rn.Engine, rn.Tyres
		w.Cars = nil
		if rn.Other != nil {
			w.Cars = []CarDisc{*rn.Other}
		}
		for k, s := range rn.Steps {
			v0 := CarSpeed(c)
			w.StepCar(sp, c, CarInput{Throttle: s.T, Steer: s.S, Handbrake: s.H, Boost: s.B}, 0.01)
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
			g := [18]float64{c.Pos[0], c.Pos[1], c.Pos[2], c.Vel[0], c.Vel[1], c.Vel[2], c.Yaw, c.YawVel, c.Pitch, c.Roll, c.Steer, float64(c.Contact), c.Comp[0], c.Comp[3], b2f(c.OnGround), b2f(c.Wet), c.Boost, c.Slip}
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

// Boost: full throttle, more acceleration and CarBoostTop more top speed
// while the tank lasts, draining a second a second; an empty tank does
// nothing.
func TestCarBoost(t *testing.T) {
	for _, typ := range carTypes {
		w, sp, c := kartCar(t, typ, Vec3{-80, 0, 22}, east)
		c.Boost = 1.05
		carSteps(w, sp, c, CarInput{Boost: true}, 110) // no throttle held: the boost is full throttle
		if c.Boost != 0 {
			t.Fatalf("%s: tank %v left after 1.1 s of a 1.05 s boost", typ, c.Boost)
		}
		_, _, plain := kartCar(t, typ, Vec3{-80, 0, 22}, east)
		carSteps(w, sp, plain, CarInput{Throttle: 1}, 110)
		if CarForward(c) < CarForward(plain)*1.3 {
			t.Fatalf("%s: boosted %v, plain %v", typ, CarForward(c), CarForward(plain))
		}
		_, _, empty := kartCar(t, typ, Vec3{-80, 0, 22}, east)
		carSteps(w, sp, empty, CarInput{Throttle: 1, Boost: true}, 110)
		if CarForward(empty) != CarForward(plain) {
			t.Fatalf("%s: an empty tank boosts: %v vs %v", typ, CarForward(empty), CarForward(plain))
		}
		_, _, c = kartCar(t, typ, Vec3{-85, 0, 22}, east)
		c.Boost = 10
		most := 0.0
		for range 400 {
			w.StepCar(sp, c, CarInput{Boost: true}, 0.01)
			most = math.Max(most, CarForward(c))
		}
		if most < sp.TopSpeed*1.1 || most > sp.TopSpeed*CarBoostTop+1e-9 {
			t.Fatalf("%s: up to %v m/s boosting (top %v, boosted %v)", typ, most, sp.TopSpeed, sp.TopSpeed*CarBoostTop)
		}
		c.Boost = 0
		v0 := CarForward(c)
		carSteps(w, sp, c, CarInput{Throttle: 1}, 50) // spent: the engine holds it back towards its top speed
		if f := CarForward(c); v0 > sp.TopSpeed+0.5 && f >= v0 {
			t.Fatalf("%s: %v m/s after the boost, %v when it ran out (top %v)", typ, f, v0, sp.TopSpeed)
		}
	}
}

// Slip (spikes, oil): the tyres lose their grip and the steering wobbles: a
// slipping car in a turn slides far more sideways than a gripping one and
// strays off a straight line; it wears off. The hovercraft never slips.
func TestCarSlip(t *testing.T) {
	run := func(typ string, slip float64, steer float64) *Car {
		w, sp, c := kartCar(t, typ, Vec3{-85, 0, 24}, east)
		carSteps(w, sp, c, CarInput{Throttle: 1}, 150)
		c.Slip = slip
		carSteps(w, sp, c, CarInput{Throttle: 1, Steer: steer}, 40)
		return c
	}
	lat := func(c *Car) float64 {
		hx, hz := BoatHeading(c.Yaw)
		return math.Abs(c.Vel[0]*-hz + c.Vel[2]*hx)
	}
	for _, typ := range []string{"buggy", "racecar", "tank"} {
		grip, slip := run(typ, 0, 1), run(typ, 2, 1)
		if lat(slip) < 3*lat(grip)+1 {
			t.Fatalf("%s: sideways %v gripping, %v slipping", typ, lat(grip), lat(slip))
		}
		if st := run(typ, 2, 0); math.Abs(st.Yaw-east) < 0.02 {
			t.Fatalf("%s: slipping straight on, the steering didn't wobble (yaw %v)", typ, st.Yaw)
		}
		if c := run(typ, 0.2, 0); c.Slip != 0 {
			t.Fatalf("%s: slip left %v after 0.4 s (0.2 s of it)", typ, c.Slip)
		}
	}
	h0, h1 := run("hovercraft", 0, 1), run("hovercraft", 2, 1)
	if h0.Pos != h1.Pos || h0.Yaw != h1.Yaw {
		t.Fatalf("the hovercraft slipped: %v / %v vs %v / %v", h1.Pos, h1.Yaw, h0.Pos, h0.Yaw)
	}
}

// Upgrades: each engine tier adds acceleration and a little top speed; each
// tyre tier grip (less sideways in a turn) and turn rate, on wheels and
// tracks; the hovercraft has no tyres.
func TestCarUpgrades(t *testing.T) {
	for _, typ := range carTypes {
		w, sp, c := kartCar(t, typ, Vec3{-85, 0, 22}, east)
		c.Engine = 3
		_, _, stock := kartCar(t, typ, Vec3{-85, 0, 22}, east)
		carSteps(w, sp, c, CarInput{Throttle: 1}, 60)
		carSteps(w, sp, stock, CarInput{Throttle: 1}, 60)
		if CarForward(c) < CarForward(stock)*1.25 {
			t.Fatalf("%s engine 3: %v m/s, stock %v", typ, CarForward(c), CarForward(stock))
		}
		turn := func(tyres int) (float64, float64) {
			w, sp, c := kartCar(t, typ, Vec3{-85, 0, 24}, east)
			c.Tyres = tyres
			carSteps(w, sp, c, CarInput{Throttle: 1}, 150)
			carSteps(w, sp, c, CarInput{Throttle: 1, Steer: 1}, 30)
			return c.Yaw, c.Pos[2]
		}
		y0, z0 := turn(0)
		y3, z3 := turn(3)
		if typ == "hovercraft" {
			if y0 != y3 || z0 != z3 {
				t.Fatalf("hovercraft tyres changed it: yaw %v vs %v", y3, y0)
			}
		} else if y3 >= y0 {
			t.Fatalf("%s tyres 3: yaw %v, stock %v (turns right: falls)", typ, y3, y0)
		}
	}
}

// The cars are solid to a body on foot (World.Cars while it steps): it walks
// into one and stops at its side, and stands on its roof.
func TestBodyAgainstACar(t *testing.T) {
	m := maps.LabKart()
	w := NewWorld(&m)
	sp := carSpecs(t)["tank"]
	c := NewCar(sp, Vec3{-80, 0, 22}, east)
	b := NewBody(Vec3{-80, 0, 18})
	b.Yaw = math.Pi // facing +z: at the car
	for range 150 {
		w.Cars = CarDiscs(sp, c, nil)
		w.Step(b, Input{F: 1}, 0.01)
	}
	if want := 22 - sp.Size[0]/2 - R; math.Abs(b.Pos[2]-want) > 0.02 {
		t.Fatalf("walked into the car: z %v, its side at %v", b.Pos[2], want)
	}
	b = NewBody(Vec3{-80, sp.Size[1] + 1, 22})
	for range 100 {
		w.Cars = CarDiscs(sp, c, nil)
		w.Step(b, Input{}, 0.01)
	}
	if math.Abs(b.Pos[1]-sp.Size[1]) > 1e-9 || !b.OnGround {
		t.Fatalf("on the roof: %v, on the ground %v", b.Pos, b.OnGround)
	}
	w.Cars = nil
}

// Pressed against a wall or guardrail, steering away gets you off it: the
// contact must not keep killing the turn (operator report: "glued to the
// guardrail" in the buggy, twice). Every car type, both ways along the wall.
func TestCarSteersOffAWall(t *testing.T) {
	for _, typ := range []string{"buggy", "racecar", "tank", "hovercraft"} {
		for _, yaw := range []float64{0.4, -0.4} {
			w, sp, c := kartCar(t, typ, Vec3{0, 0, 30}, yaw)
			carSteps(w, sp, c, CarInput{Throttle: 1}, 250) // into the island's north face (z 18) and sliding along it
			wall := c.Pos[2] // where it touches (its discs against the face at z 18)
			if wall > 18+float64(sp.Size[2]/2) {
				t.Fatalf("%s yaw %v: never reached the wall: %v", typ, yaw, c.Pos)
			}
			best := 0.0
			for _, steer := range []float64{-1, 1} {
				w2, _, c2 := kartCar(t, typ, Vec3{0, 0, 30}, yaw)
				*c2 = *c
				carSteps(w2, sp, c2, CarInput{Throttle: 1, Steer: steer}, 200)
				best = math.Max(best, c2.Pos[2]-wall)
			}
			if best < 2 {
				t.Fatalf("%s yaw %v: steering away for 2 s left it %.2f m off the wall (glued)", typ, yaw, best)
			}
		}
	}
}
