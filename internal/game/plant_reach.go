package game

import (
	"cmp"
	"math"
	"slices"

	"github.com/gigurra/counterslop/internal/content"
)

// defusable: a defender walking from the nav graph (stepping, dropping and
// jumping on the walk grid, no boost) gets to a standing spot from which the
// Big Diff at 'at' can be reverted (within defuseRange across and 1.5 m up
// or down, the defuse rule in roundUse). A plant on a spot only a boost gets
// up to (de_monorepo A's lone 2 m crate) is refused: nobody could defuse it.
//
// Tried in turn: standing at 'at' itself, then the floors around and below it
// (a crate top a defuser can't get onto but can reach across from the floor).
// A walk-grid route ends at most gridSnap from where it was asked to go, so
// every end is checked against the defuse rule. Callers hold s.mu.
func (s *Server) defusable(at Vec3) bool {
	if len(s.Map.Nav) == 0 {
		return true // no nav graph (labs, tests): nothing to check against
	}
	nodes := slices.Clone(s.Map.Nav)
	d := func(n content.NavNode) float64 {
		return math.Hypot(n.Pos[0]-at[0], n.Pos[2]-at[2]) + 3*math.Abs(n.Pos[1]-at[1])
	}
	slices.SortFunc(nodes, func(a, b content.NavNode) int { return cmp.Compare(d(a), d(b)) })
	nodes = nodes[:min(3, len(nodes))]
	reaches := func(to Vec3) bool {
		for _, n := range nodes {
			path, ok := s.gridPath(Vec3(n.Pos), to)
			if !ok {
				continue
			}
			end := path[len(path)-1]
			if math.Hypot(end[0]-at[0], end[2]-at[2]) <= defuseRange-0.1 && math.Abs(end[1]-at[1]) <= 1.5 {
				return true
			}
		}
		return false
	}
	if reaches(at) {
		return true
	}
	// Probes 1.2 m around, 1.4 m down: the grid snaps each to a floor from
	// 3 m below 'at' to 0.8 m below it, the floor beside a crate.
	for i := range 8 {
		a := float64(i) * math.Pi / 4
		if reaches(Vec3{at[0] + 1.2*math.Cos(a), at[1] - 1.4, at[2] + 1.2*math.Sin(a)}) {
			return true
		}
	}
	return false
}

// plantCheck: the last defusable answer for a plant, by spot, so a planter
// holding fire on a refused spot doesn't plan routes every tick.
type plantCheck struct {
	m  string
	at Vec3
	ok bool
}

// plantReachable: defusable(at), remembered while the planter stays within
// 0.1 m of where it was asked on the same map. Callers hold s.mu.
func (s *Server) plantReachable(at Vec3) bool {
	c := &s.plantCheck
	if c.m != s.Map.Name || math.Hypot(c.at[0]-at[0], c.at[2]-at[2]) > 0.1 || math.Abs(c.at[1]-at[1]) > 0.1 {
		*c = plantCheck{m: s.Map.Name, at: at, ok: s.defusable(at)}
	}
	return c.ok
}
