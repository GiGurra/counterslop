package sim

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gigurra/counterslop/internal/content/maps"
)

type tick struct {
	F      float64  `json:"f"`
	R      float64  `json:"r"`
	Yaw    *float64 `json:"yaw,omitempty"`
	Walk   bool     `json:"walk,omitempty"`
	Jump   bool     `json:"jump,omitempty"`
	Crouch bool     `json:"crouch,omitempty"`
}

type run struct {
	Spawn Vec3    `json:"spawn"`
	Yaw   float64 `json:"yaw"`
	Speed float64 `json:"speed"` // held weapon's speed scale
	Ticks []tick  `json:"ticks"`
}

type sample struct {
	P Vec3    `json:"p"`
	V Vec3    `json:"v"`
	G bool    `json:"g"`
	C bool    `json:"c"`
	J bool    `json:"j"`
	L float64 `json:"l"`
	S int     `json:"s"`
}

// randomRuns makes seeded input scripts that walk, run, strafe, turn, jump,
// crouch and crouch-jump around de_slop, including into walls and up steps.
func randomRuns(spawns []Vec3) []run {
	r := rand.New(rand.NewPCG(1, 2))
	var runs []run
	for i, sp := range spawns {
		rn := run{Spawn: sp, Yaw: r.Float64() * 6.28, Speed: []float64{1, 0.86, 0.8, 0.96}[i%4]}
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
			if r.IntN(25) == 0 { // crouch-jump: crouch while airborne
				tk.Crouch = true
			}
			rn.Ticks = append(rn.Ticks, tk)
		}
		runs = append(runs, rn)
		_ = i
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
	const dt = 0.01 // the 100 Hz sim tick
	runs := randomRuns(spawns)
	c, _ := json.Marshal(map[string]any{"map": m, "dt": dt, "runs": runs})
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

	w := &World{Boxes: m.Boxes}
	for ri, rn := range runs {
		b := NewBody(rn.Spawn)
		b.Yaw = rn.Yaw
		for ti, tk := range rn.Ticks {
			if tk.Yaw != nil {
				b.Yaw = *tk.Yaw
			}
			ev := w.Step(b, Input{F: tk.F, R: tk.R, Walk: tk.Walk, Jump: tk.Jump, Crouch: tk.Crouch, Speed: rn.Speed}, dt)
			want := js[ri][ti]
			for a := 0; a < 3; a++ {
				if math.Abs(b.Pos[a]-want.P[a]) > 0 || math.Abs(b.Vel[a]-want.V[a]) > 0 {
					t.Fatalf("run %d tick %d: go pos %v vel %v, js pos %v vel %v", ri, ti, b.Pos, b.Vel, want.P, want.V)
				}
			}
			if b.OnGround != want.G || b.Crouched != want.C || ev.Jumped != want.J || ev.Steps != want.S || math.Abs(ev.Landed-want.L) > 1e-6 {
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
	if jumps < 20 || crouchJumps < 5 {
		t.Fatalf("weak coverage: %d jumps, %d crouch-jumps", jumps, crouchJumps)
	}
}
