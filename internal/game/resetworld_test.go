package game

import "testing"

// A case or scenario plays the same whatever ran before it on the server
// (cmd/sim -j: the order is up to timing): ResetWorld on an empty server
// starts the tick and the player IDs over (escalators run on the tick, bots
// seed their brains with their ID). Before, two runs of team-cap-map-switch
// differed in a bot's shots and the escalator cases in sub-mm positions.
func TestResetWorldStartsTicksAndIDsOver(t *testing.T) {
	s := authServer(t)
	s.SetLockstep(true)
	s.Step(137)
	s.mu.Lock()
	s.addAI("")
	s.mu.Unlock()
	if err := s.ResetWorldOn(""); err != nil {
		t.Fatal(err)
	}
	if s.Tick() != 0 || s.SnapTick() != -1 {
		t.Fatalf("tick %d, snap tick %d after the reset; want 0, -1", s.Tick(), s.SnapTick())
	}
	s.mu.Lock()
	p := s.addAI("")
	s.mu.Unlock()
	if p == nil || p.ID != 1 {
		t.Fatalf("the first player after the reset: %+v; want ID 1", p)
	}
}
