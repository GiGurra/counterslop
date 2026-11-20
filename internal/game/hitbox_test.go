package game

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	b, err := os.ReadFile("../../web/assets/models/player.json")
	if err != nil {
		panic(err)
	}
	if err := LoadModel(b); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// The server poses every model part exactly where the client draws it.
func TestPoseParityWithJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	r := rand.New(rand.NewPCG(3, 4))
	type jsState struct {
		P      [3]float64 `json:"p"`
		Yaw    float64    `json:"yaw"`
		Pitch  float64    `json:"pitch"`
		Crouch bool       `json:"crouch"`
		Team   string     `json:"team"`
		ID     int        `json:"id"`
	}
	var states []jsState
	teams := []string{"", "vibe", "artisan"}
	for i := range 60 {
		states = append(states, jsState{P: [3]float64{r.Float64()*60 - 30, r.Float64() * 3, r.Float64()*60 - 30}, Yaw: r.Float64()*12 - 6, Pitch: r.Float64()*3 - 1.5, Crouch: r.IntN(2) == 0,
			Team: teams[i%3], ID: r.IntN(500)})
	}
	in, _ := json.Marshal(states)
	f := filepath.Join(t.TempDir(), "states.json")
	_ = os.WriteFile(f, in, 0o644)
	cmd := exec.Command(node, "testdata/pose.mjs", f)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var js [][][8]Vec3
	if err := json.Unmarshal(out, &js); err != nil {
		t.Fatal(err)
	}
	for si, st := range states {
		ps := PoseState{Pos: Vec3(st.P), Yaw: st.Yaw, Pitch: st.Pitch, Crouch: st.Crouch, Team: st.Team, ID: st.ID}
		parts := PlayerModel.PartsFor(ps)
		if len(parts) != len(js[si]) {
			t.Fatalf("state %d (%s #%d): go has %d parts, js %d", si, st.Team, st.ID, len(parts), len(js[si]))
		}
		for pi := range parts {
			got := PartCorners(&parts[pi], ps)
			for k := range 8 {
				for a := range 3 {
					if math.Abs(got[k][a]-js[si][pi][k][a]) > 2e-4 { // the client's matrices are float32
						t.Fatalf("state %d part %s corner %d: go %v js %v", si, parts[pi].Name, k, got[k], js[si][pi][k])
					}
				}
			}
		}
	}
}

// Ray-trace each hittable part's world centre from all around (and above), at
// awkward yaws and pitches, standing and crouched: the ray hits that part's
// region unless another part is in front (then it must hit before the centre).
func TestModelHitsFromEveryAngle(t *testing.T) {
	dirs := []Vec3{{0, 0, -1}, {0, 0, 1}, {1, 0, 0}, {-1, 0, 0}, {0, -1, 0}, {0.6, -0.3, 0.74}, {-0.5, -0.5, -0.7}}
	heads := 0
	for _, team := range []string{"", "vibe", "artisan"} {
		for _, crouch := range []bool{false, true} {
			for _, yaw := range []float64{0, 0.7, 2.4, -1.9} {
				st := PoseState{Pos: Vec3{3, 0, -2}, Yaw: yaw, Pitch: -0.4, Crouch: crouch, Team: team, ID: 7}
				parts := PlayerModel.PartsFor(st)
				for pi := range parts {
					p := &parts[pi]
					if p.Hit != nil && !*p.Hit {
						continue
					}
					cs := PartCorners(p, st)
					var ctr Vec3
					for _, c := range cs {
						for a := range 3 {
							ctr[a] += c[a] / 8
						}
					}
					for _, dir := range dirs {
						l := math.Sqrt(dir[0]*dir[0] + dir[1]*dir[1] + dir[2]*dir[2])
						d := Vec3{dir[0] / l, dir[1] / l, dir[2] / l}
						o := Vec3{ctr[0] - d[0]*8, ctr[1] - d[1]*8, ctr[2] - d[2]*8}
						h, tt, ok := RayModel(&PlayerModel, st, o, d)
						if !ok || tt > 8 {
							t.Fatalf("team=%q crouch=%v yaw=%v part %s dir %v: miss", team, crouch, yaw, p.Name, dir)
						}
						if p.Name == "head" && h.Part == "head" {
							heads++
						}
					}
				}
			}
		}
	}
	if heads < 150 { // head centre from 7 directions x 8 poses x 3 teams; only a few parts may be in front
		t.Fatalf("head hits %d", heads)
	}
	if h, _, _ := RayModel(&PlayerModel, PoseState{}, Vec3{0, 1.58, 8}, Vec3{0, 0, -1}); h.Mult != 4 {
		t.Fatalf("head multiplier %v", h.Mult)
	}
}
