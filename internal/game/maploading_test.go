package game

import (
	"encoding/json"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// withNavBuilder swaps the nav builder (and empties the cache) for one test.
func withNavBuilder(t *testing.T, b func(*content.Map) []content.NavNode) {
	t.Helper()
	old := navBuilder
	navBuilder = b
	resetNavCache()
	t.Cleanup(func() { navBuilder = old; resetNavCache() })
}

func resetNavCache() {
	navCache.Lock()
	navCache.m, navCache.order = map[[32]byte]*navBuild{}, nil
	navCache.Unlock()
}

// fakeNav is a one-node graph: what the tests' builders return.
func fakeNav(*content.Map) []content.NavNode {
	return []content.NavNode{{Name: "n0", Pos: [3]float64{0, 0, 0}}}
}

// loadServer: a server with two humans in play (ready, with sockets) and a bot.
func loadServer(t *testing.T) (*Server, *Player, *Player) {
	t.Helper()
	s := authServer(t)
	s.lastTickAt = gameEpoch
	a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
	a.ready, a.Team = true, content.Vibe
	b.ready, b.Team = true, content.Artisan
	s.mu.Lock()
	s.addBot()
	s.mu.Unlock()
	return s, a, b
}

// tick runs one server tick (10 ms of game time after the last).
func tick(s *Server) { s.advance(s.lastTickAt.Add(time.Second / SimHz)) }

func report(s *Server, p *Player, step string, pct int) {
	id := 0
	if s.load != nil {
		id = s.load.ID
	}
	s.handle(p, inbound{T: "loading", LoadID: id, Step: step, Pct: pct})
}

// A slow nav build runs off the lock: the server keeps ticking (and sending
// snapshots) while it builds, the world frozen; the load state is in the
// snapshot's round; when the graph is in, the server's step is ready.
func TestLoadTicksWhileNavBuilds(t *testing.T) {
	gate := make(chan struct{})
	withNavBuilder(t, func(m *content.Map) []content.NavNode { <-gate; return fakeNav(m) })
	s, a, _ := loadServer(t)
	s.adminCommand(a, "changelevel", []string{"de_serverless"}) // no hand-made nav: built from its geometry
	if s.Map.Name != "de_serverless" || s.load == nil || s.load.Step != LoadNav {
		t.Fatalf("map %s, load %+v", s.Map.Name, s.load)
	}
	if len(ofType(a, "map")) != 1 {
		t.Fatal("the map was not sent at once")
	}
	start := s.tick
	from := a.Pos
	var cmds []Cmd
	for i := 1; i <= 100; i++ {
		cmds = append(cmds, Cmd{Seq: i, F: 1})
	}
	s.queueCmds(a, cmds)
	for range 100 { // 1 s of ticks while the build blocks
		tick(s)
	}
	if s.tick-start != 100 || s.load == nil || s.load.Step != LoadNav || !s.frozen() {
		t.Fatalf("ticks %d, load %+v, frozen %v", s.tick-start, s.load, s.frozen())
	}
	if d := a.Pos; d[0] != from[0] || d[2] != from[2] || a.lastCmd.Seq != 100 {
		t.Fatalf("moved while loading: %v -> %v", from, d)
	}
	if s.load.Secs != 1 {
		t.Fatalf("secs %d, want 1", s.load.Secs)
	}
	var r struct {
		Load *loadState `json:"load"`
	}
	if err := json.Unmarshal(s.roundJSON(s.clock()), &r); err != nil || r.Load == nil || r.Load.Map != "de_serverless" || r.Load.Step != LoadNav || len(r.Load.Rows) != 2 {
		t.Fatalf("snapshot round: %s", s.roundJSON(s.clock()))
	}
	close(gate)
	deadline := time.Now().Add(10 * time.Second) // hang guard: the build's goroutine
	for s.load != nil && s.load.Step == LoadNav {
		if time.Now().After(deadline) {
			t.Fatal("the nav step never ended")
		}
		time.Sleep(time.Millisecond)
		tick(s)
	}
	if s.load == nil || s.load.Step != LoadReady || len(s.Map.Nav) != 1 || !s.navAuto {
		t.Fatalf("load %+v, nav %d", s.load, len(s.Map.Nav))
	}
}

// The match starts once the server and every human are ready (bots don't
// count), not before; whatever came before, everyone starts from the spawns.
func TestLoadStartsWhenEveryoneIsReady(t *testing.T) {
	withNavBuilder(t, fakeNav)
	s, a, b := loadServer(t)
	s.adminCommand(a, "changelevel", []string{"de_stack"}) // a hand-made nav: nothing to build
	if s.load == nil || s.load.Step != LoadReady {
		t.Fatalf("load %+v", s.load)
	}
	tick(s)
	report(s, a, "ready", 100)
	tick(s)
	if s.load == nil {
		t.Fatal("started without b")
	}
	report(s, b, "lights", 40)
	tick(s)
	if s.load == nil || s.load.Rows[1] != (loadRow{ID: b.ID, Step: "lights", Pct: 40}) {
		t.Fatalf("rows %+v", s.load.Rows)
	}
	report(s, b, "ready", 100)
	tick(s)
	if s.load != nil {
		t.Fatalf("still loading: %+v", s.load)
	}
	if s.frozen() {
		t.Fatal("frozen after the load")
	}
}

// A client that never finishes doesn't hold the match: sv_load_wait after the
// server is ready, it starts without them (and says so).
func TestLoadWaitTimesOut(t *testing.T) {
	withNavBuilder(t, fakeNav)
	s, a, _ := loadServer(t)
	s.cvars["sv_load_wait"] = "2"
	s.adminCommand(a, "changelevel", []string{"de_stack"})
	report(s, a, "ready", 100)
	msgs(a)
	for i := 0; i < 2*SimHz-1; i++ {
		tick(s)
		if s.load == nil {
			t.Fatalf("started after %d ticks", i+1)
		}
	}
	if s.load.Secs != 1 {
		t.Fatalf("secs left %d", s.load.Secs)
	}
	tick(s)
	tick(s)
	if s.load != nil {
		t.Fatal("still waiting after sv_load_wait")
	}
	if !hasText(texts(a), "starting without 1 still loading") {
		t.Fatalf("no notice: %v", texts(a))
	}
}

// Reports for another load (old, resent, reordered) are dropped; a duplicate
// changes nothing; ready stays ready; the step and percent are clamped.
func TestLoadReportsAreIdempotent(t *testing.T) {
	withNavBuilder(t, fakeNav)
	s, a, b := loadServer(t)
	s.adminCommand(a, "changelevel", []string{"de_stack"})
	id := s.load.ID
	s.handle(a, inbound{T: "loading", LoadID: id - 1, Step: "ready", Pct: 100}) // the last load's
	tick(s)
	if s.load.Rows[0].Step != "" {
		t.Fatalf("an old load's report counted: %+v", s.load.Rows[0])
	}
	report(s, a, "ready", 100)
	report(s, a, "ready", 100)
	report(s, a, "lights", 10) // reordered behind the ready
	report(s, b, "shaders-and-a-very-long-step", 900)
	tick(s)
	if s.load.Rows[0].Step != "ready" || s.load.Rows[1].Pct != 100 || len(s.load.Rows[1].Step) > 16 {
		t.Fatalf("rows %+v", s.load.Rows)
	}
	s.leave(b) // drops out mid-load: nobody waits for them
	tick(s)
	if s.load != nil {
		t.Fatalf("waiting for a player who left: %+v", s.load)
	}
}

// A player who joins mid-load is waited for too, and sees the load in the
// snapshot; a new load (another changelevel) starts over with a new id.
func TestLoadLateJoinerAndReload(t *testing.T) {
	withNavBuilder(t, fakeNav)
	s, a, b := loadServer(t)
	s.adminCommand(a, "changelevel", []string{"de_stack"})
	first := s.load.ID
	report(s, a, "ready", 100)
	report(s, b, "ready", 100)
	c := s.join("3.3.3.3")
	c.ready = true
	tick(s)
	if s.load == nil || len(s.load.Rows) != 3 {
		t.Fatalf("late joiner not waited for: %+v", s.load)
	}
	s.adminCommand(a, "changelevel", []string{"de_slop"})
	if s.load == nil || s.load.ID == first || s.load.Map != "de_slop" {
		t.Fatalf("second load %+v", s.load)
	}
	tick(s)
	if s.load.Rows[0].Step != "" {
		t.Fatal("the first load's ready carried over")
	}
}

// The harness's world reset and the map editor load a map with no load phase.
func TestNoLoadPhaseForResetAndEditor(t *testing.T) {
	withNavBuilder(t, fakeNav)
	s, a, _ := loadServer(t)
	s.adminCommand(a, "changelevel", []string{"de_stack"})
	if s.load == nil {
		t.Fatal("no load")
	}
	if err := s.ResetWorldOn("de_slop"); err != nil || s.load != nil {
		t.Fatalf("reset: %v, load %+v", err, s.load)
	}
}

// A nav graph is built once per map content: loading the same map again
// takes the cached one, at once.
func TestNavCache(t *testing.T) {
	var n atomic.Int32
	withNavBuilder(t, func(m *content.Map) []content.NavNode { n.Add(1); return fakeNav(m) })
	s, a, _ := loadServer(t)
	s.lockstep = true // the nav lands on the next tick, deterministically
	for range 2 {
		s.adminCommand(a, "changelevel", []string{"de_serverless"})
		tick(s)
		if s.load == nil || s.load.Step != LoadReady {
			t.Fatalf("load %+v", s.load)
		}
		s.adminCommand(a, "changelevel", []string{"de_slop"})
	}
	if n.Load() != 1 {
		t.Fatalf("built %d times", n.Load())
	}
}

// HoldNav (lockstep scenarios) keeps the server's nav step on for n ticks.
func TestHoldNav(t *testing.T) {
	withNavBuilder(t, fakeNav)
	s, a, _ := loadServer(t)
	s.lockstep = true
	s.HoldNav(5)
	s.adminCommand(a, "changelevel", []string{"de_serverless"})
	for i := range 5 {
		tick(s)
		if s.load.Step != LoadNav {
			t.Fatalf("tick %d: %s", i, s.load.Step)
		}
	}
	tick(s)
	if s.load.Step != LoadReady {
		t.Fatal("still building after the hold")
	}
}

// A vote starts building its options' graphs while people vote.
func TestVotePrewarmsNav(t *testing.T) {
	var built sync32
	withNavBuilder(t, func(m *content.Map) []content.NavNode { built.add(m.Name); return fakeNav(m) })
	prewarmNavOn = true
	t.Cleanup(func() { prewarmNavOn = false })
	s, _, _ := loadServer(t)
	s.Map = maps.Find("rc_ci_pipeline")()
	s.StartMapVote(time.Second)
	want := 0
	for _, o := range s.vote.options {
		if m := maps.Find(o)(); o != s.Map.Name && s.needsNav(&m) {
			want++
		}
	}
	if want == 0 {
		t.Fatalf("no option to prewarm: %v", s.vote.options)
	}
	deadline := time.Now().Add(10 * time.Second) // hang guard: the builds' goroutines
	for built.len() < want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if built.len() != want {
		t.Fatalf("prewarmed %v of %v", built.names(), s.vote.options)
	}
}

// sync32: the names the builder was called with, from its goroutines.
type sync32 struct {
	mu  sync.Mutex
	set []string
}

func (s *sync32) add(n string) { s.mu.Lock(); s.set = append(s.set, n); s.mu.Unlock() }
func (s *sync32) len() int     { return len(s.names()) }
func (s *sync32) names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.set)
}
