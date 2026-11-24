// Command sim runs scripted game scenarios against an in-process server with
// headless Chrome clients and writes snapshots (PNG of world + UI, and a JSON
// state dump) so agents without a display can see the game.
//
//	go run ./cmd/sim                    # every scenario in sim/scenarios
//	go run ./cmd/sim two-players-duel   # one scenario
//	go run ./cmd/sim -list
//
// Output: out/snapshots/<scenario>/<NN>-<shot>-<client>.png + .json, and an
// index.html contact sheet per run (out/snapshots/index.html).
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gigurra/counterslop/internal/app"
	"github.com/gigurra/counterslop/internal/game"
)

// Scenario is a JSON file in sim/scenarios.
//
// Scenarios run in lockstep (lockstep.go): the server only ticks when the
// harness steps it, so a run plays the same on a loaded CI box. "realtime":
// true runs one on wall time instead (pacing, audio, fake lag, packet faults).
// A failed step writes fail-<client>.png/.json (+ -server.json, -testctl.json).
//
//	{"description": "...", "clients": ["alice", "bob"], "steps": [
//	  {"join": true},                                  every client joins, in order
//	  {"client": "alice", "call": "set", "args": [{"pos": [0,0,0], "yaw": 1}]},
//	  {"client": "*", "call": "time", "args": [12]},   any window.__cs method; "*" = all
//	  {"step": 30},                                    every client simulates 30 frames (1/60 s); the server runs the ticks they cover
//	  {"wait": 200},                                   200 ms of server time (lockstep), then every client has caught up
//	  {"snapshot": "name", "clients": ["alice"]}       PNG + JSON per client (default: all)
//	  {"viewport": [390, 640]}                         resize the viewport (CSS px) for the rest of the run
//	  {"draw": true}                                   draw one frame on the GPU (lockstep frames don't), before asserting gfx stats
//	  {"draw": true, "bake": true}                     ... after baking the lamp shadows, as a snapshot does
//	]}
type Scenario struct {
	Description string   `json:"description"`
	Clients     []string `json:"clients"`
	Steps       []Step   `json:"steps"`
	// Realtime: the server ticks on wall time, as in production. Only for
	// scenarios about wall time itself (frame pacing, audio, fake lag, packet
	// faults). Otherwise the run is lockstep: see lockstep.go.
	Realtime bool `json:"realtime,omitempty"`
	// CallTimeout (s): a longer hang guard on each of this scenario's calls than
	// the default 60 s, for calls that are slow, not stuck (renderer benches
	// on gs_megabyte under software GL: seconds a frame on a CI runner). Such a
	// scenario runs alone, not beside other scenarios' browsers, so its
	// timings mean something. CallTimeoutReason says why (required with it).
	CallTimeout       int    `json:"callTimeout,omitempty"`
	CallTimeoutReason string `json:"callTimeoutReason,omitempty"`
}

type Step struct {
	Join     bool     `json:"join,omitempty"`
	Client   string   `json:"client,omitempty"`
	Call     string   `json:"call,omitempty"`
	Args     []any    `json:"args,omitempty"`
	Step     int      `json:"step,omitempty"`
	Wait     int      `json:"wait,omitempty"`
	Snapshot string   `json:"snapshot,omitempty"`
	Clients  []string `json:"clients,omitempty"`
	// Click [x, y] (viewport px) with a real (trusted) mouse click via CDP:
	// unlike a synthetic DOM event it counts as a user gesture (audio, lock).
	Click []float64 `json:"click,omitempty"`
	// Reload the page (as a version update or renderer restart does) and wait
	// for the test API to come back.
	Reload bool `json:"reload,omitempty"`
	// Viewport [w, h] resizes the clients' viewports (CSS px), e.g. to see a
	// layout at phone width. It stays for the rest of the scenario.
	Viewport []int `json:"viewport,omitempty"`
	// Draw renders one frame on the GPU (dt 0, after the map and its lamp
	// shadows are ready) for a step that asserts what the renderer did (gfx
	// stats, marks drawn). Lockstep frames otherwise never touch the GPU, and a
	// snapshot under -no-shots draws nothing.
	Draw bool `json:"draw,omitempty"`
	// Bake (with draw): first render frozen frames until the lamp shadows
	// are baked, as a snapshot does (for asserting their counts).
	Bake bool `json:"bake,omitempty"`
	// Redeploy: the server switches to this client build and drops every
	// connection, as a deploy does; then wait until every client has reloaded
	// itself onto that build (its page's cs-build meta says so). The startup
	// build is back for the next scenario.
	Redeploy string `json:"redeploy,omitempty"`
}

// noGLJS: a lockstep page's frames do no GPU work; it draws only when a step
// asks (snapshot, draw, a failed step): __cs.draw (web/game/testapi.js).
const noGLJS = "window.__csNoGL = true;"

// vclockJS is the virtual clock a lockstep page runs on (vclock.js).
//
//go:embed vclock.js
var vclockJS string

var (
	dir      = flag.String("scenarios", "sim/scenarios", "scenario directory")
	outDir   = flag.String("out", "out/snapshots", "output directory")
	webDir   = flag.String("dev", "web", "serve the client from this directory (empty = embedded copy)")
	netCodec = flag.String("net-codec", "json", "the server's snapshot codec (--net-codec: json, delta)")
	golden   = flag.String("golden", "sim/golden", "golden image directory for -diff / -update-golden")
	diff     = flag.Bool("diff", false, "compare snapshots with golden images and report differences")
	realtime = flag.Bool("realtime", false, "run every scenario on wall time (no lockstep), as before")
	// A lockstep run must play the same on a slow machine: -cpu-throttle 6 proves it (CI runs the smoke set so).
	cpuThrottle = flag.Float64("cpu-throttle", 1, "slow every page's CPU down this many times (Chrome Emulation.setCPUThrottlingRate)")
	advisory    = flag.Bool("diff-advisory", false, "with -diff: list differing scenarios (job summary) without failing them")
	update      = flag.Bool("update-golden", false, "copy this run's snapshots into the golden directory")
	list        = flag.Bool("list", false, "list scenarios and exit")
	width       = flag.Int("width", 1280, "viewport width")
	height      = flag.Int("height", 720, "viewport height")
	tolerant    = flag.Float64("tolerance", 0.01, "max fraction of differing pixels for -diff")
	cases       = flag.Bool("cases", false, "run sim/cases in lockstep end to end instead of the scenarios (args: case names)")
	casesDir    = flag.String("cases-dir", "sim/cases", "case directory for -cases")
	shard       = flag.String("shard", "", "run only shard i/n of the scenarios (e.g. 1/2)")
	jobs        = flag.Int("j", 4, "parallel workers (each: its own server and browser); realtime scenarios still run one at a time")
	slowStep    = flag.Duration("slow-step", 2*time.Second, "log every scenario step that takes longer than this (wall time)")
	noShots     = flag.Bool("no-shots", false, "snapshot steps write only the JSON state (no GPU draw, no PNG); a failed step still gets its fail-<client>.png (CI)")
	traceDir    = flag.String("trace", "", "with -cases: write each case's per-tick server state to DIR/<case>.jsonl (compare runs: lockstep must repeat bit for bit)")
	matrixOut   = flag.String("matrix", "", "with -cases: also write the registry coverage matrix (markdown) here")
)

func main() {
	flag.Parse()
	log.SetFlags(0)
	names, err := scenarioNames()
	if err != nil {
		log.Fatal(err)
	}
	if *list {
		for _, n := range names {
			sc, _ := loadScenario(n)
			fmt.Printf("%-24s %s\n", n, sc.Description)
		}
		return
	}
	if flag.NArg() > 0 {
		names = flag.Args()
	}
	if *shard != "" { // "i/n": shard i of n (1-based) by sim/timings.json, for parallel CI jobs (shard.go)
		var i, n int
		if _, err := fmt.Sscanf(*shard, "%d/%d", &i, &n); err != nil || i < 1 || i > n {
			log.Fatalf("-shard %q: want i/n with 1 <= i <= n", *shard)
		}
		names = shardOf(names, i, n, loadTimings(filepath.Join(filepath.Dir(*dir), "timings.json")))
		log.Printf("shard %d/%d: %s", i, n, strings.Join(names, " "))
	}

	if *cases {
		res, reg, byName := runCases(flag.Args(), *jobs)
		failed := false
		for _, r := range res {
			failed = failed || len(r.Fails) > 0
		}
		matrix := ""
		if flag.NArg() == 0 { // the registry check needs every case
			var missing []string
			matrix, missing = registryMatrix(reg, res, byName)
			if len(missing) > 0 {
				log.Printf("FAIL registry: %d entries have no passing lockstep case (and no exemption in sim/registry-exempt.json): %s", len(missing), strings.Join(missing, " "))
				failed = true
			}
		}
		writeCaseSummary(res, matrix)
		if *matrixOut != "" {
			_ = os.WriteFile(*matrixOut, []byte(matrix), 0o644)
		}
		if r := failRecap(res); r != "" {
			log.Print(r) // last, so a run's tail says which case failed and why
		}
		if failed {
			os.Exit(1)
		}
		return
	}
	failed, took := runScenarios(names, *jobs)
	writeTimings(filepath.Join(*outDir, "timings.json"), took) // refreshes sim/timings.json (see shard.go)
	writeIndex()
	log.Printf("snapshots: %s", filepath.Join(*outDir, "index.html"))
	if *diff && *advisory {
		md := "### Golden image diff (vs the previous nightly)\n\n"
		sort.Strings(goldenDiffs)
		if len(goldenDiffs) == 0 {
			md += "No scenario's snapshots differ beyond the tolerance.\n"
		}
		for _, d := range goldenDiffs {
			md += "- " + d + "\n"
			log.Printf("golden differs: %s", d)
		}
		if f := os.Getenv("GITHUB_STEP_SUMMARY"); f != "" {
			if fh, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644); err == nil {
				fh.WriteString(md)
				fh.Close()
			}
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}

func scenarioNames() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(*dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	sort.Strings(names)
	return names, nil
}

func loadScenario(name string) (*Scenario, error) {
	b, err := os.ReadFile(filepath.Join(*dir, name+".json"))
	if err != nil {
		return nil, err
	}
	var sc Scenario
	if err := json.Unmarshal(b, &sc); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if sc.CallTimeout > 0 && sc.CallTimeoutReason == "" {
		return nil, fmt.Errorf("%s: callTimeout needs a callTimeoutReason", name)
	}
	return &sc, nil
}

// startServer runs the real game server in-process, in test mode, on a free port.
func startServer() (string, *app.App, func(), error) {
	a, err := app.New(app.Options{Version: "sim", Dev: *webDir, Test: true, Bots: 0, NetCodec: *netCodec})
	if err != nil {
		return "", nil, nil, err
	}
	stopSim := make(chan struct{})
	go a.Server.Run(stopSim)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, nil, err
	}
	srv := &http.Server{Handler: a.Handler}
	go func() { _ = srv.Serve(ln) }()
	return "http://" + ln.Addr().String(), a, func() { close(stopSim); _ = srv.Close() }, nil
}

// runScenarios runs the scenarios on j workers, each with its own server and
// browser (reused from one scenario to the next: a fresh browser context per
// page, the world reset in between). Realtime scenarios (wall time: pacing,
// audio, fake lag) run last, one at a time, so no other scenario loads the
// machine under them. A scenario's log is printed in one piece when it ends.
// Returns how many failed and each one's wall time.
func runScenarios(names []string, j int) (failed int, took map[string]float64) {
	lock, alone, real := phases(names, *realtime)
	took = map[string]float64{}
	var mu sync.Mutex
	work := func(list []string, j int, soft bool) {
		next := make(chan string)
		var wg sync.WaitGroup
		for range max(1, min(j, len(list))) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				base, a, stop, err := startServer()
				if err != nil {
					log.Fatal(err)
				}
				defer stop()
				srv := a.Server
				var b *browser
				defer func() {
					if b != nil {
						b.close()
					}
				}()
				for n := range next {
					var buf strings.Builder
					lg := log.New(&buf, "", 0)
					t0 := time.Now() // wall: reports how long a scenario took
					if b == nil {
						if b, err = launchBrowser(*width, *height, soft); err != nil {
							log.Fatal(err)
						}
					}
					// The last scenario's pages are closed, but their sockets close on
					// their own time: its alice could still be leaving when this one's
					// alice joins (name-tags failed so in CI, after team-cap).
					err := waitServerEmpty(srv, 10*time.Second)
					if err == nil {
						srv.ResetWorld() // no state leaks from the scenario before (map, bots, cvars, items)
						err = run(base, a, b, n, lg)
					}
					if err != nil {
						lg.Printf("FAIL %s: %v", n, err)
						b.close() // a failed scenario may leave the browser in any state: the next one gets a new one
						b = nil
					}
					mu.Lock()
					took[n] = time.Since(t0).Seconds() // wall: report
					if err != nil {
						failed++
					}
					log.Print(strings.TrimSuffix(buf.String(), "\n"))
					mu.Unlock()
				}
			}()
		}
		for _, n := range list {
			next <- n
		}
		close(next)
		wg.Wait()
	}
	work(lock, j, true)  // nothing drawn unless asked: software compositing (launchBrowser)
	work(alone, 1, true) // heavy (callTimeout): one at a time
	work(real, 1, false) // drawing every frame: GPU compositing
	return failed, took
}

// phases splits the scenarios by how they run: lock (lockstep, j at a time),
// alone (lockstep, heavy: "callTimeout", one at a time) and real (wall time,
// one at a time). A scenario that doesn't load runs in lock and fails there.
func phases(names []string, allRealtime bool) (lock, alone, real []string) {
	for _, n := range names {
		sc, err := loadScenario(n)
		switch {
		case err == nil && (sc.Realtime || allRealtime):
			real = append(real, n)
		case err == nil && sc.CallTimeout > 0:
			alone = append(alone, n)
		default:
			lock = append(lock, n)
		}
	}
	return lock, alone, real
}

// waitServerEmpty waits until no client is connected to srv (bots aside).
func waitServerEmpty(srv *game.Server, timeout time.Duration) error {
	deadline := time.Now().Add(timeout) // wall: hang guard
	for srv.Connected() > 0 {
		if time.Now().After(deadline) { // wall: hang guard
			return fmt.Errorf("%d clients never left the server", srv.Connected())
		}
		time.Sleep(5 * time.Millisecond) // wall: polls (sockets close on their own time)
	}
	return nil
}

func run(base string, a *app.App, b *browser, name string, log *log.Logger) (err error) {
	srv := a.Server
	sc, err := loadScenario(name)
	if err != nil {
		return err
	}
	if sc.CallTimeout > 0 {
		b.callTimeout = time.Duration(sc.CallTimeout) * time.Second // wall: hang guard (callTimeoutReason)
		defer func() { b.callTimeout = 0 }()
	}
	out := filepath.Join(*outDir, name)
	_ = os.RemoveAll(out)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	log.Printf("== %s: %s", name, sc.Description)

	lockstepOn := !sc.Realtime && !*realtime
	init := pageInitFor(lockstepOn, *cpuThrottle)
	pages := map[string]*page{}
	// Lockstep from before the first page boots: a lockstep page's keepalive
	// runs on its virtual clock, stopped while the others boot, and outside
	// lockstep the server drops a client silent for 15 s of wall time (on a
	// loaded machine the first page was dropped before the last one booted).
	ls := newLockstep(srv, pages, sc.Clients, lockstepOn)
	defer ls.stop()
	// One at a time, in client order: a page connects as it boots, and the
	// server's IDs (which decide ties: the latest joiner moved to spectators,
	// push-outs) follow connect order. Booted at once, they varied run to run.
	for _, c := range sc.Clients {
		p, err := b.newPage(base+"/?test=1", *width, *height, init)
		if err != nil {
			return err
		}
		pages[c] = p
		defer p.dispose()
		if err := waitFor(p, "!!window.__cs && !!window.cs && window.cs.net?.open"); err != nil {
			return fmt.Errorf("%s: client didn't start: %w (logs: %v)", c, err, b.takeLogs(p.session))
		}
	}
	targets := func(who string, list []string) []string {
		if len(list) > 0 {
			return list
		}
		if who == "" || who == "*" {
			return sc.Clients
		}
		return []string{who}
	}

	shot := 0
	defer func() { // a failed step: every client's state at that moment, as fail-<client>.png/.json
		if err != nil {
			for _, c := range sc.Clients {
				if id := ls.id[c]; id != "" { // the server's view of that player (CaseState)
					b, _ := json.MarshalIndent(srv.CaseState(id), "", " ")
					_ = os.WriteFile(filepath.Join(out, "fail-"+c+"-server.json"), b, 0o644)
				}
				var ctl json.RawMessage
				if pages[c].eval("JSON.stringify(__cs.shotDebug())", &ctl) == nil {
					_ = os.WriteFile(filepath.Join(out, "fail-"+c+"-testctl.json"), ctl, 0o644)
				}
				if e := snapshot(pages[c], filepath.Join(out, "fail-"+c), base, true); e != nil {
					log.Printf("   fail-%s: %v", c, e)
				}
			}
		}
	}()
	for i, st := range sc.Steps {
		t0 := time.Now() // wall: reports slow steps (-slow-step)
		switch {
		case st.Join:
			for _, c := range sc.Clients {
				if err := ls.call(c, "join", []byte(fmt.Sprintf("[%q]", c))); err != nil {
					return fmt.Errorf("step %d join %s: %w", i, c, err)
				}
			}
		case st.Call != "":
			if st.Args == nil {
				st.Args = []any{}
			}
			args, _ := json.Marshal(st.Args)
			for _, c := range targets(st.Client, nil) {
				if err := ls.call(c, st.Call, args); err != nil {
					return fmt.Errorf("step %d %s %s: %w", i, c, st.Call, err)
				}
			}
		case st.Step > 0:
			if err := ls.step(st.Step); err != nil {
				return fmt.Errorf("step %d: %w", i, err)
			}
		case st.Wait > 0:
			if err := ls.wait(st.Wait); err != nil {
				return fmt.Errorf("step %d (wait): %w", i, err)
			}
		case len(st.Click) == 2:
			for _, c := range targets(st.Client, nil) {
				if err := pages[c].click(st.Click[0], st.Click[1]); err != nil {
					return fmt.Errorf("step %d click %s: %w", i, c, err)
				}
			}
		case st.Reload:
			for _, c := range targets(st.Client, nil) {
				if err := pages[c].reload(); err != nil {
					return fmt.Errorf("step %d reload %s: %w", i, c, err)
				}
			}
		case len(st.Viewport) == 2:
			for _, c := range targets(st.Client, nil) {
				if err := pages[c].resize(st.Viewport[0], st.Viewport[1]); err != nil {
					return fmt.Errorf("step %d viewport %s: %w", i, c, err)
				}
			}
		case st.Draw:
			if err := ls.each(func(_ int, c string) error {
				if !slices.Contains(targets(st.Client, st.Clients), c) {
					return nil
				}
				return pages[c].eval(fmt.Sprintf("__cs.draw({ bake: %v }).then(() => true)", st.Bake), nil)
			}); err != nil {
				return fmt.Errorf("step %d draw: %w", i, err)
			}
		case st.Redeploy != "":
			defer a.Redeploy(a.Build)
			a.Redeploy(st.Redeploy)
			cond := fmt.Sprintf(`!!window.__cs && !!window.cs && document.querySelector('meta[name="cs-build"]')?.content === %q`, st.Redeploy)
			for _, c := range targets(st.Client, nil) {
				if err := waitAdvancing(pages[c], cond); err != nil {
					return fmt.Errorf("step %d redeploy %s: no reload onto %s: %w", i, c, st.Redeploy, err)
				}
			}
		case st.Snapshot != "":
			shot++
			for _, c := range targets(st.Client, st.Clients) {
				file := fmt.Sprintf("%02d-%s-%s", shot, st.Snapshot, c)
				if err := snapshot(pages[c], filepath.Join(out, file), base, !*noShots); err != nil {
					return fmt.Errorf("snapshot %s: %w", file, err)
				}
				if !*noShots {
					log.Printf("   %s.png", file)
				}
			}
		default:
			return fmt.Errorf("step %d does nothing: %+v", i, st)
		}
		if st.Call != "" || st.Join || len(st.Click) == 2 || len(st.Viewport) == 2 {
			if err := ls.settle(); err != nil {
				return fmt.Errorf("step %d: %w", i, err)
			}
		}
		if d := time.Since(t0); d > *slowStep { // wall: report
			js, _ := json.Marshal(st)
			log.Printf("   slow step %d (%.1f s): %s", i, d.Seconds(), js)
		}
	}
	if *update || *diff {
		return goldenCheck(name, out, log)
	}
	return nil
}

// waitAdvancing is waitFor for something the page does on its own timers (a
// reconnect, a version check and reload after a redeploy): on a virtual clock
// (lockstep) it moves the page's clock straight to its next due timer, but
// only once the page is quiet (no socket connecting: that is real network
// work, waited for in real time as a hang guard). So the page's clock reaches
// the same time on every run, however fast the machine. Errors in between are
// the page navigating away.
func waitAdvancing(p *page, cond string) error {
	const next = `(() => { if (!window.__vclock) return -1; if (window.cs?.net?.connecting?.()) return -2;
		const d = __vclock.nextDue(); return d === Infinity ? -3 : d; })()`
	for range 2000 {
		var ok bool
		if err := p.eval(cond, &ok); err == nil && ok {
			return nil
		}
		var d float64
		if err := p.eval(next, &d); err != nil || d < 0 {
			time.Sleep(10 * time.Millisecond) // wall: polls browser work (navigation, a socket connecting); hang guard 20 s
			continue
		}
		_ = p.eval(advanceJS(max(d, 0.1))+".then(() => true)", nil)
	}
	return fmt.Errorf("timed out waiting for %s", cond)
}

func waitFor(p *page, cond string) error {
	for range 200 {
		var ok bool
		if err := p.eval(cond, &ok); err == nil && ok {
			return nil
		}
		time.Sleep(50 * time.Millisecond) // wall: polls; hang guard 10 s
	}
	return fmt.Errorf("timed out waiting for %s", cond)
}

// snapshot runs one frame (no time passes), then writes a JSON with the
// client's state, the server's state and any page errors, and with png the
// frame drawn on the GPU as a PNG. Without png (-no-shots) nothing is drawn:
// the frame does all but the GPU work, as a lockstep step's frames do.
func snapshot(p *page, path, base string, png bool) error {
	var client any
	// A map still loading (its shader compiling in the background) is drawn first.
	js := "__cs.mapReady().then(() => __cs.step(1, 0))"
	if png {
		js = "window.__csNoGL ? __cs.draw() : " + js // lockstep pages draw only here
	}
	if err := p.eval(js, &client); err != nil {
		return err
	}
	var img []byte
	if png {
		var err error
		if img, err = p.screenshot(); err != nil {
			return err
		}
	}
	var server any
	if resp, err := http.Get(base + "/debug/state"); err == nil {
		_ = json.NewDecoder(resp.Body).Decode(&server)
		resp.Body.Close()
	}
	logs := p.b.takeLogs(p.session)
	state, _ := json.MarshalIndent(map[string]any{"client": client, "server": server, "errors": logs}, "", "  ")
	if png {
		if err := os.WriteFile(path+".png", img, 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path+".json", state, 0o644); err != nil {
		return err
	}
	// An uncaught page exception fails the scenario (CI gates deploys on this).
	for _, l := range logs {
		if strings.HasPrefix(l, "exception:") {
			return fmt.Errorf("page exception: %s", l)
		}
	}
	return nil
}

// writeIndex makes a contact sheet of every scenario's latest snapshots.
func writeIndex() {
	dirs, _ := os.ReadDir(*outDir)
	var sb strings.Builder
	sb.WriteString(`<!doctype html><meta charset="utf-8"><title>Sim Snapshots</title>
<style>body{background:#111;color:#eee;font:14px system-ui;margin:16px}h2{color:#8bff4f;margin:24px 0 8px}
.g{display:flex;flex-wrap:wrap;gap:12px}figure{margin:0;max-width:min(640px,100%)}img{width:100%;border:1px solid #333}
figcaption{font:12px ui-monospace,monospace;opacity:.8}a{color:#7cc4ff}</style><h1>counterslop sim snapshots</h1>`)
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		pngs, _ := filepath.Glob(filepath.Join(*outDir, d.Name(), "*.png"))
		sort.Strings(pngs)
		fmt.Fprintf(&sb, "<h2>%s</h2><div class=g>", html.EscapeString(d.Name()))
		for _, p := range pngs {
			rel := filepath.Join(d.Name(), filepath.Base(p))
			js := strings.TrimSuffix(rel, ".png") + ".json"
			fmt.Fprintf(&sb, `<figure><img src="%s" loading="lazy"><figcaption>%s · <a href="%s">state</a></figcaption></figure>`,
				html.EscapeString(rel), html.EscapeString(filepath.Base(p)), html.EscapeString(js))
		}
		sb.WriteString("</div>")
	}
	_ = os.WriteFile(filepath.Join(*outDir, "index.html"), []byte(sb.String()), 0o644)
}

// failRecap is one line naming every failed case with its first failure
// (cut to 300 characters), or "" when none failed. A FAIL registry line only
// lists the uncovered commands; this says which case didn't pass and why.
func failRecap(res []caseResult) string {
	var parts []string
	for _, r := range res {
		if len(r.Fails) == 0 {
			continue
		}
		first := r.Fails[0]
		if len(first) > 300 {
			first = first[:300] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s (%s): %s", r.Name, r.Took.Round(time.Millisecond), first))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("FAILED %d case(s): %s", len(parts), strings.Join(parts, " | "))
}

// pageInitFor: a lockstep page runs its clocks and timers on game time
// (vclock.js); a realtime one never does.
func pageInitFor(lockstep bool, throttle float64) pageInit {
	in := pageInit{throttle: throttle}
	if lockstep {
		in.scripts = []string{vclockJS, noGLJS}
	}
	return in
}
