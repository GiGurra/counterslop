package maps

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// raceLines are each race track's centre line in driving order (x, z): an
// autopilot steers along it (TestDriveTheTracks). A new track adds its own.
var raceLines = map[string][][2]float64{
	"lab_race": {{-50, 12}, {-56, 0}, {-50, -12}, {0, -12}, {50, -12}, {56, 0}, {50, 12}, {0, 12}},
	"rc_ci_pipeline": {{-74, 0}, {-86, 8}, {-86, 76}, {-78, 86}, {-66, 86}, {-26, 86}, {-8, 86}, {0, 78}, {0, -78}, {8, -86}, {78, -86}, {86, -78},
		{86, -8}, {78, 0}, {20, 0}},
}

// Every race track is drivable: a simple autopilot (steer at the next point
// of the centre line, lift in sharp turns) drives a lap in every car type
// with the real car physics, from the first grid slot through every gate in
// order and over the line again, never off the track (into the pit).
func TestDriveTheTracks(t *testing.T) {
	b, err := os.ReadFile("../../../web/assets/cars.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Cars []*sim.CarSpec }
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	for name, line := range raceLines {
		m := Labs[name]
		if m == nil {
			m = All[name]
		}
		mp := m()
		for _, sp := range f.Cars {
			t.Run(name+"/"+sp.ID, func(t *testing.T) { driveLap(t, &mp, sp, line) })
		}
	}
	for name, build := range All { // every race map in the rotation has a line
		if build().Mode == c.ModeRace && raceLines[name] == nil {
			t.Errorf("%s: no centre line in raceLines", name)
		}
	}
}

func driveLap(t *testing.T, m *c.Map, sp *sim.CarSpec, line [][2]float64) {
	w := sim.NewWorld(m)
	g := m.Cars[0]
	car := sim.NewCar(sp, sim.Vec3(g.Pos), g.Yaw)
	cps := m.Checkpoints
	passed, wp := 0, 0
	const dt = 0.01
	for tick := range 12000 { // 2 minutes
		tx, tz := line[wp][0], line[wp][1]
		dx, dz := tx-car.Pos[0], tz-car.Pos[2]
		if math.Hypot(dx, dz) < 5 {
			wp = (wp + 1) % len(line)
			continue
		}
		want := math.Atan2(-dx, -dz) // the yaw looking at it: forward is (-sin yaw, -cos yaw)
		e := math.Remainder(want-car.Yaw, 2*math.Pi)
		in := sim.CarInput{Throttle: 1, Steer: math.Max(-1, math.Min(1, -3*e)), Handbrake: false}
		// Brake for a sharp turn coming up (at the next point), or while still turning hard.
		v := sim.CarSpeed(car)
		slide := math.Abs(float64(car.Vel[0]*math.Cos(car.Yaw)) - float64(car.Vel[2]*math.Sin(car.Yaw))) // sideways (a hovercraft drifts)
		vc := math.Min(cornerSpeed, 3+3*sp.Grip)                                                         // the corner's speed: lower for a car that slides (the hovercraft)
		stop := (v*v-vc*vc)/(2*sp.Brake) + 8                                                             // how far it takes to slow to it
		corner, d := false, math.Hypot(dx, dz)                                                           // a sharp turn within braking distance along the next points
		for k := 0; k < 3 && d < stop; k++ {
			i := (wp + k) % len(line)
			corner = corner || turnAt(line, i) > 0.6
			n := line[(i+1)%len(line)]
			d += math.Hypot(n[0]-line[i][0], n[1]-line[i][1])
		}
		if (corner || math.Abs(e) > 0.5 || slide > 3 && corner) && v > vc {
			in.Throttle = -1
		}
		w.StepCar(sp, car, in, dt)
		p := [3]float64(car.Pos)
		for _, z := range m.Pits {
			if z.Contains(p) {
				t.Fatalf("tick %d: off the track at %v (gate %d next, steering at %v)", tick, p, passed%len(cps), line[wp])
			}
		}
		seat := [3]float64(sim.CarSeatWorld(sp, car))
		if cps[passed%len(cps)].In(seat) {
			passed++
			if passed == len(cps)+1 {
				t.Logf("a lap in %.1f s", float64(tick)*dt)
				return
			}
		}
	}
	t.Fatalf("no lap in 2 minutes: %d gates passed, at %v", passed, car.Pos)
}

// cornerSpeed: how fast the autopilot takes a sharp turn (m/s).
const cornerSpeed = 11.0

// turnAt: how sharply the line turns at point i (radians).
func turnAt(line [][2]float64, i int) float64 {
	a, b, n := line[(i+len(line)-1)%len(line)], line[i], line[(i+1)%len(line)]
	h0 := math.Atan2(b[0]-a[0], b[1]-a[1])
	h1 := math.Atan2(n[0]-b[0], n[1]-b[1])
	return math.Abs(math.Remainder(h1-h0, 2*math.Pi))
}

// Every gate's respawn spot holds a car on the track (two seconds standing:
// on the ground, not in the pit), and on rc_ci_pipeline a car respawned at
// the jump's gate clears the gap from a standing start, every type.
func TestRaceRespawnSpots(t *testing.T) {
	b, err := os.ReadFile("../../../web/assets/cars.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Cars []*sim.CarSpec }
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	for name := range raceLines {
		build := Labs[name]
		if build == nil {
			build = All[name]
		}
		m := build()
		w := sim.NewWorld(&m)
		for i, g := range m.Checkpoints {
			car := sim.NewCar(f.Cars[0], sim.Vec3(g.Spawn), g.Yaw)
			for range 200 {
				w.StepCar(f.Cars[0], car, sim.CarInput{}, 0.01)
			}
			if !car.OnGround || math.Abs(car.Pos[1]-g.Spawn[1]) > 0.05 {
				t.Errorf("%s gate %d (%s): a car at its respawn spot %v ended at %v, on the ground %v", name, i, g.Name, g.Spawn, car.Pos, car.OnGround)
			}
		}
	}
	m := RcCiPipeline()
	w := sim.NewWorld(&m)
	g := m.Checkpoints[1]
	for _, sp := range f.Cars {
		car := sim.NewCar(sp, sim.Vec3(g.Spawn), g.Yaw)
		for range 400 {
			w.StepCar(sp, car, sim.CarInput{Throttle: 1}, 0.01)
		}
		if car.Pos[2] < 60 || car.Pos[1] < -0.1 {
			t.Errorf("%s from %s's respawn spot, 4 s full throttle: at %v, want past the gap (z 52) on the road", sp.ID, g.Name, car.Pos)
		}
	}
}
