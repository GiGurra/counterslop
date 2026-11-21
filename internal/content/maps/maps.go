// Package maps defines the playable maps from prefabs and mapgen layout helpers.
package maps

import c "github.com/gigurra/counterslop/internal/content"

// All maps by name.
var All = map[string]func() c.Map{
	"de_slop":      DeSlop,
	"de_monorepo":  DeMonorepo,
	"de_standup":   DeStandup,
	"de_stack":     DeStack,
	"de_pipeline":  DePipeline,
	"de_hackathon": DeHackathon,
}

// Order is the default rotation and the selector's order.
var Order = []string{"de_slop", "de_monorepo", "de_standup", "de_stack", "de_pipeline", "de_hackathon"}
