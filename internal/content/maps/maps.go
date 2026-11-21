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
	"de_legacy":    DeLegacy,
	"de_hackathon": DeHackathon,
	"de_cloudfarm": DeCloudfarm,
	"de_runway":    DeRunway,
	"gs_kiosk":     GsKiosk,
	"gs_megabyte":  GsMegabyte,
}

// Order is the default rotation and the selector's order.
var Order = []string{"de_slop", "de_monorepo", "de_standup", "de_stack", "de_pipeline", "de_legacy", "de_hackathon", "de_cloudfarm", "de_runway", "gs_kiosk", "gs_megabyte"}
