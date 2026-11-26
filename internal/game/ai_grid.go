package game

import (
	"container/heap"
	"math"
	"slices"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// The last leg off the nav graph. The nav graph runs between chokes, spawns
// and sites; a goal off it (the Big Diff planted behind a crate, on a crate
// top, a site's middle inside a crate stack) is walked to from the graph's
// nearest node. When that last leg is blocked, gridPath plans it on a local
// walk grid built from the map's boxes around both ends: columns of gridCell
// metres, each with the box tops a player's hull fits on, linked to their
// neighbours by a step (up to the movement code's step height), a jump (up
// to aiJumpUp) or a drop. The route comes back as waypoints; the bot jumps
// for a waypoint higher than it can step (aiMove).

// gridMargins: metres of grid around the leg's two ends, tried in turn.
var gridMargins = [2]float64{6, 16}

const (
	gridCell  = 0.25 // metres per column
	gridMax   = 48.0 // a leg longer than this (either axis) is not planned
	aiStepUp  = 0.5  // a rise the movement code steps up (sim stepUp 0.55)
	aiJumpUp  = 1.0  // a rise a standing jump gets onto (apex ~1.18 m)
	gridSnap  = 2.0  // a goal inside a box: the nearest standing spot this close
	aiDropMax = 2.5  // floorAlong: a drop deeper than this is no floor (a pit)
)

type gridNode struct{ c, k int } // column index, surface index in it

type walkGrid struct {
	x0, z0 float64
	nx, nz int
	floors [][]float64   // per column: the heights a standing hull fits on, ascending
	boxes  []content.Box // the map's boxes near the grid
}

func (g *walkGrid) col(x, z float64) (int, bool) {
	i, j := int(math.Floor((x-g.x0)/gridCell)), int(math.Floor((z-g.z0)/gridCell))
	if i < 0 || j < 0 || i >= g.nx || j >= g.nz {
		return 0, false
	}
	return j*g.nx + i, true
}

func (g *walkGrid) center(c int) (float64, float64) {
	return g.x0 + (float64(c%g.nx)+0.5)*gridCell, g.z0 + (float64(c/g.nx)+0.5)*gridCell
}

// buildWalkGrid: the walk grid over [lo, hi] (x, z) for the map's boxes.
func buildWalkGrid(m *content.Map, lo, hi [2]float64) *walkGrid {
	g := &walkGrid{x0: lo[0], z0: lo[1]}
	g.nx, g.nz = int(math.Ceil((hi[0]-lo[0])/gridCell)), int(math.Ceil((hi[1]-lo[1])/gridCell))
	var boxes []content.Box
	for _, b := range m.Boxes {
		if b.Max[0] > lo[0]-sim.R && b.Min[0] < hi[0]+sim.R && b.Max[2] > lo[1]-sim.R && b.Min[2] < hi[1]+sim.R {
			boxes = append(boxes, b)
		}
	}
	g.boxes = boxes
	g.floors = make([][]float64, g.nx*g.nz)
	for c := range g.floors {
		x, z := g.center(c)
		var tops []float64
		for _, b := range boxes {
			// stands on any box under its hull (as the movement code does)
			if x+sim.R > b.Min[0]+1e-3 && x-sim.R < b.Max[0]-1e-3 && z+sim.R > b.Min[2]+1e-3 && z-sim.R < b.Max[2]-1e-3 {
				if sim.IsWedge(&b) && !sim.Walkable(&b) {
					continue // too steep to stand on
				}
				tops = append(tops, sim.HullTop(&b, x, z)) // a ramp: where the hull stands on its slope
			}
		}
		for _, y := range tops {
			if !slices.Contains(g.floors[c], y) && hullFree(boxes, x, y, z) {
				g.floors[c] = append(g.floors[c], y)
			}
		}
		slices.Sort(g.floors[c])
	}
	return g
}

// hullFree: a standing player's hull with its feet at (x, y, z) touches no box.
func hullFree(boxes []content.Box, x, y, z float64) bool { return hullFreeSpan(boxes, x, y, y, z) }

// hullFreeSpan: the hull fits with its feet anywhere from y0 up to y1 (a
// drop or a jump passing through the column).
func hullFreeSpan(boxes []content.Box, x, y0, y1, z float64) bool {
	const e = 1e-3
	for _, b := range boxes {
		if x+sim.R > b.Min[0]+e && x-sim.R < b.Max[0]-e && z+sim.R > b.Min[2]+e && z-sim.R < b.Max[2]-e &&
			y1+sim.H > b.Min[1]+e && y0 < b.Max[1]-e && (!sim.IsWedge(&b) || !sim.AboveSlope(&b, x, y0+e, z, sim.R)) {
			return false
		}
	}
	return true
}

// land: the surface of column c a body at height y walking (rise up to
// aiStepUp) or jumping (up to aiJumpUp) into it ends up on: the highest one
// it can get onto; -1 if none.
func (g *walkGrid) land(c int, y, rise float64) int {
	for k := len(g.floors[c]) - 1; k >= 0; k-- {
		if g.floors[c][k] <= y+rise+1e-6 {
			return k
		}
	}
	return -1
}

// nearest: the grid node standing at p (or, p inside a box, the nearest one
// within gridSnap on about its floor).
func (g *walkGrid) nearest(p Vec3) (gridNode, bool) {
	best, bd := gridNode{}, math.Inf(1)
	r := int(gridSnap / gridCell)
	ci, cj := int(math.Floor((p[0]-g.x0)/gridCell)), int(math.Floor((p[2]-g.z0)/gridCell))
	for j := max(0, cj-r); j <= min(g.nz-1, cj+r); j++ {
		for i := max(0, ci-r); i <= min(g.nx-1, ci+r); i++ {
			c := j*g.nx + i
			x, z := g.center(c)
			for k, y := range g.floors[c] {
				if dy := y - p[1]; dy > aiStepUp+0.1 || dy < -1.6 {
					continue
				}
				d := math.Hypot(x-p[0], z-p[2]) + 0.5*math.Abs(y-p[1])
				if d < bd {
					best, bd = gridNode{c, k}, d
				}
			}
		}
	}
	return best, !math.IsInf(bd, 1)
}

type gridItem struct {
	n gridNode
	f float64
}
type gridHeap []gridItem

func (h gridHeap) Len() int { return len(h) }
func (h gridHeap) Less(i, j int) bool {
	return h[i].f < h[j].f || h[i].f == h[j].f && (h[i].n.c < h[j].n.c || h[i].n.c == h[j].n.c && h[i].n.k < h[j].n.k)
}
func (h gridHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *gridHeap) Push(x any)   { *h = append(*h, x.(gridItem)) }
func (h *gridHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// route: A* over the grid from a to b; the nodes, a first. nil if none.
func (g *walkGrid) route(a, b gridNode) []gridNode {
	type key = gridNode
	cost := map[key]float64{a: 0}
	prev := map[key]key{}
	bx, bz := g.center(b.c)
	hf := func(n gridNode) float64 {
		x, z := g.center(n.c)
		return math.Hypot(x-bx, z-bz)
	}
	h := &gridHeap{{a, hf(a)}}
	done := map[key]bool{}
	dirs := [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	for h.Len() > 0 {
		cur := heap.Pop(h).(gridItem).n
		if done[cur] {
			continue
		}
		done[cur] = true
		if cur == b {
			out := []gridNode{b}
			for n := b; n != a; {
				n = prev[n]
				out = append(out, n)
			}
			slices.Reverse(out)
			return out
		}
		y := g.floors[cur.c][cur.k]
		ci, cj := cur.c%g.nx, cur.c/g.nx
		for _, d := range dirs {
			i, j := ci+d[0], cj+d[1]
			if i < 0 || j < 0 || i >= g.nx || j >= g.nz {
				continue
			}
			c := j*g.nx + i
			if d[0] != 0 && d[1] != 0 { // diagonal: both sides open at this height (no corner cutting)
				if g.land(cj*g.nx+i, y, aiStepUp) < 0 || g.land(j*g.nx+ci, y, aiStepUp) < 0 {
					continue
				}
			}
			step := math.Hypot(float64(d[0]), float64(d[1])) * gridCell
			for _, rise := range [2]float64{aiStepUp, aiJumpUp} {
				k := g.land(c, y, rise)
				if k < 0 {
					continue
				}
				dy := g.floors[c][k] - y
				if rise == aiJumpUp && dy <= aiStepUp+1e-6 {
					continue // the same landing as walking
				}
				if math.Abs(dy) > aiStepUp+1e-6 {
					// a drop: the hull falls through the column from our height;
					// a jump: it rises from here to the landing's height
					x, z := g.center(c)
					if dy < 0 && !hullFreeSpan(g.boxes, x, g.floors[c][k], y, z) {
						continue
					}
					if fx, fz := g.center(cur.c); dy > 0 && !hullFreeSpan(g.boxes, fx, y, g.floors[c][k], fz) {
						continue
					}
				}
				n := gridNode{c, k}
				nc := cost[cur] + step + math.Max(0, -dy)*0.3
				if dy > aiStepUp+1e-6 {
					nc += 2 // a jump: only when it saves a walk round
				}
				if old, ok := cost[n]; (!ok || nc < old) && !done[n] {
					cost[n], prev[n] = nc, cur
					heap.Push(h, gridItem{n, nc + hf(n)})
				}
			}
		}
	}
	return nil
}

// gridPath: waypoints from 'from' to 'to' on a walk grid around both (the
// last one standing at or next to 'to'); ok false if the leg is too long or
// there is no way.
func (s *Server) gridPath(from, to Vec3) ([]Vec3, bool) {
	if math.Abs(to[0]-from[0]) > gridMax || math.Abs(to[2]-from[2]) > gridMax {
		return nil, false
	}
	key := gridKey{s.Map.Name, from, to}
	if r, ok := s.gridCache[key]; ok {
		return r.path, r.ok
	}
	var path []Vec3
	ok := false
	// A small grid first; a wider one when the way round a long wall leaves it.
	for _, margin := range gridMargins {
		lo := [2]float64{math.Min(from[0], to[0]) - margin, math.Min(from[2], to[2]) - margin}
		hi := [2]float64{math.Max(from[0], to[0]) + margin, math.Max(from[2], to[2]) + margin}
		if path, ok = buildWalkGrid(&s.Map, lo, hi).plan(from, to); ok {
			break
		}
	}
	if s.gridCache == nil || len(s.gridCache) > 4096 {
		s.gridCache = map[gridKey]gridResult{}
	}
	s.gridCache[key] = gridResult{path, ok}
	return path, ok
}

type gridKey struct {
	m        string
	from, to Vec3
}

type gridResult struct {
	path []Vec3
	ok   bool
}

func (g *walkGrid) plan(from, to Vec3) ([]Vec3, bool) {
	a, ok1 := g.nearest(from)
	b, ok2 := g.nearest(to)
	if !ok1 || !ok2 {
		return nil, false
	}
	nodes := g.route(a, b)
	if nodes == nil {
		return nil, false
	}
	pt := func(n gridNode) Vec3 {
		x, z := g.center(n.c)
		return Vec3{x, g.floors[n.c][n.k], z}
	}
	// Waypoints: both ends of every jump and drop, and the corners of the
	// walks between them, string-pulled: a waypoint goes when the line from
	// the previous kept one to the next walks on floors (steps included).
	var out []Vec3
	last := pt(nodes[0])
	for i := 1; i < len(nodes); i++ {
		p := pt(nodes[i])
		if i == len(nodes)-1 {
			out = append(out, p)
			break
		}
		nx := pt(nodes[i+1])
		if math.Abs(p[1]-last[1]) > aiStepUp+1e-6 && math.Abs(p[1]-pt(nodes[i-1])[1]) > aiStepUp+1e-6 || math.Abs(nx[1]-p[1]) > aiStepUp+1e-6 {
			out = append(out, p) // a jump or drop starts or ends here: keep the spot
			last = p
			continue
		}
		if !g.straight(last, nx) {
			out = append(out, p)
			last = p
		}
	}
	if len(out) == 0 {
		out = []Vec3{pt(b)}
	}
	if to[1] >= out[len(out)-1][1]-0.1 && math.Abs(to[1]-out[len(out)-1][1]) < 0.2 && g.straight(out[len(out)-1], to) {
		out[len(out)-1] = to // the goal itself is a standing spot: end there
	}
	return out, true
}

// straight: the line a -> b walks on floors: every column on it holds one
// within a step of the last (stairs, not a jump or a drop).
func (g *walkGrid) straight(a, b Vec3) bool {
	l := math.Hypot(b[0]-a[0], b[2]-a[2])
	n := int(l/(gridCell/2)) + 1
	y := a[1]
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		c, ok := g.col(a[0]+(b[0]-a[0])*t, a[2]+(b[2]-a[2])*t)
		if !ok {
			return false
		}
		k := g.land(c, y, aiStepUp)
		if k < 0 || y-g.floors[c][k] > aiStepUp+1e-6 {
			return false
		}
		y = g.floors[c][k]
	}
	return math.Abs(y-b[1]) < aiStepUp
}

// floorAlong: there is floor under the whole straight walk from a to b (each
// half metre: a box top within a step up and aiDropMax down of a's height).
// Callers hold s.mu.
func (s *Server) floorAlong(a, b Vec3) bool {
	l := math.Hypot(b[0]-a[0], b[2]-a[2])
	n := int(l/0.5) + 1
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		x, z := a[0]+(b[0]-a[0])*f, a[2]+(b[2]-a[2])*f
		best := math.Inf(-1)
		for j := range s.Map.Boxes {
			bx := &s.Map.Boxes[j]
			if x < bx.Min[0] || x > bx.Max[0] || z < bx.Min[2] || z > bx.Max[2] {
				continue
			}
			if t := sim.TopAt(bx, x, z); t <= a[1]+aiStepUp && t > best {
				best = t
			}
		}
		if best < a[1]-aiDropMax {
			return false
		}
	}
	return true
}
