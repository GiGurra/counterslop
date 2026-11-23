package game

import (
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/sim"
)

// Two players run into each other through the server's command path (runCmd,
// peersOf) from every 7.5°, at sub-mm offsets: neither ever gets more than a
// millimetre into the other (they used to run through each other: each saw
// the other's mm-rounded hull a fraction of a mm inside its own).
func TestSolidPlayersHeadOn(t *testing.T) {
	for deg := 0.0; deg < 360; deg += 7.5 {
		for _, off := range []float64{0, 0.0003, 0.0007} {
			s := authServer(t)
			s.Test = true
			start := time.Now()
			s.lastTickAt = start
			at := Vec3{-26 + off, 0, 4 - off/2}
			a := deg * math.Pi / 180
			d := [2]float64{math.Sin(a), -math.Cos(a)}
			yaw := math.Atan2(d[0], d[1]) // faces the other: forward is (-sin yaw, -cos yaw)
			tg, r := s.join("10.0.0.1"), s.join("10.0.0.2")
			tg.Name, tg.ready, tg.Team = "tg", true, "vibe"
			r.Name, r.ready, r.Team = "r", true, "artisan"
			s.teleport(tg, at, yaw+math.Pi, false, 0)
			s.teleport(r, Vec3{at[0] + 2.5*d[0], 0, at[2] + 2.5*d[1]}, yaw, false, 0)
			tg.inv, r.inv = s.Weapons.Loadout(tg.Team), s.Weapons.Loadout(r.Team)
			worst := 0.0
			for tick := 1; tick <= 100; tick++ {
				s.queueCmds(tg, []Cmd{{Seq: tick, F: 1, Yaw: yaw + math.Pi}})
				s.queueCmds(r, []Cmd{{Seq: tick, F: 1, Yaw: yaw}})
				s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
				pen := math.Min(2*sim.R-math.Abs(r.Pos[0]-tg.Pos[0]), 2*sim.R-math.Abs(r.Pos[2]-tg.Pos[2]))
				worst = math.Max(worst, pen)
			}
			if worst > 0.001 {
				t.Errorf("%.1f° offset %v: %.4f m into each other", deg, off, worst)
			}
		}
	}
}

// Target-practice bots (bot_add) are solid, though they have no HP: a player
// running at one stops a hull away (they used to run straight through), and
// one walking at a standing player stops instead of walking through.
func TestSolidTargetBots(t *testing.T) {
	for k := range 8 {
		s := authServer(t)
		s.Test = true
		start := time.Now()
		s.lastTickAt = start
		at := Vec3{-26, 0, 4}
		bot := s.addBot()
		bot.Pos, bot.brain.frozen = at, true
		r := s.join("10.0.0.2")
		r.Name, r.ready, r.Team = "r", true, "artisan"
		a := float64(k) * math.Pi / 4
		d := [2]float64{math.Sin(a), -math.Cos(a)}
		yaw := math.Atan2(d[0], d[1])
		s.teleport(r, Vec3{at[0] + 3*d[0], 0, at[2] + 3*d[1]}, yaw, false, 0)
		r.inv = s.Weapons.Loadout(r.Team)
		for tick := 1; tick <= 100; tick++ {
			s.queueCmds(r, []Cmd{{Seq: tick, F: 1, Yaw: yaw}})
			s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
			if pen := math.Min(2*sim.R-math.Abs(r.Pos[0]-at[0]), 2*sim.R-math.Abs(r.Pos[2]-at[2])); pen > 0.001 {
				t.Fatalf("%d°: tick %d the runner is %.3f m into the bot at %v", k*45, tick, pen, r.Pos)
			}
		}
	}
	s := authServer(t)
	s.Test = true
	start := time.Now()
	s.lastTickAt = start
	p := s.join("10.0.0.1")
	p.Name, p.ready, p.Team = "p", true, "vibe"
	s.teleport(p, Vec3{-26, 0, 4}, 0, false, 0)
	p.inv = s.Weapons.Loadout(p.Team)
	bot := s.addBot()
	bot.Pos = Vec3{-26, 0, 0}
	bot.brain.patrol = &[2]Vec3{{-26, 0, 8}, {-26, 0, 0}}
	for tick := 1; tick <= 300; tick++ {
		s.queueCmds(p, []Cmd{{Seq: tick}})
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		if pen := math.Min(2*sim.R-math.Abs(bot.Pos[0]-p.Pos[0]), 2*sim.R-math.Abs(bot.Pos[2]-p.Pos[2])); pen > 0.001 {
			t.Fatalf("tick %d: the bot walked %.3f m into the player (bot %v, player %v)", tick, pen, bot.Pos, p.Pos)
		}
	}
}
