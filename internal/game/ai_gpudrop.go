package game

import (
	"fmt"
	"math"
	"slices"

	"github.com/gigurra/counterslop/internal/content"
)

// GPU Drop bots (docs/gpu-drop.md; the rules are gpudrop.go). They play by
// sending what a human would: walk the nav graph, hold +use standing still
// within content.InGrabReach to grab or restock.
//
//   - Scalpers (Vibe Coders): the team goes for one GPU a round (a lurker now
//     and then for another, like the defusal attackers' sites: pickSite), or
//     the nearest one still on offer once that one is gone; a carrier takes
//     theirs to the exit nearest by route; with nothing left to grab they
//     head for the exit a teammate carrier is making for, to cover it.
//   - Staff (Artisans): half of them (at least one) hold a GPU spot watching
//     the way in, the rest hold a forward post on the scalpers' way to the
//     stock; who does which, and which post, changes every aiStaffShift. A dropped
//     GPU gets restocked by whoever is nearest; while one is carried, half
//     the staff cut the carrier off at its exit, the other half chase it.

// gpuName is the objective name of the i'th GPU (as in content.Map.Objectives).
func gpuName(i int) string { return fmt.Sprintf("gpu%d", i+1) }

// gpuIndex parses gpuName, or -1.
func gpuIndex(name string) int {
	var i int
	if _, err := fmt.Sscanf(name, "gpu%d", &i); err != nil || i < 1 {
		return -1
	}
	return i - 1
}

// objectiveNames: what pickSite chooses from: the bomb sites, or the GPUs.
func (s *Server) objectiveNames() []string {
	var out []string
	if s.gpuMode() {
		for i := range s.Map.GPUs {
			out = append(out, gpuName(i))
		}
		return out
	}
	for _, z := range s.Map.Sites {
		out = append(out, z.Name)
	}
	return out
}

// gpuOpen: a GPU p could go and grab (Vibe Coder) or restock (Artisan) now.
func (s *Server) gpuOpen(p *Player, i int) bool {
	g := s.round.GPUs[i]
	if g.Worker != 0 && g.Worker != p.ID {
		return false // someone else is on it
	}
	if p.Team == content.Artisan {
		return g.State == GPUDropped
	}
	return g.State == GPUShelf || g.State == GPUDropped
}

// nearestGPU: the open GPU nearest to p by route, or -1.
func (s *Server) nearestGPU(p *Player) int {
	best, bi := math.Inf(1), -1
	for i, g := range s.round.GPUs {
		if !s.gpuOpen(p, i) {
			continue
		}
		if d := pathLen(p.Pos, s.navPath(p.Pos, g.Pos)); d < best {
			best, bi = d, i
		}
	}
	return bi
}

// exitCentre is an exit zone's middle, at its floor.
func exitCentre(z content.Zone) Vec3 {
	return Vec3{(z.Min[0] + z.Max[0]) / 2, z.Min[1] + 0.5, (z.Min[2] + z.Max[2]) / 2}
}

// nearestExit: the exit nearest to `from` by route.
func (s *Server) nearestExit(from Vec3) (content.Zone, bool) {
	best, bi := math.Inf(1), -1
	for i, e := range s.Map.Exits {
		if d := pathLen(from, s.navPath(from, exitCentre(e))); d < best {
			best, bi = d, i
		}
	}
	if bi < 0 {
		return content.Zone{}, false
	}
	return s.Map.Exits[bi], true
}

// aiGPU decides a GPU Drop bot's goal in a live round: where to walk (key
// names it, for repathing), or use: stand and hold +use (a grab or restock in
// reach). hold: arrived at a GPU spot to guard (watch the way in).
func (s *Server) aiGPU(p *Player) (goal Vec3, key string, use, hold bool) {
	r := &s.round
	b := p.ai
	inReach := func(i int) bool { return content.InGrabReach([3]float64(p.Pos), [3]float64(r.GPUs[i].Pos)) }
	if p.Team == content.Vibe {
		if i := s.carriedBy(p); i >= 0 {
			if e, ok := s.nearestExit(p.Pos); ok {
				return exitCentre(e), "exit:" + e.Name, false, false
			}
			return
		}
		i := gpuIndex(b.site)
		if i < 0 || i >= len(r.GPUs) || !s.gpuOpen(p, i) {
			i = s.nearestGPU(p)
		}
		if i >= 0 {
			if inReach(i) {
				return Vec3{}, "", true, false
			}
			g := r.GPUs[i]
			return g.Pos, fmt.Sprintf("grab:%d:%.0f,%.0f", i, g.Pos[0], g.Pos[2]), false, false
		}
		// Nothing left to grab: cover a carrier's exit.
		for j := range r.GPUs {
			if r.GPUs[j].State == GPUCarried {
				if e, ok := s.nearestExit(r.GPUs[j].Pos); ok {
					return exitCentre(e), "cover:" + e.Name, false, false
				}
			}
		}
		return
	}
	// The staff. A dropped GPU: the nearest of us restocks it.
	for i, g := range r.GPUs {
		if g.State != GPUDropped || !s.gpuOpen(p, i) {
			continue
		}
		if inReach(i) {
			return Vec3{}, "", true, false
		}
		if s.nearestStaff(g.Pos) == p {
			return g.Pos, fmt.Sprintf("restock:%d:%.0f,%.0f", i, g.Pos[0], g.Pos[2]), false, false
		}
	}
	// A carried GPU: half of us cut it off at its exit, half chase it.
	for _, g := range r.GPUs {
		if g.State != GPUCarried {
			continue
		}
		if p.ID%2 == 0 {
			if e, ok := s.nearestExit(g.Pos); ok {
				return exitCentre(e), "cutoff:" + e.Name, false, false
			}
		}
		return g.Pos, fmt.Sprintf("chase:%.0f,%.0f", math.Round(g.Pos[0]/6)*6, math.Round(g.Pos[2]/6)*6), false, false
	}
	// Otherwise hold our GPU spot, or a forward post on this shift.
	if i := gpuIndex(b.site); i >= 0 && i < len(s.Map.GPUs) {
		spot := Vec3(s.Map.GPUs[i].Pos)
		if at, ok := s.staffPost(p, spot); ok {
			return at, fmt.Sprintf("post:%.1f,%.1f,%.1f", at[0], at[1], at[2]), false, true
		}
		return spot, "hold:" + b.site, false, true
	}
	return
}

// aiStaffShift: how long a staff bot keeps its role (the GPU or a forward
// post) before the staff swap round.
const aiStaffShift = 25 * SimHz

// aiPostOut, aiPostSpawn, aiPostRoute: a forward post is at least aiPostOut
// from every GPU (out of the stockroom), aiPostSpawn from the scalpers'
// spawns, and at most aiPostRoute from its spot by route.
const aiPostOut, aiPostSpawn, aiPostRoute = 20.0, 10.0, 70.0

// nearGPU: at is within r of a GPU on its floor.
func (s *Server) nearGPU(at Vec3, r float64) bool {
	for _, g := range s.Map.GPUs {
		if math.Hypot(at[0]-g.Pos[0], at[2]-g.Pos[2]) < r && math.Abs(at[1]-g.Pos[1]) < 2.5 {
			return true
		}
	}
	return false
}

// nearSpawn: at is within r of one of team's spawns (straight line).
func (s *Server) nearSpawn(team string, at Vec3, r float64) bool {
	for _, sp := range s.Map.TeamSpawns[team] {
		if math.Hypot(at[0]-sp[0], at[2]-sp[2]) < r {
			return true
		}
	}
	return false
}

// aiPost: a staff bot's forward post, picked for a shift of a round.
type aiPost struct {
	round int
	shift int64
	at    Vec3
	on    bool
}

// staffPost: the forward post p holds this shift instead of its GPU spot, if
// any. The living staff bots, in join order, take turns: each shift the
// first half (rounded down, so a lone guard stays home) after a rotating
// offset go out, each to a seeded post of the GPU spot's (stockPosts).
func (s *Server) staffPost(p *Player, spot Vec3) (Vec3, bool) {
	b := p.ai
	shift := s.tick / aiStaffShift
	if b.post.round != s.round.Number || b.post.shift != shift {
		var staff []*Player
		for _, q := range s.ordered() {
			if q.ai != nil && !q.Dead && q.Team == content.Artisan {
				staff = append(staff, q)
			}
		}
		b.post = aiPost{round: s.round.Number, shift: shift}
		if n := len(staff); n > 1 {
			rank := slices.Index(staff, p)
			if posts := s.stockPosts(spot); rank >= 0 && len(posts) > 0 && (rank+int(shift)+s.round.Number)%n < n/2 {
				b.post.at, b.post.on = posts[b.r.Int(0, len(posts)-1)], true
			}
		}
	}
	return b.post.at, b.post.on
}

// stockPosts: where staff hold forward of a GPU spot: the nav nodes out of
// the stock (aiPostOut from every GPU, or on another floor), off the
// scalpers' spawn (aiPostSpawn: no spawn camping), where the staff get
// aiHoldMargin ahead of the scalpers (from the spawns, by route, as siteHold)
// and at most aiPostRoute from the spot by route (cached per spot).
func (s *Server) stockPosts(spot Vec3) []Vec3 {
	key := fmt.Sprintf("%s:%.1f,%.1f,%.1f", s.Map.Name, spot[0], spot[1], spot[2])
	if p, ok := s.aiPosts[key]; ok {
		return p
	}
	att := s.attackerDist()
	home := s.Map.TeamSpawns[content.Artisan]
	var out []Vec3
	for _, n := range s.Map.Nav {
		at := Vec3(n.Pos)
		if s.nearGPU(at, aiPostOut) || s.nearSpawn(content.Vibe, at, aiPostSpawn) {
			continue
		}
		if d, ok := att[at]; !ok || len(home) == 0 || pathLen(Vec3(home[0]), s.navPath(Vec3(home[0]), at))+aiHoldMargin > d {
			continue // the scalpers get there first: no time to set up
		}
		if r := pathLen(spot, s.navPath(spot, at)); r <= aiPostRoute {
			out = append(out, at)
		}
	}
	if s.aiPosts == nil {
		s.aiPosts = map[string][]Vec3{}
	}
	s.aiPosts[key] = out
	return out
}

// nearestStaff: the living Artisan bot nearest to at (straight line).
func (s *Server) nearestStaff(at Vec3) *Player {
	var best *Player
	bd := math.Inf(1)
	for _, q := range s.ordered() {
		if q.ai == nil || q.Dead || q.Team != content.Artisan {
			continue
		}
		if d := math.Hypot(q.Pos[0]-at[0], q.Pos[2]-at[2]); d < bd {
			best, bd = q, d
		}
	}
	return best
}

// gpuWatch: where a staff bot guarding a GPU spot looks: the open nav node
// in sight of the spot that the scalpers reach first (cached per spot).
func (s *Server) gpuWatch(spot Vec3) (Vec3, bool) {
	key := fmt.Sprintf("%s:%.1f,%.1f,%.1f", s.Map.Name, spot[0], spot[1], spot[2])
	if w, ok := s.aiGPUWatch[key]; ok {
		return w, w != (Vec3{})
	}
	dist := s.attackerDist()
	eye := Vec3{spot[0], spot[1] + 0.5, spot[2]}
	var best Vec3
	bd := math.Inf(1)
	for _, n := range s.Map.Nav {
		at := Vec3(n.Pos)
		d, ok := dist[at]
		if !ok || math.Hypot(at[0]-spot[0], at[2]-spot[2]) > 30 || math.Hypot(at[0]-spot[0], at[2]-spot[2]) < 3 {
			continue
		}
		if d < bd && s.lineClear(eye, Vec3{at[0], at[1] + 1.2, at[2]}) {
			best, bd = Vec3{at[0], at[1] + 1.0, at[2]}, d
		}
	}
	if s.aiGPUWatch == nil {
		s.aiGPUWatch = map[string]Vec3{}
	}
	s.aiGPUWatch[key] = best
	return best, best != (Vec3{})
}
