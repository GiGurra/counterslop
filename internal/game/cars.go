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
		Cars      []*sim.CarSpec `json:"cars"`
		Equipment []*CarItem     `json:"equipment"`
		Shop      CarShopSpec    `json:"shop"`
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
	for _, it := range f.Equipment {
		if it.ID == "" || (it.Slot != "front" && it.Slot != "rear") {
			return fmt.Errorf("cars.json: equipment %q incomplete", it.ID)
		}
	}
	for _, c := range f.Cars {
		for _, id := range []string{c.Front, c.Rear} {
			found := false
			for _, it := range f.Equipment {
				found = found || it.ID == id
			}
			if id != "" && !found {
				return fmt.Errorf("cars.json: car %q: no equipment %q", c.ID, id)
			}
		}
	}
	CarTypes, CarItems, CarShop = f.Cars, f.Equipment, f.Shop
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
	ID      int    `json:"id" net:"cold"`
	Type    string `json:"k" net:"cold"` // its cars.json id
	Pos     Vec3   `json:"p" net:"hot,mm"`
	Yaw     Angle  `json:"y" net:"hot,angle"`
	Pitch   Angle  `json:"pt" net:"hot,angle"`
	Roll    Angle  `json:"rl" net:"hot,angle"`
	Vel     Vec3   `json:"v" net:"hot,mm"`     // the client draws it ahead of the snapshot by this
	Fwd     Angle  `json:"f" net:"hot,angle"`  // speed along its heading (m/s, 4 decimals; < 0 reversing): the wheels spin by it
	Steer   Angle  `json:"st" net:"hot,angle"` // the steering, -1 left .. 1 right
	Contact int    `json:"wc" net:"hot"`       // bit i: wheel i on the ground (sim.Car.Contact; 0 FL, 1 FR, 2 RL, 3 RR)
	Comp    int    `json:"wz" net:"hot"`       // wheel i's suspension in byte i: 0 hanging .. 255 compressed (CarComp)
	HP      int    `json:"hp" net:"hot"`
	MaxHP   int    `json:"mhp" net:"cold"`
	Driver  int    `json:"d,omitempty" net:"hot"` // the driver's player id (0: nobody)
	Wet     bool   `json:"wet,omitempty" net:"hot"`
	// For the looks only (web/assets/models/car.js): the yaw rate (a tank's
	// tracks run apart in a turn, the body leans) and what its driver does this
	// tick (carFx: throttle, boost, slipping, braking: the flames, the fans).
	YawVel Angle        `json:"yv" net:"hot,angle"`
	Fx     int          `json:"fx,omitempty" net:"hot"`
	Shield bool         `json:"sh,omitempty" net:"hot"` // a Code Freeze on it (pickups.go): no damage
	sim    sim.Car      `net:"-"`
	spec   *sim.CarSpec `net:"-"`
	spawn  int          `net:"-"` // its start in Map.Cars (-1: spawned by car_set)
	cs     carState     `net:"-"` // combat: equipment, charges, wreck, rams (carcombat.go)
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
	Boost    float64    `json:"bo,omitempty" net:"hot,self,exact"` // seconds of boost left (sim.Car.Boost)
	Slip     float64    `json:"sl,omitempty" net:"hot,self,exact"` // seconds of slipping left
	Engine   int        `json:"en,omitempty" net:"hot,self"`
	Tyres    int        `json:"ty,omitempty" net:"hot,self"`
	MaxHP    int        `json:"mhp,omitempty" net:"hot,self"`
	// The equipment and its charges left this lap (carcombat.go): server
	// only, shown on the HUD.
	Front string `json:"fi,omitempty" net:"hot,self"`
	Rear  string `json:"ri,omitempty" net:"hot,self"`
	FN    int    `json:"fn,omitempty" net:"hot,self"`
	RN    int    `json:"rn,omitempty" net:"hot,self"`
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
		s.fitCar(s.addCar(spec, sim.Vec3(sp.Pos), sp.Yaw, i), nil) // the type's built-ins (a racer's garage: startRace)
	}
	s.gear = s.gear[:0]
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
	c.YawVel = Angle(sc.YawVel)
}

// The snapshot's fx bits (web/assets/models/car.js FX_*): looks only.
const (
	carFxThrottle = 1
	carFxBoost    = 2
	carFxSlip     = 4
	carFxBrake    = 8
)

// carFx: the looks of car c under its driver's command b (f: the forward axis).
func carFx(c *sim.Car, f float64, b int) int {
	fx := 0
	if f > 0 {
		fx |= carFxThrottle
	}
	if b&BtnBoost != 0 && c.Boost > 0 {
		fx |= carFxBoost | carFxThrottle
	}
	if c.Slip > 0 {
		fx |= carFxSlip
	}
	if f < 0 || b&BtnJump != 0 {
		fx |= carFxBrake
	}
	return fx
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
			c.Fx = 0
			w.Cars = s.carDiscs(c)
			w.StepCar(c.spec, &c.sim, sim.CarInput{}, dt)
			w.Cars = nil
		}
		if s.outOfWorld(c.sim.Pos) {
			s.carHome(c)
		}
		c.sync()
	}
	now := s.clock()
	s.tickGear(dt)
	s.tickPits(dt)
	s.tickRams(now)
	s.tickWrecks(now)
	s.tickPickups(now)
	for _, c := range s.cars {
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
	return &carExact{Type: c.Type, Pos: sc.Pos, Vel: sc.Vel, Yaw: sc.Yaw, YawVel: sc.YawVel, Pitch: sc.Pitch, Roll: sc.Roll, Steer: sc.Steer, OnGround: sc.OnGround, Contact: sc.Contact, Comp: sc.Comp[:], HP: sc.HP, Wet: sc.Wet,
		Boost: sc.Boost, Slip: sc.Slip, Engine: sc.Engine, Tyres: sc.Tyres, MaxHP: c.MaxHP, Front: c.cs.front, Rear: c.cs.rear, FN: c.cs.fn, RN: c.cs.rn}
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
		if d < bestD && math.Abs(c.sim.Pos[1]-p.body.Pos[1]) < 2.5 && !s.carTaken(c.ID) && c.sim.HP > 0 {
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
	s.carWeapons(p, car, c)
	car.Fx = carFx(&car.sim, c.F, c.B)
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
		s.fitCar(c, p) // fitted as every car is: its items, a full tank
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
	m["carBoost"] = r(sc.Boost)
}
