package game

import (
	"math"
	"strings"

	"github.com/gigurra/counterslop/internal/content"
)

// Lanes: route variety for attackers. A map marks some nav nodes as lanes
// (content.NavNode.Lane, mapgen NavLane): the long way round, mid, a tunnel.
// Each round an attacker bot picks one (seeded) whose detour to its site is
// within aiLaneDetour of the shortest route, and walks spawn -> lane -> site.
// The lane is dropped once the bot gets there, or when the Big Diff drops
// (then everyone goes for the bomb). Maps without lane nodes are unchanged:
// no rng is drawn and paths are the plain shortest route.

const (
	aiLaneDetour = 1.6 // a lane's route may be this much longer than the shortest
	aiLaneReach  = 3.0 // this close (horizontally, same floor) and the lane is done
)

type aiLane struct {
	round int  // the round the lane was picked for
	at    Vec3 // the lane node
	on    bool // still to walk through it
	site  string
}

// laneNodes are the map's lane waypoints, in map order.
func (s *Server) laneNodes() []Vec3 {
	var out []Vec3
	for _, n := range s.Map.Nav {
		if n.Lane {
			out = append(out, Vec3(n.Pos))
		}
	}
	return out
}

// pickLane chooses this round's lane for an attacker heading to site from
// where it stands; ok false means no lane (none on the map, or none near
// enough the way).
func (s *Server) pickLane(p *Player, site Vec3) (Vec3, bool) {
	lanes := s.laneNodes()
	if len(lanes) == 0 {
		return Vec3{}, false
	}
	direct := pathLen(p.Pos, s.navPath(p.Pos, site))
	var ok []Vec3
	for _, l := range lanes {
		d := pathLen(p.Pos, s.navPath(p.Pos, l)) + pathLen(l, s.navPath(l, site))
		if d <= direct*aiLaneDetour {
			ok = append(ok, l)
		}
	}
	if len(ok) == 0 {
		return Vec3{}, false
	}
	return ok[p.ai.r.Int(0, len(ok)-1)], true
}

// laneProgress drops the lane once the bot reaches it or the bomb is dropped.
func (s *Server) laneProgress(p *Player) {
	l := &p.ai.lane
	if !l.on {
		return
	}
	if s.round.Bomb.State == BombDropped || l.round != s.round.Number ||
		math.Hypot(p.Pos[0]-l.at[0], p.Pos[2]-l.at[2]) < aiLaneReach && math.Abs(p.Pos[1]-l.at[1]) < 1.5 {
		l.on = false
	}
}

// attackPath is the route for a (re)path toward goal: through this round's
// lane for an attacker walking to (or carrying the Big Diff to) its site,
// else the shortest route.
func (s *Server) attackPath(p *Player, key string, goal Vec3) []Vec3 {
	b := p.ai
	if p.Team != content.Vibe || !s.roundsOn() || !(strings.HasPrefix(key, "site:") || strings.HasPrefix(key, "plant:")) {
		return s.navPath(p.Pos, goal)
	}
	if b.lane.round != s.round.Number || b.lane.site != b.site {
		at, ok := s.pickLane(p, goal)
		b.lane = aiLane{round: s.round.Number, at: at, on: ok, site: b.site}
	}
	if !b.lane.on {
		return s.navPath(p.Pos, goal)
	}
	return append(s.navPath(p.Pos, b.lane.at), s.navPath(b.lane.at, goal)...)
}
