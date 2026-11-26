package game

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gigurra/counterslop/internal/content"
)

// The replication schema (docs/replication.md). Every field of a type that
// reaches the wire carries a `net:"..."` tag saying what it is, so a later
// replication engine can send it by declaration instead of by hand, and
// netschema_test.go fails on a field nobody decided about.
//
//	net:"-"            never sent (server-only state); exported ones say why in a comment
//	net:"hot[,mods]"   changes most ticks (pose, the self body, timers): every snapshot
//	net:"cold[,mods]"  changes rarely (name, team, score, money, HP...): may go on change only
//
// Modifiers, at most one of each group:
//
//	scope:     self   only in the recipient's own state (me, inv, welcome's id)
//	           team   only to the recipient's team (spotted enemies)
//	precision: exact  full float64 (prediction replays from it)
//	           mm     millimetres (Vec3)
//	           angle  4 decimals (view angles, Angle)
//	           gait   Gait's quantised ints (gaitQ)
//
// A float-carrying field (float64, Vec3, [3]float64, Gait, or a container of
// them) needs exactly one precision; any other field may not have one.
const (
	netNever = "-"
	netHot   = "hot"
	netCold  = "cold"
)

var (
	netScopes     = map[string]bool{"self": true, "team": true}
	netPrecisions = map[string]bool{"exact": true, "mm": true, "angle": true, "gait": true}
)

// netTag is a parsed `net` struct tag.
type netTag struct {
	Class     string // "-", "hot" or "cold"
	Scope     string // "", "self" or "team"
	Precision string // "", "exact", "mm", "angle" or "gait"
}

func (t netTag) Sent() bool { return t.Class != netNever }

// parseNetTag reads a `net` tag value. It rejects unknown words, a missing or
// repeated class, and two words of the same modifier group.
func parseNetTag(s string) (netTag, error) {
	if s == "" {
		return netTag{}, fmt.Errorf("no net tag")
	}
	if s == netNever {
		return netTag{Class: netNever}, nil
	}
	words := strings.Split(s, ",")
	var t netTag
	switch words[0] {
	case netHot, netCold:
		t.Class = words[0]
	default:
		return netTag{}, fmt.Errorf("net tag %q: the first word must be hot, cold or -", s)
	}
	for _, w := range words[1:] {
		switch {
		case netScopes[w]:
			if t.Scope != "" {
				return netTag{}, fmt.Errorf("net tag %q: two scopes (%s, %s)", s, t.Scope, w)
			}
			t.Scope = w
		case netPrecisions[w]:
			if t.Precision != "" {
				return netTag{}, fmt.Errorf("net tag %q: two precisions (%s, %s)", s, t.Precision, w)
			}
			t.Precision = w
		case w == netHot || w == netCold || w == netNever:
			return netTag{}, fmt.Errorf("net tag %q: one class only", s)
		default:
			return netTag{}, fmt.Errorf("net tag %q: unknown word %q", s, w)
		}
	}
	return t, nil
}

// snapMsg declares the full snapshot ({"t":"snap"}) that snapBytes assembles
// by hand from pre-marshalled parts. It is the schema's root, not marshalled
// in production; TestSnapMsgMatchesSnapBytes pins the two to the same bytes.
// (The pose snapshot is Player's hot fields as rows: posesJSON,
// TestPoseRowsAreHotFields.)
type snapMsg struct {
	T       string    `json:"t" net:"cold"`
	Time    int64     `json:"time" net:"hot"` // server clock, ms
	Tick    int64     `json:"tick" net:"hot"`
	Players []*Player `json:"players" net:"hot"`
	Items   []*Item   `json:"items" net:"cold"`
	Nades   []*Nade   `json:"nades,omitempty" net:"hot"`
	Boats   []*Boat   `json:"boats,omitempty" net:"hot"`
	Spot    *[]int    `json:"spot,omitempty" net:"hot,team"` // a pointer: an empty list is sent as []
	Round   *roundMsg `json:"round,omitempty" net:"hot"`
	Me      *meState  `json:"me,omitempty" net:"hot,self"`
}

// netRoots are the state messages the schema covers: every struct type
// reachable from them through sent fields must be in netTypes.
var netRoots = []reflect.Type{
	reflect.TypeFor[snapMsg](),
	reflect.TypeFor[welcomeMsg](),
	reflect.TypeFor[invMsg](),
}

// netTypes is the registry: every struct type a state message carries.
var netTypes = []reflect.Type{
	reflect.TypeFor[snapMsg](),
	reflect.TypeFor[welcomeMsg](),
	reflect.TypeFor[invMsg](),
	reflect.TypeFor[Player](),
	reflect.TypeFor[meState](),
	reflect.TypeFor[weaponState](),
	reflect.TypeFor[hookState](),
	reflect.TypeFor[Inventory](),
	reflect.TypeFor[Held](),
	reflect.TypeFor[Item](),
	reflect.TypeFor[Nade](),
	reflect.TypeFor[Boat](),
	reflect.TypeFor[boatExact](),
	reflect.TypeFor[roundMsg](),
	reflect.TypeFor[roundState](),
	reflect.TypeFor[bombState](),
	reflect.TypeFor[gpuState](),
	reflect.TypeFor[roundResult](),
}

// netLeaves are types with their own wire encoding (a MarshalJSON): the
// schema doesn't look inside them. The value is the precision a field of
// that type must declare ("" for none).
var netLeaves = map[reflect.Type]string{
	reflect.TypeFor[Vec3]():  "mm",
	reflect.TypeFor[Angle](): "angle",
	reflect.TypeFor[Gait]():  "gait",
	// Static map data: sent whole in welcome and on a map change, never diffed.
	reflect.TypeFor[content.Map](): "",
}
