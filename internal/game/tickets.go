package game

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// Tickets (Map.Mode "tickets", Star Slop phase 1; docs/starslop.md): Earth
// (the Vibe Coders) against the Moon (the Artisans), conquest style. The
// player-facing name for a team's tickets is its BACKLOG.
//
//   - Each team starts the round with mp_tickets tickets (a map may pin it).
//     Every death of one of its players closes one of them.
//   - A team at 0 tickets has shipped everything and loses at once (the round
//     ends that tick; the match is that one round).
//   - The dead respawn after mp_respawn_delay seconds, at an owned spawn
//     point of their choice ({t:"deploy", pt}; "" = auto: the owned spawn
//     point nearest the action), with the kit of their choice (kit).
//   - Capture points (Map.Points): more living players of one team than the
//     other in a point move its bar. An enemy point is neutralised first
//     (one bar), then captured (a second bar); each bar takes captureSecs with
//     one player more than the enemy, a half or a third of it with two or
//     three more. Equal numbers (both there) contest it: nothing moves. An
//     empty point's partial bar drains at one player's speed. Locked points
//     (the mains) never change hands.
//   - Bleed: a team holding more points than the other bleeds the other one
//     ticket every bleedSecs per point of difference.
//   - The round has a long time limit (ticketRoundSecs): then the team with
//     more tickets wins (a tie: the Moon, who held).
//
// Everything is in the round state of every snapshot (tickets, points with
// their owner, bar and head counts) and the self-snapshot (spawn choice, kit,
// respawn countdown); {t:"point"} events are optional extras for the feed.

const (
	defaultTickets     = 150
	defaultRespawnSecs = 5.0
	captureSecs        = 10.0 // one bar, one player up
	captureMaxRate     = 3    // players up that still speed it up
	bleedSecs          = 4.0  // one ticket per point of difference
	ticketRoundSecs    = 1200 // 20 minutes
	ticketFreezeSecs   = 5.0  // freeze time at most (no buying to do)
)

var (
	captureTicks = int(captureSecs * SimHz)
	bleedTicks   = int(bleedSecs * SimHz)
)

// pointState is one capture point in the round state, in map order.
type pointState struct {
	ID string `json:"id" net:"cold"`
	// Own: who holds it ("" neutral); By: the team whose bar is filling
	// (neutralising or capturing it), Pr that bar, 0..1.
	Own string  `json:"own,omitempty" net:"cold"`
	By  string  `json:"by,omitempty" net:"cold"`
	Pr  float64 `json:"pr,omitempty" net:"hot,exact"`
	// Living players of each side in it, and whether both sides are (contested).
	V    int  `json:"v,omitempty" net:"cold"`
	A    int  `json:"a,omitempty" net:"cold"`
	Cont bool `json:"cont,omitempty" net:"cold"`
	cnt  int  `net:"-"` // the bar in capture units (captureTicks: full)
}

// pointEvent is sent when a point is neutralised or captured (optional).
type pointEvent struct {
	T    string `json:"t"`  // "point"
	Ev   string `json:"ev"` // "neutral", "captured"
	ID   string `json:"id"`
	Team string `json:"team"` // who did it
	From string `json:"from,omitempty"`
}

// ticketEvent: a "tickets please" gate (Map.TicketGates) scanned player ID's
// respawn ticket (once per life); Left is its team's backlog. The client
// plays the conductor's line (an event: optional, a gag).
type ticketEvent struct {
	T    string `json:"t"` // "ticket"
	ID   int    `json:"id"`
	Team string `json:"team"`
	Left int    `json:"left"`
}

// tickGates scans the ticket of every living player who walks into a ticket
// gate, once per life. Callers hold s.mu.
func (s *Server) tickGates() {
	if len(s.Map.TicketGates) == 0 {
		return
	}
	for _, p := range s.ordered() {
		if p.Dead || p.Team == content.Spectator {
			p.scanned = false
			continue
		}
		if p.scanned {
			continue
		}
		for _, g := range s.Map.TicketGates {
			if p.swept(func(a, b Vec3) bool { return g.Crossed([3]float64(a), [3]float64(b)) }) { // walked through it this tick (sweep.go)
				p.scanned = true
				s.sendAll(ticketEvent{T: "ticket", ID: p.ID, Team: p.Team, Left: s.round.Tickets[p.Team]})
				break
			}
		}
	}
}

// Kit is a tickets-mode loadout (economy.json "tickets.kits"); "rifle" in
// Weapons is the team's rifle (the Prompt or the Linter).
type Kit struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Weapons []string `json:"weapons"`
}

// ticketMode: this map plays tickets. Callers hold s.mu.
func (s *Server) ticketMode() bool { return s.Map.Mode == content.ModeTickets }

// ticketsLive: the tickets game runs (a round, not warmup). Callers hold s.mu.
func (s *Server) ticketsLive() bool { return s.ticketMode() && s.roundsOn() }

// startTickets is the tickets-mode half of startRound: fresh tickets, the
// points back to their owners.
func (s *Server) startTickets() {
	r := &s.round
	r.Tickets, r.TicketsMax, r.Points, r.bleed = nil, 0, nil, [2]int{}
	if !s.ticketMode() {
		return
	}
	n := defaultTickets
	if v, err := strconv.Atoi(s.cvar("mp_tickets")); err == nil && v > 0 {
		n = v
	}
	r.Tickets = map[string]int{content.Vibe: n, content.Artisan: n}
	r.TicketsMax = n
	for _, p := range s.Map.Points {
		r.Points = append(r.Points, pointState{ID: p.ID, Own: p.Owner})
	}
}

// tickTickets runs every live round tick in tickets mode: the points, the
// bleed, and the end (a backlog at 0, or the time). Callers hold s.mu.
func (s *Server) tickTickets(now time.Time) {
	r := &s.round
	s.tickPoints()
	held := [2]int{}
	for i, p := range r.Points {
		if t := teamIdx(p.Own); t >= 0 && i < len(s.Map.Points) {
			held[t]++
		}
	}
	if d := held[0] - held[1]; d != 0 {
		loser := 1
		if d < 0 {
			loser, d = 0, -d
		}
		r.bleed[loser] += d
		for r.bleed[loser] >= bleedTicks {
			r.bleed[loser] -= bleedTicks
			s.closeTicket(content.Teams[loser])
		}
	}
	switch {
	case r.Tickets[content.Vibe] <= 0:
		s.endRound(now, content.Artisan, "tickets")
	case r.Tickets[content.Artisan] <= 0:
		s.endRound(now, content.Vibe, "tickets")
	case now.After(r.endsAt):
		w := content.Artisan
		if r.Tickets[content.Vibe] > r.Tickets[content.Artisan] {
			w = content.Vibe
		}
		s.endRound(now, w, "time")
	}
}

// teamIdx: Vibe 0, Artisan 1, anything else -1.
func teamIdx(t string) int {
	switch t {
	case content.Vibe:
		return 0
	case content.Artisan:
		return 1
	}
	return -1
}

// closeTicket takes one ticket off team's backlog (a death or the bleed).
func (s *Server) closeTicket(team string) {
	if r := &s.round; r.Tickets != nil && r.Tickets[team] > 0 {
		r.Tickets[team]--
	}
}

// ticketDeath: a death in a live tickets round costs the victim's team a ticket.
func (s *Server) ticketDeath(victim *Player) {
	if s.ticketMode() && s.round.Phase == PhaseLive && teamIdx(victim.Team) >= 0 {
		s.closeTicket(victim.Team)
	}
}

// tickPoints counts who stands in each point and moves its bar.
func (s *Server) tickPoints() {
	r := &s.round
	for i := range r.Points {
		if i >= len(s.Map.Points) {
			break
		}
		def, st := s.Map.Points[i], &r.Points[i]
		n := [2]int{}
		for _, p := range s.ordered() {
			if p.Dead || !p.ready || p.Bot {
				continue
			}
			if t := teamIdx(p.Team); t >= 0 && def.In([3]float64(p.Pos)) {
				n[t]++
			}
		}
		st.V, st.A = n[0], n[1]
		st.Cont = n[0] > 0 && n[1] > 0
		if def.Locked {
			continue
		}
		if ev, team, from := st.step(n); ev != "" {
			s.sendAll(pointEvent{T: "point", Ev: ev, ID: st.ID, Team: team, From: from})
			for _, p := range s.ordered() { // the capturers on it score
				if !p.Dead && p.Team == team && def.In([3]float64(p.Pos)) {
					p.Points += scoreObjective
				}
			}
		}
	}
}

// step moves one point's bar by one tick with n[team] players of each side
// in it. It returns what happened ("neutral", "captured" or ""), by whom,
// and the previous owner.
func (st *pointState) step(n [2]int) (ev, team, from string) {
	lead, up := -1, 0
	switch {
	case n[0] > n[1]:
		lead, up = 0, n[0]-n[1]
	case n[1] > n[0]:
		lead, up = 1, n[1]-n[0]
	}
	defer func() { st.Pr = float64(st.cnt) / float64(captureTicks) }()
	if lead < 0 {
		if n[0] == 0 && st.cnt > 0 { // empty: the bar drains
			st.cnt--
			if st.cnt == 0 {
				st.By = ""
			}
		}
		return "", "", ""
	}
	t := content.Teams[lead]
	rate := min(up, captureMaxRate)
	if st.Own == t || (st.By != "" && st.By != t) { // defend: undo the other side's bar
		st.cnt = max(0, st.cnt-rate)
		if st.cnt == 0 {
			st.By = ""
		}
		return "", "", ""
	}
	st.By = t
	st.cnt += rate
	if st.cnt < captureTicks {
		return "", "", ""
	}
	from = st.Own
	st.cnt = 0
	if st.Own == "" {
		st.Own, st.By = t, ""
		return "captured", t, ""
	}
	st.Own = "" // neutralised: the capture bar starts next tick (By stays t)
	return "neutral", t, from
}

// spawnable: team may spawn at map point i now: a spawn point it holds, with
// no enemy in it (a main always).
func (s *Server) spawnable(team string, i int) bool {
	if i < 0 || i >= len(s.round.Points) || i >= len(s.Map.Points) {
		return false
	}
	def, st := s.Map.Points[i], s.round.Points[i]
	if !def.Spawn || st.Own != team {
		return false
	}
	enemies := st.A
	if team == content.Artisan {
		enemies = st.V
	}
	return def.Locked || enemies == 0
}

// autoSpawn is the spawn point a player of team gets without a choice: of
// the points it may spawn at, the one nearest a point it does not hold (the
// action), the main when it holds everything. -1: none.
func (s *Server) autoSpawn(team string) int {
	best, bestD := -1, math.Inf(1)
	for i, def := range s.Map.Points {
		if !s.spawnable(team, i) {
			continue
		}
		d := math.Inf(1)
		for j, o := range s.Map.Points {
			if j < len(s.round.Points) && s.round.Points[j].Own != team && !o.Locked {
				d = math.Min(d, math.Hypot(o.Pos[0]-def.Pos[0], o.Pos[2]-def.Pos[2]))
			}
		}
		if best < 0 || d < bestD || (math.IsInf(bestD, 1) && math.IsInf(d, 1) && def.Locked) {
			best, bestD = i, d
		}
	}
	return best
}

// pointIndex is the map index of point id, or -1.
func (s *Server) pointIndex(id string) int {
	for i, p := range s.Map.Points {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// deployPoint is where p spawns next: its choice if it may spawn there, else
// the auto pick. -1: nowhere (no point held: the team spawns).
func (s *Server) deployPoint(p *Player) int {
	if i := s.pointIndex(p.spawnPick); i >= 0 && s.spawnable(p.Team, i) {
		return i
	}
	return s.autoSpawn(p.Team)
}

// ticketSpawnPos: a spot at p's deploy point (clear of other players), or
// its team's spawns when it holds no spawn point.
// A point whose spots are all taken (a whole team spawning on it at a round
// start) overflows to the team's other spawn points, nearest the action
// first, its main last (a full team never shares a spot).
func (s *Server) ticketSpawnPos(p *Player) Vec3 {
	i := s.deployPoint(p)
	if i >= 0 && s.Map.Points[i].Locked && s.round.Phase == PhaseLive { // a respawn, not the opening
		if v, ok := s.spawnAboard(p); ok {
			return v
		}
	}
	if i < 0 || len(s.Map.Points[i].Spawns) == 0 {
		return s.spawnFor(p)
	}
	first, ok := s.spawnFromClear(p, "pt:"+s.Map.Points[i].ID, s.Map.Points[i].Spawns)
	if ok {
		return first
	}
	for _, j := range s.spawnOrder(p.Team) {
		if j == i || len(s.Map.Points[j].Spawns) == 0 {
			continue
		}
		if v, ok := s.spawnFromClear(p, "pt:"+s.Map.Points[j].ID, s.Map.Points[j].Spawns); ok {
			return v
		}
	}
	return first
}

// spawnOrder: team's spawnable points, nearest an enemy or neutral point
// first (autoSpawn's measure), the locked main last.
func (s *Server) spawnOrder(team string) []int {
	type pd struct {
		i      int
		d      float64
		locked bool
	}
	var out []pd
	for i, def := range s.Map.Points {
		if !s.spawnable(team, i) {
			continue
		}
		d := math.Inf(1)
		for j, o := range s.Map.Points {
			if j < len(s.round.Points) && s.round.Points[j].Own != team && !o.Locked {
				d = math.Min(d, math.Hypot(o.Pos[0]-def.Pos[0], o.Pos[2]-def.Pos[2]))
			}
		}
		out = append(out, pd{i, d, def.Locked})
	}
	slices.SortStableFunc(out, func(a, b pd) int {
		if a.locked != b.locked {
			if a.locked {
				return 1
			}
			return -1
		}
		return cmp.Compare(a.d, b.d)
	})
	idx := make([]int, len(out))
	for k, o := range out {
		idx[k] = o.i
	}
	return idx
}

// setDeploy takes a {t:"deploy"} choice: the spawn point id ("" or "auto":
// the auto pick) and the kit id ("" keeps the current one). Unknown ids are
// ignored. The choice is in every self-snapshot, so a lost message is sent
// again by the client until it shows. Callers hold s.mu.
func (s *Server) setDeploy(p *Player, pt, kit string) {
	switch {
	case pt == "auto":
		p.spawnPick = ""
	case pt != "" && s.pointIndex(pt) >= 0:
		p.spawnPick = pt
	}
	if kit != "" && s.kit(kit) != nil {
		p.kit = kit
	}
}

// kits are the tickets-mode loadouts (economy.json).
func (s *Server) kits() []Kit { return s.econ().Tickets.Kits }

// kit is the kit with this id, or nil.
func (s *Server) kit(id string) *Kit {
	ks := s.kits()
	for i := range ks {
		if ks[i].ID == id {
			return &ks[i]
		}
	}
	return nil
}

// kitOf is p's kit: its choice, else the first one (a bot: one by its id).
func (s *Server) kitOf(p *Player) *Kit {
	if k := s.kit(p.kit); k != nil {
		return k
	}
	ks := s.kits()
	if len(ks) == 0 {
		return nil
	}
	if p.ai != nil {
		return &ks[p.ID%len(ks)]
	}
	return &ks[0]
}

// kitLoadout is p's inventory on a tickets-mode spawn: its kit's weapons, the
// primary (else the sidearm) in hand.
func (s *Server) kitLoadout(p *Player) *Inventory {
	k := s.kitOf(p)
	if k == nil {
		return s.Weapons.Loadout(p.Team)
	}
	inv := &Inventory{Slots: map[string]*Held{}, Current: "melee"}
	for _, id := range k.Weapons {
		if id == "rifle" {
			id = "prompt"
			if p.Team == content.Artisan {
				id = "linter"
			}
		}
		if h := s.Weapons.newHeld(id); h != nil {
			inv.Slots[s.Weapons.Get(id).Slot] = h
		}
	}
	for _, slot := range []string{"primary", "sidearm"} {
		if inv.Slots[slot] != nil {
			inv.Current = slot
			break
		}
	}
	return inv
}

// respawnLeft: seconds until dead p respawns in a live tickets round (0 otherwise).
func (s *Server) respawnLeft(p *Player) float64 {
	if !p.Dead || !s.ticketMode() || s.round.Phase != PhaseLive {
		return 0
	}
	return math.Round(math.Max(0, p.respawnAt.Sub(s.clock()).Seconds())*100) / 100
}

// deployAuto is the id of the point p would spawn at now ("" none): the
// spawn screen marks it.
func (s *Server) deployAuto(p *Player) string {
	if !s.ticketsLive() {
		return ""
	}
	if i := s.deployPoint(p); i >= 0 {
		return s.Map.Points[i].ID
	}
	return ""
}

// lateJoin: p joined a side mid-round. In tickets mode it spawns after the
// respawn delay; otherwise it plays from the next round.
func (s *Server) lateJoin(p *Player) {
	p.Dead, p.HP = true, 0
	p.respawnAt = s.clock().Add(s.respawnDelay())
}

// aboardSpawnMargin: a ship redeploys aboard while it boards with at least
// this many seconds before its doors shut (time to find a seat).
const aboardSpawnMargin = 3.0

// spawnAboard: a spot in the aisle of a ship p's team redeploys on
// (Departure.Team) while it is docked and boarding; ok false: none is.
func (s *Server) spawnAboard(p *Player) (Vec3, bool) {
	for i := range s.Map.Departures {
		d := &s.Map.Departures[i]
		if d.Team == "" || d.Team != p.Team || !s.docked(d, aboardSpawnMargin) {
			continue
		}
		var spots [][3]float64
		for k := -5; k <= 5; k++ {
			for _, z := range []float64{-0.45, 0.45} {
				spots = append(spots, [3]float64{d.Board[0] + 1.3*float64(k), d.Board[1], d.Board[2] + z})
			}
		}
		if v, ok := s.spawnFromClear(p, "ship:"+d.Name, spots); ok {
			return v, true
		}
	}
	return Vec3{}, false
}
