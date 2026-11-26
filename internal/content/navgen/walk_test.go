package navgen_test

import (
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
