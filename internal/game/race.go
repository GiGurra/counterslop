package game

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// Slop n Roll Racing (Map.Mode "race", docs/racing.md), phase 2: the race
// rules.
//
//   - One race is the match's one round. Anyone on a team races (free for
//     all; the teams only colour the cars), one racer is enough (a time
//     trial). Playing bots race too (ai_race.go).
//   - The round's freeze time is the grid: every racer in a car on their
//     grid slot (Map.Cars, in order), the countdown (raceGridSecs) holding
//     the cars still. Then GO: race.
//   - Laps: the checkpoints (Map.Checkpoints) in order, the first the
//     start/finish line. A racer's progress is the gates passed in order
//     (racerState.CP): a gate out of order counts for nothing (no cutting).
//     The grid is behind the line: crossing it starts lap 1; crossing it
//     after the last lap (mp_race_laps, default 4; a map may pin it)
//     finishes.
//   - Positions: the finished by place, then the most gates passed, then the
//     nearest to the next gate. The round state's race list is in that order.
//   - Wrecks: a racer who dies (fell into the pit under the track, shot, the
//     car's HP at 0) is back after raceRespawnSecs in their car, repaired,
//     on the last gate they passed (its Spawn, facing its Yaw), keeping the
//     lap. During the race a racer can't get out of the car (CarExits).
//   - The end: everyone finished, raceFinishWaitSecs after the winner
//     crossed the line, or the race's time limit (raceTimeSecs). Placement
//     points (racePoints) go to each finisher's score; the winner's team
//     takes the round. The results show, then the map vote.
//
// The race is in the round state of every snapshot (laps, gates, the racers
// in position order with their progress, place, time and wreck countdown);
// no event is needed for any of it.

const (
	defaultRaceLaps    = 4
	raceGridSecs       = 5.0 // the freeze time: on the grid, the countdown's last 3 s shown big
	raceRespawnSecs    = 3.0
	raceFinishWaitSecs = 30.0 // after the winner finishes
	raceTimeSecs       = 600  // the time limit
)

// racePoints: score points by finishing place (1st, 2nd, ...); the rest 1.
var racePoints = []int{10, 8, 6, 5, 4, 3, 2}

// racerState is one racer in the round state, in position order.
type racerState struct {
	ID int `json:"id" net:"cold"`
	// CP: gates passed in order since the start; the next is CP % gates,
	// the lap CP-1 / gates + 1 (none before the line).
	CP    int     `json:"cp" net:"cold"`
	Place int     `json:"pl,omitempty" net:"cold"`      // finishing place; 0: still racing
	Time  float64 `json:"t,omitempty" net:"cold,exact"` // finished: seconds from GO
	// Wreck: seconds until a wrecked racer is back on the track (0: racing).
	Wreck float64 `json:"wr,omitempty" net:"hot,exact"`
	Car   int     `json:"car,omitempty" net:"cold"` // the racer's car (Car.ID; 0: on foot)
	// Splits (parkour, docs/racing.md): seconds from GO at the last gate
	// passed, and against the racer's best run there (Delta < 0: ahead; set
	// once there is a best); finished: the best run's time and whether this
	// one is it.
	Split    float64 `json:"sp,omitempty" net:"cold,exact"`
	Delta    float64 `json:"dl,omitempty" net:"cold,exact"`
	HasDelta bool    `json:"hd,omitempty" net:"cold"`
	PB       float64 `json:"pb,omitempty" net:"cold,exact"`
	Best     bool    `json:"best,omitempty" net:"cold"`
	Pit      bool    `json:"pit,omitempty" net:"cold"` // in the pit stop (pitstop.go): off the track, the clock running

	splits []float64 `net:"-"` // every gate's split this run
	slot   int       `net:"-"` // the grid slot (Map.Cars index)
	backAt time.Time `net:"-"` // wrecked: when they are back
	dist   float64   `net:"-"` // to the next gate's middle (positions)
}

// raceMode: this map races. Callers hold s.mu.
func (s *Server) raceMode() bool { return s.Map.Mode == content.ModeRace }

// raceLive: the race is on (after GO, before the results). Callers hold s.mu.
func (s *Server) raceLive() bool { return s.raceMode() && s.round.Phase == PhaseLive }

// raceRaces: mp_race_races (the map may pin it), at least 1: races in a match.
func (s *Server) raceRaces() int {
	if v, err := strconv.Atoi(s.cvar("mp_race_races")); err == nil && v > 0 {
		return v
	}
	return 3
}

// raceMatchWinner: the team of the player with the most points after the
// last race (the first such in id order).
func (s *Server) raceMatchWinner() string {
	best, win := -1, content.Vibe
	for _, p := range s.ordered() {
		if teamIdx(p.Team) >= 0 && p.Points > best {
			best, win = p.Points, p.Team
		}
	}
	return win
}

// raceLaps: mp_race_laps (the map may pin it), at least 1.
func (s *Server) raceLaps() int {
	if v, err := strconv.Atoi(s.cvar("mp_race_laps")); err == nil && v > 0 {
		return v
	}
	return defaultRaceLaps
}

// racer is p's entry in the race (nil: not racing). Callers hold s.mu.
func (s *Server) racer(p *Player) *racerState {
	for i := range s.round.Racers {
		if s.round.Racers[i].ID == p.ID {
			return &s.round.Racers[i]
		}
	}
	return nil
}

// carLocked: p can't get out of their car (a car race is on; on foot the
// cars are free to leave). Callers hold s.mu.
func (s *Server) carLocked(p *Player) bool {
	rs := s.racer(p)
	return s.raceLive() && rs != nil && rs.Car != 0
}

// onFoot: this race is run on foot (a parkour course, Map.OnFoot).
func (s *Server) onFoot() bool { return s.raceMode() && s.Map.OnFoot }

// raceCombat: racers can hurt each other (mp_race_combat; a map may pin it).
func (s *Server) raceCombat() bool { return s.cvar("mp_race_combat") != "0" }

// spared: by's damage doesn't reach q: a teammate with friendly fire off, or
// anyone in a race without combat (mp_race_combat 0). Your own always does.
func (s *Server) spared(by, q *Player) bool {
	if by == nil || by == q {
		return false
	}
	return s.raceMode() && !s.raceCombat() || !s.friendlyFire() && q.Team != "" && q.Team == by.Team
}

// startRace is the race half of startRound, after the spawns: the racers in
// their cars on the grid (the freeze time is the countdown).
func (s *Server) startRace() {
	r := &s.round
	r.Racers, r.Laps, r.Gates, r.started = nil, 0, 0, time.Time{}
	s.resetPickups()
	if !s.raceMode() {
		return
	}
	r.Laps, r.Gates, r.P2P = s.raceLaps(), len(s.Map.Checkpoints), s.Map.PointToPoint
	if s.Map.PointToPoint {
		r.Laps = 1
	}
	for _, p := range s.ordered() {
		if s.onFoot() && s.racing(p) { // on foot: at the start (the spawns), no car
			r.Racers = append(r.Racers, racerState{ID: p.ID})
			continue
		}
		if !s.racing(p) || len(r.Racers) >= len(s.cars) {
			continue // no grid slot left: watch this one
		}
		rs := racerState{ID: p.ID, slot: len(r.Racers)}
		c := s.cars[rs.slot]
		rs.Car = c.ID
		r.Racers = append(r.Racers, rs)
		s.fitCar(c, p) // their garage's car and upgrades (carshop.go)
		_ = s.seatInCar(p, c.ID)
	}
}

// racing: p takes part (a player or a playing bot on a team, alive or not;
// not a target-practice bot).
func (s *Server) racing(p *Player) bool {
	return !p.Bot && p.ready && teamIdx(p.Team) >= 0 && !(s.Map.OnFoot && p.ai != nil && len(s.Map.Route) == 0) // bots run a course with a route (ai_parkour.go)
}

// tickRace runs every live race tick: gates, wrecks, positions, the end.
// Callers hold s.mu.
func (s *Server) tickRace(now time.Time) {
	r := &s.round
	if r.started.IsZero() {
		r.started = now
	}
	s.joinRace()
	s.tickPitStops() // in or out of the pit as each racer's garage says (pitstop.go)
	n := len(s.Map.Checkpoints)
	finishCP := r.Laps*n + 1
	if r.P2P {
		finishCP = n // the last gate is the finish
	}
	for i := range r.Racers {
		rs := &r.Racers[i]
		p := s.players[rs.ID]
		if p == nil {
			continue
		}
		if c := s.carByID(rs.Car); c != nil && c.HP <= 0 && !p.Dead && p.car == c.ID {
			s.killBy(nil, p, "wreck", false) // the car is scrap: so is its driver
		}
		if p.pit { // in the pit stop: off the track, nothing to pass, no wreck
			rs.Wreck = 0
			continue
		}
		if p.Dead {
			if rs.backAt.IsZero() {
				rs.backAt = now.Add(secs(raceRespawnSecs))
				if c := s.carByID(rs.Car); c != nil && c.sim.HP > 0 { // the driver dead: the car is a wreck too
					c.sim.HP, c.cs.wreckAt = 0, now
					c.sync()
				}
			}
			if !now.Before(rs.backAt) {
				s.raceRespawn(p, rs)
			}
		}
		if p.Dead || rs.Place > 0 || n == 0 {
			rs.Wreck = 0
			if p.Dead {
				rs.Wreck = math.Max(0, rs.backAt.Sub(now).Seconds())
			}
			continue
		}
		rs.backAt = time.Time{}
		rs.Wreck = 0
		// The gates along the path driven this tick (sweep.go: a fast car,
		// a burst of commands in one tick: a 2 m gate is never jumped).
		for a, b := range p.steps() {
			if rs.CP >= finishCP {
				break
			}
			if s.Map.Checkpoints[rs.CP%n].Crossed([3]float64(a), [3]float64(b)) {
				rs.CP++
				s.raceSplit(p, rs, now)
				if !r.P2P && rs.CP > 1 && (rs.CP-1)%n == 0 { // over the line: a lap done
					s.raceLap(p, rs, i == 0)
				}
				if rs.CP >= finishCP {
					s.raceFinish(now, rs)
				}
			}
		}
		g := s.Map.Checkpoints[rs.CP%n]
		mid := Vec3{(g.Min[0] + g.Max[0]) / 2, (g.Min[1] + g.Max[1]) / 2, (g.Min[2] + g.Max[2]) / 2}
		rs.dist = math.Hypot(math.Hypot(p.Pos[0]-mid[0], p.Pos[1]-mid[1]), p.Pos[2]-mid[2])
	}
	slices.SortStableFunc(r.Racers, func(a, b racerState) int {
		switch {
		case a.Place > 0 && b.Place > 0:
			return cmp.Compare(a.Place, b.Place)
		case a.Place > 0:
			return -1
		case b.Place > 0:
			return 1
		case a.CP != b.CP:
			return cmp.Compare(b.CP, a.CP)
		}
		return cmp.Compare(a.dist, b.dist)
	})
	done := len(r.Racers) > 0
	for _, rs := range r.Racers {
		done = done && rs.Place > 0
	}
	if done || now.After(r.endsAt) {
		s.endRace(now)
	}
}

// dropRacer: p leaves the race (left the server, a bot kicked): their
// racer entry goes at once (the standings without a ghost) and their car,
// parked in the pit or not, is its grid slot's stock car again for whoever
// takes the slot next. Callers hold s.mu.
func (s *Server) dropRacer(p *Player) {
	p.pit, p.car = false, 0
	if !s.raceMode() {
		return
	}
	rs := s.racer(p)
	if rs == nil {
		return
	}
	if c := s.carByID(rs.Car); c != nil {
		s.carStock(c)
	}
	s.round.Racers = slices.DeleteFunc(s.round.Racers, func(r racerState) bool { return r.ID == p.ID })
}

// carStock: car c back on its start, its slot's stock car, nobody's.
func (s *Server) carStock(c *Car) {
	c.parked = false
	s.carHome(c)
	s.fitCar(c, nil)
}

// joinRace: a player who joined (or came back to a team) during the race
// races from a free grid slot, lap 0. Racers who left drop out.
func (s *Server) joinRace() {
	r := &s.round
	r.Racers = slices.DeleteFunc(r.Racers, func(rs racerState) bool {
		p := s.players[rs.ID]
		gone := p == nil || !s.racing(p)
		if c := s.carByID(rs.Car); gone && c != nil && !s.carTaken(c.ID) {
			s.carStock(c) // nobody's now: stock for the next one
		}
		return gone
	})
	for _, p := range s.ordered() {
		if !s.racing(p) || s.racer(p) != nil || p.Dead {
			continue
		}
		if s.onFoot() { // on foot: from the start, no car
			r.Racers = append(r.Racers, racerState{ID: p.ID})
			continue
		}
		taken := map[int]bool{}
		for _, rs := range r.Racers {
			taken[rs.slot] = true
		}
		for slot := range s.cars {
			if taken[slot] || s.carTaken(s.cars[slot].ID) {
				continue
			}
			c := s.cars[slot]
			sp := s.Map.Cars[min(c.spawn, len(s.Map.Cars)-1)]
			r.Racers = append(r.Racers, racerState{ID: p.ID, slot: slot, Car: c.ID})
			_ = s.seatInCar(p, c.ID, &CarAt{Pos: sp.Pos, Yaw: sp.Yaw})
			s.fitCar(c, p)
			break
		}
	}
}

// raceRespawn: a wrecked racer back in their car, repaired, on the last gate
// they passed (the grid slot before the line).
func (s *Server) raceRespawn(p *Player, rs *racerState) {
	if rs.Car == 0 { // on foot: respawn puts them on their last gate (raceSpawn)
		s.respawn(p)
		rs.backAt = time.Time{}
		return
	}
	n := len(s.Map.Checkpoints)
	at := CarAt{}
	if rs.CP > 0 && n > 0 {
		g := s.Map.Checkpoints[(rs.CP-1)%n]
		at = CarAt{Pos: g.Spawn, Yaw: g.Yaw}
	} else if sp := rs.slot; sp < len(s.Map.Cars) {
		at = CarAt{Pos: s.Map.Cars[sp].Pos, Yaw: s.Map.Cars[sp].Yaw}
	}
	s.respawn(p)
	c := s.carByID(rs.Car)
	if c == nil {
		return
	}
	c.parked = false // back from the pit stop (pitstop.go)
	c.sim = *sim.NewCar(c.spec, sim.Vec3(at.Pos), at.Yaw)
	s.fitCar(c, p) // repaired, refilled, their garage's
	_ = s.seatInCar(p, c.ID)
	rs.backAt = time.Time{}
}

// raceSpawn: where an on-foot racer p comes back: their last gate's spawn,
// facing along the course (ok false: not racing on foot, or not past the
// start yet: the team spawns).
func (s *Server) raceSpawn(p *Player) (Vec3, float64, bool) {
	if !s.onFoot() {
		return Vec3{}, 0, false
	}
	rs := s.racer(p)
	n := len(s.Map.Checkpoints)
	if rs == nil || rs.CP == 0 || n == 0 || !s.raceLive() {
		return Vec3{}, 0, false
	}
	g := s.Map.Checkpoints[(rs.CP-1)%n]
	return Vec3(g.Spawn), g.Yaw, true
}

// raceSplit: rs passed a gate (rs.CP gates now): its split, seconds from GO,
// and how it compares with p's best run of this course (the server's
// memory: by name and map, while it runs).
func (s *Server) raceSplit(p *Player, rs *racerState, now time.Time) {
	rs.Split = now.Sub(s.round.started).Seconds()
	rs.Delta, rs.HasDelta = 0, false
	if pb := s.bests[s.bestKey(p)]; rs.CP-1 < len(pb) {
		rs.Delta, rs.HasDelta = rs.Split-pb[rs.CP-1], true
	}
	rs.splits = append(rs.splits, rs.Split)
}

// bestKey: whose best run of which course.
func (s *Server) bestKey(p *Player) string { return s.Map.Name + "\x00" + p.Name }

// raceLap: racer p finished a lap (led: in the lead as they crossed): the
// car's charges and boost tank refilled (not its HP), and the leader paid.
func (s *Server) raceLap(p *Player, rs *racerState, led bool) {
	if c := s.carByID(rs.Car); c != nil {
		s.refillCar(c, s.garageOf(p))
	}
	if led {
		s.pay(p, s.econ().Race.LapLed, "led a lap")
	}
}

// raceFinish: rs crossed the line after the last lap.
func (s *Server) raceFinish(now time.Time, rs *racerState) {
	r := &s.round
	place := 1
	for _, o := range r.Racers {
		if o.Place > 0 {
			place++
		}
	}
	rs.Place, rs.Time = place, now.Sub(r.started).Seconds()
	if p := s.players[rs.ID]; p != nil {
		k := s.bestKey(p)
		if pb := s.bests[k]; len(pb) == 0 || rs.Time < pb[len(pb)-1] {
			s.bests[k] = slices.Clone(rs.splits) // a personal best: the splits to beat
			rs.Best = true
		}
		rs.PB = s.bests[k][len(s.bests[k])-1]
		p.Points += racePointsFor(place)
		money := s.econ().Race.Finish
		if pl := s.econ().Race.Place; place <= len(pl) {
			money = pl[place-1]
		}
		s.pay(p, money, "finished "+ordinal(place))
	}
	if place == 1 {
		if end := now.Add(secs(raceFinishWaitSecs)); end.Before(r.endsAt) {
			r.endsAt = end
		}
	}
	s.sendAll(raceEvent{T: "race", Ev: "finish", ID: rs.ID, Place: place, Time: rs.Time})
}

// racePointsFor: the score points for finishing place.
func racePointsFor(place int) int {
	if place >= 1 && place <= len(racePoints) {
		return racePoints[place-1]
	}
	return 1
}

// endRace: the results. The winner's team (the leader's, if nobody
// finished) takes the round.
func (s *Server) endRace(now time.Time) {
	s.pitsOut() // out of the pit for the results, on their gate
	r := &s.round
	win := content.Vibe
	if len(r.Racers) > 0 {
		if p := s.players[r.Racers[0].ID]; p != nil && teamIdx(p.Team) >= 0 {
			win = p.Team
		}
	}
	s.endRound(now, win, "race")
}

// raceEvent is sent when a racer finishes (optional: the round state has it).
type raceEvent struct {
	T     string  `json:"t"`  // "race"
	Ev    string  `json:"ev"` // "finish"
	ID    int     `json:"id"`
	Place int     `json:"pl"`
	Time  float64 `json:"time"`
}

// ordinal: 1st, 2nd, 3rd, 4th...
func ordinal(n int) string {
	suf := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suf = "st"
		case 2:
			suf = "nd"
		case 3:
			suf = "rd"
		}
	}
	return strconv.Itoa(n) + suf
}
