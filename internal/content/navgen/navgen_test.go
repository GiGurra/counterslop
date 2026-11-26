package navgen

import (
	"encoding/json"
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
