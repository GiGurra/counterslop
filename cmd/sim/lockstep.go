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
	type seqID struct {
		Seq int `json:"seq"`
		ID  int `json:"id"`
	}
	res := make([]seqID, len(l.clients))
	for f := 0; f < n; f++ {
		// Every client's frame at once (each page renders in its own process),
		// then their commands in client order.
		if err := l.each(func(i int, c string) error {
			return l.pages[c].eval(fmt.Sprintf("__cs.stepSeq(%v)", f == n-1), &res[i])
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
	return l.each(func(_ int, c string) error {
		if err := l.pages[c].eval("new Promise((r) => requestAnimationFrame(() => setTimeout(() => r(true))))", nil); err != nil {
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

// syncAll: each connected client has taken in everything the server sent.
func (l *lockstep) syncAll() error {
	marks := make([]int, len(l.clients)) // 0: not connected, nothing to sync
	for i, c := range l.clients {
		id := l.id[c]
		if id == "" || l.srv.CaseState(id) == nil {
			continue
		}
		l.syncN++
		l.srv.Sync(id, l.syncN)
		marks[i] = l.syncN
	}
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
