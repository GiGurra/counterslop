package sim

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

type tick struct {
	F      float64  `json:"f"`
	R      float64  `json:"r"`
	Yaw    *float64 `json:"yaw,omitempty"`
	Walk   bool     `json:"walk,omitempty"`
	Jump   bool     `json:"jump,omitempty"`
	Crouch bool     `json:"crouch,omitempty"`
	Held   bool     `json:"held,omitempty"`  // jump held (MoveRules.AutoBhop)
	Pitch  float64  `json:"pitch,omitempty"` // the view's pitch (swimming goes along it)
	Hook   bool     `json:"hook,omitempty"`  // the hook button held (hook.go)
	Press  bool     `json:"press,omitempty"` // ... pressed this command
}

type run struct {
	Spawn Vec3    `json:"spawn"`
	Yaw   float64 `json:"yaw"`
	Speed float64 `json:"speed"` // held weapon's speed scale
	Ticks []tick  `json:"ticks"`
	Peers []Peer  `json:"peers"` // other players standing around the spawn (solid)
	Grav  int     `json:"grav"`  // the body's gravity (gravity.go)
	Boots bool    `json:"boots"` // wears the Gravity Override (hook.go), a full meter
}

type sample struct {
	P  Vec3    `json:"p"`
	V  Vec3    `json:"v"`
	G  bool    `json:"g"`
	C  bool    `json:"c"`
	J  bool    `json:"j"`
	L  float64 `json:"l"`
	S  int     `json:"s"`
	W  float64 `json:"w"` // splash
	H  int     `json:"h"` // the hook event
	Gv int     `json:"gv"`
	E  float64 `json:"e"` // energy
}

// randomRuns makes seeded input scripts that walk, run, strafe, turn, jump,
// crouch and crouch-jump around de_slop, including into walls and up steps.
func randomRuns(spawns []Vec3) []run {
	r := rand.New(rand.NewPCG(1, 2))
	var runs []run
	for i, sp := range spawns {
		rn := run{Spawn: sp, Yaw: r.Float64() * 6.28, Speed: []float64{1, 0.86, 0.8, 0.96}[i%4]}
		for k := range 4 { // players to bump into, stand on and crouch under (a few overlap the spawn: walk out)
			a := r.Float64() * 6.28
			d := 0.5 + float64(k)*0.6
			h := H
			if k%2 == 1 {
				h = HCrouch
			}
			rn.Peers = append(rn.Peers, Peer{Pos: Vec3{sp[0] + d*math.Cos(a) + 0.0004, sp[1], sp[2] + d*math.Sin(a) - 0.0007}, H: h})
		}
		var cur tick
		for t := 0; t < 1500; t++ {
			if t%40 == 0 { // new intent every 0.4 s
				cur = tick{F: float64(r.IntN(3) - 1), R: float64(r.IntN(3) - 1), Walk: r.IntN(5) == 0, Crouch: r.IntN(5) == 0}
				y := r.Float64() * 6.28
				cur.Yaw = &y
			}
			tk := cur
			if t%40 != 0 {
				tk.Yaw = nil
			}
			tk.Jump = r.IntN(30) == 0
			tk.Held = (t/40)%3 == 0 // no draw from r: the classic runs stay as they were
			tk.Pitch = 1.2 * math.Sin(float64(t)*0.011+float64(i))
			if r.IntN(25) == 0 { // crouch-jump: crouch while airborne
				tk.Crouch = true
			}
			rn.Ticks = append(rn.Ticks, tk)
		}
		runs = append(runs, rn)
	}
	return runs
}

func TestParityWithJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	m := maps.DeSlop()
	spawns := make([]Vec3, len(m.Spawns))
	for i, s := range m.Spawns {
		spawns[i] = s
	}
	parity(t, node, m, spawns, 20, 5, MoveRules{})
	// The source model with auto bhop and a surf server's air accelerate.
	parity(t, node, m, spawns, 20, 5, MoveRules{Model: MoveSource, AirAccel: 150, AutoBhop: true})
	// gs_megabyte from its escalators' feet and tops: riding, walking with and
	// against the belts, jumping and crouching on them.
	mb := maps.GsMegabyte()
	var esc []Vec3
	for _, mv := range mb.Movers {
		x := (mv.Min[0] + mv.Max[0]) / 2
		esc = append(esc, Vec3{x, mv.Min[1] + mv.Rise, mv.Min[2] + 0.3}, Vec3{x, mv.Max[1], mv.Max[2] - 0.3})
	}
	parity(t, node, mb, esc, 10, 2, MoveRules{})
	parity(t, node, mb, esc, 10, 2, MoveRules{Model: MoveSource, AutoBhop: true})
	// lab_slopes from the feet of its ramps, the deck, the surf ridge's
	// sides and the target ramp: up, down, across, off and sliding.
	ls := maps.LabSlopes()
	slopes := []Vec3{{-25, 0, -24.5}, {-25, 0, -19.5}, {-25, 0, -14.5}, {-25, 0, -9.5}, {-21, 2.9, -9.5},
		{-3, 2, -22}, {1.5, 1.2, -24}, {-4, 1, -16}, {12, 2.5, -10}, {16.2, 1.5, 0}, {0, 0.5, 11}, {2, 0, 8}}
	parity(t, node, ls, slopes, 10, 2, MoveRules{})
	parity(t, node, ls, slopes, 10, 2, MoveRules{Model: MoveSource, AutoBhop: true})
	// lab_harbor: in the basin, at the quay's edge (in and out: water jumps),
	// on the pier and the slipway (wading).
	lh := maps.LabHarbor()
	harbor := []Vec3{{-10, -2, 0}, {10, -3.5, -20}, {0, -1, -20}, {-15, 0, 10.5}, {12, 0, 10.5}, {0, 0, 5}, {24, -1.5, 5}, {24, -3.9, 1}}
	parity(t, node, lh, harbor, 5, 1, MoveRules{})
	parity(t, node, lh, harbor, 5, 1, MoveRules{Model: MoveSource, AutoBhop: true})
}

func parity(t *testing.T, node string, m content.Map, spawns []Vec3, minJumps, minCrouchJumps int, move MoveRules) {
	t.Helper()
	parityRuns(t, node, m, randomRuns(spawns), minJumps, minCrouchJumps, 50, move)
}

func parityRuns(t *testing.T, node string, m content.Map, runs []run, minJumps, minCrouchJumps, minTouches int, move MoveRules) {
	t.Helper()
	const dt = 0.01 // the 100 Hz sim tick
	hk := move.Hook
	c, _ := json.Marshal(map[string]any{"map": m, "dt": dt, "runs": runs, "move": map[string]any{"model": move.Model, "airAccel": move.AirAccel, "autoBhop": move.AutoBhop,
		"hook": map[string]any{"range": hk.Range, "energy": hk.Energy, "recharge": hk.Recharge, "drain": hk.Drain, "cost": hk.Cost, "pull": hk.Pull}}})
	f := filepath.Join(t.TempDir(), "case.json")
	if err := os.WriteFile(f, c, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "testdata/parity.mjs", f)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var js [][]sample
	if err := json.Unmarshal(out, &js); err != nil {
		t.Fatal(err)
	}

	w := NewWorld(&m)
	w.Move = move
	hookEvents, moverHooks = map[int]int{}, map[int]int{}
	touches, ramped, swum := 0, 0, 0
	hasRamps := false
	for i := range m.Boxes {
		hasRamps = hasRamps || IsWedge(&m.Boxes[i])
	}
	for ri, rn := range runs {
		w.Peers = rn.Peers
		b := NewBody(rn.Spawn)
		b.Yaw = rn.Yaw
		b.Grav = rn.Grav
		if rn.Boots {
			b.Boots, b.Energy = true, move.Hook.Energy
		}
		for ti, tk := range rn.Ticks {
			if tk.Yaw != nil {
				b.Yaw = *tk.Yaw
			}
			b.Pitch = tk.Pitch        // the view (the hook aims along it)
			w.T = float64(ti+1) / 100 // the movers' clock (CmdTime with tick base 0)
			onMover := b.Hook.Mover > 0
			ev := w.Step(b, Input{F: tk.F, R: tk.R, Walk: tk.Walk, Jump: tk.Jump, Crouch: tk.Crouch, Speed: rn.Speed, JumpHeld: tk.Held, Pitch: tk.Pitch, Hook: tk.Hook, HookPress: tk.Press}, dt)
			hookEvents[ev.Hook]++
			if b.Hook.Mover > 0 || onMover {
				moverHooks[ev.Hook]++ // latched to a mover, or was until this command
			}
			if w.Touching(b, 1e-6) {
				touches++
			}
			if w.WaterLevel(b) >= 2 {
				swum++
			}
			if b.Grav == GravDown && w.onWedge(b.Pos) {
				ramped++
			}
			want := js[ri][ti]
			for a := 0; a < 3; a++ {
				if math.Abs(b.Pos[a]-want.P[a]) > 0 || math.Abs(b.Vel[a]-want.V[a]) > 0 {
					t.Fatalf("run %d tick %d: go pos %v vel %v, js pos %v vel %v", ri, ti, b.Pos, b.Vel, want.P, want.V)
				}
			}
			if b.OnGround != want.G || b.Crouched != want.C || ev.Jumped != want.J || ev.Steps != want.S || math.Abs(ev.Landed-want.L) > 1e-6 || ev.Splash != want.W ||
				ev.Hook != want.H || b.Grav != want.Gv || b.Energy != want.E {
				t.Fatalf("run %d tick %d: go %+v %+v ground=%v crouch=%v, js %+v", ri, ti, ev, b.Pos, b.OnGround, b.Crouched, want)
			}
		}
	}
	// The runs must actually exercise the interesting cases.
	jumps, crouchJumps := 0, 0
	for _, traj := range js {
		for i, s := range traj {
			if s.J {
				jumps++
			}
			if i > 0 && s.C && !traj[i-1].C && !traj[i-1].G {
				crouchJumps++
			}
		}
	}
	if len(m.Water) > 0 && swum < 2000 {
		t.Fatalf("weak coverage: swimming on %d ticks", swum)
	}
	if hasRamps && len(m.Water) == 0 && ramped < 120 { // (lab_harbor's one slipway: its swimming counts instead)
		t.Fatalf("weak coverage: on a slope on %d ticks", ramped)
	}
	if touches < minTouches {
		t.Fatalf("weak coverage: touched a peer on %d ticks", touches)
	}
	if jumps < minJumps || crouchJumps < minCrouchJumps {
		t.Fatalf("weak coverage: %d jumps, %d crouch-jumps", jumps, crouchJumps)
	}
}

// hookEvents: the hook events of the last parityRuns, by kind.
var hookEvents map[int]int
