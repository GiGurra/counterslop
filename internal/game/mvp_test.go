package game

import (
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// mvpServer: a live round 1 with two Vibe Coders and two Artisans.
func mvpServer(t *testing.T) (*Server, []*Player) {
	t.Helper()
	s := authServer(t)
	ps := []*Player{}
	for i, team := range []string{content.Vibe, content.Vibe, content.Artisan, content.Artisan} {
		p := s.join([]string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"}[i])
		p.ready, p.Team, p.HP = true, team, maxHP
		ps = append(ps, p)
	}
	s.round.Phase, s.round.Number = PhaseLive, 1
	return s, ps
}

func TestMVPMostKillsThenDamage(t *testing.T) {
	s, ps := mvpServer(t)
	v1, v2, a1, a2 := ps[0], ps[1], ps[2], ps[3]
	s.noteDamage(v1, a1, 150, 100) // capped at the 100 HP a1 had
	s.noteKill(v1, a1)
	s.noteDamage(v2, a2, 60, 100)
	s.noteDamage(v1, a2, 40, 40)
	s.noteKill(v1, a2)
	s.decideMVP(content.Vibe, "elimination")
	if s.round.MVP != v1.ID || s.round.MVPWhy != "2 eliminations" {
		t.Fatalf("mvp %d %q, want %d (2 eliminations)", s.round.MVP, s.round.MVPWhy, v1.ID)
	}
	// One kill each: the one with more damage.
	s.round.Number = 2
	s.noteDamage(v1, a1, 30, 100)
	s.noteDamage(v1, a1, 70, 70)
	s.noteKill(v1, a1)
	s.noteDamage(v2, a2, 100, 100)
	s.noteDamage(v2, a1, 20, 100)
	s.noteKill(v2, a2)
	s.decideMVP(content.Vibe, "elimination")
	if s.round.MVP != v2.ID || s.round.MVPWhy != "1 elimination" {
		t.Fatalf("tie on kills: mvp %d %q, want %d", s.round.MVP, s.round.MVPWhy, v2.ID)
	}
	if s.damageDealt(v1, a1) != 100 || s.damageDealt(v2, a1) != 20 || s.damageDealt(a1, v1) != 0 || s.damageDealt(nil, v1) != 0 {
		t.Fatalf("damageDealt: %d %d", s.damageDealt(v1, a1), s.damageDealt(v2, a1))
	}
	// Only damage, no kills: most damage.
	s.round.Number = 3
	s.noteDamage(a2, v1, 50, 100)
	s.decideMVP(content.Artisan, "time")
	if s.round.MVP != a2.ID || s.round.MVPWhy != "most damage" {
		t.Fatalf("damage only: %d %q", s.round.MVP, s.round.MVPWhy)
	}
	// Nothing happened: no MVP; your own team's kills and self damage don't count.
	s.round.Number = 4
	s.noteDamage(v1, v1, 50, 100)
	s.noteKill(v1, v2)
	s.decideMVP(content.Vibe, "elimination")
	if s.round.MVP != 0 {
		t.Fatalf("an MVP for nothing: %d", s.round.MVP)
	}
}

func TestMVPForThePlantAndTheDefuse(t *testing.T) {
	s, ps := mvpServer(t)
	s.noteKill(ps[1], ps[2])
	s.round.Bomb = bombState{State: BombPlanted, Carrier: ps[0].ID}
	s.decideMVP(content.Vibe, "exploded")
	if s.round.MVP != ps[0].ID || s.round.MVPWhy != "merged The Big Diff" {
		t.Fatalf("exploded: %d %q", s.round.MVP, s.round.MVPWhy)
	}
	s.round.Bomb = bombState{State: BombDefused, Carrier: ps[0].ID, Worker: ps[3].ID}
	s.decideMVP(content.Artisan, "defused")
	if s.round.MVP != ps[3].ID || s.round.MVPWhy != "reverted The Big Diff" {
		t.Fatalf("defused: %d %q", s.round.MVP, s.round.MVPWhy)
	}
}

func TestEndRoundSendsMVPAndDamageReports(t *testing.T) {
	s, ps := mvpServer(t)
	v1, a1 := ps[0], ps[2]
	s.noteDamage(v1, a1, 27, 100)
	s.noteDamage(v1, a1, 27, 73)
	s.noteDamage(a1, v1, 36, 100)
	s.noteKill(v1, a1)
	for _, p := range ps {
		msgs(p)
	}
	s.endRound(time.Now(), content.Vibe, "elimination")
	if s.round.MVP != v1.ID {
		t.Fatalf("mvp %d", s.round.MVP)
	}
	var rows []any
	for _, m := range msgs(v1) {
		if m["t"] == "dmgreport" {
			rows = m["rows"].([]any)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("v1's report: %v", rows)
	}
	r := rows[0].(map[string]any)
	if r["id"] != float64(a1.ID) || r["g"] != 54.0 || r["gh"] != 2.0 || r["t"] != 36.0 || r["th"] != 1.0 {
		t.Fatalf("row %v", r)
	}
	// The next round starts a fresh ledger.
	s.round.Number++
	if len(s.damageReport(v1)) != 0 {
		t.Fatal("last round's damage carried over")
	}
}

func TestShotsAndGrenadesGoInTheLedger(t *testing.T) {
	s := authServer(t)
	alice := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-28, 0, 0})
	bob := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-14, 0, 0})
	nadeAt(s, alice, "mergeconflict", Vec3{-14.5, 0.1, 0}, 1.6)
	s.tickNades(0.01)
	if got := s.damageDealt(alice, bob); got <= 0 || got != maxHP-bob.HP {
		t.Fatalf("HE: ledger %d, bob lost %d", got, maxHP-bob.HP)
	}
}
