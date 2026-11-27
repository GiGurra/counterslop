package game

import (
	"fmt"
	"time"
)

// SlowRead (test harness, sim/cases "slowRead"): the server reads some of a
// player's command packets late, as a loaded machine can (the socket or the
// reader starved for a while): one whose newest command's seq is a multiple
// of Every waits Ms of wall time before the server handles it. A lockstep
// run must play the same however late a command it was sent arrives (the
// harness used to give a sent command 2 s of wall time: "command N never
// arrived" under load).
type SlowRead struct {
	Every int `json:"every"`
	Ms    int `json:"ms"`
}

// SetSlowRead turns SlowRead on for the named player (Every 0: off).
func (s *Server) SetSlowRead(name string, r SlowRead) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.byName(name)
	if p == nil {
		return fmt.Errorf("no player %q", name)
	}
	if s.slowRead == nil {
		s.slowRead = map[int]SlowRead{}
	}
	if r.Every <= 0 {
		delete(s.slowRead, p.ID)
	} else {
		s.slowRead[p.ID] = r
	}
	return nil
}

// readLate holds the reader (not the server: no lock held) of a packet
// SlowRead names.
func (s *Server) readLate(p *Player, m inbound) {
	if m.T != "cmd" || len(m.Cmds) == 0 {
		return
	}
	s.mu.Lock()
	r, ok := s.slowRead[p.ID]
	s.mu.Unlock()
	if ok && m.Cmds[len(m.Cmds)-1].Seq%r.Every == 0 {
		time.Sleep(time.Duration(r.Ms) * time.Millisecond) // wall: the late read is the point (test harness only)
	}
}
