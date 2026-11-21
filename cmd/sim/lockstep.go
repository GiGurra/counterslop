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
//   - Anything else (calls, clicks, reloads, snapshots): the server is
//     "assisted": a goroutine steps it one tick per 10 ms of wall time, so
//     helpers that wait for a reply (join, team, shootBot, waitState) get
//     one. Game time stays continuous: Step advances the game clock, not
//     the wall clock.
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

	mu        sync.Mutex
	assisting bool
	quit      chan struct{}
	done      chan struct{}
}

func newLockstep(srv *game.Server, pages map[string]*page, clients []string, on bool) *lockstep {
	l := &lockstep{on: on, srv: srv, pages: pages, clients: clients, seq: map[string]int{}, id: map[string]string{}}
	if on {
		srv.SetLockstep(true)
	}
	return l
}

func (l *lockstep) stop() {
	l.pause()
	if l.on {
		l.srv.SetLockstep(false)
	}
}

// assist starts stepping the server on wall time (no-op if already).
func (l *lockstep) assist() {
	if !l.on || l.assisting {
		return
	}
	l.assisting = true
	l.quit, l.done = make(chan struct{}), make(chan struct{})
	go func(quit, done chan struct{}) {
		defer close(done)
		t := time.NewTicker(time.Second / game.SimHz)
		defer t.Stop()
		for {
			select {
			case <-quit:
				return
			case <-t.C:
				l.srv.Step(1)
			}
		}
	}(l.quit, l.done)
}

// pause stops the assist goroutine.
func (l *lockstep) pause() {
	if !l.assisting {
		return
	}
	close(l.quit)
	<-l.done
	l.assisting = false
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
	for f := 0; f < n; f++ {
		for _, c := range l.clients {
			var r struct {
				Seq int `json:"seq"`
				ID  int `json:"id"`
			}
			if err := l.pages[c].eval(fmt.Sprintf("__cs.stepSeq(%v)", f == n-1), &r); err != nil {
				return fmt.Errorf("%s: %w", c, err)
			}
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
	t0 := time.Now()
	for i := 0; i < ms*game.SimHz/1000; i++ {
		l.srv.Step(1)
		if i%10 == 9 {
			time.Sleep(time.Millisecond) // let the sockets carry what the ticks sent
		}
	}
	if err := l.syncAll(); err != nil {
		return err
	}
	if left := time.Duration(ms)*time.Millisecond - time.Since(t0); left > 0 {
		time.Sleep(left) // client-side timers (captions, fades) see the same ms
	}
	return nil
}

// settle lets every page run what a real frame would after the scripted one:
// an animation frame and a macrotask, so Preact effects (key handlers of a
// menu that just opened) are in place before the next step's input.
func (l *lockstep) settle() error {
	if !l.on {
		return nil
	}
	for _, c := range l.clients {
		if err := l.pages[c].eval("new Promise((r) => requestAnimationFrame(() => setTimeout(() => r(true))))", nil); err != nil {
			return fmt.Errorf("%s settle: %w", c, err)
		}
	}
	return nil
}

// syncAll: each connected client has taken in everything the server sent.
func (l *lockstep) syncAll() error {
	for _, c := range l.clients {
		id := l.id[c]
		if id == "" || l.srv.CaseState(id) == nil {
			continue
		}
		l.syncN++
		l.srv.Sync(id, l.syncN)
		if err := l.pages[c].eval(fmt.Sprintf("__cs.caseSync(%d).then(() => true)", l.syncN), nil); err != nil {
			return fmt.Errorf("%s sync: %w", c, err)
		}
	}
	return nil
}
