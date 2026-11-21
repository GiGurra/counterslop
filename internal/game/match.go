package game

import (
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// Match structure around the rounds (CS2 competitive): mp_maxrounds rounds
// (24 = MR12: first to 13), sides swap at halftime (the score follows the
// players), a tie goes to overtime (mp_overtime_maxrounds per overtime, first
// to half+1 of it, sides swap at each overtime half; repeat until decided) or,
// with mp_overtime_enable 0, ends as a draw. Warmup (mp_warmup_time, admin
// mp_warmup_end) runs once both sides have a player; the match then starts on
// fresh money and K/D. After the match, the end screen shows for
// mp_match_restart_delay while the map vote runs (StartMapVote); its winner
// loads a new map, which starts a new match from warmup.

const (
	PhaseHalftime  = "halftime"
	PhaseMatchOver = "matchover"
	MatchDraw      = "draw"
)

// roundResult is one played round in the history strip.
type roundResult struct {
	Winner string `json:"w"`   // side that won (vibe / artisan) at the time
	Reason string `json:"why"` // elimination, exploded, defused, time
}

// Score points (CS): kill +2, assist +1, plant +2, defuse +2, team kill -2, suicide -1.
const (
	scoreKill      = 2
	scoreAssist    = 1
	scoreObjective = 2
	scoreTeamKill  = -2
	scoreSuicide   = -1
)

func (s *Server) maxRounds() int {
	return max(2, int(s.cvarF("mp_maxrounds", 24)))
}

func (s *Server) otRounds() int {
	return max(2, int(s.cvarF("mp_overtime_maxrounds", 6)))
}

// matchResult decides the match after `played` rounds with score sc:
// the winner ("vibe", "artisan" or MatchDraw), or "" to play on.
func matchResult(played int, sc map[string]int, reg, ot int, otOn bool) string {
	v, a := sc[content.Vibe], sc[content.Artisan]
	lead := content.Vibe
	if a > v {
		lead = content.Artisan
	}
	best := max(v, a)
	half := reg / 2
	if played <= reg {
		if best >= half+1 {
			return lead
		}
		if played == reg && v == a {
			if !otOn {
				return MatchDraw
			}
		}
		return ""
	}
	// Overtime k (0-based) starts with both sides on base points.
	k := (played - reg - 1) / ot
	base := half + k*(ot/2)
	if best-base >= ot/2+1 {
		return lead
	}
	return ""
}

// halftimeAfter: sides swap after `played` rounds (regulation half, each overtime half).
func halftimeAfter(played, reg, ot int) bool {
	if played == reg/2 {
		return true
	}
	return played > reg && (played-reg)%ot == ot/2
}

// overtimeOf is the overtime number round n (1-based) is in, 0 in regulation.
func overtimeOf(n, reg, ot int) int {
	if n <= reg {
		return 0
	}
	return (n-reg-1)/ot + 1
}

// afterRound runs when the round-end delay is over: halftime, match end or
// the next round. Callers hold s.mu.
func (s *Server) afterRound(now time.Time) {
	r := &s.round
	played := len(r.History)
	reg, ot := s.maxRounds(), s.otRounds()
	if w := matchResult(played, r.Score, reg, ot, s.cvars["mp_overtime_enable"] != "0"); w != "" {
		s.endMatch(now, w)
		return
	}
	if halftimeAfter(played, reg, ot) {
		r.Phase, r.endsAt = PhaseHalftime, now.Add(secs(s.cvarF("mp_halftime_duration", 10)))
		s.sendAll(roundEvent{T: "round", Ev: "halftime", Score: r.Score, Number: r.Number})
		return
	}
	s.startRound(now)
}

// endHalftime swaps the sides and starts the next half on pistol-round money.
func (s *Server) endHalftime(now time.Time) {
	s.swapSides()
	s.resetMoney()
	s.round.pistol = true
	s.startRound(now)
}

// swapSides moves every player to the other side; the score follows them.
func (s *Server) swapSides() {
	other := map[string]string{content.Vibe: content.Artisan, content.Artisan: content.Vibe}
	for _, p := range s.players {
		if o, ok := other[p.Team]; ok && !p.Bot {
			p.Team = o
		}
	}
	sc := s.round.Score
	sc[content.Vibe], sc[content.Artisan] = sc[content.Artisan], sc[content.Vibe]
	s.sidesSwapped = !s.sidesSwapped
	s.round.Swapped = s.sidesSwapped
	s.sendAll(roundEvent{T: "round", Ev: "swap", Score: sc})
}

// startMatch: from warmup, a new match on the original sides, fresh scores,
// K/D, score points and money (startRound gives the money on round 1).
func (s *Server) startMatch(now time.Time) {
	if s.sidesSwapped {
		s.swapSides()
	}
	r := &s.round
	r.Score = map[string]int{content.Vibe: 0, content.Artisan: 0}
	r.Number, r.History, r.Overtime, r.MatchWinner = 0, nil, 0, ""
	for _, p := range s.players {
		p.Kills, p.Deaths, p.Assists, p.Points = 0, 0, 0, 0
	}
	s.startRound(now)
}

// endMatch shows the result for mp_match_restart_delay while the map vote
// runs; the vote's winner loads (changeLevel: a new match from warmup).
func (s *Server) endMatch(now time.Time, winner string) {
	r := &s.round
	d := secs(s.cvarF("mp_match_restart_delay", 20))
	r.Phase, r.endsAt, r.MatchWinner = PhaseMatchOver, now.Add(d), winner
	s.sendAll(roundEvent{T: "round", Ev: "matchend", Winner: winner, Score: r.Score, Number: r.Number})
	if s.vote == nil && len(s.voteOptions()) > 0 {
		s.StartMapVote(d)
	}
}

// restartMatch: the end screen is over and no vote changed the map: back to
// warmup on this map (the next match starts when warmup ends).
func (s *Server) restartMatch(now time.Time) {
	s.round = newRound()
	for _, p := range s.players {
		if p.Dead {
			p.respawnAt = now
		}
	}
	s.sendAll(roundEvent{T: "round", Ev: "warmup"})
}

// warmupTick: the warmup timer runs while both sides have a player; when it
// (or an admin's mp_warmup_end) is up, the match starts.
func (s *Server) warmupTick(now time.Time) {
	r := &s.round
	if r.endsAt.IsZero() {
		r.endsAt = now.Add(secs(s.cvarF("mp_warmup_time", 60)))
		s.sendAll(roundEvent{T: "round", Ev: "warmup"})
	}
	if s.warmupEnd || !now.Before(r.endsAt) {
		s.warmupEnd = false
		s.startMatch(now)
	}
}

// scoreKillPoints: killer and victim score points for one kill.
func scoreKillPoints(killer, victim *Player) {
	switch {
	case killer == nil:
	case killer == victim:
		victim.Points += scoreSuicide
	case killer.Team != "" && killer.Team == victim.Team:
		killer.Points += scoreTeamKill
	default:
		killer.Points += scoreKill
	}
}

// assistDamage: an enemy of the victim other than the killer who dealt more
// than this much damage to it this round gets an assist (CS: 41+).
const assistDamage = 41

// scoreAssists credits the assists for one kill (the damage ledger, mvp.go).
func (s *Server) scoreAssists(killer, victim *Player) {
	for _, o := range s.players {
		if o == killer || o == victim || o.Team == victim.Team || o.Team == "" {
			continue
		}
		if s.damageDealt(o, victim) >= assistDamage {
			o.Assists++
			o.Points += scoreAssist
		}
	}
}
