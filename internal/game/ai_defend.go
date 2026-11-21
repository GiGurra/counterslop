package game

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// Defending bots (Artisans): they share what they see, hold an angle on the
// way the attackers come in, and rotate to the site under attack.
//
//   - Intel: whenever a bot sees an enemy, its team knows where (for a few
//     seconds), like a callout or the radar's spotted dot.
//   - Holding: a defender goes to its site's hold point: the site's middle, or,
//     where the attackers would get there first (de_standup's B is next to
//     their spawn), the furthest point on the way it reaches well before them.
//   - Watching: a defender walking to or standing on its site looks at the
//     freshest nearby intel, else at the site's entrance (where the attackers'
//     route from their spawn enters the site), instead of where it walks.
//   - Rotating: a fresh sighting at the other site, while ours has been quiet,
//     sends defenders there, once a round.
//   - Retaking: after a plant, a defender close to the Big Diff waits for a
//     teammate to join it before going in (while the timer allows).

const (
	aiIntelTicks   = 4 * SimHz // how long a sighting stays news
	aiIntelRange   = 40.0      // watch intel closer than this
	aiRotateRange  = 26.0      // a sighting this close to a site's middle (the nearer one) is at that site
	aiQuietTicks   = 8 * SimHz // our site counts as quiet after this long without a sighting
	aiEntranceSlop = 2.0       // a route waypoint this far outside the site zone is its entrance
)

type aiIntel struct {
	pos   Vec3
	tick  int64
	round int
}

// noteIntel: a bot of team saw an enemy at pos. Callers hold s.mu.
func (s *Server) noteIntel(team string, pos Vec3) {
	if s.aiIntel == nil {
		s.aiIntel = map[string]aiIntel{}
	}
	s.aiIntel[team] = aiIntel{pos: pos, tick: s.tick, round: s.round.Number}
	if site := s.nearSite(pos); site != "" {
		s.aiIntel[team+"/"+site] = aiIntel{pos: pos, tick: s.tick, round: s.round.Number}
	}
}

// nearSite: the site pos is at or close to (within aiRotateRange of its
// middle, the nearer one), or "".
func (s *Server) nearSite(pos Vec3) string {
	best, bd := "", aiRotateRange
	for _, z := range s.Map.Sites {
		c := z.Center()
		if d := math.Hypot(pos[0]-c[0], pos[2]-c[2]); d < bd {
			best, bd = z.Name, d
		}
	}
	return best
}

// seenAt: how many ticks ago team last saw an enemy at site this round (-1: not yet).
func (s *Server) seenAt(team, site string) int64 {
	in, ok := s.aiIntel[team+"/"+site]
	if !ok || in.round != s.round.Number {
		return -1
	}
	return s.tick - in.tick
}

// intel is team's fresh sighting, if any.
func (s *Server) intel(team string) (Vec3, bool) {
	in, ok := s.aiIntel[team]
	if !ok || in.round != s.round.Number || s.tick-in.tick > aiIntelTicks {
		return Vec3{}, false
	}
	return in.pos, true
}

// siteEntrance: where the attackers' route from their spawn enters the site
// (the last waypoint outside it), cached per map.
func (s *Server) siteEntrance(name string) (Vec3, bool) {
	key := s.Map.Name + "/" + name
	if e, ok := s.aiEntrance[key]; ok {
		return e, true
	}
	z := s.siteZone(name)
	spawns := s.Map.TeamSpawns[content.Vibe]
	if z == nil || len(spawns) == 0 {
		return Vec3{}, false
	}
	path := s.navPath(Vec3(spawns[0]), s.mustSite(name))
	e, found := Vec3{}, false
	for _, w := range path {
		if w[0] < z.Min[0]-aiEntranceSlop || w[0] > z.Max[0]+aiEntranceSlop || w[2] < z.Min[2]-aiEntranceSlop || w[2] > z.Max[2]+aiEntranceSlop {
			e, found = w, true
		}
	}
	if !found {
		return Vec3{}, false
	}
	if s.aiEntrance == nil {
		s.aiEntrance = map[string]Vec3{}
	}
	s.aiEntrance[key] = e
	return e, true
}

// aiRotate: a defender whose team has fresh intel near the other site goes
// there. Callers hold s.mu.
func (s *Server) aiRotate(p *Player) {
	if p.ai.rotated == s.round.Number {
		return // once a round: rotating back and forth on every callout gets you killed in between
	}
	mine := s.seenAt(p.Team, p.ai.site)
	if mine >= 0 && mine < aiQuietTicks {
		return // ours is busy too: stay
	}
	for _, z := range s.Map.Sites {
		if t := s.seenAt(p.Team, z.Name); z.Name != p.ai.site && t >= 0 && t <= aiIntelTicks {
			p.ai.site, p.ai.rotated = z.Name, s.round.Number
			return
		}
	}
}

// aiWatch is where a defender without a target looks: fresh intel nearby,
// else its site's entrance. Callers hold s.mu.
func (s *Server) aiWatch(p *Player) (Vec3, bool) {
	if at, ok := s.intel(p.Team); ok && math.Hypot(at[0]-p.Pos[0], at[2]-p.Pos[2]) < aiIntelRange {
		return Vec3{at[0], at[1] + 1.25, at[2]}, true
	}
	if len(p.ai.path) > 0 { // on the way: the nearest open way the attackers could come from
		if at, ok := s.threatNode(p); ok {
			return Vec3{at[0], at[1] + 1.0, at[2]}, true
		}
	}
	if e, ok := s.siteEntrance(p.ai.site); ok {
		return Vec3{e[0], e[1] + 1.0, e[2]}, true
	}
	return Vec3{}, false
}

// attackerDist: walking distance from the attackers' spawn to each nav node,
// cached per map.
func (s *Server) attackerDist() map[Vec3]float64 {
	key := s.Map.Name
	if d, ok := s.aiAttDist[key]; ok {
		return d
	}
	d := map[Vec3]float64{}
	if sp := s.Map.TeamSpawns[content.Vibe]; len(sp) > 0 {
		from := Vec3(sp[0])
		for _, n := range s.Map.Nav {
			d[Vec3(n.Pos)] = pathLen(from, s.navPath(from, Vec3(n.Pos)))
		}
	}
	if s.aiAttDist == nil {
		s.aiAttDist = map[string]map[Vec3]float64{}
	}
	s.aiAttDist[key] = d
	return d
}

// threatNode: the nearest nav node p can see (within aiIntelRange) that the
// attackers reach before p's own spot and that isn't on p's way. Callers hold s.mu.
func (s *Server) threatNode(p *Player) (Vec3, bool) {
	dist := s.attackerDist()
	mine := math.Inf(1) // the attackers' distance to the node nearest p
	nearest := math.Inf(1)
	for n, d := range dist {
		if h := math.Hypot(n[0]-p.Pos[0], n[2]-p.Pos[2]); h < nearest {
			nearest, mine = h, d
		}
	}
	eye := s.eyeOf(p)
	best, bd, found := Vec3{}, aiIntelRange, false
	for _, nd := range s.Map.Nav {
		n := Vec3(nd.Pos)
		h := math.Hypot(n[0]-p.Pos[0], n[2]-p.Pos[2])
		if h < 4 || h >= bd || dist[n] >= mine || onPath(p.ai.path, n) || !s.lineClear(eye, Vec3{n[0], n[1] + 1.2, n[2]}) {
			continue
		}
		best, bd, found = n, h, true
	}
	return best, found
}

func onPath(path []Vec3, n Vec3) bool {
	for _, w := range path {
		if w == n {
			return true
		}
	}
	return false
}

// lookAt turns p's view toward at. Callers hold s.mu.
func (s *Server) lookAt(p *Player, at Vec3, step float64) {
	o := s.eyeOf(p)
	d := Vec3{at[0] - o[0], at[1] - o[1], at[2] - o[2]}
	if math.Hypot(d[0], d[2]) < 1 {
		return
	}
	p.ai.turnTo(math.Atan2(-d[0], -d[2]), math.Atan2(d[1], math.Hypot(d[0], d[2])), step)
}

// facing: q looks roughly at p (within ~50°): no time to stand and throw a
// grenade at them. Callers hold s.mu.
func (s *Server) facing(q, p *Player) bool {
	yaw := q.Yaw
	if q.body != nil {
		yaw = q.body.Yaw
	}
	return math.Abs(angleDiff(math.Atan2(-(p.Pos[0]-q.Pos[0]), -(p.Pos[2]-q.Pos[2])), yaw)) < 0.9
}

const (
	aiRetakeWait  = 22.0 // closer than this to the planted Big Diff: wait for a teammate
	aiRetakeGroup = 9.0  // a teammate this close counts as together
)

// aiWaitForRetake: p (an Artisan going for the planted Big Diff) should hold
// where it is until a teammate catches up. Callers hold s.mu.
func (s *Server) aiWaitForRetake(p *Player) bool {
	b := s.round.Bomb
	if math.Hypot(p.Pos[0]-b.Pos[0], p.Pos[2]-b.Pos[2]) > aiRetakeWait {
		return false
	}
	need := defuseSecs
	if p.Kit {
		need /= 2
	}
	if s.round.endsAt.Sub(s.now).Seconds() < need+7 {
		return false // no time left to wait
	}
	mates := 0
	for _, q := range s.ordered() {
		if q == p || q.Dead || q.Team != p.Team || q.ai == nil && !q.ready {
			continue
		}
		mates++
		if math.Hypot(q.Pos[0]-p.Pos[0], q.Pos[2]-p.Pos[2]) < aiRetakeGroup {
			return false // together: go
		}
	}
	return mates > 0 // alone in the team: nobody to wait for
}

const (
	aiHoldMargin = 14.0 // metres of walking a hold point must be ahead of the attackers
	aiHoldBack   = 5.0  // metres behind the site's middle, away from its entrance
)

// pathLen is the length of from followed by path.
func pathLen(from Vec3, path []Vec3) float64 {
	d, at := 0.0, from
	for _, w := range path {
		d += math.Hypot(w[0]-at[0], w[2]-at[2])
		at = w
	}
	return d
}

// siteHold: where defenders of the named site stand (see above), cached per map.
func (s *Server) siteHold(name string) Vec3 {
	key := s.Map.Name + "/" + name
	if h, ok := s.aiHold[key]; ok {
		return h
	}
	center := s.mustSite(name)
	hold := center
	def, att := s.Map.TeamSpawns[content.Artisan], s.Map.TeamSpawns[content.Vibe]
	if len(def) > 0 && len(att) > 0 {
		from := Vec3(def[0])
		route := append([]Vec3{from}, s.navPath(from, center)...)
		ahead := func(q Vec3, walked float64) bool {
			return walked+aiHoldMargin <= pathLen(Vec3(att[0]), s.navPath(Vec3(att[0]), q))
		}
		if !ahead(center, pathLen(from, route[1:])) {
			hold = from
			walked := 0.0
			for i := 1; i < len(route); i++ {
				a, b := route[i-1], route[i]
				seg := math.Hypot(b[0]-a[0], b[2]-a[2])
				for t := 1.0; t <= seg; t++ {
					q := Vec3{a[0] + (b[0]-a[0])*t/seg, a[1] + (b[1]-a[1])*t/seg, a[2] + (b[2]-a[2])*t/seg}
					if !ahead(q, walked+t) {
						goto done
					}
					hold = q
				}
				walked += seg
			}
		}
	}
done:
	// Holding the site itself: stand back from where they come in (more time
	// to see them), on open ground inside it.
	if hold == center {
		if e, ok := s.siteEntrance(name); ok {
			z := s.siteZone(name)
			dx, dz := center[0]-e[0], center[2]-e[2]
			for k := aiHoldBack; k >= 2 && hold == center; k-- { // as far back as there is room
				l := math.Hypot(dx, dz)
				if l == 0 {
					break
				}
				q := Vec3{center[0] + dx/l*k, center[1], center[2] + dz/l*k}
				q[0] = math.Max(z.Min[0]+1.5, math.Min(z.Max[0]-1.5, q[0]))
				q[2] = math.Max(z.Min[2]+1.5, math.Min(z.Max[2]-1.5, q[2]))
				if s.segmentClear(center, q, 0.5) && math.Abs(s.groundTop(q[0], q[2])-s.groundTop(center[0], center[2])) < 0.5 {
					hold = q
				}
			}
		}
	}
	if s.aiHold == nil {
		s.aiHold = map[string]Vec3{}
	}
	s.aiHold[key] = hold
	return hold
}
