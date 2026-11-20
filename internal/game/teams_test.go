package game

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// reply pops the next message sent to p and returns its type.
func reply(t *testing.T, p *Player) map[string]any {
	t.Helper()
	select {
	case b := <-p.send:
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m
	default:
		t.Fatal("no reply")
		return nil
	}
}

func TestJoinSpectatesThenAutoBalancesOntoTeamSpawns(t *testing.T) {
	s := NewServer()
	n := map[string]int{}
	for range 6 {
		p := s.join("127.0.0.1")
		if p.Team != content.Spectator {
			t.Fatalf("new player on %q, want spectator", p.Team)
		}
		s.handle(p, inbound{T: "team", Team: "auto"})
		if m := reply(t, p); m["t"] != "spawn" || m["team"] != p.Team {
			t.Fatalf("auto: reply %v, team %s", m, p.Team)
		}
		n[p.Team]++
		if !slices.Contains(s.Map.TeamSpawns[p.Team], [3]float64(p.Pos)) {
			t.Fatalf("player %d on %s spawned at %v, not a %s spawn", p.ID, p.Team, p.Pos, p.Team)
		}
	}
	if n[content.Vibe] != 3 || n[content.Artisan] != 3 {
		t.Fatalf("unbalanced teams: %v", n)
	}
}

func TestTeamLimitBlocksStacking(t *testing.T) {
	s := NewServer()
	a, b, c := s.join("127.0.0.1"), s.join("127.0.0.1"), s.join("127.0.0.1")
	s.handle(a, inbound{T: "team", Team: content.Vibe})
	reply(t, a)
	// 1 vs 0: a second Vibe Coder would make it 2 vs 0.
	s.handle(b, inbound{T: "team", Team: content.Vibe})
	if m := reply(t, b); m["t"] != "teamfull" || b.Team != content.Spectator {
		t.Fatalf("stacking allowed: reply %v, team %s", m, b.Team)
	}
	s.handle(b, inbound{T: "team", Team: content.Artisan})
	reply(t, b)
	s.handle(c, inbound{T: "team", Team: content.Vibe}) // 2 vs 1 is fine
	if m := reply(t, c); m["t"] != "spawn" {
		t.Fatalf("2 vs 1 refused: %v", m)
	}
	// b switching sides would make it 3 vs 0 (b no longer counts for Artisans).
	s.handle(b, inbound{T: "team", Team: content.Vibe})
	if m := reply(t, b); m["t"] != "teamfull" {
		t.Fatalf("switch to 3 vs 0 allowed: %v", m)
	}
	// Spectating is always allowed, and a spectator can't move or shoot.
	s.handle(a, inbound{T: "team", Team: content.Spectator})
	if m := reply(t, a); m["t"] != "spawn" || a.Team != content.Spectator {
		t.Fatalf("spectate: %v, team %s", m, a.Team)
	}
	pos := a.Pos
	s.handle(a, inbound{T: "state", Pos: Vec3{pos[0] + 1, 0, pos[2]}, Sp: a.spawnSeq})
	if a.Pos != pos {
		t.Fatalf("spectator moved to %v", a.Pos)
	}
}

func TestTeamSwitchRespawnsAndDropsStaleStates(t *testing.T) {
	s := NewServer()
	p := s.join("127.0.0.1")
	s.handle(p, inbound{T: "team", Team: content.Vibe})
	reply(t, p)
	s.handle(p, inbound{T: "team", Team: content.Artisan})
	if p.Team != content.Artisan || !slices.Contains(s.Map.TeamSpawns[content.Artisan], [3]float64(p.Pos)) {
		t.Fatalf("after switch: team %s pos %v", p.Team, p.Pos)
	}
	spawn := p.Pos
	reply(t, p)
	// A state sent before the client saw the respawn must not move it back.
	s.handle(p, inbound{T: "state", Pos: Vec3{-40, 0, -40}, Sp: p.spawnSeq - 1})
	if p.Pos != spawn {
		t.Fatalf("stale state moved the player to %v", p.Pos)
	}
	s.handle(p, inbound{T: "state", Pos: spawn, Sp: p.spawnSeq})
	select {
	case b := <-p.send:
		t.Fatalf("unexpected reply to a current state: %s", b)
	default:
	}
	s.handle(p, inbound{T: "team", Team: "nope"})
	if p.Team != content.Artisan {
		t.Fatalf("bad team id changed team to %q", p.Team)
	}
}
