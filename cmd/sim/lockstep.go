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
//   - {"step": n}: n client frames of 1/60 s. After each frame every client's
//     new commands must have reached the server (WaitCommand), then the
//     server runs the ticks that frame covers (100 Hz). At the end every
//     client syncs, so it has seen everything the server sent.
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
	seq     map[string]int    // last command seq seen per client
	id      map[string]string // client -> "#<server id>"
	frames  int               // client frames stepped
	ticks   int               // server ticks stepped (for frames)
	syncN   int
}

func newLockstep(srv *game.Server, pages map[string]*page, clients []string, on bool) *lockstep {
	l := &lockstep{on: on, srv: srv, pages: pages, clients: clients, seq: map[string]int{}, id: map[string]string{}}
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
		if err := l.barrier(); err != nil { // what the call sent (a join, a teleport) is in before the tick
			return err
		}
		l.srv.Step(1)
		if err := l.syncAll(); err != nil {
			return err
		}
		if err := l.advance(1000.0 / game.SimHz); err != nil {
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
	type seqID struct {
		Seq int `json:"seq"`
		ID  int `json:"id"`
	}
	res := make([]seqID, len(l.clients))
	marks := make([]int, len(l.clients))
	for f := 0; f < n; f++ {
		// Every client's frame at once (each page renders in its own process),
		// then their commands in client order. Each frame first takes in all
		// the server sent for the last one (sync marker), then moves its clock:
		// a snapshot arrives at the same frame on any machine.
		if err := l.each(func(i int, c string) error {
			return l.pages[c].eval(fmt.Sprintf("__cs.syncTo(%d).then(() => %s).then(() => __cs.stepSeq(%v))", marks[i], advanceJS(1000.0/60), f == n-1), &res[i])
		}); err != nil {
			return err
		}
		for i, c := range l.clients {
			r := res[i]
			if r.ID > 0 {
				l.id[c] = fmt.Sprintf("#%d", r.ID)
			}
			if r.Seq > l.seq[c] && l.id[c] != "" {
				if err := l.srv.WaitCommand(l.id[c], r.Seq, 3*time.Second); err != nil && l.srv.CaseState(l.id[c]) != nil {
					return fmt.Errorf("%s: %w (a scenario that drops commands on purpose needs \"realtime\": true)", c, err)
				}
			}
			l.seq[c] = r.Seq
		}
		l.frames++
		due := l.frames * game.SimHz / 60
		l.srv.Step(due - l.ticks)
		l.ticks = due
		marks = l.mark()
	}
	if err := l.syncAll(); err != nil {
		return err
	}
	return l.settle()
}

func (l *lockstep) wait(ms int) error {
	if !l.on {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return nil
	}
	for i := 0; i < ms*game.SimHz/1000; i++ {
		l.srv.Step(1)
		if i%10 == 9 {
			time.Sleep(time.Millisecond) // let the sockets carry what the ticks sent
		}
	}
	if err := l.syncAll(); err != nil {
		return err
	}
	return l.advance(float64(ms)) // client-side timers (captions, fades) see the same ms of game time
}

// advanceJS moves a page's virtual clock (vclock.js) ms on: a promise.
func advanceJS(ms float64) string {
	return fmt.Sprintf("(window.__vclock ? __vclock.advance(%g) : Promise.resolve())", ms)
}

// advance moves every page's virtual clock ms on, firing their timers.
func (l *lockstep) advance(ms float64) error {
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
