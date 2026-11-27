package game

import (
	"fmt"
	"math"
	"strings"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// Bots and ships (Map.Departures, docs/starslop.md phase 3). A map's nav
// graph can come in islands no walk joins (lab_moon: Earth and the Moon);
// a ship's timetable joins them. A tickets bot goes for points on its own
// island; when every point it wants is on another one and a departure's door
// is on its island, it boards: to the door (the jet bridge's end) while the
// ship is docked, straight in to its own spot in the aisle, then it stands
// and rides until the drop pods put it down on the other side.

// navIslands is each nav node's island (connected component), cached per
// nav graph.
func (s *Server) navIslands() []int {
	nav := s.Map.Nav
	if len(nav) == 0 {
		return nil
	}
	if s.islands.of == &nav[0] && len(s.islands.comp) == len(nav) {
		return s.islands.comp
	}
	idx := map[string]int{}
	for i, n := range nav {
		idx[n.Name] = i
	}
	comp := make([]int, len(nav))
	for i := range comp {
		comp[i] = i
	}
	var root func(i int) int
	root = func(i int) int {
		for comp[i] != i {
			comp[i] = comp[comp[i]]
			i = comp[i]
		}
		return i
	}
	for i, n := range nav {
		for _, l := range n.Links {
			if j, ok := idx[l]; ok {
				if a, b := root(i), root(j); a != b {
					comp[a] = b
				}
			}
		}
	}
	for i := range comp {
		comp[i] = root(i)
	}
	s.islands.of, s.islands.comp = &nav[0], comp
	return comp
}

// islandOf is the island of the nav node nearest p (-1: no nav graph).
func (s *Server) islandOf(p Vec3) int {
	comp := s.navIslands()
	best, bd := -1, math.Inf(1)
	for i, n := range s.Map.Nav {
		d := math.Hypot(n.Pos[0]-p[0], n.Pos[2]-p[2]) + 3*math.Abs(n.Pos[1]-p[1])
		if d < bd {
			best, bd = i, d
		}
	}
	if best < 0 {
		return -1
	}
	return comp[best]
}

// moverClock is now on the movers' clock (the command time of a command run
// this tick: CmdTime with the tick as its base).
func (s *Server) moverClock() float64 { return float64(s.tick) / SimHz }

// docked: departure d's ship is at its dock with the door open (boarding),
// with margin seconds to spare before it closes.
func (s *Server) docked(d *content.Departure, margin float64) bool {
	if d.Mover < 1 || d.Mover > len(s.Map.Movers) {
		return false
	}
	m := &s.Map.Movers[d.Mover-1]
	o := sim.MoverOffset(m, s.moverClock())
	return o == (sim.Vec3{}) && sim.CycleT(m, s.moverClock()) < d.Close-margin
}

// aboard: p is inside departure d's cabin (round its Board spot, where the
// ship is now).
func (s *Server) aboard(p *Player, d *content.Departure) bool {
	if d.Mover < 1 || d.Mover > len(s.Map.Movers) {
		return false
	}
	o := sim.MoverOffset(&s.Map.Movers[d.Mover-1], s.moverClock())
	return math.Abs(p.Pos[0]-d.Board[0]-o[0]) < 8 && math.Abs(p.Pos[2]-d.Board[2]-o[2]) < 4 && math.Abs(p.Pos[1]-d.Board[1]-o[1]) < 2
}

// aiPodFall is about how long a drop pod's throw flies (MVP-1's 70 m down on
// the Moon): a flank pod lands its sideways speed times this off the drop.
const aiPodFall = 2.2

// aiDrop is where the ship p would take (the first departure from its
// island, or the one it is aboard) puts its passengers down: its middle pods
// at the time they fire. ok false: no departure.
func (s *Server) aiDrop(p *Player) (Vec3, bool) {
	me := s.islandOf(p.Pos)
	for i := range s.Map.Departures {
		d := &s.Map.Departures[i]
		if !s.aboard(p, d) && (me < 0 || s.islandOf(Vec3(d.Door)) != me) {
			continue
		}
		for _, l := range s.Map.Launchers {
			if l.Mover == d.Mover && d.Mover >= 1 && d.Mover <= len(s.Map.Movers) {
				o := sim.MoverOffset(&s.Map.Movers[d.Mover-1], l.From)
				return Vec3{d.Board[0] + o[0], d.Board[1] + o[1], d.Board[2] + o[2]}, true
			}
		}
	}
	return Vec3{}, false
}

// aisleSpot is bot p's place in departure d's aisle (its spot by id): over
// the pods whose throw lands nearest target when the ship has more than one
// set (cq_moonshot's flank pods), spread along their third.
func (s *Server) aisleSpot(p *Player, d *content.Departure, target Vec3) Vec3 {
	k := float64(p.ID%6) - 2.5
	spot := Vec3{d.Board[0] + 2.2*k, d.Board[1], d.Board[2] + 0.4*float64(p.ID%2*2-1)}
	var pods []content.Launcher
	for _, l := range s.Map.Launchers {
		if l.Mover == d.Mover {
			pods = append(pods, l)
		}
	}
	if len(pods) < 2 || d.Mover < 1 || d.Mover > len(s.Map.Movers) {
		return spot
	}
	best, bd := -1, math.Inf(1)
	for i, l := range pods {
		o := sim.MoverOffset(&s.Map.Movers[d.Mover-1], l.From)
		x := (l.Min[0]+l.Max[0])/2 + o[0] + l.Vel[0]*aiPodFall
		z := (l.Min[2]+l.Max[2])/2 + o[2] + l.Vel[2]*aiPodFall
		if dd := math.Hypot(target[0]-x, target[2]-z); dd < bd {
			best, bd = i, dd
		}
	}
	l := pods[best]
	w := (l.Max[0] - l.Min[0]) / 2
	spot[0] = (l.Min[0]+l.Max[0])/2 + 0.5*w*(float64(p.ID%3)-1)
	return spot
}

// aiBoard is bot p's way across on a ship to target: ok false when there is
// no departure from its island. key "" (with ok): stand where it is (riding).
func (s *Server) aiBoard(p *Player, target Vec3) (goal Vec3, key string, ok bool) {
	me := s.islandOf(p.Pos)
	for i := range s.Map.Departures {
		d := &s.Map.Departures[i]
		if s.aboard(p, d) {
			if !s.docked(d, 0) {
				return p.Pos, "", true // riding: stand (and shoot)
			}
			// The aisle, a spot each (over its pods), till the door shuts.
			at := s.aisleSpot(p, d, target)
			return at, fmt.Sprintf("board:aisle:%d:%.0f", p.ID, at[0]), true
		}
		if me < 0 || s.islandOf(Vec3(d.Door)) != me {
			continue
		}
		// Inward: from the door spot towards the aisle; the doorway is the
		// first 2.2 m of that (the door is 2 m wide: aim at its middle).
		ix, iz := d.Board[0]-d.Door[0], d.Board[2]-d.Door[2]
		il := math.Hypot(ix, iz)
		ix, iz = ix/il, iz/il
		in := (p.Pos[0]-d.Door[0])*ix + (p.Pos[2]-d.Door[2])*iz // how far in
		side := math.Abs((p.Pos[0]-d.Door[0])*iz - (p.Pos[2]-d.Door[2])*ix)
		switch {
		case in > 0.8 && in < 4 && side < 1.2 && s.docked(d, 0.5):
			return Vec3(d.Board), "board:in", true // through: on to the aisle
		case math.Hypot(p.Pos[0]-d.Door[0], p.Pos[2]-d.Door[2]) < 2 && s.docked(d, 1):
			return Vec3{d.Door[0] + 2.2*ix, d.Door[1], d.Door[2] + 2.2*iz}, "board:door", true
		}
		return Vec3(d.Door), "dock:" + d.Name, true // to the door (wait there while it's away)
	}
	return Vec3{}, "", false
}

// aiIsle caches the islands a bot and its goal are on (islandOf is a scan of
// the nav graph): its own every aiIsleTicks, its goal's per goal key.
type aiIsle struct {
	at, goal int
	atTick   int64
	goalKey  string
}

const aiIsleTicks = 25

// aiAcross is bot p's goal on its way to (goal, key): when goal is on
// another nav island than the bot and a departure from its own island goes
// there, the way across (aiBoard: to the door, in, ride); else (goal, key).
// A riding bot (aiBoard's "" key) stands where it is.
func (s *Server) aiAcross(p *Player, goal Vec3, key string) (Vec3, string) {
	if len(s.Map.Departures) == 0 || len(s.Map.Nav) == 0 || strings.HasPrefix(key, "board:") || strings.HasPrefix(key, "dock:") {
		return goal, key
	}
	b := p.ai
	if b.isle.atTick == 0 || s.tick-b.isle.atTick >= aiIsleTicks {
		b.isle.at, b.isle.atTick = s.islandOf(p.Pos), s.tick
	}
	if b.isle.goalKey != key {
		b.isle.goal, b.isle.goalKey = s.islandOf(goal), key
	}
	if b.isle.at < 0 || b.isle.goal < 0 || b.isle.at == b.isle.goal {
		for i := range s.Map.Departures { // aboard and in flight: ride it out
			if d := &s.Map.Departures[i]; s.aboard(p, d) && !s.docked(d, 0) {
				return p.Pos, "ride:" + d.Name
			}
		}
		return goal, key
	}
	if g, k, ok := s.aiBoard(p, goal); ok {
		if k == "" {
			k = "ride"
		}
		return g, k
	}
	return goal, key
}
