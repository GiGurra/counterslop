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
	"strconv"
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
	Grav   int                   `json:"grav"`  // the gravity it stands in (internal/sim gravity.go)
	Boots  bool                  `json:"boots"` // wears the Gravity Override (hook.go), a full meter
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
	Tap   []string  `json:"tap"`
	Hold  []string  `json:"hold"`
	Exec  string    `json:"exec"`
	Mouse []float64 `json:"mouse"` // e2e: [dx, dy] mouse counts this tick (real mousemove events, through sensitivity etc.)
	// Flick: e2e, the first tick of the run: real input events in this order
	// within the tick: [dx, dy] a mousemove, "fire" a click (+attack pressed
	// and released): a click in the middle of a flick.
	Flick []json.RawMessage `json:"flick"`
}

// sharedFields are compared between client and server every tick.
var sharedFields = []string{"pos", "vel", "onGround", "crouched", "cur", "ammo", "reserve", "reloading", "spray", "boost", "grav", "energy", "hooked"}

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
var slotKey = map[string]string{"primary": "slot1", "sidearm": "slot2", "melee": "slot3", "he": "slot4", "flash": "slot4", "smoke": "slot4", "fire": "slot4", "bomb": "slot5"}

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
			base, a, stop, err := startServer()
			if err != nil {
				log.Fatal(err)
			}
			defer stop()
			srv := a.Server
			b, err := launchBrowser(320, 200, true)
			if err != nil {
				log.Fatal(err)
			}
			defer func() { b.close() }()
			if w == 0 { // the registry, read from a live client and server
				p, err := b.newPage(base+"/?test=1", 320, 200, pageInitFor(true, *cpuThrottle))
				if err == nil && waitFor(p, "!!window.__cs && !!window.cs") == nil {
					if reg, err = loadRegistry(p, srv); err != nil {
						log.Printf("registry: %v", err)
					}
					p.dispose()
				}
			}
			for i := range next {
				j := todo[i]
				t0 := time.Now() // wall: reports how long a case took
				cb := b
				fails := withDeadline(caseDeadline, caseDrain, func() []string { return runECase(base, srv, cb, j.name, j.c) }, cb.kill)
				if len(fails) > 0 { // a failed case may leave the browser in any state: the next one gets a new one
					b.close()
					if b, err = launchBrowser(320, 200, true); err != nil {
						log.Fatal(err)
					}
				}
				r := caseResult{Name: j.name, Title: j.c.Name, Fails: fails, Took: time.Since(t0)} // wall: report
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

// Hang guards (wall time): a case that has not finished after caseDeadline
// fails, its browser is killed (every page call then fails at once), and the
// run goes on; if it still hasn't returned caseDrain later, the run stops.
// A stuck page must fail the run, never hang it (a 'no welcome' join once
// hung a pre-push run for hours).
const (
	caseDeadline = 5 * time.Minute
	caseDrain    = 30 * time.Second
)

// withDeadline runs f; past d it calls kill and returns f's fails plus a
// timeout (or exits the program if f is still not back after drain).
func withDeadline(d, drain time.Duration, f func() []string, kill func()) []string {
	done := make(chan []string, 1)
	go func() { done <- f() }()
	select {
	case fails := <-done:
		return fails
	case <-time.After(d): // wall: hang guard (caseDeadline)
	}
	kill()
	msg := fmt.Sprintf("hung: no result after %s of wall time (hang guard); its browser was killed", d)
	select {
	case fails := <-done:
		return append([]string{msg}, fails...)
	case <-time.After(drain): // wall: hang guard
		log.Fatalf("FAIL %s, and still not back %s after the kill: stopping the run", msg, drain)
		return nil
	}
}

// Waiting for a client's command in a case (lockstep): it may be on its way
// (a socket delivers in microseconds; cmdArrive bounds a loaded machine),
// held back by a fake link fault on the page's virtual clock (the harness
// moves that clock on to its next due timer, up to faultMaxMs), or lost
// (net_fakeloss: resent, as the client's next packet would carry it).
const (
	cmdArrive  = 2 * time.Second
	faultMaxMs = 2000.0
)

var caseRun atomic.Int64 // makes player names unique across cases (earlier players may still be leaving)

type caseResult struct {
	Name, Title string
	Fails       []string
	Took        time.Duration
}

func runECase(base string, srv *game.Server, b *browser, name string, c ecase) (fails []string) {
	// Lockstep from here on: the world is reset and set up on game time the
	// harness steps, so a case starts on the same tick (0) whatever ran
	// before it and however long the reset and the page boots take.
	srv.SetLockstep(true)
	defer srv.SetLockstep(false)
	// Each case starts from the same world, whatever ran before on this worker
	// (the order depends on timing): the last case's players gone, default
	// cvars, no bots, de_slop, no dropped weapons or bans. A leftover player
	// unbalanced the teams and a leftover round respawned players over their
	// teleports, but only on a loaded machine.
	if err := waitServerEmpty(srv, 10*time.Second); err != nil {
		return []string{err.Error() + " (an earlier case's)"}
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
	// The pages boot one after another, in name order: a page connects (as a
	// hidden observer from the menu) as soon as it loads, and that is when the
	// server hands out its ID, so each waits for its socket before the next
	// loads. Cases that depend on ID order (who yields in an overlap) then
	// play the same on every run; booting them at once raced (CI: alice got
	// the higher ID and players-overlap went the other way).
	pages := map[string]*page{}
	// Lockstep from before the first page loads: the server ticks only when
	// the harness steps it, and the pages run on virtual clocks (vclock.js)
	// that move only with the ticks: nothing a case checks reads wall time.
	ls := newLockstep(srv, pages, players, true)
	defer ls.stop()
	for _, n := range players {
		p, err := b.newPage(base+"/?test=1", 320, 200, pageInitFor(true, *cpuThrottle)) // the virtual clock; nothing drawn
		if err != nil {
			return []string{err.Error()}
		}
		pages[n] = p
		defer p.dispose()
		if err := waitFor(p, "!!window.__cs && !!window.cs && window.cs.net.open"); err != nil {
			return []string{n + ": client didn't start: " + err.Error()}
		}
	}
	keys := map[string]string{}
	lastID := 0
	for _, n := range players {
		p, cp := pages[n], c.Players[n]
		team := cp.Team
		if team == "" {
			team = "vibe"
		}
		// Through the lockstep gate (a call that waits for the server gets
		// exactly the ticks it needs): setup takes the same game time on every
		// run, however slow the machine.
		steps := [][2]string{
			{"join", jsonOf([]any{pname(n), team})},
			{"cvar", `["zoom_mode", "hold"]`}, {"hud", "[false]"},
			{"set", jsonOf([]any{map[string]any{"pos": cp.Pos, "yaw": cp.Yaw, "pitch": cp.Pitch, "crouch": cp.Crouch, "grav": cp.Grav}})},
		}
		for _, st := range steps {
			if err := ls.call(n, st[0], []byte(st[1])); err != nil {
				return []string{fmt.Sprintf("%s setup %s%s: %v", n, st[0], st[1], err)}
			}
			if err := ls.settle(); err != nil {
				return []string{fmt.Sprintf("%s setup %s: %v", n, st[0], err)}
			}
		}
		var got struct {
			Team string
			ID   int
		}
		if err := p.eval("__cs.state()", &got); err != nil || got.Team != team {
			return []string{fmt.Sprintf("%s: asked for team %s, got %q (teams must stay balanced: put players on both sides)", n, team, got.Team)}
		}
		if got.ID <= lastID { // the boot order above broke: ID tie-breaks would flip
			return []string{fmt.Sprintf("%s has server ID #%d after #%d: IDs must ascend in name order", n, got.ID, lastID)}
		}
		lastID = got.ID
		ids[n] = fmt.Sprintf("#%d", got.ID)
		if len(keys) == 0 {
			var k map[string]string
			if err := p.eval(`__cs.keysFor()`, &k); err != nil {
				return []string{err.Error()}
			}
			keys = k
		}
	}
	if err := idsInOrder(players, ids); err != nil {
		return []string{err.Error()}
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
		if cp.Boots { // the client learns it from its self-snapshot (bt, en) and reconciles
			if err := srv.SetBoots(sid(n)); err != nil {
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
	var trace *os.File
	if *traceDir != "" {
		_ = os.MkdirAll(*traceDir, 0o755)
		if f, err := os.Create(filepath.Join(*traceDir, name+".jsonl")); err == nil {
			trace = f
			defer f.Close()
		}
	}
	lostUp := map[string]*int{} // per player: command packets its page's fake faults lost so far (awaitCommand)
	tick := 0
	for si, step := range c.Script {
		if raw, ok := step["wallMs"]; ok { // e2e: the harness stalls this long in real time (a loaded machine); no tick runs
			var ms int
			json.Unmarshal(raw, &ms)
			time.Sleep(time.Duration(ms) * time.Millisecond) // wall: wallMs, a real stall on purpose (nothing may move)
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
						case strings.HasPrefix(k, "nade."): // nade.<n>: the n-th live grenade's position
							i, _ := strconv.Atoi(strings.TrimPrefix(k, "nade."))
							if p := srv.NadePos(i); p != nil {
								err = cmpVec(p, v)
							} else {
								err = fmt.Errorf("no grenade %d", i)
							}
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
						if k == "speed" { // derived: |vel| in x and z (not in the traces simdiff compares)
							got = caseSpeed(srv.CaseState(sid(who))["vel"])
						}
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
		noBack := -1.0
		if raw, ok := step["noBack"]; ok {
			json.Unmarshal(raw, &noBack)
		}
		backed := map[string]bool{}
		inputs := map[string]einput{}
		for k, raw := range step {
			if k != "n" && k != "noBack" {
				var in einput
				json.Unmarshal(raw, &in)
				inputs[k] = in
			}
		}
		for i := range n {
			tick++
			if err := ls.advance(1000.0 / game.SimHz); err != nil { // the tick's 10 ms on every page's clock
				return append(fails, fmt.Sprintf("tick %d: %v", tick, err))
			}
			cBefore, sBefore := map[string]any{}, map[string]any{}
			if noBack >= 0 {
				for _, pn := range players {
					cBefore[pn] = ps[pn].client["pos"]
					if sv := srv.CaseState(sid(pn)); sv != nil {
						sBefore[pn] = sv["pos"]
					}
				}
			}
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
					cmd := map[string]string{"jump": "+jump", "crouch": "+duck", "walk": "+speed", "fire": "+attack", "scope": "+zoom", "alt": "+zoom", "reload": "reload", "use": "+use", "hook": "+hook"}[bn]
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
					for _, bn := range in.Tap { // a held button pressed and released between two commands
						cmd := map[string]string{"jump": "+jump", "fire": "+attack", "use": "+use", "alt": "+zoom", "hook": "+hook"}[bn]
						if keys[cmd] == "" {
							return append(fails, fmt.Sprintf("step %d: tap %q: not a latched button with a key", si, bn))
						}
						tap = append(tap, keys[cmd])
					}
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
					// Then a barrier: the server has read the line and its answer (a
					// team change, a notice) is in before this tick's frame, or the
					// frame's command would depend on which came first.
					if err := pages[pn].eval(fmt.Sprintf("Promise.resolve(__cs.exec(%q)).then(() => __cs.__barrier())", in.Exec), nil); err != nil {
						return append(fails, fmt.Sprintf("step %d exec %q: %v", si, in.Exec, err))
					}
				}
				// snap: the client takes the last snapshot in before its frame, and
				// tick pins its view tick to the server's (not the wall clock): the
				// command's vt and the remotes it collides with are the same on
				// every run, however fast the machine.
				targ := map[string]any{"hold": hold, "tap": tap, "yaw": st.yaw, "pitch": st.pitch, "snap": srv.SnapSent(sid(pn)), "tick": srv.Tick()}
				if len(in.Mouse) == 2 {
					targ["mouse"] = in.Mouse
				}
				if len(in.Flick) > 0 && i == 0 {
					fl := []any{}
					for _, ev := range in.Flick {
						var mv []float64
						var bn string
						switch {
						case json.Unmarshal(ev, &mv) == nil && len(mv) == 2:
							fl = append(fl, map[string]any{"mouse": mv})
						case json.Unmarshal(ev, &bn) == nil && bn == "fire":
							fl = append(fl, map[string]any{"click": keys["+attack"]})
						default:
							return append(fails, fmt.Sprintf("step %d: flick event %s: want [dx, dy] or \"fire\"", si, ev))
						}
					}
					targ["flick"] = fl
				}
				arg, _ := json.Marshal(targ)
				var cs map[string]any
				if err := pages[pn].eval("__cs.caseTick("+string(arg)+")", &cs); err != nil {
					return append(fails, fmt.Sprintf("tick %d %s: %v", tick, pn, err))
				}
				st.client = cs
				// Staleness: the server queued this client a snapshot within
				// staleTicks of its tick, and the client has it before its frame
				// (a delta it couldn't decode would leave it acting on old state).
				if sent, now := srv.SnapSent(sid(pn)), srv.Tick(); sent >= 0 && now-sent > staleTicks && !disagreed[pn+"stale"] {
					disagreed[pn+"stale"] = true
					fails = append(fails, fmt.Sprintf("tick %d %s: the server's last snapshot to it is from tick %d, %d ticks old (at most %d)", tick, pn, sent, now-sent, staleTicks))
				}
				if late, _ := cs["snapLate"].(bool); late && !disagreed[pn+"late"] {
					disagreed[pn+"late"] = true
					fails = append(fails, fmt.Sprintf("tick %d %s: never took in the snapshot of tick %v the server sent it (has %v)", tick, pn, targ["snap"], cs["snapTick"]))
				}
				if len(in.Mouse) == 2 || len(in.Flick) > 0 && i == 0 { // the view stays where the mouse put it
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
				if err := awaitCommand(srv, pages[pn], sid(pn), int(seq), lostOf(lostUp, pn)); err != nil {
					return append(fails, fmt.Sprintf("tick %d: %v", tick, err))
				}
			}
			if err := ls.barrier(); err != nil { // console lines, chat, ...: in before the tick
				return append(fails, fmt.Sprintf("tick %d: %v", tick, err))
			}
			srv.Step(1)
			if noBack >= 0 { // no rubber banding: nobody moves backwards against its input (client and server)
				for _, pn := range players {
					in := inputs[pn]
					var after any
					if sv := srv.CaseState(sid(pn)); sv != nil {
						after = sv["pos"]
					}
					for _, side := range []struct {
						n    string
						a, b any
					}{{"client", cBefore[pn], ps[pn].client["pos"]}, {"server", sBefore[pn], after}} {
						if back := backAlong(side.a, side.b, ps[pn].yaw, in.F, in.R); back > noBack && !backed[pn+side.n] {
							backed[pn+side.n] = true
							fails = append(fails, fmt.Sprintf("tick %d %s %s moved %.4f m backwards against its input (noBack %.4f): rubber banding", tick, pn, side.n, back, noBack))
						}
					}
				}
			}
			// Every client has taken in all the tick sent (a spawn, a team change:
			// not only snapshots) before its next frame.
			if err := ls.syncAll(); err != nil {
				return append(fails, fmt.Sprintf("tick %d: %v", tick, err))
			}
			if trace != nil { // -trace: the server's view of every player after this tick
				row := map[string]any{"tick": tick, "stick": srv.Tick(), "bots": srv.BotStates()}
				for _, pn := range players {
					row[pn] = srv.CaseState(sid(pn))
					row["cmd."+pn] = []any{ps[pn].client["seq"], srv.CmdTrace(sid(pn))}
				}
				b, _ := json.Marshal(row)
				trace.Write(append(b, '\n'))
			}
			for _, pn := range players {
				sv := srv.CaseState(sid(pn))
				if sv == nil || sv["pos"] == nil || ps[pn].client["pos"] == nil {
					continue // no body on one side (disconnected, spectating): nothing to agree on
				}
				if d, _ := sv["dead"].(bool); d {
					continue
				}
				synced := false
				// Moving contact (the server's test-only _contact): the body is
				// compared within contactTol (exact in practice: both sides
				// collide with the others as drawn at the command's vt).
				// crouched too: an unduck stands up only where the standing hull
				// fits, so under a rider's feet (a boost) it may stand a tick late.
				contact, _ := sv["_contact"].(bool)
				passenger, _ := sv["_passenger"].(bool) // in a boat someone else drives: where it is is the driver's input (harness.go)
				if contact {
					d, dh, dy := vecDist(ps[pn].client["pos"], sv["pos"])
					noteContactTick(name, d)
					if (dh > contactTol || dy > contactTolY) && !disagreed[pn+"pos"] {
						disagreed[pn+"pos"] = true
						fails = append(fails, fmt.Sprintf("tick %d %s.pos: client %s, server %s: %.3f m apart across, %.3f up in a moving contact (tolerance %.2f, %.2f)", tick, pn, jsonOf(ps[pn].client["pos"]), jsonOf(sv["pos"]), dh, dy, contactTol, contactTolY))
					}
				}
				for _, k := range sharedFields {
					if disagreed[pn+k] || ((contact || passenger) && (k == "pos" || k == "vel" || k == "onGround" || k == "crouched")) {
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
	// The snapshot codec: a checksum mismatch is a codec bug (the decoded state
	// isn't what the server sent), whatever the link did.
	for _, pn := range players {
		var nc struct {
			Codec      string `json:"codec"`
			Mismatches int    `json:"mismatches"`
			Last       string `json:"last"`
			Clean      int    `json:"clean"`
		}
		if err := pages[pn].eval("__cs.netCodec()", &nc); err == nil && nc.Mismatches > 0 {
			fails = append(fails, fmt.Sprintf("%s: %d %s snapshot checksum mismatch(es), last: %s", pn, nc.Mismatches, nc.Codec, nc.Last))
		} else if err == nil && nc.Clean > 0 { // no fake fault lost anything: a snapshot the server sent couldn't be decoded
			fails = append(fails, fmt.Sprintf("%s: %d undecodable %s snapshot(s) on a clean link, last: %s", pn, nc.Clean, nc.Codec, nc.Last))
		}
	}
	return fails
}

// staleTicks: the most server ticks a lockstep client may go without a
// snapshot queued to it (every tick normally; a delta connection waiting for
// its full gets it the next tick, or one later when others asked first).
const staleTicks = 2

// contactTol bounds the client/server body difference on moving-contact
// ticks across (x/z), contactTolY up. Both sides collide with the others'
// hulls where the client drew them (the command's vt: simtick.go peersOf,
// predict.js peersAt), which predicts exactly; what is left is the core (where
// the other really is now: the client knows its newest snapshot, a few ticks
// old; head-on up to ~0.11 m) and a command run before the client had any
// snapshot of the other (players put on one spot).
const (
	contactTol  = 0.12
	contactTolY = 0.06
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
// backAlong: how far (m) a body moved from a to b (positions as JSON
// values) against its wish direction (f/r at yaw); 0 without input or a
// position. = internal/game cases_test.go caseBack.
func backAlong(a, b any, yaw, f, r float64) float64 {
	var x, y []float64
	json.Unmarshal([]byte(jsonOf(a)), &x)
	json.Unmarshal([]byte(jsonOf(b)), &y)
	wx, wz := -math.Sin(yaw)*f+math.Cos(yaw)*r, -math.Cos(yaw)*f-math.Sin(yaw)*r
	l := math.Hypot(wx, wz)
	if len(x) != 3 || len(y) != 3 || l == 0 {
		return 0
	}
	return -((y[0]-x[0])*wx + (y[2]-x[2])*wz) / l
}

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
	case int64:
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
	deadline := time.Now().Add(10 * time.Second) // wall: hang guard
	for {
		var st struct {
			Pos []float64 `json:"pos"`
		}
		b, _ := json.Marshal(srv.CaseState(id))
		_ = json.Unmarshal(b, &st)
		if len(st.Pos) == 3 && math.Abs(st.Pos[0]-pos[0]) < 1e-3 && math.Abs(st.Pos[1]-pos[1]) < 1e-3 && math.Abs(st.Pos[2]-pos[2]) < 1e-3 {
			return nil
		}
		if time.Now().After(deadline) { // wall: hang guard
			return fmt.Errorf("teleport to %v never landed on the server (at %v)", pos, st.Pos)
		}
		time.Sleep(5 * time.Millisecond) // wall: polls
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

// idsInOrder: the case's players got server IDs ascending in name order
// (ids: name -> "#<id>"), as runECase's one-by-one boot gives them on every
// run: ties (the push-out of overlapping bodies) go by ID.
func idsInOrder(players []string, ids map[string]string) error {
	last := 0
	for _, n := range players {
		var id int
		if _, err := fmt.Sscanf(ids[n], "#%d", &id); err != nil {
			return fmt.Errorf("%s: no server id (%q)", n, ids[n])
		}
		if id <= last {
			return fmt.Errorf("server ids not in name order: %v (ties go by id: the case would play differently)", ids)
		}
		last = id
	}
	return nil
}

// awaitCommand waits until command seq of a case player is on the server.
// Held back by a fake link fault (net_fakelag, jitter, reorder: on the page's
// virtual clock), the page's clock moves on until it goes; lost
// (net_fakeloss), the client resends the unacknowledged commands, as its next
// packet would (in lockstep there is no next frame until this tick runs);
// sent, it is in the socket.
// lost: the page's lost count as the last call left it (updated).
func awaitCommand(srv *game.Server, p *page, id string, seq int, lost *int) error {
	// One round trip: what the page holds back and has lost; if it holds a
	// packet back, its clock moves on to its next due timer (d ms).
	const look = `(() => { const u = __cs.netUp(); if (!u.held) return { ...u, d: 0 };
		const d = Math.max(0.1, Math.min(100, __vclock.nextDue()));
		return __vclock.advance(d).then(() => ({ ...u, d })); })()`
	var u struct {
		Held, Lost int
		D          float64
	}
	for ms := 0.0; ; {
		if srv.WaitCommand(id, seq, time.Millisecond) == nil {
			return nil
		}
		if err := p.eval(look, &u); err != nil {
			return err
		}
		ms += u.D
		switch {
		case ms >= faultMaxMs:
			return fmt.Errorf("%s: command %d never arrived (%.0f ms of the page's clock)", id, seq, ms)
		case u.Lost > *lost: // lost: the next packet would repeat it
			*lost = u.Lost
			if err := p.eval("__cs.resendCmds()", nil); err != nil {
				return err
			}
		case u.Held > 0: // it was held back: the clock moved on (look); see if it went
		default: // sent: in the socket
			return srv.WaitCommand(id, seq, cmdArrive)
		}
	}
}

// lostOf is m[k], made on first use.
func lostOf(m map[string]*int, k string) *int {
	if m[k] == nil {
		m[k] = new(int)
	}
	return m[k]
}

// caseSpeed is the horizontal speed of a case state's vel ([x, y, z]), or nil.
func caseSpeed(vel any) any {
	v, ok := vel.([]float64)
	if !ok || len(v) != 3 {
		return nil
	}
	return math.Hypot(v[0], v[2])
}
