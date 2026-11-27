package maps

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapcheck"
	"github.com/gigurra/counterslop/internal/sim"
)

// pkRunner runs one body through pk_happy_path with the real movement code,
// one command (10 ms) at a time, the movers on the command's clock.
type pkRunner struct {
	t    *testing.T
	m    c.Map
	w    *sim.World
	b    *sim.Body
	tick int
	low  float64 // the lowest the feet went
}

const pkEast = -math.Pi / 2

func newPkRunner(t *testing.T, move sim.MoveRules, at [3]float64) *pkRunner {
	m := PkHappyPath()
	w := sim.NewWorld(&m)
	move.Hook = sim.DefaultHook()
	move.Hook.Recharge = 40 // the map's rule (sv_hook_recharge)
	w.Move = move
	b := sim.NewBody(sim.Vec3(at))
	b.Yaw, b.Boots, b.Energy = pkEast, true, move.Hook.Energy // the boots fitted full (internal/game fitBoots)
	r := &pkRunner{t: t, m: m, w: w, b: b, low: at[1]}
	for range 30 { // settle
		r.step(sim.Input{})
	}
	return r
}

func (r *pkRunner) step(in sim.Input) sim.Events {
	r.tick++
	r.w.T = float64(r.tick) / 100
	ev := r.w.Step(r.b, in, 0.01)
	r.low = math.Min(r.low, r.b.Pos[1])
	return ev
}

// pit: the body is in one of the map's pits (a fall: back on the gate).
func (r *pkRunner) pit() bool {
	for _, z := range r.m.Pits {
		if z.Contains(r.b.Pos) {
			return true
		}
	}
	return false
}

// run holds forward (east) until done, jumping when jump says, for at most
// secs; it fails on a fall into a pit or the time running out.
func (r *pkRunner) run(what string, secs float64, jump func(b *sim.Body) bool, done func(b *sim.Body) bool) {
	r.t.Helper()
	for i := 0; i < int(secs*100); i++ {
		r.b.Yaw = pkEast
		r.step(sim.Input{F: 1, Jump: jump != nil && r.b.OnGround && jump(r.b)})
		if r.pit() {
			r.t.Fatalf("%s: fell into a pit at %.2v", what, r.b.Pos)
		}
		if done(r.b) {
			return
		}
	}
	r.t.Fatalf("%s: not done in %.0f s, at %.2v", what, secs, r.b.Pos)
}

// rest: no keys until on the ground and still (a pad's skid), at most secs.
func (r *pkRunner) rest(what string, secs float64) {
	r.t.Helper()
	for i := 0; i < int(secs*100); i++ {
		r.step(sim.Input{})
		if r.pit() {
			r.t.Fatalf("%s: fell into a pit at %.2v", what, r.b.Pos)
		}
		if r.b.OnGround && math.Hypot(r.b.Vel[0], r.b.Vel[2]) < 0.05 {
			return
		}
	}
	r.t.Fatalf("%s: still moving after %.0f s, at %.2v", what, secs, r.b.Pos)
}

// hookTo: aim at target from the eye, fire the hook and hold it (and
// forward) until done or the hook lets go; then let go.
func (r *pkRunner) hookTo(what string, target [3]float64, done func(b *sim.Body, ev sim.Events) bool) {
	r.t.Helper()
	eye := r.b.Pos[1] + sim.Eye
	dx, dz := target[0]-r.b.Pos[0], target[2]-r.b.Pos[2]
	r.b.Yaw = math.Atan2(-dx, -dz)
	pitch := math.Atan2(target[1]-eye, math.Hypot(dx, dz))
	r.b.Pitch = pitch // the hook fires along the body's view
	for i := 0; i < 600; i++ {
		ev := r.step(sim.Input{Hook: true, HookPress: i == 0, Pitch: pitch})
		if i == 0 && ev.Hook != sim.HookFired {
			r.t.Fatalf("%s: the hook didn't latch (event %d) from %.2v at %v", what, ev.Hook, r.b.Pos, target)
		}
		if r.pit() {
			r.t.Fatalf("%s: fell into a pit at %.2v", what, r.b.Pos)
		}
		if done(r.b, ev) {
			r.step(sim.Input{}) // let go
			return
		}
		if i > 0 && !r.b.Hook.On {
			r.t.Fatalf("%s: the hook let go at %.2v", what, r.b.Pos)
		}
	}
	r.t.Fatalf("%s: not there in 6 s, at %.2v", what, r.b.Pos)
}

// hop: run east from where the body stands, jump at the edge (from), no keys
// in the air; it must land with its feet in [x0, x1] (a lift, a block).
func (r *pkRunner) hop(what string, from, x0, x1 float64) {
	r.t.Helper()
	air := false
	for i := 0; i < 400; i++ {
		r.b.Yaw = pkEast
		in := sim.Input{F: 1, Jump: r.b.OnGround && r.b.Pos[0] > from}
		if air || !r.b.OnGround {
			in = sim.Input{}
			air = true
		}
		r.step(in)
		if r.pit() {
			r.t.Fatalf("%s: fell into a pit at %.2v (aiming for x %v..%v)", what, r.b.Pos, x0, x1)
		}
		if air && r.b.OnGround {
			if r.b.Pos[0] < x0 || r.b.Pos[0] > x1 {
				r.t.Fatalf("%s: landed at %.2v, want x %v..%v", what, r.b.Pos, x0, x1)
			}
			return
		}
	}
	r.t.Fatalf("%s: no landing in 4 s, at %.2v", what, r.b.Pos)
}

// wallWalk: on a wall (gravity turned onto it), forward along whichever yaw
// climbs (the view keeps its yaw in the wall's frame) until over the top
// edge, gravity down again, standing on top.
func (r *pkRunner) wallWalk(what string, top float64) {
	r.t.Helper()
	best, up := 0.0, -1e9
	for _, yaw := range []float64{0, math.Pi / 2, math.Pi, -math.Pi / 2} {
		probe := *r.b
		w := *r.w
		for range 30 {
			probe.Yaw = yaw
			w.T += 0.01
			w.Step(&probe, sim.Input{F: 1}, 0.01)
		}
		if probe.Pos[1] > up {
			best, up = yaw, probe.Pos[1]
		}
	}
	for i := 0; r.b.Grav != sim.GravDown; i++ {
		if i > 1500 || r.pit() {
			r.t.Fatalf("%s: not over the top in 15 s, at %.2v gravity %d", what, r.b.Pos, r.b.Grav)
		}
		r.b.Yaw = best
		if ev := r.step(sim.Input{F: 1}); ev.Turn != 0 {
			best += ev.Turn // over an edge: the view turned with the body
		}
	}
	if math.Abs(r.b.Pos[1]-top) > 0.05 {
		r.t.Fatalf("%s: off the wall at %.2v, want on top (y %v)", what, r.b.Pos, top)
	}
}

// jumpOff: a running jump (forward held) up to its top: the hook is fired there.
func (r *pkRunner) jumpOff() {
	for i := 0; i < 200; i++ {
		r.b.Yaw = pkEast
		r.step(sim.Input{F: 1, Jump: r.b.OnGround})
		if !r.b.OnGround && r.b.Vel[1] <= 0 {
			return
		}
	}
	r.t.Fatalf("the jump off: never at its top, at %.2v", r.b.Pos)
}

// back: walk west to x (a run-up), then stand.
func (r *pkRunner) back(x float64) {
	for i := 0; i < 300 && r.b.Pos[0] > x; i++ {
		r.b.Yaw = pkEast
		r.step(sim.Input{F: -1})
	}
	r.rest("backed up", 2)
}

// gateSpawn: section i's start (gate i-1's spawn; 0: a start spawn).
func gateSpawn(m c.Map, i int) [3]float64 { return m.Checkpoints[i].Spawn }

// on: the feet on the ground, on top y, x in [x0, x1].
func on(x0, x1, y float64) func(b *sim.Body) bool {
	return func(b *sim.Body) bool {
		return b.OnGround && b.Pos[0] >= x0 && b.Pos[0] <= x1 && math.Abs(b.Pos[1]-y) < 0.05
	}
}

// edgeJump: jump when the feet are within d of one of the edges ahead.
func edgeJump(d float64, edges ...float64) func(b *sim.Body) bool {
	return func(b *sim.Body) bool {
		for _, e := range edges {
			if e-b.Pos[0] > 0 && e-b.Pos[0] < d {
				return true
			}
		}
		return false
	}
}

// Every section of pk_happy_path can be run with the real movement code in
// both movement models, each from its start gate's spawn to its end gate,
// never into a pit: the stepping stones jumped, the pad chain flown, the
// lifts hopped, the void hooked across, the tower's wall walked up and over,
// the low-gravity slabs hopped, the road driven in a buggy (the jump), the
// basin crossed in a dinghy and swum out of.
func TestHappyPathRuns(t *testing.T) {
	m := PkHappyPath()
	if len(m.Checkpoints) != 10 || !m.OnFoot || !m.PointToPoint {
		t.Fatalf("%d gates, on foot %v, p2p %v", len(m.Checkpoints), m.OnFoot, m.PointToPoint)
	}
	past := func(i int) func(b *sim.Body) bool { // through gate i (its volume)
		g := m.Checkpoints[i]
		return func(b *sim.Body) bool { return g.In([3]float64(b.Pos)) }
	}
	for name, move := range mapcheck.MoveModels {
		t.Run(name, func(t *testing.T) {
			t.Run("lint", func(t *testing.T) {
				r := newPkRunner(t, move, gateSpawn(m, 0))
				r.run("the stepping stones", 20, edgeJump(0.5, 8, 13.5, 19, 24.5, 30, 35.5, 41), past(1))
			})
			t.Run("build", func(t *testing.T) {
				r := newPkRunner(t, move, gateSpawn(m, 1))
				launched := false
				for i := 0; i < 300 && !launched; i++ {
					r.b.Yaw = pkEast
					launched = r.step(sim.Input{F: 1}).Launched
				}
				if !launched {
					t.Fatalf("never reached the first pad: at %.2v", r.b.Pos)
				}
				r.rest("the pad chain", 30)
				if !on(142, 166, pkIslandC)(r.b) {
					t.Fatalf("the pad chain ended at %.2v, want on build's last island before the gate", r.b.Pos)
				}
				r.run("to the gate", 10, nil, past(2))
			})
			t.Run("staging", func(t *testing.T) {
				r := newPkRunner(t, move, gateSpawn(m, 2))
				// lift k is at its bottom at t = 6n + 3(k%2), its top 3 s later.
				waitFor := func(phase float64) { // until the movers' clock mod 6 is phase (to a tick)
					for i := 0; i < 700 && math.Abs(math.Mod(r.w.T, 6)-phase) > 0.006; i++ {
						r.step(sim.Input{})
					}
				}
				r.run("to the run-up", 5, nil, func(b *sim.Body) bool { return b.Pos[0] > 171 })
				r.rest("at the run-up", 2)
				// Onto lift 1 as it is at its bottom, on to lift 2 as lift 1 tops
				// out (lift 2 at its bottom), to lift 3 the same, onto the island:
				// a run and a jump at the edge, no keys in the air.
				for k, land := range []struct{ x0, x1, phase float64 }{{177, 182, 5.3}, {184, 189, 2.4}, {191, 196, 5.4}, {198, 230, 2.4}} {
					waitFor(land.phase)
					r.hop("lift hop", land.x0-2.5, land.x0, land.x1)
					if k < 3 {
						r.rest("on the lift", 1)
						r.back(land.x0 + 1.5) // back to the lift's start: a run-up
					}
				}
				if math.Abs(r.b.Pos[1]-pkD) > 0.05 {
					t.Fatalf("off the lifts at %.2v, want on the void's island (y %v)", r.b.Pos, pkD)
				}
				r.run("to the gate", 10, nil, past(3))
			})
			t.Run("the void", func(t *testing.T) {
				r := newPkRunner(t, move, gateSpawn(m, 3))
				r.run("to the edge", 5, nil, func(b *sim.Body) bool { return b.Pos[0] > 228.5 })
				for _, hop := range []struct{ x0, x1, y float64 }{{240, 250, pkD - 1}, {260, 270, pkD - 2}, {280, 300, pkE}} {
					r.jumpOff()
					r.hookTo("hook the next block's face", [3]float64{hop.x0, hop.y - 2, 0}, func(b *sim.Body, ev sim.Events) bool { return b.Grav != sim.GravDown })
					r.wallWalk("up the block", hop.y)
					r.rest("on top", 3)
					if hop.x1 < 300 {
						r.back(hop.x0 + 1.5)
						r.run("to the edge", 3, nil, func(b *sim.Body) bool { return b.Pos[0] > hop.x1-1 })
					}
				}
				r.run("to the gate", 10, nil, past(4))
			})
			t.Run("wall run", func(t *testing.T) {
				r := newPkRunner(t, move, gateSpawn(m, 4))
				r.run("to the edge", 5, nil, func(b *sim.Body) bool { return b.Pos[0] > 299 })
				r.rest("at the edge", 2)
				r.hookTo("hook the tower's face", [3]float64{304, pkE + 8, 0}, func(b *sim.Body, ev sim.Events) bool { return b.Grav != sim.GravDown })
				r.wallWalk("the wall run", pkTower)
				r.run("to the gate", 10, nil, past(5))
			})
			t.Run("canary", func(t *testing.T) {
				r := newPkRunner(t, move, gateSpawn(m, 5))
				r.run("the low-gravity slabs", 30, edgeJump(0.4, 316, 332, 348, 364, 380), past(6))
			})
			t.Run("finish", func(t *testing.T) {
				r := newPkRunner(t, move, gateSpawn(m, 8))
				r.run("to the finish", 10, nil, past(9))
			})
		})
	}
	t.Run("deploy (the buggy)", func(t *testing.T) {
		spec := pkCarSpec(t, "buggy")
		w := sim.NewWorld(&m)
		sp := m.Cars[1]
		car := sim.NewCar(spec, sim.Vec3(sp.Pos), sp.Yaw)
		g := m.Checkpoints[7]
		passed, low := false, car.Pos[1]
		for i := 0; i < 30*100; i++ {
			w.T = float64(i) / 100
			// Steer at a point 15 m ahead on the road's middle line (z 0), as the race bots do.
			want := math.Atan2(-15, car.Pos[2])
			e := math.Remainder(want-car.Yaw, 2*math.Pi)
			in := sim.CarInput{Throttle: 1, Steer: math.Max(-1, math.Min(1, -e*2))}
			if passed {
				in = sim.CarInput{Throttle: -1}
			}
			w.StepCar(spec, car, in, 0.01)
			low = math.Min(low, car.Pos[1])
			passed = passed || g.In([3]float64(car.Pos))
			if passed && math.Hypot(car.Vel[0], car.Vel[2]) < 0.1 {
				break
			}
		}
		if !passed || low < pkF-2 || car.Pos[0] > 574 || math.Abs(car.Pos[1]-pkF) > 0.5 {
			t.Fatalf("the buggy: through the gate %v, lowest %.2f, stopped at %.2v (want on the road before the basin)", passed, low, car.Pos)
		}
	})
	t.Run("rollout (the dinghy)", func(t *testing.T) {
		w := sim.NewWorld(&m)
		sp := m.Boats[1]
		boat := &sim.Boat{Pos: sim.Vec3(sp.Pos), Yaw: sp.Yaw}
		for i := 0; i < 20*100 && boat.Pos[0] < 697; i++ {
			w.T = float64(i) / 100
			w.StepBoat(boat, sim.BoatInput{F: 1}, 0.01)
		}
		if boat.Pos[0] < 697 || math.Abs(boat.Pos[2]) > 13 {
			t.Fatalf("the dinghy: at %.2v after 20 s, want at the far quay (x 700)", boat.Pos)
		}
		// Off it (or fallen in): swim to the quay and the water jump out.
		r := newPkRunner(t, mapcheck.MoveModels["classic"], [3]float64{697, pkSurf - 1, 0})
		for i := 0; i < 500 && !(r.b.OnGround && r.b.Pos[1] > pkF-0.05); i++ {
			r.b.Yaw = pkEast
			swim := r.b.Pos[1] < pkF-0.3
			r.step(sim.Input{F: 1, Jump: swim, JumpHeld: swim})
		}
		if !r.b.OnGround || math.Abs(r.b.Pos[1]-pkF) > 0.05 {
			t.Fatalf("the water jump: at %.2v, want on the far quay (y %v)", r.b.Pos, pkF)
		}
		r.run("to the gate", 10, nil, past(8))
	})
}

func pkCarSpec(t *testing.T, id string) *sim.CarSpec {
	t.Helper()
	b, err := os.ReadFile("../../../web/assets/cars.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Cars []*sim.CarSpec }
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	for _, s := range f.Cars {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no car %q", id)
	return nil
}
