package mapgen

import (
	"math"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
)

func TestFillSpawnsTopsUpInsideTheBuyZone(t *testing.T) {
	b := New("t").Add(Enclosure(20, 4, 1, c.Concrete, c.Brick)...)
	b.Add(c.B(-15, 0, -2, -13, 2, 2, c.Crate)) // a crate in the vibe area
	b.Add(c.B(-2, 0, -20, 2, 3, 20, c.Brick))  // a wall between the sides
	b.TeamSpawns(c.Vibe, [3]float64{-16, 0, -1}, [3]float64{-16, 0, 1})
	b.TeamSpawns(c.Artisan, [3]float64{16, 0, 0})
	b.BuyAroundSpawns(3)
	b.Meta("", 12).FillSpawns(true)
	m := b.Map()
	for _, team := range c.Teams {
		sp := m.TeamSpawns[team]
		if len(sp) != 6 {
			t.Fatalf("%s: %d spawns, want 6", team, len(sp))
		}
		for i, s := range sp {
			if !m.BuyZones[team][0].Contains(s) || !OnFloor(m.Boxes, s) || !HullFree(m.Boxes, s) {
				t.Errorf("%s spawn %v: outside its zone, off the floor or in a box", team, s)
			}
			for _, o := range sp[i+1:] {
				if math.Hypot(o[0]-s[0], o[2]-s[2]) < SpawnGap {
					t.Errorf("%s spawns %v %v too close", team, s, o)
				}
			}
		}
	}
	if len(m.Spawns) != 12 {
		t.Fatalf("general spawns %d, want 12", len(m.Spawns))
	}
	// Map again: nothing more to add.
	if m2 := b.Map(); len(m2.TeamSpawns[c.Vibe]) != 6 {
		t.Fatal("a second Map added spawns")
	}
}

func TestFillSpawnsKeepsCover(t *testing.T) {
	// No wall: every point sees the enemy; with cover nothing can be added.
	b := New("t").Add(Enclosure(20, 4, 1, c.Concrete, c.Brick)...)
	b.TeamSpawns(c.Vibe, [3]float64{-16, 0, 0})
	b.TeamSpawns(c.Artisan, [3]float64{16, 0, 0})
	b.BuyAroundSpawns(3).Meta("", 8).FillSpawns(true)
	if n := len(b.Map().TeamSpawns[c.Vibe]); n != 1 {
		t.Fatalf("%d vibe spawns in the open with cover on, want 1", n)
	}
	if !SeesAny(b.Map().Boxes, [3]float64{-16, 0, 0}, [][3]float64{{16, 0, 0}}) {
		t.Fatal("open ground: they should see each other")
	}
}

func TestSpawnPointsAvoidMoversAndStopWhenFull(t *testing.T) {
	m := c.Map{Boxes: Enclosure(5, 4, 1, c.Concrete, c.Brick)}
	m.Movers = []c.Mover{{Kind: c.MoverSlide, Min: [3]float64{-1, 0, -1}, Max: [3]float64{1, 1, 1}, Move: [3]float64{2, 0, 0}}}
	zone := []c.Zone{{Min: [3]float64{-2, -0.5, -2}, Max: [3]float64{4, 3, 2}}}
	pts := SpawnPoints(m, zone, [][3]float64{{-1.5, 0, -1.5}}, 50, nil)
	if len(pts) == 0 || len(pts) >= 50 {
		t.Fatalf("%d points in a small zone", len(pts))
	}
	for _, p := range pts {
		if p[0]+SpawnR > -1 && p[0]-SpawnR < 3 && p[2]+SpawnR > -1 && p[2]-SpawnR < 1 {
			t.Errorf("point %v in the mover's path", p)
		}
	}
	if SpawnPoints(m, zone, nil, 3, nil) != nil {
		t.Fatal("no hand-placed spawns: nothing to fill around")
	}
}
