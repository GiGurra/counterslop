package game

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gigurra/counterslop/internal/content"
)

// Chat and radio. {t:"say", text, team} from a client goes out as
// {t:"chat", id, name, team, text, tm (team only), dead}; {t:"radio", id}
// goes to the sender's team as {t:"radio", from, name, id, p} (p: where the
// sender stands, for a radar ping; the client speaks the line). CS rules:
// the dead (and spectators) talk only to the dead and spectators while a
// round is on. Both are events (losable): nothing gameplay depends on them.

const (
	chatMax      = 120
	chatBurst    = 5   // messages in a burst...
	chatRefillPS = 0.5 // ...then one every 2 s
)

var radioID = regexp.MustCompile(`^[a-z_]{1,24}$`)

// sanitizeChat: printable text only, whitespace collapsed, at most chatMax runes.
func sanitizeChat(s string) string {
	var b strings.Builder
	n := 0
	space := false
	for _, r := range s {
		if n >= chatMax {
			break
		}
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if !unicode.IsPrint(r) {
			continue
		}
		if space && b.Len() > 0 {
			b.WriteRune(' ')
			n++
		}
		space = false
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// chatAllowed: a token bucket per player (anti-spam). Callers hold s.mu.
func (s *Server) chatAllowed(p *Player, now time.Time) bool {
	if p.chatAt.IsZero() {
		p.chatTokens = chatBurst
	} else {
		p.chatTokens = min(chatBurst, p.chatTokens+now.Sub(p.chatAt).Seconds()*chatRefillPS)
	}
	p.chatAt = now
	if p.chatTokens < 1 {
		return false
	}
	p.chatTokens--
	return true
}

// ghost: dead or spectating while a round is on (CS: talks only to ghosts).
func (s *Server) ghost(p *Player) bool {
	return s.roundsOn() && (p.Dead || p.Team == content.Spectator || p.Team == "")
}

func (s *Server) hears(from, to *Player, teamOnly bool) bool {
	if to.Bot || to.ai != nil {
		return false
	}
	if teamOnly && to.Team != from.Team {
		return false
	}
	return !s.ghost(from) || s.ghost(to)
}

// say handles {t:"say"}. Callers hold s.mu.
func (s *Server) say(p *Player, text string, teamOnly bool) {
	text = sanitizeChat(text)
	if text == "" {
		return
	}
	if !s.chatAllowed(p, time.Now()) {
		s.print(p, "chat: slow down")
		return
	}
	b, _ := json.Marshal(map[string]any{"t": "chat", "id": p.ID, "name": p.Name, "team": p.Team, "text": text, "tm": teamOnly, "dead": s.ghost(p)})
	for _, o := range s.players {
		if s.hears(p, o, teamOnly) {
			trySend(o, b)
		}
	}
}

// radio handles {t:"radio", id}: a callout to the living team. Callers hold s.mu.
func (s *Server) radio(p *Player, id string) {
	if !radioID.MatchString(id) || p.Team == content.Spectator || p.Team == "" || s.ghost(p) {
		return
	}
	if !s.chatAllowed(p, time.Now()) {
		return
	}
	b, _ := json.Marshal(map[string]any{"t": "radio", "from": p.ID, "name": p.Name, "id": id, "p": p.Pos})
	for _, o := range s.players {
		if s.hears(p, o, true) {
			trySend(o, b)
		}
	}
}
