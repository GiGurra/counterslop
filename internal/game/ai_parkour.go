package game

// Bots on a parkour course (Map.OnFoot with a Map.Route; docs/racing.md
// "Parkour"): a bot runs the map's route step by step, through the real
// movement code like a player (its commands go through runCmd): runs, jumps
// at the edges, lifts timed on the movers' clock, the hook and the wall
// walk, a car or a boat. A fall puts it back on its last gate (the race's
// rule), where it starts that gate's steps again; a step it can't finish in
// time puts it back there too.

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// aiParkour: a bot's place on the route.
type aiParkour struct {
	key    string   // the map the route is of (another map: start over)
	i      int      // the step
	ticks  int      // commands spent on it
	air    bool     // jump, hop, leap: off the ground since it jumped
	jumped bool     // jump, hop, leap: pressed jump
	yaw    float64  // wall: the yaw that climbs (turned with the body over edges); back: the facing
	last   sim.Vec3 // where the body was last command (a jump of more than 5 m: put back on a gate)
	from   sim.Vec3 // back: where it started
	use    bool     // board, leave: E was down last command (it acts on its down edge)
}

// pkStepTicks: a step not done in this many commands (25 s) puts the bot back on its gate.
const pkStepTicks = 2500

// pkRoute: the course's route (nil: bots don't run it).
func (s *Server) pkRoute() []content.RouteStep {
	if !s.onFoot() {
		return nil
	}
	return s.Map.Route
}

// pkSection: the route's first step from gate cp-1's respawn spot (racer
// cp gates passed; 0: the start).
func pkSection(route []content.RouteStep, cp int) int {
	if cp <= 0 {
		return 0
	}
	for i, st := range route {
		if st.Do == content.RouteFrom && st.Gate == cp-1 {
			return i + 1
		}
	}
	return 0
}

// aiParkourCmd decides a racing bot's command on a course. Callers hold s.mu.
func (s *Server) aiParkourCmd(p *Player) Cmd {
	b := p.ai
	k := &b.pk
	c := Cmd{Yaw: p.body.Yaw}
	route := s.pkRoute()
	rs := s.racer(p)
	if route == nil || rs == nil || !s.raceLive() || rs.Place > 0 {
		k.key, k.ticks = "", 0
		return c
	}
	pos := p.body.Pos
	if k.key != s.Map.Name || p.car == 0 && p.ride == 0 && dist3(pos, k.last) > 5 { // a new course, or put back on a gate: its steps again
		k.key = s.Map.Name
		s.pkGo(k, pkSection(route, rs.CP))
	}
	k.last = pos
	for k.i < len(route) && route[k.i].Do == content.RouteFrom {
		s.pkGo(k, k.i+1)
	}
	if k.i >= len(route) {
		return c
	}
	k.ticks++
	if k.ticks > pkStepTicks { // stuck: back on the gate (as a fall would)
		s.respawn(p)
		s.pkGo(k, pkSection(route, rs.CP))
		k.last = p.body.Pos
		return c
	}
	st := route[k.i]
	done := false
	switch st.Do {
	case content.RouteRun:
		c.Yaw, c.F = yawTo(pos, st.To), 1
		done = pkThere(pos, st.To, 0.8)
	case content.RouteJump, content.RouteHop, content.RouteLeap:
		c.Yaw, c.F = yawTo(pos, st.To), 1
		if !k.jumped && p.body.OnGround && (st.Do == content.RouteLeap || pkPast(pos, st.At, st.To)) {
			c.B |= BtnJump
			k.jumped = true
		} else if k.jumped && !p.body.OnGround {
			k.air = true
		}
		if k.jumped && st.Do != content.RouteLeap {
			c.Yaw = yawTo(st.At, st.To) // in the air: on along the jump, never back for To once past it
		}
		if k.air && st.Do == content.RouteHop {
			c.F = 0 // no keys in the air: onto a lift, not over it
		}
		switch st.Do {
		case content.RouteLeap:
			done = k.air && p.body.Vel[1] <= 0
		default:
			done = k.air && p.body.OnGround
		}
	case content.RouteBack:
		if k.ticks == 1 {
			k.yaw, k.from = yawTo(st.To, pos), pos // facing away from where it walks to
		}
		c.Yaw, c.F = k.yaw, -1
		done = pkPast(pos, st.To, [3]float64{2*st.To[0] - k.from[0], 0, 2*st.To[2] - k.from[2]}) // at or past it
	case content.RouteRest:
		done = p.body.OnGround && p.body.Grav == sim.GravDown && math.Hypot(p.body.Vel[0], p.body.Vel[2]) < 0.05
	case content.RouteWait:
		t := p.cmdTime(b.seq + 1)
		done = math.Abs(math.Mod(t, st.Period)-st.Phase) <= 0.006
	case content.RouteHook:
		eye := pos[1] + sim.Eye
		c.Yaw = yawTo(pos, st.At)
		c.Pitch = math.Atan2(st.At[1]-eye, math.Hypot(st.At[0]-pos[0], st.At[2]-pos[2]))
		c.B |= BtnHook
		done = p.body.Grav != sim.GravDown
		if k.ticks > 1 && !p.body.Hook.On && !done { // missed or let go: back on the gate
			k.ticks = pkStepTicks
		}
	case content.RouteWall:
		if k.ticks == 1 {
			k.yaw = s.pkClimbYaw(p)
		} else if d := angleDiff(p.body.Yaw, k.yaw); math.Abs(d) > 1e-9 {
			k.yaw = p.body.Yaw // over an edge: the body turned, the view with it
		}
		c.Yaw, c.F = k.yaw, 1
		done = p.body.Grav == sim.GravDown && p.body.OnGround
	case content.RouteBoard:
		if p.car != 0 || p.ride != 0 {
			done = true
			break
		}
		to, ok := s.pkVehicle(Vec3(pos), Vec3(st.To))
		if !ok {
			break
		}
		c.Yaw = yawTo(pos, to)
		if d := math.Hypot(to[0]-pos[0], to[2]-pos[2]); d > 2 {
			c.F = 1
		} else if !k.use {
			c.B |= BtnUse
		}
	case content.RouteDrive:
		done = s.pkDrive(p, &c, st.To)
	case content.RouteLeave:
		done = s.pkLeave(p, &c)
	case content.RouteSwim:
		c.Yaw, c.F = yawTo(pos, st.To), 1
		if !p.body.OnGround {
			c.B |= BtnJump // up out of the water, onto the ledge
		}
		done = p.body.OnGround && pos[1] >= st.To[1]-0.05
	default:
		done = true
	}
	k.use = c.B&BtnUse != 0
	if done {
		s.pkGo(k, k.i+1)
	}
	return c
}

// pkGo: on to step i.
func (s *Server) pkGo(k *aiParkour, i int) {
	k.i, k.ticks, k.air, k.jumped = i, 0, false, false
}

// yawTo: the yaw looking from a to b (x, z).
func yawTo[V ~[3]float64, W ~[3]float64](a V, b W) float64 {
	return math.Atan2(-(b[0] - a[0]), -(b[2] - a[2]))
}

// pkThere: pos within r of to (x, z).
func pkThere[V ~[3]float64](pos V, to [3]float64, r float64) bool {
	return math.Hypot(to[0]-pos[0], to[2]-pos[2]) < r
}

// pkPast: pos is past at on the way to `to` (along at -> to, x and z).
func pkPast[V ~[3]float64, W ~[3]float64](pos V, at [3]float64, to W) bool {
	dx, dz := to[0]-at[0], to[2]-at[2]
	return (pos[0]-at[0])*dx+(pos[2]-at[2])*dz >= 0
}

// pkClimbYaw: on a wall (gravity turned onto it), the yaw whose forward
// climbs fastest: each of four tried for 0.3 s on a copy of the body.
func (s *Server) pkClimbYaw(p *Player) float64 {
	w := s.simWorld()
	best, up := p.body.Yaw, math.Inf(-1)
	for _, yaw := range []float64{0, math.Pi / 2, math.Pi, -math.Pi / 2} {
		probe := *p.body
		w.T = p.cmdTime(p.ai.seq + 1)
		for range 30 {
			probe.Yaw = yaw
			w.Step(&probe, sim.Input{F: 1}, 1.0/SimHz)
			w.T += 1.0 / SimHz
		}
		if probe.Pos[1] > up {
			best, up = yaw, probe.Pos[1]
		}
	}
	return best
}

// pkVehicle: the free car or boat nearest near (within 20 m), where to walk to board it.
func (s *Server) pkVehicle(pos, near Vec3) (Vec3, bool) {
	var best Vec3
	bestD := 20.0
	for _, c := range s.cars {
		if d := xzDist(Vec3(c.sim.Pos), near); d < bestD && !s.carTaken(c.ID) && c.sim.HP > 0 {
			best, bestD = Vec3(c.sim.Pos), d
		}
	}
	for _, b := range s.boats {
		if d := xzDist(Vec3(b.sim.Pos), near); d < bestD && s.freeSeat(b, sim.Vec3(pos)) >= 0 {
			best, bestD = Vec3(b.sim.Pos), d
		}
	}
	return best, bestD < 20
}

// pkDrive: steer the car or boat p drives at `to` (a point 15 m ahead on the
// line from where it is to `to`, as the race bots do), full ahead; done once past it.
func (s *Server) pkDrive(p *Player, c *Cmd, to [3]float64) bool {
	var pos sim.Vec3
	var yaw float64
	if car := s.carByID(p.car); car != nil {
		pos, yaw = car.sim.Pos, car.sim.Yaw
	} else if boat := s.boatByID(p.ride); boat != nil {
		pos, yaw = boat.sim.Pos, boat.sim.Yaw
	} else {
		return true // not driving (out, or sunk): on
	}
	if pos[0] >= to[0] { // past it (the course runs east, +x)
		return true
	}
	// Aim along the course (east, +x) at the lane of `to`: 15 m ahead on its z.
	want := math.Atan2(-15, -(to[2] - pos[2]))
	if math.Abs(to[0]-pos[0]) < 15 {
		want = yawTo(pos, to)
	}
	e := angleDiff(want, yaw)
	c.Yaw, c.F, c.R = p.body.Yaw, 1, math.Max(-1, math.Min(1, -2*e))
	return false
}

// pkLeave: brake to a stop, then E (out of the car, up off the boat's seat).
func (s *Server) pkLeave(p *Player, c *Cmd) bool {
	if p.car == 0 && (p.ride == 0 || p.seat == SeatDeck) {
		return true
	}
	speed := 0.0
	if car := s.carByID(p.car); car != nil {
		speed = math.Hypot(car.sim.Vel[0], car.sim.Vel[2])
	} else if boat := s.boatByID(p.ride); boat != nil {
		speed = math.Hypot(boat.sim.Vel[0], boat.sim.Vel[2])
	}
	c.Yaw = p.body.Yaw
	if speed > 0.5 {
		c.F = -1
		return false
	}
	if !p.ai.pk.use {
		c.B |= BtnUse
	}
	return false
}
