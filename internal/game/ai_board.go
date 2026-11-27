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

// aiBoard is bot p's way across on a ship: ok false when there is no
// departure from its island. key "" (with ok): stand where it is (riding).
func (s *Server) aiBoard(p *Player) (goal Vec3, key string, ok bool) {
	me := s.islandOf(p.Pos)
	for i := range s.Map.Departures {
		d := &s.Map.Departures[i]
		if s.aboard(p, d) {
			if !s.docked(d, 0) {
				return p.Pos, "", true // riding: stand (and shoot)
			}
			// The aisle, a spot each (by id), till the door shuts.
			k := float64(p.ID%6) - 2.5
			return Vec3{d.Board[0] + 2.2*k, d.Board[1], d.Board[2] + 0.4*float64(p.ID%2*2-1)}, fmt.Sprintf("board:aisle:%d", p.ID), true
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
	if g, k, ok := s.aiBoard(p); ok {
		if k == "" {
			k = "ride"
		}
		return g, k
	}
	return goal, key
}
