package navgen_test

import (
	"slices"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapcheck"
	"github.com/gigurra/counterslop/internal/content/navgen"
	"github.com/gigurra/counterslop/internal/content/prefabs"
)

// The graphs navgen builds walk with the real movement code (mapcheck's
// walk, both movement models): from every spawn of every team to every
// objective.
func walks(t *testing.T, m c.Map) {
	t.Helper()
	m.Nav = navgen.Build(&m)
	if len(m.Nav) == 0 {
		t.Fatal("no graph")
	}
	for _, p := range mapcheck.WalkProblems(m) {
		t.Error(p)
	}
}

// de_stack's stair: it climbs along a raised floor and you step off it
// sideways near the top (a sharp turn on a stair: a walker stops half a
// metre short of the turn, a step lower, where the side is a wall), and the
// way back down it (the classic model falls off each step: it reaches the
// foot in the air).
func stairs() c.Map {
	m := c.Map{Name: "stairs"}
	m.Boxes = []c.Box{
		c.B(-12, -1, -20, 20, 0, 12, c.Concrete),
		c.B(0, 3.6, -12, 12, 4, 10, c.Metal), // the raised floor (open under it)
	}
	for i := range 8 { // up +z, 0.5 m a step, along its east side
		z := -10.4 + 0.8*float64(i)
		m.Boxes = append(m.Boxes, c.B(12, 0.5*float64(i), z, 16, 0.5*float64(i+1), -4, c.Metal))
	}
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{-8, 0, -4}, {-8, 0, 0}}, c.Artisan: {{6, 4, 4}, {8, 4, 4}}}
	m.Sites = []c.Zone{
		{Name: "A", Min: [3]float64{2, 3.5, -10}, Max: [3]float64{8, 7, -4}},     // up on the raised floor
		{Name: "B", Min: [3]float64{14, -0.5, -18}, Max: [3]float64{19, 3, -13}}, // past the stair's foot
	}
	return m
}

func TestStairsWalk(t *testing.T) { walks(t, stairs()) }

// gs_kiosk's GPU zones: a pedestal fills most of the zone, leaving a sliver
// of floor along it. A walker heading straight at a node there stops half a
// metre short, outside: the way in runs along the sliver (approach).
func kiosk() c.Map {
	m := c.Map{Name: "kiosk"}
	m.Boxes = []c.Box{
		c.B(-12, -1, -14, 30, 0, 14, c.Concrete),
		c.B(20, 0, -6, 22, 0.9, -2, c.Metal), // the pedestal
	}
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{-8, 0, 0}, {-8, 0, 3}}, c.Artisan: {{26, 0, 8}, {26, 0, -8}}}
	m.Sites = []c.Zone{
		{Name: "A", Min: [3]float64{19.29, -0.8, -6.06}, Max: [3]float64{21.41, 1.5, -3.94}},
		{Name: "B", Min: [3]float64{19.29, -0.8, -4.06}, Max: [3]float64{21.41, 1.5, -1.94}}, // overlapping A, as gs_kiosk's do
	}
	return m
}

func TestSliverZoneWalk(t *testing.T) { walks(t, kiosk()) }

// Two floors joined by an up escalator alone (a lane 1.2 m wide between
// balustrades, its treads moving): the way up rides it, the way down walks
// against it.
func escalatorMap() c.Map {
	m := c.Map{Name: "escalator"}
	m.Boxes = []c.Box{
		c.B(-10, -1, -30, 10, 0, 20, c.Concrete),
		c.B(-10, 4.6, 0, 10, 5, 20, c.Concrete), // the upper floor, its edge at z 0
	}
	m.Boxes = append(m.Boxes, prefabs.Escalator(true, 0, -0.6, 1.2, 0, 5, -1, c.Escalator, c.Glass)...)
	m.Movers = []c.Mover{prefabs.EscalatorMover(true, 0, -0.6, 1.2, 0, 5, -1, 0.75, c.Escalator)}
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{-6, 0, -24}, {6, 0, -24}}, c.Artisan: {{-6, 5, 16}, {6, 5, 16}}}
	m.Sites = []c.Zone{
		{Name: "A", Min: [3]float64{-8, 4.5, 6}, Max: [3]float64{8, 8, 12}}, // upstairs
		{Name: "B", Min: [3]float64{-8, -0.5, -20}, Max: [3]float64{8, 3, -14}},
	}
	return m
}

func TestEscalatorWalk(t *testing.T) { walks(t, escalatorMap()) }

// Site A on a platform 0.9 m up with no stairs: the way up is a jump (bots
// jump for a waypoint above a step close by), the way down a drop. Site B
// on a crate stack two jumps up (0.9 m, then 0.8 m more).
func ledgeMap() c.Map {
	m := c.Map{Name: "ledge"}
	m.Boxes = []c.Box{
		c.B(-20, -1, -20, 20, 0, 20, c.Concrete),
		c.B(4, 0, -8, 16, 0.9, 8, c.Concrete),   // the platform
		c.B(-16, 0, 8, -6, 0.9, 16, c.Crate),    // the stack: a wide step
		c.B(-16, 0.9, 12, -6, 1.7, 16, c.Crate), // and its top
	}
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{-12, 0, -12}, {-10, 0, -12}}, c.Artisan: {{10, 0.9, 0}, {12, 0.9, 0}}}
	m.Sites = []c.Zone{
		{Name: "A", Min: [3]float64{6, 0.5, -6}, Max: [3]float64{14, 4, 6}},
		{Name: "B", Min: [3]float64{-15, 1.5, 13}, Max: [3]float64{-7, 5, 15.5}},
	}
	return m
}

func TestLedgeWalk(t *testing.T) { walks(t, ledgeMap()) }

// Two halls split by a 0.9 m wall half a metre thick, a doorway far along
// it: the grid route hops the wall (up onto it, down off it), but its top
// has no room for a waypoint a metre on: the chain goes round on foot.
func TestWallHopGoesRound(t *testing.T) {
	m := c.Map{Name: "wallhop"}
	m.Boxes = []c.Box{
		c.B(-20, -1, -12, 20, 0, 40, c.Concrete),
		c.B(-0.25, 0, -12, 0.25, 0.9, 30, c.Concrete), // the wall; the doorway z 30..40
	}
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{-14, 0, 0}, {-14, 0, 3}}, c.Artisan: {{14, 0, -6}, {16, 0, -6}}}
	m.Sites = []c.Zone{{Name: "A", Min: [3]float64{8, -0.5, -4}, Max: [3]float64{16, 3, 4}}}
	walks(t, m)
	m.Nav = navgen.Build(&m)
	for _, n := range m.Nav {
		if n.Pos[1] > 0.5 {
			t.Errorf("%s at %v: up on the wall", n.Name, n.Pos)
		}
	}
}

// Islands over the void joined only by jump pads: the vibe's island throws
// onto the middle one (site A, 8 m up, 30 m off), which throws on to the
// artisan's (site B) and back. The graph gets a node on each pad, walked
// onto one way, and the throw to where it lands one way; every team walks
// (and flies) to both sites, and no walking link crosses a pad.
func padIslands() c.Map {
	m := c.Map{Name: "pads"}
	m.Boxes = []c.Box{
		c.B(-40, -1, -8, -20, 0, 8, c.Concrete), // vibe's
		c.B(-6, 7, -8, 6, 8, 8, c.Concrete),     // the middle, site A
		c.B(20, -1, -8, 40, 0, 8, c.Concrete),   // artisan's, site B
	}
	pad := func(name string, x, y, z float64, to [3]float64) c.Launcher {
		return c.Launcher{Name: name, Min: [3]float64{x - 1, y - 0.1, z - 1}, Max: [3]float64{x + 1, y + 0.3, z + 1}, Target: to, Apex: 3}
	}
	m.Launchers = []c.Launcher{
		pad("vibe out", -22.5, 0, 0, [3]float64{-2, 8, 0}),
		pad("middle east", 4, 8, 4, [3]float64{25, 0, 4}),
		pad("middle west", -4, 8, -4, [3]float64{-25, 0, -4}),
		pad("artisan out", 22.5, 0, -4, [3]float64{2, 8, -2}),
	}
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{-35, 0, -3}, {-35, 0, 3}}, c.Artisan: {{35, 0, -3}, {35, 0, 3}}}
	m.Sites = []c.Zone{
		{Name: "A", Min: [3]float64{-5, 7.5, -7}, Max: [3]float64{1, 11, 7}},
		{Name: "B", Min: [3]float64{26, -0.5, -7}, Max: [3]float64{32, 3, 7}},
	}
	return m
}

func TestPadIslandsWalk(t *testing.T) {
	m := padIslands()
	walks(t, m)
	nav := navgen.Build(&m)
	byName := map[string]c.NavNode{}
	for _, n := range nav {
		byName[n.Name] = n
	}
	for _, l := range m.Launchers {
		p, ok := byName["pad "+l.Name]
		if !ok {
			t.Fatalf("no node on pad %q", l.Name)
		}
		if len(p.Links) != 0 || len(p.OneWay) != 1 || p.OneWay[0] != "landing "+l.Name {
			t.Errorf("pad %q: links %v, one way %v (want only its throw)", l.Name, p.Links, p.OneWay)
		}
		onto := 0
		for _, n := range nav {
			if slices.Contains(n.Links, p.Name) {
				t.Errorf("%s links both ways with %s", n.Name, p.Name)
			}
			if slices.Contains(n.OneWay, p.Name) {
				onto++
			}
		}
		if onto == 0 {
			t.Errorf("nothing walks onto pad %q", l.Name)
		}
	}
}

// An accelerator pad (a fixed throw, no target) over a gap: the graph links
// it to where its flight comes down, and the team walks over it to the far
// island's site (and a lab with gravity zones gets no links for fixed pads:
// lab_moon's flights bend through space).
func TestAcceleratorWalk(t *testing.T) {
	m := c.Map{Name: "accel"}
	m.Boxes = []c.Box{c.B(-30, -1, -8, 0, 0, 8, c.Concrete), c.B(10, -1, -8, 40, 0, 8, c.Concrete)}
	m.Launchers = []c.Launcher{{Name: "hop", Min: [3]float64{-3, -0.1, -2}, Max: [3]float64{-1, 0.3, 2}, Vel: [3]float64{22, 8, 0}}}
	m.TeamSpawns = map[string][][3]float64{c.Vibe: {{-25, 0, -3}, {-25, 0, 3}}, c.Artisan: {{-20, 0, -3}, {-20, 0, 3}}}
	m.Sites = []c.Zone{
		{Name: "A", Min: [3]float64{-15, -0.5, -7}, Max: [3]float64{-8, 3, 7}},
		{Name: "B", Min: [3]float64{20, -0.5, -7}, Max: [3]float64{30, 3, 7}},
	}
	walks(t, m)
	nav := navgen.Build(&m)
	var hop *c.NavNode
	for i := range nav {
		if nav[i].Name == "pad hop" {
			hop = &nav[i]
		}
	}
	if hop == nil || len(hop.OneWay) != 1 {
		t.Fatalf("no link over the accelerator: %+v", hop)
	}
	m.GravZones = []c.GravZone{{Name: "half", Min: [3]float64{-50, -5, -20}, Max: [3]float64{50, 50, 20}, G: 0.5}}
	for _, n := range navgen.Build(&m) {
		if n.Name == "pad hop" {
			t.Fatal("a fixed pad linked under a gravity zone")
		}
	}
}
