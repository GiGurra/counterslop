package game

import (
	"fmt"
	"math"

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
//   - Staff (Artisans): each holds a GPU spot watching the way in; a dropped
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
	// Otherwise hold our GPU spot.
	if i := gpuIndex(b.site); i >= 0 && i < len(s.Map.GPUs) {
		return Vec3(s.Map.GPUs[i].Pos), "hold:" + b.site, false, true
	}
	return
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
