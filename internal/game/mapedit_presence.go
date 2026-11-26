package game

import (
	"encoding/json"
	"math"
	"slices"

	"github.com/gigurra/counterslop/internal/content"
)

// Editor presence (docs/map-editor.md): where each editor of the map being
// edited aims, what they hover and have selected, and with which tool, so
// every editor (the in-game editor UI, the agent over MCP) can see the others
// and act on "this". State, not events: each editor sends its whole presence
// (editpresence, at most 10 Hz) and the server sends everyone the whole list
// (editors) when it changed, at most every 100 ms, and every second anyway;
// a lost message is made good by the next.

// editRef names one thing on the map in the edit ops' terms, so a selection
// feeds straight into an op: a box, a team spawn, a zone (site and pit by
// name, buy zone by team and index), an item, a nav node.
type editRef struct {
	Kind string `json:"kind"`           // box, spawn, zone, item, nav
	Zone string `json:"zone,omitempty"` // zone: site, buy, pit
	Name string `json:"name,omitempty"` // site (A, B), pit, nav node
	Team string `json:"team,omitempty"` // spawn, buy zone
	I    *int   `json:"i,omitempty"`    // box, spawn, buy zone, item
}

// editPresence is one editor's.
type editPresence struct {
	At    *[3]float64 `json:"at"`    // the aim point on the map; nil: aiming at nothing
	Hover *editRef    `json:"hover"` // what the cursor is on
	Sel   []editRef   `json:"sel"`   // the selection
	Tool  string      `json:"tool"`  // the editor's tool (the UI's names: move, box, zone, ...)
	tick  int64       // when it last came
}

// Limits on what an editor may say about itself.
const (
	maxPresenceSel  = 64
	maxPresenceTool = 32
	presenceMinGap  = SimHz / 10 // ticks between two editors messages at most (100 ms)
	presenceRefresh = SimHz      // and one every second anyway
)

// mcpEditorID is the agent's id in the editors list (players are 1+).
const mcpEditorID = 0

// cleanRef: r as the server keeps it, or false if it names nothing an op could.
func cleanRef(r editRef) (editRef, bool) {
	ok := false
	switch r.Kind {
	case "box", "item", "hint":
		ok, r.Zone, r.Name, r.Team = r.I != nil, "", "", ""
	case "spawn":
		ok, r.Zone, r.Name = r.I != nil && (r.Team == content.Vibe || r.Team == content.Artisan), "", ""
	case "nav":
		ok, r.Zone, r.Team, r.I = r.Name != "" && len(r.Name) <= 32, "", "", nil
	case "zone":
		switch r.Zone {
		case "site", "pit":
			ok, r.Team, r.I = r.Name != "" && len(r.Name) <= 32, "", nil
		case "buy":
			ok, r.Name = r.I != nil && (r.Team == content.Vibe || r.Team == content.Artisan), ""
		}
	}
	if r.I != nil && (*r.I < 0 || *r.I > 1<<20) {
		ok = false
	}
	return r, ok
}

// cleanPresence: what an editor sent, sanitised (bad refs dropped, the aim
// point inside the world, the tool name short).
func cleanPresence(in editPresence) editPresence {
	out := editPresence{Tool: in.Tool}
	if len(out.Tool) > maxPresenceTool {
		out.Tool = out.Tool[:maxPresenceTool]
	}
	if in.At != nil && checkPoint(*in.At) == nil {
		at := *in.At
		out.At = &at
	}
	if in.Hover != nil {
		if r, ok := cleanRef(*in.Hover); ok {
			out.Hover = &r
		}
	}
	for _, r := range in.Sel {
		if len(out.Sel) == maxPresenceSel {
			break
		}
		if r, ok := cleanRef(r); ok {
			out.Sel = append(out.Sel, r)
		}
	}
	return out
}

// setPresence stores editor id's presence (a player's editpresence, the
// agent's map_presence). Only while a map is edited. Callers hold s.mu.
func (s *Server) setPresence(id int, p editPresence) bool {
	e := s.edit
	if e == nil {
		return false
	}
	if e.presence == nil {
		e.presence = map[int]*editPresence{}
	}
	c := cleanPresence(p)
	c.tick = s.tick
	e.presence[id] = &c
	e.presenceDirty = true
	return true
}

// editorEntry is one editor in the editors message (and the MCP's map_editors).
type editorEntry struct {
	ID    int         `json:"id"`
	Name  string      `json:"name"`
	MCP   bool        `json:"mcp,omitempty"`
	At    *[3]float64 `json:"at"`
	Hover *editRef    `json:"hover"`
	Sel   []editRef   `json:"sel"`
	Tool  string      `json:"tool"`
	Age   float64     `json:"age"`           // seconds since this editor's last presence; -1: none yet
	Pos   *[3]float64 `json:"pos,omitempty"` // a player's body (the server's), eye height not added
	Yaw   float64     `json:"yaw"`
	Pitch float64     `json:"pitch"`
}

// editors is the presence list, by id (the agent first): everyone who sent
// a presence, and every admin in game while a map is edited (a client
// without the editor UI: no presence, age -1; map_editors aims for them).
// Players who left are dropped. Callers hold s.mu.
func (s *Server) editors() []editorEntry {
	e := s.edit
	out := []editorEntry{}
	if e == nil {
		return out
	}
	ids := make([]int, 0, len(e.presence))
	for id := range e.presence {
		if id != mcpEditorID && s.players[id] == nil {
			delete(e.presence, id) // left
			continue
		}
		ids = append(ids, id)
	}
	for id, p := range s.players {
		if e.presence[id] == nil && !p.Bot && !p.puppet && s.isAdmin(p) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		pr := e.presence[id]
		age := -1.0
		if pr == nil {
			pr = &editPresence{}
		} else {
			age = math.Round(float64(s.tick-pr.tick)/SimHz*10) / 10
		}
		en := editorEntry{ID: id, At: pr.At, Hover: pr.Hover, Sel: pr.Sel, Tool: pr.Tool, Age: age}
		if en.Sel == nil {
			en.Sel = []editRef{}
		}
		if id == mcpEditorID {
			en.Name, en.MCP = e.mcpName, true
		} else {
			p := s.players[id]
			en.Name = p.Name
			if p.body != nil {
				pos := [3]float64(p.body.Pos)
				en.Pos, en.Yaw, en.Pitch = &pos, p.Yaw, p.Pitch
			}
		}
		out = append(out, en)
	}
	return out
}

// tickEditors sends the editors list: when it changed, at most every
// presenceMinGap ticks, and every presenceRefresh ticks anyway (a joiner, a
// lost one). Callers hold s.mu.
func (s *Server) tickEditors() {
	e := s.edit
	if e == nil {
		return
	}
	since := s.tick - e.presenceSent
	if !(e.presenceDirty && since >= presenceMinGap || since >= presenceRefresh) {
		return
	}
	s.sendEditors()
}

func (s *Server) sendEditors() {
	b, _ := json.Marshal(map[string]any{"t": "editors", "rev": s.mapRev(), "list": s.editors()})
	for _, p := range s.players {
		trySend(p, b)
	}
	if s.edit != nil {
		s.edit.presenceSent, s.edit.presenceDirty = s.tick, false
	}
}

// remapPresence keeps every editor's refs on the same things after patches
// moved indices (a box deleted before a selected one) and drops refs to what
// is gone (the deleted box itself). Callers hold s.mu.
func (s *Server) remapPresence(ps []mapPatch) {
	e := s.edit
	if e == nil || len(e.presence) == 0 {
		return
	}
	fix := func(r editRef) (editRef, bool) {
		for _, p := range ps {
			if p.I == nil || !refOnPath(r, p.P) {
				continue
			}
			if r.I == nil {
				continue
			}
			i := *r.I
			switch p.A {
			case "ins":
				if i >= *p.I {
					i++
				}
			case "del":
				if i == *p.I {
					return r, false
				}
				if i > *p.I {
					i--
				}
			}
			r.I = &i
		}
		if r.Kind == "nav" { // a deleted nav node: gone (by name)
			for _, p := range ps {
				if len(p.P) == 1 && p.P[0] == "nav" && p.A == "del" && !slices.ContainsFunc(s.Map.Nav, func(n content.NavNode) bool { return n.Name == r.Name }) {
					return r, false
				}
			}
		}
		return r, true
	}
	for _, pr := range e.presence {
		if pr.Hover != nil {
			if r, ok := fix(*pr.Hover); ok {
				pr.Hover = &r
			} else {
				pr.Hover = nil
			}
		}
		sel := pr.Sel[:0:0]
		for _, r := range pr.Sel {
			if r, ok := fix(r); ok {
				sel = append(sel, r)
			}
		}
		pr.Sel = sel
	}
	e.presenceDirty = true
}

// refOnPath: r is an element of the array at patch path p.
func refOnPath(r editRef, p []string) bool {
	switch {
	case len(p) == 1:
		return r.Kind == "box" && p[0] == "boxes" || r.Kind == "item" && p[0] == "items" || r.Kind == "hint" && p[0] == "navHints"
	case len(p) == 2 && p[0] == "teamSpawns":
		return r.Kind == "spawn" && r.Team == p[1]
	case len(p) == 2 && p[0] == "buyZones":
		return r.Kind == "zone" && r.Zone == "buy" && r.Team == p[1]
	}
	return false
}
