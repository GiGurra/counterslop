package game

import (
	"encoding/json"
	"math"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// Combat while driving (docs/racing.md, race phase 3), server only: the
// clients see it in state (car HP, charges in the driver's self-snapshot,
// the live shots and drops in every full snapshot's "gear") and the optional
// "car" events (fire, boom, wreck, ram).
//
//   - The car's built-in equipment (cars.json "equipment"): a FRONT item fired
//     along the car's heading (Q, BtnCarFront held: one shot per interval) and
//     a REAR item dropped behind it (C, BtnCarRear's down edge), each with
//     charges, refilled every lap with the boost tank (not the car's HP).
//     front: a beam (hitscan), rockets (fly straight, burst on what they
//     touch), orbs (slow, homing on the nearest car ahead), or the ram plate
//     (no charges: rams hit harder). rear: spikes and oil (a car over them
//     slips: sim.Car.Slip; oil spins it too), mines (blow up under the next
//     car, knock it up; sticky ones creep toward a car near them).
//   - Car HP (sim.Car.HP): bullets on the car's body (an oriented box), every
//     blast near it, rams. At 0 the car is wrecked: its driver dies (killed by
//     whoever did it). A driver killed (the exposed head and shoulders, on
//     the model as on foot) wrecks the car. Out of a race a wreck goes home
//     repaired after carWreckSecs.
//   - Rams: two cars meeting at speed both take damage by the closing speed,
//     the heavier one less (CarSpec.Mass), plating and the ram plate more. A
//     car hitting a body on foot at speed hurts it (run over).
const (
	carBulletK     = 0.6  // of a bullet's damage the car's body takes
	carWreckSecs   = 5.0  // out of a race: a wreck goes home repaired after this
	carRamMin      = 6.0  // m/s closing speed: slower is a nudge
	carRamK        = 2.2  // HP per m/s over carRamMin, split by mass
	carRamGap      = 0.15 // m: discs this close count as touching
	carRamEvery    = 0.6  // s: one ram per pair this often
	carRunOverMin  = 7.0  // m/s: a car this fast hurts a body it hits
	carRunOverK    = 3.0  // HP per m/s
	carRunOverKick = 7.0  // m/s: the body thrown off it
	carShotLife    = 6.0  // s: a rocket or orb that hits nothing is gone
	carDropArm     = 0.6  // s: a drop arms (its own car got clear)
	carMaxGear     = 64   // live shots and drops, at most (the oldest go)
	carPitRepair   = 35.0 // HP a second for a car in a pit lane (Map.PitLanes)
)

// CarItem is one piece of car equipment (cars.json "equipment").
type CarItem struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Slot     string  `json:"slot"` // front | rear
	Kind     string  `json:"kind"` // beam | rocket | orb | ram | spikes | oil | mine
	Desc     string  `json:"desc"`
	Charges  int     `json:"charges"`  // a lap
	Interval float64 `json:"interval"` // s between two
	Damage   float64 `json:"damage"`
	Radius   float64 `json:"radius"`  // blast radius; spikes and oil: their size
	Speed    float64 `json:"speed"`   // m/s: rockets, orbs; sticky mines creeping
	Range    float64 `json:"range"`   // m: the beam; how far a shot flies
	Seek     float64 `json:"seek"`    // m: an orb homes on a car this near ahead; a sticky mine creeps to one this near
	Turn     float64 `json:"turn"`    // rad/s: an orb's turn rate
	Slip     float64 `json:"slip"`    // s: spikes, oil
	Spin     float64 `json:"spin"`    // rad/s: oil's spin-out kick
	Trigger  float64 `json:"trigger"` // m: a mine goes off when a car is this near
	Knock    float64 `json:"knock"`   // m/s up: a mine's blast
	Life     float64 `json:"life"`    // s: a drop lasts
	Ram      float64 `json:"ram"`     // the ram plate's ram damage factor
}

// CarShopSpec is cars.json "shop" (carshop.go).
type CarShopSpec struct {
	Swap       int              `json:"swap"`
	Boost      float64          `json:"boost"`      // s of boost a lap
	Tank       float64          `json:"tank"`       // + s a tank tier
	Plating    float64          `json:"plating"`    // + car HP and ram damage a tier
	ChargePack float64          `json:"chargePack"` // + charges a tier
	Upgrades   map[string][]int `json:"upgrades"`   // prices by tier
}

// CarItems is cars.json "equipment"; CarShop its "shop" (LoadCars).
var (
	CarItems []*CarItem
	CarShop  CarShopSpec
)

// CarItemOf is equipment id, or nil.
func CarItemOf(id string) *CarItem {
	for _, it := range CarItems {
		if it.ID == id {
			return it
		}
	}
	return nil
}

// carGear is a live shot (rocket, orb) or drop (spikes, oil, mine), in every
// full snapshot ("gear") to draw.
type carGear struct {
	ID    int          `json:"id" net:"cold"`
	Item  string       `json:"k" net:"cold"` // CarItem.ID
	Pos   Vec3         `json:"p" net:"hot,mm"`
	Vel   Vec3         `json:"v" net:"hot,mm"`
	Armed bool         `json:"a,omitempty" net:"hot"`
	it    *CarItem     `net:"-"`
	by    int          `net:"-"` // the player who fired or dropped it
	car   int          `net:"-"` // their car (spared until it is clear)
	age   float64      `net:"-"`
	hit   map[int]bool `net:"-"` // spikes, oil: the cars it got already
	clear bool         `net:"-"` // its own car got clear of it once: it counts for it too from then on
}

// gearJSON is the live gear for the snapshot ("gear":[...]; nothing without).
func (s *Server) gearJSON() []byte {
	if len(s.gear) == 0 {
		return nil
	}
	b, _ := json.Marshal(s.gear)
	return append([]byte(`"gear":`), b...)
}

// carEvent is an optional car event ("car": fire, boom, wreck, ram, slip).
type carEvent struct {
	T    string  `json:"t"`  // "car"
	Ev   string  `json:"ev"` // fire | boom | wreck | ram | slip | beam | pickup
	Item string  `json:"k,omitempty"`
	At   Vec3    `json:"p"`
	To   *Vec3   `json:"to,omitempty"` // the beam's end
	Car  int     `json:"car,omitempty"`
	By   int     `json:"by,omitempty"`
	R    float64 `json:"r,omitempty"`   // a blast's radius
	Via  string  `json:"via,omitempty"` // a pickup: "flag" when a Feature Flag turned out to be Item
}

// carState is the server's own per-car combat state.
type carState struct {
	front, rear string // equipment ids
	fn, rn      int    // charges left this lap
	nextFront   int64  // cmdTicks: the front item fires again
	nextRear    int64
	maxHP       int
	plating     int
	lastHitBy   int       // the player who last hurt it
	lastHitWith string    // with what
	wreckAt     time.Time // HP 0 since
	prevVel     sim.Vec3  // last tick's velocity (rams)
	rams        map[int]time.Time
	runOver     map[int]time.Time
	owner       int       // the player whose garage fitted it
	repair      float64   // a pit lane's repair, HP not yet whole
	shieldUntil time.Time // Code Freeze (pickups.go): no damage until then
	idleSince   time.Time // parkour: left alone away from its station since (parkour.go)
}

// fitCar fits car c for player p: their garage's car type, equipment,
// upgrades, a full tank and charges, repaired. Out of a race (no garage)
// the type's built-ins. Callers hold s.mu.
func (s *Server) fitCar(c *Car, p *Player) {
	g := s.garageOf(p)
	if g.Type != "" && g.Type != c.Type {
		if sp := CarSpecOf(g.Type); sp != nil {
			c.spec, c.Type = sp, sp.ID
			pos, yaw := c.sim.Pos, c.sim.Yaw
			c.sim = *sim.NewCar(sp, pos, yaw)
		}
	}
	cs := &c.cs
	cs.front, cs.rear = c.spec.Front, c.spec.Rear
	if g.Front != "" {
		cs.front = g.Front
	}
	if g.Rear != "" {
		cs.rear = g.Rear
	}
	cs.plating = g.Plating
	cs.maxHP = int(math.Round(float64(c.spec.HP) * (1 + CarShop.Plating*float64(g.Plating))))
	c.sim.HP, c.MaxHP = cs.maxHP, cs.maxHP
	c.sim.Engine, c.sim.Tyres = g.Engine, g.Tyres
	if c.spec.Drive == sim.DriveHover {
		c.sim.Tyres = 0
	}
	cs.owner = idOf(p)
	cs.wreckAt = time.Time{}
	s.refillCar(c, g)
	c.sync()
}

// refillCar: a lap (or a respawn) fills the charges and the boost tank (not
// the HP).
func (s *Server) refillCar(c *Car, g garage) {
	s.refillCharges(c, g)
	c.sim.Boost = carTank(g)
}

// carTank: a full boost tank for garage g (seconds).
func carTank(g garage) float64 { return CarShop.Boost + CarShop.Tank*float64(g.Tank) }

// refillCharges: the front and rear items' charges full (a lap, Reorg Budget).
func (s *Server) refillCharges(c *Car, g garage) {
	cs := &c.cs
	pack := 1 + CarShop.ChargePack*float64(g.Charges)
	cs.fn, cs.rn = 0, 0
	if it := CarItemOf(cs.front); it != nil {
		cs.fn = int(math.Round(float64(it.Charges) * pack))
	}
	if it := CarItemOf(cs.rear); it != nil {
		cs.rn = int(math.Round(float64(it.Charges) * pack))
	}
}

// carWeapons runs the driver's car items for command c (after the car moved).
func (s *Server) carWeapons(p *Player, car *Car, c Cmd) {
	cs := &car.cs
	if car.sim.HP <= 0 {
		return
	}
	if fi := CarItemOf(cs.front); fi != nil && fi.Kind != "ram" && c.B&BtnCarFront != 0 && cs.fn > 0 && p.cmdTicks >= cs.nextFront {
		cs.fn--
		cs.nextFront = p.cmdTicks + Ticks(fi.Interval)
		s.fireFront(p, car, fi)
	}
	if ri := CarItemOf(cs.rear); ri != nil && c.B&BtnCarRear != 0 && p.lastCmd.B&BtnCarRear == 0 && cs.rn > 0 && p.cmdTicks >= cs.nextRear {
		cs.rn--
		cs.nextRear = p.cmdTicks + Ticks(ri.Interval)
		s.dropRear(p, car, ri)
	}
}

// carNose is where car c's front items come out, and its heading (pitched).
func carNose(c *Car) (Vec3, Vec3) {
	sc := &c.sim
	hx, hz := sim.BoatHeading(sc.Yaw)
	cp := math.Cos(sc.Pitch)
	d := Vec3{hx * cp, math.Sin(sc.Pitch), hz * cp}
	l := c.spec.Size[2]/2 + 0.3
	return Vec3{sc.Pos[0] + hx*l, sc.Pos[1] + c.spec.Size[1]*0.6, sc.Pos[2] + hz*l}, d
}

// fireFront fires front item it of p's car.
func (s *Server) fireFront(p *Player, car *Car, it *CarItem) {
	o, d := carNose(car)
	switch it.Kind {
	case "beam":
		t, hitCar, hitP := s.carRay(o, d, it.Range, car, p)
		end := Vec3{o[0] + d[0]*t, o[1] + d[1]*t, o[2] + d[2]*t}
		if hitCar != nil {
			s.damageCar(hitCar, it.Damage, p, it.ID)
		} else if hitP != nil {
			s.hurt(p, hitP, it.Damage, it.ID, o)
		}
		s.sendAll(carEvent{T: "car", Ev: "beam", Item: it.ID, At: o, To: &end, Car: car.ID, By: p.ID})
	case "rocket", "orb":
		v := Vec3{d[0] * it.Speed, d[1] * it.Speed, d[2] * it.Speed}
		sv := car.sim.Vel // it leaves with the car's speed too (along the heading)
		if f := sv[0]*d[0] + sv[2]*d[2]; f > 0 {
			v[0], v[2] = v[0]+d[0]*f, v[2]+d[2]*f
		}
		s.addGear(&carGear{Item: it.ID, Pos: o, Vel: v, it: it, by: p.ID, car: car.ID, Armed: true})
		s.sendAll(carEvent{T: "car", Ev: "fire", Item: it.ID, At: o, Car: car.ID, By: p.ID})
	}
}

// dropRear drops rear item it behind p's car.
func (s *Server) dropRear(p *Player, car *Car, it *CarItem) {
	sc := &car.sim
	hx, hz := sim.BoatHeading(sc.Yaw)
	l := car.spec.Size[2]/2 + 0.6
	at := Vec3{sc.Pos[0] - hx*l, sc.Pos[1], sc.Pos[2] - hz*l}
	s.addGear(&carGear{Item: it.ID, Pos: at, it: it, by: p.ID, car: car.ID, hit: map[int]bool{}})
	s.sendAll(carEvent{T: "car", Ev: "fire", Item: it.ID, At: at, Car: car.ID, By: p.ID})
}

// addGear adds g with a new id (the oldest goes past carMaxGear).
func (s *Server) addGear(g *carGear) {
	s.gearSeq++
	g.ID = s.gearSeq
	if len(s.gear) >= carMaxGear {
		s.gear = s.gear[1:]
	}
	s.gear = append(s.gear, g)
}

// carHalf is car c's oriented box: centre, half sizes (right, up, ahead), axes.
func carBox(c *Car) (ctr Vec3, half Vec3, right, fwd Vec3) {
	sc := &c.sim
	hx, hz := sim.BoatHeading(sc.Yaw)
	return Vec3{sc.Pos[0], sc.Pos[1] + c.spec.Size[1]/2, sc.Pos[2]}, Vec3{c.spec.Size[0] / 2, c.spec.Size[1] / 2, c.spec.Size[2] / 2}, Vec3{-hz, 0, hx}, Vec3{hx, 0, hz}
}

// carRayT is where the ray o + t*d (d a unit vector) enters car c's box, or
// +Inf (yaw only: the box doesn't pitch or roll).
func carRayT(c *Car, o, d Vec3) float64 {
	ctr, half, right, fwd := carBox(c)
	rel := Vec3{o[0] - ctr[0], o[1] - ctr[1], o[2] - ctr[2]}
	lo := [3]float64{rel[0]*right[0] + rel[2]*right[2], rel[1], rel[0]*fwd[0] + rel[2]*fwd[2]}
	ld := [3]float64{d[0]*right[0] + d[2]*right[2], d[1], d[0]*fwd[0] + d[2]*fwd[2]}
	t0, t1 := 0.0, math.Inf(1)
	for a := range 3 {
		if math.Abs(ld[a]) < 1e-12 {
			if lo[a] < -half[a] || lo[a] > half[a] {
				return math.Inf(1)
			}
			continue
		}
		ta, tb := (-half[a]-lo[a])/ld[a], (half[a]-lo[a])/ld[a]
		if ta > tb {
			ta, tb = tb, ta
		}
		t0, t1 = math.Max(t0, ta), math.Min(t1, tb)
		if t0 > t1 {
			return math.Inf(1)
		}
	}
	return t0
}

// inCarBox: point q is in car c's box grown by m.
func inCarBox(c *Car, q Vec3, m float64) bool {
	ctr, half, right, fwd := carBox(c)
	rel := Vec3{q[0] - ctr[0], q[1] - ctr[1], q[2] - ctr[2]}
	x, z := rel[0]*right[0]+rel[2]*right[2], rel[0]*fwd[0]+rel[2]*fwd[2]
	return math.Abs(x) <= half[0]+m && math.Abs(rel[1]) <= half[1]+m && math.Abs(z) <= half[2]+m
}

// carRay traces a car item's ray from o along d up to max: the map, the cars
// (but skip) and the players (but the shooter by). Returns how far it got and
// what it hit (a car or a player; neither: a wall or nothing).
func (s *Server) carRay(o, d Vec3, max float64, skip *Car, by *Player) (float64, *Car, *Player) {
	lo, hi := MapBounds(s.Map.Boxes)
	best := WallStop(WallTrace(s.Map.Boxes, lo, hi, o, d, max, 0), max)
	var hc *Car
	var hp *Player
	for _, c := range s.cars {
		if c == skip {
			continue
		}
		if t := carRayT(c, o, d); t < best {
			best, hc = t, c
		}
	}
	for _, q := range s.ordered() {
		if q == by || q.Dead || !q.ready || q.Team == content.Spectator || q.body == nil {
			continue
		}
		if _, t, ok := RayModel(&PlayerModel, poseOf(q, s.poseTime()), o, d); ok && t < best {
			best, hc, hp = t, nil, q
		}
	}
	return best, hc, hp
}

// bulletCar: a bullet from o along d that gets to wall (or a body at t) may
// hit a car's body first (not the shooter's own). Returns the car and where.
func (s *Server) bulletCar(p *Player, o, d Vec3, wall float64) (*Car, float64) {
	var hc *Car
	best := wall
	for _, c := range s.cars {
		if c.ID == p.car {
			continue
		}
		if t := carRayT(c, o, d); t < best {
			best, hc = t, c
		}
	}
	return hc, best
}

// damageCar takes dmg off car c's HP (by: who, with: the weapon or item). At
// 0 it is wrecked: its driver dies, killed by `by`. Callers hold s.mu.
func (s *Server) damageCar(c *Car, dmg float64, by *Player, with string) {
	if c.sim.HP <= 0 || dmg <= 0 || s.shielded(c) || by != nil && s.raceMode() && !s.raceCombat() {
		return // wrecked already, nothing, a Code Freeze, or a race without combat
	}
	c.sim.HP -= max(1, int(math.Round(dmg)))
	if by != nil {
		c.cs.lastHitBy, c.cs.lastHitWith = by.ID, with
		if hm, err := json.Marshal(map[string]any{"t": "carhit", "car": c.ID, "w": with, "dmg": math.Round(dmg), "hp": max(c.sim.HP, 0)}); err == nil {
			trySend(by, hm)
		}
	}
	if c.sim.HP <= 0 {
		s.wreckCar(c, by, with)
	}
	c.sync()
}

// wreckCar: car c at 0 HP. Its driver dies (killed by `by`).
func (s *Server) wreckCar(c *Car, by *Player, with string) {
	c.sim.HP = 0
	c.cs.wreckAt = s.clock()
	s.sendAll(carEvent{T: "car", Ev: "wreck", At: Vec3(c.sim.Pos), Car: c.ID})
	if d := s.players[c.Driver]; d != nil && !d.Dead && d.car == c.ID {
		if by == d {
			by = nil // your own blast: no kill to anyone
		}
		s.killBy(by, d, with, false)
	}
}

// hurt deals a car item's damage to a body (armor doesn't count: it is a
// car's gun). from: where it came from. Callers hold s.mu.
func (s *Server) hurt(by, q *Player, dmg float64, with string, from Vec3) {
	if q.Dead || dmg <= 0 {
		return
	}
	if s.spared(by, q) {
		return // a race without combat (mp_race_combat 0)
	}
	d := max(1, int(math.Round(dmg)))
	if by != nil && by != q {
		s.noteDamage(by, q, d, q.HP)
		hm, _ := json.Marshal(hitMsg{T: "hit", ID: q.ID, Name: q.Name, Part: "body", Weapon: with, Damage: float64(d), At: q.Pos})
		trySend(by, hm)
	}
	if q.Bot {
		return
	}
	q.HP -= d
	hb, _ := json.Marshal(map[string]any{"t": "hurt", "dmg": d, "from": idOf(by), "hp": max(q.HP, 0), "fp": from})
	trySend(q, hb)
	if q.HP <= 0 {
		if by == q {
			by = nil
		}
		s.killBy(by, q, with, false)
	}
}

func idOf(p *Player) int {
	if p == nil {
		return 0
	}
	return p.ID
}

// carBlast: a car item goes off at `at` (radius r, damage dmg at its middle,
// knock: m/s up for the cars in it). The cars and the bodies on foot in it
// take damage by distance (the driver is in the car: the car takes it).
func (s *Server) carBlast(at Vec3, it *CarItem, by *Player) {
	r := it.Radius
	s.sendAll(carEvent{T: "car", Ev: "boom", Item: it.ID, At: at, By: idOf(by), R: r})
	for _, c := range s.cars {
		ctr, _, _, _ := carBox(c)
		d := math.Max(0, math.Hypot(math.Hypot(ctr[0]-at[0], ctr[1]-at[1]), ctr[2]-at[2])-c.spec.Size[0]/2)
		if d > r {
			continue
		}
		k := 1 - d/r
		s.damageCar(c, it.Damage*math.Max(0.3, k), by, it.ID)
		if it.Knock > 0 {
			c.sim.Vel[1] += it.Knock * k
			c.sim.OnGround = false
		}
	}
	for _, q := range s.ordered() {
		if q.Dead || !q.ready || q.Team == content.Spectator || q.car != 0 || q.body == nil {
			continue
		}
		d := math.Hypot(math.Hypot(q.Pos[0]-at[0], q.Pos[1]+0.9-at[1]), q.Pos[2]-at[2])
		if d > r || !s.lineClear(at, Vec3{q.Pos[0], q.Pos[1] + 0.9, q.Pos[2]}) {
			continue
		}
		s.hurt(by, q, it.Damage*0.8*(1-d/r), it.ID, at)
	}
}

// tickGear flies the shots and runs the drops. Callers hold s.mu.
func (s *Server) tickGear(dt float64) {
	if len(s.gear) == 0 {
		return
	}
	keep := s.gear[:0]
	for _, g := range s.gear {
		g.age += dt
		if s.stepGear(g, dt) {
			keep = append(keep, g)
		}
	}
	clear(s.gear[len(keep):])
	s.gear = keep
}

// stepGear runs one shot or drop for dt; false: it is gone.
func (s *Server) stepGear(g *carGear, dt float64) bool {
	it, by := g.it, s.players[g.by]
	switch it.Kind {
	case "rocket", "orb":
		if g.age > carShotLife {
			return false
		}
		if it.Kind == "orb" {
			s.homeOrb(g, dt)
		}
		sp := math.Hypot(math.Hypot(g.Vel[0], g.Vel[1]), g.Vel[2])
		if sp < 1e-6 {
			return false
		}
		d := Vec3{g.Vel[0] / sp, g.Vel[1] / sp, g.Vel[2] / sp}
		step := sp * dt
		var skip *Car
		if g.age < 0.25 {
			skip = s.carByID(g.car) // out of its own car's nose first
		}
		t, _, _ := s.carRay(g.Pos, d, step, skip, by)
		if t < step || s.pitAt(g.Pos) != nil {
			at := Vec3{g.Pos[0] + d[0]*t, g.Pos[1] + d[1]*t, g.Pos[2] + d[2]*t}
			s.carBlast(at, it, by)
			return false
		}
		g.Pos = Vec3{g.Pos[0] + g.Vel[0]*dt, g.Pos[1] + g.Vel[1]*dt, g.Pos[2] + g.Vel[2]*dt}
		return true
	}
	// A drop: armed after carDropArm (its own car spared until then), gone after its life.
	if g.age > it.Life {
		return false
	}
	g.Armed = g.age >= carDropArm
	reach := math.Max(it.Trigger, it.Radius)
	if own := s.carByID(g.car); !g.clear && (own == nil || !inCarBox(own, g.Pos, reach)) {
		g.clear = true
	}
	if it.Seek > 0 && g.Armed { // the sticky mine creeps to the nearest car near it
		if c := s.nearestCar(g.Pos, it.Seek, s.carByID(g.car)); c != nil { // (never its own)
			dx, dz := c.sim.Pos[0]-g.Pos[0], c.sim.Pos[2]-g.Pos[2]
			if l := math.Hypot(dx, dz); l > 1e-6 {
				g.Vel = Vec3{dx / l * it.Speed, 0, dz / l * it.Speed}
				g.Pos[0] += g.Vel[0] * dt
				g.Pos[2] += g.Vel[2] * dt
			}
		} else {
			g.Vel = Vec3{}
		}
	}
	for _, c := range s.cars {
		if !g.Armed || (c.ID == g.car && !g.clear) {
			continue // its own car is spared until it got clear of it
		}
		switch it.Kind {
		case "mine":
			if inCarBox(c, g.Pos, it.Trigger-c.spec.Size[0]/2) {
				s.carBlast(Vec3{g.Pos[0], g.Pos[1] + 0.3, g.Pos[2]}, it, by)
				return false
			}
		case "spikes", "oil":
			if g.hit[c.ID] || c.spec.Drive == sim.DriveHover || !c.sim.OnGround || !inCarBox(c, g.Pos, it.Radius-c.spec.Size[0]/2) {
				continue
			}
			g.hit[c.ID] = true
			c.sim.Slip = it.Slip
			if it.Spin > 0 { // oil: it spins out (the way it was turning, else left)
				dir := 1.0
				if c.sim.YawVel < 0 {
					dir = -1
				}
				c.sim.YawVel += dir * it.Spin
			}
			if by != nil && by.car != c.ID {
				c.cs.lastHitBy, c.cs.lastHitWith = by.ID, it.ID
			}
			s.sendAll(carEvent{T: "car", Ev: "slip", Item: it.ID, At: g.Pos, Car: c.ID, By: g.by})
		}
	}
	return true
}

// homeOrb turns an orb toward the nearest driven car ahead of it within its
// seek range (not its own), at its turn rate.
func (s *Server) homeOrb(g *carGear, dt float64) {
	sp := math.Hypot(g.Vel[0], g.Vel[2])
	if sp < 1e-6 {
		return
	}
	hx, hz := g.Vel[0]/sp, g.Vel[2]/sp
	var best *Car
	bd := g.it.Seek
	for _, c := range s.cars {
		if c.ID == g.car || c.sim.HP <= 0 || c.Driver == 0 { // a driven car (not a parked one)
			continue
		}
		dx, dz := c.sim.Pos[0]-g.Pos[0], c.sim.Pos[2]-g.Pos[2]
		d := math.Hypot(dx, dz)
		if d < bd && d > 1e-6 && (dx*hx+dz*hz)/d > 0.3 { // ahead (within ~70 degrees)
			best, bd = c, d
		}
	}
	if best == nil {
		return
	}
	want := math.Atan2(best.sim.Pos[2]-g.Pos[2], best.sim.Pos[0]-g.Pos[0])
	cur := math.Atan2(hz, hx)
	turn := math.Max(-g.it.Turn*dt, math.Min(g.it.Turn*dt, math.Remainder(want-cur, 2*math.Pi)))
	a := cur + turn
	g.Vel[0], g.Vel[2] = math.Cos(a)*sp, math.Sin(a)*sp
	ty := best.sim.Pos[1] + best.spec.Size[1]/2 - g.Pos[1] // and up or down to it
	g.Vel[1] = math.Max(-sp/3, math.Min(sp/3, ty*2))
}

// nearestCar is the nearest driven car to p within r (not skip), or nil.
func (s *Server) nearestCar(p Vec3, r float64, skip *Car) *Car {
	var best *Car
	for _, c := range s.cars {
		if c == skip || c.sim.HP <= 0 || c.Driver == 0 {
			continue
		}
		if d := math.Hypot(c.sim.Pos[0]-p[0], c.sim.Pos[2]-p[2]); d < r && math.Abs(c.sim.Pos[1]-p[1]) < 3 {
			best, r = c, d
		}
	}
	return best
}

// tickRams: cars that met at speed since the last tick take damage, and so
// do bodies on foot a car ran into. Then every car's velocity is kept for the
// next tick. Callers hold s.mu.
func (s *Server) tickRams(now time.Time) {
	for i, a := range s.cars {
		for _, b := range s.cars[i+1:] {
			s.ram(now, a, b)
		}
		s.runOver(now, a)
	}
	for _, c := range s.cars {
		c.cs.prevVel = c.sim.Vel
	}
}

// ram: cars a and b touching after closing at speed (last tick's velocities).
func (s *Server) ram(now time.Time, a, b *Car) {
	dx, dz := b.sim.Pos[0]-a.sim.Pos[0], b.sim.Pos[2]-a.sim.Pos[2]
	if math.Abs(a.sim.Pos[1]-b.sim.Pos[1]) > 1.5 {
		return
	}
	var da, db []sim.CarDisc
	da, db = sim.CarDiscs(a.spec, &a.sim, da), sim.CarDiscs(b.spec, &b.sim, db)
	touch := false
	for _, p := range da {
		for _, q := range db {
			if math.Hypot(p.X-q.X, p.Z-q.Z) < p.R+q.R+carRamGap {
				touch = true
			}
		}
	}
	if !touch {
		return
	}
	l := math.Hypot(dx, dz)
	if l < 1e-6 {
		return
	}
	nx, nz := dx/l, dz/l
	closing := (a.cs.prevVel[0]-b.cs.prevVel[0])*nx + (a.cs.prevVel[2]-b.cs.prevVel[2])*nz
	if closing < carRamMin {
		return
	}
	if a.cs.rams == nil {
		a.cs.rams = map[int]time.Time{}
	}
	if t, ok := a.cs.rams[b.ID]; ok && now.Sub(t) < secs(carRamEvery) {
		return
	}
	a.cs.rams[b.ID] = now
	base := (closing - carRamMin) * carRamK
	ma, mb := a.spec.Mass, b.spec.Mass
	// Each takes the other's share by mass, times the other's ram factor.
	s.damageCar(a, base*2*mb/(ma+mb)*s.ramFactor(b), s.players[b.Driver], "ram")
	s.damageCar(b, base*2*ma/(ma+mb)*s.ramFactor(a), s.players[a.Driver], "ram")
	s.sendAll(carEvent{T: "car", Ev: "ram", At: Vec3{(a.sim.Pos[0] + b.sim.Pos[0]) / 2, a.sim.Pos[1] + 0.5, (a.sim.Pos[2] + b.sim.Pos[2]) / 2}, Car: a.ID, By: b.ID})
}

// ramFactor: how hard car c rams: plating, the ram plate.
func (s *Server) ramFactor(c *Car) float64 {
	k := 1 + CarShop.Plating*float64(c.cs.plating)
	if it := CarItemOf(c.cs.front); it != nil && it.Kind == "ram" {
		k *= it.Ram
	}
	return k
}

// runOver: car c moving fast into a body on foot hurts it and throws it.
func (s *Server) runOver(now time.Time, c *Car) {
	v := math.Hypot(c.cs.prevVel[0], c.cs.prevVel[2])
	if v < carRunOverMin {
		return
	}
	driver := s.players[c.Driver]
	for _, q := range s.ordered() {
		if q.Dead || !q.ready || q.Team == content.Spectator || q.car != 0 || q.body == nil || q.ride != 0 {
			continue
		}
		mid := Vec3{q.Pos[0], q.Pos[1] + 0.8, q.Pos[2]}
		if !inCarBox(c, mid, sim.R+0.15) {
			continue
		}
		if c.cs.runOver == nil {
			c.cs.runOver = map[int]time.Time{}
		}
		if t, ok := c.cs.runOver[q.ID]; ok && now.Sub(t) < secs(1) {
			continue
		}
		c.cs.runOver[q.ID] = now
		hx, hz := c.cs.prevVel[0]/v, c.cs.prevVel[2]/v
		q.body.Vel = sim.Vec3{hx * carRunOverKick, carRunOverKick * 0.6, hz * carRunOverKick}
		q.body.OnGround = false
		s.hurt(driver, q, (v-carRunOverMin+3)*carRunOverK, "ram", Vec3(c.sim.Pos))
	}
}

// tickWrecks: out of a race a wrecked car goes home repaired after
// carWreckSecs (a race's wreck comes back with its racer: race.go).
// Callers hold s.mu.
func (s *Server) tickWrecks(now time.Time) {
	if s.raceMode() {
		return
	}
	for _, c := range s.cars {
		if c.sim.HP <= 0 && !c.cs.wreckAt.IsZero() && now.Sub(c.cs.wreckAt) >= secs(carWreckSecs) {
			s.carHome(c)
			c.sim.HP = max(c.MaxHP, c.spec.HP)
			c.cs.wreckAt = time.Time{}
			c.sync()
		}
	}
}

// tickPits: a car (not a wreck) in a pit lane is repaired, carPitRepair HP a
// second, up to its max. Callers hold s.mu.
func (s *Server) tickPits(dt float64) {
	if len(s.Map.PitLanes) == 0 {
		return
	}
	for _, c := range s.cars {
		cs := &c.cs
		if c.sim.HP <= 0 || c.sim.HP >= c.MaxHP || !s.inPitLane(Vec3(c.sim.Pos)) {
			cs.repair = 0
			continue
		}
		cs.repair += carPitRepair * dt
		if n := int(cs.repair); n > 0 {
			cs.repair -= float64(n)
			c.sim.HP = min(c.MaxHP, c.sim.HP+n)
		}
	}
}

// inPitLane: pos is in one of the map's pit lanes.
func (s *Server) inPitLane(pos Vec3) bool {
	for i := range s.Map.PitLanes {
		if s.Map.PitLanes[i].Contains([3]float64(pos)) {
			return true
		}
	}
	return false
}
