package game

import (
	"math"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

const gaitDT = 1.0 / SimHz

// runGait steps g n command ticks at v m/s along -Z.
func runGait(g *Gait, v float64, n int, onGround, crouched bool) *Gait {
	for range n {
		g.Step(0, 0, -v*gaitDT, onGround, crouched, 1)
	}
	return g
}

var unitK = GaitK{Swing: 1, Waddle: 1, Bob: 1, Wobble: 1, Lean: 1}

func poseMag(p GaitPose) float64 {
	return max(math.Abs(p.FootL.Z), math.Abs(p.FootL.Y), math.Abs(p.FootR.Z), math.Abs(p.LegsRoll), math.Abs(p.BodyY), math.Abs(p.BodyPitch))
}

func TestGaitPhaseFollowsDistanceNotTime(t *testing.T) {
	g := runGait(&Gait{}, 7, 200, true, false)
	before := g.Phase
	runGait(g, 0, 50, true, false)
	if d := g.Phase - before; d != 0 {
		t.Fatalf("phase moved %v while standing", d)
	}
	g = runGait(&Gait{}, 7, 300, true, false)
	p0 := g.Phase
	runGait(g, 7, 100, true, false)
	d1 := g.Phase - p0
	runGait(g, 7, 200, true, false)
	d2 := g.Phase - p0 - d1
	if math.Abs(d2/d1-2) > 0.01 {
		t.Fatalf("twice the distance should be twice the phase: ratio %v", d2/d1)
	}
}

func TestGaitPlantedFootDoesNotSkate(t *testing.T) {
	g := runGait(&Gait{}, 7, 400, true, false)
	bestC, bestV := 2.0, 0.0
	for range 200 {
		z0 := PoseOf(g, 0, unitK).FootL.Z
		g.Step(0, 0, -7*gaitDT, true, false, 1)
		z1 := PoseOf(g, 0, unitK).FootL.Z
		if c := math.Cos(g.Phase); c < bestC { // mid-stance: cos = -1
			bestC, bestV = c, (z1-z0)/gaitDT
		}
	}
	if math.Abs(bestV-7) > 0.5 { // the foot slides back at about the body's speed: still in the world
		t.Fatalf("planted foot relative speed %v, want ~7", bestV)
	}
}

func TestGaitAmplitudeAtRestAndAfterRunning(t *testing.T) {
	idle := runGait(&Gait{}, 0, 100, true, false)
	if idle.Amp != 0 || poseMag(PoseOf(idle, 3, unitK)) > 0.06 {
		t.Fatalf("at rest: amp %v, pose %+v: only an idle sway", idle.Amp, PoseOf(idle, 3, unitK))
	}
	g := runGait(&Gait{}, 7, 200, true, false)
	if g.Amp < 0.9 {
		t.Fatalf("running amp %v", g.Amp)
	}
	runGait(g, 0, 300, true, false)
	if g.Amp > 0.01 || math.Abs(PoseOf(g, 0, unitK).FootL.Z) > 0.01 {
		t.Fatalf("after stopping: amp %v", g.Amp)
	}
}

func TestGaitSwingBoundedWalkLessThanRun(t *testing.T) {
	peak := func(v float64, crouched bool) float64 {
		g := runGait(&Gait{}, v, 300, true, crouched)
		m := 0.0
		for range 300 {
			g.Step(0, 0, -v*gaitDT, true, crouched, 1)
			m = max(m, math.Abs(PoseOf(g, 0, unitK).FootL.Z))
		}
		return m
	}
	walk, run, fast := peak(3.5, false), peak(7, false), peak(40, false)
	if walk > run*0.7 || run > gaitSwing*1.0001 || fast > gaitSwing*1.0001 {
		t.Fatalf("walk %v run %v fast %v (max %v)", walk, run, fast, gaitSwing)
	}
	if sh := peak(2.3, true); sh < 0.05 || sh > run*0.6 {
		t.Fatalf("a crouch-walk takes short steps: %v (run %v)", sh, run)
	}
}

func TestGaitNeverPops(t *testing.T) {
	g := &Gait{}
	prev := PoseOf(g, 0, unitK).FootL
	for i := range 600 {
		v := map[bool]float64{true: 7, false: 3.5}[i < 400]
		if i < 200 {
			v = 0
		}
		g.Step(0, 0, -v*gaitDT, true, i > 500, 1)
		f := PoseOf(g, float64(i)*gaitDT, unitK).FootL
		if math.Abs(f.Z-prev.Z) > 0.075 || math.Abs(f.Y-prev.Y) > 0.075 {
			t.Fatalf("tick %d: foot jumped %+v -> %+v", i, prev, f)
		}
		prev = f
	}
}

func TestGaitTeleportAndNonsenseReset(t *testing.T) {
	g := runGait(&Gait{}, 7, 200, true, false)
	if g.Phase == 0 || g.Amp < 0.5 {
		t.Fatal("setup")
	}
	g.Step(gaitTeleport*gaitDT*2, 0, 0, true, false, 1)
	if *g != (Gait{}) {
		t.Fatalf("a teleport keeps %+v", *g)
	}
	runGait(g, 7, 100, true, false)
	g.Step(0, 50, 0, true, false, 1) // a vertical warp
	if *g != (Gait{}) {
		t.Fatalf("a vertical warp keeps %+v", *g)
	}
	runGait(g, 7, 100, true, false)
	g.Step(math.NaN(), 0, 0, true, false, 1)
	if *g != (Gait{}) {
		t.Fatalf("NaN keeps %+v", *g)
	}
}

func TestGaitAirborneStopsTheStride(t *testing.T) {
	g := runGait(&Gait{}, 7, 200, false, false)
	if g.Air < 0.99 {
		t.Fatalf("air %v", g.Air)
	}
	p := g.Phase
	runGait(g, 7, 50, false, false)
	if math.Abs(g.Phase-p) > 1e-9 {
		t.Fatal("no stepping in the air")
	}
	if f := PoseOf(g, 1.234, unitK).FootL; f.Z == 0 && f.Y == 0 {
		t.Fatal("feet flail in the air")
	}
}

func TestLerpGaitWrapsForward(t *testing.T) {
	a, b := &Gait{Phase: gaitPeriod - 1, Amp: 0}, &Gait{Phase: 1, Amp: 1}
	g := LerpGait(a, b, 0.75)
	if math.Abs(g.Phase-0.5) > 1e-9 || g.Amp != 0.75 {
		t.Fatalf("lerp %+v", *g)
	}
	if LerpGait(nil, b, 0.5) != b {
		t.Fatal("a player with no earlier gait takes the later one")
	}
}

func TestGaitKOfSeededPerPlayer(t *testing.T) {
	if GaitKOf(3) != GaitKOf(3) || GaitKOf(3) == GaitKOf(4) {
		t.Fatal("factors must be seeded by id and differ between players")
	}
}

// footRay is a ray from the player's left (-X) side, level, through where the
// left foot's centre is at gait g: it meets footL first while the foot is there.
func footRay(st PoseState) (Vec3, Vec3) {
	fr := PlayerModel.Frames(st)
	var c Vec3
	for i := range PlayerModel.Parts {
		if PlayerModel.Parts[i].Name == "footL" {
			c, _, _ = PlayerModel.Parts[i].box(false)
		}
	}
	x := fr[GroupFootL]
	lc := Vec3{x.R[0]*c[0] + x.R[1]*c[1] + x.R[2]*c[2] + x.T[0], x.R[3]*c[0] + x.R[4]*c[1] + x.R[5]*c[2] + x.T[1], x.R[6]*c[0] + x.R[7]*c[1] + x.R[8]*c[2] + x.T[2]}
	// st has yaw 0 at the origin: player frame = world frame
	return Vec3{-5, lc[1], lc[2]}, Vec3{1, 0, 0}
}

// stridePose is a full-speed stride with the left foot forward (sin(phase+offset) = 1)
// or back (-1).
func stridePose(id int, forward bool) PoseState {
	ph := math.Pi/2 - GaitKOf(id).Offset
	if !forward {
		ph += math.Pi
	}
	ph = math.Mod(ph+2*gaitPeriod, gaitPeriod)
	return PoseState{ID: id, Team: content.Vibe, Gait: &Gait{Phase: ph, Amp: 1}, T: 10}
}

// The hit pose is the drawn pose: a ray through the swung-forward left foot
// hits it, the same ray half a stride later (the foot back) does not.
func TestSwingingFootIsHitOrMissedByPhase(t *testing.T) {
	fwd := stridePose(5, true)
	o, d := footRay(fwd)
	if h, _, ok := RayModel(&PlayerModel, fwd, o, d); !ok || h.Name != "footL" {
		t.Fatalf("foot forward: hit %v %+v, want footL", ok, h)
	}
	back := stridePose(5, false)
	if h, _, ok := RayModel(&PlayerModel, back, o, d); ok && h.Name == "footL" {
		t.Fatal("foot back: the ray where it was still hits it")
	}
}

// Lag compensation rewinds the gait with the position: a shot traced at the
// tick the shooter saw meets the foot where it was drawn then.
func TestLagCompRewindsTheGait(t *testing.T) {
	s := authServer(t)
	b := s.join("2.2.2.2")
	b.ready, b.Team = true, content.Artisan
	b.Pos, b.Yaw = Vec3{}, 0
	fwd, back := stridePose(b.ID, true), stridePose(b.ID, false)
	b.Gait = *fwd.Gait
	s.tick = 30
	s.recordHistory()
	seen := s.history.At(30)[b.ID]
	if seen.Gait == nil || seen.T != 30.0/SimHz {
		t.Fatalf("history pose %+v: no gait or time", seen)
	}
	b.Gait = *back.Gait
	s.tick = 33
	s.recordHistory()
	s.tick = 36

	o, d := footRay(fwd)
	rew := s.history.At(30)[b.ID]
	if h, _, ok := RayModel(&PlayerModel, rew, o, d); !ok || h.Name != "footL" {
		t.Fatalf("rewound to the seen tick: hit %v %+v, want footL", ok, h)
	}
	if h, _, ok := RayModel(&PlayerModel, poseOf(b, s.poseTime()), o, d); ok && h.Name == "footL" {
		t.Fatal("not rewound, the foot has moved on: still footL")
	}
	// between the two snapshots the phase runs forward, half way at 31.5
	mid := s.history.At(31.5)[b.ID]
	if want := math.Mod(fwd.Gait.Phase+math.Pi/2, gaitPeriod); math.Abs(mid.Gait.Phase-want) > 1.0/gaitQ || mid.T != 31.5/SimHz {
		t.Fatalf("mid phase %v (t %v), want %v", mid.Gait.Phase, mid.T, want)
	}
}

// The server steps the gait per command from the body's movement.
func TestRunCmdStepsTheGait(t *testing.T) {
	s := authServer(t)
	a := s.join("1.1.1.1")
	a.ready, a.Team = true, content.Vibe
	s.respawn(a)
	for i := range 60 {
		s.queueCmds(a, []Cmd{{Seq: i + 1, F: 1}})
		stepSim(s)
	}
	if a.Gait.Amp < 0.3 || a.Gait.Phase == 0 {
		t.Fatalf("after running forward: %+v", a.Gait)
	}
	s.teleport(a, Vec3{3, 0, 3}, 0, false, 60)
	if a.Gait != (Gait{}) {
		t.Fatalf("a teleport keeps %+v", a.Gait)
	}
}

// The gait travels in full snapshots only: the history records it on full
// ticks and interpolates between them (net.js gaitAt does the same).
func TestHistoryGaitBetweenFullSnapshots(t *testing.T) {
	var h History
	g := func(ph float64) *Gait { return &Gait{Phase: ph, Amp: 1} }
	h.Record(30, map[int]PoseState{2: {Gait: g(2)}})
	h.Record(31, map[int]PoseState{2: {}}) // pose snapshots: no gait
	h.Record(32, map[int]PoseState{2: {}})
	h.Record(33, map[int]PoseState{2: {Gait: g(2.6)}})
	h.Record(34, map[int]PoseState{2: {}})
	if got := h.At(31)[2].Gait.Phase; math.Abs(got-2.2) > 1e-9 {
		t.Fatalf("At(31) phase %v, want 2.2", got)
	}
	if got := h.At(34)[2].Gait.Phase; got != 2.6 {
		t.Fatalf("past the newest full frame: %v, want 2.6", got)
	}
	if got := h.At(29)[2].Gait.Phase; got != 2 {
		t.Fatalf("before the oldest: %v, want 2", got)
	}
}

// Practice bots don't run commands: tickBots steps their gait as they walk.
func TestPracticeBotGaitSteps(t *testing.T) {
	s := authServer(t)
	bot := s.addBot()
	bot.brain.idle, bot.brain.patrol = 0, &[2]Vec3{{-14, 0, 0}, {-14, 0, 8}}
	bot.Pos = Vec3{-14, 0, 0}
	bot.Gait = Gait{}
	for range 100 {
		s.tickBots(1.0 / SimHz)
	}
	if bot.Gait.Amp < 0.3 || bot.Gait.Phase == 0 {
		t.Fatalf("a walking bot's gait: %+v (pos %v)", bot.Gait, bot.Pos)
	}
}
