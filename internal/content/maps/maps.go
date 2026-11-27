// Package maps defines the playable maps from prefabs and mapgen layout helpers.
package maps

import c "github.com/gigurra/counterslop/internal/content"

// All maps by name.
var All = map[string]func() c.Map{
	"de_slop":             DeSlop,
	"de_monorepo":         DeMonorepo,
	"de_standup":          DeStandup,
	"de_stack":            DeStack,
	"de_pipeline":         DePipeline,
	"de_legacy":           DeLegacy,
	"de_hackathon":        DeHackathon,
	"de_cloudfarm":        DeCloudfarm,
	"de_runway":           DeRunway,
	"de_rackmount":        DeRackmount,
	"de_hostile_takeover": DeHostileTakeover,
	"he_merge_hell":       HeMergeHell,
	"surf_scope_creep":    SurfScopeCreep,
	"gs_kiosk":            GsKiosk,
	"gs_megabyte":         GsMegabyte,
	"rc_ci_pipeline":      RcCiPipeline,
	"rc_merge_train":      RcMergeTrain,
	"rc_hotfix_highway":   RcHotfixHighway,
	"cq_moonshot":         CqMoonshot,
}

// Order is the default rotation and the selector's order.
var Order = []string{"de_slop", "de_monorepo", "de_standup", "de_stack", "de_pipeline", "de_legacy", "de_hackathon", "de_cloudfarm", "de_runway", "de_rackmount", "de_hostile_takeover", "surf_scope_creep", "gs_kiosk", "gs_megabyte", "cq_moonshot", "rc_ci_pipeline", "rc_merge_train", "rc_hotfix_highway", "he_merge_hell"}
