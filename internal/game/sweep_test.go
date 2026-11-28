package game

import (
	"strings"
	"testing"
	"time"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// burst: a client's hitch. n ticks without a command from p (its body
// waits for them; its budget banks them), then n commands of cmd at once,
// all run in the next server tick.
func burst(s *Server, p *Player, n int, cmd Cmd, now *time.Time) {
	ticks(s, n, now, nil)
	cs := make([]Cmd, n)
	for i := range cs {
		cmd.Seq = p.lastQueued + 1 + i
		cs[i] = cmd
	}
	s.queueCmds(p, cs)
	ticks(s, 1, now, nil)
}

// fling puts p's body in the air at from moving at vel (a launch pad's
// throw): the next burst carries it a long way in one tick.
func fling(p *Player, from, vel Vec3) {
	b := p.body
	b.Pos, b.Vel, b.OnGround = sim.Vec3(from), sim.Vec3(vel), false
	p.Pos = from
}

// Every trigger swept along a burst: through it counts, past it (both ends
// of the burst well clear of it, the path beside it) does not.

// A race power-up: a car through one in a 40-command burst takes it.
func TestPickupTakenThroughACommandBurst(t *testing.T) {
	for _, miss := range []bool{false, true} {
		s, v, _, now := raceServer(t, "2")
		z := 9.0
		if miss {
			z += 4 // beside the car's line: 4 m off, more than PickupRadius
		}
		s.Map.Pickups = []c.Pickup{{Kind: c.PickupHotfix, Pos: [3]float64{-1, 0, z}}}
		goRace(t, s, now)
		s.resetPickups()
		car := s.carByID(v.car)
		car.sim.Pos, car.sim.Vel = [3]float64{4, car.sim.Pos[1], 9}, [3]float64{-25, 0, 0}
		car.sync()
		ticks(s, 1, now, nil)
		s.tickPickups(*now)
		if !s.round.Pickups[0].Up || v.Pos[0] < 3 {
			t.Fatalf("set-up: at x %.2f, pickup %+v; want short of it, up", v.Pos[0], s.round.Pickups[0])
		}
		burst(s, v, 40, Cmd{F: 1}, now)
		s.tickPickups(*now)
		if v.Pos[0] > -5 {
			t.Fatalf("the burst left the car at x %.2f; want well past the pickup (x < -5)", v.Pos[0])
		}
		if up := s.round.Pickups[0].Up; up != miss {
			t.Fatalf("miss %v: pickup up %v after the burst, want %v", miss, up, miss)
		}
	}
}

// A GPU Drop exit: a carrier through a (thin) exit in one burst secures it.
func TestGPUSecuredThroughACommandBurst(t *testing.T) {
	for _, miss := range []bool{false, true} {
		s, v, _, now := gpuServer(t)
		i := grabbed(t, s, v, now)
		z0, z1 := -5.0, -1.0
		if miss {
			z0, z1 = 1, 4 // beside the carrier's line
		}
		s.Map.Exits = []c.Zone{{Name: "thin", Min: [3]float64{15, 0, z0}, Max: [3]float64{15.4, 3, z1}}}
		fling(v, Vec3{18, 0.8, -3}, Vec3{-15, 0, 0})
		burst(s, v, 40, Cmd{}, now)
		if v.Pos[0] > 14 {
			t.Fatalf("the burst left the carrier at x %.2f; want past the exit (x < 14)", v.Pos[0])
		}
		if secured := s.round.GPUs[i].State == GPUSecured; secured == miss {
			t.Fatalf("miss %v: GPU %+v after the burst", miss, s.round.GPUs[i])
		}
	}
}

// A ticket gate: walked through in one burst, the ticket is scanned.
func TestTicketGateThroughACommandBurst(t *testing.T) {
	for _, miss := range []bool{false, true} {
		s, v, _, now := ticketServer(t)
		z0, z1 := -2.0, 2.0
		if miss {
			z0, z1 = 3, 6
		}
		s.Map.TicketGates = []c.Zone{{Name: "gate", Min: [3]float64{-60, 0, z0}, Max: [3]float64{-59.6, 2.6, z1}}}
		drain(v)
		fling(v, Vec3{-55, 0.8, 0}, Vec3{-20, 0, 0})
		burst(s, v, 40, Cmd{}, now)
		if v.Pos[0] > -61 {
			t.Fatalf("the burst left them at x %.2f; want past the gate (x < -61)", v.Pos[0])
		}
		if scanned := strings.Contains(drain(v), `"t":"ticket"`); scanned == miss {
			t.Fatalf("miss %v: scanned %v", miss, scanned)
		}
	}
}

// A floor weapon: run over in one burst, it is picked up.
func TestFloorWeaponTakenThroughACommandBurst(t *testing.T) {
	for _, miss := range []bool{false, true} {
		s, v, _, now := ticketServer(t)
		v.inv.Slots["primary"] = nil
		z := 0.0
		if miss {
			z = 3
		}
		s.items = nil
		s.spawnItem(*s.Weapons.newHeld("prompt"), Vec3{-59, 0, z}, -1)
		fling(v, Vec3{-55, 0.8, 0}, Vec3{-20, 0, 0})
		burst(s, v, 40, Cmd{}, now)
		s.tickItems(*now)
		if v.Pos[0] > -62 {
			t.Fatalf("the burst left them at x %.2f; want past the weapon (x < -62)", v.Pos[0])
		}
		if took := v.inv.Slots["primary"] != nil; took == miss {
			t.Fatalf("miss %v: took it %v (items %d)", miss, took, len(s.items))
		}
	}
}

// nearPath: a cylinder swept along a step.
func TestNearPath(t *testing.T) {
	q := Vec3{0, 0, 0}
	for _, k := range []struct {
		a, b Vec3
		want bool
	}{
		{Vec3{-5, 0, 0.5}, Vec3{5, 0, 0.5}, true},   // right past it
		{Vec3{-5, 0, 1.5}, Vec3{5, 0, 1.5}, false},  // beside it
		{Vec3{-5, 0, 0}, Vec3{-2, 0, 0}, false},     // short of it
		{Vec3{-5, 3, 0}, Vec3{5, 3, 0}, false},      // over it
		{Vec3{0.5, 0, 0}, Vec3{0.5, 0, 0}, true},    // a point on it
		{Vec3{-5, 0, -5}, Vec3{5, 0, 5}, true},      // diagonally through
		{Vec3{-5, 0, -3.5}, Vec3{5, 0, 6.5}, false}, // diagonally beside
	} {
		if got := nearPath(q, k.a, k.b, 1, 1.5); got != k.want {
			t.Errorf("%v -> %v: %v, want %v", k.a, k.b, got, k.want)
		}
	}
}

// A kill volume (Map.Pits): a body flung over a thin one falls in, though
// no command ends in it (each command's step is swept, not its end point).
func TestPitCountsThroughACommandBurst(t *testing.T) {
	for _, miss := range []bool{false, true} {
		s, v, _, now := ticketServer(t)
		z0, z1 := -2.0, 2.0
		if miss {
			z0, z1 = 3, 6
		}
		s.Map.Pits = []c.Zone{{Name: "merge_hell", Min: [3]float64{-58.1, 0, z0}, Max: [3]float64{-58, 2, z1}}}
		fling(v, Vec3{-55, 0.8, 0}, Vec3{-30, 0, 0})
		burst(s, v, 40, Cmd{}, now)
		if v.Dead == miss {
			t.Fatalf("miss %v: dead %v at x %.2f", miss, v.Dead, v.Pos[0])
		}
		if !miss {
			continue
		}
		if v.Pos[0] > -62 {
			t.Fatalf("the burst left them at x %.2f; want past the pit (x < -62)", v.Pos[0])
		}
	}
}
