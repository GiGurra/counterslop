package main

import (
	"math"
	"testing"
)

func TestParseCombo(t *testing.T) {
	for _, c := range []struct {
		in        string
		mods      int
		key, code string
		vk        int
	}{
		{"ctrl+c", 2, "c", "KeyC", 'C'},
		{"Shift+Insert", 8, "Insert", "Insert", 45},
		{"meta+v", 4, "v", "KeyV", 'V'},
		{"shift+a", 8, "A", "KeyA", 'A'},
		{"enter", 0, "Enter", "Enter", 13},
		{"space", 0, " ", "Space", 32},
		{"1", 0, "1", "Digit1", '1'},
	} {
		mods, k, err := parseCombo(c.in)
		if err != nil || mods != c.mods || k.key != c.key || k.code != c.code || k.vk != c.vk {
			t.Errorf("parseCombo(%q) = %d %+v %v, want %d %q %q %d", c.in, mods, k, err, c.mods, c.key, c.code, c.vk)
		}
	}
	for _, bad := range []string{"hyper+c", "f13", "ctrl+"} {
		if _, _, err := parseCombo(bad); err == nil {
			t.Errorf("parseCombo(%q): no error", bad)
		}
	}
}

// The console slides in with a CSS transition, which runs on the browser's wall
// clock: a trusted drag read the log's box mid-slide and pressed where it was a
// moment before, selecting nothing (console-clipboard, a CI flake under load).
// rect must give where the element comes to rest. A 10 s slide makes it
// certain: without finishing the transition the box is still near its start.
func TestRectIsWhereTheElementComesToRest(t *testing.T) {
	if _, err := findChrome(); err != nil {
		t.Skip("no Chrome:", err)
	}
	b, err := launchBrowser(800, 600, false)
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	const page = `data:text/html,<style>%23log{position:fixed;left:0;top:100px;width:400px;height:200px;transform:translateY(-400px);transition:transform 10s linear}%23log.open{transform:none}</style><div id="log">hello slop</div>`
	p, err := b.newPage(page, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	// The data: page replaces about:blank: wait for it (each try is a CDP round trip, no clock).
	var ready bool
	for i := 0; i < 2000 && !ready; i++ {
		if err := p.eval(`!!document.getElementById("log") && document.readyState === "complete"`, &ready); err != nil {
			t.Fatal(err)
		}
	}
	if !ready {
		t.Fatal("the test page never loaded")
	}
	// Open it as the console opens: the slide starts now.
	if err := p.eval(`(() => { const e = document.getElementById("log"); e.getBoundingClientRect(); e.classList.add("open"); e.getBoundingClientRect(); return true; })()`, nil); err != nil {
		t.Fatal(err)
	}
	_, y0, _, y1, err := p.rect("#log")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(y0-100) > 0.5 || math.Abs(y1-300) > 0.5 {
		t.Fatalf("rect top..bottom = %.1f..%.1f, want 100..300 (where it comes to rest, not mid-slide)", y0, y1)
	}
}
