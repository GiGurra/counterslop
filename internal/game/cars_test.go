package game

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

func kartServer(t *testing.T) *Server {
	t.Helper()
	s := authServer(t)
	s.Test = true
	if err := s.ResetWorldOn("lab_kart"); err != nil {
		t.Fatal(err)
	}
	return s
}

// kartPlayer is a player standing at pos on lab_kart.
func kartPlayer(t *testing.T, s *Server, i int, pos Vec3) *Player {
	t.Helper()
	p := s.join(fmt.Sprintf("10.0.8.%d", i))
	p.ready, p.Team = true, []string{content.Vibe, content.Artisan}[i%2]
	s.teleport(p, pos, 0, false, 0)
	return p
}

// cars.json loads, one of each type stands on lab_kart's grid, and every
// full snapshot carries them; a map without cars sends none.
func TestCarsOnTheGrid(t *testing.T) {
	if len(CarTypes) != 4 || CarSpecOf("tank") == nil || CarSpecOf("nope") != nil {
		t.Fatalf("car types %v", CarTypes)
	}
	if LoadCars([]byte(`{"cars":[]}`)) == nil || LoadCars([]byte(`{"cars":[{"id":"x"}]}`)) == nil || LoadCars([]byte(`{`)) == nil {
		t.Fatal("bad cars.json accepted")
	}
	s := kartServer(t)
	if len(s.cars) != 4 {
		t.Fatalf("%d cars", len(s.cars))
	}
	for i, typ := range []string{"buggy", "racecar", "tank", "hovercraft"} {
		c := s.cars[i]
		if c.ID != i+1 || c.Type != typ || c.HP != CarSpecOf(typ).HP || c.Pos != (Vec3{-60, 0, 24 + 7*float64(i)}) {
			t.Fatalf("car %d: %+v", i, c)
		}
	}
	s.mu.Lock()
	b := s.carsJSON()
	s.mu.Unlock()
	var m struct {
		Cars []struct {
			ID  int
			K   string
			P   [3]float64
			Y   float64
			HP  int
			Wc  int
			Wz  int
			Mhp int
		}
	}
	if err := json.Unmarshal(append(append([]byte("{"), b...), '}'), &m); err != nil || len(m.Cars) != 4 || m.Cars[3].K != "hovercraft" || m.Cars[2].Mhp != 250 || m.Cars[0].Wc != 15 || m.Cars[0].Wz == 0 {
		t.Fatalf("%s: %+v %v", b, m, err)
	}
	if err := s.ResetWorldOn("de_slop"); err != nil {
		t.Fatal(err)
	}
	if s.carsJSON() != nil {
		t.Fatal("cars on de_slop")
	}
}

// CarComp packs the four suspension values a byte each.
func TestCarComp(t *testing.T) {
	if got := CarComp([4]float64{0, 1, 0.5, 2}); got != 0|255<<8|128<<16|255<<24 {
		t.Fatalf("%x", got)
	}
}

// E by a car takes its driver's seat (a free car, the nearest, within
// reach); the self-snapshot carries the car, exact; the driver's commands
// drive it (W along its heading, the driver seated in it, crouched); a car
// nobody drives rolls to a stop; E gets out over the seat, on foot, and that
// press boards nothing; death and a new round put the driver out, and the
// round puts the cars back.
func TestDrivingACar(t *testing.T) {
	s := kartServer(t)
	a := kartPlayer(t, s, 0, Vec3{-60, 0, 22}) // 2 m from the buggy
	b := kartPlayer(t, s, 1, Vec3{-60, 0, 21.5})
	far := kartPlayer(t, s, 2, Vec3{0, 0, 40})
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.boardCar(a) || a.car != 1 || !a.body.Crouched {
		t.Fatalf("boarded: car %d", a.car)
	}
	if s.boardCar(b) && b.car == 1 {
		t.Fatal("two drivers in the buggy")
	}
	b.car = 0
	if s.boardCar(far) {
		t.Fatal("boarded from afar")
	}
	m := s.meStateOf(a)
	if m.Car != 1 || m.CarState == nil || m.CarState.Type != "buggy" || m.CarState.Pos != [3]float64(s.cars[0].sim.Pos) || len(m.CarState.Comp) != 4 {
		t.Fatalf("self-snapshot %+v", m)
	}
	w := s.simWorld()
	for i := range 100 {
		a.cmdTicks = int64(i + 1)
		s.runCmd(w, a, Cmd{Seq: i + 1, F: 1})
	}
	c := s.cars[0]
	spec := CarSpecOf("buggy")
	if c.sim.Pos[0] < -55 || sim.CarSpeed(&c.sim) < 10 || a.body.Pos != sim.CarSeatWorld(spec, &c.sim) || a.Pos != Vec3(a.body.Pos) || c.Fwd < 10 {
		t.Fatalf("driven: car %v speed %v body %v", c.sim.Pos, sim.CarSpeed(&c.sim), a.body.Pos)
	}
	// tickCars leaves a driven car to its driver's commands.
	at := c.sim.Pos
	s.tickCars(0.01)
	if c.sim.Pos != at || c.Driver != a.ID {
		t.Fatalf("tickCars moved a driven car: %v -> %v (driver %d)", at, c.sim.Pos, c.Driver)
	}
	// Out with E: over the seat, on foot; the same press boards nothing.
	a.cmdTicks++
	s.runCmd(w, a, Cmd{Seq: 101, B: BtnUse})
	if a.car != 0 || a.body.Pos[1] < c.sim.Pos[1]+spec.Size[1]-0.2 {
		t.Fatalf("E: car %d body %v", a.car, a.body.Pos)
	}
	for range 1500 {
		s.tickCars(0.01)
	}
	if c.Driver != 0 || sim.CarSpeed(&c.sim) > 0.01 {
		t.Fatalf("rolling on with nobody in it: %v", sim.CarSpeed(&c.sim))
	}
	// Death puts the driver out; a new round puts every car back.
	a.body.Pos = sim.Vec3{c.sim.Pos[0], 0, c.sim.Pos[2] + 2}
	if !s.boardCar(a) {
		t.Fatal("no boarding again")
	}
	s.killBy(b, a, "prompt", false)
	if a.car != 0 || s.carTaken(1) {
		t.Fatal("the dead still drives")
	}
	s.startRound(s.clock())
	if s.cars[0].Pos != (Vec3{-60, 0, 24}) || s.cars[0].Vel != (Vec3{}) {
		t.Fatalf("after the round start: %+v", s.cars[0])
	}
}

// A car off the map's edge falls out of the world and is put back at its
// start, its driver out.
func TestCarOutOfTheWorld(t *testing.T) {
	s := kartServer(t)
	a := kartPlayer(t, s, 0, Vec3{-60, 0, 22})
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.seatInCar(a, 1); err != nil {
		t.Fatal(err)
	}
	if s.seatInCar(a, 1) == nil || s.seatInCar(a, 9) == nil {
		t.Fatal("seated in a taken car or none")
	}
	c := s.cars[0]
	c.sim.Pos, c.sim.OnGround = sim.Vec3{200, 0, 0}, false
	a.car = 0 // nobody drives: tickCars steps it
	for range 300 {
		s.tickCars(0.01)
	}
	if c.Pos != (Vec3{-60, 0, 24}) {
		t.Fatalf("not back on the grid: %v", c.Pos)
	}
}

// The MCP debugger's car_set moves a car, spawns one, puts one home and
// lists them all.
func TestMCPCarSet(t *testing.T) {
	s := kartServer(t)
	c := mcpFor(t, s, true)
	r := c.call("car_set", map[string]any{"id": 2, "pos": []float64{5, 0, 30}, "yaw": 1, "vel": []float64{1, 0, 2}, "hp": 7})
	cs := r["cars"].([]any)
	if len(cs) != 4 || cs[1].(map[string]any)["yaw"].(float64) != 1 || s.cars[1].Pos != (Vec3{5, 0, 30}) || s.cars[1].HP != 7 {
		t.Fatalf("car_set %v", r)
	}
	r = c.call("car_set", map[string]any{"type": "tank", "pos": []float64{0, 0, 40}})
	if cs := r["cars"].([]any); len(cs) != 5 || cs[4].(map[string]any)["type"] != "tank" || cs[4].(map[string]any)["id"].(float64) != 5 {
		t.Fatalf("spawn %v", r)
	}
	c.call("car_set", map[string]any{"id": 2, "home": true})
	if s.cars[1].Pos != (Vec3{-60, 0, 31}) {
		t.Fatalf("home: %v", s.cars[1].Pos)
	}
	c.call("car_set", map[string]any{"id": 9}, true)
	c.call("car_set", map[string]any{"type": "bus"}, true)
	c.call("car_set", map[string]any{"id": 1, "pos": []float64{1, 2}}, true)
	if math.IsNaN(s.cars[0].sim.Pos[0]) {
		t.Fatal("nan")
	}
}
