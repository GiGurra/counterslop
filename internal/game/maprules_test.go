package game

import (
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/sim"
)

// Every cvar a map may pin is a server cvar with help, and every map's pins
// are values rcon would set (cvarValue leaves them as they are).
func TestMapRulesAreValidCvars(t *testing.T) {
	for _, k := range content.RuleCvars {
		if _, ok := defaultCvars[k]; !ok || cvarHelp[k][1] == "" {
			t.Errorf("content.RuleCvars %q: no server cvar or help", k)
		}
	}
	for _, name := range append(append([]string{}, maps.Order...), maps.LabNames()...) {
		for k, v := range maps.Find(name)().Rules {
			if got, err := cvarValue(k, v); err != nil || got != v {
				t.Errorf("%s pins %s = %q: rcon makes it %q (%v)", name, k, v, got, err)
			}
		}
	}
}

// A map's Rules pin cvars over the server's while it runs: lab_bhop plays with
// source movement and auto bhop on a classic server, de_slop with the
// server's; an admin's change to a pinned cvar is the server's value (for
// the other maps) and says so.
func TestMapRulesPinCvars(t *testing.T) {
	s := NewServer()
	s.Weapons = registry(t)
	p := s.join("1.1.1.1") // open admin
	if r := s.moveRules(); r != (sim.MoveRules{Model: sim.MoveClassic, AirAccel: sim.DefaultAirAccel}) {
		t.Fatalf("default rules %+v", r)
	}
	if !s.changeLevel("lab_bhop") {
		t.Fatal("changelevel lab_bhop")
	}
	if r := s.moveRules(); r.Model != sim.MoveSource || !r.AutoBhop || r.AirAccel != 12 {
		t.Fatalf("lab_bhop rules %+v", r)
	}
	if w := s.simWorld(); w.Move != s.moveRules() {
		t.Fatalf("sim world moves by %+v", w.Move)
	}
	s.adminCommand(p, "sv_movement", []string{"classic"})
	if s.cvars["sv_movement"] != "classic" || s.moveRules().Model != sim.MoveSource {
		t.Fatal("the map's pin must win while it runs")
	}
	s.adminCommand(p, "sv_airaccelerate", []string{"150"})
	if s.moveRules().AirAccel != 150 {
		t.Fatalf("unpinned sv_airaccelerate: %+v", s.moveRules())
	}
	s.changeLevel("de_slop")
	if r := s.moveRules(); r.Model != sim.MoveClassic || r.AutoBhop || r.AirAccel != 150 {
		t.Fatalf("de_slop rules %+v", r)
	}
	s.adminCommand(p, "sv_movement", []string{"SOURCE"})
	if s.cvars["sv_movement"] != "source" || s.moveRules().Model != sim.MoveSource {
		t.Fatalf("sv_movement source: %q", s.cvars["sv_movement"])
	}
	s.adminCommand(p, "sv_movement", []string{"quake"})
	if s.cvars["sv_movement"] != "source" {
		t.Fatalf("junk taken: %q", s.cvars["sv_movement"])
	}
	s.adminCommand(p, "sv_airaccelerate", []string{"5000"})
	if s.cvars["sv_airaccelerate"] != "1000" {
		t.Fatalf("clamp: %q", s.cvars["sv_airaccelerate"])
	}
	s.adminCommand(p, "sv_autobunnyhopping", []string{"1"})
	if !s.moveRules().AutoBhop {
		t.Fatal("sv_autobunnyhopping 1")
	}
}

// The source model in the server's real command path: holding jump on
// lab_bhop hops again on landing (auto bhop), on de_slop (classic, the
// server's default) it jumps once.
func TestMapRulesInRunCmd(t *testing.T) {
	for _, tc := range []struct {
		m     string
		jumps int
	}{{"lab_bhop", 3}, {"de_slop", 1}} {
		s := NewServer()
		s.Weapons = registry(t)
		s.changeLevel(tc.m)
		p := s.join("1.1.1.1")
		p.body = sim.NewBody(sim.Vec3{0, 0, 0})
		if tc.m == "de_slop" {
			p.body = sim.NewBody(sim.Vec3(s.Map.TeamSpawns[content.Vibe][0]))
		}
		w := s.simWorld()
		jumps := 0
		for seq := 1; seq <= 150; seq++ {
			vy := p.body.Vel[1]
			s.runCmd(w, p, Cmd{Seq: seq, B: BtnJump})
			if p.body.Vel[1] > 5 && vy < 5 { // a jump (not a landing)
				jumps++
			}
		}
		if jumps < tc.jumps || (tc.jumps == 1 && jumps != 1) {
			t.Errorf("%s: %d jumps holding jump for 1.5 s, want %d", tc.m, jumps, tc.jumps)
		}
	}
}
