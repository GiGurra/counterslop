package game

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/sim"
)

// cticks runs n whole server ticks (advance: commands, cars, gear, rams,
// the round), p's command each tick.
func cticks(s *Server, n int, now *time.Time, cmds map[*Player]Cmd) {
	for range n {
		for p, c := range cmds {
			c.Seq = p.lastQueued + 1
			s.queueCmds(p, []Cmd{c})
		}
		*now = now.Add(time.Second / SimHz)
		s.advance(*now)
	}
}

// carAt puts p's car at pos facing yaw, still.
func carAt(s *Server, p *Player, pos sim.Vec3, yaw float64) *Car {
	c := s.carByID(p.car)
	hp, eng, ty := c.sim.HP, c.sim.Engine, c.sim.Tyres
	c.sim = *sim.NewCar(c.spec, pos, yaw)
	c.sim.HP, c.sim.Engine, c.sim.Tyres = hp, eng, ty
	CarSeat(c.spec, &c.sim, p.body, 0.01)
	p.Pos = Vec3(p.body.Pos)
	c.sync()
	return c
}

const west = math.Pi / 2 // a car's yaw facing -x

// The grid: every racer's car has its type's built-ins with a full lap of
// charges and a full boost tank, in the driver's self-snapshot.
func TestCarBuiltIns(t *testing.T) {
	s, v, _, _ := raceServer(t, "2")
	c := s.carByID(v.car)
	ce := s.carExactOf(v)
	beam, spikes := CarItemOf("linter_beam"), CarItemOf("breaking_changes")
	if c.Type != "buggy" || ce.Front != beam.ID || ce.Rear != spikes.ID || ce.FN != beam.Charges || ce.RN != spikes.Charges || ce.Boost != CarShop.Boost || ce.MaxHP != c.spec.HP {
		t.Fatalf("the buggy's built-ins: %+v", *ce)
	}
	for _, sp := range CarTypes {
		if CarItemOf(sp.Front) == nil || CarItemOf(sp.Front).Slot != "front" || CarItemOf(sp.Rear) == nil || CarItemOf(sp.Rear).Slot != "rear" {
			t.Fatalf("%s: built-ins %q / %q", sp.ID, sp.Front, sp.Rear)
		}
	}
}

// The beam (the buggy's front, Q held): hitscan along the car's heading, one
// a beam interval, a charge each, into the car ahead's HP. Not before GO.
func TestCarBeamHitsTheCarAhead(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	cticks(s, 50, now, map[*Player]Cmd{v: {B: BtnCarFront}})
	if c := s.carByID(v.car); c.cs.fn != CarItemOf("linter_beam").Charges {
		t.Fatalf("fired on the grid: %d charges", c.cs.fn)
	}
	goRace(t, s, now)
	carAt(s, v, sim.Vec3{20, 0, 12}, west)
	bob := carAt(s, a, sim.Vec3{0, 0, 12}, west)
	cticks(s, 50, now, map[*Player]Cmd{v: {B: BtnCarFront}}) // 0.5 s: 5 beams (0.1 s apart)
	it := CarItemOf("linter_beam")
	fired := it.Charges - s.carByID(v.car).cs.fn
	if fired < 4 || fired > 6 {
		t.Fatalf("%d beams in 0.5 s", fired)
	}
	if want := bob.MaxHP - fired*int(it.Damage); bob.sim.HP != want || a.HP != 100 {
		t.Fatalf("bob's car %d HP, want %d; bob %d HP", bob.sim.HP, want, a.HP)
	}
}

// Rockets fly along the heading and burst on the car ahead: its HP by the
// blast (the gear in the snapshot while it flies). A racer's garage picks
// the car: the racecar has them built in.
func TestCarRocketBlast(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	v.garage = &garage{Type: "racecar"}
	s.refitOnGrid(v)
	if c := s.carByID(v.car); c.Type != "racecar" || c.cs.front != "hotfix_missiles" {
		t.Fatalf("refitted: %s with %s", c.Type, c.cs.front)
	}
	goRace(t, s, now)
	carAt(s, v, sim.Vec3{30, 0, 12}, west)
	bob := carAt(s, a, sim.Vec3{0, 0, 12}, west)
	cticks(s, 1, now, map[*Player]Cmd{v: {B: BtnCarFront}})
	if len(s.gear) != 1 || s.gear[0].Item != "hotfix_missiles" || len(s.gearJSON()) == 0 {
		t.Fatalf("gear %+v", s.gear)
	}
	cticks(s, 60, now, nil)
	it := CarItemOf("hotfix_missiles")
	if len(s.gear) != 0 || bob.sim.HP >= bob.MaxHP || bob.sim.HP < bob.MaxHP-int(it.Damage) {
		t.Fatalf("after the rocket: gear %d, bob's car %d/%d HP", len(s.gear), bob.sim.HP, bob.MaxHP)
	}
}

// Orbs home in on the nearest car ahead: one fired at a car off to the side
// turns onto it.
func TestCarOrbHomes(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	v.garage = &garage{Type: "hovercraft"}
	s.refitOnGrid(v)
	goRace(t, s, now)
	carAt(s, v, sim.Vec3{30, 0, -8}, west)
	bob := carAt(s, a, sim.Vec3{5, 0, -18}, west) // 25 m ahead, 10 m to the side (the south half: no parked cars)
	cticks(s, 1, now, map[*Player]Cmd{v: {B: BtnCarFront}})
	cticks(s, 200, now, nil)
	if bob.sim.HP >= bob.MaxHP {
		t.Fatalf("the orb missed: bob's car %d/%d, gear %+v", bob.sim.HP, bob.MaxHP, s.gear)
	}
}

// Spikes (the buggy's rear, C) dropped behind: armed a moment later; the car
// driving over them slips (sim.Car.Slip); a hovercraft floats over them.
// Oil spins a car out.
func TestCarSpikesAndOil(t *testing.T) {
	for _, tc := range []struct {
		rear, bobCar string
		slip         bool
	}{{"breaking_changes", "buggy", true}, {"spaghetti_code", "buggy", true}, {"breaking_changes", "hovercraft", false}} {
		s, v, a, now := raceServer(t, "2")
		v.garage = &garage{Rear: tc.rear}
		a.garage = &garage{Type: tc.bobCar}
		s.refitOnGrid(v)
		s.refitOnGrid(a)
		goRace(t, s, now)
		carAt(s, v, sim.Vec3{0, 0, 12}, west)
		bob := carAt(s, a, sim.Vec3{20, 0, 12}, west)
		cticks(s, 1, now, map[*Player]Cmd{v: {B: BtnCarRear}})
		cticks(s, 1, now, nil)
		cticks(s, 1, now, map[*Player]Cmd{v: {B: BtnCarRear}}) // a second press: its interval
		if v := s.carByID(v.car); len(s.gear) != 1 || v.cs.rn != CarItemOf(tc.rear).Charges-1 {
			t.Fatalf("%s: gear %d, %d charges", tc.rear, len(s.gear), v.cs.rn)
		}
		cticks(s, 60, now, nil) // armed
		slipped := false
		yaw0 := bob.sim.Yaw
		for range 150 {
			cticks(s, 1, now, map[*Player]Cmd{a: {F: 1}})
			slipped = slipped || bob.sim.Slip > 0
		}
		if slipped != tc.slip {
			t.Fatalf("%s under a %s: slipped %v", tc.rear, tc.bobCar, slipped)
		}
		if tc.rear == "spaghetti_code" && math.Abs(math.Remainder(bob.sim.Yaw-yaw0, 2*math.Pi)) < 0.3 {
			t.Fatalf("oil: no spin-out (yaw %v -> %v)", yaw0, bob.sim.Yaw)
		}
	}
}

// A mine (the tank's rear) blows up under the next car: its HP, knocked up;
// a sticky one creeps to a car near it.
func TestCarMines(t *testing.T) {
	for _, rear := range []string{"tech_debt", "sticky_tech_debt"} {
		s, v, a, now := raceServer(t, "2")
		v.garage = &garage{Rear: rear}
		s.refitOnGrid(v)
		goRace(t, s, now)
		carAt(s, v, sim.Vec3{0, 0, 12}, west)
		cticks(s, 1, now, map[*Player]Cmd{v: {B: BtnCarRear}})
		at := s.gear[0].Pos
		dx, dz, f := 15.0, 0.0, 0.5 // bob drives over it
		if rear == "sticky_tech_debt" {
			dx, dz, f = 5, 4, 0 // bob parked beside it, within its seek range: it creeps over
		}
		bob := carAt(s, a, sim.Vec3{at[0] + dx, 0, at[2] + dz}, west)
		cticks(s, 60, now, nil)
		up := false
		for range 200 {
			cticks(s, 1, now, map[*Player]Cmd{a: {F: f}})
			up = up || bob.sim.Vel[1] > 1
			if len(s.gear) == 0 {
				break
			}
		}
		it := CarItemOf(rear)
		if len(s.gear) != 0 || bob.sim.HP > bob.MaxHP-int(it.Damage*0.3) || !up {
			t.Fatalf("%s: gear %d, bob's car %d/%d, knocked up %v", rear, len(s.gear), bob.sim.HP, bob.MaxHP, up)
		}
	}
}

// Bullets: the car's body stops them and takes carBulletK of the damage; the
// driver's head over it is hit as anyone's.
func TestBulletsOnACar(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	goRace(t, s, now)
	carAt(s, a, sim.Vec3{0, 0, 12}, west)
	c := s.carByID(a.car)
	v.car = 0 // alice on foot, 10 m south of bob's car
	v.body = sim.NewBody(sim.Vec3{0, 0, 22})
	v.Pos = Vec3(v.body.Pos)
	ak := s.Weapons.Get("prompt")
	o := Vec3(sim.EyePos(v.body))
	toward := func(y float64) [2]float64 { // yaw, pitch at (0, y, 12)
		dx, dy, dz := 0-o[0], y-o[1], 12-o[2]
		return [2]float64{math.Atan2(-dx, -dz), math.Atan2(dy, math.Hypot(dx, dz))}
	}
	s.ray(v, ak, Cmd{}, o, toward(0.5), nil, false, nil) // the body's side
	if c.sim.HP != c.MaxHP-max(1, int(math.Round(ak.Damage*carBulletK))) || a.HP != 100 {
		t.Fatalf("a shot into the car's side: car %d/%d, bob %d", c.sim.HP, c.MaxHP, a.HP)
	}
	head := a.body.Pos[1] + a.body.EyeH
	s.ray(v, ak, Cmd{}, o, toward(head), nil, false, nil)
	if a.HP >= 100 {
		t.Fatalf("a shot at bob's head over the car: bob %d HP", a.HP)
	}
}

// At 0 HP the car is wrecked and its driver dies, killed by whoever did it;
// out of a race the wreck goes home repaired after carWreckSecs.
func TestCarWreck(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	goRace(t, s, now)
	c := s.carByID(a.car)
	s.damageCar(c, 1000, v, "linter_beam")
	if !a.Dead || c.sim.HP != 0 || v.Kills != 1 || c.cs.wreckAt.IsZero() {
		t.Fatalf("wrecked: bob dead %v, car HP %d, alice's kills %d", a.Dead, c.sim.HP, v.Kills)
	}
	m := maps.LabKart()
	s2 := authServer(t)
	s2.Map = m
	s2.resetCars()
	k := s2.cars[0]
	at := k.sim.Pos
	k.sim.Pos[0] += 7
	s2.damageCar(k, 1000, nil, "ram")
	if k.sim.HP != 0 || s2.boardCar(&Player{body: sim.NewBody(k.sim.Pos)}) {
		t.Fatal("a wreck: HP left, or boarded")
	}
	t2 := time.Now()
	s2.tickWrecks(t2)
	s2.tickWrecks(t2.Add(secs(carWreckSecs)))
	if k.sim.HP != k.MaxHP || k.sim.Pos != at {
		t.Fatalf("not home repaired: %d HP at %v", k.sim.HP, k.sim.Pos)
	}
}

// Rams: two cars closing at speed both take damage (the heavy tank less),
// once per meeting; a slow nudge nothing. A car at speed into a body on
// foot hurts it.
func TestCarRams(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	a.garage = &garage{Type: "tank"}
	s.refitOnGrid(a)
	goRace(t, s, now)
	buggy := carAt(s, v, sim.Vec3{20, 0, 12}, west)
	tank := carAt(s, a, sim.Vec3{0, 0, 12}, -west)
	buggy.sim.Vel = sim.Vec3{-15, 0, 0}
	tank.sim.Vel = sim.Vec3{10, 0, 0}
	for range 100 {
		cticks(s, 1, now, map[*Player]Cmd{v: {F: 1}, a: {F: 1}})
	}
	db, dt := buggy.MaxHP-buggy.sim.HP, tank.MaxHP-tank.sim.HP
	if db <= 0 || dt <= 0 || dt >= db {
		t.Fatalf("head-on at 25 m/s: buggy -%d, tank -%d", db, dt)
	}
	s2, v2, a2, now2 := raceServer(t, "2")
	goRace(t, s2, now2)
	b2 := carAt(s2, v2, sim.Vec3{20, 0, 12}, west)
	a2.car = 0 // bob on foot in alice's way (his car on its grid slot)
	a2.body = sim.NewBody(sim.Vec3{0, 0, 12})
	a2.Pos = Vec3(a2.body.Pos)
	b2.sim.Vel = sim.Vec3{-20, 0, 0}
	cticks(s2, 120, now2, map[*Player]Cmd{v2: {F: 1}})
	if a2.HP >= 100 {
		t.Fatalf("run over at 20 m/s: bob %d HP", a2.HP)
	}
}

// The shop on the grid: a car of a tier, a swap, upgrades in order (the
// hovercraft gets no tyres); the car is refitted at once (plating: more HP;
// engine, tyres: in the shared sim state). After GO: closed.
func TestCarShop(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	v.Money = 16000
	buy := func(item string) string { return s.carPurchase(v, item) }
	for _, it := range []string{"car:tank", "up:plating", "up:plating", "up:engine", "up:tyres", "front:linter_beam", "rear:spaghetti_code"} {
		if why := buy(it); why != "" {
			t.Fatalf("%s: %s", it, why)
		}
	}
	c := s.carByID(v.car)
	sp := CarSpecOf("tank")
	want := 16000 - sp.Price - CarShop.Upgrades["plating"][0] - CarShop.Upgrades["plating"][1] - CarShop.Upgrades["engine"][0] - CarShop.Upgrades["tyres"][0] - 2*CarShop.Swap
	if c.Type != "tank" || c.MaxHP != int(math.Round(float64(sp.HP)*1.5)) || c.sim.Engine != 1 || c.sim.Tyres != 1 || c.cs.front != "linter_beam" || c.cs.rear != "spaghetti_code" || v.Money != want {
		t.Fatalf("refitted: %s %d HP engine %d tyres %d, %s / %s, money %d (want %d)", c.Type, c.MaxHP, c.sim.Engine, c.sim.Tyres, c.cs.front, c.cs.rear, v.Money, want)
	}
	if g := s.meStateOf(v).Garage; g == nil || g.Type != "tank" || g.Plating != 2 {
		t.Fatalf("me.gar %+v", g)
	}
	for item, why := range map[string]string{"car:tank": "already", "front:linter_beam": "already", "up:nope": "no such", "rear:linter_beam": "no such", "zzz:x": "no such"} {
		if got := buy(item); got == "" || !strings.Contains(got, why) {
			t.Fatalf("%s: %q, want %q", item, got, why)
		}
	}
	buy("up:plating")
	if got := buy("up:plating"); got != "maxed out" {
		t.Fatalf("a 4th plating: %q", got)
	}
	v.Money = 16000
	if why := buy("car:hovercraft"); why != "" || buy("up:tyres") == "" || s.carByID(v.car).sim.Tyres != 0 {
		t.Fatalf("the hovercraft: %q, tyres %d", why, s.carByID(v.car).sim.Tyres)
	}
	v.Money = 0
	if got := buy("car:racecar"); got != "can't afford it" {
		t.Fatalf("broke: %q", got)
	}
	goRace(t, s, now)
	v.Money = 16000
	if got := buy("car:racecar"); got == "" {
		t.Fatal("bought after GO")
	}
	s.Map = maps.LabKart()
	if got := buy("car:racecar"); got != "no race on this map" {
		t.Fatalf("on lab_kart: %q", got)
	}
}

// A lap refills the charges and the boost tank, not the HP; the leader over
// the line is paid; the finish pays by place.
func TestRaceLapRefillsAndPays(t *testing.T) {
	s, v, a, now := raceServer(t, "2")
	goRace(t, s, now)
	c := s.carByID(v.car)
	c.cs.fn, c.sim.Boost, c.sim.HP = 0, 0, 50
	m0 := v.Money
	for _, g := range []int{0, 1, 2, 3, 0} {
		cp := s.Map.Checkpoints[g]
		putAt(s, v, [3]float64{(cp.Min[0] + cp.Max[0]) / 2, 0, (cp.Min[2] + cp.Max[2]) / 2})
		ticks(s, 1, now, nil)
	}
	if c.cs.fn != CarItemOf("linter_beam").Charges || c.sim.Boost != CarShop.Boost || c.sim.HP != 50 || v.Money != m0+s.econ().Race.LapLed {
		t.Fatalf("after a lap: %d charges, %v boost, %d HP, money +%d", c.cs.fn, c.sim.Boost, c.sim.HP, v.Money-m0)
	}
	for _, g := range []int{1, 2, 3, 0} {
		cp := s.Map.Checkpoints[g]
		putAt(s, v, [3]float64{(cp.Min[0] + cp.Max[0]) / 2, 0, (cp.Min[2] + cp.Max[2]) / 2})
		ticks(s, 1, now, nil)
	}
	if want := m0 + 2*s.econ().Race.LapLed + s.econ().Race.Place[0]; s.racer(v).Place != 1 || v.Money != want {
		t.Fatalf("finished %d, money %d want %d", s.racer(v).Place, v.Money, want)
	}
	_ = a
}

// A match is mp_race_races races (the shop between them, the garage kept);
// after the last, the team of the player with the most points wins it.
func TestRaceMatchIsSeveralRaces(t *testing.T) {
	s, v, a, now := raceServer(t, "1")
	s.cvars["mp_race_races"] = "2"
	s.cvars["mp_round_restart_delay"] = "1"
	v.garage = &garage{Type: "racecar"}
	for race := 1; race <= 2; race++ {
		if race == 2 {
			if s.round.Number != 2 || s.round.Phase != PhaseFreeze || s.carByID(v.car).Type != "racecar" {
				t.Fatalf("race 2: round %d phase %q car %s", s.round.Number, s.round.Phase, s.carByID(v.car).Type)
			}
		}
		goRace(t, s, now)
		for _, p := range []*Player{a, v} { // bob wins every race
			for _, g := range []int{0, 1, 2, 3, 0} {
				cp := s.Map.Checkpoints[g]
				putAt(s, p, [3]float64{(cp.Min[0] + cp.Max[0]) / 2, 0, (cp.Min[2] + cp.Max[2]) / 2})
				ticks(s, 1, now, nil)
			}
		}
		ticks(s, int(1.2*SimHz), now, nil)
	}
	if s.round.Phase != PhaseMatchOver || s.round.MatchWinner != content.Artisan || a.Points != 20 {
		t.Fatalf("after 2 races: phase %q winner %q, bob %d points", s.round.Phase, s.round.MatchWinner, a.Points)
	}
}

// Cars are solid to people on foot: walking into a parked car stops at its side.
func TestCarSolidOnFoot(t *testing.T) {
	s := authServer(t)
	s.Map = maps.LabKart()
	s.resetCars()
	c := s.cars[0]
	p := s.join("1.1.1.1")
	p.ready, p.Team = true, content.Vibe
	s.respawn(p)
	x := c.sim.Pos[0] + 3
	p.body = sim.NewBody(sim.Vec3{x, c.sim.Pos[1], c.sim.Pos[2]})
	p.body.Yaw = -math.Pi / 2 // facing -x: at the car
	now := time.Now()
	for range 100 {
		cticks(s, 1, &now, map[*Player]Cmd{p: {F: 1, Yaw: math.Pi / 2}})
	}
	if p.body.Pos[0] < c.sim.Pos[0]+0.4 {
		t.Fatalf("walked through the car: x %v, the car at %v", p.body.Pos[0], c.sim.Pos[0])
	}
}

// A driver's spread doesn't count the car's speed (phase 1's known issue):
// at full speed it is a crouched standing shooter's.
func TestDriverSpreadIgnoresTheCar(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	goRace(t, s, now)
	cticks(s, 150, now, map[*Player]Cmd{v: {F: 1}})
	c := s.carByID(v.car)
	if sim.CarSpeed(&c.sim) < 15 {
		t.Fatalf("not driving fast: %v m/s", sim.CarSpeed(&c.sim))
	}
	w := s.Weapons.Get("prompt")
	still := sim.NewBody(sim.Vec3{0, 0, 0})
	still.Crouched, still.OnGround = true, true
	if got, want := ShotSpread(w, nil, v.body, false), ShotSpread(w, nil, still, false); got != want {
		t.Fatalf("driving at %v m/s: spread %v, crouched still %v", sim.CarSpeed(&c.sim), got, want)
	}
}

// A pit lane (rc_ci_pipeline's, beside the start straight) repairs a car
// driving through it, carPitRepair HP a second, up to its max; nowhere else.
func TestPitLaneRepairs(t *testing.T) {
	s := authServer(t)
	s.Map = maps.RcCiPipeline()
	s.resetCars()
	c := s.cars[0]
	c.sim.HP = 20
	pit := s.Map.PitLanes[0]
	mid := sim.Vec3{(pit.Min[0] + pit.Max[0]) / 2, 0, (pit.Min[2] + pit.Max[2]) / 2}
	c.sim.Pos = sim.Vec3{mid[0], 0, 0} // on the straight beside it
	for range 100 {
		s.tickPits(0.01)
	}
	if c.sim.HP != 20 {
		t.Fatalf("repaired off the pit lane: %d", c.sim.HP)
	}
	c.sim.Pos = mid
	for range 100 {
		s.tickPits(0.01)
	}
	if c.sim.HP < 20+int(carPitRepair)-1 || c.sim.HP > 20+int(carPitRepair) {
		t.Fatalf("a second in the pit lane: %d HP", c.sim.HP)
	}
	for range 1000 {
		s.tickPits(0.01)
	}
	if c.sim.HP != c.MaxHP {
		t.Fatalf("10 s in the pit lane: %d / %d", c.sim.HP, c.MaxHP)
	}
}

// The race shop through the buy message ({t:"buy"} -> {t:"bought"}), the
// garage's car type with no garage (the car sat in, else the racer's, else
// the first type), Force Push's ram factor, and the race cvars' fallbacks.
func TestCarShopEdges(t *testing.T) {
	s, v, a, _ := raceServer(t, "2")
	drain(v)
	s.buy(v, "up:engine")
	if out := drain(v); !strings.Contains(out, `"item":"up:engine","ok":true`) {
		t.Fatalf("bought: %s", out)
	}
	s.buy(v, "up:nope")
	if out := drain(v); !strings.Contains(out, `"ok":false`) {
		t.Fatalf("not for sale: %s", out)
	}
	if sp := s.garageCarSpec(a); sp == nil || sp.ID != s.carByID(a.car).Type {
		t.Fatalf("no garage, in a car: %v", sp)
	}
	a.car = 0
	if sp := s.garageCarSpec(a); sp == nil || sp.ID != s.carByID(s.racer(a).Car).Type {
		t.Fatalf("no garage, out of the car: %v", sp)
	}
	q := &Player{ID: 99}
	if sp := s.garageCarSpec(q); sp != CarTypes[0] {
		t.Fatalf("not racing: %v", sp)
	}
	q.garage = &garage{Type: "no-such-car"}
	if sp := s.garageCarSpec(q); sp != CarTypes[0] {
		t.Fatalf("an unknown type: %v", sp)
	}
	c := s.carByID(v.car)
	base := s.ramFactor(c)
	c.cs.front = "force_push"
	if got := s.ramFactor(c); got != base*CarItemOf("force_push").Ram {
		t.Fatalf("Force Push: %v (plain %v)", got, base)
	}
	s.cvars["mp_race_races"], s.cvars["mp_race_laps"] = "x", "0"
	if s.raceRaces() != 3 || s.raceLaps() != defaultRaceLaps {
		t.Fatalf("fallbacks: %d races, %d laps", s.raceRaces(), s.raceLaps())
	}
}

// Car items never hurt the dead, nothing for 0 damage, never the target-
// practice dummies; one's own blast kills with no credit.
func TestCarHurtEdges(t *testing.T) {
	s, v, a, _ := raceServer(t, "2")
	a.Bot = true
	s.hurt(v, a, 50, "tech_debt", Vec3{})
	a.Bot = false
	s.hurt(v, a, 0, "tech_debt", Vec3{})
	if a.HP != 100 {
		t.Fatalf("a dummy, or no damage: %d HP", a.HP)
	}
	s.hurt(v, v, 500, "tech_debt", Vec3{})
	if !v.Dead {
		t.Fatal("one's own mine at 500")
	}
	hp := v.HP
	s.hurt(a, v, 50, "tech_debt", Vec3{})
	if v.HP != hp {
		t.Fatal("hurt the dead")
	}
}

// The MCP's player_set buy buys from the race shop as the client's buy does
// (it said "no such item" for every car item).
func TestMCPBuysFromTheRaceShop(t *testing.T) {
	s, v, _, _ := raceServer(t, "2")
	v.Money = 16000
	c := mcpFor(t, s, true)
	c.call("player_set", map[string]any{"player": v.Name, "buy": "up:plating"})
	if v.garage == nil || v.garage.Plating != 1 {
		t.Fatalf("garage after the buy: %+v", v.garage)
	}
	if e := c.call("player_set", map[string]any{"player": v.Name, "buy": "up:nope"}, true)["error"].(string); !strings.Contains(e, "no such") {
		t.Fatalf("a bad item: %q", e)
	}
}

// The race shop is open on the grid and over the results of a race another
// follows (not the last one's), and the round's buy time counts down to GO
// (it said the CS buy time, 20 s past GO, while the shop had closed).
func TestRaceShopBetweenRaces(t *testing.T) {
	s, v, _, now := raceServer(t, "1")
	s.cvars["mp_race_races"] = "2"
	cticks(s, 1, now, nil)
	if s.round.Phase != PhaseFreeze {
		t.Fatalf("phase %v", s.round.Phase)
	}
	if b := s.roundMsgOf(*now).Buy; b <= 0 || b > raceGridSecs {
		t.Fatalf("on the grid: buy %v (want to GO, at most %v)", b, raceGridSecs)
	}
	goRace(t, s, now)
	if why := s.carShopOpen(v); why == "" {
		t.Fatal("open after GO")
	}
	if b := s.roundMsgOf(*now).Buy; b != 0 {
		t.Fatalf("racing: buy %v", b)
	}
	s.endRound(*now, content.Vibe, "race")
	if s.round.Phase != PhaseOver || s.carShopOpen(v) != "" {
		t.Fatalf("between races: phase %v, %q", s.round.Phase, s.carShopOpen(v))
	}
	if b, left := s.roundMsgOf(*now).Buy, s.roundMsgOf(*now).Left; b != left+raceGridSecs {
		t.Fatalf("between races: buy %v, left %v", b, left)
	}
	s.round.Number = 2
	if s.carShopOpen(v) == "" {
		t.Fatal("open after the last race")
	}
}

// The looks bits in the car snapshot (fx: throttle, boost, slipping, braking)
// and the yaw rate (yv), from the driver's command; nobody in it: none.
func TestCarSnapshotLooks(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	goRace(t, s, now)
	c := s.carByID(v.car)
	cticks(s, 30, now, map[*Player]Cmd{v: {F: 1, R: 1}})
	if c.Fx != carFxThrottle || c.YawVel == 0 {
		t.Fatalf("throttle and steering: fx %d, yv %v", c.Fx, c.YawVel)
	}
	cticks(s, 1, now, map[*Player]Cmd{v: {B: BtnBoost}})
	if c.Fx != carFxThrottle|carFxBoost {
		t.Fatalf("boost: fx %d", c.Fx)
	}
	c.sim.Slip = 1
	cticks(s, 1, now, map[*Player]Cmd{v: {F: -1}})
	if c.Fx != carFxSlip|carFxBrake {
		t.Fatalf("slipping, braking: fx %d", c.Fx)
	}
	v.car = 0
	cticks(s, 1, now, nil)
	if c.Fx != 0 {
		t.Fatalf("empty: fx %d", c.Fx)
	}
}

// car_hp sets a car's HP (its damage looks); 0 wrecks it and its driver dies.
func TestCarHPCommand(t *testing.T) {
	s, v, _, now := raceServer(t, "2")
	goRace(t, s, now)
	s.adminCommand(v, "car_hp", []string{strconv.Itoa(v.car), "20"})
	if c := s.carByID(v.car); c.sim.HP != 20 || c.HP != 20 {
		t.Fatalf("car_hp 20: %d / %d", c.sim.HP, c.HP)
	}
	s.adminCommand(v, "car_hp", []string{"99", "5"})
	s.adminCommand(v, "car_hp", []string{"1"})
	if out := drain(v); !strings.Contains(out, "no car 99") || !strings.Contains(out, "usage") {
		t.Fatalf("errors: %s", out)
	}
	id := v.car
	s.adminCommand(v, "car_hp", []string{strconv.Itoa(id), "0"})
	if c := s.carByID(id); c.sim.HP != 0 || !v.Dead {
		t.Fatalf("car_hp 0: %d HP, driver dead %v", c.sim.HP, v.Dead)
	}
}
