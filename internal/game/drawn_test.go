package game

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/sim"
)

// drawnDelay: the client draws the others this many ticks behind the server
// on a clean link (net.js: two snapshot intervals and a tick at 100
// snapshots/s, 30 ms), and says so in each command (vt).
const drawnDelay = 3

// A player runs across bob's path while bob runs at it. Bob's client draws
// her drawnDelay ticks in the past (History.At(vt)): he must collide with
// that drawn model, never stopping more than 0.2 m short of it (an invisible
// wall ahead of her) and never getting more than 0.2 m into it.
func TestSolidAgainstDrawnModel(t *testing.T) {
	worstIn, worstWall := 0.0, 0.0
	for lead := -39; lead <= 60; lead += 3 { // bob sets off at tick 40+lead: every way of meeting her
		for _, mover := range []string{"human", "tpbot"} {
			in, wall := drawnRun(t, mover, lead)
			worstIn, worstWall = math.Max(worstIn, in), math.Max(worstWall, wall)
			if in > 0.2 || wall > 0.2 {
				t.Errorf("%s target, bob %+d ticks: %.2f m into her drawn model, blocked %.2f m short of it", mover, lead, in, wall)
			}
		}
	}
	t.Logf("worst: %.3f m into the drawn model, %.3f m invisible wall", worstIn, worstWall)
}

// drawnRun: how deep bob's own moves took him into the drawn target, and the
// largest gap between them at a tick he was stopped by her. (Her drawn model
// can still walk into him: she collides with where her client drew him.)
func drawnRun(t *testing.T, mover string, lead int) (in, wall float64) {
	s := authServer(t)
	s.Test = true
	start := time.Now()
	s.lastTickAt = start
	var tg *Player
	if mover == "human" {
		tg = s.join("10.0.0.1")
		tg.Name, tg.ready, tg.Team = "alice", true, "vibe"
		s.teleport(tg, Vec3{-30, 0, 4}, 0, false, 0)
		tg.inv = s.Weapons.Loadout(tg.Team)
	} else {
		tg = s.addBot()
		tg.Pos = Vec3{-30, 0, 4}
		tg.brain.patrol = &[2]Vec3{{-21, 0, 4}, {-30, 0, 4}}
	}
	bob := s.join("10.0.0.2")
	bob.Name, bob.ready, bob.Team = "bob", true, "artisan"
	s.teleport(bob, Vec3{-26, 0, 7.5}, 0, false, 0)
	bob.inv = s.Weapons.Loadout(bob.Team)
	for tick := 1; tick <= 160; tick++ {
		if tg.ai == nil && !tg.Bot {
			s.queueCmds(tg, []Cmd{{Seq: tick, R: 1}}) // strafes +x
		}
		vt := float64(s.tick - drawnDelay)
		f := 0.0
		if tick >= 40+lead {
			f = 1 // runs -z, across her line
		}
		s.queueCmds(bob, []Cmd{{Seq: tick, F: f, VT: vt}})
		b0 := bob.body.Pos
		z0 := b0[2]
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		d, ok := s.history.At(vt)[tg.ID] // where bob's client drew her for this command
		if !ok || vt < 1 {
			continue
		}
		pen := func(p Vec3) float64 {
			return math.Min(2*sim.R-math.Abs(p[0]-d.Pos[0]), 2*sim.R-math.Abs(p[2]-d.Pos[2]))
		}
		dx, dz := math.Abs(bob.Pos[0]-d.Pos[0]), math.Abs(bob.Pos[2]-d.Pos[2])
		if after := pen(bob.Pos); after > pen(Vec3(b0)) { // his own move took him deeper (her drawn model walking into him doesn't count)
			in = math.Max(in, after)
		}
		if f == 1 && tick > 40+lead+15 && z0-bob.body.Pos[2] < 0.01 && bob.body.Pos[2] > 4 { // stopped short of her line
			wall = math.Max(wall, math.Max(dx, dz)-2*sim.R)
		}
	}
	return in, wall
}

// Chasing a playing (AI) bot, a teammate going about its business: bob keeps
// running at it as his client draws it (drawnDelay behind). His own moves
// never take him more than 0.2 m into the drawn bot, and he is never stopped
// more than 0.2 m short of it.
func TestSolidChaseAIBot(t *testing.T) {
	s := authServer(t)
	s.Test = true
	start := time.Now()
	s.lastTickAt = start
	bot := s.addAI("vibe")
	bob := s.join("10.0.0.2")
	bob.Name, bob.ready, bob.Team = "bob", true, "vibe"
	s.teleport(bob, Vec3{bot.Pos[0] + 2, bot.Pos[1], bot.Pos[2] + 2}, 0, false, 0)
	bob.inv = s.Weapons.Loadout(bob.Team)
	in, wall, near, moved := 0.0, 0.0, 0, 0.0
	b0 := bot.Pos
	for tick := 1; tick <= 1500; tick++ {
		vt := float64(s.tick - drawnDelay)
		yaw := bob.body.Yaw
		if d, ok := s.history.At(vt)[bot.ID]; ok && vt >= 1 {
			yaw = math.Atan2(-(d.Pos[0] - bob.Pos[0]), -(d.Pos[2] - bob.Pos[2])) // forward is (-sin yaw, -cos yaw)
		}
		s.queueCmds(bob, []Cmd{{Seq: tick, F: 1, Yaw: yaw, VT: vt}})
		before := Vec3(bob.body.Pos)
		v0 := bob.body.Vel
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		d, ok := s.history.At(vt)[bot.ID]
		if !ok || vt < 1 || bot.Dead || math.Abs(bob.Pos[1]-d.Pos[1]) > 1 {
			continue
		}
		pen := func(p Vec3) float64 {
			return math.Min(2*sim.R-math.Abs(p[0]-d.Pos[0]), 2*sim.R-math.Abs(p[2]-d.Pos[2]))
		}
		if after := pen(bob.Pos); after > pen(before) {
			in = math.Max(in, after)
		}
		gap := -pen(bob.Pos)
		if gap < 0.5 {
			near++
			stopped := (math.Abs(v0[0]) > 0.5 && bob.body.Vel[0] == 0) || (math.Abs(v0[2]) > 0.5 && bob.body.Vel[2] == 0)
			if stopped {
				wall = math.Max(wall, gap)
			}
		}
		moved = math.Max(moved, math.Hypot(bot.Pos[0]-b0[0], bot.Pos[2]-b0[2]))
	}
	t.Logf("AI bot moved up to %.1f m; %d ticks near it; %.3f m into it, stopped %.3f m short", moved, near, in, wall)
	if moved < 3 || near < 50 {
		t.Fatalf("no chase: the bot moved %.1f m, bob near it %d ticks", moved, near)
	}
	if in > 0.2 || wall > 0.2 {
		t.Errorf("chasing the AI bot: %.3f m into its drawn model, stopped %.3f m short of it", in, wall)
	}
}

// Head-on with both clients drawing the other drawnDelay ticks back (and
// 0, 5, 25): each collides with the other's past, so where they really are
// ('now') their hulls can overlap, but each other's core (the centre line
// where it really is) keeps the centres R apart: at most R of overlap, and
// nobody passes through anyone (they may slide round, or past each other
// half-overlapped). Walking back apart is never blocked (no trap).
func TestSolidHeadOnNowOverlap(t *testing.T) {
	for _, delay := range []int64{0, drawnDelay, 5, 10, 25} {
		worst := 0.0
		for deg := 0.0; deg < 360; deg += 15 {
			s := authServer(t)
			s.Test = true
			start := time.Now()
			s.lastTickAt = start
			at := Vec3{-26, 0, 4}
			a := deg * math.Pi / 180
			d := [2]float64{math.Sin(a), -math.Cos(a)}
			yaw := math.Atan2(d[0], d[1])
			p, q := s.join("10.0.0.1"), s.join("10.0.0.2")
			p.Name, p.ready, p.Team = "p", true, "vibe"
			q.Name, q.ready, q.Team = "q", true, "artisan"
			s.teleport(p, at, yaw+math.Pi, false, 0)
			s.teleport(q, Vec3{at[0] + 3*d[0], 0, at[2] + 3*d[1]}, yaw, false, 0)
			p.inv, q.inv = s.Weapons.Loadout(p.Team), s.Weapons.Loadout(q.Team)
			// Passing through: x order flips while overlapping in z, or z order
			// while overlapping in x (sliding round each other is fine).
			sx, sz := math.Signbit(q.Pos[0]-p.Pos[0]), math.Signbit(q.Pos[2]-p.Pos[2])
			over := 0.0
			var p0, q0 Vec3
			for tick := 1; tick <= 250; tick++ {
				f := 1.0
				if tick > 150 {
					f = -1 // back off
				}
				if tick == 151 {
					p0, q0 = p.Pos, q.Pos
				}
				vt := float64(s.tick - delay)
				s.queueCmds(p, []Cmd{{Seq: tick, F: f, Yaw: yaw + math.Pi, VT: vt}})
				s.queueCmds(q, []Cmd{{Seq: tick, F: f, Yaw: yaw, VT: vt}})
				s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
				over = math.Max(over, math.Min(2*sim.R-math.Abs(p.Pos[0]-q.Pos[0]), 2*sim.R-math.Abs(p.Pos[2]-q.Pos[2])))
				nx, nz := math.Signbit(q.Pos[0]-p.Pos[0]), math.Signbit(q.Pos[2]-p.Pos[2])
				if (nx != sx && math.Abs(q.Pos[2]-p.Pos[2]) < sim.R-0.002) || (nz != sz && math.Abs(q.Pos[0]-p.Pos[0]) < sim.R-0.002) {
					t.Fatalf("delay %d, %v°: tick %d they passed through each other: p %v q %v", delay, deg, tick, p.Pos, q.Pos)
				}
				sx, sz = nx, nz
			}
			if mp, mq := math.Hypot(p.Pos[0]-p0[0], p.Pos[2]-p0[2]), math.Hypot(q.Pos[0]-q0[0], q.Pos[2]-q0[2]); mp < 1 || mq < 1 {
				t.Errorf("delay %d, %v°: backing off for 1 s moved them only %.2f and %.2f m (trapped?)", delay, deg, mp, mq)
			}
			worst = math.Max(worst, over)
		}
		t.Logf("head-on, view delay %2d ticks: up to %.3f m overlap 'now', no pass-through in 24 angles", delay, worst)
		if worst > sim.R-sim.CoreHalf+0.002 {
			t.Errorf("delay %d: now-overlap %.3f m: the cores should keep centres R+CoreHalf apart (overlap <= %.1f m)", delay, worst, sim.R-sim.CoreHalf)
		}
	}
}

// No rubber banding: two players holding forward into each other (head-on,
// and at an angle, both clients drawing the other delay ticks back) never
// move backwards along where they push: each tick's displacement along their
// own wish direction is >= 0 (within drawnSlop: mm rounding). Before the
// drawn hulls stopped pushing, a hull catching up shoved them back and the
// pair rocked ~0.15 m (players-head-on, e2e).
func TestSolidNoRubberBand(t *testing.T) {
	for _, delay := range []int64{0, drawnDelay, 5, 10, 25} {
		worst, worstAt := 0.0, ""
		for _, skew := range []float64{0, 30, -30} {
			for deg := 0.0; deg < 360; deg += 15 {
				s := authServer(t)
				s.Test = true
				start := time.Now()
				s.lastTickAt = start
				at := Vec3{-26, 0, 4}
				a := deg * math.Pi / 180
				d := [2]float64{math.Sin(a), -math.Cos(a)}
				yaw := math.Atan2(d[0], d[1])
				py, qy := yaw+math.Pi+skew*math.Pi/180, yaw+skew*math.Pi/180 // skew: each pushes at an angle to the line between them
				p, q := s.join("10.0.0.1"), s.join("10.0.0.2")
				p.Name, p.ready, p.Team = "p", true, "vibe"
				q.Name, q.ready, q.Team = "q", true, "artisan"
				s.teleport(p, at, py, false, 0)
				s.teleport(q, Vec3{at[0] + 3*d[0], 0, at[2] + 3*d[1]}, qy, false, 0)
				p.inv, q.inv = s.Weapons.Loadout(p.Team), s.Weapons.Loadout(q.Team)
				// Wish directions (sim: forward is (-sin yaw, -cos yaw)).
				pw := [2]float64{-math.Sin(py), -math.Cos(py)}
				qw := [2]float64{-math.Sin(qy), -math.Cos(qy)}
				for tick := 1; tick <= 200; tick++ {
					pp, qp := p.Pos, q.Pos
					vt := float64(s.tick - delay)
					s.queueCmds(p, []Cmd{{Seq: tick, F: 1, Yaw: py, VT: vt}})
					s.queueCmds(q, []Cmd{{Seq: tick, F: 1, Yaw: qy, VT: vt}})
					s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
					for _, m := range []struct {
						n      string
						a, b   Vec3
						wx, wz float64
					}{{"p", pp, p.Pos, pw[0], pw[1]}, {"q", qp, q.Pos, qw[0], qw[1]}} {
						if back := -((m.b[0]-m.a[0])*m.wx + (m.b[2]-m.a[2])*m.wz); back > worst {
							worst, worstAt = back, fmt.Sprintf("%v° skew %v tick %d %s", deg, skew, tick, m.n)
						}
					}
				}
			}
		}
		t.Logf("pushing into each other, view delay %2d ticks: worst backward step %.4f m (%s)", delay, worst, worstAt)
		if worst > 0.0015 { // drawnSlop: mm rounding
			t.Errorf("delay %d: moved %.4f m backwards in one tick while pushing forward (%s): rubber banding", delay, worst, worstAt)
		}
	}
}

// The crossing cost of the core: a target runs flat out across bob's path
// (as drawn, delay ticks behind) while bob runs into it, straight and at
// ±30°, from every timing. The invisible wall is the largest gap between
// bob's hull and her drawn model at a tick his run was stopped short of it
// (her core, where she really is, reaching ahead of the model he sees).
// Stopped: he lost speed along his run that tick (re-accelerating after
// sliding off her is not a stop, though he is slow then too).
func TestSolidCrossingWall(t *testing.T) {
	for _, delay := range []int64{3, 4, 5, 6, drawnDelay, 25} {
		worst, at, speed := 0.0, "", 0.0
		for _, skew := range []float64{0, 30, -30} {
			for lead := -60; lead <= 60; lead += 2 {
				s := authServer(t)
				s.Test = true
				start := time.Now()
				s.lastTickAt = start
				tg := s.join("10.0.0.1")
				tg.Name, tg.ready, tg.Team = "alice", true, "vibe"
				tyaw := -math.Pi / 2 // runs +x
				s.teleport(tg, Vec3{-31, 0, 4}, tyaw, false, 0)
				tg.inv = s.Weapons.Loadout(tg.Team)
				bob := s.join("10.0.0.2")
				bob.Name, bob.ready, bob.Team = "bob", true, "artisan"
				byaw := skew * math.Pi / 180 // 0: runs -z, across her line
				s.teleport(bob, Vec3{-26 + 3.5*math.Sin(byaw), 0, 4 + 3.5*math.Cos(byaw)}, byaw, false, 0)
				bob.inv = s.Weapons.Loadout(bob.Team)
				fx, fz := -math.Sin(byaw), -math.Cos(byaw)
				prev := 0.0 // his progress along his run the tick before
				for tick := 1; tick <= 220; tick++ {
					vt := float64(s.tick - delay)
					s.queueCmds(tg, []Cmd{{Seq: tick, F: 1, Yaw: tyaw, VT: vt}})
					f := 0.0
					if tick >= 60+lead {
						f = 1
					}
					s.queueCmds(bob, []Cmd{{Seq: tick, F: f, Yaw: byaw, VT: vt}})
					b0, t0 := bob.Pos, tg.Pos
					s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
					if tick > 5 {
						speed = math.Max(speed, math.Hypot(tg.Pos[0]-t0[0], tg.Pos[2]-t0[2])*SimHz)
					}
					d, ok := s.history.At(vt)[tg.ID]
					if !ok || f == 0 || tick < 60+lead+20 {
						prev = (bob.Pos[0]-b0[0])*fx + (bob.Pos[2]-b0[2])*fz
						continue
					}
					moved := (bob.Pos[0]-b0[0])*fx + (bob.Pos[2]-b0[2])*fz
					blocked := moved < 0.75*prev // lost a quarter of his speed this tick: something stopped him (not re-accelerating after a slide)
					prev = moved
					gap := math.Max(math.Abs(bob.Pos[0]-d.Pos[0]), math.Abs(bob.Pos[2]-d.Pos[2])) - 2*sim.R
					if blocked && gap > 0 && gap > worst { // his run was stopped short of her drawn model
						worst, at = gap, fmt.Sprintf("skew %+.0f° bob %+d ticks, tick %d", skew, lead, tick)
					}
				}
			}
		}
		t.Logf("crossing a %.1f m/s runner, view delay %2d ticks: invisible wall up to %.3f m (%s)", speed, delay, worst, at)
		if delay <= 6 && worst > 0.2 { // a clean link draws 30 ms back; 60 ms with ~40 ms of jitter
			t.Errorf("view delay %d ticks: an invisible wall %.3f m ahead of the drawn runner (over 0.2 m)", delay, worst)
		}
	}
}
