package content

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// GPU Drop map rules (docs/gpu-drop.md).
const (
	MinGPUs        = 2    // mp_gpus_to_win's default: a map must stock at least that many
	MinExits       = 2    // the loading dock, the front entrance, ...
	MinExitSpacing = 20.0 // metres between exit centres (x/z): "far apart"
)

// NoBomb: the map's Rules pin team deathmatch (mp_respawn 1: he_merge_hell),
// so its sites never see a bomb; they are only where the bots go to fight.
func (m Map) NoBomb() bool { return m.Rules["mp_respawn"] == "1" }

// ModeProblems lists what makes m break its mode's rules: defusal maps are
// de_ with sites A and B; GPU Drop maps are gs_ in the GPU Drop category with
// stock, exits far apart and no sites. Empty means it plays.
func (m Map) ModeProblems() []string {
	var out []string
	bad := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	switch m.Mode {
	case ModeDefusal:
		if len(m.Points) > 0 {
			bad("a defusal map with capture points (set Mode %q)", ModeTickets)
		}
		var sites []string
		for _, z := range m.Sites {
			sites = append(sites, z.Name)
		}
		if fmt.Sprint(sites) != "[A B]" {
			bad("sites %v, want [A B]", sites)
		}
		if len(m.GPUs) > 0 || len(m.Exits) > 0 {
			bad("a defusal map with GPUs or exits (set Mode %q)", ModeGPUDrop)
		}
		if m.Category == GPUDrop {
			bad("category %q needs Mode %q", GPUDrop, ModeGPUDrop)
		}
		he := strings.HasPrefix(m.Name, "he_")
		if !strings.HasPrefix(m.Name, "de_") && !strings.HasPrefix(m.Name, "surf_") && !he {
			bad("a defusal map is named de_* (or surf_*: defusal on surf ramps; he_*: grenades only)")
		}
		if he && m.Rules["mp_loadout"] != "nades" {
			bad("an he_* map pins mp_loadout nades (its Rules)")
		}
	case ModeGPUDrop:
		if len(m.Sites) > 0 {
			bad("GPU Drop has no bomb sites")
		}
		if m.Category != GPUDrop {
			bad("category %q, want %q", m.Category, GPUDrop)
		}
		if !strings.HasPrefix(m.Name, "gs_") {
			bad("a GPU Drop map is named gs_*")
		}
		if len(m.GPUs) < MinGPUs {
			bad("%d GPUs on the shelves, want at least %d", len(m.GPUs), MinGPUs)
		}
		if len(m.Exits) < MinExits {
			bad("%d exits, want at least %d", len(m.Exits), MinExits)
		}
		var names []string
		for i, e := range m.Exits {
			if e.Name == "" || slices.Contains(names, e.Name) {
				bad("exit %d: name %q is empty or taken", i, e.Name)
			}
			names = append(names, e.Name)
			for _, f := range m.Exits[:i] {
				a, b := e.Center(), f.Center()
				if d := math.Hypot(a[0]-b[0], a[2]-b[2]); d < MinExitSpacing {
					bad("exits %s and %s are %.1f m apart, want %.0f+", f.Name, e.Name, d, MinExitSpacing)
				}
			}
			for _, g := range m.GPUs {
				if e.Contains(g.Pos) {
					bad("a GPU at %v stands in exit %s", g.Pos, e.Name)
				}
			}
		}
	case ModeTickets:
		out = append(out, m.ticketProblems()...)
	case ModeRace:
		out = append(out, m.raceProblems()...)
	default:
		bad("unknown mode %q", m.Mode)
	}
	return out
}

// ticketProblems: a tickets map (Star Slop, docs/starslop.md) is cq_* (or a
// lab_* test map) in the Conquest category, with no sites, GPUs or exits; its
// capture points have unique ids, a radius and known owners; each team has
// exactly one main (a locked spawn point it owns) and at least one point can
// be captured; every spawn point has spawn spots inside it.
func (m Map) ticketProblems() []string {
	var out []string
	bad := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	lab := strings.HasPrefix(m.Name, "lab_")
	if !strings.HasPrefix(m.Name, "cq_") && !lab {
		bad("a tickets map is named cq_* (or lab_* for a test map)")
	}
	if m.Category != Conquest && !lab {
		bad("category %q, want %q", m.Category, Conquest)
	}
	if len(m.Sites) > 0 || len(m.GPUs) > 0 || len(m.Exits) > 0 {
		bad("a tickets map has no bomb sites, GPUs or exits")
	}
	ids := map[string]bool{}
	mains := map[string]int{}
	open := 0
	for i, p := range m.Points {
		if p.ID == "" || ids[p.ID] {
			bad("point %d: id %q is empty or taken", i, p.ID)
		}
		ids[p.ID] = true
		if p.Radius <= 0 {
			bad("point %s: radius %v", p.ID, p.Radius)
		}
		if p.Owner != "" && !slices.Contains(Teams, p.Owner) {
			bad("point %s: owner %q is not a team", p.ID, p.Owner)
		}
		if p.Locked {
			if p.Owner == "" || !p.Spawn {
				bad("point %s: a locked point is a team's main (owned, a spawn)", p.ID)
			}
			mains[p.Owner]++
		} else {
			open++
		}
		if p.Spawn && len(p.Spawns) == 0 {
			bad("point %s: a spawn point without spawn spots", p.ID)
		}
		for _, s := range p.Spawns {
			if !p.In(s) {
				bad("point %s: spawn spot %v outside it", p.ID, s)
			}
		}
	}
	for _, t := range Teams {
		if mains[t] != 1 {
			bad("%d mains for %s, want 1 (a locked spawn point it owns)", mains[t], t)
		}
	}
	if open == 0 {
		bad("no capturable point")
	}
	return out
}

// Race track rules (docs/racing.md).
const (
	MinCheckpoints = 3 // the start/finish line and two more: no cutting across
	MinGrid        = 2 // grid slots (Map.Cars), one car per racer
)

// raceProblems: a race map (Slop n Roll Racing, docs/racing.md) is rc_* (or
// a lab_* test map) in the Racing category, with no sites, GPUs, exits or
// capture points; its checkpoints (at least MinCheckpoints, the first the
// start/finish line) are real volumes with a respawn spot outside each, and
// it has a grid (Map.Cars, MinGrid or more) and a fall-off (a pit under the
// track).
func (m Map) raceProblems() []string {
	var out []string
	bad := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	lab := strings.HasPrefix(m.Name, "lab_")
	prefix, cat := "rc_", Racing
	if m.OnFoot {
		prefix, cat = "pk_", Parkour
	}
	if !strings.HasPrefix(m.Name, prefix) && !lab {
		bad("a %s map is named %s* (or lab_* for a test map)", cat, prefix)
	}
	if m.Category != cat && !lab {
		bad("category %q, want %q", m.Category, cat)
	}
	if len(m.Sites) > 0 || len(m.GPUs) > 0 || len(m.Exits) > 0 || len(m.Points) > 0 {
		bad("a race map has no bomb sites, GPUs, exits or capture points")
	}
	if len(m.Checkpoints) < MinCheckpoints {
		bad("%d checkpoints, want at least %d", len(m.Checkpoints), MinCheckpoints)
	}
	for i, c := range m.Checkpoints {
		for a := range 3 {
			if c.Min[a] >= c.Max[a] {
				bad("checkpoint %d: empty on axis %d", i, a)
			}
		}
	}
	if m.OnFoot {
		if len(m.TeamSpawns[Vibe]) == 0 || len(m.TeamSpawns[Artisan]) == 0 {
			bad("an on-foot course starts at the team spawns: both teams need some")
		}
	} else if len(m.Cars) < MinGrid {
		bad("%d grid slots (Map.Cars), want at least %d", len(m.Cars), MinGrid)
	} else if m.PointToPoint {
		bad("point to point is for on-foot courses (a car race laps its track)")
	}
	if len(m.Pits) == 0 {
		bad("no pit under the track: falling off must wreck the car (on foot: put you back)")
	}
	return out
}

// Objectives are the volumes a team must be able to walk to: the bomb sites,
// or in GPU Drop every GPU spot (its grab range) and every exit, or in
// tickets mode every capturable point (the box inside its circle).
func (m Map) Objectives() []Zone {
	if m.Mode == ModeTickets {
		var out []Zone
		for _, p := range m.Points {
			if p.Locked {
				continue
			}
			h := p.Radius / math.Sqrt2
			out = append(out, Zone{Name: p.ID, Min: [3]float64{p.Pos[0] - h, p.Pos[1] - 0.5, p.Pos[2] - h},
				Max: [3]float64{p.Pos[0] + h, p.Pos[1] + PointHeight, p.Pos[2] + h}})
		}
		return out
	}
	if m.Mode != ModeGPUDrop {
		return m.Sites
	}
	out := make([]Zone, 0, len(m.GPUs)+len(m.Exits))
	for i, g := range m.GPUs {
		p := g.Pos // where the grabber's feet may be (InGrabReach; a box, so the walk test can end in it)
		out = append(out, Zone{Name: fmt.Sprintf("gpu%d", i+1),
			Min: [3]float64{p[0] - GrabRange/math.Sqrt2, p[1] - ReachUp, p[2] - GrabRange/math.Sqrt2},
			Max: [3]float64{p[0] + GrabRange/math.Sqrt2, p[1] + ReachDown, p[2] + GrabRange/math.Sqrt2}})
	}
	return append(out, m.Exits...)
}

// Grab reach (grab and restock): within GrabRange metres across, the box no
// more than ReachUp above the feet (a shelf at about eye height) or ReachDown
// below them (on the floor of a step down).
const (
	GrabRange = 1.5
	ReachUp   = 1.7
	ReachDown = 0.6
)

// InGrabReach: a player standing at feet can grab or restock a GPU at gpu
// (its floor point). The server's grab code and the map tests share it.
func InGrabReach(feet, gpu [3]float64) bool {
	dy := gpu[1] - feet[1]
	return math.Hypot(gpu[0]-feet[0], gpu[2]-feet[2]) <= GrabRange && dy <= ReachUp && dy >= -ReachDown
}

// The boxed GPU's size (metres): GPUSize[0] x GPUSize[2] on the floor (along
// its yaw), GPUSize[1] tall: web/assets/models/gpu.js GPU_BOX {w, h, d}
// (gpudrop.test.mjs checks they match).
var GPUSize = [3]float64{0.62, 0.36, 0.2}

// GPUBox is the space a boxed GPU on its spot takes, any yaw (the footprint
// is the box's longest side squared, centred on the spot): the map tests
// keep it out of shelves and walls.
func GPUBox(g GPUSpot) Zone {
	h := math.Max(GPUSize[0], GPUSize[2]) / 2
	p := g.Pos
	return Zone{Min: [3]float64{p[0] - h, p[1], p[2] - h}, Max: [3]float64{p[0] + h, p[1] + GPUSize[1], p[2] + h}}
}
