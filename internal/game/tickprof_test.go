package game

import (
	"strings"
	"testing"
	"time"
)

// A scripted clock for the tick profiler: each read moves it on by step.
func stepClock(step time.Duration) func() time.Time {
	t := gameEpoch
	return func() time.Time { t = t.Add(step); return t }
}

// The window's numbers from scripted section times: means, nearest-rank
// percentiles, max, the tick total, largest section first.
func TestTickProfReport(t *testing.T) {
	var p tickProf
	var at time.Time
	p.now = func() time.Time { return at }
	for i := range 100 {
		p.start()
		at = at.Add(time.Millisecond) // cmds: 1 ms every tick
		p.mark(secCmds)
		ai := time.Duration(0)
		if i == 99 {
			ai = 5 * time.Millisecond // one slow AI tick
		} else if i >= 95 {
			ai = 2 * time.Millisecond
		}
		at = at.Add(ai)
		p.mark(secAI)
		p.resume() // the snapshot, after advance: what ran in between isn't counted
		at = at.Add(time.Hour)
		p.resume()
		at = at.Add(500 * time.Microsecond)
		p.mark(secSnapshot)
	}
	p.start() // closes the 100th tick
	r := p.report()
	if r.Ticks != 100 || r.BudgetMs != 10 {
		t.Fatalf("ticks %d budget %v", r.Ticks, r.BudgetMs)
	}
	sec := map[string]profStat{}
	for _, s := range r.Sections {
		sec[s.Name] = s
	}
	if c := sec["cmds"]; c.Mean != 1 || c.Max != 1 || c.P99 != 1 {
		t.Fatalf("cmds %+v", c)
	}
	if a := sec["ai"]; a.Max != 5 || a.P95 != 0 || a.P99 != 2 || a.Mean != 0.13 {
		t.Fatalf("ai %+v", a)
	}
	if s := sec["snapshot"]; s.Mean != 0.5 {
		t.Fatalf("snapshot %+v (the hour between advance and the snapshot must not count)", s)
	}
	if r.Tick.Mean != 1.63 || r.Tick.Max != 6.5 {
		t.Fatalf("tick %+v", r.Tick)
	}
	if r.Sections[0].Name != "cmds" || r.Sections[1].Name != "snapshot" || r.Sections[2].Name != "ai" {
		t.Fatalf("order %v", r.Sections)
	}
	if l := r.lines(); len(l) != 2+int(nSecs) || !strings.Contains(l[0], "of the 10 ms budget: 16.3%") {
		t.Fatalf("lines %q", l)
	}
	// a ring of profTicks: older ticks roll off
	for range profTicks + 10 {
		p.start()
		at = at.Add(3 * time.Millisecond)
		p.mark(secBots)
	}
	if r := p.report(); r.Ticks != profTicks || r.Tick.Mean != 3 {
		t.Fatalf("after roll-off %+v", r.Tick)
	}
	p.reset()
	if r := p.report(); r.Ticks != 0 || r.lines()[0] != "sv_prof: no ticks measured yet" {
		t.Fatalf("reset %+v", r)
	}
}

// The real tick, on a clock that moves 1 ms a read: every step of advance and
// of the snapshot is measured, and sv_prof (rcon) and server_profile (MCP) report it.
func TestTickProfServer(t *testing.T) {
	s := authServer(t)
	s.prof.now = stepClock(time.Millisecond)
	s.mu.Lock()
	s.addBot()
	s.mu.Unlock()
	for _, p := range s.players {
		p.ready = true
	}
	s.Step(31)
	s.mu.Lock()
	r := s.prof.report()
	s.mu.Unlock()
	if r.Ticks != 30 {
		t.Fatalf("ticks %d", r.Ticks)
	}
	for _, sec := range r.Sections { // one read per mark: 1 ms each, every tick (a snapshot every tick: snapEvery 1)
		if sec.Mean != 1 || sec.Max != 1 {
			t.Errorf("%s %+v", sec.Name, sec)
		}
	}
	s.mu.Lock()
	out, err := s.mcpRcon("sv_prof")
	s.mu.Unlock()
	if err != nil || len(out) != 2+int(nSecs) || !strings.Contains(out[0], "of the 10 ms budget") || !strings.Contains(strings.Join(out, "\n"), "cmds") {
		t.Fatalf("sv_prof %q %v", out, err)
	}
	c := mcpFor(t, s, false)
	m := c.call("server_profile", nil)
	if m["ticks"].(float64) != 30 || len(m["sections"].([]any)) != int(nSecs) {
		t.Fatalf("server_profile %v", m)
	}
	if m := c.call("server_profile", map[string]any{"reset": true}); m["ticks"].(float64) != 0 {
		t.Fatalf("server_profile reset %v", m)
	}
	s.mu.Lock()
	out, _ = s.mcpRcon("sv_prof reset")
	s.mu.Unlock()
	if len(out) != 1 || out[0] != "sv_prof: reset" {
		t.Fatalf("sv_prof reset %q", out)
	}
}
