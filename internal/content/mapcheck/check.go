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
	"github.com/gigurra/counterslop/internal/sim"
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
	for i, pt := range m.Pits {
		if pt.Max[1] <= c.OutOfWorldY {
			bad("pit %d (%s) is all below y %v: a body there is out of the world (put back at a spawn) before it can fall in", i, pt.Name, c.OutOfWorldY)
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
		if l.Apex > 0 { // a jump pad: aimed at Target
			if far(l.Target) || l.Apex > 100 {
				bad("jump pad %d (%s): target %v too far out or apex %.1f m over 100", i, l.Name, l.Target, l.Apex)
			}
		} else if sp := math.Sqrt(l.Vel[0]*l.Vel[0] + l.Vel[1]*l.Vel[1] + l.Vel[2]*l.Vel[2]); sp == 0 || sp > 100 {
			bad("launcher %d (%s): speed %.1f m/s (0 < speed <= 100)", i, l.Name, sp)
		}
	}
	if !slices.Contains(c.Skies, m.Sky) {
		bad("sky %q: one of %q (\"\": the day sky)", m.Sky, c.Skies)
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
// objective (both sites; in GPU Drop every GPU and exit; in tickets the
// capture points) along the nav graph, from its team's node ("vibe",
// "artisan"), with the real movement code: the map's pinned sv_movement,
// else both models (a server may run either). This is how bots get there.
// An objective on another nav island (cq_moonshot's Moon) is reached by a
// departure (Crossings): the team walks to its door, and from where its drop
// pods land walks on to the objective.
func WalkProblems(m c.Map) []string {
	var out []string
	models := []string{"classic", "source"}
	if v := m.Rules["sv_movement"]; v != "" {
		models = []string{v}
	}
	walk := func(what string, start [3]float64, route []c.NavNode, z c.Zone) bool {
		ok := true
		for _, model := range models {
			end, err := WalkIn(m, start, route, MoveModels[model])
			if err != nil {
				out = append(out, fmt.Sprintf("%s movement: %s: %v", model, what, err))
				ok = false
			} else if !z.Contains(end) {
				out = append(out, fmt.Sprintf("%s movement: %s: ended at %.2v, outside it", model, what, end))
				ok = false
			}
		}
		return ok
	}
	crossings := Crossings(m)
	for _, team := range c.Teams {
		for si, spawn := range m.TeamSpawns[team] {
			for _, site := range m.Objectives() {
				what := fmt.Sprintf("%s spawn %d -> %s", team, si, site.Name)
				if route := NavPath(m, team, site); route != nil {
					walk(what, spawn, route, site)
					continue
				}
				crossed := false
				for _, cr := range crossings {
					toDoor, onward := NavPath(m, team, cr.Door), NavPath(m, cr.From, site)
					if toDoor == nil || onward == nil {
						continue
					}
					crossed = true
					if walk(what+" (to "+cr.Door.Name+")", spawn, toDoor, cr.Door) {
						walk(what+" (from "+cr.Door.Name+"'s drop)", cr.Land, onward, site)
					}
					break
				}
				if !crossed {
					out = append(out, fmt.Sprintf("no nav route from %q to %s (nav: a node named %q linked to one inside it; or a departure from its island landing on its)", team, site.Name, team))
				}
			}
		}
	}
	return out
}

// Crossing is a departure seen as a way across (Map.Departures): a zone round
// its door to walk to, where its drop pods put you down (Land: on the floor
// under the cabin when they fire) and the nav node nearest that (From).
type Crossing struct {
	Door c.Zone
	Land [3]float64
	From string
}

// Crossings: every departure with drop pods (an Air launcher on its mover)
// and a nav node within 8 m of where they land.
func Crossings(m c.Map) []Crossing {
	var out []Crossing
	for _, d := range m.Departures {
		if d.Mover < 1 || d.Mover > len(m.Movers) {
			continue
		}
		for _, l := range m.Launchers {
			if l.Mover != d.Mover || !l.Air {
				continue
			}
			o := sim.MoverOffset(&m.Movers[d.Mover-1], l.From)
			p := [3]float64{d.Board[0] + o[0], d.Board[1] + o[1], d.Board[2] + o[2]}
			floor := math.Inf(-1)
			for _, b := range m.Boxes {
				if p[0] >= b.Min[0] && p[0] <= b.Max[0] && p[2] >= b.Min[2] && p[2] <= b.Max[2] && b.Max[1] < p[1]-1 && b.Max[1] > floor {
					floor = b.Max[1]
				}
			}
			if math.IsInf(floor, -1) {
				continue
			}
			land := [3]float64{p[0], floor, p[2]}
			from, best := "", 8.0
			for _, n := range m.Nav {
				if dd := math.Hypot(n.Pos[0]-land[0], n.Pos[2]-land[2]); dd < best && math.Abs(n.Pos[1]-land[1]) < 1.5 {
					from, best = n.Name, dd
				}
			}
			if from == "" {
				continue
			}
			out = append(out, Crossing{Door: c.Zone{Name: d.Name + " door", Min: [3]float64{d.Door[0] - 1.5, d.Door[1] - 0.5, d.Door[2] - 1.5},
				Max: [3]float64{d.Door[0] + 1.5, d.Door[1] + 2, d.Door[2] + 1.5}}, Land: land, From: from})
			break
		}
	}
	return out
}

// Solid: a standing player's hull at p overlaps a box.
func Solid(m c.Map, p [3]float64) bool {
	const r = 0.4 // player radius
	for i := range m.Boxes {
		b := &m.Boxes[i]
		top := b.Max[1]
		if sim.IsWedge(b) { // a ramp: its slope where a hull at p stands on it
			top = sim.HullTop(b, p[0], p[2]) - 1e-6
		}
		if p[0]+r > b.Min[0] && p[0]-r < b.Max[0] && p[2]+r > b.Min[2] && p[2]-r < b.Max[2] &&
			p[1] < top && p[1]+1.8 > b.Min[1] {
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
