package game

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Shared client↔server golden vectors for the weapon clock: a command stream
// (fire held, reload, switch, per tick) and what must come of it, tick by tick.
// This test runs the server's rules (Registry fire/reload/switchTo in runCmd's
// order) and checks testdata/weapons_golden.json; web/game/arsenal.test.mjs
// replays the same file through the client's predictor (arsenal.js). If either
// side drifts, its test fails.
//
//	UPDATE=1 go test ./internal/game -run TestWeaponGolden   rewrites the file
//
// Only rewrite it on purpose (a rules change on both sides); then the JS test
// must pass against the new file too.

const goldenPath = "testdata/weapons_golden.json"

type goldenCmd struct {
	N      int    `json:"n"` // repeat this command for n ticks
	Fire   bool   `json:"fire,omitempty"`
	Reload bool   `json:"reload,omitempty"` // first tick of the run only
	W      string `json:"w,omitempty"`      // switch to slot, first tick of the run only
}

// goldenObs is what both sides must agree on after a run of commands.
type goldenObs = Observed

type goldenCase struct {
	Name  string           `json:"name"`
	Slots map[string]*Held `json:"slots"`
	Cmds  []goldenCmd      `json:"cmds"`
	// Trace has one char per tick: '*' a shot went off, '.' none.
	Trace string      `json:"trace"`
	Obs   []goldenObs `json:"obs"` // after each command run
	// Kicks has one [spray index, up, right] per shot (recoil.go): the index the
	// shot used and its aim offset.
	Kicks [][3]float64 `json:"kicks"`
}

func goldenInputs() []goldenCase {
	full := func(ids ...string) map[string]*Held {
		m := map[string]*Held{}
		for _, id := range ids {
			switch id {
			case "prompt":
				m["primary"] = &Held{ID: id, Ammo: 30, Reserve: 90}
			case "linter":
				m["primary"] = &Held{ID: id, Ammo: 30, Reserve: 90}
			case "codereview":
				m["primary"] = &Held{ID: id, Ammo: 5, Reserve: 20}
			case "stub":
				m["sidearm"] = &Held{ID: id, Ammo: 12, Reserve: 48}
			case "keyboard":
				m["melee"] = &Held{ID: id}
			}
		}
		return m
	}
	c := func(n int, fire bool) goldenCmd { return goldenCmd{N: n, Fire: fire} }
	return []goldenCase{
		{Name: "auto: hold fire through the deploy, empty the mag, dry fire", Slots: full("prompt", "stub", "keyboard"),
			Cmds: []goldenCmd{c(400, true), c(5, false)}},
		{Name: "auto: burst, reload, wait it out, fire again", Slots: full("linter", "stub", "keyboard"),
			Cmds: []goldenCmd{c(30, false), c(60, true), {N: 1, Reload: true}, c(230, false), c(50, true)}},
		{Name: "reload is refused with a full mag and with no reserve", Slots: map[string]*Held{"primary": {ID: "prompt", Ammo: 30, Reserve: 0}},
			Cmds: []goldenCmd{c(30, false), {N: 1, Reload: true}, c(40, true), {N: 1, Reload: true}, c(10, false)}},
		{Name: "fire held during a reload does nothing until it completes", Slots: map[string]*Held{"primary": {ID: "prompt", Ammo: 3, Reserve: 10}},
			Cmds: []goldenCmd{c(30, false), {N: 1, Reload: true, Fire: true}, c(300, true)}},
		{Name: "semi: one shot per press", Slots: full("stub", "keyboard"),
			Cmds: []goldenCmd{{N: 1, W: "sidearm"}, c(30, false), c(50, true), c(3, false), c(5, true), c(1, false), c(5, true), c(20, false), c(2, true), c(2, false)}},
		{Name: "bolt: cycle after each shot, held trigger doesn't refire", Slots: full("codereview", "stub"),
			Cmds: []goldenCmd{c(30, false), c(200, true), c(1, false), c(50, true), c(1, false), c(150, true), c(1, false), c(10, true)}},
		{Name: "switch mid-reload cancels it", Slots: full("prompt", "stub", "keyboard"),
			Cmds: []goldenCmd{c(30, false), c(30, true), {N: 50, Reload: true}, {N: 40, W: "sidearm"}, {N: 300, W: "primary"}, c(10, true)}},
		{Name: "a finished reload stays finished after switching away and back", Slots: full("prompt", "stub", "keyboard"),
			Cmds: []goldenCmd{c(30, false), c(30, true), {N: 300, Reload: true}, {N: 40, W: "sidearm"}, {N: 40, W: "primary"}, c(30, true)}},
		// The reload completes on the very tick of the switch: it counts (the
		// switch settles first). arsenal.js once deployed first and lost it.
		{Name: "a reload completing on the switch tick fills the mag", Slots: full("prompt", "stub", "keyboard"),
			Cmds: []goldenCmd{c(30, false), c(30, true), {N: 250, Reload: true}, {N: 40, W: "sidearm"}, {N: 40, W: "primary"}, c(5, true)}},
		{Name: "switch while firing: the deploy delay applies again", Slots: full("prompt", "stub", "keyboard"),
			Cmds: []goldenCmd{c(30, false), c(20, true), {N: 40, W: "sidearm", Fire: true}, c(1, false), c(30, true), {N: 60, W: "melee", Fire: true}}},
		{Name: "melee: no ammo, auto swing on its interval", Slots: full("keyboard"),
			Cmds: []goldenCmd{{N: 1, W: "melee"}, c(200, true)}},
		{Name: "burst: a tap fires the whole burst, holding doesn't start another", Slots: map[string]*Held{"primary": {ID: "copypaste", Ammo: 25, Reserve: 90}},
			Cmds: []goldenCmd{c(30, false), c(1, true), c(40, false), c(100, true), c(1, false), c(60, true), c(5, false)}},
		{Name: "burst: tapping on the interval empties the mag, the last burst cut short", Slots: map[string]*Held{"primary": {ID: "copypaste", Ammo: 7, Reserve: 0}},
			Cmds: []goldenCmd{c(30, false), c(1, true), c(49, false), c(1, true), c(49, false), c(1, true), c(49, false), c(1, true), c(20, false)}},
		{Name: "burst: a switch mid-burst ends it", Slots: map[string]*Held{"primary": {ID: "copypaste", Ammo: 25, Reserve: 90}, "sidearm": {ID: "stub", Ammo: 12, Reserve: 48}},
			Cmds: []goldenCmd{c(30, false), c(7, true), {N: 40, W: "sidearm"}, {N: 40, W: "primary"}, c(40, true)}},
		{Name: "burst: a reload mid-burst ends it", Slots: map[string]*Held{"primary": {ID: "copypaste", Ammo: 20, Reserve: 90}},
			Cmds: []goldenCmd{c(30, false), c(3, true), {N: 400, Reload: true}, c(10, true)}},
		{Name: "pump shotgun: semi, its interval, 8 shells", Slots: map[string]*Held{"primary": {ID: "tokenburner", Ammo: 8, Reserve: 32}},
			Cmds: []goldenCmd{c(30, false), c(100, true), c(1, false), c(50, true), c(1, false), c(100, true)}},
		{Name: "spray: the index climbs while held, a short pause recovers part of it, a long one all of it", Slots: full("prompt", "stub", "keyboard"),
			Cmds: []goldenCmd{c(30, false), c(120, true), c(25, false), c(40, true), c(140, false), c(25, true), c(1, false), {N: 40, W: "sidearm", Fire: true}, c(13, false), c(1, true), c(30, false), c(1, true)}},
		{Name: "heavy: a long belt on auto", Slots: map[string]*Held{"primary": {ID: "contextwindow", Ammo: 150, Reserve: 200}},
			Cmds: []goldenCmd{c(30, false), c(300, true), c(10, false)}},
		{Name: "The Big Diff never fires (the trigger plants it, server-side); a pistol after it fires as usual", Slots: map[string]*Held{"bomb": {ID: "bigdiff"}, "sidearm": {ID: "stub", Ammo: 12, Reserve: 48}},
			Cmds: []goldenCmd{{N: 1, W: "bomb"}, c(150, true), {N: 5, Reload: true, Fire: true}, c(3, false), {N: 30, W: "sidearm", Fire: true}, c(1, false), c(30, true), {N: 40, W: "bomb", Fire: true}}},
	}
}

// runGolden plays a case through the server's weapon rules (runCmd's order:
// switch, reload, release/fire) on the command clock and fills Trace and Obs.
func runGolden(r *Registry, gc goldenCase) goldenCase {
	inv := &Inventory{Slots: map[string]*Held{}, Current: "primary"}
	for k, h := range gc.Slots {
		hh := *h
		inv.Slots[k] = &hh
	}
	inv.drawn(0)
	gc.Kicks = nil
	var tick int64
	trace := []byte{}
	var obs []goldenObs
	for _, c := range gc.Cmds {
		for i := 0; i < c.N; i++ {
			tick++
			if i == 0 && c.W != "" {
				r.switchTo(inv, c.W, tick)
			}
			if i == 0 && c.Reload {
				r.reload(inv, tick)
			}
			w, _ := r.trigger(inv, tick, c.Fire)
			shot := w != nil
			if shot {
				off := RecoilOffset(w.Recoil.Pattern, inv.shotIdx)
				gc.Kicks = append(gc.Kicks, [3]float64{inv.shotIdx, off[0], off[1]})
				trace = append(trace, '*')
			} else {
				trace = append(trace, '.')
			}
		}
		obs = append(obs, observe(r, inv, tick))
	}
	gc.Trace, gc.Obs = string(trace), obs
	return gc
}

func TestWeaponGolden(t *testing.T) {
	r := registry(t)
	var got []goldenCase
	for _, gc := range goldenInputs() {
		got = append(got, runGolden(r, gc))
	}
	if os.Getenv("UPDATE") != "" {
		b, _ := json.MarshalIndent(map[string]any{"cases": got}, "", " ")
		if err := os.WriteFile(goldenPath, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("%v (UPDATE=1 go test ./internal/game -run TestWeaponGolden writes it)", err)
	}
	var want struct{ Cases []goldenCase }
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if len(want.Cases) != len(got) {
		t.Fatalf("golden file has %d cases, the inputs %d: rerun with UPDATE=1", len(want.Cases), len(got))
	}
	for i, g := range got {
		w := want.Cases[i]
		if g.Name != w.Name || !reflect.DeepEqual(g.Cmds, w.Cmds) {
			t.Fatalf("case %d inputs changed (%q): rerun with UPDATE=1", i, g.Name)
		}
		if g.Trace != w.Trace {
			t.Errorf("%s: shot trace differs at tick %d\n got  %s\n want %s", g.Name, firstDiff(g.Trace, w.Trace)+1, g.Trace, w.Trace)
		}
		if !reflect.DeepEqual(g.Kicks, w.Kicks) {
			t.Errorf("%s: spray kicks differ\n got  %v\n want %v", g.Name, g.Kicks, w.Kicks)
		}
		if !reflect.DeepEqual(g.Obs, w.Obs) {
			t.Errorf("%s: observations differ\n got  %+v\n want %+v", g.Name, g.Obs, w.Obs)
		}
	}
}

func firstDiff(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

// Shot ray angles (pellets and spread from the shot's seed): the client's
// arsenal.js shotAngles must give the same numbers (web/game/arsenal.test.mjs).
const anglesPath = "testdata/shot_angles_golden.json"

type anglesCase struct {
	Yaw, Pitch, Spread float64
	Seed               uint32
	Pellets            int
	Out                [][2]float64
}

func TestShotAnglesGolden(t *testing.T) {
	ins := []anglesCase{
		{Yaw: 0, Pitch: 0, Spread: 0, Seed: 1, Pellets: 0},
		{Yaw: 1.2, Pitch: -0.3, Spread: 0.005, Seed: SpreadSeed(3, 77), Pellets: 0},
		{Yaw: -2.5, Pitch: 0.4, Spread: 0.09, Seed: SpreadSeed(1, 1234), Pellets: 9},
		{Yaw: 0.3, Pitch: 0, Spread: 0.12, Seed: SpreadSeed(42, 99999), Pellets: 9},
	}
	for i := range ins {
		ins[i].Out = ShotAngles(ins[i].Yaw, ins[i].Pitch, ins[i].Spread, ins[i].Seed, ins[i].Pellets)
	}
	if len(ins[2].Out) != 9 || ins[2].Out[0] == ins[2].Out[1] {
		t.Fatalf("pellets: %v", ins[2].Out)
	}
	if os.Getenv("UPDATE") != "" {
		b, _ := json.MarshalIndent(ins, "", " ")
		if err := os.WriteFile(anglesPath, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(anglesPath)
	if err != nil {
		t.Fatalf("%v (UPDATE=1 go test ./internal/game -run TestShotAnglesGolden writes it)", err)
	}
	var want []anglesCase
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, ins) {
		t.Fatalf("shot angles changed:\n got  %v\n want %v", ins, want)
	}
}
