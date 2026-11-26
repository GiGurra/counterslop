package game

import (
	"fmt"
	"math"
)

// Tickets-mode bots (tickets.go, docs/starslop.md). Simple on purpose: a bot
// goes for the nearest point its team does not hold; with every point held it
// goes to defend the one under attack (an enemy in it or an enemy bar on it),
// else the held point nearest the enemy's main. In a point it stands on its
// own spot inside the circle (spread by its id), fighting whoever comes.

// aiTicketGoal is where bot p heads in a live tickets round, and the route
// key (the same key keeps the route). ok false: nothing to do (no points).
func (s *Server) aiTicketGoal(p *Player) (goal Vec3, key string, ok bool) {
	best, bestD := -1, math.Inf(1)
	me := s.islandOf(p.Pos)
	reach := func(i int) bool { return me < 0 || s.islandOf(Vec3(s.Map.Points[i].Pos)) == me }
	across := false // a point it wants is on another island (ai_board.go)
	dist := func(i int) float64 {
		q := s.Map.Points[i].Pos
		return math.Hypot(q[0]-p.Pos[0], q[2]-p.Pos[2])
	}
	for i, def := range s.Map.Points { // attack: the nearest point not ours
		if i >= len(s.round.Points) || def.Locked || s.round.Points[i].Own == p.Team {
			continue
		}
		if !reach(i) {
			across = true
			continue
		}
		if d := dist(i); d < bestD {
			best, bestD = i, d
		}
	}
	if best < 0 && across {
		if g, k, ok := s.aiBoard(p); ok {
			return g, k, true
		}
	}
	if best < 0 { // defend: a point under attack, else the one nearest the enemy
		enemyMain := Vec3{}
		for _, def := range s.Map.Points {
			if def.Locked && def.Owner != p.Team {
				enemyMain = Vec3(def.Pos)
			}
		}
		for i, def := range s.Map.Points {
			if i >= len(s.round.Points) || def.Locked || !reach(i) {
				continue
			}
			st := s.round.Points[i]
			enemies := st.A
			if p.Team == "artisan" {
				enemies = st.V
			}
			d := math.Hypot(def.Pos[0]-enemyMain[0], def.Pos[2]-enemyMain[2])
			if enemies > 0 || (st.By != "" && st.By != p.Team) {
				d -= 1e4 // under attack first
			}
			if d < bestD {
				best, bestD = i, d
			}
		}
	}
	if best < 0 {
		return Vec3{}, "", false
	}
	def := s.Map.Points[best]
	a := float64(p.ID) * 2.39996 // the golden angle: teammates spread over the circle
	r := def.Radius * 0.45
	goal = Vec3{def.Pos[0] + r*math.Sin(a), def.Pos[1], def.Pos[2] + r*math.Cos(a)}
	return goal, fmt.Sprintf("pt:%s:%d", def.ID, p.ID), true
}
