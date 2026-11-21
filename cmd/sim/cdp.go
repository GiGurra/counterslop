package main

// A minimal Chrome DevTools Protocol client: just enough to open isolated
// pages, evaluate JS and take screenshots. Uses the websocket library the game
// server already depends on, so the harness adds no dependencies.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type browser struct {
	cmd     *exec.Cmd
	conn    *websocket.Conn
	dataDir string

	mu      sync.Mutex
	nextID  int
	pending map[int]chan cdpResp
	logs    map[string][]string // sessionId -> console errors
}

type cdpResp struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// findChrome returns a Chrome/Chromium binary: $CHROME, then common locations.
func findChrome() (string, error) {
	if p := os.Getenv("CHROME"); p != "" {
		return p, nil
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "/opt/google/chrome/chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no Chrome found; set CHROME=/path/to/chrome")
}

func launchBrowser(width, height int) (*browser, error) {
	bin, err := findChrome()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "cs-sim-chrome-")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin,
		"--headless=new", "--no-sandbox", "--mute-audio", "--no-first-run",
		"--enable-unsafe-swiftshader", "--use-angle=swiftshader",
		"--disable-background-timer-throttling", "--disable-renderer-backgrounding", "--disable-backgrounding-occluded-windows",
		fmt.Sprintf("--window-size=%d,%d", width, height),
		"--remote-debugging-port=0", "--user-data-dir="+dir, "about:blank")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Chrome prints its DevTools websocket URL on stderr.
	urlCh := make(chan string, 1)
	go func() {
		re := regexp.MustCompile(`DevTools listening on (ws://\S+)`)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if m := re.FindStringSubmatch(sc.Text()); m != nil {
				urlCh <- m[1]
			}
		}
	}()
	var wsURL string
	select {
	case wsURL = <-urlCh:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		return nil, errors.New("chrome did not start")
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	b := &browser{cmd: cmd, conn: conn, dataDir: dir, pending: map[int]chan cdpResp{}, logs: map[string][]string{}}
	go b.readLoop()
	return b, nil
}

func (b *browser) readLoop() {
	for {
		var m struct {
			ID        int             `json:"id"`
			Method    string          `json:"method"`
			SessionID string          `json:"sessionId"`
			Params    json.RawMessage `json:"params"`
			cdpResp
		}
		if err := b.conn.ReadJSON(&m); err != nil {
			return
		}
		if m.ID != 0 {
			b.mu.Lock()
			ch := b.pending[m.ID]
			delete(b.pending, m.ID)
			b.mu.Unlock()
			if ch != nil {
				ch <- m.cdpResp
			}
			continue
		}
		// Keep page errors and console errors/warnings: they go into the snapshot JSON.
		switch m.Method {
		case "Runtime.exceptionThrown":
			var p struct {
				Details struct {
					Text      string `json:"text"`
					Exception struct {
						Description string `json:"description"`
					} `json:"exception"`
				} `json:"exceptionDetails"`
			}
			_ = json.Unmarshal(m.Params, &p)
			b.addLog(m.SessionID, "exception: "+p.Details.Text+" "+p.Details.Exception.Description)
		case "Runtime.consoleAPICalled":
			var p struct {
				Type string `json:"type"`
				Args []struct {
					Value       any    `json:"value"`
					Description string `json:"description"`
				} `json:"args"`
			}
			_ = json.Unmarshal(m.Params, &p)
			if p.Type == "error" || p.Type == "warning" {
				parts := []string{}
				for _, a := range p.Args {
					if a.Value != nil {
						parts = append(parts, fmt.Sprint(a.Value))
					} else {
						parts = append(parts, a.Description)
					}
				}
				b.addLog(m.SessionID, p.Type+": "+strings.Join(parts, " "))
			}
		}
	}
}

func (b *browser) addLog(session, line string) {
	b.mu.Lock()
	b.logs[session] = append(b.logs[session], line)
	b.mu.Unlock()
}

func (b *browser) takeLogs(session string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	l := b.logs[session]
	delete(b.logs, session)
	return l
}

// call sends a CDP command (to the browser, or to a page when session != "").
func (b *browser) call(session, method string, params any, out any) error {
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	ch := make(chan cdpResp, 1)
	b.pending[id] = ch
	msg := map[string]any{"id": id, "method": method, "params": params}
	if session != "" {
		msg["sessionId"] = session
	}
	err := b.conn.WriteJSON(msg)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	select {
	case r := <-ch:
		if r.Error != nil {
			return fmt.Errorf("%s: %s", method, r.Error.Message)
		}
		if out != nil {
			return json.Unmarshal(r.Result, out)
		}
		return nil
	case <-time.After(60 * time.Second):
		return fmt.Errorf("%s: timeout", method)
	}
}

// page is one isolated browser context (own storage) with one tab.
type page struct {
	b       *browser
	session string
}

func (b *browser) newPage(url string, width, height int) (*page, error) {
	var ctx struct {
		ID string `json:"browserContextId"`
	}
	if err := b.call("", "Target.createBrowserContext", map[string]any{}, &ctx); err != nil {
		return nil, err
	}
	var tgt struct {
		ID string `json:"targetId"`
	}
	if err := b.call("", "Target.createTarget", map[string]any{"url": "about:blank", "browserContextId": ctx.ID, "newWindow": true}, &tgt); err != nil {
		return nil, err
	}
	var att struct {
		Session string `json:"sessionId"`
	}
	if err := b.call("", "Target.attachToTarget", map[string]any{"targetId": tgt.ID, "flatten": true}, &att); err != nil {
		return nil, err
	}
	p := &page{b: b, session: att.Session}
	for _, c := range []struct {
		m string
		p any
	}{
		{"Page.enable", map[string]any{}},
		{"Runtime.enable", map[string]any{}},
		{"Emulation.setDeviceMetricsOverride", map[string]any{"width": width, "height": height, "deviceScaleFactor": 1, "mobile": false}},
		{"Emulation.setFocusEmulationEnabled", map[string]any{"enabled": true}},
		{"Page.navigate", map[string]any{"url": url}},
	} {
		if err := b.call(p.session, c.m, c.p, nil); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// eval runs a JS expression (promises awaited) and decodes its JSON value into out.
func (p *page) eval(expr string, out any) error {
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	err := p.b.call(p.session, "Runtime.evaluate", map[string]any{"expression": expr, "awaitPromise": true, "returnByValue": true}, &r)
	if err != nil {
		return err
	}
	if r.Exception != nil {
		return fmt.Errorf("%s %s", r.Exception.Text, r.Exception.Exception.Description)
	}
	if out != nil && len(r.Result.Value) > 0 {
		return json.Unmarshal(r.Result.Value, out)
	}
	return nil
}

func (p *page) screenshot() ([]byte, error) {
	var r struct {
		Data []byte `json:"data"` // base64 in JSON, decoded by encoding/json
	}
	err := p.b.call(p.session, "Page.captureScreenshot", map[string]any{"format": "png"}, &r)
	return r.Data, err
}

func (b *browser) close() {
	_ = b.call("", "Browser.close", map[string]any{}, nil)
	_ = b.conn.Close()
	done := make(chan struct{})
	go func() { _ = b.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = b.cmd.Process.Kill()
	}
	_ = os.RemoveAll(b.dataDir)
}

// click presses and releases the left mouse button at (x, y): a trusted input
// event, so the page gets user activation.
func (p *page) click(x, y float64) error {
	for _, typ := range []string{"mousePressed", "mouseReleased"} {
		err := p.b.call(p.session, "Input.dispatchMouseEvent", map[string]any{"type": typ, "x": x, "y": y, "button": "left", "clickCount": 1}, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

// reload reloads the page and waits (up to 15 s) for window.__cs to exist again.
func (p *page) reload() error {
	// As the game reloads itself (no leave prompt, which would block headless).
	if err := p.eval("setTimeout(() => __cs.restartPage(), 0), true", nil); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	for range 150 {
		var ok bool
		if err := p.eval("typeof window.__cs === 'object'", &ok); err == nil && ok {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("test API did not come back after reload")
}
