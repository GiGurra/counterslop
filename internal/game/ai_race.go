package game

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// Bots that race (race mode, docs/racing.md phase 5). A racing bot drives
// the map's racing line (content.Map.RaceLine: a spline through hand-placed
// points; without one, through the gates) with the same commands a player's
// keys make: it steers for a point ahead on the line (further ahead the
// faster it goes), plans its speed from the curves ahead (a curve allows the
// speed the car's turn rate can follow, braking for it in time), boosts on
// straights, fires its front item at a car in its sights, drops its rear item
// on a car close behind and shoots its gun at enemies near it. Stuck (on a
// rail, nose in a wall) it backs off; a wreck puts it back on the line at the
// gate it comes back at. Everything is a function of the state and the tick:
// a race with bots replays the same.

// aiRace is the bot's driving state (aiBrain.race).
type aiRace struct {
	key   string // the line it tracks progress on (raceLine.key)
	i     int    // its place on the line: the index of the nearest point
	stuck int    // ticks it has been barely moving with the throttle on
	back  int    // ticks left backing off (stuck)
	steer float64
}

// raceLine is a map's racing line sampled every raceLineStep m along a
// closed Catmull-Rom spline: points, distance along the loop, curvature.
type raceLine struct {
	key string
	pts []Vec3
	cum []float64 // distance from pts[0] to pts[i]
	k   []float64 // curvature (1/m) at pts[i] (xz)
	len float64   // the loop's length
}

const (
	raceLineStep   = 1.5  // m between sampled points
	raceSearchBack = 4    // points behind its place a bot looks for itself
	raceSearchFwd  = 40   // and ahead
	raceLost       = 14.0 // m off the line: it searches the whole line (a wreck, a jump gone wrong)
	raceStuckSpeed = 1.5  // m/s: under this with the throttle on, it is stuck
	raceStuckTicks = SimHz * 3 / 2
	raceBackTicks  = SimHz * 6 / 5
	raceSteerK     = 2.6 // steering per radian of heading error
	raceFrontRange = 60.0
	raceRearRange  = 22.0
	raceGunRange   = 25.0
	raceGunErr     = 3.0  // a driving bot's aim error: this many times its skill's standing one
	raceSlideMax   = 0.22 // rad between the nose and the velocity: sliding
	raceJumpLook   = 30.0 // m ahead a ramp up counts as a jump coming
	raceJumpRise   = 1.0  // m the line climbs within raceJumpLook: a jump's ramp
	raceJumpSpeed  = 0.85 // the share of top speed a bot takes a jump at
	raceGunGrace   = 5.0  // s after GO before a bot shoots (no massacre on the grid)
)

// raceCornerMargin: the share of the car's full turn rate a bot plans corners
// with, per drive (a hover slides, so it takes them slower).
var raceCornerMargin = map[string]float64{"wheels": 0.55, "tracks": 0.6, "hover": 0.5}

// raceGripAccel: the sideways acceleration (m/s²) a bot plans corners with,
// per unit of the car's grip (sideways speed taken away a second): a car
// that slides (the hovercraft, grip 1.2) carries its speed out of a corner
// however fast its nose comes round, so its grip, not its turn rate, sets
// how fast it can take one.
const raceGripAccel = 8.0

// raceSkill: the share of its top speed a bot drives at, by difficulty.
var raceSkill = [4]float64{0.8, 0.88, 0.95, 1}

// raceLineOf is the current map's racing line (built once per map), nil on a
// map with fewer than 3 points to drive through.
func (s *Server) raceLineOf() *raceLine {
	ctrl := make([]Vec3, 0, len(s.Map.RaceLine))
	for _, p := range s.Map.RaceLine {
		ctrl = append(ctrl, Vec3(p))
	}
	if len(ctrl) == 0 { // no line drawn: gate to gate
		for _, g := range s.Map.Checkpoints {
			ctrl = append(ctrl, Vec3{(g.Min[0] + g.Max[0]) / 2, g.Min[1] + 1, (g.Min[2] + g.Max[2]) / 2})
		}
	}
	if len(ctrl) < 3 {
		return nil
	}
	key := s.Map.Name + "|" + itoa(len(ctrl))
	if s.aiRaceLine != nil && s.aiRaceLine.key == key {
		return s.aiRaceLine
	}
	s.aiRaceLine = buildRaceLine(key, ctrl)
	return s.aiRaceLine
}

// buildRaceLine samples the closed Catmull-Rom spline through ctrl.
func buildRaceLine(key string, ctrl []Vec3) *raceLine {
	n := len(ctrl)
	l := &raceLine{key: key}
	for i := range n {
		p0, p1, p2, p3 := ctrl[(i+n-1)%n], ctrl[i], ctrl[(i+1)%n], ctrl[(i+2)%n]
		seg := math.Hypot(p2[0]-p1[0], p2[2]-p1[2])
		steps := max(1, int(math.Ceil(seg/raceLineStep)))
		for k := range steps {
			t := float64(k) / float64(steps)
			l.pts = append(l.pts, catmullRom(p0, p1, p2, p3, t))
		}
	}
	m := len(l.pts)
	l.cum = make([]float64, m)
	for i := 1; i < m; i++ {
		l.cum[i] = l.cum[i-1] + xzDist(l.pts[i-1], l.pts[i])
	}
	l.len = l.cum[m-1] + xzDist(l.pts[m-1], l.pts[0])
	l.k = make([]float64, m)
	for i := range m {
		a, b, c := l.pts[(i+m-3)%m], l.pts[i], l.pts[(i+3)%m]
		h1, h2 := math.Atan2(b[0]-a[0], b[2]-a[2]), math.Atan2(c[0]-b[0], c[2]-b[2])
		d := xzDist(a, b) + xzDist(b, c)
		if d > 1e-6 {
			l.k[i] = math.Abs(angleDiff(h2, h1)) / (d / 2)
		}
	}
	return l
}

func catmullRom(p0, p1, p2, p3 Vec3, t float64) Vec3 {
	t2, t3 := t*t, t*t*t
	var o Vec3
	for i := range 3 {
		o[i] = 0.5 * (2*p1[i] + (-p0[i]+p2[i])*t + (2*p0[i]-5*p1[i]+4*p2[i]-p3[i])*t2 + (-p0[i]+3*p1[i]-3*p2[i]+p3[i])*t3)
	}
	return o
}

func xzDist(a, b Vec3) float64 { return math.Hypot(b[0]-a[0], b[2]-a[2]) }

// lineCost: how far pos is from line point q (height counts more: the road
// above or below on a crossover is not this one).
func lineCost(pos, q Vec3) float64 {
	dx, dy, dz := q[0]-pos[0], q[1]-pos[1], q[2]-pos[2]
	return dx*dx + dz*dz + 9*dy*dy
}

// place: the nearest point to pos, near from (or anywhere if it is lost).
func (l *raceLine) place(pos Vec3, from int, known bool) int {
	m := len(l.pts)
	if known && from >= 0 && from < m && math.Sqrt(lineCost(pos, l.pts[from])) < raceLost {
		best, bi := math.Inf(1), from
		for d := -raceSearchBack; d <= raceSearchFwd; d++ {
			j := (from + d + m) % m
			if c := lineCost(pos, l.pts[j]); c < best {
				best, bi = c, j
			}
		}
		return bi
	}
	best, bi := math.Inf(1), 0
	for j, q := range l.pts {
		if c := lineCost(pos, q); c < best {
			best, bi = c, j
		}
	}
	return bi
}

// ahead: the point d m along the line from i.
func (l *raceLine) ahead(i int, d float64) int {
	m := len(l.pts)
	j := i
	for walked := 0.0; walked < d; {
		n := (j + 1) % m
		walked += xzDist(l.pts[j], l.pts[n])
		j = n
		if j == i {
			break
		}
	}
	return j
}

// aiRace is a racing bot's command for this tick (aiThink in race mode).
func (s *Server) aiRace(p *Player) Cmd {
	b := p.ai
	c := Cmd{Yaw: p.body.Yaw, Pitch: 0}
	if s.round.Phase == PhaseFreeze && b.round != s.round.Number {
		b.round = s.round.Number
		s.aiRaceBuy(p)
	}
	car := s.carByID(p.car)
	l := s.raceLineOf()
	if car == nil || l == nil || !s.raceLive() || s.racer(p) == nil {
		b.race.stuck, b.race.back = 0, 0
		return c // on the grid (the countdown holds the cars), or not racing
	}
	sc := &car.sim
	pos := Vec3(sc.Pos)
	rs := &b.race
	rs.i = l.place(pos, rs.i, rs.key == l.key)
	rs.key = l.key
	hx, hz := sim.BoatHeading(sc.Yaw)
	v := sc.Vel[0]*hx + sc.Vel[2]*hz // ahead
	speed := math.Abs(v)
	sp := car.spec

	// Steering: for a point ahead on the line, further at speed.
	t := l.pts[l.ahead(rs.i, 4+0.45*speed)]
	if q, ok := s.raceDetour(p, car, l, rs.i, pos, hx, hz); ok {
		t = q // a pickup worth it, just off the line ahead: through it
	}
	want := math.Atan2(-(t[0] - pos[0]), -(t[2] - pos[2]))
	errYaw := angleDiff(want, sc.Yaw) // > 0: the line is to the left (yaw grows turning left)
	steer := math.Max(-1, math.Min(1, -errYaw*raceSteerK))

	// Speed: what the curves ahead allow, braking in time for them.
	margin := raceCornerMargin[sp.Drive]
	if margin == 0 {
		margin = 0.5
	}
	top := sp.TopSpeed * raceSkill[s.aiDifficulty()]
	brake := sp.Brake * 0.6
	horizon := speed*speed/(2*brake) + 12
	vt := top
	for j, d := rs.i, 0.0; d < horizon; {
		if k := l.k[j]; k > 1e-4 {
			lim := math.Min(sp.TurnRate*margin/k, math.Sqrt(sp.Grip*raceGripAccel/k))
			vt = math.Min(vt, math.Sqrt(lim*lim+2*brake*d))
		}
		n := (j + 1) % len(l.pts)
		d += xzDist(l.pts[j], l.pts[n])
		j = n
		if j == rs.i {
			break
		}
	}
	vt = math.Max(vt, 4)        // never stop for a curve
	if math.Abs(errYaw) > 1.2 { // facing well off the line: slow, turn
		vt = math.Min(vt, 6)
	}
	if l.climbs(rs.i, raceJumpLook) { // a jump ahead (the line climbs a ramp): its speed, whatever else
		vt = math.Max(vt, top*raceJumpSpeed)
	} else if speed > 8 && math.Abs(angleDiff(math.Atan2(-sc.Vel[0], -sc.Vel[2]), sc.Yaw)) > raceSlideMax {
		// Sliding (the nose well off where it is going, a hovercraft out of a
		// corner): off the throttle and braking until the slide settles.
		vt = math.Min(vt, speed-2)
	}
	thr := 0.0
	switch {
	case v < vt-0.5:
		thr = 1
	case v > vt+1:
		thr = -1
	default:
		thr = 0.3
	}

	// Stuck: barely moving with the throttle on: back off, steering the other way.
	if rs.back > 0 {
		rs.back--
		thr, steer = -1, -rs.steer
	} else if thr > 0 && speed < raceStuckSpeed {
		if rs.stuck++; rs.stuck > raceStuckTicks {
			rs.stuck, rs.back, rs.steer = 0, raceBackTicks, steer
		}
	} else {
		rs.stuck = 0
	}
	c.F, c.R = thr, steer

	// Boost on a straight: at full throttle with nothing to brake for in the next 50 m.
	if sc.Boost > 0 && thr == 1 && rs.back == 0 && speed > 8 && vt >= top-0.5 && math.Abs(errYaw) < 0.15 && s.lineStraight(l, rs.i, 50) {
		c.B |= BtnBoost
	}
	if !s.aiPeace {
		s.aiRaceItems(p, car, &c)
		s.aiRaceGun(p, car, &c)
	}
	return c
}

// climbs: the line rises raceJumpRise within d m ahead of i (a jump's ramp;
// the interchange's too, where keeping the speed up does no harm).
func (l *raceLine) climbs(i int, d float64) bool {
	y0 := l.pts[i][1]
	for j, walked := i, 0.0; walked < d; {
		if l.pts[j][1]-y0 > raceJumpRise {
			return true
		}
		n := (j + 1) % len(l.pts)
		walked += xzDist(l.pts[j], l.pts[n])
		j = n
	}
	return false
}

// lineStraight: no curve ahead of i for d m that needs braking at top speed.
func (s *Server) lineStraight(l *raceLine, i int, d float64) bool {
	for j, walked := i, 0.0; walked < d; {
		if l.k[j] > 0.02 {
			return false
		}
		n := (j + 1) % len(l.pts)
		walked += xzDist(l.pts[j], l.pts[n])
		j = n
	}
	return true
}

// raceRival: the nearest other racer's car in the cone around dir (cos of
// the half angle), within rng m and |dy| < 3, in sight; nil if none.
func (s *Server) raceRival(p *Player, car *Car, dir Vec3, cosHalf, rng float64) *Car {
	o := Vec3(car.sim.Pos)
	var best *Car
	bd := rng
	for _, rs := range s.round.Racers {
		if rs.ID == p.ID || rs.Car == 0 {
			continue
		}
		q := s.carByID(rs.Car)
		if q == nil || q.sim.HP <= 0 {
			continue
		}
		qp := Vec3(q.sim.Pos)
		d := Vec3{qp[0] - o[0], qp[1] - o[1], qp[2] - o[2]}
		dist := math.Hypot(d[0], d[2])
		if dist < 1e-3 || dist > bd || math.Abs(d[1]) > 3 {
			continue
		}
		if (d[0]*dir[0]+d[2]*dir[2])/dist < cosHalf {
			continue
		}
		up := Vec3{o[0], o[1] + 1, o[2]}
		if !s.lineClear(up, Vec3{qp[0], qp[1] + 1, qp[2]}) {
			continue
		}
		best, bd = q, dist
	}
	return best
}

// aiRaceItems: the front item at a car in its sights, the rear one on a car
// close behind (on the button's down edge, as a player presses it).
func (s *Server) aiRaceItems(p *Player, car *Car, c *Cmd) {
	cs := &car.cs
	hx, hz := sim.BoatHeading(car.sim.Yaw)
	fwd := Vec3{hx, 0, hz}
	if fi := CarItemOf(cs.front); fi != nil && fi.Kind != "ram" && cs.fn > 0 {
		cone, rng := 0.995, math.Min(fi.Range, raceFrontRange) // a beam or a rocket: along the heading
		if fi.Kind == "orb" {
			cone, rng = 0.5, raceFrontRange // orbs home
		}
		if rng <= 0 {
			rng = raceFrontRange
		}
		if s.raceRival(p, car, fwd, cone, rng) != nil {
			c.B |= BtnCarFront
		}
	}
	if ri := CarItemOf(cs.rear); ri != nil && cs.rn > 0 && p.lastCmd.B&BtnCarRear == 0 {
		if s.raceRival(p, car, Vec3{-hx, 0, -hz}, 0.85, raceRearRange) != nil {
			c.B |= BtnCarRear
		}
	}
}

// aiRaceGun: the driver's gun at the nearest enemy racer near it (the view is
// free while driving: the car steers by the command's R), with the bot's aim
// error; otherwise it looks along the car.
func (s *Server) aiRaceGun(p *Player, car *Car, c *Cmd) {
	b := p.ai
	sk := aiSkill[s.aiDifficulty()]
	c.Yaw, c.Pitch = car.sim.Yaw, 0
	var tgt *Player
	best := raceGunRange
	if s.round.started.IsZero() || s.clock().Sub(s.round.started).Seconds() < raceGunGrace {
		best = 0 // the start: everyone gets away first
	}
	for _, q := range s.ordered() {
		if q == p || q.Dead || q.body == nil || q.Team == p.Team || teamIdx(q.Team) < 0 || s.racer(q) == nil {
			continue
		}
		d, ok := s.visible(p, q, 2*math.Pi)
		if ok && d < best {
			tgt, best = q, d
		}
	}
	if tgt == nil {
		b.target, b.lastFire = 0, false
		b.yaw = c.Yaw
		return
	}
	if b.target != tgt.ID {
		b.target, b.seenAt = tgt.ID, s.tick
		e := sk.err * raceGunErr
		b.aimErr = [2]float64{b.r.Range(-1, 1) * e * 3, b.r.Range(-1, 1) * e * 2}
	}
	o, to := s.eyeOf(p), above(tgt, 1.0)
	d := Vec3{to[0] - o[0], to[1] - o[1], to[2] - o[2]}
	yaw := math.Atan2(-d[0], -d[2]) + b.aimErr[0]
	pitch := math.Atan2(d[1], math.Hypot(d[0], d[2])) + b.aimErr[1]
	b.turnTo(yaw, pitch, sk.turn)
	c.Yaw, c.Pitch = b.yaw, b.pitch
	b.aimErr[0] *= 0.995 // it settles slowly: both hands on the wheel
	b.aimErr[1] *= 0.995
	held := p.inv.Slots[p.inv.Current]
	if held != nil && held.Ammo > 0 && s.tick-b.seenAt >= sk.react && math.Abs(angleDiff(yaw, b.yaw)) < 0.05 {
		fire := true
		if w := s.Weapons.Get(held.ID); w != nil && w.Action != "auto" {
			fire = !b.lastFire // semi/bolt: a fresh press each shot
		}
		if fire {
			c.B |= BtnFire
			c.VT = float64(s.tick)
		}
		b.lastFire = fire
	} else {
		b.lastFire = false
	}
}

// raceUpgrades: the order a bot buys upgrades in, a tier of each in turn.
var raceUpgrades = []string{"engine", "plating", "tyres", "charges", "tank"}

// aiRaceBuy: on the grid, the best car the bot can afford (a better one than
// it has; never the hovercraft: it slides, a bot takes it slowly), then
// upgrades with what is left, a tier of each in turn (the race shop as a
// player uses it: carshop.go).
func (s *Server) aiRaceBuy(p *Player) {
	cur := s.garageCarSpec(p)
	var best *sim.CarSpec
	for _, sp := range CarTypes {
		if sp.Drive == sim.DriveHover {
			continue
		}
		if sp.Tier > cur.Tier && sp.Price <= p.Money && (best == nil || sp.Tier > best.Tier) {
			best = sp
		}
	}
	if best != nil {
		s.carPurchase(p, "car:"+best.ID)
	}
	for bought := true; bought; {
		bought = false
		for _, up := range raceUpgrades {
			if s.carPurchase(p, "up:"+up) == "" {
				bought = true
			}
		}
	}
}

// Power-ups (pickups.go): how far ahead a bot looks for one, how far off its
// line it goes for one, and how wide either side of its nose.
const (
	raceDetourNear = 6.0  // closer than this it is under the car (or missed)
	raceDetourFar  = 45.0 // further: later
	raceDetourOff  = 7.0  // from the line, across
	raceDetourCone = 0.45 // rad either side of the heading
)

// raceDetour: a pickup up ahead (within raceDetourFar, near the line, in front
// of the nose, not on a jump's ramp) that is worth it to this car: a repair
// when hurt, the boost or the charges when low, the rest always. The nearest.
func (s *Server) raceDetour(p *Player, car *Car, l *raceLine, i int, pos Vec3, hx, hz float64) (Vec3, bool) {
	if l.climbs(i, raceJumpLook) {
		return Vec3{}, false // a jump: its speed and its line
	}
	var best Vec3
	bestD := math.Inf(1)
	for _, pu := range s.round.Pickups {
		if !pu.Up || !s.pickupWorth(p, car, pu.Kind) {
			continue
		}
		dx, dz := pu.Pos[0]-pos[0], pu.Pos[2]-pos[2]
		along := dx*hx + dz*hz
		d := math.Hypot(dx, dz)
		if along < raceDetourNear || d > raceDetourFar || d >= bestD || math.Abs(pu.Pos[1]-pos[1]) > 3 {
			continue
		}
		if math.Acos(math.Min(1, along/d)) > raceDetourCone {
			continue
		}
		if q := l.pts[l.ahead(i, along)]; xzDist(q, pu.Pos) > raceDetourOff {
			continue // off the line: not this way
		}
		best, bestD = pu.Pos, d
	}
	return best, !math.IsInf(bestD, 1)
}

// pickupWorth: a power-up of kind is worth a detour to p in car.
func (s *Server) pickupWorth(p *Player, car *Car, kind string) bool {
	switch kind {
	case content.PickupHotfix:
		return car.sim.HP < car.MaxHP*3/4
	case content.PickupCaffeine:
		return car.sim.Boost < carTank(s.garageOf(p))/2
	case content.PickupReorg:
		g := s.garageOf(p)
		fn, rn := car.cs.fn, car.cs.rn
		s.refillCharges(car, g) // what full is
		full := car.cs.fn + car.cs.rn
		car.cs.fn, car.cs.rn = fn, rn
		return fn+rn < full
	}
	return true
}
