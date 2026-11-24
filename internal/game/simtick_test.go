package game

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

func authServer(t *testing.T) *Server {
	s := NewServer()
	s.Weapons = registry(t)
	return s
}

// stepSim runs one server tick 10 ms (one tick of real time) after the last.
func stepSim(s *Server) {
	now := s.lastTickAt.Add(time.Second / SimHz)
	if s.lastTickAt.IsZero() {
		now = time.Now()
	}
	s.simTick(now)
}

func TestCommandsMovePlayer(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	p.ready, p.Team = true, content.Vibe
	p.Pos = Vec3{0, 0, 20}
	var cmds []Cmd
	for i := 1; i <= 100; i++ {
		cmds = append(cmds, Cmd{Seq: i, F: 1, Yaw: 0}) // yaw 0 = -Z
	}
	s.queueCmds(p, cmds)
	if len(p.cmds) != maxQueued || p.cmds[len(p.cmds)-1].Seq != 100 {
		t.Fatalf("queued %d commands (last %d), want the newest %d", len(p.cmds), p.cmds[len(p.cmds)-1].Seq, maxQueued)
	}
	for range 100 {
		stepSim(s)
	}
	// The newest 64 commands ran (0.64 s accelerating to 7 m/s); no extrapolation after.
	moved := 20 - p.Pos[2]
	if moved < 3.5 || moved > 4.6 || math.Abs(p.Pos[0]) > 1e-9 {
		t.Fatalf("moved %.2f m on 64 forward commands (pos %v)", moved, p.Pos)
	}
	if p.ackSeq != 100 || len(p.cmds) != 0 {
		t.Fatalf("ack %d, %d still queued", p.ackSeq, len(p.cmds))
	}
}

// A client whose clock runs 1% fast must not build a backlog (that delays
// every shot and, once full, used to drop fire commands); one running 50%
// fast (a speedhack) gets no more than real time plus the drift allowance.
func TestCommandBudgetFollowsRealTime(t *testing.T) {
	for _, tc := range []struct {
		name    string
		perTick float64
		maxRun  float64 // commands run per server tick, at most
		maxLag  int
	}{
		{"2% fast", 1.02, 1.04, 3},
		{"speedhack", 1.5, 1.04, maxQueued},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := authServer(t)
			p := s.join("1.1.1.1")
			p.ready, p.Team = true, content.Vibe
			seq, acc := 0, 0.0
			const ticks = 1000
			for range ticks {
				acc += tc.perTick
				var batch []Cmd
				for acc >= 1 {
					acc--
					seq++
					batch = append(batch, Cmd{Seq: seq, B: BtnFire})
				}
				s.queueCmds(p, batch)
				stepSim(s)
			}
			if lag := len(p.cmds); lag > tc.maxLag {
				t.Fatalf("%d commands waiting after %d ticks", lag, ticks)
			}
			if run := float64(p.cmdTicks) / ticks; run > tc.maxRun {
				t.Fatalf("ran %.3f commands per tick", run)
			}
		})
	}
}

func TestServerShotKillsAndRespawns(t *testing.T) {
	s := authServer(t)
	a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
	a.ready, b.ready = true, true
	a.Team, b.Team = content.Vibe, content.Artisan
	// Open floor, b 6 m in front of a (a looks along -Z), both standing.
	a.Pos, b.Pos = Vec3{-14, 0, 6}, Vec3{-14, 0, 0}
	s.queueCmds(a, []Cmd{{Seq: 1}})
	s.queueCmds(b, []Cmd{{Seq: 1}})
	stepSim(s)
	drain(a)
	drain(b)
	// Aim at b's chest from a's eye: chest y ≈ 0.9, eye ≈ 1.62.
	pitch := math.Atan2(0.9-1.62, 6)
	seq := 2
	for i := 0; i < 400 && !b.Dead; i++ {
		s.queueCmds(a, []Cmd{{Seq: seq, Pitch: pitch, B: BtnFire}})
		s.queueCmds(b, []Cmd{{Seq: seq}})
		seq++
		stepSim(s)
	}
	if !b.Dead || b.Deaths != 1 || a.Kills != 1 {
		t.Fatalf("b dead=%v hp=%d deaths=%d, a kills=%d", b.Dead, b.HP, b.Deaths, a.Kills)
	}
	if out := drain(a); !strings.Contains(out, `"t":"kill"`) || !strings.Contains(out, `"t":"hit"`) {
		t.Fatalf("shooter didn't get hit+kill: %.300s", out)
	}
	b.respawnAt = time.Now().Add(-time.Second)
	stepSim(s)
	if b.Dead || b.HP != maxHP {
		t.Fatalf("no respawn: dead=%v hp=%d", b.Dead, b.HP)
	}
}

func TestServerShotSkipsTeammatesUnlessFriendlyFire(t *testing.T) {
	for _, ff := range []string{"0", "1"} {
		s := authServer(t)
		s.cvars["mp_friendlyfire"] = ff
		a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
		a.ready, b.ready = true, true
		a.Team, b.Team = content.Vibe, content.Vibe
		a.Pos, b.Pos = Vec3{-14, 0, 6}, Vec3{-14, 0, 0}
		s.queueCmds(a, []Cmd{{Seq: 1}})
		s.queueCmds(b, []Cmd{{Seq: 1}})
		stepSim(s)
		pitch := math.Atan2(0.9-1.62, 6)
		for seq := 2; seq < 60; seq++ {
			s.queueCmds(a, []Cmd{{Seq: seq, Pitch: pitch, B: BtnFire}})
			s.queueCmds(b, []Cmd{{Seq: seq}})
			stepSim(s)
		}
		if hurt := b.HP < maxHP; hurt != (ff == "1") {
			t.Fatalf("mp_friendlyfire %s: teammate hp %d", ff, b.HP)
		}
	}
}

func TestRespawnUsesTeamSpawnAndSpectatorsGetNoBody(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	s.queueCmds(p, []Cmd{{Seq: 1}})
	if p.body != nil {
		t.Fatal("spectator got a body from commands")
	}
	p.lastQueued = 1 // a command-driven client
	s.handle(p, inbound{T: "team", Team: content.Artisan})
	if p.body == nil || !slices.Contains(s.Map.TeamSpawns[content.Artisan], [3]float64(p.Pos)) {
		t.Fatalf("team switch: body %v pos %v", p.body != nil, p.Pos)
	}
	s.handle(p, inbound{T: "team", Team: content.Spectator})
	if p.body != nil {
		t.Fatal("spectator kept a body")
	}
}

// The relayed "shot" carries the shooter's eye and where its rays stopped in
// the map (remote bullet holes); a rifle bullet through a player goes on to
// the wall behind (penetration.go; one stopped in a body leaves none: TestPenetrate*).
func TestShotRelayCarriesWallImpacts(t *testing.T) {
	s := authServer(t)
	a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
	a.ready, b.ready = true, true
	a.Team, b.Team = content.Vibe, content.Artisan
	a.Pos, b.Pos = Vec3{-14, 0, 6}, Vec3{-14, 0, 0}
	s.queueCmds(a, []Cmd{{Seq: 1}})
	s.queueCmds(b, []Cmd{{Seq: 1}})
	stepSim(s)
	msgs(b)
	s.queueCmds(a, []Cmd{{Seq: 2, Pitch: -1.2, B: BtnFire}}) // at the floor in front
	s.queueCmds(b, []Cmd{{Seq: 2}})
	stepSim(s)
	shots := ofType(b, "shot")
	if len(shots) != 1 {
		t.Fatalf("shots %v", shots)
	}
	h, _ := shots[0]["h"].([]any)
	o, _ := shots[0]["o"].([]any)
	if len(h) != 1 || len(o) != 3 {
		t.Fatalf("shot %v", shots[0])
	}
	if p := h[0].([]any); math.Abs(p[1].(float64)) > 0.01 || p[2].(float64) > 6 {
		t.Fatalf("impact %v not on the floor ahead", p)
	}
	// Aimed at b's chest (the next shot, before recoil climbs): it hurts b and
	// goes through (the Prompt's penetration) to the floor behind b.
	for seq := 3; seq < 60; seq++ {
		s.queueCmds(a, []Cmd{{Seq: seq, Pitch: math.Atan2(0.9-1.62, 6), B: BtnFire}})
		s.queueCmds(b, []Cmd{{Seq: seq}})
		stepSim(s)
		if m := ofType(b, "shot"); len(m) > 0 {
			h, _ := m[0]["h"].([]any)
			if len(h) != 1 || h[0].([]any)[2].(float64) >= 0 || b.HP == maxHP {
				t.Fatalf("a shot through a player: %v, hp %d", m[0], b.HP)
			}
			return
		}
	}
	t.Fatal("second shot never relayed")
}

// A link stall (the client's commands held back G ticks, then arriving at
// once, as a frozen Wi-Fi or a tab hitch delivers them) must not leave a
// lasting backlog: within a few ticks the queue is back to about one command
// and every command after it runs the tick it arrives (a shot registers on
// time). The bank may have been spent by an earlier stall.
func TestStallBurstLeavesNoBacklog(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stalls  []int // ticks each (after 30 s of steady play: a full bank)
		between int   // ticks of steady play between stalls (the bank refills only at cmdDrift-1 a tick)
	}{
		{"0.5 s stall", []int{50}, 0},
		{"0.6 s stall", []int{60}, 0},
		{"0.64 s stall", []int{64}, 0},
		{"1 s stall", []int{100}, 0},
		{"0.6 s stall 2 s after another", []int{100, 60}, 200},
		{"three 1 s stalls 2 s apart", []int{100, 100, 100}, 200},
		{"a 1 s stall right after another (bank spent)", []int{100, 100}, 1},
		{"a 0.3 s stall right after a 1 s one", []int{100, 30}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := authServer(t)
			p := s.join("1.1.1.1")
			p.ready, p.Team = true, content.Vibe
			seq := 0
			next := func() Cmd { seq++; return Cmd{Seq: seq} }
			steady := func(n int) {
				for range n {
					s.queueCmds(p, []Cmd{next()})
					stepSim(s)
				}
			}
			steady(3000) // a full bank
			for i, gap := range tc.stalls {
				if i > 0 {
					steady(tc.between)
				}
				var held []Cmd
				for range gap {
					held = append(held, next())
					stepSim(s)
				}
				s.queueCmds(p, held)
			}
			worst := 0
			for i := range 300 {
				s.queueCmds(p, []Cmd{next()})
				stepSim(s)
				if i >= 3 {
					worst = max(worst, len(p.cmds))
				}
			}
			if worst > 1 {
				t.Errorf("after the stall's burst: %d commands still queued a few ticks later (every action runs %d ms late)", worst, worst*1000/SimHz)
			}
			if p.ackSeq != seq {
				t.Errorf("acked %d of %d sent", p.ackSeq, seq)
			}
		})
	}
}

// A speedhack (commands 10% faster than real time, steadily) after a full
// bank gains at most the bank and the drift allowance: the rest queues and is
// dropped.
func TestSpeedhackGainsAtMostTheBank(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	p.ready, p.Team = true, content.Vibe
	seq := 0
	for range 3000 { // honest play: the bank fills
		seq++
		s.queueCmds(p, []Cmd{{Seq: seq}})
		stepSim(s)
	}
	if p.cmdBudget < cmdBank-1 {
		t.Fatalf("bank %.1f after 30 s of play, want about %d", p.cmdBudget, cmdBank)
	}
	ran0, acc := p.ackSeq-p.cmdsDropped, 0.0
	const ticks = 2000
	for range ticks {
		var batch []Cmd
		for acc += 1.1; acc >= 1; acc-- {
			seq++
			batch = append(batch, Cmd{Seq: seq})
		}
		s.queueCmds(p, batch)
		stepSim(s)
	}
	ran := p.ackSeq - p.cmdsDropped - ran0 // acked minus dropped: the ones run
	if limit := int(ticks*cmdDrift) + cmdBank + 1; ran > limit {
		t.Fatalf("a 1.1x speedhack ran %d commands in %d ticks: over %d (real time x drift + the bank)", ran, ticks, limit)
	}
	t.Logf("1.1x for %d ticks: ran %d (%.3f a tick), dropped %d", ticks, ran, float64(ran)/ticks, p.cmdsDropped)
}

// The server's clock may run at another rate than the client's (WSL's drifts)
// or step: a backlog the budget never makes up is cut down to backlogTarget,
// so a shot after 30 s still runs within a few ticks of arriving.
func TestBacklogBoundedWhateverTheClocks(t *testing.T) {
	for _, tc := range []struct {
		name string
		dt   func(tick int) time.Duration // the server clock's advance per (real) tick
	}{
		{"server clock 5% slow", func(int) time.Duration { return time.Second / SimHz * 95 / 100 }},
		{"server clock 20% slow", func(int) time.Duration { return time.Second / SimHz * 80 / 100 }},
		{"server clock steps back 1 s every 3 s", func(tick int) time.Duration {
			if tick%300 == 299 {
				return -time.Second
			}
			return time.Second / SimHz
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := authServer(t)
			p := s.join("1.1.1.1")
			p.ready, p.Team = true, content.Vibe
			seq := 0
			clock := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
			step := func(tick int, c Cmd) {
				s.queueCmds(p, []Cmd{c})
				clock = clock.Add(tc.dt(tick))
				s.simTick(clock)
			}
			worst := 0
			for tick := range 3000 { // 30 s
				seq++
				step(tick, Cmd{Seq: seq})
				if tick > 200 {
					worst = max(worst, len(p.cmds))
				}
			}
			if worst > backlogTarget+3 {
				t.Errorf("the queue reached %d commands (%d ms behind)", worst, worst*1000/SimHz)
			}
			seq++
			fire := seq
			for tick := 3000; p.ackSeq < fire; tick++ {
				if tick > 3000+backlogTarget+3 {
					t.Fatalf("a shot after 30 s still not run %d ticks after it arrived (%d queued)", tick-3000, len(p.cmds))
				}
				step(tick, Cmd{Seq: seq, B: BtnFire})
				seq++
			}
			t.Logf("queue at most %d; %d commands dropped in 30 s", worst, p.cmdsDropped)
		})
	}
}
