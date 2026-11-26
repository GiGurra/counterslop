package mapcheck

import (
	"fmt"
	"math"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// NavPath is the shortest nav route (by distance) from node `from` to any node
// inside zone.
func NavPath(m c.Map, from string, zone c.Zone) []c.NavNode {
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

// Walk runs the real movement code (internal/sim, the server's twin of
// web/engine/physics.js) at the sim rate, steering straight at each waypoint
// in turn. It fails if a leg takes much longer than running it would (stuck
// on a wall, a step too high) or ends at the wrong height (missed the stairs).
func Walk(m c.Map, start [3]float64, route []c.NavNode) (sim.Vec3, error) {
	return WalkIn(m, start, route, sim.MoveRules{})
}

// WalkIn is walk under the given movement rules.
func WalkIn(m c.Map, start [3]float64, route []c.NavNode, move sim.MoveRules) (sim.Vec3, error) {
	end, _, err := WalkTimedIn(m, start, route, move)
	return end, err
}

// MoveModels are the movement models every route must walk in: a server or
// a map may run either (sim.MoveRules, sv_movement).
var MoveModels = map[string]sim.MoveRules{"classic": {Model: sim.MoveClassic}, "source": {Model: sim.MoveSource}}

// WalkTimed is walk, also returning the seconds the route took (after settling).
func WalkTimed(m c.Map, start [3]float64, route []c.NavNode) (sim.Vec3, float64, error) {
	return WalkTimedIn(m, start, route, sim.MoveRules{})
}

func WalkTimedIn(m c.Map, start [3]float64, route []c.NavNode, move sim.MoveRules) (sim.Vec3, float64, error) {
	w := sim.NewWorld(&m)
	w.Move = move
	body := sim.NewBody(sim.Vec3(start))
	const dt = 0.01
	tick := 0
	step := func(in sim.Input) { // on the movers' clock: tick k at k/100 s (CmdTime)
		tick++
		w.T = float64(tick) / 100
		w.Step(body, in, dt)
	}
	for i := 0; i < 50; i++ { // settle onto the floor
		step(sim.Input{})
	}
	settled := tick
	secs := func() float64 { return float64(tick-settled) * dt }
	var blocked [3]int
	// The level a waypoint was reached on: checked where the body stands. One
	// reached in the air (coming down stairs: the classic model falls off each
	// step) is checked when it lands.
	var airborne *c.NavNode
	stalled := false // the last step went (almost) nowhere
	landed := func() error {
		if airborne != nil && body.OnGround {
			n := airborne
			airborne = nil
			if math.Abs(body.Pos[1]-n.Pos[1]) > 0.6 {
				return fmt.Errorf("reached %s at height %.2f, want %.2f", n.Name, body.Pos[1], n.Pos[1])
			}
		}
		return nil
	}
	for ri := range route {
		n := route[ri]
		blocked = [3]int{}
		dx, dz := n.Pos[0]-body.Pos[0], n.Pos[2]-body.Pos[2]
		limit := int((math.Hypot(dx, dz)/sim.Run*1.6 + 1) / dt)
		for t := 0; ; t++ {
			dx, dz = n.Pos[0]-body.Pos[0], n.Pos[2]-body.Pos[2]
			if math.Hypot(dx, dz) < 0.5 {
				break
			}
			if t > limit {
				return body.Pos, secs(), fmt.Errorf("stuck at %.2v on the way to %s %v", body.Pos, n.Name, n.Pos)
			}
			if move.Model == sim.MoveSource {
				// Source keeps momentum, accelerates slowly and has friction: a
				// player steers along the velocity error toward running at the
				// waypoint (or orbits it), and along a wall it is blocked by (or
				// stalls against it at a shallow angle, as in CS).
				d := math.Hypot(dx, dz)
				vx, vz := dx/d*sim.Run, dz/d*sim.Run
				if blocked[0] > 0 && blocked[2] == 0 {
					vx, vz = 0, math.Copysign(sim.Run, vz)
				} else if blocked[2] > 0 && blocked[0] == 0 {
					vx, vz = math.Copysign(sim.Run, vx), 0
				}
				dx, dz = vx-body.Vel[0], vz-body.Vel[2]
			}
			body.Yaw = math.Atan2(-dx, -dz)
			from := body.Pos
			// A waypoint higher than a step, close by, and the body held up (at
			// a ledge's face): jump, as a bot does (internal/game ai.go aiMove;
			// a bot also jumps for one up a steep ramp, the walk doesn't need to).
			rise := n.Pos[1] - body.Pos[1]
			jump := body.OnGround && stalled && rise > 0.5 && rise < 1.2 && math.Hypot(n.Pos[0]-body.Pos[0], n.Pos[2]-body.Pos[2]) < 1.2
			step(sim.Input{F: 1, Jump: jump})
			stalled = math.Hypot(body.Pos[0]-from[0], body.Pos[2]-from[2]) < 0.005
			if err := landed(); err != nil {
				return body.Pos, secs(), err
			}
			// An axis the body wished along but did not move on: a wall; slide
			// along it for a while, then try again.
			for _, a := range []int{0, 2} {
				if blocked[a] > 0 {
					blocked[a]--
				} else if math.Abs(n.Pos[a]-from[a]) > 0.3 && math.Abs(body.Pos[a]-from[a]) < 1e-9 {
					blocked[a] = 20
				}
			}
		}
		if !body.OnGround && body.Pos[1] > n.Pos[1] { // still coming down: on landing
			airborne = &route[ri]
		} else if math.Abs(body.Pos[1]-n.Pos[1]) > 0.6 {
			return body.Pos, secs(), fmt.Errorf("reached %s at height %.2f, want %.2f", n.Name, body.Pos[1], n.Pos[1])
		}
	}
	for t := 0; airborne != nil && t < 100; t++ { // the last one: let it land
		step(sim.Input{})
		if err := landed(); err != nil {
			return body.Pos, secs(), err
		}
	}
	return body.Pos, secs(), nil
}
