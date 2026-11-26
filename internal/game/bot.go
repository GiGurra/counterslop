package game

import (
	"math"
	"strconv"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// Bots are server-side target-practice players: they wander between reachable
// floor points, sometimes crouch or jump (to exercise every hitbox), never
// fire and never die. They show up in snapshots like anyone else, flagged bot.

const (
	botSpeed = 3.0
	botGrav  = 22.0
	botJump  = 6.5
)

type botBrain struct {
	target     Vec3
	idle       float64 // seconds to stand still
	crouchLeft float64
	vy         float64
	frozen     bool
	holdCrouch bool
	patrol     *[2]Vec3 // bot_patrol: walk back and forth between two points, never pausing (lag-comp tests)
	leg        int
}

func (s *Server) addBot() *Player {
	s.nextID++
	id := s.nextID
	p := &Player{
		ID:         id,
		Name:       "Target Practice (bot " + strconv.Itoa(id) + ")",
		Color:      "#c0c0c0",
		Pos:        Vec3{0, 0, 10},
		Bot:        true,
		ready:      true,
		lastUpdate: s.clock(),
		brain:      &botBrain{},
	}
	if pt, ok := s.randomFloorPoint(p.Pos); ok {
		p.Pos = pt
	}
	p.brain.target = p.Pos
	s.players[id] = p
	return p
}

func (s *Server) kickBots() int {
	n := 0
	for _, p := range s.ordered() {
		if p.Bot {
			delete(s.players, p.ID)
			n++
		}
	}
	return n
}

// tickBots advances every bot by dt seconds, and its gait (gait_shared.go:
// bots don't run commands, so it steps here, once a tick). Callers hold s.mu.
func (s *Server) tickBots(dt float64) {
	for _, p := range s.ordered() {
		if !p.Bot {
			continue
		}
		from := p.Pos
		s.tickBot(p, dt)
		p.Gait.Step(p.Pos[0]-from[0], p.Pos[1]-from[1], p.Pos[2]-from[2], p.Pos[1] <= 0 && p.brain.vy == 0, p.Crouch, GaitKOf(p.ID).Swing)
	}
}

// tickBot moves one practice bot.
func (s *Server) tickBot(p *Player, dt float64) {
	{
		b := p.brain
		if b.frozen {
			return
		}
		// vertical: simple hop on flat floor (bots only walk on y=0)
		if b.vy != 0 || p.Pos[1] > 0 {
			b.vy -= botGrav * dt
			p.Pos[1] += b.vy * dt
			if p.Pos[1] <= 0 {
				p.Pos[1], b.vy = 0, 0
			}
		}
		if b.crouchLeft > 0 {
			b.crouchLeft -= dt
			p.Crouch = b.crouchLeft > 0 || b.holdCrouch
		}
		if b.patrol != nil {
			b.target = b.patrol[b.leg]
			if math.Hypot(b.target[0]-p.Pos[0], b.target[2]-p.Pos[2]) < 0.05 {
				b.leg = 1 - b.leg
				b.target = b.patrol[b.leg]
			}
		}
		if b.idle > 0 {
			b.idle -= dt
			return
		}
		dx, dz := b.target[0]-p.Pos[0], b.target[2]-p.Pos[2]
		dist := math.Hypot(dx, dz)
		if dist < 0.2 && b.patrol == nil {
			// arrived: maybe pause, crouch or hop, then pick the next point
			switch r := s.rng.Float64(); {
			case r < 0.25:
				b.idle = 0.5 + s.rng.Float64()*1.5
			case r < 0.45:
				b.crouchLeft = 1 + s.rng.Float64()*2
				p.Crouch = true
			case r < 0.6 && p.Pos[1] == 0:
				b.vy = botJump
			}
			if pt, ok := s.randomFloorPoint(p.Pos); ok {
				b.target = pt
			}
			return
		}
		speed := botSpeed
		if p.Crouch {
			speed /= 2
		}
		if dist < 1e-9 {
			return
		}
		step := math.Min(dist, speed*dt)
		p.Yaw = math.Atan2(-dx, -dz) // yaw 0 faces -Z
		to := Vec3{p.Pos[0] + dx/dist*step, p.Pos[1], p.Pos[2] + dz/dist*step}
		if s.botBlocked(p, to) {
			b.idle = 0.3 // someone in the way: wait, then walk somewhere else
			if pt, ok := s.randomFloorPoint(p.Pos); ok {
				b.target = pt
			}
			return
		}
		p.Pos = to
	}
}

// botBlocked: stepping bot p to 'to' would take it into (or deeper into) a
// solid player's hull. Bots don't run the sim, so players are solid to them
// here: they stop instead of walking through anyone.
func (s *Server) botBlocked(p *Player, to Vec3) bool {
	if !s.solidPlayers() {
		return false
	}
	h := sim.H
	if p.Crouch {
		h = sim.HCrouch
	}
	for _, q := range s.players {
		if q == p || !solid(q) {
			continue
		}
		qh := sim.H
		if q.Crouch {
			qh = sim.HCrouch
		}
		dx, dz := math.Abs(to[0]-q.Pos[0]), math.Abs(to[2]-q.Pos[2])
		if dx >= 2*sim.R || dz >= 2*sim.R || to[1] >= q.Pos[1]+qh || q.Pos[1] >= to[1]+h {
			continue
		}
		if math.Hypot(dx, dz) < math.Hypot(p.Pos[0]-q.Pos[0], p.Pos[2]-q.Pos[2]) {
			return true
		}
	}
	return false
}

// randomFloorPoint picks a point within 12 m on the open floor that a straight
// walk from 'from' reaches without touching any obstacle.
func (s *Server) randomFloorPoint(from Vec3) (Vec3, bool) {
	const r = 0.5 // keep clear of walls
	for range 40 {
		a, d := s.rng.Float64()*2*math.Pi, 3+s.rng.Float64()*9
		pt := Vec3{from[0] + math.Cos(a)*d, 0, from[2] + math.Sin(a)*d}
		if s.segmentClear(from, pt, r) {
			return pt, true
		}
	}
	return from, false
}

// segmentClear: does a disc of radius r sweep from a to b without overlapping
// any box in the way at walking height (from a step above the lower end's
// floor to head height over the higher end: not the floor below, not a deck
// or ceiling above; de_stack has floors over floors)?
//
// A walkable ramp (a wedge, internal/sim) is walked over: it blocks only
// where its slope (under the disc) rises more than a step over the
// segment's own height there, as at its high end or its tall sides.
func (s *Server) segmentClear(a, b Vec3, r float64) bool {
	lo, hi := math.Min(a[1], b[1])+0.45, math.Max(a[1], b[1])+1.7
	for _, bx := range s.Map.Boxes {
		if bx.Max[1] <= lo || bx.Min[1] >= hi {
			continue
		}
		if sim.IsWedge(&bx) && sim.Walkable(&bx) {
			if !rampClear(&bx, a, b, r) {
				return false
			}
			continue
		}
		mn := Vec3{bx.Min[0] - r, -1, bx.Min[2] - r}
		mx := Vec3{bx.Max[0] + r, 1, bx.Max[2] + r}
		d := Vec3{b[0] - a[0], 0, b[2] - a[2]}
		o := Vec3{a[0], 0, a[2]}
		if t := rayAABB(o, d, mn, mx); t >= 0 && t <= 1 {
			return false
		}
	}
	// Water (Map.Water) is no floor to a bot: a walk that would step off into
	// a basin whose surface is within a fall of its feet doesn't count as
	// clear (de_hostile_takeover's quay: bots wandered into the harbour and
	// swam there all round). From in the water any way out is fine.
	for _, wx := range s.Map.Water {
		if wx.Max[1] < lo-0.45-4 || wx.Min[1] >= hi || inWaterXZ(&wx, a) {
			continue
		}
		mn, mx := Vec3{wx.Min[0] - r, -1, wx.Min[2] - r}, Vec3{wx.Max[0] + r, 1, wx.Max[2] + r}
		if t := rayAABB(Vec3{a[0], 0, a[2]}, Vec3{b[0] - a[0], 0, b[2] - a[2]}, mn, mx); t >= 0 && t <= 1 {
			return false
		}
	}
	return true
}

// aiKeepDry: a bot on dry ground never steps into water (Map.Water): if the
// command's move would put it within a stride of a basin it isn't in (a
// strafe at the quay's edge, a sidestep, a dodge), it stands still instead.
// Bots don't swim or ride boats (v1): a bot in the water stays lost.
func (s *Server) aiKeepDry(p *Player, c *Cmd) {
	if len(s.Map.Water) == 0 || c.F == 0 && c.R == 0 {
		return
	}
	sy, cy := math.Sin(c.Yaw), math.Cos(c.Yaw)
	wx, wz := -sy*c.F+cy*c.R, -cy*c.F-sy*c.R
	dirs := [][2]float64{{wx, wz}}
	if p.body != nil { // and where it is going (sliding along a wall goes where the keys don't)
		dirs = append(dirs, [2]float64{p.body.Vel[0], p.body.Vel[2]})
	}
	for _, d := range dirs {
		l := math.Hypot(d[0], d[1])
		if l < 0.1 {
			continue
		}
		ahead := Vec3{p.Pos[0] + d[0]/l*(sim.R+0.6), p.Pos[1], p.Pos[2] + d[1]/l*(sim.R+0.6)}
		for _, w := range s.Map.Water {
			over := ahead[0] > w.Min[0] && ahead[0] < w.Max[0] && ahead[2] > w.Min[2] && ahead[2] < w.Max[2]
			if over && !inWaterXZ(&w, p.Pos) && s.groundTop(ahead[0], ahead[2]) < p.Pos[1]-aiStepUp {
				c.F, c.R = 0, 0
				return
			}
		}
	}
}

// inWaterXZ: is p over water box wx (and not above its surface by more than a step)?
func inWaterXZ(wx *content.Box, p Vec3) bool {
	return p[0] > wx.Min[0] && p[0] < wx.Max[0] && p[2] > wx.Min[2] && p[2] < wx.Max[2] && p[1] < wx.Max[1]+aiStepUp
}

// rampClear: walking a disc of radius r from a to b (feet heights a[1] to
// b[1]) over walkable wedge bx: nowhere does its slope under the disc stand
// more than a step over the feet. Sampled every 0.2 m.
func rampClear(bx *content.Box, a, b Vec3, r float64) bool {
	n := int(math.Hypot(b[0]-a[0], b[2]-a[2])/0.2) + 1
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		x, y, z := a[0]+(b[0]-a[0])*f, a[1]+(b[1]-a[1])*f, a[2]+(b[2]-a[2])*f
		if x+r <= bx.Min[0] || x-r >= bx.Max[0] || z+r <= bx.Min[2] || z-r >= bx.Max[2] || y+sim.H <= bx.Min[1] {
			continue
		}
		cx, cz := math.Max(bx.Min[0], math.Min(bx.Max[0], x)), math.Max(bx.Min[2], math.Min(bx.Max[2], z))
		if sim.HullTop(bx, cx, cz)-y > aiStepUp {
			return false
		}
	}
	return true
}
