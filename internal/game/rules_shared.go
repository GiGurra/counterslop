//gojs:shared

package game

// The movement rules from the server settings, shared with the client
// (shared.gen.js): both sides turn the effective cvars (the map's Rules over
// the server's, Server.cvar / web/game/rules.js) into the same sim.MoveRules.

import (
	"math"

	"github.com/gigurra/counterslop/internal/sim"
)

// MoveRulesOf is the movement rules of sv_movement ("classic" | "source",
// anything else is classic), sv_airaccelerate (0 or less: the default) and
// sv_autobunnyhopping.
func MoveRulesOf(movement string, airAccel float64, autoBhop bool) sim.MoveRules {
	model := sim.MoveClassic
	if movement == "source" {
		model = sim.MoveSource
	}
	return sim.MoveRules{Model: model, AirAccel: math.Max(0, math.Min(sim.MaxAirAccelerate, airAccel)), AutoBhop: autoBhop}
}
