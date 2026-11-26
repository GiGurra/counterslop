package game

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maprender"
	"github.com/gigurra/counterslop/internal/detmath"
	"github.com/gigurra/counterslop/internal/mcp"
	"github.com/gigurra/counterslop/internal/sim"
)

// The map editor over MCP (docs/map-editor.md, "Co-editing with an agent"):
// the agent edits the same session as the admins in game, through the same
// edit ops (EditOp), as editor "mcp". It sees what they see: the map and its
// revision, pictures of it (map_render), and every editor's aim point, hover
// and selection (map_editors), so it can act on "this"; they see its
// selection and aim point (map_presence, and what each of its ops touched).

// mcpAuthor is the agent's name in the op log and announcements.
const mcpAuthor = "mcp"

// mcpColor is the agent's colour in renders (the in-game editor picks its own).
var mcpColor = color.RGBA{0, 230, 255, 255}

// editRefSchema is an editRef's JSON schema.
var editRefSchema = mcp.Object(map[string]any{
	"kind": mcp.Enum("what it is", "box", "spawn", "zone", "item", "nav", "hint"),
	"zone": mcp.Enum("zone: which", "site", "buy", "pit"),
	"name": mcp.Str("site (A, B), pit, nav node"),
	"team": mcp.Enum("spawn, buy zone", content.Vibe, content.Artisan),
	"i":    mcp.Int("box, spawn, buy zone, item, nav hint: its index"),
}, "kind")

var vec3Schema = mcp.Arr("[x, y, z] metres (y up)", mcp.Num(""))

// editOpSchema is an EditOp's.
func editOpSchema() map[string]any {
	ops := make([]string, len(editOpHelp))
	var help strings.Builder
	for i, h := range editOpHelp {
		ops[i] = h[0]
		fmt.Fprintf(&help, "%s: %s. ", h[0], h[1])
	}
	return mcp.Object(map[string]any{
		"op":   mcp.Enum(strings.TrimSpace(help.String()), ops...),
		"i":    mcp.Int("the box, spawn, zone or item's index"),
		"team": mcp.Enum("spawn and buy zone ops", content.Vibe, content.Artisan),
		"name": mcp.Str("a site (A, B), a pit, a nav node"),
		"kind": mcp.Str("a material (box ops; materials: map_get fields [materials]); zone ops: site, buy, pit"),
		"box": mcp.Object(map[string]any{"min": vec3Schema, "max": vec3Schema, "kind": mcp.Str("material"),
			"ramp": mcp.Int("0 a box; 1-4 a wedge rising to +x -x +z -z")}, "min", "max", "kind"),
		"min": vec3Schema, "max": vec3Schema, "by": vec3Schema, "pos": vec3Schema,
		"ramp":       mcp.Int("box_ramp: 0 a box, 1-4 a wedge rising to +x -x +z -z"),
		"weapon":     mcp.Str("item ops: a weapon id"),
		"links":      mcp.Arr("nav_set: the nodes it links to (both ways)", mcp.Str("")),
		"key":        mcp.Str("rule ops: a cvar a map may pin"),
		"value":      mcp.Str("rule_set: its value"),
		"desc":       mcp.Str("meta_set"),
		"maxPlayers": mcp.Int("meta_set"),
		"category":   mcp.Str("meta_set"),
	}, "op")
}

// mcpSession is the edit session at a glance (nil: nothing is edited).
func (s *Server) mcpSession() any {
	e := s.edit
	if e == nil {
		return nil
	}
	return map[string]any{"name": e.name, "rev": e.rev, "boxes": len(s.Map.Boxes), "undo": len(e.undo), "redo": len(e.redo),
		"playtest": e.playtest, "editors": len(s.editors())}
}

// mcpEditorTools: the map editor's tools (every admin MCP: co-editing is
// what the editor port is for).
func (s *Server) mcpEditorTools(m *mcp.Server) {
	m.Add(mcp.Tool{Name: "map_edit", Description: "The map edit session (one at a time, on the map in play; everyone in game sees each change live): status; new (a blank map named name); clone (map, named name, default <map>_edit); stop (unsaved edits are gone). Admins in game fly while it is on (noclip) and bots stand still. Then map_get, map_render, map_editors, map_op, map_undo, map_validate, map_playtest, map_save.",
		Input: mcp.Object(map[string]any{"action": mcp.Enum("what to do", "status", "new", "clone", "stop"),
			"map": mcp.Str("clone: the map to start from (maps lists them)"), "name": mcp.Str("the new map's name (de_/gs_/he_..., lower case)")}, "action"),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct{ Action, Map, Name string }
			if err := mcp.Args(raw, &a); err != nil {
				return nil, err
			}
			return s.locked(func() (any, error) {
				var out string
				var err error
				switch a.Action {
				case "status":
					out = s.editStatus()
				case "new":
					out, err = s.editStart(mcpAuthor, []string{"new", a.Name})
				case "clone":
					args := []string{"clone", a.Map}
					if a.Name != "" {
						args = append(args, a.Name)
					}
					out, err = s.editStart(mcpAuthor, args)
				case "stop":
					out, err = s.editStop(mcpAuthor)
				default:
					err = fmt.Errorf("action status, new, clone or stop")
				}
				if err != nil {
					return nil, err
				}
				return map[string]any{"output": out, "session": s.mcpSession()}, nil
			})
		}})
	m.Add(mcp.Tool{Name: "map_get", Description: "The map in play as JSON (the custom map format, docs/custom-maps.md): array positions are the indices ops use (boxes[13] is box 13). fields picks top-level fields (boxes, teamSpawns, buyZones, sites, pits, items, nav, rules, ...); materials lists the box materials. With the session's revision: a change by anyone bumps it.",
		Input: mcp.Object(map[string]any{"fields": mcp.Arr("top-level fields to return (default: all)", mcp.Str(""))}),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct{ Fields []string }
			if err := mcp.Args(raw, &a); err != nil {
				return nil, err
			}
			return s.locked(func() (any, error) {
				var whole map[string]json.RawMessage
				b, _ := json.Marshal(s.Map)
				_ = json.Unmarshal(b, &whole)
				out := map[string]any{"session": s.mcpSession(), "rev": s.mapRev()}
				if len(a.Fields) == 0 {
					out["map"] = json.RawMessage(b)
					return out, nil
				}
				part := map[string]any{}
				for _, f := range a.Fields {
					switch v, ok := whole[f]; {
					case f == "materials":
						part[f] = content.Materials
					case ok:
						part[f] = v
					default:
						part[f] = nil // not set on this map
					}
				}
				out["map"] = part
				return out, nil
			})
		}})
	m.Add(mcp.Tool{Name: "map_op", Description: "Apply edit ops to the map being edited, in order (each its own undo step; the first refused one stops the rest and says why). Everyone in game sees each at once. Your selection (seen in game) becomes what the last op touched. Box: min/max corners in metres, y up, the floor's top usually at y 0; a player is 1.8 m tall, jumps ~1.2 m, a crate is ~1.5 m.",
		Input: mcp.Object(map[string]any{"ops": mcp.Arr("the ops", editOpSchema())}, "ops"),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct{ Ops []json.RawMessage }
			if err := mcp.Args(raw, &a); err != nil {
				return nil, err
			}
			if len(a.Ops) == 0 || len(a.Ops) > 200 {
				return nil, fmt.Errorf("1-200 ops")
			}
			return s.locked(func() (any, error) {
				var outs []string
				var touched *editRef
				for k, rawOp := range a.Ops {
					var op EditOp
					d := json.NewDecoder(strings.NewReader(string(rawOp)))
					d.DisallowUnknownFields()
					err := d.Decode(&op)
					var out string
					if err == nil {
						out, err = s.editOp(mcpAuthor, op)
					}
					if err != nil {
						if k == 0 {
							return nil, err
						}
						s.mcpSelect(touched)
						return map[string]any{"applied": k, "outputs": outs, "failed": map[string]any{"index": k, "error": err.Error()}, "session": s.mcpSession()}, nil
					}
					outs = append(outs, out)
					touched = s.touchedBy(op)
				}
				s.mcpSelect(touched)
				return map[string]any{"applied": len(a.Ops), "outputs": outs, "session": s.mcpSession()}, nil
			})
		}})
	m.Add(mcp.Tool{Name: "map_undo", Description: "Undo (or redo) the last n ops, anyone's (the in-game editor's too).",
		Input: mcp.Object(map[string]any{"redo": mcp.Bool("redo instead"), "n": mcp.Int("how many (default 1, at most 50)")}),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct {
				Redo bool
				N    int
			}
			if err := mcp.Args(raw, &a); err != nil {
				return nil, err
			}
			n := min(max(a.N, 1), 50)
			return s.locked(func() (any, error) {
				var outs []string
				for range n {
					out, err := s.editUndo(mcpAuthor, a.Redo)
					if err != nil {
						if len(outs) == 0 {
							return nil, err
						}
						break
					}
					outs = append(outs, out)
				}
				return map[string]any{"outputs": outs, "session": s.mcpSession()}, nil
			})
		}})
	m.Add(mcp.Tool{Name: "map_validate", Description: "Check the map being edited as map_save would (and --maps-dir does): every team can walk from every spawn to every objective (both movement models; a nav graph is built from the geometry if it has none), spawns in buy zones, sites A and B, sizes, names. ok, or the problems with reasons (e.g. 'stuck at [x y z] on the way to A').",
		Input: mcp.Object(map[string]any{"name": mcp.Str("the name it would be saved as (default the session's)")}),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct{ Name string }
			if err := mcp.Args(raw, &a); err != nil {
				return nil, err
			}
			s.mu.Lock()
			if s.edit == nil {
				s.mu.Unlock()
				return nil, fmt.Errorf("no map is being edited (map_edit)")
			}
			name := a.Name
			if name == "" {
				name = s.edit.name
			}
			m := s.editSaved(name) // copied under the lock, checked outside it
			s.mu.Unlock()
			p := s.checkSaved(m, name+".json")
			return map[string]any{"ok": len(p) == 0, "problems": p, "name": name}, nil
		}})
	m.Add(mcp.Tool{Name: "map_render", Description: "A picture of the map in play (PNG) with a caption saying its scale and axes: top (from above: x right, z down), side (x right, y up) or front (z right, y up; side and front see through boxes). Boxes in their material's colour, lighter higher (top) or nearer; box indices and nav names with labels; sites, buy zones, pits outlined; spawns, items, nav; and each editor's aim point (crosshair, named) and selection (outlined in their colour, the agent cyan). min/max frame a region (a y range from above shows one floor).",
		Input: mcp.Object(map[string]any{"view": mcp.Enum("which view (default top)", maprender.Views...),
			"min": vec3Schema, "max": vec3Schema, "size": mcp.Int("the longer side in pixels (default 800)"),
			"labels": mcp.Bool("box indices and nav names (default true)"), "nav": mcp.Bool("the nav graph (default true)"),
			"editors": mcp.Bool("the editors' aim points and selections (default true)")}),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct {
				View                 string
				Min, Max             *[3]float64
				Size                 int
				Labels, Nav, Editors *bool
			}
			if err := mcp.Args(raw, &a); err != nil {
				return nil, err
			}
			on := func(b *bool) bool { return b == nil || *b }
			o := maprender.Options{View: a.View, Min: a.Min, Max: a.Max, Size: a.Size, Labels: on(a.Labels), Nav: on(a.Nav)}
			s.mu.Lock()
			m := s.Map // replaced, never changed in place (applyPatches): safe to draw outside the lock
			var who []string
			if on(a.Editors) {
				o.Marks, o.Lights, who = s.editorMarks()
			}
			s.mu.Unlock()
			b, f, err := maprender.Render(m, o)
			if err != nil {
				return nil, err
			}
			caption := m.Name + ": " + f.Caption()
			if len(who) > 0 {
				caption += " Editors: " + strings.Join(who, ", ") + "."
			}
			return mcp.Image{PNG: b, Caption: caption}, nil
		}})
	m.Add(mcp.Tool{Name: "map_editors", Description: "Who is editing and what they are on: each editor's aim point (at: where their crosshair meets the map), what they hover and have selected (refs in the ops' terms, with the objects themselves), their tool, and a player's position and view. 'This' is usually their selection, else what they hover, else where they aim.",
		Call: func(json.RawMessage) (any, error) {
			return s.locked(func() (any, error) {
				type resolved struct {
					editorEntry
					Aim      *[3]float64 `json:"aim,omitempty"` // a player with no at: where their view meets the map (the server's ray)
					HoverObj any         `json:"hoverObject,omitempty"`
					SelObj   []any       `json:"selObjects,omitempty"`
				}
				var out []resolved
				for _, en := range s.editors() {
					r := resolved{editorEntry: en}
					if en.Hover != nil {
						r.HoverObj, _, _, _ = s.refObject(*en.Hover)
					}
					for _, ref := range en.Sel {
						o, _, _, _ := s.refObject(ref)
						r.SelObj = append(r.SelObj, o)
					}
					if en.At == nil && !en.MCP {
						r.Aim = s.aimPoint(s.players[en.ID])
					}
					out = append(out, r)
				}
				return map[string]any{"rev": s.mapRev(), "session": s.mcpSession(), "editors": out}, nil
			})
		}})
	m.Add(mcp.Tool{Name: "map_presence", Description: "Show the editors in game where you are working: your aim point (at), what you hover and have selected (refs as map_editors gives them; sel [] clears it), a tool name, and the name shown by your marker (default mcp). Fields left out stay as they were. Each map_op also selects what it touched.",
		Input: mcp.Object(map[string]any{"at": vec3Schema, "hover": editRefSchema, "sel": mcp.Arr("the selection", editRefSchema),
			"tool": mcp.Str("what you are doing (short)"), "name": mcp.Str("your marker's name")}),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct {
				At    *[3]float64
				Hover *editRef
				Sel   []editRef // nil: left out (keep); []: clear
				Tool  *string
				Name  string
			}
			if err := mcp.Args(raw, &a); err != nil {
				return nil, err
			}
			return s.locked(func() (any, error) {
				if s.edit == nil {
					return nil, fmt.Errorf("no map is being edited (map_edit)")
				}
				pr := editPresence{}
				if old := s.edit.presence[mcpEditorID]; old != nil {
					pr = *old
				}
				if a.At != nil {
					pr.At = a.At
				}
				if a.Hover != nil {
					pr.Hover = a.Hover
				}
				if a.Sel != nil {
					pr.Sel = a.Sel
				}
				if a.Tool != nil {
					pr.Tool = *a.Tool
				}
				s.setPresence(mcpEditorID, pr)
				if n := sanitizeName(a.Name); n != "" {
					s.edit.mcpName = n
				}
				return map[string]any{"editors": s.editors()}, nil
			})
		}})
	m.Add(mcp.Tool{Name: "map_playtest", Description: "Play the map being edited as it is (a fresh match, bots on, nobody flies; a map without nav gets one built from its geometry), or back to editing (on false). Edits wait until playtest is off.",
		Input: mcp.Object(map[string]any{"on": mcp.Bool("on (default: toggle)")}),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct{ On *bool }
			if err := mcp.Args(raw, &a); err != nil {
				return nil, err
			}
			var args []string
			if a.On != nil {
				args = []string{map[bool]string{true: "1", false: "0"}[*a.On]}
			}
			return s.locked(func() (any, error) {
				out, err := s.editPlaytest(mcpAuthor, args)
				if err != nil {
					return nil, err
				}
				return map[string]any{"output": out, "session": s.mcpSession()}, nil
			})
		}})
	m.Add(mcp.Tool{Name: "map_save", Description: "Save the map being edited to the server's maps dir as <name>.json (default the session's name) after the checks map_validate runs; refused with the reasons. It is then in the maps list (a custom map) for changelevel and the vote.",
		Input: mcp.Object(map[string]any{"name": mcp.Str("the name to save as")}),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct{ Name string }
			if err := mcp.Args(raw, &a); err != nil {
				return nil, err
			}
			var args []string
			if a.Name != "" {
				args = []string{a.Name}
			}
			return s.locked(func() (any, error) {
				out, err := s.editSave(mcpAuthor, args)
				if err != nil {
					return nil, err
				}
				return map[string]any{"output": out, "session": s.mcpSession()}, nil
			})
		}})
	m.Add(mcp.Tool{Name: "map_log", Description: "The session's op log, newest last: revision, author (an admin's name, or mcp) and the op.",
		Input: mcp.Object(map[string]any{"n": mcp.Int("the last n (default 20)")}),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct{ N int }
			if err := mcp.Args(raw, &a); err != nil {
				return nil, err
			}
			return s.locked(func() (any, error) {
				var args []string
				if a.N > 0 {
					args = []string{strconv.Itoa(a.N)}
				}
				out, err := s.editLogText(args)
				if err != nil {
					return nil, err
				}
				return mcp.Text(out), nil
			})
		}})
}

// mcpSelect makes the agent's selection what its last op touched (nothing:
// as it was), its aim point that thing's centre. Callers hold s.mu.
func (s *Server) mcpSelect(r *editRef) {
	if r == nil {
		return
	}
	_, lo, hi, ok := s.refObject(*r)
	if !ok {
		return
	}
	at := [3]float64{(lo[0] + hi[0]) / 2, (lo[1] + hi[1]) / 2, (lo[2] + hi[2]) / 2}
	tool := ""
	if pr := s.edit.presence[mcpEditorID]; pr != nil {
		tool = pr.Tool
	}
	s.setPresence(mcpEditorID, editPresence{At: &at, Sel: []editRef{*r}, Tool: tool})
}

// touchedBy: what op (just applied) made or changed, or nil (a delete, a
// rule). Callers hold s.mu.
func (s *Server) touchedBy(op EditOp) *editRef {
	i := func(n int) *int { return &n }
	m := &s.Map
	switch op.Op {
	case "box_add":
		return &editRef{Kind: "box", I: i(len(m.Boxes) - 1)}
	case "box_set", "box_move", "box_resize", "box_material", "box_ramp":
		return &editRef{Kind: "box", I: i(op.I)}
	case "spawn_add":
		return &editRef{Kind: "spawn", Team: op.Team, I: i(len(m.TeamSpawns[op.Team]) - 1)}
	case "spawn_move":
		return &editRef{Kind: "spawn", Team: op.Team, I: i(op.I)}
	case "item_add":
		return &editRef{Kind: "item", I: i(len(m.Items) - 1)}
	case "item_move":
		return &editRef{Kind: "item", I: i(op.I)}
	case "hint_add":
		return &editRef{Kind: "hint", I: i(len(m.NavHints) - 1)}
	case "hint_move", "hint_lane":
		return &editRef{Kind: "hint", I: i(op.I)}
	case "nav_set":
		return &editRef{Kind: "nav", Name: op.Name}
	case "zone_set":
		if op.Kind == "buy" {
			return &editRef{Kind: "zone", Zone: "buy", Team: op.Team, I: i(min(op.I, len(m.BuyZones[op.Team])-1))}
		}
		return &editRef{Kind: "zone", Zone: op.Kind, Name: op.Name}
	}
	return nil
}

// refObject: the thing r names on the map, and its extent (a point's: a
// player-sized box on it). Callers hold s.mu.
func (s *Server) refObject(r editRef) (any, [3]float64, [3]float64, bool) {
	m := &s.Map
	in := func(n int) bool { return r.I != nil && *r.I >= 0 && *r.I < n }
	point := func(p [3]float64) ([3]float64, [3]float64) {
		return [3]float64{p[0] - 0.4, p[1], p[2] - 0.4}, [3]float64{p[0] + 0.4, p[1] + 1.8, p[2] + 0.4}
	}
	zone := func(zs []content.Zone) (any, [3]float64, [3]float64, bool) {
		for _, z := range zs {
			if z.Name == r.Name {
				return z, z.Min, z.Max, true
			}
		}
		return nil, [3]float64{}, [3]float64{}, false
	}
	switch {
	case r.Kind == "box" && in(len(m.Boxes)):
		b := m.Boxes[*r.I]
		return b, b.Min, b.Max, true
	case r.Kind == "spawn" && in(len(m.TeamSpawns[r.Team])):
		p := m.TeamSpawns[r.Team][*r.I]
		lo, hi := point(p)
		return p, lo, hi, true
	case r.Kind == "item" && in(len(m.Items)):
		it := m.Items[*r.I]
		lo, hi := point(it.Pos)
		return it, lo, hi, true
	case r.Kind == "hint" && in(len(m.NavHints)):
		h := m.NavHints[*r.I]
		lo, hi := point(h.Pos)
		return h, lo, hi, true
	case r.Kind == "nav":
		for _, n := range m.Nav {
			if n.Name == r.Name {
				lo, hi := point(n.Pos)
				return n, lo, hi, true
			}
		}
	case r.Kind == "zone" && r.Zone == "site":
		return zone(m.Sites)
	case r.Kind == "zone" && r.Zone == "pit":
		return zone(m.Pits)
	case r.Kind == "zone" && r.Zone == "buy" && in(len(m.BuyZones[r.Team])):
		z := m.BuyZones[r.Team][*r.I]
		return z, z.Min, z.Max, true
	}
	return nil, [3]float64{}, [3]float64{}, false
}

// aimPoint: where p's view meets the map's boxes (within 200 m), or nil.
// Callers hold s.mu.
func (s *Server) aimPoint(p *Player) *[3]float64 {
	if p == nil || p.body == nil {
		return nil
	}
	eye := sim.Vec3{p.body.Pos[0], p.body.Pos[1] + p.body.EyeH, p.body.Pos[2]}
	cp := detmath.Cos(p.Pitch)
	d := sim.Vec3{-float64(detmath.Sin(p.Yaw) * cp), detmath.Sin(p.Pitch), -float64(detmath.Cos(p.Yaw) * cp)}
	best := 200.0
	hit := false
	for i := range s.Map.Boxes {
		if t, _, _ := sim.RaySolid(eye, d, &s.Map.Boxes[i]); t >= 0 && t < best {
			best, hit = t, true
		}
	}
	if !hit {
		return nil
	}
	at := [3]float64{eye[0] + d[0]*best, eye[1] + d[1]*best, eye[2] + d[2]*best}
	for i := range at {
		at[i] = math.Round(at[i]*1000) / 1000
	}
	return &at
}

// editorMarks: every editor's aim point (a named crosshair) and selection
// (outlined; hover thin) in their colour, for map_render, and who is who.
// Callers hold s.mu.
func (s *Server) editorMarks() ([]maprender.Mark, []maprender.Highlight, []string) {
	var marks []maprender.Mark
	var lights []maprender.Highlight
	var who []string
	for _, en := range s.editors() {
		col := mcpColor
		if !en.MCP {
			col = hexColor(s.players[en.ID].Color)
		}
		who = append(who, fmt.Sprintf("%s #%02x%02x%02x", en.Name, col.R, col.G, col.B))
		at := en.At
		if at == nil && !en.MCP {
			at = s.aimPoint(s.players[en.ID])
		}
		if at != nil {
			marks = append(marks, maprender.Mark{Pos: *at, Color: col, Label: en.Name})
		}
		if en.Hover != nil {
			if _, lo, hi, ok := s.refObject(*en.Hover); ok {
				lights = append(lights, maprender.Highlight{Min: lo, Max: hi, Color: col, Thin: true})
			}
		}
		for _, r := range en.Sel {
			if _, lo, hi, ok := s.refObject(r); ok {
				lights = append(lights, maprender.Highlight{Min: lo, Max: hi, Color: col})
			}
		}
	}
	return marks, lights, who
}

// hexColor: "#rrggbb" (a player's colour), white if it is not one.
func hexColor(h string) color.RGBA {
	v, err := strconv.ParseUint(strings.TrimPrefix(h, "#"), 16, 32)
	if err != nil || len(h) != 7 {
		return color.RGBA{255, 255, 255, 255}
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}
