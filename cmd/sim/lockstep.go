package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/gigurra/counterslop/internal/game"
)

// Lockstep scenarios: the server's ticker is paused and the harness moves the
// game clock itself, so a scenario plays the same way however loaded the
// machine is (the flakes of real-time runs were all races against it).
//
//   - {"step": n}: n client frames of 1/60 s, each at its time on the pages'
//     clocks. Then the server runs the ticks that frame covers (100 Hz), each
//     one after the server has read everything the pages sent (a barrier:
//     what a fake link fault holds back stays in flight, as on a real link),
//     and each tick's messages reach the pages one frame after its tick time
//     (evenly spaced snapshots). At the end every client syncs, so it has
//     seen everything the server sent.
//   - {"wait": ms}: ms of game time on the server (ms/10 ticks, no client
//     input), then a sync; the same ms pass in real time for client timers.
//   - Snapshots: the server is paused (a snapshot renders one frame; no game
//     time passes however long the screenshot takes).
//   - Calls (call): the server stays paused unless the call asks for game
//     time. A helper that waits for the server (join, team, waitState,
//     shootBot, ...) parks on testapi's gate; the harness then steps one
//     tick, syncs every client and releases it, until the call returns. So
//     a call takes exactly the game time the state it waits for needs, on
//     any machine. (It used to tick on wall time while a call ran: game time
//     then depended on how slow the machine was; rounds.json's plant only
//     finished on a slow one.)
//   - Clicks and reloads: no game time passes.
//
// A scenario with "realtime": true (or -realtime) skips all of this.
type lockstep struct {
	on      bool
	srv     *game.Server
	pages   map[string]*page
	clients []string
	id      map[string]string // client -> "#<server id>"
	// On the pages' clocks (ms since lockstep began, as the harness moved
	// them): where they are, the last frame, the last server tick's messages.
	// Every tick's arrive 10 ms after the last one's, on every path, so the
	// remotes' adaptive delay sees a clean link (sim lockstep-link).
	pageNow, lastFrame, lastTick float64
	syncN                        int
}

func newLockstep(srv *game.Server, pages map[string]*page, clients []string, on bool) *lockstep {
	l := &lockstep{on: on, srv: srv, pages: pages, clients: clients, id: map[string]string{}}
	if on {
		srv.SetLockstep(true)
	}
	return l
}

func (l *lockstep) stop() {
	if l.on {
		l.srv.SetLockstep(false)
	}
}

// tickMs: one server tick of game time.
const tickMs = 1000.0 / game.SimHz

// callTimeout bounds the game time one call may take.
const callTimeout = 120 * game.SimHz

// call runs testapi's name(args) on client c. In lockstep the server ticks
// only when the call is parked on the gate (see the top of this file).
func (l *lockstep) call(c, name string, args []byte) error {
	p := l.pages[c]
	if !l.on {
		return p.eval(fmt.Sprintf("__cs[%q](...%s)", name, args), nil)
	}
	if err := p.eval(fmt.Sprintf("__cs.__lockstep(true) && __cs.__run(%q, %s)", name, args), nil); err != nil {
		return err
	}
	for ticks := 0; ; ticks++ {
		var r struct {
			Done bool    `json:"done"`
			Err  *string `json:"err"`
		}
		if err := p.eval("__cs.__poll()", &r); err != nil {
			return err
		}
		if r.Done {
			if r.Err != nil {
				return fmt.Errorf("%s", *r.Err)
			}
			return nil
		}
		if ticks >= callTimeout {
			return fmt.Errorf("%s still waiting after %d s of game time", name, callTimeout/game.SimHz)
		}
		if err := l.tick(); err != nil {
			return err
		}
		if err := p.eval("__cs.__ticked()", nil); err != nil {
			return err
		}
	}
}

func (l *lockstep) step(n int) error {
	if !l.on {
		for _, c := range l.clients {
			if err := l.pages[c].eval(fmt.Sprintf("(__cs.step(%d), true)", n), nil); err != nil {
				return err
			}
		}
		return nil
	}
	const frameMs = 1000.0 / 60
	type res struct {
		ID int `json:"id"`
	}
	marks := make([]int, len(l.clients))
	// on runs js on every page after it has taken in the marked messages,
	// then reads back each page's server ID (for the next marks).
	on := func(js string) error {
		r := make([]res, len(l.clients))
		if err := l.each(func(i int, c string) error {
			return l.pages[c].eval(fmt.Sprintf("__cs.syncTo(%d).then(() => %s)", marks[i], js), &r[i])
		}); err != nil {
			return err
		}
		for i, c := range l.clients {
			if r[i].ID > 0 {
				l.id[c] = fmt.Sprintf("#%d", r[i].ID)
			}
		}
		return nil
	}
	// The page's message barrier: the server has read everything it put on its
	// socket (commands a fake link fault holds back stay in flight).
	const barrier = "__cs.__barrier().then((id) => ({ id }))"
	// Ticks and frames in order of their times on the pages' clocks (a tick
	// first on a tie): a tick 10 ms after the last tick's (as on every path:
	// tick), what the pages sent meanwhile (a lagged command coming out) read
	// before it runs, its messages taken in at its time; a frame 1/60 s after
	// the last one (or now, after a wait or a call), its commands sent. After
	// the last frame, the ticks up to the next one (they run its commands).
	frame := max(l.lastFrame+frameMs, l.pageNow)
	for f := 0; f < n || l.lastTick+tickMs <= frame; {
		if t := l.lastTick + tickMs; t <= frame {
			if err := on(fmt.Sprintf("%s.then(() => %s)", advanceJS(t-l.pageNow), barrier)); err != nil {
				return err
			}
			l.pageNow, l.lastTick = max(l.pageNow, t), t
			l.srv.Step(1)
			marks = l.mark()
			continue
		}
		if err := on(fmt.Sprintf("%s.then(() => __cs.stepSeq(%v)).then(() => %s)", advanceJS(frame-l.pageNow), f == n-1, barrier)); err != nil {
			return err
		}
		l.pageNow, l.lastFrame = max(l.pageNow, frame), frame
		f++
		frame += frameMs
	}
	if err := l.syncAll(); err != nil {
		return err
	}
	return l.settle()
}

func (l *lockstep) wait(ms int) error {
	if !l.on {
		time.Sleep(time.Duration(ms) * time.Millisecond) // wall: realtime scenarios only
		return nil
	}
	// Tick by tick, each one's messages taken in 10 ms apart on the pages'
	// clocks (a burst at the end reads as a jittery link; client timers,
	// captions and fades see the same ms of game time).
	for range ms * game.SimHz / 1000 {
		if err := l.tick(); err != nil {
			return err
		}
	}
	return nil
}

// tick runs one server tick outside the frames (a call's, a wait's): the
// pages' clocks move its 10 ms on first, what they sent is read (barrier),
// and its messages are taken in at once. So each tick's messages arrive 10 ms
// after the last tick's, as in step: the same spacing on every path (a
// change of phase reads as a late snapshot to the remotes' adaptive delay).
func (l *lockstep) tick() error {
	t := max(l.lastTick+tickMs, l.pageNow)
	if err := l.advance(t - l.pageNow); err != nil {
		return err
	}
	l.lastTick = t
	if err := l.barrier(); err != nil { // what the page sent (a join, a teleport) is in before the tick
		return err
	}
	l.srv.Step(1)
	return l.syncAll()
}

// advanceJS moves a page's virtual clock (vclock.js) ms on: a promise.
func advanceJS(ms float64) string {
	return fmt.Sprintf("(window.__vclock ? __vclock.advance(%g) : Promise.resolve())", ms)
}

// advance moves every page's virtual clock ms on (none if <= 0), firing their timers.
func (l *lockstep) advance(ms float64) error {
	if ms <= 0 {
		return nil
	}
	l.pageNow += ms
	return l.each(func(_ int, c string) error {
		return l.pages[c].eval(advanceJS(ms)+".then(() => true)", nil)
	})
}

// settle lets every page run what a real frame would after the scripted one:
// an animation frame and a macrotask, so Preact effects (key handlers of a
// menu that just opened) are in place before the next step's input.
func (l *lockstep) settle() error {
	if !l.on {
		return nil
	}
	return l.each(func(_ int, c string) error {
		// On a virtual clock (vclock.js): what is due now, the clock stays put.
		// First the server takes in what the call sent (a teleport, a console
		// line: read on its own goroutine, so else it could land after the next
		// ticks on a loaded machine).
		if err := l.pages[c].eval("(window.__cs?.__barrier ? __cs.__barrier() : Promise.resolve()).then(() => window.__vclock ? __vclock.settle() : new Promise((r) => requestAnimationFrame(() => setTimeout(() => r(true)))))", nil); err != nil {
			return fmt.Errorf("settle: %w", err)
		}
		return nil
	})
}

// each runs f for every client at once (the pages are independent; one by one
// they cost the sum of their frame times under SwiftShader, at once about the
// slowest) and returns the first error in client order, "<client>: <err>".
func (l *lockstep) each(f func(i int, c string) error) error {
	errs := make([]error, len(l.clients))
	var wg sync.WaitGroup
	for i, c := range l.clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = f(i, c)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("%s: %w", l.clients[i], err)
		}
	}
	return nil
}

// barrier: the server has read everything each page put on its socket (a
// ping round trip; the server reads sockets on their own goroutines, so else
// a join or a console line could land a tick early or late, by machine
// speed). It also learns each page's server ID (for the sync markers).
func (l *lockstep) barrier() error {
	ids := make([]int, len(l.clients))
	if err := l.each(func(i int, c string) error {
		return l.pages[c].eval("window.__cs ? __cs.__barrier() : 0", &ids[i])
	}); err != nil {
		return fmt.Errorf("barrier: %w", err)
	}
	for i, c := range l.clients {
		if ids[i] > 0 {
			l.id[c] = fmt.Sprintf("#%d", ids[i])
		}
	}
	return nil
}

// syncAll: each connected client has taken in everything the server sent.
func (l *lockstep) syncAll() error {
	marks := l.mark()
	return l.each(func(i int, c string) error {
		if marks[i] == 0 {
			return nil
		}
		if err := l.pages[c].eval(fmt.Sprintf("__cs.caseSync(%d).then(() => true)", marks[i]), nil); err != nil {
			return fmt.Errorf("sync: %w", err)
		}
		return nil
	})
}

// mark asks the server to send each connected client a sync marker after
// everything sent so far (0: not connected, nothing to sync).
func (l *lockstep) mark() []int {
	marks := make([]int, len(l.clients))
	for i, c := range l.clients {
		id := l.id[c]
		if id == "" || l.srv.CaseState(id) == nil {
			continue
		}
		l.syncN++
		l.srv.Sync(id, l.syncN)
		marks[i] = l.syncN
	}
	return marks
}
