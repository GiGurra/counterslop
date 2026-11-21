package game

import (
	"encoding/json"
	"sort"
)

// Round MVP and the end-of-round damage report (CS: the MVP star and the
// console's "Damage Given / Damage Taken").
//
// A per-round ledger notes every hit (attacker, victim, damage actually taken,
// capped at the victim's HP) and kill. At round end:
//   - the MVP goes in the round state (every snapshot: "mvp", "mvpWhy"):
//     exploded -> the planter, defused -> the defuser, otherwise the winning
//     team's player with the most kills this round (ties: more damage);
//   - each player gets their own report ({t: "dmgreport"}: per opponent, damage
//     and hits given and taken). Optional, like any event: the client prints it.

type dmgRec struct{ Dmg, Hits int }

type roundLedger struct {
	round int
	dmg   map[[2]int]*dmgRec // [attacker, victim]
	kills map[int]int
}

// ledger is this round's, started fresh when the round number changes.
func (s *Server) ledger() *roundLedger {
	l := &s.dmgLedger
	if l.dmg == nil || l.round != s.round.Number {
		*l = roundLedger{round: s.round.Number, dmg: map[[2]int]*dmgRec{}, kills: map[int]int{}}
	}
	return l
}

// noteDamage records a hit of dmg on victim, who had hpBefore. Callers hold s.mu.
func (s *Server) noteDamage(att, victim *Player, dmg, hpBefore int) {
	if att == nil || victim == nil || att == victim || dmg <= 0 {
		return
	}
	k := [2]int{att.ID, victim.ID}
	l := s.ledger()
	r := l.dmg[k]
	if r == nil {
		r = &dmgRec{}
		l.dmg[k] = r
	}
	r.Dmg += min(dmg, max(hpBefore, 0))
	r.Hits++
}

// damageDealt is the damage att has done to victim this round (assists: the
// ledger already has the killing hit when killBy runs). Callers hold s.mu.
func (s *Server) damageDealt(att, victim *Player) int {
	if att == nil || victim == nil {
		return 0
	}
	if r := s.ledger().dmg[[2]int{att.ID, victim.ID}]; r != nil {
		return r.Dmg
	}
	return 0
}

// noteKill counts a kill for the round. Callers hold s.mu.
func (s *Server) noteKill(killer, victim *Player) {
	if killer != nil && killer != victim && killer.Team != victim.Team {
		s.ledger().kills[killer.ID]++
	}
}

// decideMVP sets the round's MVP. Call at the start of endRound (the
// defuser is still the bomb's worker then). Callers hold s.mu.
func (s *Server) decideMVP(winner, reason string) {
	r := &s.round
	r.MVP, r.MVPWhy = 0, ""
	switch {
	case reason == "exploded" && r.Bomb.Carrier != 0:
		r.MVP, r.MVPWhy = r.Bomb.Carrier, "merged The Big Diff"
		return
	case reason == "defused" && r.Bomb.Worker != 0:
		r.MVP, r.MVPWhy = r.Bomb.Worker, "reverted The Big Diff"
		return
	}
	l := s.ledger()
	given := map[int]int{}
	for k, d := range l.dmg {
		given[k[0]] += d.Dmg
	}
	best, bk, bd := 0, 0, 0
	ids := make([]int, 0, len(s.players))
	for id := range s.players {
		ids = append(ids, id)
	}
	sort.Ints(ids) // deterministic ties
	for _, id := range ids {
		p := s.players[id]
		if p.Team != winner {
			continue
		}
		k, d := l.kills[id], given[id]
		if k > bk || k == bk && d > bd {
			best, bk, bd = id, k, d
		}
	}
	if best == 0 || bk == 0 && bd == 0 {
		return // nobody did anything: no MVP
	}
	r.MVP = best
	switch bk {
	case 0:
		r.MVPWhy = "most damage"
	case 1:
		r.MVPWhy = "1 elimination"
	default:
		r.MVPWhy = itoa(bk) + " eliminations"
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// DmgRow is one opponent in a player's damage report.
type DmgRow struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Given     int    `json:"g"`
	GivenHits int    `json:"gh"`
	Taken     int    `json:"t"`
	TakenHits int    `json:"th"`
}

// damageReport is p's rows this round: everyone p hit or was hit by, by id.
func (s *Server) damageReport(p *Player) []DmgRow {
	l := s.ledger()
	rows := map[int]*DmgRow{}
	row := func(id int) *DmgRow {
		if rows[id] == nil {
			name := ""
			if q := s.players[id]; q != nil {
				name = q.Name
			}
			rows[id] = &DmgRow{ID: id, Name: name}
		}
		return rows[id]
	}
	for k, d := range l.dmg {
		switch p.ID {
		case k[0]:
			r := row(k[1])
			r.Given, r.GivenHits = r.Given+d.Dmg, r.GivenHits+d.Hits
		case k[1]:
			r := row(k[0])
			r.Taken, r.TakenHits = r.Taken+d.Dmg, r.TakenHits+d.Hits
		}
	}
	out := make([]DmgRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// sendDamageReports sends every human player their report. Callers hold s.mu.
func (s *Server) sendDamageReports() {
	for _, p := range s.players {
		if p.Bot || p.ai != nil {
			continue
		}
		b, _ := json.Marshal(map[string]any{"t": "dmgreport", "n": s.round.Number, "rows": s.damageReport(p)})
		trySend(p, b)
	}
}
