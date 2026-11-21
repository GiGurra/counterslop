package maps

import (
	"fmt"
	"math"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// navPath is the shortest nav route (by distance) from node `from` to any node
// inside zone.
func navPath(m c.Map, from string, zone c.Zone) []c.NavNode {
	byName := map[string]c.NavNode{}
	adj := map[string][]string{}
	for _, n := range m.Nav {
		byName[n.Name] = n
		for _, l := range n.Links {
			adj[n.Name] = append(adj[n.Name], l)
			adj[l] = append(adj[l], n.Name)
		}
	}
	dist := map[string]float64{from: 0}
	prev := map[string]string{}
	done := map[string]bool{}
	for {
		cur, best := "", math.Inf(1)
		for n, d := range dist {
			if !done[n] && d < best {
				cur, best = n, d
			}
		}
		if cur == "" {
			return nil
		}
		if zone.Contains(byName[cur].Pos) {
			var path []c.NavNode
			for n := cur; ; n = prev[n] {
				path = append([]c.NavNode{byName[n]}, path...)
				if n == from {
					return path
				}
			}
		}
		done[cur] = true
		for _, l := range adj[cur] {
			a, b := byName[cur].Pos, byName[l].Pos
			d := best + math.Hypot(b[0]-a[0], b[2]-a[2])
			if old, ok := dist[l]; !ok || d < old {
				dist[l], prev[l] = d, cur
			}
		}
	}
}

// walk runs the real movement code (internal/sim, the server's twin of
// web/engine/physics.js) at the sim rate, steering straight at each waypoint
// in turn. It fails if a leg takes much longer than running it would (stuck
// on a wall, a step too high) or ends at the wrong height (missed the stairs).
func walk(m c.Map, start [3]float64, route []c.NavNode) (sim.Vec3, error) {
	w := sim.NewWorld(&m)
	body := sim.NewBody(sim.Vec3(start))
	const dt = 0.01
	for i := 0; i < 50; i++ { // settle onto the floor
		w.Step(body, sim.Input{}, dt)
	}
	for _, n := range route {
		dx, dz := n.Pos[0]-body.Pos[0], n.Pos[2]-body.Pos[2]
		limit := int((math.Hypot(dx, dz)/sim.Run*1.6 + 1) / dt)
		for t := 0; ; t++ {
			dx, dz = n.Pos[0]-body.Pos[0], n.Pos[2]-body.Pos[2]
			if math.Hypot(dx, dz) < 0.5 {
				break
			}
			if t > limit {
				return body.Pos, fmt.Errorf("stuck at %.2v on the way to %s %v", body.Pos, n.Name, n.Pos)
			}
			body.Yaw = math.Atan2(-dx, -dz)
			w.Step(body, sim.Input{F: 1}, dt)
		}
		if math.Abs(body.Pos[1]-n.Pos[1]) > 0.6 {
			return body.Pos, fmt.Errorf("reached %s at height %.2f, want %.2f", n.Name, body.Pos[1], n.Pos[1])
		}
	}
	return body.Pos, nil
}

// Every team can walk from every one of its spawns to every objective along the
// nav graph: both sites, or in GPU Drop every GPU spot and every exit.
func TestWalkToSites(t *testing.T) {
	for _, name := range Order {
		m := All[name]()
		for _, team := range c.Teams {
			for si, spawn := range m.TeamSpawns[team] {
				for _, site := range m.Objectives() {
					route := navPath(m, team, site)
					if route == nil {
						t.Errorf("%s: no nav route from %q to site %s", name, team, site.Name)
						continue
					}
					end, err := walk(m, spawn, route)
					if err != nil {
						t.Errorf("%s: %s spawn %d -> %s: %v", name, team, si, site.Name, err)
					} else if !site.Contains(end) {
						t.Errorf("%s: %s spawn %d -> %s: ended at %.2v, outside the site", name, team, si, site.Name, end)
					}
				}
			}
		}
	}
}

// Every nav link can be walked both ways with the real movement code: the
// routes bots take that aren't the shortest to a site (lanes, rotations) work
// too. Competitive and GPU Drop maps: those are the ones with lanes.
func TestWalkEveryNavLink(t *testing.T) {
	for _, name := range Order {
		m := All[name]()
		if m.Category != c.Competitive && m.Category != c.GPUDrop {
			continue
		}
		byName := map[string]c.NavNode{}
		for _, n := range m.Nav {
			byName[n.Name] = n
		}
		for _, n := range m.Nav {
			for _, l := range n.Links {
				for _, leg := range [][2]c.NavNode{{n, byName[l]}, {byName[l], n}} {
					if _, err := walk(m, leg[0].Pos, []c.NavNode{leg[1]}); err != nil {
						t.Errorf("%s: %s -> %s: %v", name, leg[0].Name, leg[1].Name, err)
					}
				}
			}
		}
	}
}
