package game

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/gigurra/counterslop/internal/sim"
)

// Cars (docs/racing.md): the map's cars (content.Map.Cars, types from
// web/assets/cars.json), back at their starts every round and on a map
// change, stepped every tick (internal/sim car.go): by their driver's
// command (car_shared.go DriveStep), or with no input when nobody drives
// them (they roll to a stop). Every full snapshot carries them ("cars").

// CarTypes is web/assets/cars.json (LoadCars), in its order (by tier).
var CarTypes []*sim.CarSpec

// LoadCars reads web/assets/cars.json into CarTypes.
func LoadCars(b []byte) error {
	var f struct {
		Cars []*sim.CarSpec `json:"cars"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("cars.json: %w", err)
	}
	if len(f.Cars) == 0 {
		return fmt.Errorf("cars.json: no cars")
	}
	for _, c := range f.Cars {
		if c.ID == "" || c.TopSpeed <= 0 || c.Size[0] <= 0 || c.Size[2] <= 0 || c.HP <= 0 {
			return fmt.Errorf("cars.json: car %q incomplete", c.ID)
		}
	}
	CarTypes = f.Cars
	return nil
}

// CarSpecOf is car type id (nil: none).
func CarSpecOf(id string) *sim.CarSpec {
	for _, c := range CarTypes {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// Car is one car as everyone sees it: enough to draw it, its wheels and its
// suspension (phase 4 animates them from Fwd, Steer, Contact and Comp).
type Car struct {
	ID      int          `json:"id" net:"cold"`
	Type    string       `json:"k" net:"cold"` // its cars.json id
	Pos     Vec3         `json:"p" net:"hot,mm"`
	Yaw     Angle        `json:"y" net:"hot,angle"`
	Pitch   Angle        `json:"pt" net:"hot,angle"`
	Roll    Angle        `json:"rl" net:"hot,angle"`
	Vel     Vec3         `json:"v" net:"hot,mm"`     // the client draws it ahead of the snapshot by this
	Fwd     Angle        `json:"f" net:"hot,angle"`  // speed along its heading (m/s, 4 decimals; < 0 reversing): the wheels spin by it
	Steer   Angle        `json:"st" net:"hot,angle"` // the steering, -1 left .. 1 right
	Contact int          `json:"wc" net:"hot"`       // bit i: wheel i on the ground (sim.Car.Contact; 0 FL, 1 FR, 2 RL, 3 RR)
	Comp    int          `json:"wz" net:"hot"`       // wheel i's suspension in byte i: 0 hanging .. 255 compressed (CarComp)
	HP      int          `json:"hp" net:"hot"`
	MaxHP   int          `json:"mhp" net:"cold"`
	Driver  int          `json:"d,omitempty" net:"hot"` // the driver's player id (0: nobody)
	Wet     bool         `json:"wet,omitempty" net:"hot"`
	sim     sim.Car      `net:"-"`
	spec    *sim.CarSpec `net:"-"`
	spawn   int          `net:"-"` // its start in Map.Cars (-1: spawned by car_set)
}

// CarComp packs the four suspension values (0..1) a byte each, wheel i in byte i.
func CarComp(c [4]float64) int {
	n := 0
	for i, v := range c {
		n |= int(math.Round(math.Max(0, math.Min(1, v))*255)) << (8 * i)
	}
	return n
}

// carExact is a car's simulated state, exact (sim.Car), for its driver's prediction.
type carExact struct {
	Type     string     `json:"k" net:"cold,self"`
	Pos      [3]float64 `json:"p" net:"hot,self,exact"`
	Vel      [3]float64 `json:"v" net:"hot,self,exact"`
	Yaw      float64    `json:"y" net:"hot,self,exact"`
	YawVel   float64    `json:"yv" net:"hot,self,exact"`
	Pitch    float64    `json:"pt" net:"hot,self,exact"`
	Roll     float64    `json:"rl" net:"hot,self,exact"`
	Steer    float64    `json:"st" net:"hot,self,exact"`
	OnGround bool       `json:"g,omitempty" net:"hot,self"`
	Contact  int        `json:"wc" net:"hot,self"`
	Comp     []float64  `json:"wz" net:"hot,self,exact"`
	HP       int        `json:"hp" net:"hot,self"`
	Wet      bool       `json:"wet,omitempty" net:"hot,self"`
}

// resetCars puts every car back at its start, new. Callers hold s.mu.
func (s *Server) resetCars() {
	for _, p := range s.players { // everyone out: the cars go back without them
		p.car = 0
	}
	s.cars = s.cars[:0]
	for i, sp := range s.Map.Cars {
		spec := CarSpecOf(sp.Type)
		if spec == nil {
			continue // a type this build doesn't have (a custom map)
		}
		s.addCar(spec, sim.Vec3(sp.Pos), sp.Yaw, i)
	}
}

// addCar puts a new car of type spec at pos facing yaw (spawn: its start in
// Map.Cars, -1: none). Callers hold s.mu.
func (s *Server) addCar(spec *sim.CarSpec, pos sim.Vec3, yaw float64, spawn int) *Car {
	id := 1
	for _, c := range s.cars {
		id = max(id, c.ID+1)
	}
	c := &Car{ID: id, Type: spec.ID, MaxHP: spec.HP, spec: spec, spawn: spawn, sim: *sim.NewCar(spec, pos, yaw)}
	c.sync()
	s.cars = append(s.cars, c)
	return c
}

// sync copies the simulated state to what is sent.
func (c *Car) sync() {
	sc := &c.sim
	c.Pos, c.Yaw, c.Pitch, c.Roll, c.Vel = Vec3(sc.Pos), Angle(sc.Yaw), Angle(sc.Pitch), Angle(sc.Roll), Vec3(sc.Vel)
	c.Fwd, c.Steer, c.Contact, c.Comp, c.HP, c.Wet = Angle(sim.CarForward(sc)), Angle(sc.Steer), sc.Contact, CarComp(sc.Comp), sc.HP, sc.Wet
}

// carDiscs is every car's discs but skip's (the other cars, for one car's step).
func (s *Server) carDiscs(skip *Car) []sim.CarDisc {
	var out []sim.CarDisc
	for _, c := range s.cars {
		if c != skip {
			out = sim.CarDiscs(c.spec, &c.sim, out)
		}
	}
	return out
}

// tickCars steps the cars nobody drives (they roll to a stop) and puts one
// that fell out of the world back at its start. A driven car moves only by
// its driver's commands (DriveStep), however they arrive: as its driver's
// client predicts it. Callers hold s.mu.
func (s *Server) tickCars(dt float64) {
	if len(s.cars) == 0 {
		return
	}
	w := s.simWorld()
	for _, c := range s.cars {
		c.Driver = 0
	}
	for _, p := range s.players {
		if c := s.carByID(p.car); p.car != 0 && c != nil && p.body != nil && !p.Dead {
			c.Driver = p.ID
		}
	}
	for _, c := range s.cars {
		if c.Driver == 0 {
			w.Cars = s.carDiscs(c)
			w.StepCar(c.spec, &c.sim, sim.CarInput{}, dt)
			w.Cars = nil
		}
		if s.outOfWorld(c.sim.Pos) {
			s.carHome(c)
		}
		c.sync()
	}
}

// carHome puts car c back at its start (a spawned one: where it is, over
// the map's first spawn), still, its driver out. Callers hold s.mu.
func (s *Server) carHome(c *Car) {
	pos, yaw := c.sim.Pos, c.sim.Yaw
	if c.spawn >= 0 && c.spawn < len(s.Map.Cars) {
		pos, yaw = sim.Vec3(s.Map.Cars[c.spawn].Pos), s.Map.Cars[c.spawn].Yaw
	} else if len(s.Map.Spawns) > 0 {
		pos = sim.Vec3(s.Map.Spawns[0])
	}
	hp := c.sim.HP
	c.sim = *sim.NewCar(c.spec, pos, yaw)
	c.sim.HP = hp
	c.Driver = 0
	for _, p := range s.players {
		if p.car == c.ID {
			p.car = 0
		}
	}
}

// carsJSON is the cars for the snapshot ("cars":[...]; nothing on a map without).
func (s *Server) carsJSON() []byte {
	if len(s.cars) == 0 {
		return nil
	}
	b, _ := json.Marshal(s.cars)
	return append([]byte(`"cars":`), b...)
}

// carByID is car id, or nil.
func (s *Server) carByID(id int) *Car {
	for _, c := range s.cars {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// carExactOf is the car p drives, exact, for its self-snapshot (nil on foot).
func (s *Server) carExactOf(p *Player) *carExact {
	c := s.carByID(p.car)
	if p.car == 0 || c == nil {
		return nil
	}
	sc := &c.sim
	return &carExact{Type: c.Type, Pos: sc.Pos, Vel: sc.Vel, Yaw: sc.Yaw, YawVel: sc.YawVel, Pitch: sc.Pitch, Roll: sc.Roll, Steer: sc.Steer, OnGround: sc.OnGround, Contact: sc.Contact, Comp: sc.Comp[:], HP: sc.HP, Wet: sc.Wet}
}

// carTaken: someone alive drives car id.
func (s *Server) carTaken(id int) bool {
	for _, q := range s.players {
		if q.car == id && !q.Dead && q.body != nil {
			return true
		}
	}
	return false
}

// boardCar puts p in the driver's seat of the nearest free car within
// CarBoardReach (feet, x and z; within 2.5 m up or down). Callers hold s.mu.
func (s *Server) boardCar(p *Player) bool {
	if p.car != 0 || p.ride != 0 || p.body == nil || p.Dead {
		return false
	}
	var best *Car
	bestD := CarBoardReach
	for _, c := range s.cars {
		d := math.Hypot(c.sim.Pos[0]-p.body.Pos[0], c.sim.Pos[2]-p.body.Pos[2])
		if d < bestD && math.Abs(c.sim.Pos[1]-p.body.Pos[1]) < 2.5 && !s.carTaken(c.ID) {
			best, bestD = c, d
		}
	}
	if best == nil {
		return false
	}
	p.car = best.ID
	best.Driver = p.ID
	CarSeat(best.spec, &best.sim, p.body, 1.0/SimHz)
	p.Pos, p.Crouch = Vec3(p.body.Pos), true
	return true
}

// carCmd runs command c of p if it drives a car: E gets out (then it moves
// on foot this command), else DriveStep. Reports whether p drove. Callers
// hold s.mu.
func (s *Server) carCmd(w *sim.World, p *Player, c Cmd) bool {
	if p.car == 0 {
		return false
	}
	car := s.carByID(p.car)
	if car == nil || p.Dead {
		p.car = 0
		return false
	}
	if CarLeaves(c.B, p.lastCmd.B, s.carLocked(p)) {
		CarLeave(car.spec, &car.sim, p.body) // as the client predicts it
		p.car, p.leftCar = 0, p.cmdTicks
		return false
	}
	DriveStep(w, car.spec, &car.sim, p.body, c.F, c.R, c.B, s.carDiscs(car), 1.0/SimHz)
	car.sync()
	return true
}

// CarAt is a car's place (sim/cases "carAt": where the case's car starts).
type CarAt struct {
	Pos [3]float64 `json:"pos"`
	Yaw float64    `json:"yaw"`
}

// seatInCar puts p in car id's driver's seat (test harness, sim/cases "car"),
// the car new at at first if given. Callers hold s.mu.
func (s *Server) seatInCar(p *Player, id int, at ...*CarAt) error {
	c := s.carByID(id)
	if c == nil || p.body == nil {
		return fmt.Errorf("no car %d (%d on this map) or no body", id, len(s.cars))
	}
	if len(at) > 0 && at[0] != nil {
		c.sim = *sim.NewCar(c.spec, sim.Vec3(at[0].Pos), at[0].Yaw)
		c.sync()
	}
	if s.carTaken(id) {
		return fmt.Errorf("car %d is taken", id)
	}
	p.ride, p.seat, p.car = 0, 0, id
	c.Driver = p.ID
	CarSeat(c.spec, &c.sim, p.body, 1.0/SimHz)
	p.Pos, p.Crouch = Vec3(p.body.Pos), true
	return nil
}

// carCaseState adds the car p drives to its sim/cases state (nothing on
// foot: every other case's trace stays as it was).
func (s *Server) carCaseState(p *Player, m map[string]any) {
	c := s.carByID(p.car)
	if p.car == 0 || c == nil {
		return
	}
	r := func(v float64) float64 { return math.Round(v*1e4) / 1e4 }
	sc := &c.sim
	m["car"], m["carGround"], m["carWet"], m["carHP"] = c.ID, sc.OnGround, sc.Wet, sc.HP
	m["carPos"] = []float64{r(sc.Pos[0]), r(sc.Pos[1]), r(sc.Pos[2])}
	m["carVel"] = []float64{r(sc.Vel[0]), r(sc.Vel[1]), r(sc.Vel[2])}
	m["carYaw"] = r(sc.Yaw)
}
