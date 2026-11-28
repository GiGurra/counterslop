package game

import (
	"encoding/json"
	"math"
)

// Crushing (docs/physics.md "Movers push and crush"): a mover that pushes a
// body into what won't give (a wall, the floor, another mover, a player who
// can't be pushed on) crushes it. The movement code reports how deep the
// push would have gone (sim Events.Crushed, metres per command); the body
// itself is never moved into the wall. A crush of crushKill or more in one
// command (a mover at 5 m/s or faster: a lift into a ceiling at speed, a ship
// landing on someone) kills at once; a slower one hurts crushPerMetre per
// metre, at least 1 HP a command (a door closing at 1 m/s kills in ~0.2 s).
// The kill feed names it "crush" (the world killed it: no killer).
const (
	crushKill     = 0.05  // m in one command: dead at once
	crushPerMetre = 500.0 // HP per metre below that
)

// crushDamage is the damage of a crush depth d (m, one command).
func crushDamage(d float64) int {
	if d <= 0 {
		return 0
	}
	if d >= crushKill {
		return 1 << 20
	}
	return max(1, int(math.Round(d*crushPerMetre)))
}

// crush hurts p for a crush of depth d (m) this command. Callers hold s.mu.
func (s *Server) crush(p *Player, d float64) {
	dmg := crushDamage(d)
	if dmg == 0 || p.Dead || p.body == nil || p.body.Noclip {
		return
	}
	p.HP -= dmg
	hb, _ := json.Marshal(map[string]any{"t": "hurt", "dmg": min(dmg, 100), "from": 0, "hp": max(p.HP, 0)})
	trySend(p, hb)
	if p.HP <= 0 {
		s.killBy(nil, p, "crush", false)
	}
}
