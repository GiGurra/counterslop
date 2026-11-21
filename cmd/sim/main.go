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
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gigurra/counterslop/internal/app"
	"github.com/gigurra/counterslop/internal/game"
)

// Scenario is a JSON file in sim/scenarios.
//
//	{"description": "...", "clients": ["alice", "bob"], "steps": [
//	  {"join": true},                                  every client joins, in order
//	  {"client": "alice", "call": "set", "args": [{"pos": [0,0,0], "yaw": 1}]},
//	  {"client": "*", "call": "time", "args": [12]},   any window.__cs method; "*" = all
//	  {"step": 30},                                    every client simulates 30 frames (1/60 s)
//	  {"wait": 200},                                   real milliseconds (network catch-up)
//	  {"snapshot": "name", "clients": ["alice"]}       PNG + JSON per client (default: all)
//	]}
type Scenario struct {
	Description string   `json:"description"`
	Clients     []string `json:"clients"`
	Steps       []Step   `json:"steps"`
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
}

var (
	dir      = flag.String("scenarios", "sim/scenarios", "scenario directory")
	outDir   = flag.String("out", "out/snapshots", "output directory")
	webDir   = flag.String("dev", "web", "serve the client from this directory (empty = embedded copy)")
	golden   = flag.String("golden", "sim/golden", "golden image directory for -diff / -update-golden")
	diff     = flag.Bool("diff", false, "compare snapshots with golden images and report differences")
	update   = flag.Bool("update-golden", false, "copy this run's snapshots into the golden directory")
	list     = flag.Bool("list", false, "list scenarios and exit")
	width    = flag.Int("width", 1280, "viewport width")
	height   = flag.Int("height", 720, "viewport height")
	tolerant = flag.Float64("tolerance", 0.01, "max fraction of differing pixels for -diff")
	cases    = flag.Bool("cases", false, "run sim/cases in lockstep end to end instead of the scenarios (args: case names)")
	casesDir = flag.String("cases-dir", "sim/cases", "case directory for -cases")
	jobs     = flag.Int("j", 4, "parallel workers for -cases (each: its own server and browser)")
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

	if *cases {
		res := runCases(flag.Args(), *jobs)
		writeCaseSummary(res)
		for _, r := range res {
			if len(r.Fails) > 0 {
				os.Exit(1)
			}
		}
		return
	}
	base, _, stop, err := startServer()
	if err != nil {
		log.Fatal(err)
	}
	defer stop()

	failed := 0
	for _, n := range names {
		if err := run(base, n); err != nil {
			log.Printf("FAIL %s: %v", n, err)
			failed++
		}
	}
	writeIndex()
	log.Printf("snapshots: %s", filepath.Join(*outDir, "index.html"))
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
	return &sc, nil
}

// startServer runs the real game server in-process, in test mode, on a free port.
func startServer() (string, *game.Server, func(), error) {
	a, err := app.New(app.Options{Version: "sim", Dev: *webDir, Test: true, Bots: 0})
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
	return "http://" + ln.Addr().String(), a.Server, func() { close(stopSim); _ = srv.Close() }, nil
}

func run(base, name string) error {
	sc, err := loadScenario(name)
	if err != nil {
		return err
	}
	out := filepath.Join(*outDir, name)
	_ = os.RemoveAll(out)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	log.Printf("== %s: %s", name, sc.Description)

	b, err := launchBrowser(*width, *height)
	if err != nil {
		return err
	}
	defer b.close()

	pages := map[string]*page{}
	for _, c := range sc.Clients {
		p, err := b.newPage(base+"/?test=1", *width, *height)
		if err != nil {
			return err
		}
		pages[c] = p
	}
	for _, c := range sc.Clients { // wait for the client to boot
		if err := waitFor(pages[c], "!!window.__cs && !!window.cs"); err != nil {
			return fmt.Errorf("%s: client didn't start: %w (logs: %v)", c, err, b.takeLogs(pages[c].session))
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
	for i, st := range sc.Steps {
		switch {
		case st.Join:
			for _, c := range sc.Clients {
				if err := pages[c].eval(fmt.Sprintf("__cs.join(%q)", c), nil); err != nil {
					return fmt.Errorf("step %d join %s: %w", i, c, err)
				}
			}
		case st.Call != "":
			if st.Args == nil {
				st.Args = []any{}
			}
			args, _ := json.Marshal(st.Args)
			for _, c := range targets(st.Client, nil) {
				expr := fmt.Sprintf("__cs[%q](...%s)", st.Call, args)
				if err := pages[c].eval(expr, nil); err != nil {
					return fmt.Errorf("step %d %s %s: %w", i, c, st.Call, err)
				}
			}
		case st.Step > 0:
			for _, c := range sc.Clients {
				if err := pages[c].eval(fmt.Sprintf("(__cs.step(%d), true)", st.Step), nil); err != nil {
					return fmt.Errorf("step %d: %w", i, err)
				}
			}
		case st.Wait > 0:
			time.Sleep(time.Duration(st.Wait) * time.Millisecond)
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
		case st.Snapshot != "":
			shot++
			for _, c := range targets(st.Client, st.Clients) {
				file := fmt.Sprintf("%02d-%s-%s", shot, st.Snapshot, c)
				if err := snapshot(pages[c], filepath.Join(out, file), base); err != nil {
					return fmt.Errorf("snapshot %s: %w", file, err)
				}
				log.Printf("   %s.png", file)
			}
		default:
			return fmt.Errorf("step %d does nothing: %+v", i, st)
		}
	}
	if *update || *diff {
		return goldenCheck(name, out)
	}
	return nil
}

func waitFor(p *page, cond string) error {
	for range 200 {
		var ok bool
		if err := p.eval(cond, &ok); err == nil && ok {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", cond)
}

// snapshot renders one frame (no time passes), then writes the PNG and a JSON
// with the client's state, the server's state and any page errors.
func snapshot(p *page, path, base string) error {
	var client any
	if err := p.eval("__cs.step(1, 0)", &client); err != nil {
		return err
	}
	png, err := p.screenshot()
	if err != nil {
		return err
	}
	var server any
	if resp, err := http.Get(base + "/debug/state"); err == nil {
		_ = json.NewDecoder(resp.Body).Decode(&server)
		resp.Body.Close()
	}
	logs := p.b.takeLogs(p.session)
	state, _ := json.MarshalIndent(map[string]any{"client": client, "server": server, "errors": logs}, "", "  ")
	if err := os.WriteFile(path+".png", png, 0o644); err != nil {
		return err
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
