package game

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/content/navgen"
)

// Map loading (tk_73d9636d): a map change is a phase the server and every
// client go through together, never a silent server and a "connection lost".
//
// changeLevel puts the new map in play at once (geometry, spawns, a fresh
// match) and sends it to everyone, and the match holds in a load phase: the
// world is frozen (nobody moves or shoots, bots don't think, no round timer
// runs) while
//   - the server builds the map's nav graph (navgen: seconds on a race track)
//     in a goroutine, off s.mu: the ticks and snapshots go on;
//   - every client builds the map (geometry, light grid, shaders) and reports
//     how far it is ({t:"loading", id, step, pct}, resent until the
//     snapshots show it: the network may lose anything).
// The load state rides in every snapshot (round.load: the map, the server's
// step, each player's step), so a client that reconnects or joins mid-load
// shows the same loading screen. The match starts when the server is ready
// and every player is too, or sv_load_wait seconds after the server is ready
// (a stuck client joins late, never holds everyone hostage).
//
// Nav graphs are cached by the map's content (navCache): the same map loads
// at once the second time, and a map vote starts building every option's
// graph while people vote.

// Server load steps (loadState.Step).
const (
	LoadNav   = "nav"   // building the nav graph
	LoadReady = "ready" // done: waiting for the clients
)

// Client load steps (loadRow.Step; web/game/loading.js): "" (nothing heard
// yet), "geometry", "lights", "shaders", "ready".
const clientReady = "ready"

// loadState is the load phase, sent in every snapshot's round (round.load).
type loadState struct {
	ID   int    `json:"id" net:"cold"`   // this load (players report progress against it)
	Map  string `json:"map" net:"cold"`  // the map being loaded
	Step string `json:"step" net:"cold"` // the server's step: nav, ready
	// Secs: whole seconds the server has been at it (nav), or before the match
	// starts without whoever is still loading (ready).
	Secs int       `json:"secs" net:"cold"`
	Rows []loadRow `json:"pl" net:"cold"` // every human player's progress, by id

	began   time.Time              `net:"-"` // game clock
	readyAt time.Time              `net:"-"` // when the server's step went ready
	got     map[int]loadRow        `net:"-"` // the progress players reported
	done    chan []content.NavNode `net:"-"` // the nav build's result (nil: nothing to build)
	holdTo  int64                  `net:"-"` // lockstep: the nav step lasts until this tick (HoldNav)
	navKey  [32]byte               `net:"-"` // the geometry the nav is for (s.Map's, unless the editor changed it since)
}

// loadRow is one player's progress in a load.
type loadRow struct {
	ID   int    `json:"id" net:"cold"`
	Step string `json:"step" net:"cold"`
	Pct  int    `json:"pct,omitempty" net:"cold"`
}

// loadMsgIn is a client's progress report.
type loadMsgIn struct {
	ID   int    `json:"id"`
	Step string `json:"step"`
	Pct  int    `json:"pct"`
}

// ---- the nav cache ----

// navCache holds every nav graph built, by the map's content (not its name:
// a custom map edited on disk is a new one). Builds in flight are shared: a
// vote's prewarm and the winner's load wait on the same one.
var navCache = struct {
	sync.Mutex
	m     map[[32]byte]*navBuild
	order [][32]byte
}{m: map[[32]byte]*navBuild{}}

const navCacheMax = 32

type navBuild struct {
	done chan struct{}
	nav  []content.NavNode
}

// prewarmNavOn: a vote builds its options' graphs ahead (off in unit tests).
var prewarmNavOn = true

// navBuilder builds a map's graph (tests swap in a slow or counting one).
var navBuilder = navgen.Build

// navKeyOf is the cache key of m's geometry (its JSON without the nav).
func navKeyOf(m content.Map) [32]byte {
	m.Nav = nil
	b, _ := json.Marshal(m)
	return sha256.Sum256(b)
}

// navFor starts building m's nav graph unless it is cached or under way, and
// returns the build to wait on. m is copied: the build never shares memory
// with the map in play.
func navFor(key [32]byte, m content.Map) *navBuild {
	navCache.Lock()
	defer navCache.Unlock()
	if b := navCache.m[key]; b != nil {
		return b
	}
	b := &navBuild{done: make(chan struct{})}
	navCache.m[key] = b
	navCache.order = append(navCache.order, key)
	if len(navCache.order) > navCacheMax {
		delete(navCache.m, navCache.order[0])
		navCache.order = navCache.order[1:]
	}
	var cp content.Map
	src, _ := json.Marshal(m)
	_ = json.Unmarshal(src, &cp)
	cp.Nav = nil
	go func() {
		b.nav = navBuilder(&cp)
		close(b.done)
	}()
	return b
}

// needsNav: m gets a graph built from its geometry (autoNav's rule).
func (s *Server) needsNav(m *content.Map) bool {
	return len(m.Nav) == 0 || s.cvars["bot_autonav"] == "1"
}

// prewarmNav starts building the graphs of maps that may load next (a
// vote's options). Callers hold s.mu.
func (s *Server) prewarmNav(names []string) {
	if !prewarmNavOn || s.lockstep {
		return // unit tests and the lockstep harness: no builds nobody waits for
	}
	for _, n := range names {
		if n == s.Map.Name {
			continue
		}
		build := maps.Find(n)
		if build == nil {
			continue
		}
		if m := build(); s.needsNav(&m) {
			navFor(navKeyOf(m), m)
		}
	}
}

// ---- the load phase ----

// loading: a map load is under way (the world is frozen). Callers hold s.mu.
func (s *Server) loading() bool { return s.load != nil }

// beginLoad starts the load phase for the map just put in play (loadMap):
// its nav graph off the lock, when it needs one. Callers hold s.mu.
func (s *Server) beginLoad() {
	s.loadSeq++
	l := &loadState{ID: s.loadSeq, Map: s.Map.Name, Step: LoadReady, began: s.clock(), got: map[int]loadRow{}}
	if s.needsNav(&s.Map) {
		l.Step = LoadNav
		l.navKey = navKeyOf(s.Map)
		b := navFor(l.navKey, s.Map)
		l.done = make(chan []content.NavNode, 1)
		go func() { <-b.done; l.done <- b.nav }()
		l.holdTo = s.tick + s.navHold // tickLoad runs before the tick counts on: n ticks of nav, in on the next
		s.navHold = 0
	} else {
		l.readyAt = s.clock()
	}
	s.load = l
	slog.Info("map load", "map", l.Map, "id", l.ID, "step", l.Step)
}

// HoldNav makes the next map load's nav step last at least n ticks
// (lockstep scenarios: the loading screen shows "server: building nav").
func (s *Server) HoldNav(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.navHold = max(0, n)
}

// endLoad drops the load phase without starting anything (the map editor
// took the map, or another load replaced it). Callers hold s.mu.
func (s *Server) endLoad() { s.load = nil }

// tickLoad moves the load phase on: the nav graph in, then the start once
// everyone is ready or sv_load_wait is up. Callers hold s.mu.
func (s *Server) tickLoad(now time.Time) {
	l := s.load
	if l == nil {
		return
	}
	if l.Step == LoadNav {
		var nav []content.NavNode
		got := false
		if s.lockstep && s.tick >= l.holdTo {
			nav, got = <-l.done, true // lockstep: on this tick, however long the build takes (the same run every time)
		} else if !s.lockstep {
			select {
			case nav = <-l.done:
				got = true
			default:
			}
		}
		if !got {
			l.Secs = int(now.Sub(l.began).Seconds())
			s.loadRows()
			return
		}
		if len(nav) > 0 && s.Map.Name == l.Map && navKeyOf(s.Map) == l.navKey {
			s.Map.Nav, s.navAuto = nav, true
			s.aiAttDist, s.gridCache = nil, nil // routes cached on the old graph
		}
		l.Step, l.readyAt = LoadReady, now
		slog.Info("map load: server ready", "map", l.Map, "id", l.ID, "secs", now.Sub(l.began).Seconds())
	}
	wait := secs(s.cvarF("sv_load_wait", 20))
	s.loadRows()
	waiting := 0
	for _, r := range l.Rows {
		if r.Step != clientReady {
			waiting++
		}
	}
	left := wait - now.Sub(l.readyAt)
	l.Secs = int(max(0, left.Seconds()) + 0.999)
	if waiting > 0 && left > 0 {
		return
	}
	if waiting > 0 {
		s.announce(fmt.Sprintf("%s: starting without %d still loading", l.Map, waiting))
	}
	slog.Info("map load: start", "map", l.Map, "id", l.ID, "secs", now.Sub(l.began).Seconds(), "waited", waiting)
	s.load = nil
	s.restart() // fresh from the spawns: whatever the frozen seconds did
}

// loadRows is every connected human's progress, by id. Callers hold s.mu.
func (s *Server) loadRows() {
	l := s.load
	rows := []loadRow{} // a new slice: a sent snapshot may still hold the last one
	for _, p := range s.ordered() {
		if p.Bot || !p.ready || p.send == nil {
			continue
		}
		r := l.got[p.ID]
		r.ID = p.ID
		rows = append(rows, r)
	}
	l.Rows = rows
}

// loadProgress records a client's report. A report for another load (an old
// one, resent or reordered) is dropped; steps only go forward. Callers hold s.mu.
func (s *Server) loadProgress(p *Player, m loadMsgIn) {
	l := s.load
	if l == nil || m.ID != l.ID || p.Bot {
		return
	}
	step := m.Step
	if len(step) > 16 {
		step = step[:16]
	}
	pct := min(max(m.Pct, 0), 100)
	if old, ok := l.got[p.ID]; ok && old.Step == clientReady {
		return // ready stays ready (a late, reordered report)
	}
	l.got[p.ID] = loadRow{ID: p.ID, Step: step, Pct: pct}
}
