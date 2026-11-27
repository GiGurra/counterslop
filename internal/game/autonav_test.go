package game

import (
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// A map gets a nav graph built from its geometry (navgen) when it has none
// (a custom map) or bot_autonav is 1; otherwise it keeps its own.
func TestAutoNav(t *testing.T) {
	s := authServer(t)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changeLevelHow("de_slop", false) // at once: no load phase (maploading_test.go covers that)
	own := len(s.Map.Nav)
	if s.navAuto || own == 0 {
		t.Fatalf("de_slop: navAuto %v, %d nodes: want its own graph", s.navAuto, own)
	}
	s.cvars["bot_autonav"] = "1"
	s.changeLevelHow("de_slop", false) // at once: no load phase (maploading_test.go covers that)
	if !s.navAuto || len(s.Map.Nav) == 0 {
		t.Fatalf("bot_autonav 1: navAuto %v, %d nodes", s.navAuto, len(s.Map.Nav))
	}
	names := map[string]bool{}
	for _, n := range s.Map.Nav {
		names[n.Name] = true
	}
	for _, want := range []string{content.Vibe, content.Artisan, "A", "B"} {
		if !names[want] {
			t.Errorf("no %q node in the built graph", want)
		}
	}
	// A map without a graph (a custom map) gets one with bot_autonav 0 too.
	s.cvars["bot_autonav"] = "0"
	s.Map.Nav = nil
	s.autoNav()
	if !s.navAuto || len(s.Map.Nav) == 0 {
		t.Fatal("a map without nav got none")
	}
}
