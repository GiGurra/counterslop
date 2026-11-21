package game

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/gigurra/counterslop/internal/content"
)

// AFK handling and team autobalance, both between rounds (CS: mp_autokick's
// idle check, mp_autoteambalance).
//
//   - A human who played a whole round (spawned at its start) without any input
//     (moving, looking, a button, a slot, buying, chat or radio) is idle that
//     round; after mp_afk_rounds idle rounds in a row they go to spectators at
//     the next round start.
//   - At a round start, while one side has 2+ more players than the other, the
//     most recently joined player on the bigger side (playing bots first) moves
//     to the smaller side, keeping their money and weapons.
//
// Both are "auto" by default: on, except on test-harness servers (scenarios and
// cases set up their teams and stand still on purpose), where a case turns
// them on with the cvar. Teams are in every snapshot; the notices are optional.

const afkRoundsDefault = 2

// afkRounds is how many idle rounds move a player to spectators (0: never).
func (s *Server) afkRounds() int {
	switch v := s.cvars["mp_afk_rounds"]; v {
	case "", "auto":
		if s.Test {
			return 0
		}
		return afkRoundsDefault
	default:
		n, _ := strconv.Atoi(v)
		return max(n, 0)
	}
}

func (s *Server) autoBalance() bool {
	v := s.cvars["mp_autoteambalance"]
	return v == "1" || (v != "0" && !s.Test)
}

// active notes input from p this round. Callers hold s.mu.
func (s *Server) active(p *Player) { p.activeRound = s.round.Number }

// noteInput: a command with any input makes p active. Callers hold s.mu.
func (s *Server) noteInput(p *Player, c Cmd) {
	look := [2]float64{c.Yaw, c.Pitch}
	if c.F != 0 || c.R != 0 || c.B != 0 || c.W != "" || look != p.lastLook {
		s.active(p)
	}
	p.lastLook = look
}

// countIdle: at round end, everyone who played the round from its start
// without input gets an idle round; anyone else starts over. Callers hold s.mu.
func (s *Server) countIdle() {
	for _, p := range s.players {
		switch {
		case p.Bot || p.ai != nil:
		case p.activeRound == s.round.Number || p.Team == content.Spectator:
			p.idleRounds = 0
		case p.playedFrom == s.round.Number:
			p.idleRounds++
		}
	}
}

// moveAFK sends players idle for mp_afk_rounds rounds to spectators. Callers hold s.mu.
func (s *Server) moveAFK() {
	n := s.afkRounds()
	if n == 0 {
		return
	}
	for _, p := range s.sortedPlayers() {
		if p.Bot || p.ai != nil || p.Team == content.Spectator || p.idleRounds < n {
			continue
		}
		p.idleRounds = 0
		s.dropBomb(p)
		p.Team = content.Spectator
		s.respawn(p)
		s.noticeAll(p.Name + " is AFK: moved to spectators (idle " + strconv.Itoa(n) + " rounds)")
		s.notice(p, "You were moved to spectators for being idle: open the team menu to play again.")
	}
}

// balanceTeams moves players from the bigger side until the sides are within
// one of each other. Callers hold s.mu.
func (s *Server) balanceTeams() {
	if !s.autoBalance() {
		return
	}
	a, b := content.Teams[0], content.Teams[1]
	for range len(s.players) {
		n := map[string]int{}
		for _, p := range s.players {
			if !p.Bot {
				n[p.Team]++
			}
		}
		big, small := a, b
		if n[b] > n[a] {
			big, small = b, a
		}
		if n[big]-n[small] < 2 {
			return
		}
		var cands []*Player
		for _, p := range s.players {
			if !p.Bot && p.Team == big {
				cands = append(cands, p)
			}
		}
		// Playing bots first, then the most recently joined (ties: the higher id).
		m := slices.MaxFunc(cands, func(x, y *Player) int {
			if c := cmp.Compare(boolInt(x.ai != nil), boolInt(y.ai != nil)); c != 0 {
				return c
			}
			if c := x.joinedAt.Compare(y.joinedAt); c != 0 {
				return c
			}
			return cmp.Compare(x.ID, y.ID)
		})
		m.Team = small
		s.noticeAll(m.Name + " was moved to " + teamName(small) + " to balance the teams")
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func teamName(t string) string {
	return map[string]string{content.Vibe: "the Vibe Coders", content.Artisan: "the Artisans"}[t]
}

func (s *Server) noticeAll(text string) {
	for _, p := range s.players {
		s.notice(p, text)
	}
}

// sortedPlayers is s.players by id (deterministic order). Callers hold s.mu.
func (s *Server) sortedPlayers() []*Player {
	out := make([]*Player, 0, len(s.players))
	for _, p := range s.players {
		out = append(out, p)
	}
	slices.SortFunc(out, func(x, y *Player) int { return x.ID - y.ID })
	return out
}
