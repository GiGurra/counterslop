package game

import (
	"encoding/json"
	"math"
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
	overSecs    = 7.0
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
	State   string `json:"st,omitempty" net:"cold"`
	Carrier int    `json:"by,omitempty" net:"cold"` // carrier while carried; planter / defuser while working on it
	Pos     Vec3   `json:"p" net:"hot,mm"`
	Site    string `json:"site,omitempty" net:"cold"`
	// Progress of the plant (carried) or defuse (planted), 0..1, and who.
	Progress float64 `json:"pr,omitempty" net:"hot,exact"`
	Worker   int     `json:"who,omitempty" net:"cold"`
}

type roundState struct {
	Phase  string         `json:"ph" net:"cold"`
	Number int            `json:"n" net:"cold"`
	Score  map[string]int `json:"sc" net:"cold"`
	Bomb   bombState      `json:"bomb" net:"hot"`
	Winner string         `json:"win,omitempty" net:"cold"`
	Reason string         `json:"why,omitempty" net:"cold"`
	// GPU Drop (gpudrop.go): the map's mode ("gpudrop"; left out in
	// defusal), every GPU, how many are secured and how many win the round.
	Mode    string     `json:"mode,omitempty" net:"cold"`
	GPUs    []gpuState `json:"gpus,omitempty" net:"hot"`
	Secured int        `json:"secured,omitempty" net:"cold"`
	ToWin   int        `json:"toWin,omitempty" net:"cold"`
	MVP     int        `json:"mvp,omitempty" net:"cold"`    // the round's MVP (mvp.go), once it is over
	MVPWhy  string     `json:"mvpWhy,omitempty" net:"cold"` // why: "2 eliminations", "reverted The Big Diff", ...
	// Match (match.go): rounds to play, the overtime round n is in, played
	// rounds, whether the sides are swapped, and the match result once over.
	MaxRounds   int           `json:"max" net:"cold"`
	Overtime    int           `json:"ot,omitempty" net:"cold"`
	History     []roundResult `json:"hist" net:"cold"`
	Swapped     bool          `json:"sw,omitempty" net:"cold"`
	MatchWinner string        `json:"mw,omitempty" net:"cold"`

	pistol    bool      `net:"-"` // the next round starts fresh (after halftime): no weapons carried over
	endsAt    time.Time `net:"-"`
	buyUntil  time.Time `net:"-"` // mp_buytime after the round start
	workTicks int       `net:"-"` // plant or defuse progress in commands (sim ticks)
}

// roundMsg is the round state in every snapshot (left = seconds in the phase).
type roundMsg struct {
	*roundState `net:"cold"`
	Left        float64 `json:"left" net:"hot,exact"`
	// Buy: seconds of buy time left in freeze and live (the HUD's buy icon and
	// the buy menu count it down); absent in warmup (no limit) and once over.
	Buy float64 `json:"buy,omitempty" net:"hot,exact"`
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

// cvarF is a numeric cvar in force (Server.cvar: the map may pin it), or def
// when it is not a number or negative.
func (s *Server) cvarF(name string, def float64) float64 {
	v, err := strconv.ParseFloat(s.cvar(name), 64)
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
	for _, p := range s.ordered() {
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
	for _, o := range s.ordered() {
		trySend(o, b)
	}
	s.tvEventOut(b)
}

// tickRound advances the round state machine. Callers hold s.mu.
func (s *Server) tickRound(now time.Time) {
	r := &s.round
	vibes, _ := s.fighters(content.Vibe)
	arts, _ := s.fighters(content.Artisan)
	ready := vibes > 0 && arts > 0 && !s.deathmatch()
	if !ready {
		if r.Phase != PhaseWarmup {
			*r = newRound() // the match is off: warmup until both sides have players again
			s.sendAll(roundEvent{T: "round", Ev: "warmup"})
			for _, p := range s.ordered() {
				if p.Dead {
					p.respawnAt = now.Add(s.respawnDelay())
				}
			}
			s.syncBombSlots() // no round, no Big Diff
			s.syncCarry()     // nor GPUs
		}
		return
	}
	switch r.Phase {
	case PhaseWarmup:
		s.warmupTick(now)
	case PhaseFreeze:
		if now.After(r.endsAt) {
			r.Phase, r.endsAt = PhaseLive, now.Add(secs(s.cvarF("mp_roundtime", 115)))
			s.sendAll(roundEvent{T: "round", Ev: "live", Number: r.Number})
		}
	case PhaseLive:
		s.pickUpBomb()
		if s.gpuMode() {
			s.tickGPUs() // may secure the winning GPU (the round is over then)
		}
		if r.Phase != PhaseLive {
			break
		}
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
			s.afterRound(now)
		}
	case PhaseHalftime:
		if now.After(r.endsAt) {
			s.endHalftime(now)
		}
	case PhaseMatchOver:
		if now.After(r.endsAt) {
			s.restartMatch(now)
		}
	}
	s.syncBombSlots() // round changes (new round, warmup, rounds off) settle the bomb slot too
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
	if r.Number == 1 { // the match starts (from warmup): everyone gets the start money
		s.resetMoney()
	}
	r.MaxRounds = s.maxRounds()
	r.Overtime = overtimeOf(r.Number, r.MaxRounds, s.otRounds())
	r.Swapped = s.sidesSwapped
	r.Phase, r.endsAt = PhaseFreeze, now.Add(secs(s.cvarF("mp_freezetime", 6)))
	r.buyUntil = r.endsAt.Add(secs(s.cvarF("mp_buytime", 20)))
	r.Winner, r.Reason = "", ""
	r.Bomb = bombState{}
	r.workTicks = 0
	s.nades = nil    // no smoke carries over into the next round
	s.clearDropped() // nor guns on the floor (warmup buys included)
	s.spawnNext = map[string]int{}
	s.moveAFK()
	s.balanceTeams()
	s.placeAll(s.ordered())
	var vibes []*Player
	for _, p := range s.ordered() {
		if p.Bot || p.Team == content.Spectator || p.Team == "" {
			continue
		}
		keep := s.carryOver(p) // survivors keep their weapons and armor (CS)
		s.respawn(p)
		p.playedFrom = r.Number
		s.restoreCarried(p, keep)
		if p.Team == content.Vibe && p.ready {
			vibes = append(vibes, p)
		}
	}
	r.pistol = false
	s.resetGPUs()                       // GPU Drop: every GPU back on its shelf
	if len(vibes) > 0 && !s.gpuMode() { // GPU Drop has no Big Diff
		c := vibes[s.rng.IntN(len(vibes))]
		if s.Test { // harness scenarios: the first Vibe Coder to say hello (join) carries it
			c = slices.MinFunc(vibes, func(a, b *Player) int { return a.helloSeq - b.helloSeq })
		}
		r.Bomb = bombState{State: BombCarried, Carrier: c.ID, Pos: c.Pos}
	}
	s.syncBombSlots() // the carrier gets it in slot 5; a survivor's carried-over one goes
	s.tvRoundStart()  // a new replay file, which starts with this event
	s.sendAll(roundEvent{T: "round", Ev: "start", Number: r.Number, Score: r.Score, By: r.Bomb.Carrier})
}

func (s *Server) endRound(now time.Time, winner, reason string) {
	r := &s.round
	s.decideMVP(winner, reason) // first: the defuser is still the bomb's worker
	s.countIdle()
	r.Score[winner]++
	r.History = append(r.History, roundResult{Winner: winner, Reason: reason})
	// planted: the Vibe Coders got the objective going (the Big Diff down, or a
	// GPU secured): no "saved for free" rule, and the planted-loss bonus.
	s.moneyOnRoundEnd(winner, reason, r.Bomb.State == BombPlanted || r.Bomb.State == BombDefused || reason == "exploded" || r.Secured > 0)
	r.Phase, r.endsAt = PhaseOver, now.Add(secs(s.cvarF("mp_round_restart_delay", overSecs)))
	r.Winner, r.Reason = winner, reason
	r.stopWork()
	s.sendAll(roundEvent{T: "round", Ev: "end", Winner: winner, Reason: reason, Score: r.Score, Number: r.Number})
	s.sendDamageReports()
}

// blast: the merge goes through. Everyone close to it takes damage (falling
// off with distance); the explosion's kills count as the planter's.
func (s *Server) blast(at Vec3) {
	const radius = 14.0
	var planter *Player
	for _, p := range s.ordered() {
		if p.ID == s.round.Bomb.Carrier {
			planter = p
		}
	}
	for _, p := range s.ordered() {
		if p.Dead || p.Bot || p.Team == content.Spectator || !p.ready {
			continue
		}
		d := math.Hypot(math.Hypot(p.Pos[0]-at[0], p.Pos[1]-at[1]), p.Pos[2]-at[2])
		if d >= radius {
			continue
		}
		dmg := int(math.Round(500 * (1 - d/radius)))
		s.noteDamage(planter, p, dmg, p.HP)
		p.HP -= dmg
		if p.HP <= 0 {
			var killer *Player // a world kill: no credit, no penalty...
			if planter != nil && planter.Team != p.Team {
				killer = planter // ...except an enemy, who counts as the planter's (CS)
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
	for _, p := range s.ordered() {
		if p.Dead || p.Team != content.Vibe || !p.ready {
			continue
		}
		if math.Hypot(p.Pos[0]-b.Pos[0], p.Pos[2]-b.Pos[2]) < pickupRange && math.Abs(p.Pos[1]-b.Pos[1]) < 1.5 {
			b.State, b.Carrier = BombCarried, p.ID
			s.sendAll(roundEvent{T: "round", Ev: "pickup", By: p.ID})
			s.syncBombSlots() // into slot 5, not drawn
			return
		}
	}
}

// dropBomb leaves the Big Diff where p is (death, team switch, disconnect).
// The same goes for a GPU they carry (GPU Drop).
func (s *Server) dropBomb(p *Player) {
	s.dropBombAt(p, p.Pos)
	s.dropGPUAt(p, p.Pos)
}

// dropBombAt: p's Big Diff goes to the ground at pos (dropBomb, or G with it drawn).
func (s *Server) dropBombAt(p *Player, pos Vec3) {
	b := &s.round.Bomb
	if b.State == BombCarried && b.Carrier == p.ID {
		b.State, b.Carrier, b.Pos = BombDropped, 0, pos
		s.sendAll(roundEvent{T: "round", Ev: "dropped", Pos: &b.Pos})
		s.syncBombSlots()
	}
}

// syncBombSlots keeps the inventories in step with the round: the carrier has
// The Big Diff in slot 5 (not drawn when it arrives), nobody else has it. When
// it leaves the hand that held it (planted, dropped, round over), that player
// goes back to the slot they held before it, or their best weapon. The round
// state is the truth; this runs on every change and each round tick. Callers
// hold s.mu.
func (s *Server) syncBombSlots() {
	b := &s.round.Bomb
	for _, p := range s.ordered() {
		if p.inv == nil || s.Weapons == nil {
			continue
		}
		should := b.State == BombCarried && b.Carrier == p.ID && !p.Dead
		has := p.inv.Slots[BombSlot] != nil
		switch {
		case should && !has:
			if h := s.Weapons.newHeld("bigdiff"); h != nil {
				p.inv.Slots[BombSlot] = h
				s.sendInv(p)
			}
		case !should && has:
			delete(p.inv.Slots, BombSlot)
			p.inv.useBack = ""
			if p.inv.Current == BombSlot {
				next := p.inv.prev
				if next == BombSlot || p.inv.Slots[next] == nil {
					next = s.Weapons.bestSlot(p.inv)
				}
				p.inv.Current = next
				p.inv.drawn(p.cmdTicks)
			}
			s.sendInv(p)
		}
	}
}

// roundUse runs after each of p's commands: planting (The Big Diff drawn,
// fire or +use held: E in a site draws it, UseDraw) or defusing (+use held), standing still on the ground. A command
// that lets go or moves resets that player's progress (a starved tick repeats
// the last command, so network jitter doesn't). Callers hold s.mu.
func (s *Server) roundUse(p *Player, c Cmd) {
	r := &s.round
	b := &r.Bomb
	planting := c.B&(BtnFire|BtnUse) != 0 && p.inv != nil && p.inv.Current == BombSlot
	defusing := c.B&BtnUse != 0
	still := (planting || defusing) && !p.Dead && p.body != nil && c.F == 0 && c.R == 0 &&
		p.body.OnGround && math.Hypot(p.body.Vel[0], p.body.Vel[2]) < 0.5
	if s.gpuMode() {
		s.gpuUse(p, still && defusing)
		return
	}
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
	case planting && r.Phase == PhaseLive && b.State == BombCarried && b.Carrier == p.ID:
		site := ""
		for _, st := range s.Map.Sites {
			if st.Contains([3]float64(p.Pos)) {
				site = st.Name
			}
		}
		if site == "" {
			return
		}
		if b.Worker != p.ID && !s.plantReachable(p.Pos) {
			return // boosted up where no defender can get: no plant (plant_reach.go)
		}
		s.work(p, plantSecs, func() {
			b.State, b.Pos, b.Site, b.Carrier = BombPlanted, p.Pos, site, p.ID
			r.Phase, r.endsAt = PhasePlanted, s.now.Add(secs(s.cvarF("mp_c4timer", 40)))
			s.sendAll(roundEvent{T: "round", Ev: "planted", Site: site, Pos: &b.Pos, By: p.ID})
			s.moneyOnPlant(p)
			p.Points += scoreObjective
			s.syncBombSlots() // out of the hand: back to the previous weapon
		})
	case defusing && s.canDefuse(p):
		if b.Worker != 0 && b.Worker != p.ID {
			return // someone else is already reverting it
		}
		total := defuseSecs
		if p.Kit {
			total /= 2 // the Rollback Kit
		}
		s.work(p, total, func() {
			b.State = BombDefused
			s.sendAll(roundEvent{T: "round", Ev: "defused", By: p.ID, Pos: &b.Pos})
			s.moneyOnDefuse(p)
			p.Points += scoreObjective
			s.endRound(s.now, content.Artisan, "defused")
		})
	}
}

// canDefuse: p is an Artisan next to the planted Big Diff (E defuses it,
// before taking a floor weapon). Callers hold s.mu.
func (s *Server) canDefuse(p *Player) bool {
	b := &s.round.Bomb
	return s.round.Phase == PhasePlanted && p.Team == content.Artisan && !s.gpuMode() &&
		math.Hypot(p.Pos[0]-b.Pos[0], p.Pos[2]-b.Pos[2]) <= defuseRange && math.Abs(p.Pos[1]-b.Pos[1]) <= 1.5
}

// inSite: feet at pos stand in one of the map's bomb sites.
func (s *Server) inSite(pos Vec3) bool {
	for _, st := range s.Map.Sites {
		if st.Contains([3]float64(pos)) {
			return true
		}
	}
	return false
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
		done() // first: the round end still sees who did it (the MVP)
		r.stopWork()
	}
}

// roundJSON is the round state for snapshots. Callers hold s.mu.
func (s *Server) roundJSON(now time.Time) []byte {
	b, _ := json.Marshal(s.roundMsgOf(now))
	return b
}

// roundMsgOf is the round state for snapshots, as of now. Callers hold s.mu.
func (s *Server) roundMsgOf(now time.Time) roundMsg {
	r := &s.round
	r.Mode = s.Map.Mode
	for i := range r.GPUs {
		if g := &r.GPUs[i]; g.State == GPUCarried {
			if p := s.players[g.Carrier]; p != nil {
				g.Pos = p.Pos
			}
		}
	}
	if r.Bomb.State == BombCarried {
		for _, p := range s.ordered() {
			if p.ID == r.Bomb.Carrier {
				r.Bomb.Pos = p.Pos
			}
		}
	}
	left := 0.0
	if !r.endsAt.IsZero() { // every phase with a timer, warmup included
		left = math.Max(0, r.endsAt.Sub(now).Seconds())
	}
	buy := 0.0
	if r.Phase == PhaseFreeze || r.Phase == PhaseLive {
		buy = math.Max(0, r.buyUntil.Sub(now).Seconds())
	}
	return roundMsg{roundState: r, Left: math.Round(left*100) / 100, Buy: math.Round(buy*100) / 100}
}
