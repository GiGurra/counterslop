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
// the map (remote bullet holes); a ray stopped by a player leaves none.
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
	// Aimed at b's chest (the next shot, before recoil climbs): it stops in b, no wall impact.
	for seq := 3; seq < 60; seq++ {
		s.queueCmds(a, []Cmd{{Seq: seq, Pitch: math.Atan2(0.9-1.62, 6), B: BtnFire}})
		s.queueCmds(b, []Cmd{{Seq: seq}})
		stepSim(s)
		if m := ofType(b, "shot"); len(m) > 0 {
			if m[0]["h"] != nil || b.HP == maxHP {
				t.Fatalf("a shot into a player: %v, hp %d", m[0], b.HP)
			}
			return
		}
	}
	t.Fatal("second shot never relayed")
}
