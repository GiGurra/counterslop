package game

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// Declarative command → simulation → state cases (sim/cases/*.json), run here
// through the server's real tick (queueCmds, then advance: commands, items, history) and by
// web/game/cases.test.mjs through the client's predictor (localsim.js). The
// format is documented in web/game/cases.js, which also runs them.

const casesDir = "../../sim/cases"

type caseFile struct {
	Name    string                       `json:"name"`
	Levels  []string                     `json:"levels"`
	Players map[string]casePlayer        `json:"players"`
	Script  []map[string]json.RawMessage `json:"script"`
}

type casePlayer struct {
	Team   string           `json:"team"`
	Pos    Vec3             `json:"pos"`
	Yaw    float64          `json:"yaw"`
	Pitch  float64          `json:"pitch"`
	Crouch bool             `json:"crouch"`
	Slots  map[string]*Held `json:"slots"`
	Cur    string           `json:"cur"`
}

type caseInput struct {
	F     float64  `json:"f"`
	R     float64  `json:"r"`
	Yaw   *float64 `json:"yaw"`
	Pitch *float64 `json:"pitch"`
	B     []string `json:"b"`
	W     string   `json:"w"`
	Drop  bool     `json:"drop"`
}

var caseButtons = map[string]int{"jump": BtnJump, "crouch": BtnCrouch, "walk": BtnWalk, "fire": BtnFire, "scope": BtnScope, "reload": BtnReload}

// caseFields: every assertable field, and the levels that know it.
var caseFields = map[string]string{
	"pos": "go js", "vel": "go js", "onGround": "go js", "crouched": "go js",
	"cur": "go js", "ammo": "go js", "reserve": "go js", "reloading": "go js", "shots": "go js",
	"scoped": "js", "jumps": "js",
	"hp": "go", "dead": "go", "kills": "go", "deaths": "go", "slots": "go", "items": "go",
}

var whyNot = map[string]bool{"no weapon": true, "reloading": true, "cycling": true, "semi": true, "empty": true, "cooldown": true}

func TestCases(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(casesDir, "*.json"))
	if len(files) == 0 {
		t.Fatal("no cases in " + casesDir)
	}
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
		t.Run(strings.TrimSuffix(filepath.Base(f), ".json"), func(t *testing.T) { runCase(t, c) })
	}
}

type casePlayerState struct {
	p     *Player
	yaw   float64
	pitch float64
	seq   int
	shots int
}

func runCase(t *testing.T, c caseFile) {
	s := authServer(t)
	s.Test = true // every queued command runs in its tick
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
		n := 1
		if raw, ok := step["n"]; ok {
			json.Unmarshal(raw, &n)
		}
		inputs := map[string]caseInput{}
		for k, raw := range step {
			if k == "n" {
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
		for range n {
			tick++
			for _, name := range names {
				st, in := ps[name], inputs[name]
				if in.Yaw != nil {
					st.yaw = *in.Yaw
				}
				if in.Pitch != nil {
					st.pitch = *in.Pitch
				}
				if in.Drop {
					s.drop(st.p)
				}
				b := 0
				for _, bn := range in.B {
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
			for _, st := range ps {
				countShots(st)
			}
		}
	}
}

// countShots drains the player's outbox and counts "fired" reports of real shots.
func countShots(st *casePlayerState) {
	for {
		select {
		case b := <-st.p.send:
			var m struct{ T, R string }
			if json.Unmarshal(b, &m) == nil && m.T == "fired" && !whyNot[m.R] {
				st.shots++
			}
		default:
			return
		}
	}
}

func checkField(s *Server, st *casePlayerState, who, k string, raw json.RawMessage) error {
	if who == "world" {
		if k != "items" {
			return fmt.Errorf("unknown world field")
		}
		return cmpNum(float64(len(s.items)), raw)
	}
	if st == nil {
		return fmt.Errorf("unknown player")
	}
	lv, ok := caseFields[k]
	if !ok {
		return fmt.Errorf("unknown field")
	}
	if !strings.Contains(lv, "go") {
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
	case "onGround":
		return cmpJSON(p.body.OnGround, raw)
	case "crouched":
		return cmpJSON(p.body.Crouched, raw)
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
	case "hp":
		return cmpNum(float64(p.HP), raw)
	case "dead":
		return cmpJSON(p.Dead, raw)
	case "kills":
		return cmpNum(float64(p.Kills), raw)
	case "deaths":
		return cmpNum(float64(p.Deaths), raw)
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
