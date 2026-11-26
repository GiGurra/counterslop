package game

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// ticketServer: tickets mode on lab_conquest, one player on Earth (vibe)
// and one on the Moon (artisan), the round live (no freeze).
func ticketServer(t *testing.T) (*Server, *Player, *Player, *time.Time) {
	t.Helper()
	s, v, a := econServer(t)
	s.Weapons = registry(t)
	s.Map = maps.LabConquest()
	s.round = newRound()
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_respawn"] = "0"
	s.cvars["mp_tickets"] = "20"
	now := time.Now()
	for _, p := range []*Player{v, a} {
		s.queueCmds(p, []Cmd{{Seq: 1}})
	}
	s.simTick(now)
	ticks(s, 3, &now, nil)
	if s.round.Phase != PhaseLive {
		t.Fatalf("phase %q, want live", s.round.Phase)
	}
	return s, v, a, &now
}

// owners is every point's owner in map order ("-" neutral).
func owners(s *Server) string {
	var out []string
	for _, p := range s.round.Points {
		o := p.Own
		if o == "" {
			o = "-"
		}
		out = append(out, o)
	}
	return strings.Join(out, " ")
}

func TestTicketsRoundStart(t *testing.T) {
	s, v, a, now := ticketServer(t)
	r := &s.round
	if r.Tickets[content.Vibe] != 20 || r.Tickets[content.Artisan] != 20 || r.TicketsMax != 20 || r.MaxRounds != 1 {
		t.Fatalf("tickets %v of %d, max rounds %d", r.Tickets, r.TicketsMax, r.MaxRounds)
	}
	if got := owners(s); got != "vibe - - - - - artisan" {
		t.Fatalf("owners %q", got)
	}
	if r.Bomb.State != "" || v.inv.Slots[BombSlot] != nil {
		t.Fatal("a Big Diff in tickets mode")
	}
	// Everyone spawned at their main, with the first kit (the intern's rifle).
	if !s.Map.Points[0].In([3]float64(v.Pos)) || !s.Map.Points[6].In([3]float64(a.Pos)) {
		t.Fatalf("spawns %v %v, want the mains", v.Pos, a.Pos)
	}
	if v.inv.Slots["primary"].ID != "prompt" || a.inv.Slots["primary"].ID != "linter" || v.inv.Slots["smoke"] == nil || v.inv.Current != "primary" {
		t.Fatalf("kit loadouts %+v / %+v", v.inv.Slots, a.inv.Slots)
	}
	if ok, why := s.canBuy(v); ok || !strings.Contains(why, "kit") {
		t.Fatalf("canBuy %v %q", ok, why)
	}
	var msg map[string]any
	json.Unmarshal(s.roundJSON(*now), &msg)
	if msg["mode"] != "tickets" || len(msg["pts"].([]any)) != 7 || msg["tk"].(map[string]any)["vibe"] != 20.0 {
		t.Fatalf("round JSON %v", msg)
	}
	// About 20 minutes on the clock.
	if l := s.roundMsgOf(*now).Left; l < ticketRoundSecs-1 || l > ticketRoundSecs {
		t.Fatalf("left %v", l)
	}
}

func TestTicketLossOnDeathAndRespawnDelay(t *testing.T) {
	s, v, a, now := ticketServer(t)
	s.cvars["mp_respawn_delay"] = "2"
	s.killBy(a, v, "linter", false)
	if s.round.Tickets[content.Vibe] != 19 || s.round.Tickets[content.Artisan] != 20 {
		t.Fatalf("tickets %v after an Earth death", s.round.Tickets)
	}
	if left := s.respawnLeft(v); left < 1.99 || left > 2 {
		t.Fatalf("respawn in %v s, want 2", left)
	}
	if me := s.meStateOf(v); me.Respawn < 1.99 || me.SpawnAt != "earth" {
		t.Fatalf("self-snapshot respawn %v at %q", me.Respawn, me.SpawnAt)
	}
	ticks(s, 2*SimHz-2, now, nil)
	if !v.Dead {
		t.Fatal("respawned before mp_respawn_delay")
	}
	ticks(s, 3, now, nil)
	if v.Dead || v.HP != maxHP || !s.Map.Points[0].In([3]float64(v.Pos)) {
		t.Fatalf("after 2 s: dead %v hp %d at %v, want alive at Earth's main", v.Dead, v.HP, v.Pos)
	}
	if s.round.Tickets[content.Vibe] != 19 {
		t.Fatalf("the respawn cost again: %v", s.round.Tickets)
	}
	// Warmup deaths cost nothing.
	s.round.Phase = PhaseWarmup
	s.killBy(a, v, "linter", false)
	if s.round.Tickets[content.Vibe] != 19 {
		t.Fatalf("a warmup death closed a ticket: %v", s.round.Tickets)
	}
}

// step is the capture rule on its own: one player up fills a bar in
// captureSecs, two in half that, four no faster than three; equal numbers hold
// it; an enemy point is neutralised first, then captured.
func TestCaptureStep(t *testing.T) {
	run := func(st *pointState, n [2]int, ticks int) (evs []string) {
		for range ticks {
			if ev, team, _ := st.step(n); ev != "" {
				evs = append(evs, ev+":"+team)
			}
		}
		return
	}
	st := &pointState{}
	if ev := run(st, [2]int{1, 0}, captureTicks-1); len(ev) != 0 || st.By != content.Vibe || st.Pr < 0.99 {
		t.Fatalf("one Earth player, 9.99 s: %v %+v", ev, st)
	}
	if ev := run(st, [2]int{1, 0}, 1); len(ev) != 1 || ev[0] != "captured:vibe" || st.Own != content.Vibe || st.Pr != 0 || st.By != "" {
		t.Fatalf("10 s: %v %+v", ev, st)
	}
	// Contested: equal numbers, nothing moves.
	st = &pointState{Own: content.Vibe}
	run(st, [2]int{0, 1}, 300)
	pr := st.Pr
	if run(st, [2]int{2, 2}, 500); st.Pr != pr || st.Own != content.Vibe {
		t.Fatalf("contested moved it: %+v (was %v)", st, pr)
	}
	// Two Moon players up: the rest of the neutralising bar in half the time.
	if ev := run(st, [2]int{0, 2}, (captureTicks-300)/2); len(ev) != 1 || ev[0] != "neutral:artisan" || st.Own != "" || st.By != content.Artisan {
		t.Fatalf("neutralised: %v %+v", ev, st)
	}
	// Five up go at captureMaxRate: a third of the time.
	if ev := run(st, [2]int{0, 5}, captureTicks/3+1); len(ev) != 1 || ev[0] != "captured:artisan" || st.Own != content.Artisan {
		t.Fatalf("captured at 3x: %v %+v", ev, st)
	}
	// A defender undoes the attacker's bar; an empty point drains it.
	st = &pointState{Own: content.Artisan}
	run(st, [2]int{1, 0}, 400)
	run(st, [2]int{0, 1}, 100)
	if st.cnt != 300 || st.By != content.Vibe {
		t.Fatalf("defended: %+v", st)
	}
	run(st, [2]int{}, 300)
	if st.cnt != 0 || st.By != "" || st.Pr != 0 {
		t.Fatalf("drained: %+v", st)
	}
	// Attackers on a bar the other side started undo it before theirs fills.
	st = &pointState{}
	run(st, [2]int{1, 0}, 200)
	run(st, [2]int{0, 1}, 250)
	if st.By != content.Artisan || st.cnt != 50 {
		t.Fatalf("turned over: %+v", st)
	}
}

func TestCaptureOnTheMapAndLockedMains(t *testing.T) {
	s, v, a, now := ticketServer(t)
	c := s.Map.Points[3] // C, the middle
	at(s, v, c.Pos[0], c.Pos[2])
	ticks(s, captureTicks+2, now, nil)
	if got := owners(s); got != "vibe - - vibe - - artisan" {
		t.Fatalf("owners %q after 10 s on C", got)
	}
	if st := s.round.Points[3]; st.V != 1 || st.A != 0 || st.Cont {
		t.Fatalf("C counts %+v", st)
	}
	if v.Points < scoreObjective {
		t.Fatalf("capture scored %d", v.Points)
	}
	// Both on C: contested.
	at(s, a, c.Pos[0]+1, c.Pos[2])
	ticks(s, 2, now, nil)
	if st := s.round.Points[3]; !st.Cont || st.V != 1 || st.A != 1 {
		t.Fatalf("contested C %+v", st)
	}
	// The Moon's main can't be taken.
	m := s.Map.Points[6]
	at(s, a, c.Pos[0], c.Pos[2]+20)
	at(s, v, m.Pos[0], m.Pos[2])
	ticks(s, 3*captureTicks, now, nil)
	if st := s.round.Points[6]; st.Own != content.Artisan || st.Pr != 0 || st.V != 1 {
		t.Fatalf("the Moon's main %+v", st)
	}
}

// Bleed: holding more points bleeds the other side a ticket every bleedSecs
// per point of difference; equal holdings bleed nobody.
func TestTicketBleed(t *testing.T) {
	s, _, _, now := ticketServer(t)
	r := &s.round
	ticks(s, 10*SimHz, now, nil)
	if r.Tickets[content.Vibe] != 20 || r.Tickets[content.Artisan] != 20 {
		t.Fatalf("equal holdings bled: %v", r.Tickets)
	}
	r.Points[1].Own, r.Points[2].Own = content.Vibe, content.Vibe // Earth 3, Moon 1: 2 up
	ticks(s, bleedTicks/2*3, now, nil)                            // 6 s: three tickets at 2 s each
	if r.Tickets[content.Artisan] != 17 || r.Tickets[content.Vibe] != 20 {
		t.Fatalf("after 6 s 2 up: %v, want the Moon at 17", r.Tickets)
	}
	r.Points[5].Own, r.Points[4].Own, r.Points[3].Own = content.Artisan, content.Artisan, content.Artisan // Moon 4, Earth 3
	ticks(s, bleedTicks, now, nil)
	if r.Tickets[content.Vibe] != 19 || r.Tickets[content.Artisan] != 17 {
		t.Fatalf("after 4 s 1 down: %v, want Earth at 19", r.Tickets)
	}
}

// A team whose backlog reaches 0 has shipped everything: it loses at once,
// and the match is that one round.
func TestTicketsWinAndMatchEnd(t *testing.T) {
	s, v, a, now := ticketServer(t)
	s.round.Tickets[content.Artisan] = 1
	s.killBy(v, a, "prompt", false)
	ticks(s, 1, now, nil)
	r := &s.round
	if r.Phase != PhaseOver || r.Winner != content.Vibe || r.Reason != "tickets" || r.Tickets[content.Artisan] != 0 {
		t.Fatalf("phase %s winner %s (%s), tickets %v", r.Phase, r.Winner, r.Reason, r.Tickets)
	}
	if !a.Dead {
		t.Fatal("respawned after the round ended")
	}
	ticks(s, int(overSecs*SimHz)+5, now, nil)
	if r.Phase != PhaseMatchOver || r.MatchWinner != content.Vibe {
		t.Fatalf("after the round: phase %s match winner %q", r.Phase, r.MatchWinner)
	}
}

func TestTicketsTimeLimit(t *testing.T) {
	s, _, _, now := ticketServer(t)
	r := &s.round
	r.Tickets[content.Vibe] = 5
	r.endsAt = now.Add(time.Second / SimHz)
	ticks(s, 2, now, nil)
	if r.Phase != PhaseOver || r.Winner != content.Artisan || r.Reason != "time" {
		t.Fatalf("time out: phase %s winner %s (%s)", r.Phase, r.Winner, r.Reason)
	}
}

// Spawn selection: the auto pick is the held spawn point nearest a point the
// team does not hold; a choice is kept while it may be used, a point with an
// enemy in it may not (a main always may), unknown ids are ignored.
func TestSpawnSelection(t *testing.T) {
	s, v, a, now := ticketServer(t)
	r := &s.round
	if got := s.autoSpawn(content.Vibe); got != 0 {
		t.Fatalf("auto at the start: %d, want Earth's main", got)
	}
	r.Points[1].Own = content.Vibe // A, the launch pad: nearer the action than the main
	if got := s.autoSpawn(content.Vibe); got != 1 {
		t.Fatalf("auto with A: %d", got)
	}
	r.Points[2].Own = content.Vibe // B, the crater: nearer still (C is 16 m off)
	if got := s.autoSpawn(content.Vibe); got != 2 {
		t.Fatalf("auto with A and B: %d", got)
	}
	s.setDeploy(v, "A", "senior")
	if v.spawnPick != "A" || v.kit != "senior" || s.deployPoint(v) != 1 {
		t.Fatalf("choice %q %q -> %d", v.spawnPick, v.kit, s.deployPoint(v))
	}
	s.setDeploy(v, "nowhere", "admiral") // unknown: ignored
	if v.spawnPick != "A" || v.kit != "senior" {
		t.Fatalf("unknown ids taken: %q %q", v.spawnPick, v.kit)
	}
	// An enemy on A: not there, the auto pick instead.
	at(s, a, s.Map.Points[1].Pos[0], s.Map.Points[1].Pos[2])
	ticks(s, 1, now, nil)
	if got := s.deployPoint(v); got != 2 {
		t.Fatalf("A contested: deploy %d, want B", got)
	}
	// A point it doesn't hold (C, no spawn anyway): the auto pick.
	s.setDeploy(v, "C", "")
	if got := s.deployPoint(v); got != 2 {
		t.Fatalf("C: deploy %d", got)
	}
	// Dead, it respawns at the choice with the chosen kit.
	at(s, a, 40, 20)
	ticks(s, 1, now, nil) // the head counts follow
	s.setDeploy(v, "A", "")
	s.cvars["mp_respawn_delay"] = "0"
	s.killBy(a, v, "linter", false)
	ticks(s, 3, now, nil)
	if v.Dead || !s.Map.Points[1].In([3]float64(v.Pos)) || v.inv.Slots["primary"].ID != "codereview" {
		t.Fatalf("respawned dead=%v at %v with %v, want at A with Code Review", v.Dead, v.Pos, v.inv.Slots["primary"])
	}
	if me := s.meStateOf(v); me.SpawnPick != "A" || me.KitID != "senior" {
		t.Fatalf("self-snapshot choice %q %q", me.SpawnPick, me.KitID)
	}
	s.setDeploy(v, "auto", "")
	if v.spawnPick != "" {
		t.Fatalf("auto: %q", v.spawnPick)
	}
	// Bots get a kit by id.
	if k := s.kitOf(&Player{ID: 3, ai: &aiBrain{}}); k == nil || k.ID != s.kits()[3%len(s.kits())].ID {
		t.Fatalf("bot kit %+v", k)
	}
}

// Every kit's weapons exist, and the default economy carries the same kits as
// economy.json.
func TestKits(t *testing.T) {
	s, _, _ := econServer(t)
	reg := registry(t)
	if len(s.kits()) < 3 {
		t.Fatalf("%d kits", len(s.kits()))
	}
	for _, k := range s.kits() {
		for _, w := range k.Weapons {
			if w != "rifle" && reg.Get(w) == nil {
				t.Errorf("kit %s: unknown weapon %q", k.ID, w)
			}
		}
	}
	a, _ := json.Marshal(s.kits())
	b, _ := json.Marshal(DefaultEconomy.Tickets.Kits)
	if string(a) != string(b) {
		t.Errorf("DefaultEconomy kits %s, economy.json %s", b, a)
	}
}

// A player joining mid-round in tickets mode respawns after the delay
// instead of waiting for the next round.
func TestTicketsLateJoin(t *testing.T) {
	s, _, _, now := ticketServer(t)
	s.cvars["mp_respawn_delay"] = "1"
	p := s.join("3.3.3.3")
	p.ready = true
	s.handle(p, inbound{T: "team", Team: content.Vibe})
	if !p.Dead {
		t.Fatal("a mid-round join spawned at once")
	}
	s.queueCmds(p, []Cmd{{Seq: 1}})
	ticks(s, SimHz+2, now, nil)
	if p.Dead {
		t.Fatal("still dead after mp_respawn_delay")
	}
}

// Earth, with more tickets left when the time runs out, wins.
func TestTicketsTimeLimitEarth(t *testing.T) {
	s, _, _, now := ticketServer(t)
	s.round.Tickets[content.Artisan] = 5
	s.round.endsAt = now.Add(time.Second / SimHz)
	ticks(s, 2, now, nil)
	if s.round.Winner != content.Vibe || s.round.Reason != "time" {
		t.Fatalf("winner %s (%s)", s.round.Winner, s.round.Reason)
	}
}

// A team holding no spawn point (its main gone in a broken map) spawns at its
// team spawns; no kits at all: the team loadout.
func TestTicketSpawnFallbacks(t *testing.T) {
	s, v, _, _ := ticketServer(t)
	s.round.Points[0].Own = ""
	if s.deployPoint(v) != -1 || s.deployAuto(v) != "" {
		t.Fatalf("deploy %d %q with nothing held", s.deployPoint(v), s.deployAuto(v))
	}
	if pos := s.ticketSpawnPos(v); !slices.Contains(s.Map.TeamSpawns[content.Vibe], [3]float64(pos)) {
		t.Fatalf("spawned at %v, want a team spawn", pos)
	}
	if s.spawnable(content.Vibe, 99) {
		t.Fatal("point 99 spawnable")
	}
	s.Economy = &Economy{}
	if s.kitOf(v) != nil || s.kitLoadout(v).Slots["primary"].ID != "prompt" {
		t.Fatal("no kits: want the team loadout")
	}
}

// The sim/cases round fields of the tickets mode.
func TestTicketRoundFields(t *testing.T) {
	s, v, a, now := ticketServer(t)
	at(s, v, 0, 0)
	at(s, a, 0, 1)
	ticks(s, 2, now, nil)
	s.round.Points[3].By, s.round.Points[3].Pr = content.Vibe, 0.123
	for k, want := range map[string]any{"tickets.vibe": 20, "tickets.artisan": 20, "points": "vibe - - - - - artisan",
		"point.C": "-", "point.C.pr": 0.12, "point.C.by": "vibe", "point.C.cont": true, "point.X": nil, "point.C.zz": nil} {
		if got := s.RoundField(k); got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}
	if st := s.caseState(v); st["deployAt"] != "earth" || st["respawnIn"] != 0.0 {
		t.Errorf("caseState %v", st)
	}
}

// Bots attack the nearest point not held; with every point held they defend
// the one under attack, else the one nearest the enemy's main.
func TestAITicketGoal(t *testing.T) {
	s, v, a, _ := ticketServer(t)
	bot := &Player{ID: 9, Team: content.Vibe, Pos: Vec3{-50, 0, 0}, ai: &aiBrain{}}
	in := func(g Vec3, id string) bool { return s.Map.Points[s.pointIndex(id)].In([3]float64(g)) }
	if g, k, ok := s.aiTicketGoal(bot); !ok || !in(g, "A") || !strings.HasPrefix(k, "pt:A:") {
		t.Fatalf("attack: %v %q %v, want A", g, k, ok)
	}
	for i := 1; i <= 5; i++ {
		s.round.Points[i].Own = content.Vibe
	}
	if g, _, _ := s.aiTicketGoal(bot); !in(g, "E") {
		t.Fatalf("all held: goal %v, want E (nearest the Moon)", g)
	}
	s.round.Points[2].A = 1 // a Moon player in B
	if g, _, _ := s.aiTicketGoal(bot); !in(g, "B") {
		t.Fatalf("B under attack: goal %v", g)
	}
	s.round.Points[2].A, s.round.Points[4].By = 0, content.Artisan // the Moon's bar on D
	if g, _, _ := s.aiTicketGoal(bot); !in(g, "D") {
		t.Fatalf("D being taken: goal %v", g)
	}
	abot := &Player{ID: 8, Team: content.Artisan, Pos: Vec3{50, 0, 0}, ai: &aiBrain{}}
	s.round.Points[5].V = 1
	if g, _, _ := s.aiTicketGoal(abot); !in(g, "E") {
		t.Fatalf("Moon bot: goal %v, want E (nearest not held)", g)
	}
	s.Map.Points = nil
	if _, _, ok := s.aiTicketGoal(bot); ok {
		t.Fatal("a goal without points")
	}
	_, _ = v, a
}
