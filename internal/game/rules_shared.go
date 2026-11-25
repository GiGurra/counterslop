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
// HookRulesOf: the hook's rules from the sv_hook_* cvars (each at least 0).
func HookRulesOf(rng, energy, recharge, drain, cost, pull float64) sim.HookRules {
	return sim.HookRules{Range: math.Max(0, rng), Energy: math.Max(0, energy), Recharge: math.Max(0, recharge), Drain: math.Max(0, drain), Cost: math.Max(0, cost), Pull: math.Max(0, pull)}
}

func MoveRulesOf(movement string, airAccel float64, autoBhop bool) sim.MoveRules {
	model := sim.MoveClassic
	if movement == "source" {
		model = sim.MoveSource
	}
	return sim.MoveRules{Model: model, AirAccel: math.Max(0, math.Min(sim.MaxAirAccelerate, airAccel)), AutoBhop: autoBhop, Hook: sim.HookRules{Range: 0, Energy: 0, Recharge: 0, Drain: 0, Cost: 0, Pull: 0}}
}
