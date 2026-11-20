package game

import (
	"strings"
	"testing"
)

func drain(p *Player) string {
	var sb strings.Builder
	for {
		select {
		case b := <-p.send:
			sb.Write(b)
			sb.WriteByte('\n')
		default:
			return sb.String()
		}
	}
}

func TestOpenAdmin(t *testing.T) {
	s := NewServer()
	p := s.join("1.1.1.1")
	s.motd(p)
	if out := drain(p); !strings.Contains(out, "everyone is admin") {
		t.Fatalf("no open-admin motd: %s", out)
	}
	s.adminCommand(p, "sv_gravity", []string{"10"})
	if s.cvars["sv_gravity"] != "10" {
		t.Fatal("open admin couldn't set a cvar")
	}
}

func TestPasswordAdminAndRateLimit(t *testing.T) {
	s := NewServer()
	s.AdminPassword = "hunter2"
	p := s.join("2.2.2.2")
	s.adminCommand(p, "sv_cheats", []string{"1"})
	if s.cvars["sv_cheats"] != "0" {
		t.Fatal("non-admin changed a cvar")
	}
	for range loginFailLimit {
		s.adminLogin(p, "wrong")
	}
	s.adminLogin(p, "hunter2") // blocked now, even with the right password
	if p.admin {
		t.Fatal("login allowed during backoff")
	}
	q := s.join("3.3.3.3") // other IPs aren't affected
	s.adminLogin(q, "hunter2")
	if !q.admin {
		t.Fatal("good login refused")
	}
	victim := s.join("4.4.4.4")
	s.adminCommand(q, "ban", []string{victim.Name, "5"})
	if s.players[victim.ID] != nil || !s.banned("4.4.4.4") {
		t.Fatal("ban didn't kick and ban")
	}
}
