package navgen

import (
	"encoding/json"
	"math"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

// An L-shaped yard: the Vibe Coders' spawn in the west arm, the Artisans'
// in the north arm, site A at the corner, a crate in the way, a 2 m ledge
// (no way up it on foot) and a pool (bots don't swim).
func yard() c.Map {
	m := c.Map{Name: "yard"}
	m.Boxes = []c.Box{
		c.B(-30, -1, -10, 10, 0, 10, c.Concrete), // the west arm's floor
		c.B(-10, -1, 10, 10, 0, 40, c.Concrete),  // the north arm's floor
		c.B(-31, 0, -11, 11, 4, -10, c.Brick),    // walls
		c.B(-31, 0, 10, -10, 4, 11, c.Brick),
		c.B(10, 0, -11, 11, 4, 41, c.Brick),
		c.B(-11, 0, 11, -10, 4, 41, c.Brick),
		c.B(-31, 0, -10, -30, 4, 10, c.Brick),
		c.B(-10, 0, 40, 10, 4, 41, c.Brick),
		c.B(-2, 0, -2, 2, 1.5, 2, c.Crate),        // in the corner
		c.B(-20, 0, 5, -14, 2, 10, c.Concrete),    // a 2 m ledge
		c.B(-26, -3, -9, -22, -1, -5, c.Concrete), // the pool's floor
	}
	m.Water = []c.Box{c.B(-26, -3, -9, -22, -0.2, -5, c.Water)}
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{-27, 0, 0}, {-27, 0, 3}}, c.Artisan: {{0, 0, 37}, {3, 0, 37}}}
	m.Sites = []c.Zone{{Name: "A", Min: [3]float64{-8, -0.5, -8}, Max: [3]float64{8, 3, 8}}}
	return m
}

func TestBuild(t *testing.T) {
	m := yard()
	nav := Build(&m)
	byName := map[string]c.NavNode{}
	for _, n := range nav {
		byName[n.Name] = n
	}
	for _, want := range []string{c.Vibe, c.Artisan, "A"} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("no %q node in %d", want, len(nav))
		}
	}
	// One piece: every node reaches every other.
	adj := map[string][]string{}
	for _, n := range nav {
		for _, l := range n.Links {
			if _, ok := byName[l]; !ok {
				t.Fatalf("%s links to unknown %s", n.Name, l)
			}
			adj[n.Name] = append(adj[n.Name], l)
			adj[l] = append(adj[l], n.Name)
		}
	}
	seen := map[string]bool{c.Vibe: true}
	for q := []string{c.Vibe}; len(q) > 0; q = q[1:] {
		for _, l := range adj[q[0]] {
			if !seen[l] {
				seen[l] = true
				q = append(q, l)
			}
		}
	}
	if len(seen) != len(nav) {
		t.Fatalf("%d of %d nodes connected", len(seen), len(nav))
	}
	a := byName["A"].Pos
	if !m.Sites[0].Contains(a) || a[1] != 0 {
		t.Fatalf("A's node %v: not on the site's floor", a)
	}
	for _, n := range nav {
		p := n.Pos
		if p[1] > 0.1 { // nothing on the crate or the ledge (no way up on foot)
			t.Errorf("%s at %v: up a ledge", n.Name, p)
		}
		if p[0] > -26 && p[0] < -22 && p[2] > -9 && p[2] < -5 {
			t.Errorf("%s at %v: in the pool", n.Name, p)
		}
	}
	// Deterministic.
	x, _ := json.Marshal(nav)
	y, _ := json.Marshal(Build(&m))
	if string(x) != string(y) {
		t.Fatal("two builds differ")
	}
}

func TestBuildNothing(t *testing.T) {
	if Build(&c.Map{}) != nil {
		t.Fatal("a graph for no boxes")
	}
	m := yard()
	m.TeamSpawns = nil
	if Build(&m) != nil {
		t.Fatal("a graph without spawns")
	}
	m = yard()
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{100, 0, 100}}}
	if Build(&m) != nil {
		t.Fatal("a graph from a spawn off the map")
	}
}

// Two rooms joined by a narrow corridor with a bend (1.3 m wide: no lattice
// node fits in it and no straight link passes it): the pieces are joined
// along a grid route, string-pulled into nodes at the bend.
func TestBuildJoinsAlongACorridor(t *testing.T) {
	m := c.Map{Name: "rooms"}
	m.Boxes = []c.Box{
		c.B(-40, -1, -20, 40, 0, 20, c.Concrete), // the floor
		c.B(-40, 0, -20, -39, 3, 20, c.Brick),    // west room's west wall
		c.B(-20, 0, -20, -19, 3, -0.65, c.Brick), // west room's east wall, a door at z 0
		c.B(-20, 0, 0.65, -19, 3, 20, c.Brick),
		c.B(-19, 0, -20, 19, 3, -0.65, c.Brick), // the corridor's south side to x 19
		c.B(-19, 0, 0.65, 0.65, 3, 20, c.Brick), // its north side to the bend
		c.B(0.65, 0, 12, 19, 3, 20, c.Brick),    // past the bend: north, then east again
		c.B(1.95, 0, 0.65, 19, 3, 10.7, c.Brick),
		c.B(19, 0, -20, 20, 3, 10.7, c.Brick), // east room's west wall, a door at z 11.35
		c.B(19, 0, 12, 20, 3, 20, c.Brick),
		c.B(40, 0, -20, 41, 3, 20, c.Brick),
		c.B(-40, 0, -21, 41, 3, -20, c.Brick),
		c.B(-40, 0, 20, 41, 3, 21, c.Brick),
	}
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{-30, 0, 0}}, c.Artisan: {{30, 0, 0}}}
	m.Sites = []c.Zone{{Name: "A", Min: [3]float64{24, -0.5, -10}, Max: [3]float64{36, 3, 10}}}
	nav := Build(&m)
	byName := map[string]c.NavNode{}
	adj := map[string][]string{}
	for _, n := range nav {
		byName[n.Name] = n
		for _, l := range n.Links {
			adj[n.Name] = append(adj[n.Name], l)
			adj[l] = append(adj[l], n.Name)
		}
	}
	seen := map[string]bool{c.Vibe: true}
	for q := []string{c.Vibe}; len(q) > 0; q = q[1:] {
		for _, l := range adj[q[0]] {
			if !seen[l] {
				seen[l] = true
				q = append(q, l)
			}
		}
	}
	if !seen[c.Artisan] || !seen["A"] {
		t.Fatalf("the rooms aren't joined: %d nodes, %d reached from vibe", len(nav), len(seen))
	}
	inCorridor := 0
	for _, n := range nav {
		if n.Pos[0] > -19 && n.Pos[0] < 19 {
			inCorridor++
		}
	}
	if inCorridor == 0 {
		t.Fatal("no waypoint in the corridor: the bend can't be walked straight")
	}
}

// de_stack's stair: up +z along a raised floor, stepped off sideways near the
// top. A walker coming up it stops half a metre short of a corner on the
// stair, a step lower, where the raised floor's side is a 1 m wall: no turn
// there (turns), and pull never makes one.
func TestStairTurn(t *testing.T) {
	m := c.Map{Name: "stairs", Boxes: []c.Box{
		c.B(-12, -1, -20, 20, 0, 12, c.Concrete),
		c.B(0, 3.6, -12, 12, 4, 10, c.Metal),
	}}
	for i := range 8 {
		m.Boxes = append(m.Boxes, c.B(12, 0.5*float64(i), -10.4+0.8*float64(i), 16, 0.5*float64(i+1), -4, c.Metal))
	}
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{14, 0, -16}}}
	g := newGrid(&m)
	sp, _ := g.nearest([3]float64{14, 0, -16})
	g.flood([]int{sp})
	g.clearance()
	at := func(p [3]float64) int {
		n, ok := g.nearest(p)
		if !ok || math.Abs(g.y[n]-p[1]) > 0.01 {
			t.Fatalf("no floor at %v (%v)", p, g.pos(n))
		}
		return n
	}
	below, corner, top := at([3]float64{13.125, 2, -8.375}), at([3]float64{13.125, 3.5, -5.875}), at([3]float64{11.875, 4, -6.125})
	if s := g.stopShort(below, corner); g.y[s] != 3 {
		t.Errorf("a walker up the stair stops short of the corner at %v, want a step lower (3)", g.pos(s))
	}
	if g.turns(below, corner, top) {
		t.Error("the corner on the stair counts as a turn")
	}
	// The grid route up the stair and onto the raised floor, pulled: every
	// corner turns.
	r := g.route(at([3]float64{14.125, 0, -14.125}), at([3]float64{6.125, 4, -6.125}), 100)
	ws, ok := g.pull(r, nil)
	if !ok {
		t.Fatal("no chain")
	}
	chain := append(append([]int{r[0]}, ws...), r[len(r)-1])
	for i := 1; i+1 < len(chain); i++ {
		if !g.turns(chain[i-1], chain[i], chain[i+1]) {
			t.Errorf("corner %d %v: a walker from %v doesn't turn there for %v", i, g.pos(chain[i]), g.pos(chain[i-1]), g.pos(chain[i+1]))
		}
	}
}
