package game

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// Rounds and The Big Diff (the bomb: a force push to main).
//
// Phases: warmup (fewer than one player per side, or mp_respawn 1: timed
// respawns, no objective) -> freeze (everyone respawned at their side's
// spawns, nobody moves or shoots) -> live (mp_roundtime) -> planted (the CI
// timer, mp_c4timer) -> over (the result for a few seconds) -> freeze...
//
// Win conditions: the CI timer runs out = Vibe Coders ("Shipped to prod");
// defused, or the round time runs out without a plant = Artisans ("Reverted");
// elimination either way (with the Big Diff planted, killing every Artisan
// still wins it for the Vibe Coders; killing every Vibe Coder doesn't stop it).
//
// One Big Diff per round, carried by a random Vibe Coder, dropped where its
// carrier dies (any live Vibe Coder walks over it to pick it up). Plant: hold
// +use standing still on the ground inside a site. Defuse ("git revert"):
// an Artisan holds +use standing still next to it. Moving resets progress.

const (
	PhaseWarmup  = "warmup"
	PhaseFreeze  = "freeze"
	PhaseLive    = "live"
	PhasePlanted = "planted"
	PhaseOver    = "over"

	plantSecs   = 3.0
	defuseSecs  = 8.0
	overSecs    = 5.0
	pickupRange = 1.2
	defuseRange = 1.8
)

// Bomb states.
const (
	BombCarried  = "carried"
	BombDropped  = "dropped"
	BombPlanted  = "planted"
	BombDefused  = "defused"
	BombExploded = "exploded"
)

type bombState struct {
	State   string `json:"st,omitempty"`
	Carrier int    `json:"by,omitempty"` // carrier while carried; planter / defuser while working on it
	Pos     Vec3   `json:"p"`
	Site    string `json:"site,omitempty"`
	// Progress of the plant (carried) or defuse (planted), 0..1, and who.
	Progress float64 `json:"pr,omitempty"`
	Worker   int     `json:"who,omitempty"`
}

type roundState struct {
	Phase  string         `json:"ph"`
	Number int            `json:"n"`
	Score  map[string]int `json:"sc"`
	Bomb   bombState      `json:"bomb"`
	Winner string         `json:"win,omitempty"`
	Reason string         `json:"why,omitempty"`

	endsAt    time.Time
	workTicks int // plant or defuse progress in commands (sim ticks)
}

// roundMsg is the round state in every snapshot (left = seconds in the phase).
type roundMsg struct {
	*roundState
	Left float64 `json:"left"`
}

// roundEvent is sent once when something happens: start, live, planted,
// defused, exploded, end (with winner and reason).
type roundEvent struct {
	T      string         `json:"t"`
	Ev     string         `json:"ev"`
	Winner string         `json:"win,omitempty"`
	Reason string         `json:"why,omitempty"`
	Site   string         `json:"site,omitempty"`
	Pos    *Vec3          `json:"p,omitempty"`
	By     int            `json:"by,omitempty"`
	Score  map[string]int `json:"sc,omitempty"`
	Number int            `json:"n,omitempty"`
}

func newRound() roundState {
	return roundState{Phase: PhaseWarmup, Score: map[string]int{content.Vibe: 0, content.Artisan: 0}}
}

func (s *Server) cvarF(name string, def float64) float64 {
	v, err := strconv.ParseFloat(s.cvars[name], 64)
	if err != nil || v < 0 {
		return def
	}
	return v
}

func secs(v float64) time.Duration { return time.Duration(v * float64(time.Second)) }

// roundsOn: the objective game runs (not warmup or deathmatch). Callers hold s.mu.
func (s *Server) roundsOn() bool { return s.round.Phase != PhaseWarmup }

// deathmatch: mp_respawn 1, or auto (the default) on a test-harness server,
// so scenarios and cases keep timed respawns unless they set mp_respawn 0.
func (s *Server) deathmatch() bool {
	v := s.cvars["mp_respawn"]
	return v == "1" || (v != "0" && s.Test)
}

// timedRespawn: the dead come back after mp_respawntime (warmup, deathmatch).
func (s *Server) timedRespawn() bool { return !s.roundsOn() }

// frozen: freeze time, nobody moves or shoots.
func (s *Server) frozen() bool { return s.round.Phase == PhaseFreeze }

// fighters are the players on a side (bots and spectators aren't).
func (s *Server) fighters(team string) (all, alive int) {
	for _, p := range s.players {
		if p.Bot || !p.ready || p.Team != team {
			continue
		}
		all++
		if !p.Dead {
			alive++
		}
	}
	return
}

func (s *Server) sendAll(v any) {
	b, _ := json.Marshal(v)
	for _, o := range s.players {
		trySend(o, b)
	}
}

// tickRound advances the round state machine. Callers hold s.mu.
func (s *Server) tickRound(now time.Time) {
	r := &s.round
	vibes, _ := s.fighters(content.Vibe)
	arts, _ := s.fighters(content.Artisan)
	ready := vibes > 0 && arts > 0 && !s.deathmatch()
	if !ready {
		if r.Phase != PhaseWarmup {
			*r = roundState{Phase: PhaseWarmup, Score: r.Score, Number: r.Number}
			s.sendAll(roundEvent{T: "round", Ev: "warmup"})
			for _, p := range s.players {
				if p.Dead {
					p.respawnAt = now.Add(s.respawnDelay())
				}
			}
		}
		return
	}
	switch r.Phase {
	case PhaseWarmup:
		r.Score = map[string]int{content.Vibe: 0, content.Artisan: 0}
		r.Number = 0
		s.startRound(now)
	case PhaseFreeze:
		if now.After(r.endsAt) {
			r.Phase, r.endsAt = PhaseLive, now.Add(secs(s.cvarF("mp_roundtime", 115)))
			s.sendAll(roundEvent{T: "round", Ev: "live", Number: r.Number})
		}
	case PhaseLive:
		s.pickUpBomb()
		if now.After(r.endsAt) {
			s.endRound(now, content.Artisan, "time")
		} else {
			s.checkElimination(now)
		}
	case PhasePlanted:
		if now.After(r.endsAt) {
			r.Bomb.State = BombExploded
			s.sendAll(roundEvent{T: "round", Ev: "exploded", Pos: &r.Bomb.Pos, Site: r.Bomb.Site})
			s.blast(r.Bomb.Pos)
			s.endRound(now, content.Vibe, "exploded")
		} else {
			s.checkElimination(now)
		}
	case PhaseOver:
		if now.After(r.endsAt) {
			s.startRound(now)
		}
	}
	// The worker died, left or switched sides: their progress is gone.
	if w := r.Bomb.Worker; w != 0 {
		if p := s.players[w]; p == nil || p.Dead {
			r.stopWork()
		}
	}
}

func (r *roundState) stopWork() { r.workTicks, r.Bomb.Progress, r.Bomb.Worker = 0, 0, 0 }

func (s *Server) checkElimination(now time.Time) {
	_, vAlive := s.fighters(content.Vibe)
	_, aAlive := s.fighters(content.Artisan)
	switch {
	case aAlive == 0:
		s.endRound(now, content.Vibe, "elimination")
	case vAlive == 0 && s.round.Phase == PhaseLive:
		s.endRound(now, content.Artisan, "elimination")
	}
}

// startRound respawns every fighter at their side and hands the Big Diff to
// a random Vibe Coder. Callers hold s.mu.
func (s *Server) startRound(now time.Time) {
	r := &s.round
	r.Number++
	r.Phase, r.endsAt = PhaseFreeze, now.Add(secs(s.cvarF("mp_freezetime", 6)))
	r.Winner, r.Reason = "", ""
	r.Bomb = bombState{}
	r.workTicks = 0
	s.spawnNext = map[string]int{}
	var vibes []*Player
	for _, p := range s.players {
		if p.Bot || p.Team == content.Spectator || p.Team == "" {
			continue
		}
		s.respawn(p)
		if p.Team == content.Vibe && p.ready {
			vibes = append(vibes, p)
		}
	}
	if len(vibes) > 0 {
		c := vibes[rand.IntN(len(vibes))]
		if s.Test { // harness scenarios: the first Vibe Coder to join carries it
			c = slices.MinFunc(vibes, func(a, b *Player) int { return a.ID - b.ID })
		}
		r.Bomb = bombState{State: BombCarried, Carrier: c.ID, Pos: c.Pos}
	}
	s.sendAll(roundEvent{T: "round", Ev: "start", Number: r.Number, Score: r.Score, By: r.Bomb.Carrier})
}

func (s *Server) endRound(now time.Time, winner, reason string) {
	r := &s.round
	r.Score[winner]++
	r.Phase, r.endsAt = PhaseOver, now.Add(secs(s.cvarF("mp_round_restart_delay", overSecs)))
	r.Winner, r.Reason = winner, reason
	r.stopWork()
	s.sendAll(roundEvent{T: "round", Ev: "end", Winner: winner, Reason: reason, Score: r.Score, Number: r.Number})
}

// blast: the merge goes through. Everyone close to it takes damage (falling
// off with distance); the explosion's kills count as the planter's.
func (s *Server) blast(at Vec3) {
	const radius = 14.0
	var planter *Player
	for _, p := range s.players {
		if p.ID == s.round.Bomb.Carrier {
			planter = p
		}
	}
	for _, p := range s.players {
		if p.Dead || p.Bot || p.Team == content.Spectator || !p.ready {
			continue
		}
		d := math.Hypot(math.Hypot(p.Pos[0]-at[0], p.Pos[1]-at[1]), p.Pos[2]-at[2])
		if d >= radius {
			continue
		}
		dmg := int(math.Round(500 * (1 - d/radius)))
		p.HP -= dmg
		if p.HP <= 0 {
			killer := planter
			if killer == nil {
				killer = p
			}
			s.killBy(killer, p, "bigdiff", false)
		}
	}
}

// pickUpBomb: a live Vibe Coder standing on the dropped Big Diff takes it.
func (s *Server) pickUpBomb() {
	b := &s.round.Bomb
	if b.State != BombDropped {
		return
	}
	for _, p := range s.players {
		if p.Dead || p.Team != content.Vibe || !p.ready {
			continue
		}
		if math.Hypot(p.Pos[0]-b.Pos[0], p.Pos[2]-b.Pos[2]) < pickupRange && math.Abs(p.Pos[1]-b.Pos[1]) < 1.5 {
			b.State, b.Carrier = BombCarried, p.ID
			s.sendAll(roundEvent{T: "round", Ev: "pickup", By: p.ID})
			return
		}
	}
}

// dropBomb leaves the Big Diff where p is (death, team switch, disconnect).
func (s *Server) dropBomb(p *Player) {
	b := &s.round.Bomb
	if b.State == BombCarried && b.Carrier == p.ID {
		b.State, b.Carrier, b.Pos = BombDropped, 0, p.Pos
		s.sendAll(roundEvent{T: "round", Ev: "dropped", Pos: &b.Pos})
	}
}

// roundUse runs after each of p's commands: planting or defusing while +use
// is held, standing still on the ground. A command that lets go or moves
// resets that player's progress (a starved tick repeats the last command, so
// network jitter doesn't). Callers hold s.mu.
func (s *Server) roundUse(p *Player, c Cmd) {
	r := &s.round
	b := &r.Bomb
	still := c.B&BtnUse != 0 && !p.Dead && p.body != nil && c.F == 0 && c.R == 0 &&
		p.body.OnGround && math.Hypot(p.body.Vel[0], p.body.Vel[2]) < 0.5
	if !still {
		if b.Worker == p.ID {
			r.stopWork()
		}
		return
	}
	defer func() { // used, but not on anything: no progress either
		if b.Worker == p.ID && !s.worked {
			r.stopWork()
		}
		s.worked = false
	}()
	switch {
	case r.Phase == PhaseLive && b.State == BombCarried && b.Carrier == p.ID:
		site := ""
		for _, st := range s.Map.Sites {
			if st.Contains([3]float64(p.Pos)) {
				site = st.Name
			}
		}
		if site == "" {
			return
		}
		s.work(p, plantSecs, func() {
			b.State, b.Pos, b.Site, b.Carrier = BombPlanted, p.Pos, site, p.ID
			r.Phase, r.endsAt = PhasePlanted, s.now.Add(secs(s.cvarF("mp_c4timer", 40)))
			s.sendAll(roundEvent{T: "round", Ev: "planted", Site: site, Pos: &b.Pos, By: p.ID})
		})
	case r.Phase == PhasePlanted && p.Team == content.Artisan:
		if math.Hypot(p.Pos[0]-b.Pos[0], p.Pos[2]-b.Pos[2]) > defuseRange || math.Abs(p.Pos[1]-b.Pos[1]) > 1.5 {
			return
		}
		if b.Worker != 0 && b.Worker != p.ID {
			return // someone else is already reverting it
		}
		s.work(p, defuseSecs, func() {
			b.State = BombDefused
			s.sendAll(roundEvent{T: "round", Ev: "defused", By: p.ID, Pos: &b.Pos})
			s.endRound(s.now, content.Artisan, "defused")
		})
	}
}

// work advances p's plant/defuse by one tick and calls done when complete.
func (s *Server) work(p *Player, total float64, done func()) {
	r := &s.round
	if r.Bomb.Worker != p.ID {
		r.workTicks = 0
	}
	r.Bomb.Worker = p.ID
	r.workTicks++
	s.worked = true
	r.Bomb.Progress = math.Min(1, float64(r.workTicks)/(total*SimHz))
	if r.Bomb.Progress >= 1 {
		r.stopWork()
		done()
	}
}

// roundJSON is the round state for snapshots. Callers hold s.mu.
func (s *Server) roundJSON(now time.Time) []byte {
	r := &s.round
	if r.Bomb.State == BombCarried {
		for _, p := range s.players {
			if p.ID == r.Bomb.Carrier {
				r.Bomb.Pos = p.Pos
			}
		}
	}
	left := 0.0
	if r.Phase != PhaseWarmup {
		left = math.Max(0, r.endsAt.Sub(now).Seconds())
	}
	b, _ := json.Marshal(roundMsg{roundState: r, Left: math.Round(left*100) / 100})
	return b
}
