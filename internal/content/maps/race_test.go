package maps

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// raceLines: a line in driving order (x, z) for a race map without its own
// racing line (Map.RaceLine): an autopilot steers along it (TestDriveTheTracks).
var raceLines = map[string][][2]float64{
	"lab_race": {{-50, 12}, {-56, 0}, {-50, -12}, {0, -12}, {50, -12}, {56, 0}, {50, 12}, {0, 12}},
}

// raceMaps: every race map (the rotation's and the labs') by name.
func raceMaps() map[string]c.Map {
	out := map[string]c.Map{}
	for _, all := range []map[string]func() c.Map{All, Labs} {
		for name, build := range all {
			if m := build(); m.Mode == c.ModeRace {
				out[name] = m
			}
		}
	}
	return out
}

// lineOf: the map's racing line (x, z), else its entry in raceLines.
func lineOf(name string, m c.Map) [][2]float64 {
	if len(m.RaceLine) == 0 {
		return raceLines[name]
	}
	out := make([][2]float64, len(m.RaceLine))
	for i, p := range m.RaceLine {
		out[i] = [2]float64{p[0], p[2]}
	}
	return out
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
	for name, mp := range raceMaps() {
		line := lineOf(name, mp)
		if All[name] != nil && len(mp.RaceLine) < 3 { // the rotation's tracks: a racing line for the bots
			t.Errorf("%s: no racing line (Map.RaceLine)", name)
		}
		if len(line) < 3 {
			t.Errorf("%s: no line to drive", name)
			continue
		}
		for _, sp := range f.Cars {
			t.Run(name+"/"+sp.ID, func(t *testing.T) { driveLap(t, &mp, sp, line) })
		}
	}
}

func driveLap(t *testing.T, m *c.Map, sp *sim.CarSpec, line [][2]float64) {
	w := sim.NewWorld(m)
	g := m.Cars[0]
	car := sim.NewCar(sp, sim.Vec3(g.Pos), g.Yaw)
	cps := m.Checkpoints
	passed, wp := 0, 0
	for i, q := range line { // the first point ahead of the grid slot
		hx, hz := -math.Sin(g.Yaw), -math.Cos(g.Yaw)
		if (q[0]-car.Pos[0])*hx+(q[1]-car.Pos[2])*hz > 2 && math.Hypot(q[0]-car.Pos[0], q[1]-car.Pos[2]) < math.Hypot(line[wp][0]-car.Pos[0], line[wp][1]-car.Pos[2]) {
			wp = i
		}
	}
	const dt = 0.01
	for tick := range 12000 { // 2 minutes
		tx, tz := line[wp][0], line[wp][1]
		dx, dz := tx-car.Pos[0], tz-car.Pos[2]
		hx, hz := -math.Sin(car.Yaw), -math.Cos(car.Yaw)
		if math.Hypot(dx, dz) < 5 || dx*hx+dz*hz < 0 { // reached, or gone by (a jump flies past points)
			wp = (wp + 1) % len(line)
			continue
		}
		for k := 1; k < len(line) && math.Hypot(dx, dz) < 12; k++ { // aim 12 m out at least (a close point over-steers a sliding hovercraft)
			q := line[(wp+k)%len(line)]
			dx, dz = q[0]-car.Pos[0], q[1]-car.Pos[2]
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
		turn := 0.0                                                                                      // the turn of the line within braking distance (a corner over several points adds up)
		for k := 0; k < len(line) && d < stop; k++ {
			i := (wp + k) % len(line)
			turn += turnAt(line, i)
			corner = corner || turn > 0.6
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
	for name, m := range raceMaps() {
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

// rc_ci_pipeline's pit lane is drivable: off the east leg's last corner into
// it, through it, and back onto the start straight, in every car type, never
// into the void, a while inside it.
func TestPitLaneDrivable(t *testing.T) {
	b, err := os.ReadFile("../../../web/assets/cars.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Cars []*sim.CarSpec }
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	m := RcCiPipeline()
	pit := m.PitLanes[0]
	line := [][2]float64{{62, -3}, {54, -9.5}, {36, -9.5}, {18, -9.5}, {10, -2}, {-20, 0}}
	for _, sp := range f.Cars {
		w := sim.NewWorld(&m)
		car := sim.NewCar(sp, sim.Vec3{78, 0, 0}, math.Pi/2)
		in, wp := 0, 0
		for tick := range 4000 {
			if wp == len(line) {
				break
			}
			dx, dz := line[wp][0]-car.Pos[0], line[wp][1]-car.Pos[2]
			if math.Hypot(dx, dz) < 3 {
				wp++
				continue
			}
			e := math.Remainder(math.Atan2(-dx, -dz)-car.Yaw, 2*math.Pi)
			thr := 1.0
			if sim.CarSpeed(car) > 12 {
				thr = 0
			}
			w.StepCar(sp, car, sim.CarInput{Throttle: thr, Steer: math.Max(-1, math.Min(1, -3*e))}, 0.01)
			for _, z := range m.Pits {
				if z.Contains([3]float64(car.Pos)) {
					t.Fatalf("%s: off the track at %v (tick %d)", sp.ID, car.Pos, tick)
				}
			}
			if pit.Contains([3]float64(car.Pos)) {
				in++
			}
		}
		if wp < len(line) || in < 100 {
			t.Fatalf("%s: through the pit lane: %d of %d points, %d ticks in it, at %v", sp.ID, wp, len(line), in, car.Pos)
		}
	}
}
