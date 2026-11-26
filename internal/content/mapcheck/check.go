// Package mapcheck holds a map to the rules every built-in map passes: the
// checks a custom map (a file, --maps-dir; docs/custom-maps.md) is loaded
// with, and the walk (the real movement code along the nav graph) they use.
// Apart from internal/content/maps so that maps never imports internal/sim
// (whose tests use the maps).
package mapcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/navgen"
)

// Limits on a map from a file (custom maps): well past every built-in map,
// short of what would choke the server's collision or the client's renderer.
const (
	MaxBoxes   = 20000
	MaxLights  = 256
	MaxMovers  = 256
	MaxHints   = 256
	MaxExtent  = 2000.0 // metres from the origin, any axis
	maxNameLen = 40
)

var nameRe = regexp.MustCompile(`^[a-z0-9_]+$`)

// NameProblem is what is wrong with a map name ("" if nothing): 3-40
// characters of a-z, 0-9 and _ (it is a file name too).
func NameProblem(name string) string {
	if len(name) < 3 || len(name) > maxNameLen || !nameRe.MatchString(name) {
		return fmt.Sprintf("name %q: 3-%d characters of a-z, 0-9 and _", name, maxNameLen)
	}
	return ""
}

// Problems lists what keeps m from playing: the checks every built-in map
// passes (TestAllMapsAreValid, TestWalkToSites), so a map from a file is held
// to the same rules. weapon says whether an item's weapon id exists (nil: not
// checked). Empty means it plays.
func Problems(m c.Map, weapon func(string) bool) []string {
	var out []string
	bad := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	if p := NameProblem(m.Name); p != "" {
		bad("%s", p)
	}
	if len(m.Boxes) == 0 || len(m.Boxes) > MaxBoxes {
		bad("%d boxes (1-%d)", len(m.Boxes), MaxBoxes)
	}
	if len(m.Lights) > MaxLights || len(m.Movers) > MaxMovers {
		bad("%d lights, %d movers (at most %d, %d)", len(m.Lights), len(m.Movers), MaxLights, MaxMovers)
	}
	if len(out) > 0 {
		return out // nothing below is worth checking (or affordable)
	}
	materials := map[string]bool{}
	for _, k := range c.Materials {
		materials[k] = true
	}
	far := func(p [3]float64) bool {
		return math.Abs(p[0]) > MaxExtent || math.Abs(p[1]) > MaxExtent || math.Abs(p[2]) > MaxExtent
	}
	for i, b := range m.Boxes {
		if !materials[b.Kind] {
			bad("box %d: unknown material %q (materials: %v)", i, b.Kind, c.Materials)
		}
		for a := range 3 {
			if b.Min[a] >= b.Max[a] {
				bad("box %d is empty on axis %d: min %v max %v", i, a, b.Min, b.Max)
				break
			}
		}
		if far(b.Min) || far(b.Max) {
			bad("box %d is more than %.0f m out", i, MaxExtent)
		}
	}
	for i, w := range m.Water {
		if w.Min[0] >= w.Max[0] || w.Min[1] >= w.Max[1] || w.Min[2] >= w.Max[2] || far(w.Min) || far(w.Max) {
			bad("water %d: empty or too far out: min %v max %v", i, w.Min, w.Max)
		}
	}
	for i, z := range m.GravZones {
		if z.Min[0] >= z.Max[0] || z.Min[1] >= z.Max[1] || z.Min[2] >= z.Max[2] || far(z.Min) || far(z.Max) {
			bad("gravity zone %d (%s): empty or too far out: min %v max %v", i, z.Name, z.Min, z.Max)
		}
		if z.G < 0 || z.G > 4 || z.Jump < 0 || z.Jump > 4 {
			bad("gravity zone %d (%s): g %v jump %v (fractions of the world's, 0..4; g 0: space)", i, z.Name, z.G, z.Jump)
		}
	}
	for i, l := range m.Launchers {
		if l.Min[0] >= l.Max[0] || l.Min[1] >= l.Max[1] || l.Min[2] >= l.Max[2] || far(l.Min) || far(l.Max) {
			bad("launcher %d (%s): empty or too far out: min %v max %v", i, l.Name, l.Min, l.Max)
		}
		if sp := math.Sqrt(l.Vel[0]*l.Vel[0] + l.Vel[1]*l.Vel[1] + l.Vel[2]*l.Vel[2]); sp == 0 || sp > 100 {
			bad("launcher %d (%s): speed %.1f m/s (0 < speed <= 100)", i, l.Name, sp)
		}
	}
	if m.Sky != "" && m.Sky != c.SkySpace {
		bad("sky %q: \"\" (day) or %q", m.Sky, c.SkySpace)
	}
	for i, mv := range m.Movers {
		if mv.Kind == c.MoverSlide && mv.Period <= 0 {
			bad("mover %d: period %v (seconds per cycle, > 0)", i, mv.Period)
		}
	}
	for _, team := range c.Teams {
		if len(m.TeamSpawns[team]) == 0 {
			bad("no spawns for %s (teamSpawns)", team)
		}
	}
	if len(m.Spawns) == 0 {
		bad("no spawns")
	}
	for _, s := range m.Spawns {
		if Solid(m, s) {
			bad("spawn %v is inside a box", s)
		}
	}
	for _, team := range c.Teams {
		for _, s := range m.TeamSpawns[team] {
			in := false
			for _, z := range m.BuyZones[team] {
				in = in || z.Contains(s)
			}
			if !in {
				bad("%s spawn %v is outside its buy zones (buyZones)", team, s)
			}
		}
	}
	for _, it := range m.Items {
		if weapon != nil && !weapon(it.Weapon) {
			bad("item %q is not a weapon", it.Weapon)
		}
		if Solid(m, it.Pos) {
			bad("item at %v is inside a box", it.Pos)
		}
	}
	if len(m.NavHints) > MaxHints {
		bad("%d nav hints (at most %d)", len(m.NavHints), MaxHints)
	}
	for i, h := range m.NavHints {
		if far(h.Pos) || Solid(m, h.Pos) {
			bad("nav hint %d at %v: inside a box or too far out", i, h.Pos)
		}
	}
	for i, l := range m.Lights {
		if err := l.Check(); err != nil {
			bad("light %d: %v", i, err)
		}
	}
	for k := range m.Rules {
		if !slices.Contains(c.RuleCvars, k) {
			bad("rule %q: a map may pin only %v", k, c.RuleCvars)
		}
	}
	out = append(out, m.ModeProblems()...)
	if !slices.Contains(c.Categories, m.Category) {
		bad("category %q is not one of %v", m.Category, c.Categories)
	}
	if m.Desc == "" || m.MaxPlayers < 2 {
		bad("needs a desc and maxPlayers >= 2 (desc %q, maxPlayers %d)", m.Desc, m.MaxPlayers)
	}
	if m.Mode == c.ModeRace { // driven, not walked: no nav graph to walk (the track's drive test: internal/content/maps race_test.go)
		return out
	}
	if len(m.Nav) == 0 { // none in the file: the server builds one at load (internal/game autoNav); walk that
		m.Nav = navgen.Build(&m)
	}
	nodes := map[string]bool{}
	for _, n := range m.Nav {
		if nodes[n.Name] {
			bad("nav node %q twice", n.Name)
		}
		nodes[n.Name] = true
		if Solid(m, n.Pos) {
			bad("nav node %s %v is inside a box", n.Name, n.Pos)
		}
	}
	for _, n := range m.Nav {
		for _, l := range n.Links {
			if !nodes[l] {
				bad("nav node %s links to unknown %q", n.Name, l)
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	return append(out, WalkProblems(m)...)
}

// WalkProblems: every team walks from every one of its spawns to every
// objective (both sites; in GPU Drop every GPU and exit) along the nav graph,
// from its team's node ("vibe", "artisan"), with the real movement code: the
// map's pinned sv_movement, else both models (a server may run either). This
// is how bots get there.
func WalkProblems(m c.Map) []string {
	var out []string
	models := []string{"classic", "source"}
	if v := m.Rules["sv_movement"]; v != "" {
		models = []string{v}
	}
	for _, team := range c.Teams {
		for si, spawn := range m.TeamSpawns[team] {
			for _, site := range m.Objectives() {
				route := NavPath(m, team, site)
				if route == nil {
					out = append(out, fmt.Sprintf("no nav route from %q to %s (nav: a node named %q linked to one inside it)", team, site.Name, team))
					continue
				}
				for _, model := range models {
					end, err := WalkIn(m, spawn, route, MoveModels[model])
					if err != nil {
						out = append(out, fmt.Sprintf("%s movement: %s spawn %d -> %s: %v", model, team, si, site.Name, err))
					} else if !site.Contains(end) {
						out = append(out, fmt.Sprintf("%s movement: %s spawn %d -> %s: ended at %.2v, outside it", model, team, si, site.Name, end))
					}
				}
			}
		}
	}
	return out
}

// Solid: a standing player's hull at p overlaps a box.
func Solid(m c.Map, p [3]float64) bool {
	const r = 0.4 // player radius
	for _, b := range m.Boxes {
		if p[0]+r > b.Min[0] && p[0]-r < b.Max[0] && p[2]+r > b.Min[2] && p[2]-r < b.Max[2] &&
			p[1] < b.Max[1] && p[1]+1.8 > b.Min[1] {
			return true
		}
	}
	return false
}

// MaxFileBytes: a map file larger than this is refused unread.
const MaxFileBytes = 16 << 20

// Check is a parsed map's problems as a custom map from file: Problems, and
// it may not take a built-in map's or a lab's name (builtin: maps.Builtin),
// and its name is its file's (<name>.json).
func Check(m c.Map, file string, weapon, builtin func(string) bool) []string {
	var out []string
	if stem := strings.TrimSuffix(filepath.Base(file), ".json"); m.Name != stem {
		out = append(out, fmt.Sprintf("its name %q is not its file's (%s.json)", m.Name, stem))
	}
	if builtin != nil && builtin(m.Name) {
		out = append(out, fmt.Sprintf("%q is a built-in map's name", m.Name))
	}
	return append(out, Problems(m, weapon)...)
}

// LoadDir reads every *.json in dir as a custom map: the ones that pass
// Check, and why each other file was refused (by file name). A missing dir
// is no maps, not an error.
func LoadDir(dir string, weapon, builtin func(string) bool) (map[string]c.Map, map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, nil, err
	}
	ok, refused := map[string]c.Map{}, map[string]string{}
	for _, f := range files {
		base := filepath.Base(f)
		st, err := os.Stat(f)
		if err != nil {
			refused[base] = err.Error()
			continue
		}
		if st.Size() > MaxFileBytes {
			refused[base] = fmt.Sprintf("%d bytes (at most %d)", st.Size(), MaxFileBytes)
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			refused[base] = err.Error()
			continue
		}
		m, err := Parse(b)
		if err != nil {
			refused[base] = err.Error()
			continue
		}
		if p := Check(m, f, weapon, builtin); len(p) > 0 {
			if len(p) > 8 {
				p = append(p[:8], fmt.Sprintf("and %d more", len(p)-8))
			}
			refused[base] = strings.Join(p, "; ")
			continue
		}
		ok[m.Name] = m
	}
	return ok, refused, nil
}

// Parse reads one map file: exactly one Map object, no unknown fields (a
// typo is an error, not a silently missing wall).
func Parse(b []byte) (c.Map, error) {
	var m c.Map
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, fmt.Errorf("not a map: %w", err)
	}
	if d.More() {
		return m, fmt.Errorf("not a map: more after the map object")
	}
	return m, nil
}
