package game

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// roundServer: one Vibe Coder and one Artisan, rounds on, no freeze time.
func roundServer(t *testing.T) (*Server, *Player, *Player) {
	s := authServer(t)
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	v, a := s.join("1.1.1.1"), s.join("2.2.2.2")
	v.ready, a.ready = true, true
	v.Team, a.Team = content.Vibe, content.Artisan
	s.queueCmds(v, []Cmd{{Seq: 1}})
	s.queueCmds(a, []Cmd{{Seq: 1}})
	s.simTick(time.Now())
	return s, v, a
}

// ticks runs n sim + round ticks, v and a sending idle commands with buttons b.
func ticks(s *Server, n int, now *time.Time, cmds map[*Player]Cmd) {
	for range n {
		for p, c := range cmds {
			c.Seq = p.lastQueued + 1
			s.queueCmds(p, []Cmd{c})
		}
		*now = now.Add(time.Second / SimHz)
		s.simTick(*now)
		s.tickRound(*now)
	}
}

func TestRoundStartsAndGivesTheBigDiff(t *testing.T) {
	s, v, _ := roundServer(t)
	now := time.Now()
	ticks(s, 2, &now, nil)
	if s.round.Phase != PhaseLive || s.round.Number != 1 {
		t.Fatalf("phase %q round %d, want live round 1", s.round.Phase, s.round.Number)
	}
	if s.round.Bomb.State != BombCarried || s.round.Bomb.Carrier != v.ID {
		t.Fatalf("bomb %+v, want carried by the only Vibe Coder", s.round.Bomb)
	}
}

func TestEliminationEndsRoundAndNoTimedRespawn(t *testing.T) {
	s, v, a := roundServer(t)
	now := time.Now()
	ticks(s, 2, &now, nil)
	s.killBy(v, a, "prompt", false)
	ticks(s, 1, &now, nil)
	if s.round.Phase != PhaseOver || s.round.Winner != content.Vibe || s.round.Score[content.Vibe] != 1 {
		t.Fatalf("round %+v, want Vibe Coders win by elimination", s.round)
	}
	now = now.Add(4 * time.Second) // past mp_respawntime: still dead until the next round
	ticks(s, 1, &now, nil)
	if !a.Dead {
		t.Fatal("respawned mid-round")
	}
	now = now.Add(overSecs * time.Second)
	ticks(s, 2, &now, nil)
	if a.Dead || s.round.Number != 2 {
		t.Fatalf("dead %v round %d, want everyone back for round 2", a.Dead, s.round.Number)
	}
}

func TestPlantExplodeAndDefuse(t *testing.T) {
	for _, defuse := range []bool{false, true} {
		s, v, a := roundServer(t)
		s.cvars["mp_c4timer"] = "20"
		now := time.Now()
		ticks(s, 2, &now, nil)
		site := s.Map.Sites[0]
		at := Vec3{(site.Min[0] + site.Max[0]) / 2, 1.5, (site.Min[2] + site.Max[2]) / 2}
		s.teleport(v, at, 0, false, v.lastQueued)
		ticks(s, 30, &now, map[*Player]Cmd{v: {}}) // land
		// The carrier has it in slot 5, not drawn; E doesn't plant (it only defuses), nor does fire with a gun.
		held := v.inv.Current
		if v.inv.Slots[BombSlot] == nil || held == BombSlot {
			t.Fatalf("carrier slots %v current %q, want The Big Diff in slot 5, not drawn", v.inv.Slots, held)
		}
		ticks(s, int(plantSecs*SimHz)+2, &now, map[*Player]Cmd{v: {B: BtnUse}})
		ticks(s, int(plantSecs*SimHz)+2, &now, map[*Player]Cmd{v: {B: BtnFire}})
		if s.round.Bomb.State != BombCarried {
			t.Fatalf("bomb %+v: E or a gun's fire planted it", s.round.Bomb)
		}
		// Drawn (slot 5), fire held: moving resets the plant.
		ticks(s, 2, &now, map[*Player]Cmd{v: {W: BombSlot}})
		ticks(s, int(plantSecs*SimHz)-10, &now, map[*Player]Cmd{v: {B: BtnFire}})
		if s.round.Bomb.Progress < 0.9 {
			t.Fatalf("bomb %+v: fire with The Big Diff drawn isn't planting", s.round.Bomb)
		}
		ticks(s, 2, &now, map[*Player]Cmd{v: {F: 1, B: BtnFire}})
		if s.round.Bomb.State != BombCarried || s.round.Bomb.Progress != 0 {
			t.Fatalf("bomb %+v after moving, want progress reset", s.round.Bomb)
		}
		ticks(s, 30, &now, map[*Player]Cmd{v: {}})
		ticks(s, int(plantSecs*SimHz)+2, &now, map[*Player]Cmd{v: {B: BtnFire}})
		if s.round.Phase != PhasePlanted || s.round.Bomb.Site != site.Name {
			t.Fatalf("phase %q bomb %+v, want planted at %s", s.round.Phase, s.round.Bomb, site.Name)
		}
		if v.inv.Slots[BombSlot] != nil || v.inv.Current != held {
			t.Fatalf("after the plant: slots %v current %q, want slot 5 gone and back to %q", v.inv.Slots, v.inv.Current, held)
		}
		if !defuse {
			now = now.Add(21 * time.Second)
			ticks(s, 1, &now, nil)
			if s.round.Winner != content.Vibe || s.round.Reason != "exploded" || !v.Dead {
				t.Fatalf("round %+v planter dead %v, want an explosion win (that kills the planter standing on it)", s.round, v.Dead)
			}
			if s.round.MVP != v.ID {
				t.Fatalf("exploded: MVP %d, want the planter %d", s.round.MVP, v.ID)
			}
			continue
		}
		s.teleport(a, Vec3{s.round.Bomb.Pos[0] + 1, s.round.Bomb.Pos[1], s.round.Bomb.Pos[2]}, 0, false, a.lastQueued)
		ticks(s, 30, &now, map[*Player]Cmd{a: {}})
		ticks(s, int(defuseSecs*SimHz)+2, &now, map[*Player]Cmd{a: {B: BtnUse}})
		if s.round.Winner != content.Artisan || s.round.Reason != "defused" {
			t.Fatalf("round %+v, want Artisans win by defuse", s.round)
		}
		if s.round.MVP != a.ID || s.round.MVPWhy != "reverted The Big Diff" {
			t.Fatalf("defused: MVP %d %q, want the defuser %d", s.round.MVP, s.round.MVPWhy, a.ID)
		}
	}
}

func TestBigDiffDropsOnDeathAndGetsPickedUp(t *testing.T) {
	s, v, a := roundServer(t)
	v2 := s.join("3.3.3.3")
	v2.ready, v2.Team = true, content.Vibe
	now := time.Now()
	s.round.Phase = PhaseWarmup
	ticks(s, 2, &now, nil)
	carrier, other := v, v2
	if s.round.Bomb.Carrier == v2.ID {
		carrier, other = v2, v
	}
	s.killBy(a, carrier, "prompt", false)
	if s.round.Bomb.State != BombDropped {
		t.Fatalf("bomb %+v, want dropped", s.round.Bomb)
	}
	s.teleport(other, s.round.Bomb.Pos, 0, false, other.lastQueued)
	ticks(s, 1, &now, nil)
	if s.round.Bomb.State != BombCarried || s.round.Bomb.Carrier != other.ID {
		t.Fatalf("bomb %+v, want picked up by %d", s.round.Bomb, other.ID)
	}
}

func TestTimeoutWithoutPlantIsArtisanWinAndDeathmatchRespawns(t *testing.T) {
	s, _, a := roundServer(t)
	now := time.Now()
	ticks(s, 2, &now, nil)
	now = now.Add(116 * time.Second)
	ticks(s, 1, &now, nil)
	if s.round.Winner != content.Artisan || s.round.Reason != "time" {
		t.Fatalf("round %+v, want Artisans win on time", s.round)
	}
	s.cvars["mp_respawn"] = "1"
	ticks(s, 1, &now, nil)
	if s.round.Phase != PhaseWarmup {
		t.Fatalf("phase %q, want deathmatch (warmup)", s.round.Phase)
	}
	s.killBy(a, a, "prompt", false)
	now = now.Add(4 * time.Second)
	ticks(s, 1, &now, nil)
	if a.Dead {
		t.Fatal("no timed respawn in deathmatch")
	}
}

// On a test server the first Vibe Coder to join (say hello) carries The Big
// Diff, not the lowest ID: clients connect on page load, before PLAY, so a
// later joiner can hold an earlier ID (the rounds scenario gave it to carol).
func TestTestServerCarrierIsTheFirstToJoin(t *testing.T) {
	s := authServer(t)
	s.Test = true
	s.cvars["mp_freezetime"], s.cvars["mp_warmup_time"], s.cvars["mp_respawn"] = "0", "0", "0"
	carol, alice, bob := s.join("1.1.1.1"), s.join("2.2.2.2"), s.join("3.3.3.3") // carol's page connected first
	for _, j := range []struct {
		p    *Player
		team string
	}{{alice, content.Vibe}, {bob, content.Artisan}, {carol, content.Vibe}} { // alice joined first
		s.handle(j.p, inbound{T: "hello", Name: "x"})
		j.p.Team = j.team
	}
	now := time.Now()
	ticks(s, 2, &now, map[*Player]Cmd{alice: {}, bob: {}, carol: {}})
	if s.round.Bomb.Carrier != alice.ID {
		t.Fatalf("carrier #%d, want alice #%d (first to join; carol #%d connected first)", s.round.Bomb.Carrier, alice.ID, carol.ID)
	}
}

// The round snapshot carries the buy time left (the HUD's buy icon counts it
// down): mp_buytime after the freeze, none in warmup or once it has run out.
func TestRoundJSONBuyTimeLeft(t *testing.T) {
	buyLeft := func(s *Server, now time.Time) float64 {
		var m struct {
			Buy float64 `json:"buy"`
		}
		if err := json.Unmarshal(s.roundJSON(now), &m); err != nil {
			t.Fatal(err)
		}
		return m.Buy
	}
	s := authServer(t)
	if b := buyLeft(s, time.Now()); b != 0 {
		t.Fatalf("warmup: buy %v, want none (no limit)", b)
	}
	s, _, _ = roundServer(t)
	s.cvars["mp_buytime"] = "20"
	now := time.Now()
	ticks(s, 2, &now, nil)
	s.round.buyUntil = now.Add(20 * time.Second)
	if b := buyLeft(s, now.Add(5*time.Second)); b != 15 {
		t.Fatalf("live, 5 s in: buy %v, want 15", b)
	}
	if b := buyLeft(s, now.Add(25*time.Second)); b != 0 {
		t.Fatalf("after buy time: buy %v, want 0", b)
	}
}
