package game

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapcheck"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// The map editor's server side (docs/map-editor.md): one edit session at a
// time, on the map in play. Admins send typed edit ops (EditOp: rcon map_op,
// the in-game editor, the MCP); each is checked, compiled to patches on the
// map's JSON (mapPatch), applied on the server and broadcast as a map delta
// that every client applies the same way (web/game/mappatch.js). Undo and redo
// replay the inverse and the patches; every op is logged with its author.
// While editing, admins fly through walls (noclip) and bots stand still;
// playtest plays the map as it is. map_save checks it (mapcheck) and writes
// it to the maps dir, where it loads as a custom map.

// editSession is the map being edited.
type editSession struct {
	name     string // the name map_save writes by default
	rev      int    // edits applied so far: every snapshot carries it (meState.MR)
	undo     []editStep
	redo     []editStep
	log      []editLog
	playtest bool              // playing the map as it is (no noclip, bots on)
	navHand  []content.NavNode // the map's own nav while playtest plays on a built one (autoNav)

	presence      map[int]*editPresence // by editor: player id, or mcpEditorID (mapedit_presence.go)
	presenceDirty bool                  // changed since the last editors message
	presenceSent  int64                 // the tick of the last editors message
	mcpName       string                // the agent's name in the editors list
}

// editStep is one applied op: the patches that did it and the ones that undo it.
type editStep struct {
	author, what string
	do, undo     []mapPatch
}

// editLog is one line of the op log: what, by whom, at which revision.
type editLog struct {
	Rev    int    `json:"rev"`
	Author string `json:"author"`
	What   string `json:"what"`
}

// Limits on a session's history and log (an op beyond drops the oldest).
const (
	maxUndo    = 500
	maxEditLog = 2000
)

// mapPatch is one change to the map's JSON: at P (a field, or a field of a
// field: ["teamSpawns","vibe"]) set, insert or delete (A: set|ins|del) the
// array element I, the object key K, or (neither) the field itself, with the
// value V. The server and every client apply the same patches the same way
// (applyPatches; web/game/mappatch.js), so they hold the same map.
type mapPatch struct {
	P []string        `json:"p"`
	A string          `json:"a"`
	I *int            `json:"i,omitempty"`
	K string          `json:"k,omitempty"`
	V json.RawMessage `json:"v,omitempty"`
}

// EditOp is one typed edit (the editor's and the MCP's one API). Which
// fields an op reads is in editOpHelp.
type EditOp struct {
	Op     string       `json:"op"`
	I      int          `json:"i,omitempty"`      // the box, spawn, zone, item: its index
	Team   string       `json:"team,omitempty"`   // spawn and buy zone ops
	Name   string       `json:"name,omitempty"`   // a site (A, B), a pit, a nav node
	Kind   string       `json:"kind,omitempty"`   // a material (box ops); zone ops: site, buy, pit
	Box    *content.Box `json:"box,omitempty"`    // box_add, box_set
	Min    *[3]float64  `json:"min,omitempty"`    // box_resize, zone_set
	Max    *[3]float64  `json:"max,omitempty"`    //
	By     *[3]float64  `json:"by,omitempty"`     // box_move
	Pos    *[3]float64  `json:"pos,omitempty"`    // spawns, items, nav nodes
	Ramp   *int         `json:"ramp,omitempty"`   // box_ramp: 0 a box, 1-4 a wedge rising to +x -x +z -z
	Weapon string       `json:"weapon,omitempty"` // item ops
	Links  []string     `json:"links,omitempty"`  // nav_set
	Key    string       `json:"key,omitempty"`    // rule ops: the cvar
	Value  string       `json:"value,omitempty"`  //
	Desc   *string      `json:"desc,omitempty"`   // meta_set
	MaxPl  *int         `json:"maxPlayers,omitempty"`
	Cat    *string      `json:"category,omitempty"`
	Lane   *bool        `json:"lane,omitempty"` // hint_add, hint_lane
}

// editOpHelp lists the ops and the fields each reads (map_op help).
var editOpHelp = [][2]string{
	{"box_add", "box {min, max, kind, ramp}: a box or wedge (prints its index)"},
	{"box_set", "i, box: replace box i"},
	{"box_move", "i, by [dx,dy,dz]"},
	{"box_resize", "i, min, max"},
	{"box_material", "i, kind (a material)"},
	{"box_ramp", "i, ramp (0 a box; 1-4 a wedge rising to +x -x +z -z)"},
	{"box_delete", "i"},
	{"spawn_add", "team, pos"},
	{"spawn_move", "team, i, pos"},
	{"spawn_delete", "team, i"},
	{"zone_set", "kind site|buy|pit, name (site A|B, pit) or team + i (buy; i = the count adds one), min, max"},
	{"zone_delete", "kind, name or team + i"},
	{"item_add", "weapon, pos"},
	{"item_move", "i, pos"},
	{"item_delete", "i"},
	{"hint_add", "pos, lane (optional): a point a generated nav graph passes through (docs/navgen.md)"},
	{"hint_move", "i, pos"},
	{"hint_lane", "i, lane (true: an attacker bot's route choice)"},
	{"hint_delete", "i"},
	{"nav_set", "name, pos, links: add or replace a nav node (a hand-made graph's)"},
	{"nav_delete", "name (and every link to it)"},
	{"rule_set", "key (a map rule cvar), value"},
	{"rule_delete", "key"},
	{"meta_set", "desc, maxPlayers, category (any of them)"},
}

// editing: a session is on and not playtesting (admins fly, bots stand).
func (s *Server) editing() bool { return s.edit != nil && !s.edit.playtest }

// noclipFor: p flies (sim.Body.Noclip): an admin while the map is edited.
func (s *Server) noclipFor(p *Player) bool {
	return s.editing() && !p.Bot && s.isAdmin(p)
}

// refitNoclip sets every body's noclip to what noclipFor says (the session
// started, stopped, playtest toggled, someone logged in).
func (s *Server) refitNoclip() {
	for _, p := range s.players {
		if p.body != nil {
			p.body.Noclip = s.noclipFor(p)
		}
	}
}

// blankMap is map_edit new: a floor, a spawn and a buy zone a side, and the
// two sites to move where they belong.
func blankMap(name string) content.Map {
	b := func(x0, y0, z0, x1, y1, z1 float64, k string) content.Box {
		return content.B(x0, y0, z0, x1, y1, z1, k)
	}
	return content.Map{
		Name:   name,
		Boxes:  []content.Box{b(-32, -1, -32, 32, 0, 32, content.Concrete)},
		Spawns: [][3]float64{{-24, 0, 0}, {24, 0, 0}},
		TeamSpawns: map[string][][3]float64{
			content.Vibe: {{-24, 0, 0}}, content.Artisan: {{24, 0, 0}},
		},
		BuyZones: map[string][]content.Zone{
			content.Vibe:    {{Name: content.Vibe, Min: [3]float64{-28, -0.5, -4}, Max: [3]float64{-20, 3, 4}}},
			content.Artisan: {{Name: content.Artisan, Min: [3]float64{20, -0.5, -4}, Max: [3]float64{28, 3, 4}}},
		},
		Sites: []content.Zone{
			{Name: "A", Min: [3]float64{8, -0.5, 8}, Max: [3]float64{16, 3, 16}},
			{Name: "B", Min: [3]float64{8, -0.5, -16}, Max: [3]float64{16, 3, -8}},
		},
		Desc:       "A map in the making.",
		MaxPlayers: 10,
		Category:   content.Training,
	}
}

// editStart: map_edit new <name> | clone <map> [<name>].
func (s *Server) editStart(who string, args []string) (string, error) {
	if len(args) < 2 || args[0] != "new" && args[0] != "clone" {
		return "", fmt.Errorf("map_edit new <name> | clone <map> [<name>] | stop | log [n]")
	}
	var m content.Map
	name := args[1]
	if args[0] == "clone" {
		build := maps.Find(args[1])
		if build == nil {
			return "", fmt.Errorf("no map %q", args[1])
		}
		m = build()
		name = args[1] + "_edit"
		if maps.Builtin(args[1]) && len(args) < 3 {
			name = strings.Replace(args[1], "_", "_my", 1) // de_slop -> de_myslop: a built-in's name is the built-in's
		}
		if len(args) > 2 {
			name = args[2]
		}
	} else {
		m = blankMap(name)
	}
	if p := mapcheck.NameProblem(name); p != "" {
		return "", fmt.Errorf("%s", p)
	}
	if maps.Builtin(name) {
		return "", fmt.Errorf("%q is a built-in map's name: pick another (map_edit clone %s <name>)", name, args[1])
	}
	if s.edit != nil {
		s.announce(fmt.Sprintf("map editing of %s ended (unsaved edits are gone)", s.edit.name))
	}
	m.Name = name
	s.edit = nil
	s.loadMap(m, false)
	s.edit = &editSession{name: name, mcpName: "mcp"}
	s.editLog(who, args[0]+" "+strings.Join(args[1:], " "))
	s.refitNoclip()
	s.announce(fmt.Sprintf("%s is editing %s (map_op, map_undo, map_redo, map_playtest, map_save)", who, name))
	return fmt.Sprintf("editing %s, rev 0", name), nil
}

// editStop: map_edit stop: the map stays in play as it is; editing ends.
func (s *Server) editStop(who string) (string, error) {
	if s.edit == nil {
		return "", fmt.Errorf("no map is being edited")
	}
	name := s.edit.name
	s.handNavBack()
	s.edit = nil
	s.sendEditors() // nobody is editing now
	s.refitNoclip()
	s.announce(fmt.Sprintf("%s stopped editing %s", who, name))
	return "stopped", nil
}

func (s *Server) editLog(who, what string) {
	e := s.edit
	e.log = append(e.log, editLog{Rev: e.rev, Author: who, What: what})
	if len(e.log) > maxEditLog {
		e.log = e.log[len(e.log)-maxEditLog:]
	}
}

// editOp checks op, applies it and broadcasts it. Returns what to print.
// Callers hold s.mu.
func (s *Server) editOp(who string, op EditOp) (string, error) {
	if s.edit == nil {
		return "", fmt.Errorf("no map is being edited (map_edit new|clone)")
	}
	if s.edit.playtest {
		return "", fmt.Errorf("playtesting: map_playtest 0 first")
	}
	do, out, err := s.compileOp(op)
	if err != nil {
		return "", fmt.Errorf("%s: %v", op.Op, err)
	}
	undo, err := s.applyPatches(do)
	if err != nil {
		return "", fmt.Errorf("%s: %v", op.Op, err)
	}
	e := s.edit
	what, _ := json.Marshal(op)
	e.undo = append(e.undo, editStep{author: who, what: string(what), do: do, undo: undo})
	if len(e.undo) > maxUndo {
		e.undo = e.undo[1:]
	}
	e.redo = nil
	s.editLog(who, string(what))
	s.sendDelta(do)
	return fmt.Sprintf("%srev %d", out, e.rev), nil
}

// editUndo and editRedo step back and forward through the ops (anyone's).
func (s *Server) editUndo(who string, redo bool) (string, error) {
	e := s.edit
	if e == nil {
		return "", fmt.Errorf("no map is being edited")
	}
	if e.playtest {
		return "", fmt.Errorf("playtesting: map_playtest 0 first")
	}
	from, to := &e.undo, &e.redo
	verb := "undo"
	if redo {
		from, to, verb = &e.redo, &e.undo, "redo"
	}
	if len(*from) == 0 {
		return "", fmt.Errorf("nothing to %s", verb)
	}
	st := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	ps := st.undo
	if redo {
		ps = st.do
	}
	if _, err := s.applyPatches(ps); err != nil {
		return "", err
	}
	*to = append(*to, st)
	s.editLog(who, verb+" "+st.what)
	s.sendDelta(ps)
	return fmt.Sprintf("%s %s (by %s), rev %d", verb, st.what, st.author, e.rev), nil
}

// sendDelta tells everyone the patches of revision s.edit.rev. A client that
// misses one sees the revision in its snapshot move past its own and asks for
// the whole map (mapget): the delta is an event, the revision is state.
func (s *Server) sendDelta(ps []mapPatch) {
	b, _ := json.Marshal(map[string]any{"t": "mapdelta", "rev": s.edit.rev, "patch": ps})
	for _, p := range s.players {
		trySend(p, b)
	}
}

// mapRev is the map's edit revision (0: not edited): in every self-snapshot.
func (s *Server) mapRev() int {
	if s.edit == nil {
		return 0
	}
	return s.edit.rev
}

// sendMapSync is the whole map at its revision to p (it asked: mapget).
func (s *Server) sendMapSync(p *Player) {
	b, _ := json.Marshal(map[string]any{"t": "mapsync", "rev": s.mapRev(), "map": s.Map})
	trySend(p, b)
}

// applyPatches applies ps to the map in play, all or none, and returns the
// patches that undo them. The edit revision moves on.
func (s *Server) applyPatches(ps []mapPatch) ([]mapPatch, error) {
	b, err := json.Marshal(s.Map)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	undo := make([]mapPatch, 0, len(ps))
	for _, p := range ps {
		u, err := patchDoc(doc, p)
		if err != nil {
			return nil, err
		}
		undo = append(undo, u)
	}
	slices.Reverse(undo)
	nb, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var m content.Map
	if err := json.Unmarshal(nb, &m); err != nil {
		return nil, err
	}
	s.Map = m
	s.gridCache = nil
	s.edit.rev++
	s.remapPresence(ps) // selections stay on what they were on
	return undo, nil
}

// patchDoc applies one patch to a map's JSON (decoded: objects, arrays,
// float64) and returns its inverse. Missing objects and arrays on the path
// are made (an empty buy zone list, no rules yet).
func patchDoc(doc map[string]any, p mapPatch) (mapPatch, error) {
	if len(p.P) == 0 {
		return mapPatch{}, fmt.Errorf("patch without a path")
	}
	var v any
	if len(p.V) > 0 {
		if err := json.Unmarshal(p.V, &v); err != nil {
			return mapPatch{}, err
		}
	}
	parent := doc
	for _, k := range p.P[:len(p.P)-1] {
		next, ok := parent[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			parent[k] = next
		}
		parent = next
	}
	field := p.P[len(p.P)-1]
	inv := mapPatch{P: p.P}
	raw := func(x any) json.RawMessage { b, _ := json.Marshal(x); return b }
	switch {
	case p.I != nil:
		arr, _ := parent[field].([]any)
		i := *p.I
		inv.I = p.I
		switch p.A {
		case "set":
			if i < 0 || i >= len(arr) {
				return inv, fmt.Errorf("%s[%d]: out of range (%d)", field, i, len(arr))
			}
			inv.A, inv.V = "set", raw(arr[i])
			arr[i] = v
		case "ins":
			if i < 0 || i > len(arr) {
				return inv, fmt.Errorf("%s[%d]: out of range (%d)", field, i, len(arr))
			}
			inv.A = "del"
			arr = slices.Insert(arr, i, v)
		case "del":
			if i < 0 || i >= len(arr) {
				return inv, fmt.Errorf("%s[%d]: out of range (%d)", field, i, len(arr))
			}
			inv.A, inv.V = "ins", raw(arr[i])
			arr = slices.Delete(arr, i, i+1)
		default:
			return inv, fmt.Errorf("patch action %q", p.A)
		}
		parent[field] = arr
	case p.K != "":
		obj, ok := parent[field].(map[string]any)
		if !ok {
			obj = map[string]any{}
			parent[field] = obj
		}
		inv.K = p.K
		old, had := obj[p.K]
		if had {
			inv.A, inv.V = "set", raw(old)
		} else {
			inv.A = "del"
		}
		switch p.A {
		case "set":
			obj[p.K] = v
		case "del":
			delete(obj, p.K)
		default:
			return inv, fmt.Errorf("patch action %q", p.A)
		}
	default:
		old, had := parent[field]
		if had {
			inv.A, inv.V = "set", raw(old)
		} else {
			inv.A = "del"
		}
		switch p.A {
		case "set":
			parent[field] = v
		case "del":
			delete(parent, field)
		default:
			return inv, fmt.Errorf("patch action %q", p.A)
		}
	}
	return inv, nil
}

// compileOp checks op against the map in play and turns it into patches.
// out is printed before the revision (box_add: the new box's index).
func (s *Server) compileOp(op EditOp) (ps []mapPatch, out string, err error) {
	m := &s.Map
	raw := func(x any) json.RawMessage { b, _ := json.Marshal(x); return b }
	idx := func(i int) *int { return &i }
	at := func(p []string, a string, i int, v any) mapPatch {
		mp := mapPatch{P: p, A: a, I: idx(i)}
		if v != nil {
			mp.V = raw(v)
		}
		return mp
	}
	boxAt := func() (content.Box, error) {
		if op.I < 0 || op.I >= len(m.Boxes) {
			return content.Box{}, fmt.Errorf("no box %d (%d boxes)", op.I, len(m.Boxes))
		}
		return m.Boxes[op.I], nil
	}
	setBox := func(b content.Box) ([]mapPatch, string, error) {
		if err := checkBox(b); err != nil {
			return nil, "", err
		}
		return []mapPatch{at([]string{"boxes"}, "set", op.I, b)}, "", nil
	}
	team := func() error {
		if op.Team != content.Vibe && op.Team != content.Artisan {
			return fmt.Errorf("team %q (vibe or artisan)", op.Team)
		}
		return nil
	}
	pos := func() ([3]float64, error) {
		if op.Pos == nil {
			return [3]float64{}, fmt.Errorf("pos missing")
		}
		return *op.Pos, checkPoint(*op.Pos)
	}
	// Spawns is every team spawn (vibe's, then artisan's): set it whole after a spawn op.
	spawnsAfter := func(t string, list [][3]float64) mapPatch {
		var all [][3]float64
		for _, tm := range content.Teams {
			l := m.TeamSpawns[tm]
			if tm == t {
				l = list
			}
			all = append(all, l...)
		}
		return mapPatch{P: []string{"spawns"}, A: "set", V: raw(all)}
	}
	switch op.Op {
	case "box_add":
		if op.Box == nil {
			return nil, "", fmt.Errorf("box missing")
		}
		if len(m.Boxes) >= mapcheck.MaxBoxes {
			return nil, "", fmt.Errorf("%d boxes already (at most %d)", len(m.Boxes), mapcheck.MaxBoxes)
		}
		if err := checkBox(*op.Box); err != nil {
			return nil, "", err
		}
		return []mapPatch{at([]string{"boxes"}, "ins", len(m.Boxes), *op.Box)}, fmt.Sprintf("box %d, ", len(m.Boxes)), nil
	case "box_set":
		if _, err := boxAt(); err != nil {
			return nil, "", err
		}
		if op.Box == nil {
			return nil, "", fmt.Errorf("box missing")
		}
		return setBox(*op.Box)
	case "box_move":
		b, err := boxAt()
		if err != nil {
			return nil, "", err
		}
		if op.By == nil {
			return nil, "", fmt.Errorf("by missing")
		}
		for a := range 3 {
			b.Min[a] += op.By[a]
			b.Max[a] += op.By[a]
		}
		return setBox(b)
	case "box_resize":
		b, err := boxAt()
		if err != nil {
			return nil, "", err
		}
		if op.Min == nil || op.Max == nil {
			return nil, "", fmt.Errorf("min and max")
		}
		b.Min, b.Max = *op.Min, *op.Max
		return setBox(b)
	case "box_material":
		b, err := boxAt()
		if err != nil {
			return nil, "", err
		}
		b.Kind = op.Kind
		return setBox(b)
	case "box_ramp":
		b, err := boxAt()
		if err != nil {
			return nil, "", err
		}
		if op.Ramp == nil {
			return nil, "", fmt.Errorf("ramp missing")
		}
		b.Ramp = *op.Ramp
		return setBox(b)
	case "box_delete":
		if _, err := boxAt(); err != nil {
			return nil, "", err
		}
		return []mapPatch{at([]string{"boxes"}, "del", op.I, nil)}, "", nil
	case "spawn_add", "spawn_move", "spawn_delete":
		if err := team(); err != nil {
			return nil, "", err
		}
		list := slices.Clone(m.TeamSpawns[op.Team])
		path := []string{"teamSpawns", op.Team}
		if op.Op != "spawn_add" && (op.I < 0 || op.I >= len(list)) {
			return nil, "", fmt.Errorf("no %s spawn %d (%d)", op.Team, op.I, len(list))
		}
		var p mapPatch
		switch op.Op {
		case "spawn_add", "spawn_move":
			q, err := pos()
			if err != nil {
				return nil, "", err
			}
			if op.Op == "spawn_add" {
				if len(list) >= 32 {
					return nil, "", fmt.Errorf("32 %s spawns already", op.Team)
				}
				p, list = at(path, "ins", len(list), q), append(list, q)
				out = fmt.Sprintf("%s spawn %d, ", op.Team, len(list)-1)
			} else {
				p, list[op.I] = at(path, "set", op.I, q), q
			}
		default:
			if len(list) == 1 {
				return nil, "", fmt.Errorf("a team keeps one spawn")
			}
			p, list = at(path, "del", op.I, nil), slices.Delete(list, op.I, op.I+1)
		}
		return []mapPatch{p, spawnsAfter(op.Team, list)}, out, nil
	case "zone_set", "zone_delete":
		return compileZoneOp(m, op)
	case "item_add", "item_move", "item_delete":
		if op.Op != "item_add" && (op.I < 0 || op.I >= len(m.Items)) {
			return nil, "", fmt.Errorf("no item %d (%d)", op.I, len(m.Items))
		}
		switch op.Op {
		case "item_delete":
			return []mapPatch{at([]string{"items"}, "del", op.I, nil)}, "", nil
		case "item_move":
			q, err := pos()
			if err != nil {
				return nil, "", err
			}
			it := m.Items[op.I]
			it.Pos = q
			return []mapPatch{at([]string{"items"}, "set", op.I, it)}, "", nil
		}
		q, err := pos()
		if err != nil {
			return nil, "", err
		}
		if s.Weapons == nil || s.Weapons.Get(op.Weapon) == nil {
			return nil, "", fmt.Errorf("no weapon %q", op.Weapon)
		}
		if len(m.Items) >= 256 {
			return nil, "", fmt.Errorf("256 items already")
		}
		return []mapPatch{at([]string{"items"}, "ins", len(m.Items), content.Item{Weapon: op.Weapon, Pos: q})}, fmt.Sprintf("item %d, ", len(m.Items)), nil
	case "hint_add", "hint_move", "hint_lane", "hint_delete":
		if op.Op != "hint_add" && (op.I < 0 || op.I >= len(m.NavHints)) {
			return nil, "", fmt.Errorf("no nav hint %d (%d)", op.I, len(m.NavHints))
		}
		path := []string{"navHints"}
		switch op.Op {
		case "hint_delete":
			return []mapPatch{at(path, "del", op.I, nil)}, "", nil
		case "hint_lane":
			if op.Lane == nil {
				return nil, "", fmt.Errorf("hint_lane needs lane (true or false)")
			}
			h := m.NavHints[op.I]
			h.Lane = *op.Lane
			return []mapPatch{at(path, "set", op.I, h)}, "", nil
		}
		q, err := pos()
		if err != nil {
			return nil, "", err
		}
		if op.Op == "hint_move" {
			h := m.NavHints[op.I]
			h.Pos = q
			return []mapPatch{at(path, "set", op.I, h)}, "", nil
		}
		if len(m.NavHints) >= mapcheck.MaxHints {
			return nil, "", fmt.Errorf("%d nav hints already", mapcheck.MaxHints)
		}
		return []mapPatch{at(path, "ins", len(m.NavHints), content.NavHint{Pos: q, Lane: op.Lane != nil && *op.Lane})}, fmt.Sprintf("nav hint %d, ", len(m.NavHints)), nil
	case "nav_set":
		if op.Name == "" || len(op.Name) > 32 {
			return nil, "", fmt.Errorf("a nav node needs a name (32 characters at most)")
		}
		q, err := pos()
		if err != nil {
			return nil, "", err
		}
		names := map[string]bool{op.Name: true}
		for _, n := range m.Nav {
			names[n.Name] = true
		}
		for _, l := range op.Links {
			if !names[l] || l == op.Name {
				return nil, "", fmt.Errorf("link to %q: no such node", l)
			}
		}
		n := content.NavNode{Name: op.Name, Pos: q, Links: op.Links}
		for i, o := range m.Nav {
			if o.Name == op.Name {
				n.Lane = o.Lane
				return []mapPatch{at([]string{"nav"}, "set", i, n)}, "", nil
			}
		}
		if len(m.Nav) >= 1024 {
			return nil, "", fmt.Errorf("1024 nav nodes already")
		}
		return []mapPatch{at([]string{"nav"}, "ins", len(m.Nav), n)}, "", nil
	case "nav_delete":
		i := slices.IndexFunc(m.Nav, func(n content.NavNode) bool { return n.Name == op.Name })
		if i < 0 {
			return nil, "", fmt.Errorf("no nav node %q", op.Name)
		}
		// The links to it first (indices before the delete), then the node.
		for j, n := range m.Nav {
			if j != i && slices.Contains(n.Links, op.Name) {
				n.Links = slices.DeleteFunc(slices.Clone(n.Links), func(l string) bool { return l == op.Name })
				ps = append(ps, at([]string{"nav"}, "set", j, n))
			}
		}
		return append(ps, at([]string{"nav"}, "del", i, nil)), "", nil
	case "rule_set":
		if !slices.Contains(content.RuleCvars, op.Key) {
			return nil, "", fmt.Errorf("a map may pin only %s", strings.Join(content.RuleCvars, " "))
		}
		v, err := cvarValue(op.Key, op.Value)
		if err != nil {
			return nil, "", err
		}
		return []mapPatch{{P: []string{"rules"}, A: "set", K: op.Key, V: raw(v)}}, "", nil
	case "rule_delete":
		if _, ok := m.Rules[op.Key]; !ok {
			return nil, "", fmt.Errorf("the map doesn't pin %q", op.Key)
		}
		return []mapPatch{{P: []string{"rules"}, A: "del", K: op.Key}}, "", nil
	case "meta_set":
		if op.Desc != nil {
			if len(*op.Desc) > 400 {
				return nil, "", fmt.Errorf("desc: 400 characters at most")
			}
			ps = append(ps, mapPatch{P: []string{"desc"}, A: "set", V: raw(*op.Desc)})
		}
		if op.MaxPl != nil {
			if *op.MaxPl < 2 || *op.MaxPl > 32 {
				return nil, "", fmt.Errorf("maxPlayers 2-32")
			}
			ps = append(ps, mapPatch{P: []string{"maxPlayers"}, A: "set", V: raw(*op.MaxPl)})
		}
		if op.Cat != nil {
			if !slices.Contains(content.Categories, *op.Cat) {
				return nil, "", fmt.Errorf("category %q (%s)", *op.Cat, strings.Join(content.Categories, " "))
			}
			ps = append(ps, mapPatch{P: []string{"category"}, A: "set", V: raw(*op.Cat)})
		}
		if len(ps) == 0 {
			return nil, "", fmt.Errorf("desc, maxPlayers or category")
		}
		return ps, "", nil
	}
	return nil, "", fmt.Errorf("unknown op (map_op help lists them)")
}

// compileZoneOp: zone_set and zone_delete on sites (A, B), buy zones (per
// team, by index) and pits (by name).
func compileZoneOp(m *content.Map, op EditOp) ([]mapPatch, string, error) {
	raw := func(x any) json.RawMessage { b, _ := json.Marshal(x); return b }
	var path []string
	var list []content.Zone
	name := op.Name
	i := -1
	switch op.Kind {
	case "site":
		if name != "A" && name != "B" {
			return nil, "", fmt.Errorf("sites are A and B")
		}
		path, list = []string{"sites"}, m.Sites
	case "pit":
		if name == "" || len(name) > 32 {
			return nil, "", fmt.Errorf("a pit needs a name (the kill feed's: merged into <name>)")
		}
		path, list = []string{"pits"}, m.Pits
	case "buy":
		if op.Team != content.Vibe && op.Team != content.Artisan {
			return nil, "", fmt.Errorf("team %q (vibe or artisan)", op.Team)
		}
		path, list, name, i = []string{"buyZones", op.Team}, m.BuyZones[op.Team], op.Team, op.I
		if i < 0 || i > len(list) || op.Op == "zone_delete" && i == len(list) {
			return nil, "", fmt.Errorf("no %s buy zone %d (%d)", op.Team, i, len(list))
		}
	default:
		return nil, "", fmt.Errorf("zone kind %q (site, buy, pit)", op.Kind)
	}
	if op.Kind != "buy" {
		i = slices.IndexFunc(list, func(z content.Zone) bool { return z.Name == name })
	}
	if op.Op == "zone_delete" {
		if i < 0 {
			return nil, "", fmt.Errorf("no %s %q", op.Kind, name)
		}
		return []mapPatch{{P: path, A: "del", I: &i}}, "", nil
	}
	if op.Min == nil || op.Max == nil {
		return nil, "", fmt.Errorf("min and max")
	}
	z := content.Zone{Name: name, Min: *op.Min, Max: *op.Max}
	if err := checkBox(content.Box{Min: z.Min, Max: z.Max, Kind: content.Concrete}); err != nil {
		return nil, "", err
	}
	if i < 0 || i == len(list) {
		n := len(list)
		if n >= 64 {
			return nil, "", fmt.Errorf("64 already")
		}
		return []mapPatch{{P: path, A: "ins", I: &n, V: raw(z)}}, "", nil
	}
	return []mapPatch{{P: path, A: "set", I: &i, V: raw(z)}}, "", nil
}

// checkBox: a box an op may put in the map (known material, not empty, in
// range, a ramp 0-4).
func checkBox(b content.Box) error {
	if !slices.Contains(content.Materials, b.Kind) {
		return fmt.Errorf("material %q (%s)", b.Kind, strings.Join(content.Materials, " "))
	}
	for a := range 3 {
		if !(b.Min[a] < b.Max[a]) {
			return fmt.Errorf("empty on axis %d: min %v max %v", a, b.Min, b.Max)
		}
	}
	if err := checkPoint(b.Min); err != nil {
		return err
	}
	if err := checkPoint(b.Max); err != nil {
		return err
	}
	if b.Ramp < 0 || b.Ramp > content.RampNZ {
		return fmt.Errorf("ramp %d (0 a box, 1-4 a wedge)", b.Ramp)
	}
	return nil
}

// checkPoint: a point inside the editable world (mapcheck.MaxExtent).
func checkPoint(p [3]float64) error {
	for _, v := range p {
		if math.IsNaN(v) || math.Abs(v) > mapcheck.MaxExtent {
			return fmt.Errorf("%v: more than %.0f m out", p, mapcheck.MaxExtent)
		}
	}
	return nil
}

// editSave: map_save [name]: the map in play, checked as a custom map file
// would be (mapcheck), written to the maps dir as <name>.json, then loaded
// with the others (OnMapSaved).
func (s *Server) editSave(who string, args []string) (string, error) {
	if s.edit == nil {
		return "", fmt.Errorf("no map is being edited")
	}
	if s.MapsDir == "" {
		return "", fmt.Errorf("the server has no maps dir (--maps-dir): nowhere to save")
	}
	name := s.edit.name
	if len(args) > 0 {
		name = args[0]
	}
	file := filepath.Join(s.MapsDir, name+".json")
	m, p := s.editCheck(name, file)
	if len(p) > 0 {
		if len(p) > 8 {
			p = append(p[:8], fmt.Sprintf("and %d more", len(p)-8))
		}
		return "", fmt.Errorf("not saved: %s", strings.Join(p, "; "))
	}
	b, err := maps.Export(m)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(file, b, 0o644); err != nil {
		return "", err
	}
	s.edit.name = name
	s.editLog(who, "save "+name)
	s.announce(fmt.Sprintf("%s saved %s (%d boxes)", who, name, len(m.Boxes)))
	if s.OnMapSaved != nil {
		go s.OnMapSaved() // takes s.mu (MapsReloaded): not from under it
	}
	return "saved " + file, nil
}

// editCheck: the map being edited as it would be saved under name (to
// file), and what --maps-dir would refuse it for; none: it is fine.
// Callers hold s.mu.
func (s *Server) editCheck(name, file string) (content.Map, []string) {
	m := s.editSaved(name)
	return m, s.checkSaved(m, file)
}

// editSaved: the map being edited as it would be saved under name (the
// map's own nav, not playtest's). Callers hold s.mu.
func (s *Server) editSaved(name string) content.Map {
	m := s.Map
	m.Name = name
	if s.navAuto {
		m.Nav = s.edit.navHand // playtest's graph is not the map's
	}
	return m
}

// checkSaved: what --maps-dir would refuse m (from file) for (mapcheck.Check:
// the walks take a while; needs no lock).
func (s *Server) checkSaved(m content.Map, file string) []string {
	weapon := func(id string) bool { return s.Weapons != nil && s.Weapons.Get(id) != nil }
	return mapcheck.Check(m, file, weapon, maps.Builtin)
}

// editPlaytest: map_playtest [0|1]: play the map as it is (bots on, nobody
// flies, a fresh match), or back to editing.
func (s *Server) editPlaytest(who string, args []string) (string, error) {
	e := s.edit
	if e == nil {
		return "", fmt.Errorf("no map is being edited")
	}
	on := !e.playtest
	if len(args) > 0 {
		on = args[0] == "1" || args[0] == "on"
	}
	if on == e.playtest {
		return fmt.Sprintf("playtest already %v", on), nil
	}
	e.playtest = on
	if on {
		// A map without nav (or bot_autonav 1) plays on a graph built from its
		// geometry (autoNav, Custom maps 2): the server's alone (clients don't
		// walk it), dropped when editing goes on; edits wait until then.
		e.navHand = s.Map.Nav
		s.autoNav()
	} else {
		s.handNavBack()
	}
	s.gridCache = nil
	s.editLog(who, "playtest "+strconv.FormatBool(on))
	s.refitNoclip()
	if on {
		s.restart() // everyone on a spawn, a fresh match
		s.refitNoclip()
		s.announce(fmt.Sprintf("%s: playtesting %s", who, e.name))
		return "playtest on", nil
	}
	s.announce(fmt.Sprintf("%s: back to editing %s", who, e.name))
	return "playtest off", nil
}

// mapEditCommand runs the editor's rcon commands. Callers hold s.mu.
func (s *Server) mapEditCommand(p *Player, cmd string, args []string) {
	var out string
	var err error
	who := p.Name
	switch cmd {
	case "map_edit":
		switch {
		case len(args) == 0:
			out = s.editStatus()
		case args[0] == "stop":
			out, err = s.editStop(who)
		case args[0] == "log":
			out, err = s.editLogText(args[1:])
		default:
			out, err = s.editStart(who, args)
		}
	case "map_op":
		line := strings.TrimSpace(strings.Join(args, " "))
		if line == "" || line == "help" {
			var sb strings.Builder
			sb.WriteString(`map_op {"op": ..., fields}: one edit (undo: map_undo)`)
			for _, h := range editOpHelp {
				fmt.Fprintf(&sb, "\n  %-13s %s", h[0], h[1])
			}
			out = sb.String()
			break
		}
		var op EditOp
		d := json.NewDecoder(strings.NewReader(line))
		d.DisallowUnknownFields()
		if err = d.Decode(&op); err != nil {
			err = fmt.Errorf("map_op wants one JSON op (map_op help): %v", err)
			break
		}
		out, err = s.editOp(who, op)
	case "map_undo":
		out, err = s.editUndo(who, false)
	case "map_redo":
		out, err = s.editUndo(who, true)
	case "map_save":
		out, err = s.editSave(who, args)
	case "map_playtest":
		out, err = s.editPlaytest(who, args)
	}
	if err != nil {
		out = cmd + ": " + err.Error()
	}
	s.print(p, out)
}

func (s *Server) editStatus() string {
	e := s.edit
	if e == nil {
		return "no map is being edited (map_edit new <name> | clone <map> [<name>])"
	}
	return fmt.Sprintf("editing %s: rev %d, %d boxes, %d undo, %d redo, playtest %v", e.name, e.rev, len(s.Map.Boxes), len(e.undo), len(e.redo), e.playtest)
}

func (s *Server) editLogText(args []string) (string, error) {
	e := s.edit
	if e == nil {
		return "", fmt.Errorf("no map is being edited")
	}
	n := 20
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	l := e.log[max(0, len(e.log)-n):]
	lines := make([]string, len(l))
	for i, x := range l {
		lines[i] = fmt.Sprintf("rev %d %s: %s", x.Rev, x.Author, x.What)
	}
	return strings.Join(lines, "\n"), nil
}

// handNavBack: after playtest on a built nav graph, the map's own again.
func (s *Server) handNavBack() {
	if s.edit != nil && s.navAuto {
		s.Map.Nav, s.navAuto = s.edit.navHand, false
		s.aiAttDist, s.gridCache = nil, nil
	}
}
