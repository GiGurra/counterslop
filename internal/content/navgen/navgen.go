// Package navgen builds a map's bot nav graph from its geometry, for maps
// that come without one (custom maps; docs/custom-maps.md, internal/game
// changeLevel): content.Map.Nav, the graph bots route on between spawns,
// chokes and objectives.
//
//  1. A walk grid over the whole map (cell metres): per column, the box tops a
//     standing hull fits on (ramps where the hull stands on the slope), none
//     under water (bots don't swim).
//  2. Two-way walking links between neighbouring floors (a step up or down,
//     no jumps or drops: a nav link is walked both ways), flood-filled from
//     every team spawn: the reachable floor.
//  3. Clearance: each reachable floor's distance (in cells) to the edge of the
//     reachable floor (a wall, a ledge).
//  4. Nodes: "vibe" and "artisan" at each team's first spawn, one per
//     objective zone named after it (a site "A", GPU Drop's gpuN and exits)
//     at its most open reachable spot, and one per block (Block metres
//     square, per floor level) at the block's most open spot.
//  5. Links: two nodes within LinkRange whose straight line walks on the
//     reachable floor. Pieces left apart (a corridor round a corner) are
//     joined along a grid route, string-pulled into new nodes.
//
// Everything is deterministic (no map iteration order, no randomness): the
// same map gives the same graph.
package navgen

import (
	"container/heap"
	"fmt"
	"math"
	"slices"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

const (
	cell      = 0.25 // metres per grid column
	stepUp    = 0.5  // a rise a walking hull steps (sim stepUp 0.55)
	Block     = 8.0  // metres: one lattice node per block and floor level
	LinkRange = 13.0 // metres across: the longest straight link
	levelH    = 3.0  // metres: floors this far apart are different levels
	nearSpec  = 4.0  // a lattice node this close to a named one is dropped
	minClear  = 2    // cells: a lattice node at least this far from an edge if it can
)

// Build returns a nav graph for m (m.Nav is ignored). Nil if m has no team
// spawns or no floor at them.
func Build(m *c.Map) []c.NavNode {
	g := newGrid(m)
	if g == nil {
		return nil
	}
	var seeds []int
	for _, team := range c.Teams {
		for _, sp := range m.TeamSpawns[team] {
			if n, ok := g.nearest(sp); ok {
				seeds = append(seeds, n)
			}
		}
	}
	if len(seeds) == 0 {
		return nil
	}
	g.flood(seeds)
	g.clearance()
	out := g.graph(m)
	return out
}

// grid is the walk grid: floors per column, flattened into nodes.
type grid struct {
	x0, z0  float64
	nx, nz  int
	start   []int     // per column: its first node
	y       []float64 // per node: the floor's height
	col     []int     // per node: its column
	reach   []bool    // per node: reachable from a spawn
	clear   []int     // per node: cells to the reachable floor's edge
	boxes   []c.Box
	buckets map[[2]int][]int // 4 m buckets of box indices
	// route's scratch
	rCost        []float64
	rPrev, rSeen []int32
	rGen         int32
	routes       int // route calls (a test's budget)
}

const bucket = 4.0

func (g *grid) bucketOf(x, z float64) [2]int {
	return [2]int{int(math.Floor(x / bucket)), int(math.Floor(z / bucket))}
}

// near: the indices of the boxes that may overlap a hull at (x, z).
func (g *grid) near(x, z float64) []int { return g.buckets[g.bucketOf(x, z)] }

func newGrid(m *c.Map) *grid {
	if len(m.Boxes) == 0 {
		return nil
	}
	lo, hi := [2]float64{math.Inf(1), math.Inf(1)}, [2]float64{math.Inf(-1), math.Inf(-1)}
	for _, b := range m.Boxes {
		lo[0], lo[1] = math.Min(lo[0], b.Min[0]), math.Min(lo[1], b.Min[2])
		hi[0], hi[1] = math.Max(hi[0], b.Max[0]), math.Max(hi[1], b.Max[2])
	}
	g := &grid{x0: lo[0], z0: lo[1], boxes: m.Boxes, buckets: map[[2]int][]int{}}
	g.nx, g.nz = int(math.Ceil((hi[0]-lo[0])/cell)), int(math.Ceil((hi[1]-lo[1])/cell))
	for i, b := range m.Boxes { // a box goes in every bucket a hull overlapping it may stand in
		a, z := g.bucketOf(b.Min[0]-sim.R, b.Min[2]-sim.R), g.bucketOf(b.Max[0]+sim.R, b.Max[2]+sim.R)
		for bi := a[0]; bi <= z[0]; bi++ {
			for bj := a[1]; bj <= z[1]; bj++ {
				g.buckets[[2]int{bi, bj}] = append(g.buckets[[2]int{bi, bj}], i)
			}
		}
	}
	g.start = make([]int, g.nx*g.nz+1)
	for ci := range g.nx * g.nz {
		g.start[ci] = len(g.y)
		x, z := g.center(ci)
		var tops []float64
		for _, bi := range g.near(x, z) {
			b := &g.boxes[bi]
			if x+sim.R > b.Min[0]+1e-3 && x-sim.R < b.Max[0]-1e-3 && z+sim.R > b.Min[2]+1e-3 && z-sim.R < b.Max[2]-1e-3 {
				if sim.IsWedge(b) && !sim.Walkable(b) {
					continue // too steep; or an escalator's static treads (they move: Map.Movers)
				}
				tops = append(tops, sim.HullTop(b, x, z))
			}
		}
		slices.Sort(tops)
		tops = slices.Compact(tops)
		for _, y := range tops {
			if g.free(x, y, z) && !underWater(m, x, y, z) {
				g.y = append(g.y, y)
				g.col = append(g.col, ci)
			}
		}
	}
	g.start[g.nx*g.nz] = len(g.y)
	g.reach = make([]bool, len(g.y))
	g.clear = make([]int, len(g.y))
	return g
}

// free: a standing hull with its feet at (x, y, z) touches no box.
func (g *grid) free(x, y, z float64) bool {
	const e = 1e-3
	for _, bi := range g.near(x, z) {
		b := &g.boxes[bi]
		if x+sim.R > b.Min[0]+e && x-sim.R < b.Max[0]-e && z+sim.R > b.Min[2]+e && z-sim.R < b.Max[2]-e &&
			y+sim.H > b.Min[1]+e && y < b.Max[1]-e && (!sim.IsWedge(b) || !sim.AboveSlope(b, x, y+e, z, sim.R)) {
			return false
		}
	}
	return true
}

// underWater: feet at (x, y, z) stand under a water volume's surface.
func underWater(m *c.Map, x, y, z float64) bool {
	for _, w := range m.Water {
		if x > w.Min[0] && x < w.Max[0] && z > w.Min[2] && z < w.Max[2] && y < w.Max[1] && y+sim.H > w.Min[1] {
			return true
		}
	}
	return false
}

func (g *grid) center(ci int) (float64, float64) {
	return g.x0 + (float64(ci%g.nx)+0.5)*cell, g.z0 + (float64(ci/g.nx)+0.5)*cell
}

func (g *grid) colAt(x, z float64) (int, bool) {
	i, j := int(math.Floor((x-g.x0)/cell)), int(math.Floor((z-g.z0)/cell))
	if i < 0 || j < 0 || i >= g.nx || j >= g.nz {
		return 0, false
	}
	return j*g.nx + i, true
}

// at: the node of column ci within a step of height y (the nearest), -1 if none.
func (g *grid) at(ci int, y float64) int {
	best, bd := -1, stepUp+1e-6
	for n := g.start[ci]; n < g.start[ci+1]; n++ {
		if d := math.Abs(g.y[n] - y); d <= bd {
			best, bd = n, d
		}
	}
	return best
}

// nearest: the node standing at p, or the nearest within 2 m about its height.
func (g *grid) nearest(p [3]float64) (int, bool) {
	best, bd := -1, math.Inf(1)
	ci, cj := int(math.Floor((p[0]-g.x0)/cell)), int(math.Floor((p[2]-g.z0)/cell))
	r := int(2 / cell)
	for j := max(0, cj-r); j <= min(g.nz-1, cj+r); j++ {
		for i := max(0, ci-r); i <= min(g.nx-1, ci+r); i++ {
			c := j*g.nx + i
			x, z := g.center(c)
			for n := g.start[c]; n < g.start[c+1]; n++ {
				if dy := g.y[n] - p[1]; dy > stepUp+0.1 || dy < -1.6 {
					continue
				}
				if d := math.Hypot(x-p[0], z-p[2]) + 0.5*math.Abs(g.y[n]-p[1]); d < bd {
					best, bd = n, d
				}
			}
		}
	}
	return best, best >= 0
}

var dirs = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

// neighbours calls f with each node walkable from n both ways (a step at
// most), diagonals only with both sides open (no corner cutting).
func (g *grid) neighbours(n int, f func(m int, dist float64)) {
	ci := g.col[n]
	i0, j0, y := ci%g.nx, ci/g.nx, g.y[n]
	for _, d := range dirs {
		i, j := i0+d[0], j0+d[1]
		if i < 0 || j < 0 || i >= g.nx || j >= g.nz {
			continue
		}
		m := g.at(j*g.nx+i, y)
		if m < 0 {
			continue
		}
		if d[0] != 0 && d[1] != 0 && (g.at(j0*g.nx+i, y) < 0 || g.at(j*g.nx+i0, y) < 0) {
			continue
		}
		f(m, math.Hypot(float64(d[0]), float64(d[1]))*cell)
	}
}

func (g *grid) flood(seeds []int) {
	q := slices.Clone(seeds)
	for _, s := range seeds {
		g.reach[s] = true
	}
	for len(q) > 0 {
		n := q[0]
		q = q[1:]
		g.neighbours(n, func(m int, _ float64) {
			if !g.reach[m] {
				g.reach[m] = true
				q = append(q, m)
			}
		})
	}
}

// clearance: a breadth-first pass from the reachable floor's edge nodes
// (those with fewer than 8 reachable neighbours).
func (g *grid) clearance() {
	var q []int
	for n := range g.y {
		g.clear[n] = -1
		if !g.reach[n] {
			continue
		}
		k := 0
		g.neighbours(n, func(m int, _ float64) {
			if g.reach[m] {
				k++
			}
		})
		if k < 8 {
			g.clear[n] = 0
			q = append(q, n)
		}
	}
	for len(q) > 0 {
		n := q[0]
		q = q[1:]
		g.neighbours(n, func(m int, _ float64) {
			if g.reach[m] && g.clear[m] < 0 {
				g.clear[m] = g.clear[n] + 1
				q = append(q, m)
			}
		})
	}
}

func (g *grid) pos(n int) [3]float64 {
	x, z := g.center(g.col[n])
	return [3]float64{x, g.y[n], z}
}

// straight: the line a -> b walks on the reachable floor, a step at a time
// (stairs and ramps, no jumps or drops).
func (g *grid) straight(a, b int) bool { return g.straightC(a, b, true) }

// straightC: straight, with room to the sides (wide) or along a wall (an
// approach into a sliver of objective zone: approach).
func (g *grid) straightC(a, b int, wide bool) bool {
	pa, pb := g.pos(a), g.pos(b)
	l := math.Hypot(pb[0]-pa[0], pb[2]-pa[2])
	k := int(l/(cell/2)) + 1
	y := pa[1]
	for i := 0; i <= k; i++ {
		t := float64(i) / float64(k)
		ci, ok := g.colAt(pa[0]+(pb[0]-pa[0])*t, pa[2]+(pb[2]-pa[2])*t)
		if !ok {
			return false
		}
		n := g.at(ci, y)
		if n < 0 || !g.reach[n] {
			return false
		}
		// Room to the sides away from the ends: a walker arrives up to half a
		// metre off a node (a route stops that short) and walks on from there.
		if d := math.Min(t, 1-t) * l; wide && d > endRoom && g.clear[n] < minClear {
			return false
		}
		y = g.y[n]
	}
	return math.Abs(y-pb[1]) <= stepUp
}

// endRoom: metres at each end of a link where it may run next to an edge.
const endRoom = 0.75

// arrive: a walker (a bot, mapcheck's walk) counts a waypoint reached this
// close to it (horizontally), and turns for the next one from there.
const arrive = 0.5

// stopShort: the floor node where a walker on the straight a -> b first
// comes within arrive of b (on stairs: another step than b's), -1 if the
// line doesn't walk.
func (g *grid) stopShort(a, b int) int {
	pa, pb := g.pos(a), g.pos(b)
	l := math.Hypot(pb[0]-pa[0], pb[2]-pa[2])
	if l < arrive {
		return a
	}
	k := int(l/(cell/4)) + 1
	y := pa[1]
	for i := 0; i <= k; i++ {
		t := float64(i) / float64(k)
		ci, ok := g.colAt(pa[0]+(pb[0]-pa[0])*t, pa[2]+(pb[2]-pa[2])*t)
		if !ok {
			return -1
		}
		n := g.at(ci, y)
		if n < 0 {
			return -1
		}
		if (1-t)*l < arrive {
			return n
		}
		y = g.y[n]
	}
	return b
}

// enters: a walker coming a -> b into an objective zone b stands in (from
// outside it) stops short inside it: a route ends at the first node inside
// a zone, and the bot's (or the walk's) end must be in it.
func (g *grid) enters(a, b int, zones []c.Zone) bool {
	pa, pb := g.pos(a), g.pos(b)
	for _, z := range zones {
		if z.Contains(pb) && !z.Contains(pa) {
			s := g.stopShort(a, b)
			if s < 0 {
				return false
			}
			ps := g.pos(s)
			l := math.Hypot(pb[0]-pa[0], pb[2]-pa[2])
			// a little further out than the stop: it is a few centimetres early
			t := (arrive + 0.1) / l
			q := [3]float64{pb[0] + (pa[0]-pb[0])*t, ps[1], pb[2] + (pa[2]-pb[2])*t}
			if !z.Contains(ps) || !z.Contains(q) {
				return false
			}
		}
	}
	return true
}

// approach: for an objective node n in zone z that a walker heading at it
// from outside may stop short of outside the zone (a sliver of floor along a
// prop): a way in on a line, a1 -> a2 -> n, both outside every zone. A walker
// comes to a1 from anywhere and stops short of it (the floor arrive metres
// around it); from there, and from where it stops short of a2, its walks on
// run far enough into z (enters). a2 is the approach's alone (no other
// links: a walker reaches it from a1, on the line). -1s if none.
func (g *grid) approach(n int, zones []c.Zone) (int, int) {
	p := g.pos(n)
	b1, b2, bs := -1, -1, math.Inf(-1)
	free := func(m int) bool { return m >= 0 && g.reach[m] && !inAny(zones, g.pos(m)) }
	for k := range 16 {
		a := float64(k) * math.Pi / 8
		dx, dz := math.Cos(a), math.Sin(a)
		for l2 := 1.0; l2 <= 3; l2 += 0.5 {
			c2, ok := g.colAt(p[0]-dx*l2, p[2]-dz*l2)
			if !ok {
				continue
			}
			a2 := g.at(c2, p[1])
			if !free(a2) || !g.straightC(a2, n, false) || !g.straightC(n, a2, false) || !g.enters(a2, n, zones) {
				continue
			}
			for l1 := l2 + 2; l1 <= l2+4; l1++ {
				c1, ok := g.colAt(p[0]-dx*l1, p[2]-dz*l1)
				if !ok {
					continue
				}
				a1 := g.at(c1, p[1])
				if !free(a1) || !g.straightC(a1, a2, false) || !g.straightC(a2, a1, false) || !g.viaEnters(a1, a1, a2, n, zones) {
					continue
				}
				good := true
				pa := g.pos(a1)
				for r := 0; r < 8 && good; r++ { // wherever a walker stops short of a1
					ra := float64(r) * math.Pi / 4
					cr, in := g.colAt(pa[0]+math.Cos(ra)*arrive, pa[2]+math.Sin(ra)*arrive)
					if !in {
						continue
					}
					if s := g.at(cr, pa[1]); s >= 0 && g.reach[s] {
						good = g.viaEnters(s, a1, a2, n, zones)
					}
				}
				if sc := float64(min(g.clear[a1], 4)) - 0.2*l1; good && sc > bs {
					b1, b2, bs = a1, a2, sc
				}
			}
		}
	}
	return b1, b2
}

// viaEnters: a walker at s, on its way a1 -> a2 -> n (s near a1), walks to
// a2, stops short of it and walks on into n's zone.
func (g *grid) viaEnters(s, a1, a2, n int, zones []c.Zone) bool {
	if !g.straightC(s, a2, false) {
		return false
	}
	t := g.stopShort(s, a2)
	return t >= 0 && math.Abs(g.y[t]-g.y[a2]) < stepUp && g.straightC(t, n, false) && g.enters(t, n, zones)
}

func inAny(zones []c.Zone, p [3]float64) bool {
	for _, z := range zones {
		if z.Contains(p) {
			return true
		}
	}
	return false
}

// turns: a walker coming a -> b stops short of b at b's height (within a
// step) and walks on straight to c from there.
func (g *grid) turns(a, b, c int) bool {
	s := g.stopShort(a, b)
	return s >= 0 && math.Abs(g.y[s]-g.y[b]) <= stepUp-0.1 && g.straight(s, c) && g.straight(c, s)
}

type item struct {
	n int
	f float64
}
type pq []item

func (h pq) Len() int           { return len(h) }
func (h pq) Less(i, j int) bool { return h[i].f < h[j].f || h[i].f == h[j].f && h[i].n < h[j].n }
func (h pq) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *pq) Push(x any)        { *h = append(*h, x.(item)) }
func (h *pq) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// route: A* over the reachable floor from a to b, at most limit metres long,
// preferring open floor (a step next to an edge costs more). Nil if none.
// Its scratch (per grid node, stamped per search) is the grid's.
func (g *grid) route(a, b int, limit float64) []int {
	g.routes++
	if g.rCost == nil {
		g.rCost, g.rPrev, g.rSeen = make([]float64, len(g.y)), make([]int32, len(g.y)), make([]int32, len(g.y))
	}
	g.rGen++
	gen := g.rGen
	// rSeen: gen (open, cost set) or -gen (done)
	g.rSeen[a], g.rCost[a] = gen, 0
	pb := g.pos(b)
	hf := func(n int) float64 { p := g.pos(n); return math.Hypot(p[0]-pb[0], p[2]-pb[2]) }
	h := &pq{{a, hf(a)}}
	for h.Len() > 0 {
		cur := heap.Pop(h).(item).n
		if g.rSeen[cur] == -gen {
			continue
		}
		g.rSeen[cur] = -gen
		if cur == b {
			out := []int{b}
			for n := b; n != a; {
				n = int(g.rPrev[n])
				out = append(out, n)
			}
			slices.Reverse(out)
			return out
		}
		cc := g.rCost[cur]
		g.neighbours(cur, func(m int, d float64) {
			if !g.reach[m] || g.rSeen[m] == -gen {
				return
			}
			if g.clear[m] < minClear {
				d *= 3
			}
			nc := cc + d
			if nc > limit*3 {
				return
			}
			if g.rSeen[m] != gen || nc < g.rCost[m] {
				g.rSeen[m], g.rCost[m], g.rPrev[m] = gen, nc, int32(cur)
				heap.Push(h, item{m, nc + hf(m)})
			}
		})
	}
	return nil
}

type node struct {
	name string
	n    int        // grid node
	pos  [3]float64 // where it stands (a spawn's own spot for a team node)
	spec bool
	only bool // linked by its approach alone (approach's a2)
}

func (g *grid) graph(m *c.Map) []c.NavNode {
	var nodes []node
	zones := m.Objectives()
	var forced [][2]int // approach links (along a wall: not straight's wide walk)
	// Named nodes: the teams' first spawns, the objectives.
	for _, team := range c.Teams {
		if sp := m.TeamSpawns[team]; len(sp) > 0 {
			if n, ok := g.nearest(sp[0]); ok && g.reach[n] {
				nodes = append(nodes, node{team, n, g.pos(n), true, false})
			}
		}
	}
	for _, z := range m.Objectives() {
		best, bs := -1, math.Inf(-1)
		cx, cz := (z.Min[0]+z.Max[0])/2, (z.Min[2]+z.Max[2])/2
		for n := range g.y {
			p := g.pos(n)
			if !g.reach[n] || !z.Contains(p) {
				continue
			}
			in := math.Min(math.Min(p[0]-z.Min[0], z.Max[0]-p[0]), math.Min(p[2]-z.Min[2], z.Max[2]-p[2]))
			f := 0.0
			if g.flat(n) {
				f = 3
			}
			s := 10*math.Min(in, 1.5) + f + 0.5*float64(min(g.clear[n], 8)) - 0.05*math.Hypot(p[0]-cx, p[2]-cz) // well inside: a route stops 0.5 m short of it
			if s > bs {
				best, bs = n, s
			}
		}
		if best >= 0 {
			nodes = append(nodes, node{z.Name, best, g.pos(best), true, false})
			// Not deep inside (a sliver of floor along a prop): a way in that ends inside.
			if inside(z, g.pos(best)) < 1 {
				if a1, a2 := g.approach(best, zones); a1 >= 0 {
					zi := len(nodes) - 1
					nodes = append(nodes, node{"", a2, g.pos(a2), true, true}, node{"", a1, g.pos(a1), true, false})
					forced = append(forced, [2]int{zi, zi + 1}, [2]int{zi + 1, zi + 2})
				}
			}
		}
	}
	// Escalators: a chain up each lane (too narrow for a link with room to the
	// sides), outer -> inner at each end: the outer on open floor on the lane's
	// axis, so a walker lines up with the lane before it gets on.
	for _, mv := range m.Movers {
		if mv.Kind != c.MoverEscalator {
			continue
		}
		ax, o := mv.Axis, 2-mv.Axis
		up := float64(mv.Up)
		low, high := mv.Min[ax], mv.Max[ax]
		if up < 0 {
			low, high = high, low
		}
		at := func(along, y float64) int {
			var p [3]float64
			p[ax], p[o] = along, (mv.Min[o]+mv.Max[o])/2
			ci, ok := g.colAt(p[0], p[2])
			if !ok {
				return -1
			}
			if n := g.at(ci, y); n >= 0 && g.reach[n] {
				return n
			}
			return -1
		}
		lane := func(a, b int) bool { return g.straightC(a, b, false) && g.straightC(b, a, false) }
		outer := func(end, y, out float64, inner int) int {
			for d := 2.5; d <= 5; d += 0.5 {
				if n := at(end+out*d, y); n >= 0 && g.clear[n] >= minClear && lane(n, inner) {
					return n
				}
			}
			return -1
		}
		ib, it := at(low-up*0.6, mv.Min[1]), at(high+up*0.6, mv.Max[1])
		if ib < 0 || it < 0 || !lane(ib, it) {
			continue
		}
		ob, ot := outer(low, mv.Min[1], -up, ib), outer(high, mv.Max[1], up, it)
		if ob < 0 || ot < 0 {
			continue
		}
		i := len(nodes)
		nodes = append(nodes, node{"", ob, g.pos(ob), true, false}, node{"", ib, g.pos(ib), true, true},
			node{"", it, g.pos(it), true, true}, node{"", ot, g.pos(ot), true, false})
		forced = append(forced, [2]int{i, i + 1}, [2]int{i + 1, i + 2}, [2]int{i + 2, i + 3})
	}
	// The lattice: per block and level, the most open reachable spot.
	type key struct{ i, j, l int }
	pick := map[key]int{}
	var keys []key
	for n := range g.y {
		if !g.reach[n] {
			continue
		}
		p := g.pos(n)
		k := key{int(math.Floor(p[0] / Block)), int(math.Floor(p[2] / Block)), int(math.Floor(p[1] / levelH))}
		o, ok := pick[k]
		if !ok {
			pick[k] = n
			keys = append(keys, k)
			continue
		}
		score := func(n int) float64 {
			q := g.pos(n)
			cx, cz := (float64(k.i)+0.5)*Block, (float64(k.j)+0.5)*Block
			f := 0.0
			if g.flat(n) {
				f = 20 // stairs and slopes only when nothing is level
			}
			return f + float64(min(g.clear[n], 12)) - 0.1*math.Hypot(q[0]-cx, q[2]-cz)
		}
		if score(n) > score(o) {
			pick[k] = n
		}
	}
	for _, k := range keys {
		n := pick[k]
		p := g.pos(n)
		if g.clear[n] < minClear {
			continue // a sliver of floor along a wall: the neighbours cover it
		}
		close := false
		for _, z := range m.Objectives() { // inside a zone: its own node is the way in (a route ends at the first node inside)
			if p[0] > z.Min[0]-1 && p[0] < z.Max[0]+1 && p[2] > z.Min[2]-1 && p[2] < z.Max[2]+1 && p[1] >= z.Min[1]-1 && p[1] <= z.Max[1]+1 {
				close = true
			}
		}
		for _, s := range nodes {
			if s.spec && math.Hypot(s.pos[0]-p[0], s.pos[2]-p[2]) < nearSpec && math.Abs(s.pos[1]-p[1]) < levelH {
				close = true
				break
			}
		}
		if !close {
			nodes = append(nodes, node{"", n, p, false, false})
		}
	}
	// Links: straight walks within range.
	links := map[[2]int]bool{}
	link := func(a, b int) {
		if a > b {
			a, b = b, a
		}
		links[[2]int{a, b}] = true
	}
	for _, f := range forced {
		link(f[0], f[1])
	}
	inRange := func(a, b node) bool {
		return math.Hypot(a.pos[0]-b.pos[0], a.pos[2]-b.pos[2]) <= LinkRange && math.Abs(a.pos[1]-b.pos[1]) <= LinkRange/2
	}
	for i := range nodes {
		for j := i + 1; j < len(nodes); j++ {
			if !nodes[i].only && !nodes[j].only && inRange(nodes[i], nodes[j]) && g.straight(nodes[i].n, nodes[j].n) && g.straight(nodes[j].n, nodes[i].n) &&
				g.enters(nodes[i].n, nodes[j].n, zones) && g.enters(nodes[j].n, nodes[i].n, zones) {
				link(i, j)
			}
		}
	}
	// Join the pieces: the closest pair of nodes in different pieces with a
	// grid route between them gets that route as a chain of new nodes, until
	// one piece is left (or nothing joins).
	failed := map[[2]int]bool{} // pairs with no walkable route: the same next round
	for range len(nodes) {
		comp := components(len(nodes), links)
		type pair struct {
			a, b int
			d    float64
		}
		var cands []pair
		for i := range nodes {
			for j := i + 1; j < len(nodes); j++ {
				if comp[i] != comp[j] && !nodes[i].only && !nodes[j].only && !failed[[2]int{i, j}] {
					a, b := nodes[i].pos, nodes[j].pos
					cands = append(cands, pair{i, j, math.Hypot(a[0]-b[0], a[2]-b[2]) + math.Abs(a[1]-b[1])})
				}
			}
		}
		if len(cands) == 0 {
			break
		}
		slices.SortStableFunc(cands, func(x, y pair) int {
			if x.d != y.d {
				if x.d < y.d {
					return -1
				}
				return 1
			}
			return 0
		})
		joined := false
		for _, p := range cands[:min(len(cands), 64)] {
			r := g.route(nodes[p.a].n, nodes[p.b].n, 4*p.d+8)
			ws, walks := []int(nil), false
			if r != nil {
				ws, walks = g.pull(r, zones)
			}
			if !walks {
				failed[[2]int{p.a, p.b}] = true
				continue
			}
			prev := p.a
			for _, w := range ws {
				nodes = append(nodes, node{"", w, g.pos(w), false, false})
				link(prev, len(nodes)-1)
				prev = len(nodes) - 1
			}
			link(prev, p.b)
			joined = true
			break
		}
		if !joined {
			break
		}
	}
	lanes := pickLanes(nodes, links, zones)
	out := make([]c.NavNode, len(nodes))
	auto := 0
	for i, nd := range nodes {
		if nd.name == "" {
			auto++
			nd.name = fmt.Sprintf("auto%d", auto)
			nodes[i].name = nd.name
		}
		out[i] = c.NavNode{Name: nd.name, Pos: nd.pos, Lane: lanes[i]}
	}
	keysL := make([][2]int, 0, len(links))
	for k := range links {
		keysL = append(keysL, k)
	}
	slices.SortFunc(keysL, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	for _, k := range keysL {
		out[k[0]].Links = append(out[k[0]].Links, nodes[k[1]].name)
	}
	return out
}

// pull: the corners of grid route r (its ends left out) that a straight walk
// needs: each kept waypoint is as far along as a straight line reaches, and a
// walker that stops short of it (arrive) turns for the next from there (on
// stairs that is another step: turns). A corner prefers flat floor, and one
// on the edge of an objective zone moves into it (a route ends at the first
// node inside the zone) or out of it.
func (g *grid) pull(r []int, zones []c.Zone) ([]int, bool) {
	var out []int
	two := func(a, b int) bool { return g.straight(a, b) && g.straight(b, a) }
	li, pi := 0, -1
	ok := func(k int) bool {
		return two(r[li], r[k]) && g.enters(r[li], r[k], zones) && g.enters(r[k], r[li], zones) && (pi < 0 || g.turns(r[pi], r[li], r[k]))
	}
	cont := func(j int) bool { return j+1 >= len(r) || g.turns(r[li], r[j], r[j+1]) }
	for li < len(r)-1 {
		if !ok(li + 1) {
			return nil, false // not even the next step: no walkable chain here
		}
		k := li + 1
		for k+1 < len(r) && ok(k+1) {
			k++
		}
		if k == len(r)-1 {
			break // the last waypoint walks to the route's end
		}
		pick := -1
		for _, flatOnly := range []bool{true, false} {
			for j := k; j > li && pick < 0; j-- {
				if (!flatOnly || g.flat(r[j])) && ok(j) && cont(j) {
					pick = j
				}
			}
		}
		if pick < 0 {
			return nil, false // no corner a walker turns at
		}
		if z, inZone := edgeOf(zones, g.pos(r[pick])); inZone {
			moved := false
			for j := pick + 1; j <= k; j++ { // on, into the zone
				if inside(z, g.pos(r[j])) >= 1 && ok(j) && cont(j) {
					pick, moved = j, true
					break
				}
			}
			for j := pick - 1; !moved && j > li; j-- { // or back, out of it
				if inside(z, g.pos(r[j])) < 0 && ok(j) && cont(j) {
					pick, moved = j, true
				}
			}
		}
		out = append(out, r[pick])
		pi, li = li, pick
	}
	return out, true
}

// flat: the floor at n is level for 1.25 m around (a stop half a metre short, plus the hull) (no stair or slope a
// walker stopping short of n would stand on instead).
func (g *grid) flat(n int) bool {
	ci, y := g.col[n], g.y[n]
	i0, j0 := ci%g.nx, ci/g.nx
	for dj := -5; dj <= 5; dj++ {
		for di := -5; di <= 5; di++ {
			i, j := i0+di, j0+dj
			if i < 0 || j < 0 || i >= g.nx || j >= g.nz {
				continue
			}
			if m := g.at(j*g.nx+i, y); m >= 0 && math.Abs(g.y[m]-y) > 0.02 {
				return false
			}
		}
	}
	return true
}

// inside: how far p is inside zone z across (negative: outside), or -1 off its heights.
func inside(z c.Zone, p [3]float64) float64 {
	if p[1] < z.Min[1] || p[1] > z.Max[1] {
		return -1
	}
	return math.Min(math.Min(p[0]-z.Min[0], z.Max[0]-p[0]), math.Min(p[2]-z.Min[2], z.Max[2]-p[2]))
}

// edgeOf: the zone p is on the edge of (inside it, less than a metre in).
func edgeOf(zones []c.Zone, p [3]float64) (c.Zone, bool) {
	for _, z := range zones {
		if d := inside(z, p); d >= 0 && d < 1 {
			return z, true
		}
	}
	return c.Zone{}, false
}

// components labels each of n nodes with its connected piece under links.
func components(n int, links map[[2]int]bool) []int {
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for k := range links {
		a, b := find(k[0]), find(k[1])
		if a != b {
			parent[max(a, b)] = min(a, b)
		}
	}
	out := make([]int, n)
	for i := range out {
		out[i] = find(i)
	}
	return out
}

// Lanes (content.NavNode.Lane: an attacker bot, a Vibe Coder, walks spawn ->
// a lane picked each round -> its objective, internal/game ai_lanes.go): per
// objective, from the Vibe Coders' node, the
// node halfway along the shortest route (going straight stays a choice) and
// up to laneAlts others, each the one furthest from the routes picked so far
// (at least laneApart metres) whose way through is at most laneDetour times
// the shortest.
const (
	laneDetour = 1.5 // under the bots' own 1.6 (aiLaneDetour)
	laneApart  = 8.0
	laneAlts   = 2
)

func pickLanes(nodes []node, links map[[2]int]bool, sites []c.Zone) []bool {
	lane := make([]bool, len(nodes))
	adj := make([][]int, len(nodes))
	keys := make([][2]int, 0, len(links))
	for k := range links {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	for _, k := range keys {
		adj[k[0]] = append(adj[k[0]], k[1])
		adj[k[1]] = append(adj[k[1]], k[0])
	}
	dist := func(a, b int) float64 {
		p, q := nodes[a].pos, nodes[b].pos
		return math.Hypot(p[0]-q[0], p[2]-q[2])
	}
	// shortest: Dijkstra from a: distances, predecessors.
	shortest := func(a int) ([]float64, []int) {
		d, prev, done := make([]float64, len(nodes)), make([]int, len(nodes)), make([]bool, len(nodes))
		for i := range d {
			d[i], prev[i] = math.Inf(1), -1
		}
		d[a] = 0
		for {
			cur := -1
			for i := range d {
				if !done[i] && !math.IsInf(d[i], 1) && (cur < 0 || d[i] < d[cur]) {
					cur = i
				}
			}
			if cur < 0 {
				return d, prev
			}
			done[cur] = true
			for _, n := range adj[cur] {
				if nd := d[cur] + dist(cur, n); nd < d[n] {
					d[n], prev[n] = nd, cur
				}
			}
		}
	}
	mark := func(v int) { // one lane per spot: one within laneApart/2 of another is that one
		for u := range nodes {
			if lane[u] && dist(u, v) < laneApart/2 && math.Abs(nodes[u].pos[1]-nodes[v].pos[1]) < levelH {
				return
			}
		}
		lane[v] = true
	}
	index := map[string]int{}
	for i, n := range nodes {
		if n.name != "" {
			index[n.name] = i
		}
	}
	for _, team := range []string{c.Vibe} {
		t, ok := index[team]
		if !ok {
			continue
		}
		fromT, _ := shortest(t)
		for _, site := range sites {
			si, ok := index[site.Name]
			if !ok || math.IsInf(fromT[si], 1) {
				continue
			}
			fromS, prevS := shortest(si)
			var route []int // the shortest route, site -> team
			for n := t; n >= 0; n = prevS[n] {
				route = append(route, n)
			}
			// halfway along it
			half := fromT[si] / 2
			for _, n := range route {
				if fromS[n] <= half {
					if nodes[n].name == "" && !nodes[n].only {
						mark(n)
					}
					break
				}
			}
			// The others: far from every route picked so far.
			picked := [][]int{route}
			away := func(v int) float64 {
				best := math.Inf(1)
				for _, r := range picked {
					for i := 0; i+1 < len(r); i++ {
						best = math.Min(best, segDist(nodes[v].pos, nodes[r[i]].pos, nodes[r[i+1]].pos))
					}
				}
				return best
			}
			for range laneAlts {
				best, bd := -1, laneApart
				for v := range nodes {
					if nodes[v].name != "" || nodes[v].only || math.IsInf(fromT[v], 1) || fromT[v]+fromS[v] > laneDetour*fromT[si] {
						continue
					}
					if d := away(v); d > bd {
						best, bd = v, d
					}
				}
				if best < 0 {
					break
				}
				mark(best)
				var r []int // its way: team -> best -> site
				for n := best; n >= 0; n = prevS[n] {
					r = append(r, n)
				}
				_, prevT := shortest(t)
				for n := prevT[best]; n >= 0; n = prevT[n] {
					r = append(r, n)
				}
				picked = append(picked, r)
			}
		}
	}
	return lane
}

// segDist: horizontal distance from p to the segment a-b.
func segDist(p, a, b [3]float64) float64 {
	dx, dz := b[0]-a[0], b[2]-a[2]
	l2 := dx*dx + dz*dz
	t := 0.0
	if l2 > 0 {
		t = math.Max(0, math.Min(1, ((p[0]-a[0])*dx+(p[2]-a[2])*dz)/l2))
	}
	return math.Hypot(p[0]-a[0]-t*dx, p[2]-a[2]-t*dz)
}
