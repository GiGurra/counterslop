package game

import (
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// upgradedRacer: v in a tank with plating and a swapped front, on the grid,
// then racing a while (its command clock well on) with the front fired once
// (its next shot due on v's clock).
func upgradedRacer(t *testing.T, s *Server, v *Player, now *time.Time) *Car {
	t.Helper()
	v.Money = 16000
	for _, it := range []string{"car:tank", "up:plating", "front:linter_beam"} {
		if why := s.carPurchase(v, it); why != "" {
			t.Fatalf("%s: %s", it, why)
		}
	}
	goRace(t, s, now)
	v.cmdTicks += 50000 // a long race: v's command clock far ahead of a newcomer's
	cticks(s, 1, now, map[*Player]Cmd{v: {B: BtnCarFront}})
	c := s.carByID(v.car)
	if c.Type != "tank" || c.cs.front != "linter_beam" || c.cs.fn != CarItemOf("linter_beam").Charges-1 {
		t.Fatalf("upgraded: %s front %s fn %d", c.Type, c.cs.front, c.cs.fn)
	}
	return c
}

// stockCar: c is its grid slot's stock car again: the map's type, its
// built-ins, stock HP, no upgrades, at its slot, owner's (0: nobody's).
func stockCar(t *testing.T, s *Server, c *Car, owner int) {
	t.Helper()
	g := s.Map.Cars[c.spawn]
	sp := CarSpecOf(g.Type)
	if c.Type != g.Type || c.spec != sp || c.cs.front != sp.Front || c.cs.rear != sp.Rear || c.MaxHP != sp.HP || c.cs.plating != 0 || c.cs.owner != owner || c.parked {
		t.Fatalf("car %d not stock: %s (%s) front %s rear %s max HP %d plating %d owner %d parked %v", c.ID, c.Type, g.Type, c.cs.front, c.cs.rear, c.MaxHP, c.cs.plating, c.cs.owner, c.parked)
	}
	if c.sim.Pos != sim.Vec3(g.Pos) {
		t.Fatalf("car %d at %v, want its slot %v", c.ID, c.sim.Pos, g.Pos)
	}
}

// A racer who leaves mid-race: their racer entry goes at once (no ghost in
// the standings) and their car is its grid slot's stock car again, never a
// half-owned upgraded one.
func TestRaceLeaveMidRace(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	c := upgradedRacer(t, s, v, now)
	s.leave(v)
	if s.racer(v) != nil || len(s.round.Racers) != 1 || s.round.Racers[0].ID != a.ID {
		t.Fatalf("racers after the leave: %+v", s.round.Racers)
	}
	stockCar(t, s, c, 0)
	ticks(s, 5, now, nil)
	stockCar(t, s, c, 0)
}

// A rejoin (a reload: a new player) mid-race races properly: a racer on a
// free slot at lap 0, the slot's stock car fitted from their own (empty)
// garage, its front item firing on their clock, the shop pricing against
// that car.
func TestRaceRejoinMidRace(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	upgradedRacer(t, s, v, now)
	s.leave(v)
	n := s.join("9.9.9.9")
	s.handle(n, inbound{T: "hello", Name: v.Name})
	s.handle(n, inbound{T: "team", Team: content.Vibe})
	s.queueCmds(n, []Cmd{{Seq: 1}})
	n.respawnAt = *now
	ticks(s, 5, now, nil)
	rs := s.racer(n)
	if n.Dead || rs == nil || n.car == 0 || rs.Car != n.car || rs.CP != 0 {
		t.Fatalf("rejoiner: dead %v car %d racer %+v", n.Dead, n.car, rs)
	}
	c := s.carByID(n.car)
	stockAt := c.sim.Pos
	c.sim.Pos = sim.Vec3(s.Map.Cars[c.spawn].Pos) // stockCar checks the place too
	stockCar(t, s, c, n.ID)
	c.sim.Pos = stockAt
	fn := c.cs.fn
	cticks(s, 1, now, map[*Player]Cmd{n: {B: BtnCarFront}})
	if c.cs.fn != fn-1 {
		t.Fatalf("the rejoiner's front item didn't fire: %d -> %d charges (next at %d, clock %d)", fn, c.cs.fn, c.cs.nextFront, n.cmdTicks)
	}
	if sp := s.garageCarSpec(n); sp.ID != c.Type {
		t.Fatalf("the shop prices a %s, the car is a %s", sp.ID, c.Type)
	}
	n.Money = CarSpecOf("tank").Price
	s.pitMsg(n, true, 1)
	if !n.pit {
		t.Fatal("the rejoiner can't pit")
	}
	if why := s.carPurchase(n, "car:tank"); why != "" || n.Money != 0 {
		t.Fatalf("buying the tank in the pit: %q, money %d", why, n.Money)
	}
}

// A racer who leaves from the pit stop: the parked car is back on its slot,
// stock, in the world.
func TestRaceLeaveFromThePit(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	c := upgradedRacer(t, s, v, now)
	s.pitMsg(v, true, 1)
	if !c.parked {
		t.Fatal("not parked")
	}
	s.leave(v)
	stockCar(t, s, c, 0)
	ticks(s, 2, now, nil)
	stockCar(t, s, c, 0)
}

// A bot kicked mid-race goes the same way: no racer, its car stock.
func TestRaceBotKickedMidRace(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	c := upgradedRacer(t, s, v, now)
	v.ai = &aiBrain{} // kickAI takes playing bots
	if s.kickAI("", -1) != 1 {
		t.Fatal("no bot kicked")
	}
	if s.racer(v) != nil {
		t.Fatalf("racers: %+v", s.round.Racers)
	}
	stockCar(t, s, c, 0)
}
