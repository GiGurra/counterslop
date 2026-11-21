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

// ModeProblems lists what makes m break its mode's rules: defusal maps are
// de_ with sites A and B; GPU Drop maps are gs_ in the GPU Drop category with
// stock, exits far apart and no sites. Empty means it plays.
func (m Map) ModeProblems() []string {
	var out []string
	bad := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	switch m.Mode {
	case ModeDefusal:
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
		if !strings.HasPrefix(m.Name, "de_") {
			bad("a defusal map is named de_*")
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
	default:
		bad("unknown mode %q", m.Mode)
	}
	return out
}

// Objectives are the volumes a team must be able to walk to: the bomb sites,
// or in GPU Drop every GPU spot (its grab range) and every exit.
func (m Map) Objectives() []Zone {
	if m.Mode != ModeGPUDrop {
		return m.Sites
	}
	out := make([]Zone, 0, len(m.GPUs)+len(m.Exits))
	for i, g := range m.GPUs {
		p := g.Pos
		out = append(out, Zone{Name: fmt.Sprintf("gpu%d", i+1),
			Min: [3]float64{p[0] - GrabRange, p[1] - 1, p[2] - GrabRange},
			Max: [3]float64{p[0] + GrabRange, p[1] + 1, p[2] + GrabRange}})
	}
	return append(out, m.Exits...)
}

// GrabRange is how near (metres) a player must stand to grab or restock a GPU.
const GrabRange = 1.5
