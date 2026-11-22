package game

import (
	"math"
	"strconv"
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

// tickBots advances every bot by dt seconds. Callers hold s.mu.
func (s *Server) tickBots(dt float64) {
	for _, p := range s.ordered() {
		if !p.Bot {
			continue
		}
		b := p.brain
		if b.frozen {
			continue
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
			continue
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
			continue
		}
		speed := botSpeed
		if p.Crouch {
			speed /= 2
		}
		if dist < 1e-9 {
			continue
		}
		step := math.Min(dist, speed*dt)
		p.Pos[0] += dx / dist * step
		p.Pos[2] += dz / dist * step
		p.Yaw = math.Atan2(-dx, -dz) // yaw 0 faces -Z
	}
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
func (s *Server) segmentClear(a, b Vec3, r float64) bool {
	lo, hi := math.Min(a[1], b[1])+0.45, math.Max(a[1], b[1])+1.7
	for _, bx := range s.Map.Boxes {
		if bx.Max[1] <= lo || bx.Min[1] >= hi {
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
	return true
}
