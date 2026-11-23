package game

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// Bots and solid players (mp_solid_players): a bot that makes no headway to
// its next waypoint with a player right in front of it gives way, by simple
// rules (ai_way_test.go):
//   - blocked: less than aiWayProgress closer to the waypoint over aiWayCheck
//     ticks, with a solid player within aiWayReach ahead;
//   - of two bots walking into each other the higher ID gives way, the other
//     keeps going; a human or a bot standing still is always given way to;
//   - giving way: a sidestep to the side with room (the wider; seeded on a
//     tie) or, with no room on either side (a corridor), backing off from the
//     blocker, as long as it stays in the way;
//   - after aiWayTries of those without getting past, wait aiWayCheck, then
//     route again.
//
// Round starts are staggered by aiWayStagger ticks per bot of a team (in ID
// order), so a team doesn't reach its spawn's doorway in lockstep.

type aiWay struct {
	next     Vec3       // the waypoint the progress is measured to
	checkAt  int64      // the next progress check (command ticks; 0: start one)
	dist     float64    // the distance to next at the last check
	until    int64      // giving way until this command tick
	from     int        // the player given way to
	dir      [2]float64 // the way to go meanwhile (world x, z)
	tries    int        // ways given in a row without getting past
	waitTill int64      // standing still until this command tick
	round    int        // the round the start was staggered for
}

const (
	aiWayCheck    = SimHz / 2 // 0.5 s
	aiWayProgress = 0.3
	aiWayReach    = 1.2 // centre to centre
	aiWayTries    = 2
	aiWayStagger  = 8
	aiWayRoom     = 1.0 // m of free floor beside the bot to sidestep into
)

// aiGiveWay steers p around (or away from) a player in its way to next;
// true: c is decided (don't steer it further).
func (s *Server) aiGiveWay(p *Player, c *Cmd, next Vec3, engaged bool) bool {
	b := p.ai
	w := &b.way
	t := p.cmdTicks
	if !s.solidPlayers() {
		return false
	}
	if s.roundsOn() && !s.frozen() && w.round != s.round.Number {
		w.round = s.round.Number
		w.waitTill = t + int64(aiWayStagger*s.aiRank(p))
	}
	if engaged {
		w.until, w.tries, w.checkAt = 0, 0, 0
		return false
	}
	if t < w.waitTill {
		c.F, c.R = 0, 0
		return true
	}
	if t < w.until {
		if q := s.players[w.from]; t+1 >= w.until && q != nil && s.aiBlocker(p, next) == q {
			w.until += aiWayCheck // still in the way: on
		}
		b.walk(c, w.dir[0], w.dir[1])
		if t+1 >= w.until { // done: route again from here (the sidestep left the line between waypoints)
			w.checkAt = 0
			b.repathAt = t + 1
		}
		return true
	}
	d := math.Hypot(next[0]-p.Pos[0], next[2]-p.Pos[2])
	if w.checkAt == 0 || w.next != next {
		w.next, w.dist, w.checkAt = next, d, t+aiWayCheck
		return false
	}
	if t < w.checkAt {
		return false
	}
	progress := w.dist - d
	w.dist, w.checkAt = d, t+aiWayCheck
	if progress >= aiWayProgress {
		w.tries = 0
		return false
	}
	q := s.aiBlocker(p, next)
	if q == nil || (q.ai != nil && !aiStanding(q) && q.ID > p.ID) {
		return false // not a player in the way (the stuck rule), or the other one gives way
	}
	w.tries++
	if w.tries > aiWayTries {
		w.tries, w.waitTill = 0, t+aiWayCheck
		b.repathAt = t + aiWayCheck
		c.F, c.R = 0, 0
		return true
	}
	// Sideways: perpendicular to the way to the waypoint.
	ux, uz := (next[0]-p.Pos[0])/math.Max(d, 1e-9), (next[2]-p.Pos[2])/math.Max(d, 1e-9)
	chest := Vec3{p.Pos[0], p.Pos[1] + 1.0, p.Pos[2]}
	right := mapDistance(&s.Map, chest, Vec3{-uz, 0, ux})
	left := mapDistance(&s.Map, chest, Vec3{uz, 0, -ux})
	switch {
	case math.Max(left, right) < aiWayRoom: // no room: back off from it
		ax, az := p.Pos[0]-q.Pos[0], p.Pos[2]-q.Pos[2]
		l := math.Max(math.Hypot(ax, az), 1e-9)
		w.dir = [2]float64{ax / l, az / l}
	case right > left || (right == left && b.r.Float() < 0.5):
		w.dir = [2]float64{-uz, ux}
	default:
		w.dir = [2]float64{uz, -ux}
	}
	w.until, w.from = t+aiWayCheck, q.ID
	b.walk(c, w.dir[0], w.dir[1])
	return true
}

// walk: move along the world direction (x, z) whatever way the bot faces.
func (b *aiBrain) walk(c *Cmd, x, z float64) {
	rel := math.Atan2(-x, -z) - b.yaw
	c.F, c.R = math.Cos(rel), -math.Sin(rel)
}

// aiBlocker: the solid player standing in p's way to next (within aiWayReach
// ahead of it, a hull's width either side), or nil.
func (s *Server) aiBlocker(p *Player, next Vec3) *Player {
	dx, dz := next[0]-p.Pos[0], next[2]-p.Pos[2]
	l := math.Hypot(dx, dz)
	if l < 1e-9 {
		return nil
	}
	ux, uz := dx/l, dz/l
	var best *Player
	bestD := math.Inf(1)
	for _, q := range s.ordered() {
		if q == p || !solid(q) || math.Abs(q.Pos[1]-p.Pos[1]) > 1.5 {
			continue
		}
		rx, rz := q.Pos[0]-p.Pos[0], q.Pos[2]-p.Pos[2]
		ahead, side := rx*ux+rz*uz, math.Abs(rx*uz-rz*ux)
		if ahead > 0.1 && ahead < aiWayReach && side < 2*sim.R+0.1 && ahead < bestD {
			best, bestD = q, ahead
		}
	}
	return best
}

// aiStanding: a bot that isn't trying to walk anywhere (holding a spot, waiting).
func aiStanding(q *Player) bool {
	return len(q.ai.path) == 0 || q.cmdTicks < q.ai.way.waitTill
}

// aiRank: how many bots of p's team have a lower ID (its place in the staggered start).
func (s *Server) aiRank(p *Player) int {
	n := 0
	for _, q := range s.players {
		if q.ai != nil && q.Team == p.Team && q.ID < p.ID && q.Team != content.Spectator {
			n++
		}
	}
	return n
}
