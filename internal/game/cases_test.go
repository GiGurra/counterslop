package game

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Declarative command → simulation → state cases (sim/cases/*.json), run here
// through the server's real tick (queueCmds, then advance: commands, items, history) and by
// web/game/cases.test.mjs through the client's predictor (localsim.js). The
// format is documented in web/game/cases.js, which also runs them.

// CASES_DIR overrides where the cases are read from (cmd/simdiff runs the head's
// cases against an older tree); CASES_TRACE=<dir> writes a per-tick state trace
// per case (<dir>/<case>.go.json) for cmd/simdiff.
func casesDir() string {
	if d := os.Getenv("CASES_DIR"); d != "" {
		return d
	}
	return "../../sim/cases"
}

type caseFile struct {
	Name    string                       `json:"name"`
	Levels  []string                     `json:"levels"`
	Map     string                       `json:"map"` // "" = de_slop
	Players map[string]casePlayer        `json:"players"`
	Script  []map[string]json.RawMessage `json:"script"`
}

type casePlayer struct {
	Team   string           `json:"team"`
	Pos    Vec3             `json:"pos"`
	Yaw    float64          `json:"yaw"`
	Pitch  float64          `json:"pitch"`
	Crouch bool             `json:"crouch"`
	Grav   int              `json:"grav"`  // the gravity it stands in (sim.Grav*)
	Boots  bool             `json:"boots"` // wears the Gravity Override (sim hook.go), a full meter
	Slots  map[string]*Held `json:"slots"`
	Cur    string           `json:"cur"`
	Boost  int64            `json:"boost"` // energy drink: the first n commands move faster
}

type caseInput struct {
	F     float64  `json:"f"`
	R     float64  `json:"r"`
	Yaw   *float64 `json:"yaw"`
	Pitch *float64 `json:"pitch"`
	B     []string `json:"b"`
	W     string   `json:"w"`
	Drop  bool     `json:"drop"`
	Act   []string `json:"act"`
	Tap   []string `json:"tap"`
	Hold  []string `json:"hold"`
	Exec  string   `json:"exec"`
}

var caseButtons = map[string]int{"jump": BtnJump, "crouch": BtnCrouch, "walk": BtnWalk, "fire": BtnFire, "scope": BtnScope, "reload": BtnReload, "use": BtnUse, "alt": BtnAlt, "hook": BtnHook}

// caseFields: every assertable field and who knows it (sim/case-fields.json).
var caseFields = func() map[string]string {
	b, err := os.ReadFile("../../sim/case-fields.json")
	if err != nil {
		panic(err)
	}
	m := map[string]string{}
	json.Unmarshal(b, &m)
	delete(m, "_doc")
	return m
}()

// serverSide: fields the go level (the server) answers.
func serverSide(k string) bool { return caseFields[k] == "sim" || caseFields[k] == "server" }

var whyNot = map[string]bool{"no weapon": true, "reloading": true, "cycling": true, "semi": true, "empty": true, "cooldown": true, "thrown": true, "pin": true, "pinned": true} // a grenade throw (or its pin) is not a shot

// TestCasesAgree runs every go+js case through both the server and the client
// predictor (node web/game/casetrace.mjs) and fails on the first tick where any
// field both sides know differs, expectations or not: the twin sims must agree
// on every tick, not just where a case looks.
func TestCasesAgree(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	dir, _ := filepath.Abs(casesDir())
	cmd := exec.Command(node, "../../web/game/casetrace.mjs", dir)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var js map[string][]map[string]map[string]any
	if err := json.Unmarshal(out, &js); err != nil {
		t.Fatal(err)
	}
	if len(js) == 0 {
		t.Fatal("no js-level cases")
	}
	for name, jtrace := range js {
		b, err := os.ReadFile(filepath.Join(dir, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var c caseFile
		json.Unmarshal(b, &c)
		if len(c.Levels) > 0 && !slices.Contains(c.Levels, "go") {
			continue // nothing to compare with: the server can't run it on its own
		}
		gtrace := runCase(t, c)
		if len(gtrace) != len(jtrace) {
			t.Errorf("%s: %d server ticks, %d client ticks", name, len(gtrace), len(jtrace))
			continue
		}
	tick:
		for i := range gtrace {
			for who, jf := range jtrace[i] {
				gf := gtrace[i][who]
				for k, jv := range jf {
					if caseFields[k] != "sim" {
						continue
					}
					if !sameValue(gf[k], jv) {
						t.Errorf("%s tick %d %s.%s: server %v, client %v", name, i+1, who, k, gf[k], jv)
						break tick
					}
				}
			}
		}
	}
}

// sameValue compares a Go trace value with its JSON-decoded JS counterpart.
func sameValue(g, j any) bool {
	b, _ := json.Marshal(g)
	var gv any
	json.Unmarshal(b, &gv)
	if ga, ok := gv.([]any); ok {
		ja, ok := j.([]any)
		if !ok || len(ga) != len(ja) {
			return false
		}
		for i := range ga {
			if math.Abs(ga[i].(float64)-ja[i].(float64)) > 2e-4 {
				return false
			}
		}
		return true
	}
	return fmt.Sprint(gv) == fmt.Sprint(j)
}

func TestCases(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(casesDir(), "*.json"))
	if len(files) == 0 {
		t.Fatal("no cases in " + casesDir())
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var c caseFile
		if err := json.Unmarshal(b, &c); err != nil {
			t.Errorf("%s: %v", f, err) // this case only: the rest still run (cmd/simdiff runs newer cases on older trees)
			continue
		}
		if len(c.Levels) > 0 && !slices.Contains(c.Levels, "go") {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		t.Run(name, func(t *testing.T) {
			defer func() { // a case that crashes the server fails, not the whole run
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			trace := runCase(t, c)
			if dir := os.Getenv("CASES_TRACE"); dir != "" {
				b, _ := json.Marshal(trace)
				if err := os.WriteFile(filepath.Join(dir, name+".go.json"), b, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// TestCasesDelta runs every go case again with the delta codec on: each tick
// the server broadcasts, every player's snapshots are decoded (the Go twin of
// the client's decoder) and must equal the json codec's full snapshot, and
// the trace must be the json run's (the codec changes nothing in play).
func TestCasesDelta(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(casesDir(), "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var c caseFile
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if len(c.Levels) > 0 && !slices.Contains(c.Levels, "go") {
			continue
		}
		t.Run(strings.TrimSuffix(filepath.Base(f), ".json"), func(t *testing.T) {
			want := runCase(t, c)
			got := runCaseWith(t, c, CodecDelta)
			if len(got) != len(want) {
				t.Fatalf("%d ticks with delta, %d with json", len(got), len(want))
			}
			for i := range got {
				if !reflect.DeepEqual(got[i], want[i]) {
					t.Fatalf("tick %d: delta run %v, json run %v", i+1, got[i], want[i])
				}
			}
		})
	}
}

type casePlayerState struct {
	p     *Player
	yaw   float64
	pitch float64
	seq   int
	shots int
}

// traceTick is every player's state after one tick: player -> field -> value.
type traceTick map[string]map[string]any

func runCase(t *testing.T, c caseFile) []traceTick { return runCaseWith(t, c, CodecJSON) }

// runCaseWith runs case c on a server with that snapshot codec. The json run
// only ticks; a delta run also broadcasts every tick and checks what each
// player decodes against the json codec's snapshot (TestCasesDelta).
func runCaseWith(t *testing.T, c caseFile, codec string) []traceTick {
	var trace []traceTick
	s := authServer(t)
	s.Test = true // every queued command runs in its tick
	if err := s.SetNetCodec(codec); err != nil {
		t.Fatal(err)
	}
	dec := map[*Player]*deltaDecoder{}
	if c.Map != "" {
		if err := s.ResetWorldOn(c.Map); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	s.lastTickAt = start
	names := make([]string, 0, len(c.Players))
	for n := range c.Players {
		names = append(names, n)
	}
	sort.Strings(names)
	ps := map[string]*casePlayerState{}
	for _, n := range names {
		cp := c.Players[n]
		p := s.join("10.0.0." + fmt.Sprint(len(ps)+1))
		p.Name, p.ready = n, true
		p.Team = cp.Team
		if p.Team == "" {
			p.Team = "vibe"
		}
		s.teleport(p, cp.Pos, cp.Yaw, cp.Crouch, 0)
		p.body.Grav, p.Grav = cp.Grav, cp.Grav
		if cp.Boots {
			p.body.Boots, p.body.Energy = true, s.moveRules().Hook.Energy
		}
		inv := s.Weapons.Loadout(p.Team)
		if cp.Slots != nil {
			inv = &Inventory{Slots: map[string]*Held{}, Current: "primary"}
			for k, h := range cp.Slots {
				hh := *h
				inv.Slots[k] = &hh
			}
		}
		if cp.Cur != "" {
			inv.Current = cp.Cur
		}
		inv.drawn(0) // as the client's setInv on a fresh inventory
		if cp.Boost > 0 {
			inv.boostEnd = p.cmdTicks + cp.Boost
		}
		p.inv = inv
		ps[n] = &casePlayerState{p: p, yaw: cp.Yaw, pitch: cp.Pitch}
	}
	tick := 0
	for si, step := range c.Script {
		if raw, ok := step["expect"]; ok {
			var want map[string]map[string]json.RawMessage
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatalf("step %d: %v", si, err)
			}
			for who, fields := range want {
				for k, v := range fields {
					if err := checkField(s, ps[who], who, k, v); err != nil {
						t.Errorf("step %d (tick %d) %s.%s: %v", si, tick, who, k, err)
					}
				}
			}
			continue
		}
		if _, ok := step["wallMs"]; ok {
			continue // e2e only (a stall in real time): no tick here
		}
		n := 1
		if raw, ok := step["n"]; ok {
			json.Unmarshal(raw, &n)
		}
		noBack := -1.0
		if raw, ok := step["noBack"]; ok {
			json.Unmarshal(raw, &noBack)
		}
		inputs := map[string]caseInput{}
		for k, raw := range step {
			if k == "n" || k == "noBack" {
				continue
			}
			if ps[k] == nil {
				t.Fatalf("step %d: unknown key or player %q", si, k)
			}
			var in caseInput
			if err := json.Unmarshal(raw, &in); err != nil {
				t.Fatalf("step %d %s: %v", si, k, err)
			}
			inputs[k] = in
		}
		backed := map[string]bool{}
		for i := range n {
			tick++
			before := map[string]Vec3{}
			for _, name := range names {
				before[name] = ps[name].p.Pos
			}
			for _, name := range names {
				st, in := ps[name], inputs[name]
				if in.Yaw != nil {
					st.yaw = *in.Yaw
				}
				if in.Pitch != nil {
					st.pitch = *in.Pitch
				}
				if len(in.Act) > 0 || len(in.Hold) > 0 || in.Exec != "" {
					t.Fatalf("step %d: act/hold/exec need the client; leave \"go\" out of this case's levels", si)
				}
				if in.Drop {
					s.drop(st.p)
				}
				b := 0
				taps := in.Tap
				if i > 0 {
					taps = nil // a tap is one press: the step's first command
				}
				for _, bn := range append(slices.Clone(in.B), taps...) {
					bit, ok := caseButtons[bn]
					if !ok {
						t.Fatalf("step %d: unknown button %q", si, bn)
					}
					b |= bit
				}
				if st.p.Dead {
					continue
				}
				st.seq++
				s.queueCmds(st.p, []Cmd{{Seq: st.seq, F: in.F, R: in.R, Yaw: st.yaw, Pitch: st.pitch, B: b, W: in.W}})
			}
			s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
			if codec == CodecDelta {
				s.broadcast()
				for _, name := range names {
					st := ps[name]
					if dec[st.p] == nil {
						dec[st.p] = &deltaDecoder{}
					}
					caseDecode(t, s, st, dec[st.p])
				}
			}
			for _, st := range ps {
				countShots(st)
			}
			for _, name := range names {
				if noBack < 0 || backed[name] {
					continue
				}
				if back := caseBack(before[name], ps[name].p.Pos, ps[name].yaw, inputs[name]); back > noBack {
					backed[name] = true
					t.Errorf("step %d (tick %d) %s moved %.4f m backwards against its input (noBack %.4f): rubber banding", si, tick, name, back, noBack)
				}
			}
			tt := traceTick{"world": {"items": len(s.items)}}
			if len(s.nades) > 0 { // only when there are any: cases without grenades trace as before
				tt["world"]["nades"] = len(s.nades)
			}
			for name, st := range ps {
				tt[name] = traceState(s, st)
			}
			trace = append(trace, tt)
		}
	}
	return trace
}

// caseBack: how far (m) a body moved from a to b against its wish direction
// (the move input f/r at yaw, as sim.Step reads it); 0 without input.
func caseBack(a, b Vec3, yaw float64, in caseInput) float64 {
	wx, wz := -math.Sin(yaw)*in.F+math.Cos(yaw)*in.R, -math.Cos(yaw)*in.F-math.Sin(yaw)*in.R
	l := math.Hypot(wx, wz)
	if l == 0 {
		return 0
	}
	return -((b[0]-a[0])*wx + (b[2]-a[2])*wz) / l
}

// traceState: the server-side value of every go-level field, rounded for diffs.
func traceState(s *Server, st *casePlayerState) map[string]any {
	m := s.caseState(st.p)
	m["shots"] = st.shots
	return m
}

// caseDecode drains st's outbox: snapshots go through its decoder (the
// decoded state must be the json codec's snapshot), the rest is counted as
// countShots does.
func caseDecode(t *testing.T, s *Server, st *casePlayerState, d *deltaDecoder) {
	t.Helper()
	for {
		select {
		case b := <-st.p.send:
			if len(b) > 0 && b[0] == deltaMagic {
				if err := d.apply(b); err != nil {
					t.Fatalf("tick %d, %s: %v", s.tick, st.p.Name, err)
				}
				continue
			}
			countShot(st, b)
		default:
			if d.state != nil && d.tick == s.tick {
				got, want := toJSON(netSchema, d.state), wantSnapJSON(t, s, st.p)
				if !reflect.DeepEqual(got, any(want)) {
					gb, _ := json.Marshal(got)
					wb, _ := json.Marshal(want)
					t.Fatalf("tick %d, %s decoded\n%s\nwant (json codec)\n%s", s.tick, st.p.Name, gb, wb)
				}
			}
			return
		}
	}
}

// countShots drains the player's outbox and counts "fired" reports of real shots.
func countShots(st *casePlayerState) {
	for {
		select {
		case b := <-st.p.send:
			countShot(st, b)
		default:
			return
		}
	}
}

func countShot(st *casePlayerState, b []byte) {
	var m struct{ T, R string }
	if json.Unmarshal(b, &m) == nil && m.T == "fired" && !whyNot[m.R] {
		st.shots++
	}
}

func checkField(s *Server, st *casePlayerState, who, k string, raw json.RawMessage) error {
	if who == "world" {
		if k == "items" {
			return cmpNum(float64(len(s.items)), raw)
		}
		if k == "nades" {
			return cmpNum(float64(len(s.nades)), raw)
		}
		if n, ok := strings.CutPrefix(k, "nade."); ok { // nade.<n>: the n-th live grenade's position
			i, _ := strconv.Atoi(n)
			if p := s.NadePos(i); p != nil {
				return cmpVec(p, raw)
			}
			return fmt.Errorf("no grenade %s (%d live)", n, len(s.nades))
		}
		if strings.HasPrefix(k, "cvar.") {
			return cmpJSON(s.cvars[strings.TrimPrefix(k, "cvar.")], raw)
		}
		if k == "navAuto" {
			return cmpJSON(s.navAuto, raw)
		}
		return fmt.Errorf("unknown world field")
	}
	if st == nil {
		return fmt.Errorf("unknown player")
	}
	if _, ok := caseFields[k]; !ok {
		return fmt.Errorf("unknown field")
	}
	if !serverSide(k) {
		return nil // a client-only field
	}
	p := st.p
	var o goldenObs
	if p.inv != nil {
		o = observe(s.Weapons, p.inv, p.cmdTicks)
	}
	switch k {
	case "pos":
		return cmpVec(p.body.Pos[:], raw)
	case "vel":
		return cmpVec(p.body.Vel[:], raw)
	case "speed":
		return cmpNum(math.Hypot(p.body.Vel[0], p.body.Vel[2]), raw)
	case "onGround":
		return cmpJSON(p.body.OnGround, raw)
	case "crouched":
		return cmpJSON(p.body.Crouched, raw)
	case "grav":
		return cmpNum(float64(p.body.Grav), raw)
	case "energy":
		return cmpNum(p.body.Energy, raw)
	case "hooked":
		return cmpJSON(p.body.Hook.On, raw)
	case "ride", "seat": // null: on foot
		var v any
		if p.ride != 0 {
			v = map[string]int{"ride": p.ride, "seat": p.seat}[k]
		}
		return cmpJSON(v, raw)
	case "cur":
		return cmpJSON(o.Cur, raw)
	case "ammo":
		return cmpNum(float64(o.Ammo), raw)
	case "reserve":
		return cmpNum(float64(o.Reserve), raw)
	case "reloading":
		return cmpJSON(o.Reloading, raw)
	case "shots":
		return cmpNum(float64(st.shots), raw)
	case "spray":
		return cmpNum(p.inv.recoil, raw)
	case "boost":
		return cmpNum(float64(max(0, p.inv.boostEnd-p.cmdTicks)), raw)
	case "hp":
		return cmpNum(float64(p.HP), raw)
	case "dead":
		return cmpJSON(p.Dead, raw)
	case "kills":
		return cmpNum(float64(p.Kills), raw)
	case "deaths":
		return cmpNum(float64(p.Deaths), raw)
	case "points":
		return cmpNum(float64(p.Points), raw)
	case "name":
		return cmpJSON(p.Name, raw)
	case "team":
		return cmpJSON(p.Team, raw)
	case "slots":
		got := map[string]string{}
		if p.inv != nil {
			for k, h := range p.inv.Slots {
				got[k] = h.ID
			}
		}
		return cmpJSON(got, raw)
	}
	return fmt.Errorf("unhandled field")
}

func cmpJSON(got any, raw json.RawMessage) error {
	var want, g any
	json.Unmarshal(raw, &want)
	b, _ := json.Marshal(got)
	json.Unmarshal(b, &g)
	if fmt.Sprint(want) != fmt.Sprint(g) {
		return fmt.Errorf("got %s, want %s", b, raw)
	}
	return nil
}

// cmpNum: a number matches exactly; {"min": a, "max": b} matches a range.
func cmpNum(got float64, raw json.RawMessage) error {
	var r struct{ Min, Max *float64 }
	if json.Unmarshal(raw, &r) == nil && (r.Min != nil || r.Max != nil) {
		if (r.Min != nil && got < *r.Min) || (r.Max != nil && got > *r.Max) {
			return fmt.Errorf("got %v, want in %s", got, raw)
		}
		return nil
	}
	var want float64
	if err := json.Unmarshal(raw, &want); err != nil {
		return err
	}
	if math.Abs(got-want) > 1e-9 {
		return fmt.Errorf("got %v, want %v", got, want)
	}
	return nil
}

// cmpVec: [x, y, z] each within 0.01, or null (skip), or {"min": a, "max": b};
// or {"v": [...], "eps": e}.
func cmpVec(got []float64, raw json.RawMessage) error {
	var spec struct {
		V   []json.RawMessage
		Eps float64
	}
	if json.Unmarshal(raw, &spec.V) != nil {
		if err := json.Unmarshal(raw, &spec); err != nil {
			return err
		}
	}
	if spec.Eps == 0 {
		spec.Eps = 0.01
	}
	for i, c := range spec.V {
		if string(c) == "null" {
			continue
		}
		var w float64
		if json.Unmarshal(c, &w) == nil {
			if math.Abs(got[i]-w) > spec.Eps {
				return fmt.Errorf("got %.4f, want %s ±%g", got, raw, spec.Eps)
			}
		} else if err := cmpNum(got[i], c); err != nil {
			return fmt.Errorf("component %d: %v (all: %.4f)", i, err, got)
		}
	}
	return nil
}
