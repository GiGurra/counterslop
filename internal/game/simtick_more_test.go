package game

import (
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// cmdPlayer is a command-driven player standing on open floor at pos.
func cmdPlayer(t *testing.T, s *Server, ip string, team string, pos Vec3) *Player {
	t.Helper()
	p := s.join(ip)
	p.ready, p.Team, p.Pos = true, team, pos
	s.queueCmds(p, []Cmd{{Seq: 1}})
	stepSim(s)
	msgs(p)
	return p
}

// run queues and applies one command per call.
type runner struct {
	s   *Server
	p   *Player
	seq int
}

func (r *runner) cmd(c Cmd) {
	r.seq++
	c.Seq = r.seq + 1 // seq 1 was the setup command
	r.s.queueCmds(r.p, []Cmd{c})
	stepSim(r.s)
}

func TestQueueCmdsFiltersAndClamps(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	p.Team = content.Vibe
	nan := math.NaN()
	s.queueCmds(p, []Cmd{
		{Seq: 1, F: 5, R: -5, Pitch: 3},
		{Seq: 1, F: 1}, // resend
		{Seq: 2, F: nan},
		{Seq: 3, R: math.Inf(1)},
		{Seq: 4, Yaw: nan},
		{Seq: 5, Pitch: nan},
		{Seq: 6, Pitch: -3},
		{Seq: 0},
	})
	if len(p.cmds) != 2 || p.lastQueued != 6 {
		t.Fatalf("queued %+v (last %d)", p.cmds, p.lastQueued)
	}
	if c := p.cmds[0]; c.F != 1 || c.R != -1 || c.Pitch != 1.6 {
		t.Fatalf("not clamped: %+v", c)
	}
	if p.cmds[1].Pitch != -1.6 {
		t.Fatalf("pitch %v", p.cmds[1].Pitch)
	}
}

// Test mode keeps every command (the harness steps faster than real time)
// unless the player asked for real pacing.
func TestQueueOverflowInTestMode(t *testing.T) {
	for _, paced := range []bool{false, true} {
		s := authServer(t)
		s.Test = true
		p := s.join("1.1.1.1")
		p.Team, p.paced = content.Vibe, paced
		var cmds []Cmd
		for i := 1; i <= 100; i++ {
			cmds = append(cmds, Cmd{Seq: i})
		}
		s.queueCmds(p, cmds)
		want, drop := 100, 0
		if paced {
			want, drop = maxQueued, 100-maxQueued
		}
		if len(p.cmds) != want || p.cmdsDropped != drop {
			t.Fatalf("paced=%v: %d queued, %d dropped", paced, len(p.cmds), p.cmdsDropped)
		}
		stepSim(s)
		if paced && (p.ackSeq != drop+1 || len(p.cmds) != want-1) {
			t.Fatalf("paced: ack %d, %d left after one tick", p.ackSeq, len(p.cmds))
		}
		if !paced && (p.ackSeq != 100 || len(p.cmds) != 0) {
			t.Fatalf("unpaced test mode: ack %d, %d left", p.ackSeq, len(p.cmds))
		}
	}
}

func TestSimTickBudget(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	p.Team = content.Vibe
	var cmds []Cmd
	for i := 1; i <= 60; i++ {
		cmds = append(cmds, Cmd{Seq: i})
	}
	s.queueCmds(p, cmds)
	t0 := time.Now()
	s.simTick(t0) // first tick: one tick of budget
	if p.ackSeq != 1 {
		t.Fatalf("first tick ran %d", p.ackSeq)
	}
	s.simTick(t0.Add(time.Second)) // a long stall catches up at most cmdBurst ticks
	if got := p.ackSeq - 1; float64(got) != math.Floor(cmdBurst*cmdDrift+(cmdDrift-1)) {
		t.Fatalf("after a 1 s stall ran %d commands", got)
	}
	ack := p.ackSeq
	s.simTick(t0.Add(time.Second)) // no time passed: no budget
	if p.ackSeq != ack {
		t.Fatal("ran commands with zero elapsed time")
	}
	s.simTick(t0) // clock went backwards: clamped to zero
	if p.ackSeq != ack {
		t.Fatal("ran commands with negative elapsed time")
	}
}

func TestSpread(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	p.body = sim.NewBody(sim.Vec3{})
	stub, review := s.Weapons.Get("stub"), s.Weapons.Get("codereview")
	st := stub.Spread
	for _, tc := range []struct {
		name   string
		w      *Weapon
		vel    sim.Vec3
		ground bool
		crouch bool
		scope  bool
		cvar   string
		want   float64
	}{
		{"standing", stub, sim.Vec3{}, true, false, false, "1", st.Base},
		// counter-strafed: under 34% of the Stub's max speed (0.34 × 7 × 0.96 = 2.2848 m/s) costs nothing
		{"slow (2 m/s)", stub, sim.Vec3{2, 0, 0}, true, false, false, "1", st.Base},
		{"moving 5 m/s", stub, sim.Vec3{3, 0, 4}, true, false, false, "1", st.Base + (5-0.34*7*stub.Speed)*st.Move},
		{"in the air", stub, sim.Vec3{}, false, false, false, "1", st.Base + st.Air},
		{"crouched", stub, sim.Vec3{}, true, true, false, "1", st.Base * st.Crouch},
		{"unscoped sniper", review, sim.Vec3{}, true, false, false, "1", review.Spread.Base + review.Spread.Unscoped},
		{"scoped sniper", review, sim.Vec3{}, true, false, true, "1", review.Spread.Base},
		{"weapon_spread 0", stub, sim.Vec3{3, 0, 4}, false, true, false, "0", 0},
	} {
		s.cvars["weapon_spread"] = tc.cvar
		p.body.Vel, p.body.OnGround, p.body.Crouched = tc.vel, tc.ground, tc.crouch
		c := Cmd{}
		if tc.scope {
			c.B = BtnScope
		}
		if got := s.spread(p, tc.w, c); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: spread %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRespawnDelayAndSpawnPoint(t *testing.T) {
	s := NewServer()
	for v, want := range map[string]time.Duration{"3": 3 * time.Second, "0": 0, "1.5": 1500 * time.Millisecond, "x": 3 * time.Second, "-2": 3 * time.Second} {
		s.cvars["mp_respawntime"] = v
		if got := s.respawnDelay(); got != want {
			t.Errorf("mp_respawntime %q: %v, want %v", v, got, want)
		}
	}
	// Teamless players use the general spawns by id; team players cycle their team's.
	p := s.join("1.1.1.1")
	q := s.join("2.2.2.2")
	p.Team, q.Team = "", ""
	if s.spawnPoint(p) != Vec3(s.Map.Spawns[0]) || s.spawnPoint(q) != Vec3(s.Map.Spawns[1]) {
		t.Fatal("teamless spawn points")
	}
	p.Team = content.Vibe
	vs := s.Map.TeamSpawns[content.Vibe]
	for i := range 2 * len(vs) {
		if got := s.spawnPoint(p); got != Vec3(vs[i%len(vs)]) {
			t.Fatalf("vibe spawn %d: %v", i, got)
		}
	}
	// spawnFor falls back to the general spawns for a team without its own
	// (p stood on the first one: off it).
	p.Pos = Vec3{100, 0, 100}
	if got := s.spawnFor(&Player{Team: "nobody"}); got != Vec3(s.Map.Spawns[0]) {
		t.Fatalf("spawnFor fallback %v", got)
	}
}

func TestFallingOutOfTheWorldRespawns(t *testing.T) {
	s := authServer(t)
	s.Test = true
	p := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-14, 0, 6})
	s.teleport(p, Vec3{-14, -30, 6}, 0, false, 1)
	r := &runner{s: s, p: p}
	r.cmd(Cmd{})
	if p.Pos[1] < 0 || !contains(s.Map.TeamSpawns[content.Vibe], p.Pos) {
		t.Fatalf("after falling out: %v", p.Pos)
	}
}

func contains(list [][3]float64, v Vec3) bool {
	for _, x := range list {
		if x == [3]float64(v) {
			return true
		}
	}
	return false
}

func TestJumpOnlyOnTheButtonEdge(t *testing.T) {
	s := authServer(t)
	p := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-14, 0, 6})
	r := &runner{s: s, p: p}
	for range 20 {
		r.cmd(Cmd{}) // settle on the floor
	}
	if !p.body.OnGround {
		t.Fatal("not on the ground")
	}
	takeoffs := 0
	was := true
	for range 300 { // hold jump for 3 s: one jump only
		r.cmd(Cmd{B: BtnJump})
		if was && !p.body.OnGround {
			takeoffs++
		}
		was = p.body.OnGround
	}
	if takeoffs != 1 {
		t.Fatalf("%d jumps with the button held", takeoffs)
	}
	r.cmd(Cmd{})
	for range 10 { // crouched on the ground: jump is ignored
		r.cmd(Cmd{B: BtnCrouch})
	}
	r.cmd(Cmd{B: BtnCrouch | BtnJump})
	if !p.body.OnGround || !p.Crouch {
		t.Fatalf("crouched jump: ground %v crouch %v", p.body.OnGround, p.Crouch)
	}
}

func TestSwitchReloadAndFireResults(t *testing.T) {
	s := authServer(t)
	p := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-14, 0, 6})
	r := &runner{s: s, p: p}
	for range DeployTicks {
		r.cmd(Cmd{})
	}
	r.cmd(Cmd{W: "sidearm"})
	if p.inv.Current != "sidearm" || len(ofType(p, "inv")) != 1 {
		t.Fatalf("switch: cur %s", p.inv.Current)
	}
	r.cmd(Cmd{W: "sidearm"}) // already there: no inventory update
	if len(ofType(p, "inv")) != 0 {
		t.Fatal("switch to the current slot sent an inventory")
	}
	// Fire while drawing: one rejected press reported, not every held tick.
	r.cmd(Cmd{B: BtnFire, Pitch: -1})
	r.cmd(Cmd{B: BtnFire, Pitch: -1})
	if f := ofType(p, "fired"); len(f) != 1 || f[0]["r"] != "cooldown" {
		t.Fatalf("fired while drawing: %v", f)
	}
	for range DeployTicks {
		r.cmd(Cmd{})
	}
	r.cmd(Cmd{B: BtnFire, Pitch: -1}) // into the sky
	if f := ofType(p, "fired"); len(f) != 1 || f[0]["r"] != "miss" || f[0]["s"] != float64(r.seq+1) {
		t.Fatalf("fired: %v", f)
	}
	h := p.inv.Slots["sidearm"]
	if h.Ammo != 11 {
		t.Fatalf("ammo %d", h.Ammo)
	}
	r.cmd(Cmd{B: BtnReload})
	w := s.Weapons.Get("stub")
	for range Ticks(w.Reload) - 2 {
		r.cmd(Cmd{})
	}
	r.cmd(Cmd{B: BtnFire, Pitch: -1}) // one tick before the reload ends
	if f := ofType(p, "fired"); len(f) != 1 || f[0]["r"] != "reloading" {
		t.Fatalf("fired during reload: %v", f)
	}
	r.cmd(Cmd{})
	// The reload settles on the next weapon action (fire/reload), not on idle ticks.
	r.cmd(Cmd{B: BtnFire, Pitch: -1})
	if f := ofType(p, "fired"); len(f) != 1 || f[0]["r"] != "miss" {
		t.Fatalf("first shot after the reload: %v", f)
	}
	if h.Ammo != w.Mag-1 || h.Reserve != w.Reserve-1 {
		t.Fatalf("after reload: %+v", h)
	}
}

func TestShowImpactsSendsTracedPoses(t *testing.T) {
	s := authServer(t)
	s.cvars["sv_showimpacts"] = "1"
	a := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-14, 0, 6})
	b := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-14, 0, 0})
	msgs(a)
	r := &runner{s: s, p: a}
	for range DeployTicks {
		r.cmd(Cmd{})
	}
	r.cmd(Cmd{B: BtnFire, Pitch: math.Atan2(0.9-1.62, 6), VT: float64(s.tick)})
	imp := ofType(a, "impacts")
	if len(imp) != 1 {
		t.Fatalf("impacts: %v", imp)
	}
	poses := imp[0]["poses"].([]any)
	if len(poses) != 1 || poses[0].(map[string]any)["id"] != float64(b.ID) || poses[0].(map[string]any)["team"] != content.Artisan {
		t.Fatalf("poses %v", poses)
	}
	end := imp[0]["end"].([]any)
	if z := end[2].(float64); math.Abs(z-0.23) > 0.05 {
		t.Fatalf("impact at z %.3f, want b's chest front (~0.23)", z)
	}
	if b.HP != maxHP-int(registry(t).Get("prompt").Damage) {
		t.Fatalf("b hp %d", b.HP)
	}
}

// A bot hit by a command-driven player takes no damage and never dies.
func TestBotsAreTargetPractice(t *testing.T) {
	s := authServer(t)
	s.cvars["weapon_spread"] = "0"
	a := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-14, 0, 6})
	bot := s.addBot()
	bot.Pos = Vec3{-14, 0, 0}
	hp := bot.HP
	r := &runner{s: s, p: a}
	for range DeployTicks {
		r.cmd(Cmd{})
	}
	for range 200 {
		r.cmd(Cmd{B: BtnFire, Pitch: math.Atan2(1.58-1.62, 6)}) // head height
	}
	if bot.HP != hp || bot.Dead {
		t.Fatalf("bot hp %d dead %v", bot.HP, bot.Dead)
	}
	hits := ofType(a, "hit")
	// the Prompt's damage × 4 on the head, less a little falloff over the 6 m
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	head := registry(t).Get("prompt").Damage * 4
	if d, _ := hits[0]["dmg"].(float64); hits[0]["part"] != "head" || d < head*0.95 || d > head {
		t.Fatalf("hits %v", hits)
	}
}

func TestDeadPlayersWaitThenRespawn(t *testing.T) {
	s := authServer(t)
	a := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-14, 0, 6})
	b := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-14, 0, 0})
	s.kill(a, b, s.Weapons.Get("prompt"), true)
	if k := ofType(a, "kill"); len(k) != 1 || k[0]["hs"] != true || k[0]["k"] != float64(a.ID) || k[0]["v"] != float64(b.ID) {
		t.Fatalf("kill message %v", k)
	}
	if !b.Dead || b.HP != 0 || b.Deaths != 1 || a.Kills != 1 {
		t.Fatalf("after kill: b dead %v hp %d deaths %d, a kills %d", b.Dead, b.HP, b.Deaths, a.Kills)
	}
	if d := b.respawnAt.Sub(s.clock()); d != 3*time.Second { // the server clock, as the respawn check
		t.Fatalf("respawn in %v", d)
	}
	seq := b.spawnSeq
	s.queueCmds(b, []Cmd{{Seq: 50, F: 1}})
	stepSim(s)
	if !b.Dead || len(b.cmds) != 0 || b.ackSeq == 50 {
		t.Fatal("a dead player's commands ran")
	}
	b.respawnAt = time.Now().Add(-time.Millisecond)
	stepSim(s)
	if b.Dead || b.HP != maxHP || b.spawnSeq != seq+1 || b.body == nil {
		t.Fatalf("no respawn: dead %v hp %d seq %d", b.Dead, b.HP, b.spawnSeq)
	}
	sp := ofType(b, "spawn")
	if len(sp) != 1 || sp[0]["team"] != content.Artisan || sp[0]["sp"] != float64(b.spawnSeq) {
		t.Fatalf("spawn message %v", sp)
	}
	// Suicide counts a death but no kill.
	s.kill(a, a, s.Weapons.Get("prompt"), false)
	if a.Kills != 1 || a.Deaths != 1 {
		t.Fatalf("suicide: kills %d deaths %d", a.Kills, a.Deaths)
	}
}

func TestRespawnFacesTheMiddle(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	p.Team = content.Artisan
	s.respawn(p)
	if p.body != nil {
		t.Fatal("legacy player got a sim body")
	}
	// yaw 0 faces -Z: the forward vector (-sin yaw, -cos yaw) points at the origin.
	fx, fz := -math.Sin(p.Yaw), -math.Cos(p.Yaw)
	l := math.Hypot(p.Pos[0], p.Pos[2])
	if math.Abs(fx+p.Pos[0]/l) > 1e-9 || math.Abs(fz+p.Pos[2]/l) > 1e-9 {
		t.Fatalf("at %v facing (%.3f, %.3f)", p.Pos, fx, fz)
	}
}

func TestSpreadSeedMatchesClientFormula(t *testing.T) {
	// arsenal.js: (id * 2654435761) ^ seq, in uint32.
	for _, tc := range []struct {
		id, seq int
		want    uint32
	}{
		{0, 0, 0},
		{1, 0, 2654435761},
		{1, 1, 2654435760},
		{2, 7, uint32(uint64(2*2654435761)&0xffffffff) ^ 7},
	} {
		if got := SpreadSeed(tc.id, tc.seq); got != tc.want {
			t.Errorf("SpreadSeed(%d, %d) = %d, want %d", tc.id, tc.seq, got, tc.want)
		}
	}
}

// Alone on the server, sv_showimpacts still sends a list of poses (empty, not
// null: the client maps over it, and null threw).
func TestShowImpactsAloneSendsNoPosesNotNull(t *testing.T) {
	s := authServer(t)
	s.cvars["sv_showimpacts"] = "1"
	a := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-14, 0, 6})
	msgs(a)
	r := &runner{s: s, p: a}
	for range DeployTicks {
		r.cmd(Cmd{})
	}
	r.cmd(Cmd{B: BtnFire, VT: float64(s.tick)})
	imp := ofType(a, "impacts")
	if len(imp) != 1 {
		t.Fatalf("impacts: %v", imp)
	}
	if poses, ok := imp[0]["poses"].([]any); !ok || len(poses) != 0 {
		t.Fatalf("poses %#v, want []", imp[0]["poses"])
	}
}
