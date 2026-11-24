package game

import (
	"testing"
	"time"
)

// gearUp hands p a warmup-rich kit: a rifle in hand, a sniper to drop, armor,
// helmet, kit, shades and full money.
func gearUp(s *Server, p *Player) {
	p.inv.Slots["primary"] = s.Weapons.newHeld("codereview")
	p.inv.Current = "primary"
	p.Armor, p.Helmet, p.Kit, p.Shades = 100, true, true, true
	p.Money = 16000
}

// droppedOnGround: the ground's dropped weapons (not the map's own).
func droppedOnGround(s *Server) []string {
	var out []string
	for _, it := range s.items {
		if it.mapSlot < 0 {
			out = append(out, it.Weapon)
		}
	}
	return out
}

// wantMatchStartState: the state CS gives everyone at a match (re)start.
func wantMatchStartState(t *testing.T, s *Server, p *Player) {
	t.Helper()
	if s.round.Number != 1 || s.round.Phase != PhaseFreeze && s.round.Phase != PhaseLive {
		t.Fatalf("round %d %q, want round 1 started", s.round.Number, s.round.Phase)
	}
	if got := droppedOnGround(s); len(got) != 0 {
		t.Errorf("dropped weapons on the ground at match start: %v", got)
	}
	if p.inv.Slots["primary"] != nil {
		t.Errorf("primary %q carried into the match", p.inv.Slots["primary"].ID)
	}
	if p.Money != s.econ().StartMoney {
		t.Errorf("money %d, want the start money %d", p.Money, s.econ().StartMoney)
	}
	if p.Armor != 0 || p.Helmet || p.Kit || p.Shades {
		t.Errorf("armor %d helmet %v kit %v shades %v carried into the match", p.Armor, p.Helmet, p.Kit, p.Shades)
	}
}

func TestNewRoundClearsDroppedWeaponsButKeepsMapItems(t *testing.T) {
	s, v, _ := roundServer(t)
	s.cvars["mp_round_restart_delay"] = "0"
	now := time.Now()
	ticks(s, 2, &now, nil)
	if s.round.Phase != PhaseLive {
		t.Fatalf("phase %q, want live", s.round.Phase)
	}
	gearUp(s, v)
	s.drop(v)
	mapItem := s.spawnItem(*s.Weapons.newHeld("prompt"), Vec3{5, 0, 5}, 0) // placed by the map
	if got := droppedOnGround(s); len(got) != 1 || got[0] != "codereview" {
		t.Fatalf("dropped %v, want the Code Review on the ground", got)
	}
	s.endRound(now, v.Team, "elimination")
	ticks(s, 2, &now, nil)
	if s.round.Number != 2 {
		t.Fatalf("round %d %q, want round 2", s.round.Number, s.round.Phase)
	}
	if got := droppedOnGround(s); len(got) != 0 {
		t.Errorf("dropped weapons still on the ground in round 2: %v", got)
	}
	if !containsItem(s, mapItem) {
		t.Errorf("the map's own item went with the dropped ones")
	}
	if s.round.Bomb.State != BombCarried {
		t.Errorf("bomb %+v, want carried in the new round", s.round.Bomb)
	}
}

func containsItem(s *Server, it *Item) bool {
	for _, o := range s.items {
		if o == it {
			return true
		}
	}
	return false
}

// Warmup money buys and drops guns; none of it reaches the match.
func TestWarmupGearAndDropsDontReachTheMatch(t *testing.T) {
	s, v, _ := roundServer(t)
	s.cvars["mp_warmup_time"] = "60"
	s.round = newRound()
	now := time.Now()
	ticks(s, 2, &now, nil)
	if s.round.Phase != PhaseWarmup {
		t.Fatalf("phase %q, want warmup", s.round.Phase)
	}
	gearUp(s, v)
	s.drop(v)
	gearUp(s, v) // and one more in hand
	if len(droppedOnGround(s)) != 1 {
		t.Fatalf("dropped %v, want one gun on the ground", droppedOnGround(s))
	}
	s.warmupEnd = true
	ticks(s, 1, &now, nil)
	wantMatchStartState(t, s, v)
}

// The admin's restart (mp_restartgame) mid-match is a match start too.
func TestRestartClearsGroundAndGear(t *testing.T) {
	s, v, _ := roundServer(t)
	now := time.Now()
	ticks(s, 2, &now, nil)
	s.round.Number = 5
	gearUp(s, v)
	s.drop(v)
	gearUp(s, v)
	s.restart()
	wantMatchStartState(t, s, v)
}

// After the last round: rich players drop guns on the end screen; the next
// match (via warmup) starts clean.
func TestNextMatchStartsClean(t *testing.T) {
	s, v, _ := roundServer(t)
	s.cvars["mp_match_restart_delay"] = "0"
	now := time.Now()
	ticks(s, 2, &now, nil)
	s.round.Number = 12
	s.endMatch(now, v.Team)
	gearUp(s, v)
	s.drop(v)
	gearUp(s, v)
	ticks(s, 3, &now, nil) // match over -> warmup (0 s) -> the next match
	wantMatchStartState(t, s, v)
}

// restart in warmup (no rounds) clears the floor too.
func TestWarmupRestartClearsGround(t *testing.T) {
	s, v, _ := roundServer(t)
	s.cvars["mp_warmup_time"] = "60"
	s.round = newRound()
	now := time.Now()
	ticks(s, 2, &now, nil)
	gearUp(s, v)
	s.drop(v)
	s.restart()
	if got := droppedOnGround(s); len(got) != 0 {
		t.Errorf("dropped weapons on the ground after restart: %v", got)
	}
}
