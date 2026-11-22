package game

import (
	"encoding/json"
	"fmt"
	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Test-harness hooks for lockstep end-to-end runs (cmd/sim -cases): the
// harness, not Run's ticker, advances the server one tick at a time, waits for
// each client's command to arrive first, and reads the authoritative state of
// every player to assert it and compare it with the client's prediction.
// Only cmd/sim (test mode) and tests call these.

// SetLockstep stops (true) or resumes (false) Run's own ticking.
func (s *Server) SetLockstep(on bool) {
	s.mu.Lock()
	s.lockstep = on
	s.lockstepOn.Store(on)
	s.mu.Unlock()
}

// Step runs n server ticks on the game clock (10 ms each), broadcasting
// snapshots as Run does.
func (s *Server) Step(n int) {
	for range n {
		s.mu.Lock()
		snap := s.advance(s.clock().Add(time.Second / SimHz))
		s.mu.Unlock()
		if snap {
			s.broadcast()
		}
	}
}

// WaitCommand blocks until the named player has sent command seq (or timeout).
func (s *Server) WaitCommand(name string, seq int, timeout time.Duration) error {
	deadline := wallNow().Add(timeout)
	for {
		s.mu.Lock()
		p := s.byName(name)
		got, spec := 0, false
		if p != nil {
			got, spec = p.lastQueued, p.Team == content.Spectator
		}
		s.mu.Unlock()
		// A spectator's commands are dropped unqueued (queueCmds: no body), so a
		// command sent just before the server took its switch to spectators
		// never arrives: nothing to wait for.
		if got >= seq || spec {
			return nil
		}
		if wallNow().After(deadline) {
			return fmt.Errorf("%s: command %d never arrived (last %d)", name, seq, got)
		}
		time.Sleep(200 * time.Microsecond)
	}
}

// SetInventory gives the named player these slots (cur = held slot) and tells its client.
// SetBoost gives the named player an energy drink for the next n commands.
func (s *Server) SetBoost(name string, n int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.byName(name)
	if p == nil || p.inv == nil {
		return fmt.Errorf("no player %q with an inventory", name)
	}
	p.inv.boostEnd = p.cmdTicks + n
	return nil
}

func (s *Server) SetInventory(name string, slots map[string]*Held, cur string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.byName(name)
	if p == nil {
		return fmt.Errorf("no player %q", name)
	}
	inv := &Inventory{Slots: map[string]*Held{}, Current: cur}
	for k, h := range slots {
		hh := *h
		inv.Slots[k] = &hh
	}
	inv.drawn(p.cmdTicks)
	p.inv = inv
	s.sendInv(p)
	return nil
}

// CaseState is the named player's authoritative state in sim/cases terms
// (field names as web/game/cases.js; weapons observed without settling).
func (s *Server) CaseState(name string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.byName(name)
	if p == nil {
		return nil
	}
	return s.caseState(p)
}

// ItemCount is how many dropped weapons lie on the floor (map-placed ones don't count).
// NadeCount is how many grenades are live (flying, or a standing smoke).
func (s *Server) NadeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.nades)
}

func (s *Server) ItemCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, it := range s.items {
		if it.mapSlot < 0 {
			n++
		}
	}
	return n
}

// AdminCommands lists every rcon command: the admin help's commands and the cvars.
func (s *Server) AdminCommands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, sec := range adminHelp {
		for _, r := range sec.Rows {
			out = append(out, r.Name)
		}
	}
	for k := range s.cvars {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Cvar is a server cvar's value ("" if unknown).
func (s *Server) Cvar(k string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cvars[k]
}

// RoundField is one fact about the round for sim cases ("round.<k>"): phase,
// number, bomb (state), site, carrier (name), winner, reason, score.vibe /
// score.artisan; the match: max (rounds), overtime, played, swapped, matchwinner.
func (s *Server) RoundField(k string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := &s.round
	switch k {
	case "phase":
		return r.Phase
	case "number":
		return r.Number
	case "bomb":
		return r.Bomb.State
	case "site":
		return r.Bomb.Site
	case "carrier":
		for _, p := range s.players {
			if p.ID == r.Bomb.Carrier {
				return p.Name
			}
		}
		return ""
	case "mvp", "mvpWhy":
		if k == "mvpWhy" {
			return r.MVPWhy
		}
		for _, p := range s.players {
			if p.ID == r.MVP {
				return p.Name
			}
		}
		return ""
	case "winner":
		return r.Winner
	case "reason":
		return r.Reason
	case "score.vibe", "score.artisan":
		return r.Score[strings.TrimPrefix(k, "score.")]
	case "max":
		return r.MaxRounds
	case "overtime":
		return r.Overtime
	case "played":
		return len(r.History)
	case "swapped":
		return s.sidesSwapped
	case "matchwinner":
		return r.MatchWinner
	case "mode":
		return s.Map.Mode
	case "secured":
		return r.Secured
	case "toWin":
		return r.ToWin
	case "gpus": // every GPU's state, in map order: "shelf shelf carried dropped"
		st := make([]string, len(r.GPUs))
		for i, g := range r.GPUs {
			st[i] = g.State
		}
		return strings.Join(st, " ")
	}
	// gpu<n> (1-based): that GPU's state; gpu<n>.by: its carrier's (or securer's) name
	if n, by, _ := strings.Cut(strings.TrimPrefix(k, "gpu"), "."); strings.HasPrefix(k, "gpu") {
		i, err := strconv.Atoi(n)
		if err != nil || i < 1 || i > len(r.GPUs) {
			return nil
		}
		g := r.GPUs[i-1]
		if by == "" {
			return g.State
		}
		for _, p := range s.players {
			if p.ID == g.Carrier {
				return p.Name
			}
		}
		return ""
	}
	return nil
}

// PlayerCount / BotCount: humans (who said hello: a menu's hidden observer
// connection is not a player) and bots on the server.
func (s *Server) PlayerCount() (humans, bots int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.players {
		if p.Bot || p.ai != nil {
			bots++
		} else if p.ready {
			humans++
		}
	}
	return
}

// ResetWorld puts the server back to its starting state between sim
// scenarios: default cvars (told to every client), no bots, de_slop with a
// fresh match, no dropped weapons or bans. Players stay connected.
func (s *Server) ResetWorld() { _ = s.ResetWorldOn("") }

// ResetWorldOn is ResetWorld on the named map ("" = de_slop).
func (s *Server) ResetWorldOn(name string) error {
	if name == "" {
		name = "de_slop"
	}
	if maps.All[name] == nil {
		return fmt.Errorf("unknown map %q", name)
	}
	s.mu.Lock()
	s.kickBots()
	s.cvars = copyCvars()
	b := s.cvarsMsg()
	for _, o := range s.players {
		trySend(o, b)
	}
	s.changeLevel(name)                 // also a fresh match: rounds, items, spawns
	s.rng = rand.New(rand.NewPCG(1, 1)) // bots and draws replay the same in every case/scenario
	s.sidesSwapped, s.warmupEnd = false, false
	s.mu.Unlock()
	s.ClearDropped()
	return nil
}

// ForgetPoses drops the lag-compensation history: shots from now on are traced
// against poses from now on (a case's world starts at its setup, not where
// everyone stood in the case before; clients forget theirs in caseReset).
func (s *Server) ForgetPoses() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = History{}
}

// ClearDropped removes every dropped weapon from the floor and lifts every ban (a fresh case).
func (s *Server) ClearDropped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.bans)
	kept := s.items[:0]
	for _, it := range s.items {
		if it.mapSlot >= 0 {
			kept = append(kept, it)
		}
	}
	s.items = kept
}

// Sync sends the named player a {"t":"sync","n":n} marker: once the client has
// seen it, it has seen everything the server sent before (one ordered socket).
func (s *Server) Sync(name string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.byName(name); p != nil {
		b, _ := json.Marshal(map[string]any{"t": "sync", "n": n})
		trySend(p, b)
	}
}

// caseState: callers hold s.mu.
func (s *Server) caseState(p *Player) map[string]any {
	r := func(v float64) float64 { return math.Round(v*1e4) / 1e4 }
	m := map[string]any{"hp": p.HP, "dead": p.Dead, "kills": p.Kills, "deaths": p.Deaths, "points": p.Points, "name": p.Name, "team": p.Team, "money": p.Money}
	if p.body != nil {
		m["pos"] = []float64{r(p.body.Pos[0]), r(p.body.Pos[1]), r(p.body.Pos[2])}
		m["vel"] = []float64{r(p.body.Vel[0]), r(p.body.Vel[1]), r(p.body.Vel[2])}
		m["onGround"], m["crouched"] = p.body.OnGround, p.body.Crouched
	}
	if p.inv != nil {
		o := observe(s.Weapons, p.inv, p.cmdTicks)
		m["cur"], m["ammo"], m["reserve"], m["reloading"] = o.Cur, o.Ammo, o.Reserve, o.Reloading
		slots := map[string]string{}
		for k, h := range p.inv.Slots {
			slots[k] = h.ID
		}
		m["slots"] = slots
		m["spray"] = r(p.inv.recoil)
		m["carry"] = p.inv.carry
	}
	return m
}

// byName finds a human player by name, or by id written "#<id>" (survives a rename).
func (s *Server) byName(name string) *Player {
	if id, err := strconv.Atoi(strings.TrimPrefix(name, "#")); err == nil && strings.HasPrefix(name, "#") {
		for _, p := range s.players {
			if p.ID == id {
				return p
			}
		}
		return nil
	}
	for _, p := range s.players {
		if p.Name == name && !p.Bot {
			return p
		}
	}
	return nil
}

// Observed is what a player holds as of command tick `tick`, without changing
// anything: a reload whose time has come counts as done (the client settles
// every tick; the server when the gun is next used).
type Observed struct {
	Tick      int64  `json:"tick"`
	Cur       string `json:"cur"`
	Ammo      int    `json:"ammo"`
	Reserve   int    `json:"reserve"`
	Reloading bool   `json:"reloading"`
}

func observe(r *Registry, inv *Inventory, tick int64) Observed {
	o := Observed{Tick: tick, Cur: inv.Current}
	h := inv.Slots[inv.Current]
	if h == nil {
		return o
	}
	o.Ammo, o.Reserve = h.Ammo, h.Reserve
	if inv.reloadEnd != 0 {
		if tick >= inv.reloadEnd {
			w := r.Get(h.ID)
			take := min(w.Mag-h.Ammo, h.Reserve)
			o.Ammo, o.Reserve = h.Ammo+take, h.Reserve-take
		} else {
			o.Reloading = true
		}
	}
	return o
}

// Seed makes the server's randomness (bots, carrier pick, team ties) replayable.
func (s *Server) Seed(seed uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rng = rand.New(rand.NewPCG(seed, seed))
}
