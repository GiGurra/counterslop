package game

import (
	"math"
	"slices"

	"github.com/gigurra/counterslop/internal/content"
)

// The defenders' plan for a round (Artisan bots), drawn once per round from
// the first of them to think, so the attackers can't learn one setup:
//
//   - Split: every site gets its share of the team (2-1-2 style: 5 over two
//     sites is 3-2 or 2-3), and now and then a stack (one more on a site, never
//     leaving a site empty while there are enough of us), rarer a gamble (a
//     site's lone defender stacks the other: 2-1 becomes 3-0).
//   - Spots: each site has several hold spots (siteSpots: around its hold,
//     a few metres apart, each in sight of where the attackers come in, no
//     deeper in than the hold, reached well before them); its defenders take
//     different ones, shuffled every round. Spread wider (crossfires, deep
//     corners) they won too much: de_pipeline's attackers 53% -> 30% (3v3 bots).
//   - Forward: now and then one of a site's defenders (not its only one) holds
//     the attackers' way in further out, for aiForwardSecs, then falls back to
//     a spot on the site.
//
// Rotations (aiRotate) still move a defender to the site under attack, to one
// of that site's spots.

const (
	aiStackChance   = 0.25 // a round where one site gets an extra defender from another
	aiGambleChance  = 0.12 // a round (without a stack) where a site's lone defender stacks another (aiStackChance never empties one)
	aiForwardChance = 0.3  // a round where one defender holds forward
	aiForwardSecs   = 20.0 // a forward defender falls back after this long into the round
	aiSpotMax       = 5    // hold spots per site
	aiSpotGap       = 3.0  // metres between a site's hold spots at least
	aiSpotNear      = 8.0  // and at most this far from the first (variety, not a full crossfire)
	aiSpotDeep      = 3.0  // and at most this much further from the site's entrance than the first
	aiSpotAround    = 6.0  // spots may be this far outside the site zone
	aiSpotRise      = 1.0  // and this far above or below the hold's floor (a stage, steps)
	aiSpotStep      = 0.6  // walking between spots: a step at most from one 1.5 m cell to the next
	aiSpotNav       = 15.0 // a nav node this close (walking straight there) reaches a spot
	aiSpotMargin    = 6.0  // metres of walking a spot must be ahead of the attackers (the hold itself: aiHoldMargin)
	aiForwardMin    = 12.0 // a forward spot is at least this far (walking) out from the site's hold
	aiForwardMax    = 35.0 // and at most this far
)

// aiDefPost: where one defender plays this round.
type aiDefPost struct {
	site    string
	at      Vec3
	forward bool // out on the attackers' way in (until aiForwardSecs)
	watch   Vec3 // forward: where the attackers come from
	back    Vec3 // forward: its spot on the site, after aiForwardSecs
}

type aiDefPlan struct {
	round int
	posts map[int]aiDefPost // by player id
}

// defPlan: this round's plan, drawn with p's rng if there is none yet.
// Callers hold s.mu.
func (s *Server) defPlan(p *Player) *aiDefPlan {
	if s.aiDefense.round == s.round.Number && s.aiDefense.posts != nil {
		return &s.aiDefense
	}
	s.aiDefense = aiDefPlan{round: s.round.Number, posts: map[int]aiDefPost{}}
	names := s.objectiveNames()
	var defs []*Player
	for _, q := range s.ordered() {
		if q.ai != nil && q.Team == p.Team && !q.Dead {
			defs = append(defs, q)
		}
	}
	if len(names) == 0 || len(defs) == 0 {
		return &s.aiDefense
	}
	r := p.ai.r
	counts := splitDefenders(len(defs), len(names), r.Float, func(n int) int { return r.Int(0, n-1) })
	// Who goes where: a shuffled team, dealt out by the counts.
	for i := len(defs) - 1; i > 0; i-- {
		j := r.Int(0, i)
		defs[i], defs[j] = defs[j], defs[i]
	}
	forwardDone := r.Float() >= aiForwardChance
	k := 0
	for si, name := range names {
		spots := slices.Clone(s.siteSpots(name))
		for i := len(spots) - 1; i > 0; i-- {
			j := r.Int(0, i)
			spots[i], spots[j] = spots[j], spots[i]
		}
		for n := range counts[si] {
			q := defs[k]
			k++
			post := aiDefPost{site: name, at: spots[n%len(spots)]}
			if !forwardDone && counts[si] >= 2 && n == counts[si]-1 {
				if at, watch, ok := s.forwardSpot(name); ok {
					post.back = post.at
					post.at, post.watch, post.forward = at, watch, true
					forwardDone = true
				}
			}
			s.aiDefense.posts[q.ID] = post
		}
	}
	return &s.aiDefense
}

// splitDefenders: how many of n defenders play each of k sites: an even share
// (the odd ones out to random sites); with aiStackChance a stack (a site
// gets one more than the biggest share, never emptying another), else with
// aiGambleChance a site's lone defender moved to another (emptying it).
func splitDefenders(n, k int, float func() float64, pick func(int) int) []int {
	counts := make([]int, k)
	for i := range counts {
		counts[i] = n / k
	}
	extra := make([]int, k)
	for i := range extra {
		extra[i] = i
	}
	for i := k - 1; i > 0; i-- {
		j := pick(i + 1)
		extra[i], extra[j] = extra[j], extra[i]
	}
	for i := range n % k {
		counts[extra[i]]++
	}
	switch {
	case k < 2:
	case float() < aiStackChance:
		// A stack: one site gets one more than the biggest share, from
		// the others (from those with two or more: none is emptied).
		to := pick(k)
		want := slices.Max(counts) + 1
		for counts[to] < want {
			var from []int
			for i, c := range counts {
				if i != to && c >= 2 {
					from = append(from, i)
				}
			}
			if len(from) == 0 {
				break
			}
			counts[from[pick(len(from))]]--
			counts[to]++
		}
	case float() < aiGambleChance:
		// A read: a site's lone defender stacks another (the site is
		// empty if the attackers come there; a 3-man team's 2-1 becomes 3-0).
		from := pick(k)
		if counts[from] == 1 {
			counts[from]--
			counts[(from+1+pick(k-1))%k]++
		}
	}
	return counts
}

// defPost: where p plays this round (its plan post, or after a rotation or
// for a bot that wasn't there when the plan was drawn, a spot at its site).
// Callers hold s.mu.
func (s *Server) defPost(p *Player) aiDefPost {
	plan := s.defPlan(p)
	post, ok := plan.posts[p.ID]
	if ok && post.site == p.ai.site {
		if post.forward && s.liveSecs() >= aiForwardSecs {
			post = aiDefPost{site: post.site, at: post.back}
			plan.posts[p.ID] = post
		}
		return post
	}
	spots := s.siteSpots(p.ai.site)
	post = aiDefPost{site: p.ai.site, at: spots[p.ai.r.Int(0, len(spots)-1)]}
	plan.posts[p.ID] = post
	return post
}

// liveSecs: seconds since the round went live.
func (s *Server) liveSecs() float64 {
	return s.cvarF("mp_roundtime", 115) - s.round.endsAt.Sub(s.now).Seconds()
}

// siteSpots: the site's hold spots, cached per map. The first is its
// siteHold (or, where that is inside a prop, the nearest open ground to it);
// the rest are open ground on its floor (or a step off it) walked to from
// it or from a nav node (around cover, up a step), at least aiSpotGap from each other
// and within aiSpotNear of the first, no more than aiSpotDeep further from the
// site's entrance than it, reached aiSpotMargin before the attackers and in sight
// of the entrance.
func (s *Server) siteSpots(name string) []Vec3 {
	key := s.Map.Name + "/" + name
	if sp, ok := s.aiSpots[key]; ok {
		return sp
	}
	hold := s.siteHold(name)
	spots := []Vec3{hold}
	z := s.siteZone(name)
	def, att := s.Map.TeamSpawns[content.Artisan], s.Map.TeamSpawns[content.Vibe]
	ent, hasEnt := s.siteEntrance(name)
	if z != nil && len(def) > 0 && len(att) > 0 {
		from, foe := Vec3(def[0]), Vec3(att[0])
		ground := s.floorUnder(hold)
		// A grid over and around the site, and around the hold (short of
		// the site where the attackers get there first).
		const step = 1.5
		x0, x1 := math.Min(z.Min[0]-aiSpotAround, hold[0]-aiSpotAround), math.Max(z.Max[0]+aiSpotAround, hold[0]+aiSpotAround)
		z0, z1 := math.Min(z.Min[2]-aiSpotAround, hold[2]-aiSpotAround), math.Max(z.Max[2]+aiSpotAround, hold[2]+aiSpotAround)
		nx, nz := int((x1-x0)/step)+1, int((z1-z0)/step)+1
		floor := make([]float64, nx*nz)
		at := func(i, j int) Vec3 { return Vec3{x0 + float64(i)*step, floor[i*nz+j], z0 + float64(j)*step} }
		open := make([]bool, nx*nz)
		start, sd := -1, math.Inf(1) // the open cell nearest the hold
		for i := range nx {
			for j := range nz {
				floor[i*nz+j] = s.floorUnder(Vec3{x0 + float64(i)*step, ground, z0 + float64(j)*step})
				q := at(i, j)
				if math.Abs(q[1]-ground) > aiSpotRise || !s.pointFree(q, 0.6) {
					continue
				}
				open[i*nz+j] = true
				if d := math.Hypot(q[0]-hold[0], q[2]-hold[2]); d < sd {
					start, sd = i*nz+j, d
				}
			}
		}
		if start >= 0 && !s.pointFree(Vec3{hold[0], ground, hold[2]}, 0.6) {
			spots[0] = at(start/nz, start%nz)
		} else {
			spots[0][1] = ground
		}
		// Flood from there, and from every cell a nav node walks to straight:
		// what can be walked to.
		var cands []Vec3
		seen := make([]bool, nx*nz)
		queue := []int{}
		if start >= 0 {
			queue = append(queue, start)
		}
		for c := range open {
			if !open[c] || c == start {
				continue
			}
			q := at(c/nz, c%nz)
			for _, n := range s.Map.Nav {
				if math.Hypot(n.Pos[0]-q[0], n.Pos[2]-q[2]) < aiSpotNav && math.Abs(n.Pos[1]-q[1]) <= aiSpotStep && s.segmentClear(Vec3(n.Pos), q, 0.5) {
					queue = append(queue, c)
					break
				}
			}
		}
		for ; len(queue) > 0; queue = queue[1:] {
			c := queue[0]
			if seen[c] {
				continue
			}
			seen[c] = true
			i, j := c/nz, c%nz
			q := at(i, j)
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				a, b := i+d[0], j+d[1]
				if a >= 0 && a < nx && b >= 0 && b < nz && open[a*nz+b] && !seen[a*nz+b] && math.Abs(floor[a*nz+b]-q[1]) <= aiSpotStep && s.segmentClear(q, at(a, b), 0.5) {
					queue = append(queue, a*nz+b)
				}
			}
			if math.Hypot(q[0]-spots[0][0], q[2]-spots[0][2]) > aiSpotNear {
				continue
			}
			if hasEnt {
				// Not deeper in than the hold: further back from where they
				// come in, a defender sees them first, every time.
				if math.Hypot(q[0]-ent[0], q[2]-ent[2]) > math.Hypot(spots[0][0]-ent[0], spots[0][2]-ent[2])+aiSpotDeep {
					continue
				}
			}
			if hasEnt && !s.lineClear(Vec3{q[0], q[1] + 1.2, q[2]}, Vec3{ent[0], ent[1] + 1.0, ent[2]}) {
				continue
			}
			if pathLen(from, s.navPath(from, q))+aiSpotMargin > pathLen(foe, s.navPath(foe, q)) {
				continue
			}
			cands = append(cands, q)
		}
		// Farthest-point picks: spread out, not all in one corner.
		for len(spots) < aiSpotMax {
			best, bd := -1, aiSpotGap
			for i, q := range cands {
				d := math.Inf(1)
				for _, o := range spots {
					d = math.Min(d, math.Hypot(q[0]-o[0], q[2]-o[2]))
				}
				if d > bd {
					best, bd = i, d
				}
			}
			if best < 0 {
				break
			}
			spots = append(spots, cands[best])
		}
	}
	if s.aiSpots == nil {
		s.aiSpots = map[string][]Vec3{}
	}
	s.aiSpots[key] = spots
	return spots
}

// pointFree: a disc of radius r at q overlaps no box at walking height.
func (s *Server) pointFree(q Vec3, r float64) bool {
	lo, hi := q[1]+0.45, q[1]+1.7
	for i := range s.Map.Boxes {
		b := &s.Map.Boxes[i]
		if b.Max[1] > lo && b.Min[1] < hi && q[0] > b.Min[0]-r && q[0] < b.Max[0]+r && q[2] > b.Min[2]-r && q[2] < b.Max[2]+r {
			return false
		}
	}
	return true
}

// floorUnder: the top of the highest box under q up to a player's height
// over its feet (a stage the site's middle is on; not a roof or a deck
// above).
func (s *Server) floorUnder(q Vec3) float64 {
	top := math.Inf(-1)
	for i := range s.Map.Boxes {
		b := &s.Map.Boxes[i]
		if q[0] >= b.Min[0] && q[0] <= b.Max[0] && q[2] >= b.Min[2] && q[2] <= b.Max[2] && b.Max[1] <= q[1]+1.6 && b.Max[1] > top {
			top = b.Max[1]
		}
	}
	if math.IsInf(top, -1) {
		return q[1]
	}
	return top
}

// forwardSpot: a waypoint on the attackers' route to the site, between
// aiForwardMin and aiForwardMax walking out from its hold, that the
// defenders still reach aiHoldMargin before them (the furthest such), and
// where it looks: the route's waypoint before it. Cached per map.
func (s *Server) forwardSpot(name string) (Vec3, Vec3, bool) {
	key := s.Map.Name + "/" + name
	if f, ok := s.aiForward[key]; ok {
		return f[0], f[1], f[0] != f[1]
	}
	var at, watch Vec3
	def, att := s.Map.TeamSpawns[content.Artisan], s.Map.TeamSpawns[content.Vibe]
	if len(def) > 0 && len(att) > 0 {
		from, foe, hold := Vec3(def[0]), Vec3(att[0]), s.siteHold(name)
		route := append([]Vec3{foe}, s.navPath(foe, hold)...)
		for i := 1; i < len(route)-1; i++ {
			w := route[i]
			out := pathLen(hold, s.navPath(hold, w))
			if out < aiForwardMin || out > aiForwardMax {
				continue
			}
			if pathLen(from, s.navPath(from, w))+aiHoldMargin > pathLen(foe, route[1:i+1]) {
				continue
			}
			at, watch = w, route[i-1]
			break // the first from their side: the furthest out
		}
	}
	if s.aiForward == nil {
		s.aiForward = map[string][2]Vec3{}
	}
	s.aiForward[key] = [2]Vec3{at, watch}
	return at, watch, at != watch
}
