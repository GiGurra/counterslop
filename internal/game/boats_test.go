package game

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/sim"
)

func harborServer(t *testing.T) *Server {
	t.Helper()
	s := authServer(t)
	s.Test = true
	if err := s.ResetWorldOn("lab_harbor"); err != nil {
		t.Fatal(err)
	}
	return s
}

// The map's boats start at their moorings, a pushed boat drifts and comes
// to rest against the ship's hull, and a new round puts it back.
func TestBoatsDriftRestAndReset(t *testing.T) {
	s := harborServer(t)
	if len(s.boats) != 2 || s.boats[0].Pos != (Vec3{-12, -0.6, 8}) {
		t.Fatalf("boats %+v", s.boats)
	}
	s.mu.Lock()
	s.boats[0].sim.Pos, s.boats[0].sim.Vel = sim.Vec3{-12, -0.6, -24}, sim.Vec3{0, 0, -9}
	s.mu.Unlock()
	s.Step(300)
	b := s.boats[0]
	if want := -30 + sim.BoatDisc + sim.BoatDiscOff; math.Abs(b.sim.Pos[2]-want) > 1e-6 || b.sim.Vel[2] != 0 {
		t.Fatalf("against the hull: %+v, want z %v", b.sim, want)
	}
	if b.Pos != Vec3(b.sim.Pos) {
		t.Fatalf("sent %v, simulated %v", b.Pos, b.sim.Pos)
	}
	s.mu.Lock()
	s.startRound(s.clock())
	s.mu.Unlock()
	if s.boats[0].Pos != (Vec3{-12, -0.6, 8}) || s.boats[0].Vel != (Vec3{}) {
		t.Fatalf("after the round start: %+v", s.boats[0])
	}
}

// Every full snapshot carries the boats (id, position, heading, velocity);
// a map without boats sends none.
func TestSnapshotCarriesBoats(t *testing.T) {
	s := harborServer(t)
	var m struct {
		Boats []struct {
			ID int
			P  [3]float64
			Y  float64
			V  [3]float64
		}
	}
	s.mu.Lock()
	b := s.boatsJSON()
	s.mu.Unlock()
	if err := json.Unmarshal(append(append([]byte("{"), b...), '}'), &m); err != nil || len(m.Boats) != 2 || m.Boats[1].ID != 2 || m.Boats[1].P != [3]float64{12, -0.6, 8} {
		t.Fatalf("%s: %+v %v", b, m, err)
	}
	if err := s.ResetWorldOn("de_slop"); err != nil {
		t.Fatal(err)
	}
	if s.boatsJSON() != nil {
		t.Fatal("boats on de_slop")
	}
}

// The MCP debugger's boat_set moves a boat and lists them all.
func TestMCPBoatSet(t *testing.T) {
	s := harborServer(t)
	c := mcpFor(t, s, true)
	r := c.call("boat_set", map[string]any{"id": 2, "pos": []float64{5, -0.6, -10}, "yaw": 1, "vel": []float64{1, 0, 2}})
	bs := r["boats"].([]any)
	if len(bs) != 2 || bs[1].(map[string]any)["yaw"].(float64) != 1 || s.boats[1].Pos != (Vec3{5, -0.6, -10}) {
		t.Fatalf("boat_set %v", r)
	}
	c.call("boat_set", map[string]any{"id": 3}, true)
	c.call("boat_set", map[string]any{"id": 1, "pos": []float64{1, 2}}, true)
}

// The seats in the world: the driver's aft, the passengers' forward of it,
// turning with the boat.
func TestSeatWorld(t *testing.T) {
	b := &sim.Boat{Pos: sim.Vec3{10, -0.6, 5}, Yaw: 0} // bow to -z
	if got := SeatWorld(b, 0); math.Abs(got[0]-10) > 1e-12 || math.Abs(got[1]+0.5) > 1e-12 || math.Abs(got[2]-6.15) > 1e-12 {
		t.Fatalf("driver at %v", got)
	}
	if got := SeatWorld(b, 2); math.Abs(got[2]-4.25) > 1e-12 {
		t.Fatalf("bow seat at %v", got)
	}
	b.Yaw = -math.Pi / 2 // bow to +x: aft is -x
	if got := SeatWorld(b, 0); math.Abs(got[0]-8.85) > 1e-12 || math.Abs(got[2]-5) > 1e-12 {
		t.Fatalf("turned: driver at %v", got)
	}
}

// RideStep: the driver's command steers the boat and seats them; a
// passenger's leaves the boat alone. RideLeave: out at the seat with the
// boat's velocity, up at jump speed on a hop.
func TestRideStepAndLeave(t *testing.T) {
	m := s0Harbor()
	w := sim.NewWorld(&m)
	b := &sim.Boat{Pos: sim.Vec3{0, -0.6, -15}}
	d, p := sim.NewBody(sim.Vec3{}), sim.NewBody(sim.Vec3{})
	for range 100 {
		RideStep(w, b, d, 1, 0, 0, 0.01)
		RideStep(w, b, p, 1, 1, 1, 0.01) // ignored: not the driver
	}
	if b.Vel[2] > -5 || b.Yaw != 0 || d.Pos != SeatWorld(b, 0) || !d.Crouched || d.H != sim.HCrouch || !d.OnGround {
		t.Fatalf("boat %+v driver %+v", b, d)
	}
	RideStep(w, b, p, 0, 0, 1, 0.01)
	if p.Pos != SeatWorld(b, 1) || p.Vel != b.Vel {
		t.Fatalf("passenger %+v", p)
	}
	y := p.Pos[1]
	RideLeave(b, p, true)
	if p.Vel[1] != sim.JumpSpeed || p.Vel[2] != b.Vel[2] || p.OnGround || p.Pos[1] != y+0.05 {
		t.Fatalf("hopped out: %+v", p)
	}
	if !RideLeaves(BtnUse, 0, false) || !RideLeaves(BtnJump|BtnFire, BtnFire, false) || RideLeaves(BtnUse, BtnUse, false) || RideLeaves(BtnFire, 0, false) ||
		RideLeaves(BtnHook, 0, false) || !RideLeaves(BtnHook, 0, true) {
		t.Fatal("RideLeaves")
	}
}

func s0Harbor() content.Map { return maps.LabHarbor() }

// Boarding: E within reach of a boat takes its driver's seat, the next one
// a passenger's, a fourth finds it full; too far boards nothing. Death, a
// respawn and a new round put riders out; the E press that got you out
// boards nothing.
func TestBoardingRules(t *testing.T) {
	s := harborServer(t)
	var ps []*Player
	for i := range 4 {
		p := s.join(fmt.Sprintf("10.0.9.%d", i))
		p.ready, p.Team = true, []string{content.Vibe, content.Artisan}[i%2]
		s.teleport(p, Vec3{-12.3 + 0.2*float64(i), 0, 10.5}, 0, false, 0)
		ps = append(ps, p)
	}
	far := s.join("10.0.9.9")
	far.ready, far.Team = true, content.Vibe
	s.teleport(far, Vec3{0, 0, 20}, 0, false, 0)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range ps {
		ok := s.boardBoat(p)
		if want := i < BoatSeats; ok != want || (ok && (p.ride != 1 || p.seat != i)) {
			t.Fatalf("player %d: boarded %v ride %d seat %d", i, ok, p.ride, p.seat)
		}
	}
	if s.boardBoat(far) {
		t.Fatal("boarded from 12 m away")
	}
	if m := s.meStateOf(ps[0]); m.Ride != 1 || m.Seat != 0 || m.Boat == nil || m.Boat.Pos != [3]float64(s.boats[0].sim.Pos) {
		t.Fatalf("self-snapshot %+v", m)
	}
	s.killBy(ps[3], ps[0], "prompt", false)
	if ps[0].ride != 0 || s.seatTaken(1, 0) == true {
		t.Fatal("the dead still drives")
	}
	if !s.boardBoat(ps[3]) || ps[3].seat != 0 {
		t.Fatalf("the freed driver's seat: %d", ps[3].seat)
	}
	// Up with E: standing on the deck where it sat; that press sits it
	// nowhere again.
	ps[1].cmdTicks = 50
	s.runCmd(s.simWorld(), ps[1], Cmd{Seq: 1, B: BtnUse})
	if x, _, z := SeatLocal(1); ps[1].ride != 1 || ps[1].seat != SeatDeck || ps[1].deck.Pos != (sim.Vec3{x, sim.DeckTop, z}) {
		t.Fatalf("E didn't stand it up: ride %d seat %d deck %v", ps[1].ride, ps[1].seat, ps[1].deck)
	}
	if m := s.meStateOf(ps[1]); m.Deck == nil || m.Deck.Pos != [3]float64(ps[1].deck.Pos) || m.Seat != SeatDeck || m.Boat == nil || !m.Boat.Driven {
		t.Fatalf("self-snapshot on the deck %+v", m)
	}
	s.startRound(s.clock())
	for _, p := range ps {
		if p.ride != 0 {
			t.Fatalf("still riding after the round start: %+v", p.Name)
		}
	}
}

// A body dropped onto a boat lands on its floor and stands there (on the
// deck: ride is the boat, seat SeatDeck), instead of falling through it into
// the water.
func TestDropOntoBoat(t *testing.T) {
	s := harborServer(t)
	p := s.join("10.0.9.1")
	p.ready, p.Team = true, content.Vibe
	s.teleport(p, Vec3{-12, 1.5, 8.3}, 0, false, 0) // over boat 1 (moored at -12, -0.6, 8)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range 80 {
		s.runCmd(s.simWorld(), p, Cmd{Seq: i + 1})
		s.tickBoats(1.0 / SimHz)
	}
	if p.ride != 1 || p.seat != SeatDeck {
		t.Fatalf("not on the boat: ride %d seat %d at %v", p.ride, p.seat, p.body.Pos)
	}
	if want := -0.6 + sim.DeckTop; math.Abs(p.body.Pos[1]-want) > 1e-9 || !p.body.OnGround || p.body.Crouched {
		t.Fatalf("standing at %v (want feet at %v), on ground %v", p.body.Pos, want, p.body.OnGround)
	}
}

// Seats: from the deck, E (or +seat) sits you at the free tiller, else in the
// free passenger seat nearest to where you stand; +seat moves a passenger to
// the free tiller and the driver to the free passenger seat nearest the
// stern; nothing free, nothing happens.
func TestSeatSwitching(t *testing.T) {
	s := harborServer(t)
	var ps []*Player
	for i := range 4 {
		p := s.join(fmt.Sprintf("10.0.8.%d", i))
		p.ready, p.Team = true, content.Vibe
		s.teleport(p, Vec3{-12, 0, 10.5}, 0, false, 0)
		ps = append(ps, p)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.boats[0]
	stand := func(p *Player, z float64) {
		p.ride, p.seat, p.deck = b.ID, SeatDeck, sim.Deck{Pos: sim.Vec3{0, sim.DeckTop, z}}
		DeckCarry(&b.sim, p.body, &p.deck)
	}
	stand(ps[0], -1.5) // at the bow
	if !s.boardBoat(ps[0]) || ps[0].seat != 0 {
		t.Fatalf("the free tiller first: seat %d", ps[0].seat)
	}
	stand(ps[1], -1.5)
	if !s.boardBoat(ps[1]) || ps[1].seat != 2 {
		t.Fatalf("at the bow, the bow seat: %d", ps[1].seat)
	}
	stand(ps[2], 1.5)
	if !s.boardBoat(ps[2]) || ps[2].seat != 1 {
		t.Fatalf("the last free seat: %d", ps[2].seat)
	}
	stand(ps[3], 0)
	if s.boardBoat(ps[3]) || ps[3].seat != SeatDeck {
		t.Fatal("sat down in a full boat")
	}
	if s.switchSeat(ps[1], b) || s.switchSeat(ps[0], b) {
		t.Fatal("switched in a full boat")
	}
	ps[2].ride, ps[2].seat = 0, 0 // seat 1 free
	if !s.switchSeat(ps[0], b) || ps[0].seat != 1 || ps[0].body.Pos != SeatWorld(&b.sim, 1) || !ps[0].body.Crouched {
		t.Fatalf("the driver to the free passenger seat: %d at %v", ps[0].seat, ps[0].body.Pos)
	}
	if !s.switchSeat(ps[1], b) || ps[1].seat != 0 {
		t.Fatalf("a passenger to the free tiller: %d", ps[1].seat)
	}
	if s.switchSeat(ps[0], b) || ps[0].seat != 1 {
		t.Fatal("to a taken tiller")
	}
	// +seat in a command: the same rules, and on the deck it sits you down.
	ps[3].cmdTicks = 10
	s.runCmd(s.simWorld(), ps[3], Cmd{Seq: 1, B: BtnSeat})
	if ps[3].seat != 2 {
		t.Fatalf("+seat on the deck: seat %d", ps[3].seat)
	}
}

// Getting up: E or a jump from a seat stands you on the deck where you sat
// (a jump hops up); the hook (boots) pulls you out into the map; the hook
// from the deck takes you off it too, E and a jump there don't.
func TestRideGetUp(t *testing.T) {
	b := &sim.Boat{Pos: sim.Vec3{0, -0.6, -15}, Vel: sim.Vec3{0, 0, -4}, Yaw: 0.5}
	for _, c := range []struct {
		name       string
		btn        int
		boots      bool
		deck, hop  bool
		wantLeaves bool
	}{
		{"E", BtnUse, false, true, false, true},
		{"jump", BtnJump, false, true, true, true},
		{"hook", BtnHook, true, false, false, true},
	} {
		s := sim.NewBody(SeatWorld(b, 1))
		var d sim.Deck
		on := RideGetUp(b, s, 1, c.btn, 0, c.boots, &d)
		if on != c.deck {
			t.Fatalf("%s: on the deck %v", c.name, on)
		}
		x, _, z := SeatLocal(1)
		if on && (d.Pos != sim.Vec3{x, sim.DeckTop, z} || (d.Vel[1] == sim.JumpSpeed) != c.hop || s.Pos != sim.DeckWorld(b, d.Pos) || s.Vel != sim.DeckVel(b, d.Vel)) {
			t.Fatalf("%s: deck %+v body %v", c.name, d, s.Pos)
		}
		if !on && s.Vel != b.Vel {
			t.Fatalf("%s: out with %v", c.name, s.Vel)
		}
	}
	if !DeckLeaves(BtnHook, 0, true) || DeckLeaves(BtnHook, 0, false) || DeckLeaves(BtnUse|BtnJump, 0, true) || DeckLeaves(BtnHook, BtnHook, true) {
		t.Fatal("DeckLeaves")
	}
}

// A body standing on a boat nobody drives is carried as it coasts: after
// each tick it is where it stands on the boat, moving with it, and sent
// there (p.Pos: what the others' shots and the bots see).
func TestDeckCarriedByTheBoat(t *testing.T) {
	s := harborServer(t)
	p := s.join("10.0.8.9")
	p.ready, p.Team = true, content.Vibe
	s.teleport(p, Vec3{-12, 0, 10.5}, 0, false, 0)
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.boats[0]
	b.sim.Vel = sim.Vec3{0, 0, -6}
	b.start = b.sim
	p.ride, p.seat, p.deck = b.ID, SeatDeck, sim.Deck{Pos: sim.Vec3{0.3, sim.DeckTop, 0}}
	z0 := b.sim.Pos[2]
	for i := range 50 {
		s.runCmd(s.simWorld(), p, Cmd{Seq: i + 1})
		s.tickBoats(1.0 / SimHz)
	}
	if b.sim.Pos[2] > z0-2 {
		t.Fatalf("the boat didn't coast: %v", b.sim.Pos)
	}
	if p.ride != b.ID || p.deck.Pos != (sim.Vec3{0.3, sim.DeckTop, 0}) || p.body.Pos != sim.DeckWorld(&b.sim, p.deck.Pos) || Vec3(p.body.Pos) != p.Pos || p.body.Vel != b.sim.Vel {
		t.Fatalf("not carried: deck %+v body %v vel %v, boat %v", p.deck, p.body.Pos, p.body.Vel, b.sim.Pos)
	}
}
