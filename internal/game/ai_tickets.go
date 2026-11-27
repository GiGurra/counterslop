package game

import (
	"fmt"
	"math"
	"slices"
)

// Tickets-mode bots (tickets.go, docs/starslop.md phase 5).
//
// Attack: a bot goes for a point its team does not hold, scored by the walk
// there, the enemies standing in it and its teammates already headed there
// (aiTkFollow each: a team moves as a wave, not one by one). When every such
// point is on another nav island it goes by ship (ai_board.go), standing in
// the cabin over the pods that land nearest its target (the flank pods).
//
// Defence (nothing left to take): a point under attack (an enemy in it, or
// an enemy bar on it) draws the nearest half of the team's bots on that
// island; the rest spread over the held points, one each round the front
// first, and rotate every aiTkRotate seconds (no camping the drop zone).
//
// In a point it stands on its own spot inside the circle (spread by its id),
// fighting whoever comes.

const (
	aiTkFollow  = 20.0 // metres a teammate headed for a point is worth
	aiTkEnemy   = 30.0 // metres an enemy standing in a point costs
	aiTkRotate  = 45.0 // seconds a defender holds its post before rotating
	aiTkRespond = 0.5  // the share of the defenders that answer an attack
)

// aiTk is a tickets bot's choice: the point it heads for (index+1; 0 none).
type aiTk struct {
	pt int
}

// aiTicketGoal is where bot p heads in a live tickets round, and the route
// key (the same key keeps the route). ok false: nothing to do (no points).
func (s *Server) aiTicketGoal(p *Player) (goal Vec3, key string, ok bool) {
	pts := s.Map.Points
	me := s.islandOf(p.Pos)
	reach := func(i int) bool { return me < 0 || s.islandOf(Vec3(pts[i].Pos)) == me }
	dist := func(a Vec3, i int) float64 { return math.Hypot(pts[i].Pos[0]-a[0], pts[i].Pos[2]-a[2]) }
	enemiesIn := func(i int) int {
		if p.Team == "artisan" {
			return s.round.Points[i].V
		}
		return s.round.Points[i].A
	}
	headed := map[int]int{} // point -> teammates headed there
	for _, q := range s.ordered() {
		if q != p && q.ai != nil && q.Team == p.Team && !q.Dead && q.ai.tk.pt > 0 {
			headed[q.ai.tk.pt-1]++
		}
	}
	if i := s.aiGarrison(p, me, reach); i >= 0 { // some hold what the team took
		return s.aiTkGoal(p, i)
	}
	best, bestS := -1, math.Inf(1)
	var across []int          // points it wants on another island
	for i, def := range pts { // attack
		if i >= len(s.round.Points) || def.Locked || s.round.Points[i].Own == p.Team {
			continue
		}
		if !reach(i) {
			across = append(across, i)
			continue
		}
		if sc := dist(p.Pos, i) + aiTkEnemy*float64(enemiesIn(i)) - aiTkFollow*float64(min(headed[i], 3)); sc < bestS {
			best, bestS = i, sc
		}
	}
	if best < 0 && len(across) > 0 {
		// The wave's target over there: the fewest enemies in it, the most
		// teammates headed there (the ship's drop decides the rest).
		drop, _ := s.aiDrop(p)
		t, tS := -1, math.Inf(1)
		for _, i := range across {
			if sc := dist(drop, i) + aiTkEnemy*float64(enemiesIn(i)) - aiTkFollow*float64(min(headed[i], 3)); sc < tS {
				t, tS = i, sc
			}
		}
		if g, k, ok := s.aiBoard(p, Vec3(pts[t].Pos)); ok {
			p.ai.tk.pt = t + 1
			return g, k, true
		}
	}
	if best < 0 {
		best = s.aiDefendPoint(p, me, reach)
	}
	if best < 0 {
		p.ai.tk.pt = 0
		return Vec3{}, "", false
	}
	return s.aiTkGoal(p, best)
}

// aiTkGoal: bot p heads for point i, to its own spot inside the circle.
func (s *Server) aiTkGoal(p *Player, i int) (Vec3, string, bool) {
	p.ai.tk.pt = i + 1
	def := s.Map.Points[i]
	a := float64(p.ID) * 2.39996 // the golden angle: teammates spread over the circle
	r := def.Radius * 0.45
	goal := Vec3{def.Pos[0] + r*math.Sin(a), def.Pos[1], def.Pos[2] + r*math.Cos(a)}
	return goal, fmt.Sprintf("pt:%s:%d", def.ID, p.ID), true
}

// aiGarrison is the held point bot p stays to hold while its team attacks
// (-1: p attacks). Of the team's bots on p's island, a share as large as the
// held points' share of the points there to hold or take stays (at least one
// attacks while there is anything to take), the lowest ids first; they
// defend as aiDefendPoint says.
func (s *Server) aiGarrison(p *Player, me int, reach func(int) bool) int {
	held, open := 0, 0
	for i, def := range s.Map.Points {
		if i >= len(s.round.Points) || def.Locked || !reach(i) {
			continue
		}
		if s.round.Points[i].Own == p.Team {
			held++
		} else {
			open++
		}
	}
	if held == 0 || open == 0 {
		return -1
	}
	rank, n := 0, 0
	for _, q := range s.ordered() {
		if q.ai == nil || q.Team != p.Team || q.Dead {
			continue
		}
		if q != p && me >= 0 {
			at := q.ai.isle.at
			if q.ai.isle.atTick == 0 {
				at = s.islandOf(q.Pos)
			}
			if at != me {
				continue
			}
		}
		n++
		if q.ID < p.ID {
			rank++
		}
	}
	keep := min(n-1, int(math.Round(float64(n)*float64(held)/float64(held+open))))
	if rank >= keep {
		return -1
	}
	return s.aiDefendPoint(p, me, reach)
}

// aiDefendPoint is the held point bot p defends (-1 none): a point under
// attack when p is among the nearest aiTkRespond of its team's bots on its
// island, else its post (the held points from the front back, one per bot by
// rank, rotating every aiTkRotate seconds).
func (s *Server) aiDefendPoint(p *Player, me int, reach func(int) bool) int {
	pts := s.Map.Points
	enemyMain := Vec3{}
	for _, def := range pts {
		if def.Locked && def.Owner != p.Team {
			enemyMain = Vec3(def.Pos)
		}
	}
	var held, attacked []int
	for i, def := range pts {
		if i >= len(s.round.Points) || def.Locked || s.round.Points[i].Own != p.Team || !reach(i) {
			continue
		}
		held = append(held, i)
		st := s.round.Points[i]
		enemies := st.A
		if p.Team == "artisan" {
			enemies = st.V
		}
		if enemies > 0 || (st.By != "" && st.By != p.Team) {
			attacked = append(attacked, i)
		}
	}
	if len(held) == 0 {
		return -1
	}
	var mates []*Player // the team's bots on p's island, p among them
	for _, q := range s.ordered() {
		if q.ai == nil || q.Team != p.Team || q.Dead {
			continue
		}
		if q != p && me >= 0 {
			at := q.ai.isle.at
			if q.ai.isle.atTick == 0 {
				at = s.islandOf(q.Pos)
			}
			if at != me {
				continue
			}
		}
		mates = append(mates, q)
	}
	d2 := func(q *Player, i int) float64 {
		return math.Hypot(pts[i].Pos[0]-q.Pos[0], pts[i].Pos[2]-q.Pos[2])
	}
	for _, i := range attacked { // the nearest share answer it
		n := max(1, int(math.Ceil(aiTkRespond*float64(len(mates)))))
		closer := 0
		for _, q := range mates {
			if q != p && (d2(q, i) < d2(p, i) || (d2(q, i) == d2(p, i) && q.ID < p.ID)) {
				closer++
			}
		}
		if closer < n {
			return i
		}
	}
	slices.SortStableFunc(held, func(a, b int) int { // the front first
		da := math.Hypot(pts[a].Pos[0]-enemyMain[0], pts[a].Pos[2]-enemyMain[2])
		db := math.Hypot(pts[b].Pos[0]-enemyMain[0], pts[b].Pos[2]-enemyMain[2])
		switch {
		case da < db:
			return -1
		case da > db:
			return 1
		}
		return 0
	})
	rank := 0
	for _, q := range mates {
		if q.ID < p.ID {
			rank++
		}
	}
	epoch := int(float64(s.tick) / SimHz / aiTkRotate)
	return held[(rank+epoch)%len(held)]
}
