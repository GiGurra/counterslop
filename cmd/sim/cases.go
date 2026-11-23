package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gigurra/counterslop/internal/game"
)

// Lockstep end-to-end runs of sim/cases (go run ./cmd/sim -cases [name...]):
// the real server (stepped by the harness, not its ticker) and one headless
// client per player (stepped one fixed 10 ms frame per tick). Each case command
// goes in through the client's real input path (key / mouse / wheel events on
// the client's own binds); the client builds and sends the command; the harness
// waits for it, steps the server one tick, and checks that the client's
// predicted state and the server's authoritative state agree on every shared
// field, every tick. Expectations are checked on both sides.

type ecase struct {
	Name    string                       `json:"name"`
	Levels  []string                     `json:"levels"`
	Map     string                       `json:"map"` // e2e: played on this map (loaded before anyone joins); default de_slop
	Players map[string]eplayer           `json:"players"`
	Script  []map[string]json.RawMessage `json:"script"`
}

type eplayer struct {
	Team   string                `json:"team"`
	Pos    [3]float64            `json:"pos"`
	Yaw    float64               `json:"yaw"`
	Pitch  float64               `json:"pitch"`
	Crouch bool                  `json:"crouch"`
	Slots  map[string]*game.Held `json:"slots"`
	Cur    string                `json:"cur"`
	Boost  int64                 `json:"boost"` // energy drink: the first n commands move faster
}

type einput struct {
	F     float64   `json:"f"`
	R     float64   `json:"r"`
	Yaw   *float64  `json:"yaw"`
	Pitch *float64  `json:"pitch"`
	B     []string  `json:"b"`
	W     string    `json:"w"`
	Drop  bool      `json:"drop"`
	Act   []string  `json:"act"`
	Hold  []string  `json:"hold"`
	Exec  string    `json:"exec"`
	Mouse []float64 `json:"mouse"` // e2e: [dx, dy] mouse counts this tick (real mousemove events, through sensitivity etc.)
}

// sharedFields are compared between client and server every tick.
var sharedFields = []string{"pos", "vel", "onGround", "crouched", "cur", "ammo", "reserve", "reloading", "spray"}

// fieldKind: who knows each field (sim/case-fields.json): the client answers
// sim, predictor and ui fields, the server sim (all but shots) and server ones.
var fieldKind = func() map[string]string {
	b, err := os.ReadFile("sim/case-fields.json")
	if err != nil { // go test runs in cmd/sim
		b, err = os.ReadFile("../../sim/case-fields.json")
	}
	if err != nil {
		log.Fatal(err)
	}
	m := map[string]string{}
	json.Unmarshal(b, &m)
	delete(m, "_doc")
	return m
}()

// slotKey is the action that draws a slot; 4 draws the next grenade, so a case
// switching to a grenade slot carries only that kind.
var slotKey = map[string]string{"primary": "slot1", "sidearm": "slot2", "melee": "slot3", "he": "slot4", "flash": "slot4", "smoke": "slot4", "bomb": "slot5"}

func clientKnows(k string) bool {
	return fieldKind[k] == "sim" || fieldKind[k] == "predictor" || fieldKind[k] == "ui"
}
func serverKnows(k string) bool {
	return (fieldKind[k] == "sim" && k != "shots") || fieldKind[k] == "server"
}

var loadouts = map[string]map[string]*game.Held{
	"vibe":    {"primary": {ID: "prompt", Ammo: 30, Reserve: 90}, "sidearm": {ID: "stub", Ammo: 12, Reserve: 48}, "melee": {ID: "keyboard"}},
	"artisan": {"primary": {ID: "linter", Ammo: 30, Reserve: 90}, "sidearm": {ID: "stub", Ammo: 12, Reserve: 48}, "melee": {ID: "keyboard"}},
}

func caseLevels(c ecase) []string {
	if len(c.Levels) == 0 {
		return []string{"go", "js", "e2e"}
	}
	return c.Levels
}

// runCases runs every e2e-level case (or the named ones) on `jobs` workers,
// each with its own in-process server and browser (lockstep steps a whole
// server, so cases can't share one), and returns the results in name order.
func runCases(names []string, jobs int) ([]caseResult, registry, map[string]ecase) {
	files, _ := filepath.Glob(filepath.Join(*casesDir, "*.json"))
	sort.Strings(files)
	type job struct {
		name string
		c    ecase
	}
	var todo []job
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		if len(names) > 0 && !slices.Contains(names, name) {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		var c ecase
		if err := json.Unmarshal(b, &c); err != nil {
			log.Fatalf("%s: %v", f, err)
		}
		if slices.Contains(caseLevels(c), "e2e") {
			todo = append(todo, job{name, c})
		}
	}
	results := make([]caseResult, len(todo))
	byName := map[string]ecase{}
	for _, j := range todo {
		byName[j.name] = j.c
	}
	var reg registry
	next := make(chan int)
	var wg sync.WaitGroup
	for w := range min(jobs, len(todo)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			base, srv, stop, err := startServer()
			if err != nil {
				log.Fatal(err)
			}
			defer stop()
			b, err := launchBrowser(320, 200)
			if err != nil {
				log.Fatal(err)
			}
			defer b.close()
			if w == 0 { // the registry, read from a live client and server
				p, err := b.newPage(base+"/?test=1", 320, 200)
				if err == nil && waitFor(p, "!!window.__cs && !!window.cs") == nil {
					if reg, err = loadRegistry(p, srv); err != nil {
						log.Printf("registry: %v", err)
					}
					p.dispose()
				}
			}
			for i := range next {
				j := todo[i]
				t0 := time.Now()
				fails := runECase(base, srv, b, j.name, j.c)
				r := caseResult{Name: j.name, Title: j.c.Name, Fails: fails, Took: time.Since(t0)}
				results[i] = r
				if len(fails) > 0 {
					log.Printf("FAIL case %s (%s)%s\n   %s", j.name, r.Took.Round(time.Millisecond), contactSummary(j.name), strings.Join(fails, "\n   "))
				} else {
					log.Printf("ok   case %s (%s)%s", j.name, r.Took.Round(time.Millisecond), contactSummary(j.name))
				}
			}
		}()
	}
	for i := range todo {
		next <- i
	}
	close(next)
	wg.Wait()
	return results, reg, byName
}

var caseRun atomic.Int64 // makes player names unique across cases (earlier players may still be leaving)

type caseResult struct {
	Name, Title string
	Fails       []string
	Took        time.Duration
}

func runECase(base string, srv *game.Server, b *browser, name string, c ecase) (fails []string) {
	srv.SetLockstep(false)
	// Each case starts from the same world, whatever ran before on this worker
	// (the order depends on timing): the last case's players gone, default
	// cvars, no bots, de_slop, no dropped weapons or bans. A leftover player
	// unbalanced the teams and a leftover round respawned players over their
	// teleports, but only on a loaded machine.
	deadline := time.Now().Add(10 * time.Second)
	for h, _ := srv.PlayerCount(); h > 0 && time.Now().Before(deadline); h, _ = srv.PlayerCount() {
		time.Sleep(10 * time.Millisecond)
	}
	if h, _ := srv.PlayerCount(); h > 0 {
		return []string{fmt.Sprintf("%d players from an earlier case never left the server", h)}
	}
	if err := srv.ResetWorldOn(c.Map); err != nil {
		return []string{err.Error()}
	}
	players := make([]string, 0, len(c.Players))
	for n := range c.Players {
		players = append(players, n)
	}
	sort.Strings(players)
	// Unique names per case run: earlier cases' players may still be leaving.
	run := caseRun.Add(1)
	pname := func(n string) string { return fmt.Sprintf("%.8s.%d", n, run) }
	ids := map[string]string{} // case player -> "#<server id>": lookups that survive a rename
	sid := func(n string) string {
		if id := ids[n]; id != "" {
			return id
		}
		return pname(n)
	}
	pages := map[string]*page{}
	keys := map[string]string{}
	for _, n := range players {
		cp := c.Players[n]
		team := cp.Team
		if team == "" {
			team = "vibe"
		}
		p, err := b.newPage(base+"/?test=1", 320, 200)
		if err != nil {
			return []string{err.Error()}
		}
		pages[n] = p
		defer p.dispose()
		if err := waitFor(p, "!!window.__cs && !!window.cs"); err != nil {
			return []string{n + ": client didn't start: " + err.Error()}
		}
		steps := []string{
			fmt.Sprintf("__cs.join(%q, %q)", pname(n), team),
			`__cs.cvar("zoom_mode", "hold")`, `__cs.hud(false)`,
			fmt.Sprintf("__cs.set({pos: %s, yaw: %v, pitch: %v, crouch: %v})", jsonOf(cp.Pos), cp.Yaw, cp.Pitch, cp.Crouch),
		}
		for _, s := range steps {
			if err := p.eval(s, nil); err != nil {
				return []string{fmt.Sprintf("%s setup %s: %v", n, s, err)}
			}
		}
		var got struct {
			Team string
			ID   int
		}
		if err := p.eval("__cs.state()", &got); err != nil || got.Team != team {
			return []string{fmt.Sprintf("%s: asked for team %s, got %q (teams must stay balanced: put players on both sides)", n, team, got.Team)}
		}
		ids[n] = fmt.Sprintf("#%d", got.ID)
		if len(keys) == 0 {
			var k map[string]string
			if err := p.eval(`__cs.keysFor()`, &k); err != nil {
				return []string{err.Error()}
			}
			keys = k
		}
	}
	// The teleports ("tp") must have landed on the server before anything else:
	// on a loaded machine a fixed sleep wasn't enough, and a late one reset the
	// player mid-case (back at spawn, velocity zeroed).
	for _, n := range players {
		if err := waitServerPos(srv, sid(n), c.Players[n].Pos); err != nil {
			return []string{n + ": " + err.Error()}
		}
	}
	for _, n := range players {
		cp := c.Players[n]
		slots := cp.Slots
		if slots == nil {
			team := cp.Team
			if team == "" {
				team = "vibe"
			}
			slots = loadouts[team]
		}
		cur := cp.Cur
		if cur == "" {
			cur = "primary"
		}
		if err := srv.SetInventory(sid(n), slots, cur); err != nil {
			return []string{err.Error()}
		}
		if err := waitFor(pages[n], fmt.Sprintf("__cs.caseState().cur === %q", cur)); err != nil {
			return []string{n + ": inventory never arrived"}
		}
		if cp.Boost > 0 { // the client learns it from the snapshot's weapon state ("bo") and reconciles
			if err := srv.SetBoost(sid(n), cp.Boost); err != nil {
				return []string{err.Error()}
			}
		}
	}
	// Every client has taken in everything the server sent during setup
	// (inventory, boost, the teleport's ack) before the first stepped tick.
	for i, n := range players {
		if srv.CaseState(sid(n)) == nil {
			continue
		}
		srv.Sync(sid(n), -1-i)
		if err := pages[n].eval(fmt.Sprintf("__cs.caseSync(%d).then(() => true)", -1-i), nil); err != nil {
			return []string{n + ": setup sync: " + err.Error()}
		}
	}
	srv.ClearDropped()
	srv.ForgetPoses() // with every client's remotes (caseReset): no rewinding to before the setup
	srv.SetLockstep(true)
	defer srv.SetLockstep(false)

	type pstate struct {
		yaw, pitch float64
		client     map[string]any
	}
	ps := map[string]*pstate{}
	for _, n := range players {
		var st map[string]any
		if err := pages[n].eval("__cs.caseReset()", &st); err != nil {
			return []string{err.Error()}
		}
		ps[n] = &pstate{yaw: c.Players[n].Yaw, pitch: c.Players[n].Pitch, client: st}
	}
	// sync lets the client take in everything the server sent so far (inventory
	// after a drop or pickup, ...), then refreshes its state.
	syncN := 0
	sync := func(pn string) {
		if srv.CaseState(sid(pn)) == nil {
			return // not on the server (kicked, disconnected): nothing in flight
		}
		syncN++
		srv.Sync(sid(pn), syncN)
		var cs map[string]any
		if err := pages[pn].eval(fmt.Sprintf("__cs.caseSync(%d)", syncN), &cs); err == nil {
			ps[pn].client = cs
		}
	}
	disagreed := map[string]bool{}
	tick := 0
	for si, step := range c.Script {
		if raw, ok := step["wallMs"]; ok { // e2e: the harness stalls this long in real time (a loaded machine); no tick runs
			var ms int
			json.Unmarshal(raw, &ms)
			time.Sleep(time.Duration(ms) * time.Millisecond)
			continue
		}
		if raw, ok := step["expect"]; ok {
			var want map[string]map[string]json.RawMessage
			json.Unmarshal(raw, &want)
			for _, pn := range players {
				sync(pn)
			}
			for who, fields := range want {
				for k, v := range fields {
					if who == "world" {
						var err error
						switch {
						case k == "items":
							err = cmpNum(float64(srv.ItemCount()), v)
						case k == "nades":
							err = cmpNum(float64(srv.NadeCount()), v)
						case k == "players":
							h, _ := srv.PlayerCount()
							err = cmpNum(float64(h), v)
						case k == "bots":
							_, b := srv.PlayerCount()
							err = cmpNum(float64(b), v)
						case strings.HasPrefix(k, "cvar."):
							err = cmpField(srv.Cvar(strings.TrimPrefix(k, "cvar.")), v)
						case strings.HasPrefix(k, "round."):
							err = cmpField(srv.RoundField(strings.TrimPrefix(k, "round.")), v)
						default:
							err = fmt.Errorf("unknown world field")
						}
						if err != nil {
							fails = append(fails, fmt.Sprintf("step %d (tick %d) world.%s: %v", si, tick, k, err))
						}
						continue
					}
					if _, ok := fieldKind[k]; !ok {
						fails = append(fails, fmt.Sprintf("step %d: unknown field %s", si, k))
						continue
					}
					st := ps[who]
					if st == nil {
						fails = append(fails, fmt.Sprintf("step %d: unknown player %s", si, who))
						continue
					}
					if clientKnows(k) {
						if err := cmpField(st.client[k], v); err != nil {
							fails = append(fails, fmt.Sprintf("step %d (tick %d) %s.%s client: %v", si, tick, who, k, err))
						}
					}
					if serverKnows(k) {
						got := srv.CaseState(sid(who))[k]
						if k == "name" { // players run as <name>.<run>: compare the case's name
							got = strings.Split(fmt.Sprint(got), ".")[0]
						}
						if err := cmpField(got, v); err != nil {
							fails = append(fails, fmt.Sprintf("step %d (tick %d) %s.%s server: %v", si, tick, who, k, err))
						}
					}
				}
			}
			continue
		}
		n := 1
		if raw, ok := step["n"]; ok {
			json.Unmarshal(raw, &n)
		}
		inputs := map[string]einput{}
		for k, raw := range step {
			if k != "n" {
				var in einput
				json.Unmarshal(raw, &in)
				inputs[k] = in
			}
		}
		for i := range n {
			tick++
			for _, pn := range players {
				st, in := ps[pn], inputs[pn]
				if in.Yaw != nil {
					st.yaw = *in.Yaw
				}
				if in.Pitch != nil {
					st.pitch = *in.Pitch
				}
				hold, tap := []string{}, []string{}
				axis := func(v float64, pos, neg string) {
					if v > 0 {
						hold = append(hold, keys[pos])
					} else if v < 0 {
						hold = append(hold, keys[neg])
					}
				}
				axis(in.F, "+forward", "+back")
				axis(in.R, "+moveright", "+moveleft")
				for _, bn := range in.B {
					cmd := map[string]string{"jump": "+jump", "crouch": "+duck", "walk": "+speed", "fire": "+attack", "scope": "+zoom", "reload": "reload", "use": "+use"}[bn]
					if cmd == "reload" {
						tap = append(tap, keys[cmd])
					} else {
						hold = append(hold, keys[cmd])
					}
				}
				if in.W != "" {
					tap = append(tap, keys[slotKey[in.W]])
				}
				if in.Drop {
					tap = append(tap, keys["drop"])
				}
				if i == 0 { // run-once presses: at the start of the run
					for _, a := range in.Act {
						if keys[a] == "" {
							return append(fails, fmt.Sprintf("step %d: no key bound to %q", si, a))
						}
						tap = append(tap, keys[a])
					}
				}
				for _, a := range in.Hold {
					hold = append(hold, keys[a])
				}
				if in.Exec != "" && i == 0 {
					if err := pages[pn].eval(fmt.Sprintf("__cs.exec(%q)", in.Exec), nil); err != nil {
						return append(fails, fmt.Sprintf("step %d exec %q: %v", si, in.Exec, err))
					}
				}
				targ := map[string]any{"hold": hold, "tap": tap, "yaw": st.yaw, "pitch": st.pitch}
				if len(in.Mouse) == 2 {
					targ["mouse"] = in.Mouse
				}
				arg, _ := json.Marshal(targ)
				var cs map[string]any
				if err := pages[pn].eval("__cs.caseTick("+string(arg)+")", &cs); err != nil {
					return append(fails, fmt.Sprintf("tick %d %s: %v", tick, pn, err))
				}
				st.client = cs
				if len(in.Mouse) == 2 { // the view stays where the mouse put it
					st.yaw, _ = cs["yaw"].(float64)
					st.pitch, _ = cs["pitch"].(float64)
				}
			}
			for _, pn := range players {
				// connected and seq decide whether this tick waits for the client's
				// command: a caseState without them (a broken test API) would step the
				// server without it, a timing-dependent twin-sim failure. Fail loudly.
				if err := caseStateKeys(ps[pn].client); err != nil {
					return append(fails, fmt.Sprintf("tick %d %s: %v", tick, pn, err))
				}
				seq, _ := ps[pn].client["seq"].(float64)
				sv := srv.CaseState(sid(pn))
				if conn, _ := ps[pn].client["connected"].(bool); !conn || sv == nil || sv["pos"] == nil {
					continue // gone, disconnected or no body (spectating): no commands to wait for
				}
				if dead, _ := sv["dead"].(bool); dead {
					continue
				}
				// A lost packet (net_fakeloss) would be repeated by the client's next
				// one; in lockstep there is no next frame until this tick runs, so
				// resend the unacknowledged commands as it would, until they arrive.
				err := srv.WaitCommand(sid(pn), int(seq), 30*time.Millisecond)
				for try := 0; err != nil && try < 60; try++ {
					pages[pn].eval("__cs.resendCmds()", nil)
					err = srv.WaitCommand(sid(pn), int(seq), 30*time.Millisecond)
				}
				if err != nil {
					return append(fails, fmt.Sprintf("tick %d: %v", tick, err))
				}
			}
			srv.Step(1)
			for _, pn := range players {
				sv := srv.CaseState(sid(pn))
				if sv == nil || sv["pos"] == nil || ps[pn].client["pos"] == nil {
					continue // no body on one side (disconnected, spectating): nothing to agree on
				}
				if d, _ := sv["dead"].(bool); d {
					continue
				}
				synced := false
				// Moving contact (the server's test-only _contact): the client
				// predicts against the others' newest snapshot, a tick or more
				// old, so its body may be off by a few cm until the next
				// reconcile. Only then is the body compared within contactTol.
				// crouched too: an unduck stands up only where the standing hull
				// fits, so under a rider's feet (a boost) it may stand a tick late.
				contact, _ := sv["_contact"].(bool)
				if contact {
					d, dh, dy := vecDist(ps[pn].client["pos"], sv["pos"])
					noteContactTick(name, d)
					if (dh > contactTol || dy > contactTolY) && !disagreed[pn+"pos"] {
						disagreed[pn+"pos"] = true
						fails = append(fails, fmt.Sprintf("tick %d %s.pos: client %s, server %s: %.3f m apart across, %.3f up in a moving contact (tolerance %.2f, %.2f)", tick, pn, jsonOf(ps[pn].client["pos"]), jsonOf(sv["pos"]), dh, dy, contactTol, contactTolY))
					}
				}
				for _, k := range sharedFields {
					if disagreed[pn+k] || (contact && (k == "pos" || k == "vel" || k == "onGround" || k == "crouched")) {
						continue
					}
					if !sameJSON(ps[pn].client[k], sv[k]) && !synced {
						sync(pn) // maybe a server message (inventory) is still in flight
						synced = true
					}
					if !sameJSON(ps[pn].client[k], sv[k]) {
						disagreed[pn+k] = true
						if os.Getenv("CASES_DEBUG") != "" {
							log.Printf("tick %d %s\n client %s\n server %s", tick, pn, jsonOf(ps[pn].client), jsonOf(sv))
						}
						fails = append(fails, fmt.Sprintf("tick %d %s.%s: client %s, server %s (twin sims disagree)", tick, pn, k, jsonOf(ps[pn].client[k]), jsonOf(sv[k])))
					}
				}
			}
		}
	}
	return fails
}

// contactTol bounds the client/server body difference on moving-contact
// ticks across (x/z), contactTolY up: a few cm off near a stair or tread edge
// is a whole step up (sim stepUp) on one side and not the other.
const (
	contactTol  = 0.15
	contactTolY = 0.56
)

// contactStat: lockstep ticks compared within contactTol (a moving contact)
// in one case, and how far apart client and server were on them.
type contactStat struct {
	Ticks       int
	Max, SumPos float64
}

var (
	contactMu    sync.Mutex
	contactStats = map[string]*contactStat{}
)

func noteContactTick(name string, d float64) {
	contactMu.Lock()
	defer contactMu.Unlock()
	st := contactStats[name]
	if st == nil {
		st = &contactStat{}
		contactStats[name] = st
	}
	st.Ticks++
	st.Max = math.Max(st.Max, d)
	st.SumPos += d
}

// contactSummary is the case's line about moving-contact ticks ("" if none).
func contactSummary(name string) string {
	contactMu.Lock()
	defer contactMu.Unlock()
	st := contactStats[name]
	if st == nil {
		return ""
	}
	return fmt.Sprintf(" [moving contact: %d player-ticks within %.2f m across, %.2f up; max %.4f m, mean %.4f m]", st.Ticks, contactTol, contactTolY, st.Max, st.SumPos/float64(st.Ticks))
}

// vecDist: the distance between two [x, y, z] (JSON-ish), across (x/z) and up.
func vecDist(a, b any) (d, across, up float64) {
	var x, y []float64
	json.Unmarshal([]byte(jsonOf(a)), &x)
	json.Unmarshal([]byte(jsonOf(b)), &y)
	if len(x) != 3 || len(y) != 3 {
		inf := math.Inf(1)
		return inf, inf, inf
	}
	across, up = math.Hypot(x[0]-y[0], x[2]-y[2]), math.Abs(x[1]-y[1])
	return math.Hypot(across, up), across, up
}

func lastLines(s string, n int) string {
	l := strings.Split(s, "\n")
	return strings.Join(l[max(0, len(l)-n):], " / ")
}

func jsonOf(v any) string { b, _ := json.Marshal(v); return string(b) }

func sameJSON(a, b any) bool {
	var x, y any
	json.Unmarshal([]byte(jsonOf(a)), &x)
	json.Unmarshal([]byte(jsonOf(b)), &y)
	if xa, ok := x.([]any); ok {
		ya, ok := y.([]any)
		if !ok || len(xa) != len(ya) {
			return false
		}
		for i := range xa {
			if math.Abs(xa[i].(float64)-ya[i].(float64)) > 2e-4 {
				return false
			}
		}
		return true
	}
	return fmt.Sprint(x) == fmt.Sprint(y)
}

// cmpField matches a value against an expectation (as the go/js runners do):
// strings also match {"contains": "..."}, objects match the listed keys only.
func cmpField(got any, raw json.RawMessage) error {
	var c struct{ Contains, Lacks *string }
	if str, ok := got.(string); ok && json.Unmarshal(raw, &c) == nil && (c.Contains != nil || c.Lacks != nil) {
		if c.Contains != nil && !strings.Contains(str, *c.Contains) {
			return fmt.Errorf("%q does not contain %q", lastLines(str, 6), *c.Contains)
		}
		if c.Lacks != nil && strings.Contains(str, *c.Lacks) {
			return fmt.Errorf("%q contains %q", lastLines(str, 6), *c.Lacks)
		}
		return nil
	}
	if obj, ok := got.(map[string]any); ok {
		var want map[string]any
		if json.Unmarshal(raw, &want) == nil {
			for k, w := range want {
				if !sameJSON(obj[k], w) {
					return fmt.Errorf("%s: got %s, want %s", k, jsonOf(obj[k]), jsonOf(w))
				}
			}
			return nil
		}
	}
	if arr, ok := got.([]any); ok {
		g := make([]float64, len(arr))
		for i, v := range arr {
			g[i], _ = v.(float64)
		}
		return cmpVec(g, raw)
	}
	if fl, ok := got.([]float64); ok {
		return cmpVec(fl, raw)
	}
	switch v := got.(type) {
	case float64:
		return cmpNum(v, raw)
	case int:
		return cmpNum(float64(v), raw)
	}
	if !sameJSON(got, json.RawMessage(raw)) {
		var w any
		json.Unmarshal(raw, &w)
		if !sameJSON(got, w) {
			return fmt.Errorf("got %s, want %s", jsonOf(got), raw)
		}
	}
	return nil
}

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
			return fmt.Errorf("component %d: %v", i, err)
		}
	}
	return nil
}

// writeCaseSummary prints the results and, under GitHub Actions, adds them to the job summary.
func writeCaseSummary(res []caseResult, extra string) {
	var md strings.Builder
	md.WriteString("### Lockstep e2e cases\n\n| case | checks | result | time |\n|---|---|---|---|\n")
	for _, r := range res {
		status := "✅"
		if len(r.Fails) > 0 {
			status = "❌ " + strings.ReplaceAll(r.Fails[0], "|", "\\|")
		}
		fmt.Fprintf(&md, "| %s | %s | %s | %s |\n", r.Name, strings.ReplaceAll(r.Title, "|", "\\|"), status, r.Took.Round(time.Millisecond))
	}
	md.WriteString("\n" + extra)
	if f := os.Getenv("GITHUB_STEP_SUMMARY"); f != "" {
		if fh, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644); err == nil {
			fh.WriteString(md.String())
			fh.Close()
		}
	}
}

// waitServerPos waits (wall time, before lockstep starts) until the server has
// the player at pos.
func waitServerPos(srv *game.Server, id string, pos [3]float64) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		var st struct {
			Pos []float64 `json:"pos"`
		}
		b, _ := json.Marshal(srv.CaseState(id))
		_ = json.Unmarshal(b, &st)
		if len(st.Pos) == 3 && math.Abs(st.Pos[0]-pos[0]) < 1e-3 && math.Abs(st.Pos[1]-pos[1]) < 1e-3 && math.Abs(st.Pos[2]-pos[2]) < 1e-3 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("teleport to %v never landed on the server (at %v)", pos, st.Pos)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// caseStateKeys checks that a client caseState carries what lockstep relies on.
func caseStateKeys(st map[string]any) error {
	if _, ok := st["connected"].(bool); !ok {
		keys := make([]string, 0, len(st))
		for k := range st {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return fmt.Errorf("client caseState has no boolean connected (broken test API?); keys: %s", strings.Join(keys, " "))
	}
	if _, ok := st["seq"].(float64); !ok {
		return fmt.Errorf("client caseState has no numeric seq (broken test API?)")
	}
	return nil
}
