package game

import (
	"math"
	"strings"
	"testing"
	"time"
)

func authServer(t *testing.T) *Server {
	s := NewServer()
	s.Weapons = registry(t)
	return s
}

func TestCommandsMovePlayer(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	p.ready = true
	p.Pos = Vec3{0, 0, 20}
	var cmds []Cmd
	for i := 1; i <= 100; i++ {
		cmds = append(cmds, Cmd{Seq: i, F: 1, Yaw: 0}) // yaw 0 = -Z
	}
	s.queueCmds(p, cmds)
	if len(p.cmds) != maxQueued {
		t.Fatalf("queued %d commands, want the cap %d", len(p.cmds), maxQueued)
	}
	for range 100 {
		s.simTick(time.Now())
	}
	// 32 real commands (0.32 s accelerating to 7 m/s), then repeats of the last intent.
	moved := 20 - p.Pos[2]
	if moved < 5 || moved > 7 || math.Abs(p.Pos[0]) > 1e-9 {
		t.Fatalf("moved %.2f m in 1 s of forward commands (pos %v)", moved, p.Pos)
	}
	if p.ackSeq != maxQueued {
		t.Fatalf("ack %d", p.ackSeq)
	}
}

func TestServerShotKillsAndRespawns(t *testing.T) {
	s := authServer(t)
	a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
	a.ready, b.ready = true, true
	// Open floor, b 6 m in front of a (a looks along -Z), both standing.
	a.Pos, b.Pos = Vec3{-14, 0, 6}, Vec3{-14, 0, 0}
	s.queueCmds(a, []Cmd{{Seq: 1}})
	s.queueCmds(b, []Cmd{{Seq: 1}})
	s.simTick(time.Now())
	drain(a)
	drain(b)
	// Aim at b's chest from a's eye: chest y ≈ 0.9, eye ≈ 1.62.
	pitch := math.Atan2(0.9-1.62, 6)
	seq := 2
	for i := 0; i < 400 && !b.Dead; i++ {
		s.queueCmds(a, []Cmd{{Seq: seq, Pitch: pitch, B: BtnFire}})
		s.queueCmds(b, []Cmd{{Seq: seq}})
		seq++
		s.simTick(time.Now())
	}
	if !b.Dead || b.Deaths != 1 || a.Kills != 1 {
		t.Fatalf("b dead=%v hp=%d deaths=%d, a kills=%d", b.Dead, b.HP, b.Deaths, a.Kills)
	}
	if out := drain(a); !strings.Contains(out, `"t":"kill"`) || !strings.Contains(out, `"t":"hit"`) {
		t.Fatalf("shooter didn't get hit+kill: %.300s", out)
	}
	b.respawnAt = time.Now().Add(-time.Second)
	s.simTick(time.Now())
	if b.Dead || b.HP != maxHP {
		t.Fatalf("no respawn: dead=%v hp=%d", b.Dead, b.HP)
	}
}
