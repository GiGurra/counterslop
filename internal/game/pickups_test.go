package game

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// pickupRace: a live two-car race on lab_race with one pickup of each kind
// given, far from the grid (at x 1000 + 20 i).
func pickupRace(t *testing.T, kinds ...string) (*Server, *Player, *Player, *time.Time) {
	t.Helper()
	s, v, a, now := raceServer(t, "3")
	s.Map.Pickups = nil
	for i, k := range kinds {
		s.Map.Pickups = append(s.Map.Pickups, c.Pickup{Kind: k, Pos: [3]float64{1000 + 20*float64(i), 0, 0}})
	}
	goRace(t, s, now)
	s.resetPickups()
	return s, v, a, now
}

// driveThrough: p's car through pickup i, one pickup tick.
func driveThrough(s *Server, p *Player, i int, now time.Time) {
	putAt(s, p, [3]float64(s.round.Pickups[i].Pos))
	s.tickPickups(now)
}

func TestPickupsEachKind(t *testing.T) {
	s, v, a, now := pickupRace(t, c.PickupBonus, c.PickupHotfix, c.PickupReorg, c.PickupCaffeine, c.PickupFreeze)
	car := s.carByID(v.car)
	if len(s.round.Pickups) != 5 || !s.round.Pickups[0].Up {
		t.Fatalf("pickups %+v", s.round.Pickups)
	}
	s.tickPickups(*now) // nobody near: nothing taken
	for _, pu := range s.round.Pickups {
		if !pu.Up {
			t.Fatalf("taken with nobody near: %+v", pu)
		}
	}

	money := v.Money
	driveThrough(s, v, 0, *now)
	if v.Money != money+s.econ().Race.Bonus || s.econ().Race.Bonus != 500 || s.round.Pickups[0].Up {
		t.Fatalf("Bonus Approved: money %d -> %d, up %v", money, v.Money, s.round.Pickups[0].Up)
	}

	car.sim.HP = 10
	driveThrough(s, v, 1, *now)
	if want := 10 + int(float64(car.MaxHP)*pickupHotfixFrac+0.5); car.sim.HP != want || car.HP != want {
		t.Fatalf("Hotfix Patch: HP %d (snapshot %d), want %d", car.sim.HP, car.HP, want)
	}
	car.sim.HP = car.MaxHP - 1
	s.round.Pickups[1].Up = true
	driveThrough(s, v, 1, *now)
	if car.sim.HP != car.MaxHP {
		t.Fatalf("Hotfix Patch over the max: %d of %d", car.sim.HP, car.MaxHP)
	}

	fn, rn := car.cs.fn, car.cs.rn
	car.cs.fn, car.cs.rn = 0, 0
	driveThrough(s, v, 2, *now)
	if car.cs.fn != fn || car.cs.rn != rn || fn+rn == 0 {
		t.Fatalf("Reorg Budget: charges %d/%d, want %d/%d", car.cs.fn, car.cs.rn, fn, rn)
	}

	tank := carTank(s.garageOf(v))
	car.sim.Boost = 0
	driveThrough(s, v, 3, *now)
	if car.sim.Boost != tank || tank <= 0 {
		t.Fatalf("Caffeine: boost %v, want %v", car.sim.Boost, tank)
	}

	driveThrough(s, v, 4, *now)
	hp := car.sim.HP
	s.damageCar(car, 30, a, "test")
	if car.sim.HP != hp || !car.Shield {
		t.Fatalf("Code Freeze: HP %d -> %d, shield %v", hp, car.sim.HP, car.Shield)
	}
	*now = now.Add(time.Duration(pickupFreezeSecs*float64(time.Second)) + time.Second)
	s.lastTickAt = *now
	s.tickPickups(*now)
	s.damageCar(car, 30, a, "test")
	if car.sim.HP != hp-30 || car.Shield {
		t.Fatalf("after the freeze: HP %d, want %d; shield %v", car.sim.HP, hp-30, car.Shield)
	}
}

func TestPickupRespawnsAndOneCarTakesIt(t *testing.T) {
	s, v, a, now := pickupRace(t, c.PickupCaffeine)
	s.Map.Pickups[0].Respawn = 7
	s.resetPickups()
	cv, ca := s.carByID(v.car), s.carByID(a.car)
	cv.sim.Boost, ca.sim.Boost = 0, 0
	pos := [3]float64(s.round.Pickups[0].Pos)
	putAt(s, v, pos)
	putAt(s, a, [3]float64{pos[0] + 1, pos[1], pos[2]})
	s.tickPickups(*now)
	if (cv.sim.Boost > 0) == (ca.sim.Boost > 0) {
		t.Fatalf("both or neither took it: boost %v / %v", cv.sim.Boost, ca.sim.Boost)
	}
	back := now.Add(7 * time.Second)
	s.tickPickups(back.Add(-time.Millisecond))
	if s.round.Pickups[0].Up {
		t.Fatal("back before its 7 s")
	}
	s.tickPickups(back)
	if !s.round.Pickups[0].Up {
		t.Fatal("not back after its 7 s")
	}
}

// Before GO (and out of a race) nothing is taken; a new race puts them all back.
func TestPickupsOnlyInALiveRace(t *testing.T) {
	s, v, _, now := raceServer(t, "3")
	s.Map.Pickups = []c.Pickup{{Kind: c.PickupBonus, Pos: [3]float64{1000, 0, 0}}}
	s.resetPickups()
	money := v.Money
	driveThrough(s, v, 0, *now)
	if !s.round.Pickups[0].Up || v.Money != money {
		t.Fatalf("taken on the grid: %+v, money %d -> %d", s.round.Pickups[0], money, v.Money)
	}
	goRace(t, s, now)
	driveThrough(s, v, 0, *now)
	if s.round.Pickups[0].Up {
		t.Fatal("not taken in the race")
	}
	s.startRace()
	if !s.round.Pickups[0].Up {
		t.Fatal("not back for the next race")
	}
}

// A Feature Flag turns into one of the others, from the server's seeded rng:
// over many, each of them.
func TestFeatureFlagIsEachOfTheOthers(t *testing.T) {
	s, v, _, now := pickupRace(t, c.PickupFlag)
	s.Seed(5)
	seen := map[string]bool{}
	ch := make(chan []byte, 512)
	v.send = ch
	for range 200 {
		s.round.Pickups[0].Up = true
		driveThrough(s, v, 0, *now)
		for len(ch) > 0 {
			var e carEvent
			if json.Unmarshal(<-ch, &e) == nil && e.T == "car" && e.Ev == "pickup" {
				if e.Via != c.PickupFlag {
					t.Fatalf("a flag's event: %+v", e)
				}
				seen[e.Item] = true
			}
		}
	}
	for _, k := range flagKinds {
		if !seen[k] {
			t.Errorf("a Feature Flag never turned into %s (%v)", k, seen)
		}
	}
	if seen[c.PickupFlag] {
		t.Error("a Feature Flag turned into a Feature Flag")
	}
}

// DefaultEconomy's race money is economy.json's.
func TestDefaultEconomyRace(t *testing.T) {
	b, err := os.ReadFile("../../web/assets/economy.json")
	if err != nil {
		t.Fatal(err)
	}
	e, err := LoadEconomy(b)
	if err != nil {
		t.Fatal(err)
	}
	x, _ := json.Marshal(e.Race)
	y, _ := json.Marshal(DefaultEconomy.Race)
	if string(x) != string(y) {
		t.Fatalf("DefaultEconomy race %s, economy.json %s", y, x)
	}
}

// On every rc_ track the bots take the power-ups: over 3 laps of 2 v 2 each
// pickup is taken at least once (the ones over a jump's gap or a shortcut's
// too: the cars fly through them; not one off the bots' line, as the
// shortcut's), and the bots keep finishing.
func TestBotsTakePickupsEveryTrack(t *testing.T) {
	for _, name := range raceMaps() {
		if len(maps.Find(name)().Pickups) == 0 {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			taken := map[int]int{}
			s := botRace(t, name, 2, "3", 600, nil, func(s *Server) {
				for i, pu := range s.round.Pickups {
					if !pu.Up && !pu.backAt.IsZero() && pu.backAt.Sub(s.clock()) > time.Duration((pu.respawn-0.011)*float64(time.Second)) {
						taken[i]++ // taken this tick
					}
				}
			})
			t.Logf("%s: taken %v of %d", name, taken, len(s.round.Pickups))
			l := s.raceLineOf()
			for i, mp := range s.Map.Pickups {
				near := false
				for _, q := range l.pts {
					near = near || xzDist(q, Vec3(mp.Pos)) <= raceDetourOff
				}
				if !near {
					t.Logf("%s at %v: off the bots' line (for players)", mp.Kind, mp.Pos)
					continue
				}
				if taken[i] == 0 {
					t.Errorf("%s at %v: never taken", mp.Kind, mp.Pos)
				}
			}
			for _, rs := range s.round.Racers {
				if rs.Place == 0 {
					t.Errorf("%s didn't finish", s.players[rs.ID].Name)
				}
			}
		})
	}
}

// A bot detours for a pickup worth it a few metres off its line (out of a
// car's reach from the line), not for one it has no use for.
func TestBotDetoursForAPickup(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want bool
	}{{c.PickupBonus, true}, {c.PickupHotfix, false}} {
		t.Run(tc.kind, func(t *testing.T) {
			taken := false
			botRace(t, "rc_ci_pipeline", 1, "1", 120, func(s *Server, bots []*Player) {
				s.aiPeace = true // nobody hurt: a repair is no use
				// The start straight's line is near z 0 (heading west, the cars at z
				// 0.5..1 here); this is 4.5 m off it, out of reach from the line.
				s.Map.Pickups = []c.Pickup{{Kind: tc.kind, Pos: [3]float64{-30, 0, 4.5}}}
				s.resetPickups()
			}, func(s *Server) {
				if len(s.round.Pickups) == 1 && !s.round.Pickups[0].Up {
					taken = true
				}
			})
			if taken != tc.want {
				t.Fatalf("%s 4.5 m off the line: taken %v, want %v", tc.kind, taken, tc.want)
			}
		})
	}
}

func TestPickupWorth(t *testing.T) {
	s, v, _, _ := pickupRace(t)
	car := s.carByID(v.car)
	for _, k := range []string{c.PickupBonus, c.PickupFreeze, c.PickupFlag} {
		if !s.pickupWorth(v, car, k) {
			t.Errorf("%s not worth it", k)
		}
	}
	if s.pickupWorth(v, car, c.PickupHotfix) || s.pickupWorth(v, car, c.PickupCaffeine) || s.pickupWorth(v, car, c.PickupReorg) {
		t.Fatal("a repair, the boost or charges worth it to a whole, full car")
	}
	fn := car.cs.fn
	car.sim.HP, car.sim.Boost, car.cs.fn = car.MaxHP/2, 0, 0
	if !s.pickupWorth(v, car, c.PickupHotfix) || !s.pickupWorth(v, car, c.PickupCaffeine) || !s.pickupWorth(v, car, c.PickupReorg) {
		t.Fatal("not worth it to a hurt car out of boost and charges")
	}
	if car.cs.fn != 0 || fn == 0 {
		t.Fatalf("pickupWorth changed the charges: %d (full %d)", car.cs.fn, fn)
	}
}
