// Package maps defines the playable maps from prefabs and mapgen layout helpers.
package maps

import c "github.com/gigurra/counterslop/internal/content"

// All maps by name.
var All = map[string]func() c.Map{
	"de_slop": DeSlop,
}
