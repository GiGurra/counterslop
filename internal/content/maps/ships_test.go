package maps

import (
	"math"
	"testing"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// A ship on a timetable (a path mover) is back at its dock before its cycle
// wraps: else it jumps there at the wrap and shoves whoever stands where it
// reappears (cq_moonshot's MVP-1 flies back empty, doors shut).
func TestShipsDockBeforeTheWrap(t *testing.T) {
	for _, name := range Order {
		m := Find(name)()
		for i := range m.Movers {
			mv := &m.Movers[i]
			if mv.Kind != c.MoverPath || mv.Period <= 0 {
				continue
			}
			end := sim.MoverOffset(mv, mv.Period-1e-6)
			if d := math.Sqrt(end[0]*end[0] + end[1]*end[1] + end[2]*end[2]); d > 1e-6 {
				t.Errorf("%s: mover %d is %.1f m from its dock when its cycle wraps (it jumps back)", name, i+1, d)
			}
		}
	}
}

// cq_moonshot: Earth's spawns and the departure's door are on one nav
// island, every capture point on the other (the Moon), and MVP-1's drop
// pods land Earth on the Moon by the drop zone (mapcheck.Crossings).
func TestMoonshotCrossing(t *testing.T) {
	m := CqMoonshot()
	if p := m.ModeProblems(); len(p) != 0 {
		t.Fatalf("mode problems: %v", p)
	}
	if len(m.Departures) != 1 || len(m.TicketGates) != 1 || len(m.Scanners) != 3 || len(m.Objectives()) != 5 {
		t.Fatalf("%d departures, %d gates, %d scanners, %d points", len(m.Departures), len(m.TicketGates), len(m.Scanners), len(m.Objectives()))
	}
	if m.Category != c.Conquest || m.MaxPlayers != 32 {
		t.Fatalf("category %q, %d players", m.Category, m.MaxPlayers)
	}
	live := 0
	for _, g := range m.Signs {
		if g.Live {
			live++
		}
	}
	if live < 3 {
		t.Errorf("%d live signs (the backlog counter, the departure board, the T-minus clock)", live)
	}
}
