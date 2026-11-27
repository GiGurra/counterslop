package game

import (
	"bytes"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// raceTo2: v past the first two gates (the line, then west): CP 2.
func raceTo2(t *testing.T, s *Server, v *Player, now *time.Time) {
	t.Helper()
	putAt(s, v, [3]float64{0, 0, 12})
	ticks(s, 1, now, nil)
	putAt(s, v, [3]float64{-50, 0, 12})
	ticks(s, 1, now, nil)
	if s.racer(v).CP != 2 {
		t.Fatalf("CP %d, want 2", s.racer(v).CP)
	}
}

// The garage mid-race is a pit stop: the car parked out of the world (not
// sent, not driven), the driver held (no corpse), the race clock running,
// the shop open; leaving puts them on the last gate passed in the car they
// picked in there, repaired and refilled, their guns kept.
func TestPitStopParksTheCarAndKeepsTheClock(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	goRace(t, s, now)
	raceTo2(t, s, v, now)
	started := s.round.started
	c := s.carByID(v.car)
	c.sim.HP, c.cs.fn = 30, 0
	inv := v.inv
	v.Money = 16000
	if why := s.carPurchase(v, "car:racecar"); why == "" {
		t.Fatal("bought a car on the track (not in the pit)")
	}

	s.pitMsg(v, true, 1)
	rs := s.racer(v)
	if !v.pit || !v.Dead || !v.Vanish || v.car != 0 || !c.parked || !rs.Pit || c.sim.Pos != pitPark {
		t.Fatalf("in the pit: pit %v dead %v vanish %v car %d parked %v racer %+v car at %v", v.pit, v.Dead, v.Vanish, v.car, c.parked, *rs, c.sim.Pos)
	}
	if bytes.Contains(s.carsJSON(), []byte(`"id":`+strconv.Itoa(c.ID)+`,`)) {
		t.Fatalf("the parked car is still sent: %s", s.carsJSON())
	}
	if m := s.meStateOf(v); !m.Pit || m.PitSeq != 1 || !m.Buy {
		t.Fatalf("me: pit %v ps %d buy %v", m.Pit, m.PitSeq, m.Buy)
	}
	ticks(s, 4*SimHz, now, nil) // longer than a wreck's countdown: still in
	rs = s.racer(v)
	if !v.pit || rs.Wreck != 0 || rs.CP != 2 || s.round.started != started || c.sim.Pos != pitPark || v.Deaths != 0 {
		t.Fatalf("4 s in: pit %v racer %+v, deaths %d, car at %v", v.pit, *rs, v.Deaths, c.sim.Pos)
	}
	if why := s.carPurchase(v, "car:racecar"); why != "" {
		t.Fatalf("the racecar in the pit: %s", why)
	}
	if why := s.carPurchase(v, "up:plating"); why != "" {
		t.Fatalf("plating in the pit: %s", why)
	}
	if c.Type != "buggy" && c.sim.Pos != pitPark {
		t.Fatalf("a buy moved the parked car: %s at %v", c.Type, c.sim.Pos)
	}

	s.pitMsg(v, false, 2)
	c = s.carByID(s.racer(v).Car)
	g := s.Map.Checkpoints[1]
	if v.pit || v.Dead || v.car != c.ID || c.parked || c.Type != "racecar" || c.sim.Pos != sim.Vec3(g.Spawn) || c.sim.Yaw != g.Yaw || c.HP != c.MaxHP || c.cs.fn == 0 {
		t.Fatalf("out: pit %v dead %v car %d/%d parked %v %s at %v yaw %v hp %d/%d fn %d; want at %v", v.pit, v.Dead, v.car, c.ID, c.parked, c.Type, c.sim.Pos, c.sim.Yaw, c.HP, c.MaxHP, c.cs.fn, g.Spawn)
	}
	if v.inv != inv || s.racer(v).CP != 2 || s.racer(v).Pit || s.round.started != started {
		t.Fatalf("out: inventory kept %v, racer %+v", v.inv == inv, *s.racer(v))
	}
	ticks(s, 2, now, map[*Player]Cmd{v: {F: 1}})
	if v.car != c.ID || c.Driver != v.ID {
		t.Fatalf("not driving after the pit: car %d driver %d", v.car, c.Driver)
	}
}

// {t:"pit"} is level-triggered and resend-safe: a repeat or an older seq
// (reordered) changes nothing; a newer one does.
func TestPitStopResendSafe(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	goRace(t, s, now)
	s.pitMsg(v, true, 5)
	s.pitMsg(v, true, 5) // duplicated
	s.pitMsg(v, true, 4) // an older one
	if !v.pit || v.pitSeq != 5 {
		t.Fatalf("in: pit %v seq %d", v.pit, v.pitSeq)
	}
	ticks(s, 3, now, nil)
	s.pitMsg(v, false, 6)
	car := v.car
	s.pitMsg(v, true, 5) // the old "in" arriving late
	s.pitMsg(v, false, 6)
	ticks(s, 3, now, nil)
	if v.pit || v.Dead || v.car == 0 || v.car != car || v.pitSeq != 6 {
		t.Fatalf("out: pit %v dead %v car %d (was %d) seq %d", v.pit, v.Dead, v.car, car, v.pitSeq)
	}
	if m := s.meStateOf(v); m.Pit || m.PitSeq != 6 {
		t.Fatalf("me: pit %v ps %d", m.Pit, m.PitSeq)
	}
	// Before GO the wish is kept but does nothing; GO drops it (the client
	// closes the garage at GO).
	s2, v2, _, now2 := raceServer(t, "2")
	s2.pitMsg(v2, true, 1)
	if v2.pit || v2.Dead {
		t.Fatal("in the pit on the grid")
	}
	goRace(t, s2, now2)
	if v2.pit || v2.Dead || v2.pitWant || v2.car == 0 {
		t.Fatalf("GO with the garage open: pit %v dead %v want %v car %d", v2.pit, v2.Dead, v2.pitWant, v2.car)
	}
}

// Wrecked, then into the garage: out of it, the wreck's countdown still
// holds (no skipping the respawn wait); after it, back at once.
func TestPitStopAfterAWreck(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	goRace(t, s, now)
	raceTo2(t, s, v, now)
	s.killBy(nil, v, "wreck", false)
	ticks(s, 1, now, nil)
	s.pitMsg(v, true, 1)
	if !v.pit {
		t.Fatal("wrecked: no pit")
	}
	s.pitMsg(v, false, 2)
	if !v.Dead || v.pit {
		t.Fatalf("out at once after a wreck: dead %v pit %v", v.Dead, v.pit)
	}
	ticks(s, int(raceRespawnSecs*SimHz)+2, now, nil)
	if v.Dead || v.car == 0 {
		t.Fatalf("after the countdown: dead %v car %d", v.Dead, v.car)
	}
	// A long stay: out of it straight onto the gate.
	s.killBy(nil, v, "wreck", false)
	ticks(s, 1, now, nil)
	s.pitMsg(v, true, 3)
	ticks(s, int(raceRespawnSecs*SimHz)+10, now, nil)
	if !v.Dead || !v.pit {
		t.Fatalf("in the pit past the countdown: dead %v pit %v", v.Dead, v.pit)
	}
	s.pitMsg(v, false, 4)
	if v.Dead || v.car == 0 || s.carByID(v.car).sim.Pos != sim.Vec3(s.Map.Checkpoints[1].Spawn) {
		t.Fatalf("out: dead %v car %d", v.Dead, v.car)
	}
}

// The race ends with a racer in the pit: out, on their gate, for the results.
func TestPitStopRaceEnds(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	goRace(t, s, now)
	s.pitMsg(v, true, 1)
	s.endRace(*now)
	if v.pit || v.Dead || v.car == 0 || s.carByID(v.car).parked {
		t.Fatalf("results: pit %v dead %v car %d", v.pit, v.Dead, v.car)
	}
}

// The garage and its shop open from anywhere on a race map: no buy zones,
// no distances: on foot or in a car, mid-air, far from the spawns and the
// grid. Only the phase decides: before GO it is the shop, mid-race the pit
// stop (a car racer) or the shop (on foot, not racing).
func TestGarageAnywhere(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	far := [3]float64{55, 30, -20} // the east end, 30 m up
	v.Money, a.Money = 16000, 16000

	// Warmup, on foot, far away and in the air.
	s.round.Phase = PhaseWarmup
	v.car = 0
	v.Pos = Vec3(far)
	v.body.Pos = sim.Vec3(far)
	if ok, why := s.canBuy(v); !ok {
		t.Fatalf("warmup on foot far away: %s", why)
	}
	if why := s.carPurchase(v, "up:engine"); why != "" {
		t.Fatalf("warmup buy far away: %s", why)
	}
	s.round.Phase = PhaseFreeze

	// The grid, in a car, put far away.
	putAt(s, a, far)
	if ok, why := s.canBuy(a); !ok || s.carPurchase(a, "up:tyres") != "" {
		t.Fatalf("on the grid in a car far away: %s", why)
	}

	// Mid-race in a car, far away in the air: the pit stop.
	goRace(t, s, now)
	putAt(s, a, far)
	s.pitMsg(a, true, 1)
	if !a.pit {
		t.Fatal("mid-race in a car far away: no pit")
	}
	if ok, why := s.canBuy(a); !ok {
		t.Fatalf("in the pit: %s", why)
	}

	// Mid-race on foot, not racing (no grid slot left), far away: the shop.
	s.cars = s.cars[:2]
	q := s.join("3.3.3.3")
	q.ready, q.Team = true, content.Vibe
	s.queueCmds(q, []Cmd{{Seq: 1}})
	s.respawn(q)
	q.Money = 16000
	ticks(s, 2, now, nil)
	if s.racer(q) != nil || q.car != 0 {
		t.Fatalf("q races: %+v car %d", s.racer(q), q.car)
	}
	q.Pos = Vec3(far)
	q.body.Pos = sim.Vec3(far)
	if ok, why := s.canBuy(q); !ok || s.carPurchase(q, "car:tank") != "" {
		t.Fatalf("mid-race on foot far away: %s", why)
	}
	s.pitMsg(q, true, 1)
	if q.pit || q.Dead {
		t.Fatal("a pit stop without a car in the race")
	}
}

// Cars bought this match stay yours: switching back to one is free; a new
// match starts stock with nothing owned.
func TestGarageOwnedCars(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	v.Money = 10000
	for _, step := range []struct {
		item string
		cost int
	}{{"car:racecar", CarSpecOf("racecar").Price}, {"car:buggy", 0}, {"car:racecar", 0}, {"car:tank", CarSpecOf("tank").Price}, {"car:racecar", 0}} {
		m := v.Money
		if why := s.carPurchase(v, step.item); why != "" {
			t.Fatalf("%s: %s", step.item, why)
		}
		if m-v.Money != step.cost {
			t.Fatalf("%s cost %d, want %d", step.item, m-v.Money, step.cost)
		}
	}
	g := s.meStateOf(v).Garage
	if g.Type != "racecar" || !slices.Equal(g.Owned, []string{"racecar", "tank"}) || s.carByID(v.car).Type != "racecar" {
		t.Fatalf("garage %+v", *g)
	}
	_ = now
	s.startMatch(*now)
	if v.garage != nil || s.garageOwns(v, "racecar") || !s.garageOwns(v, "buggy") || s.garageOwns(v, "nope") {
		t.Fatalf("a new match: garage %+v", v.garage)
	}
}

// mp_startmoney (CS) overrides economy.json's start money, capped at the max.
func TestStartMoneyCvar(t *testing.T) {
	s, _, _ := econServer(t)
	if s.startMoney() != s.econ().StartMoney {
		t.Fatalf("default %d", s.startMoney())
	}
	s.cvars["mp_startmoney"] = "9000"
	if s.startMoney() != 9000 {
		t.Fatalf("9000: %d", s.startMoney())
	}
	s.cvars["mp_startmoney"] = "999999"
	if s.startMoney() != s.econ().MaxMoney {
		t.Fatalf("capped: %d", s.startMoney())
	}
}

// Out of the pit some other way: a team switch (a respawn) or leaving the
// server: the car comes back into the world, never stays parked for good.
func TestPitStopCarNeverLost(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	goRace(t, s, now)
	s.pitMsg(v, true, 1)
	c := s.carByID(s.racer(v).Car)
	v.Team = content.Artisan
	s.respawn(v)
	if v.pit || c.parked || c.sim.Pos == pitPark || s.racer(v).Pit {
		t.Fatalf("respawned from the pit: pit %v parked %v at %v", v.pit, c.parked, c.sim.Pos)
	}
	s.pitMsg(a, true, 1)
	ca := s.carByID(s.racer(a).Car)
	delete(s.players, a.ID)
	ticks(s, 2, now, nil)
	if ca.parked || ca.sim.Pos == pitPark || !bytes.Contains(s.carsJSON(), []byte(`"id":`+strconv.Itoa(ca.ID)+`,`)) {
		t.Fatalf("the car of a racer who left: parked %v at %v", ca.parked, ca.sim.Pos)
	}
}
