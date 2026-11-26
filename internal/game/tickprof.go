package game

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"time"
)

// The tick profiler: where the server's 100 Hz tick goes, step by step
// (sv_prof, the MCP's server_profile; docs/profiling.md). Every tick is timed
// in contiguous sections (each mark closes the one before), over the last
// profTicks ticks: mean, p95, p99, max per section and for the whole tick,
// against the 10 ms budget. A report of how long work took, never gameplay:
// it reads profClock (the wall clock) unless a test injects its own clock.

const profTicks = 200 // 2 s of ticks

// The sections, in tick order. The snapshot ones run after advance, in sendSnapshot.
const (
	secVote = iota
	secNades
	secEditors
	secAI
	secPuppets
	secCmds
	secBoats
	secRound
	secBots
	secItems
	secHistory
	secSnapshot // build + encode + queue every player's snapshot (the net codec)
	secTV       // the replay recorder's frame
	secMCP      // the MCP's observer snapshot
	nSecs
)

var secNames = [nSecs]string{"vote", "nades", "editors", "ai", "puppets", "cmds", "boats", "round", "bots", "items", "history", "snapshot", "tv", "mcp"}

type tickProf struct {
	now  func() time.Time
	cur  [nSecs]time.Duration
	ring [profTicks][nSecs]float32 // ms
	i, n int
	last time.Time
	open bool
}

// profClock is the tick profiler's clock: wall time (TestWallNowOnlyWhereAllowed).
func profClock() time.Time { return wallNow() }

func (t *tickProf) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return profClock()
}

// start begins a tick (advance): the tick before it, snapshot included, is done.
func (t *tickProf) start() {
	t.flush()
	t.open = true
	t.last = t.clock()
}

// resume: the clock runs from here again (the snapshot, after advance released the lock).
func (t *tickProf) resume() {
	if t.open {
		t.last = t.clock()
	}
}

// mark: the time since the last mark (or start) was section sec's.
func (t *tickProf) mark(sec int) {
	if !t.open {
		return
	}
	n := t.clock()
	t.cur[sec] += n.Sub(t.last)
	t.last = n
}

func (t *tickProf) flush() {
	if !t.open {
		return
	}
	for k, d := range t.cur {
		t.ring[t.i][k] = float32(d.Seconds() * 1000)
	}
	t.cur = [nSecs]time.Duration{}
	t.i = (t.i + 1) % profTicks
	t.n = min(profTicks, t.n+1)
	t.open = false
}

func (t *tickProf) reset() { t.n, t.i, t.open, t.cur = 0, 0, false, [nSecs]time.Duration{} }

// profStat: ms over the window (nearest-rank percentiles).
type profStat struct {
	Name string  `json:"name"`
	Mean float64 `json:"mean"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
	Max  float64 `json:"max"`
}

// ProfReport is sv_prof's table: the whole tick and each section, largest first.
type ProfReport struct {
	Ticks    int        `json:"ticks"`
	BudgetMs float64    `json:"budgetMs"` // one tick's time (1000 / SimHz)
	Tick     profStat   `json:"tick"`
	Sections []profStat `json:"sections"`
}

func statOf(name string, v []float64) profStat {
	if len(v) == 0 {
		return profStat{Name: name}
	}
	s := slices.Clone(v)
	slices.Sort(s)
	sum := 0.0
	for _, x := range s {
		sum += x
	}
	rank := func(q float64) float64 {
		return s[min(len(s)-1, max(0, int(math.Ceil(q*float64(len(s))))-1))]
	}
	r3 := func(x float64) float64 { return math.Round(x*1000) / 1000 }
	return profStat{Name: name, Mean: r3(sum / float64(len(s))), P95: r3(rank(0.95)), P99: r3(rank(0.99)), Max: r3(s[len(s)-1])}
}

func (t *tickProf) report() ProfReport {
	r := ProfReport{Ticks: t.n, BudgetMs: 1000.0 / SimHz}
	tot := make([]float64, t.n)
	for k := range nSecs {
		v := make([]float64, t.n)
		for j := range t.n {
			x := float64(t.ring[(t.i-1-j+profTicks)%profTicks][k])
			v[j] = x
			tot[j] += x
		}
		r.Sections = append(r.Sections, statOf(secNames[k], v))
	}
	r.Tick = statOf("tick", tot)
	sort.SliceStable(r.Sections, func(a, b int) bool { return r.Sections[a].Mean > r.Sections[b].Mean })
	return r
}

// lines: the report as sv_prof prints it.
func (r ProfReport) lines() []string {
	if r.Ticks == 0 {
		return []string{"sv_prof: no ticks measured yet"}
	}
	out := []string{fmt.Sprintf("tick %.3f ms mean (p95 %.3f, p99 %.3f, max %.3f) of the %.0f ms budget: %.1f%% · last %d ticks",
		r.Tick.Mean, r.Tick.P95, r.Tick.P99, r.Tick.Max, r.BudgetMs, 100*r.Tick.Mean/r.BudgetMs, r.Ticks),
		fmt.Sprintf("  %-10s %8s %8s %8s %8s", "section", "mean", "p95", "p99", "max")}
	for _, s := range r.Sections {
		out = append(out, fmt.Sprintf("  %-10s %8.3f %8.3f %8.3f %8.3f", s.Name, s.Mean, s.P95, s.P99, s.Max))
	}
	return out
}
