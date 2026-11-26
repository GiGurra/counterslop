package game

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

// An agent co-edits over MCP with an admin in game: the session, ops (each
// seen in game as a delta, the agent's selection moving with them), undo and
// redo, the map, presence both ways ('this': what the admin selected, with
// the objects), a picture with both editors on it, validation with reasons,
// playtest, save, the log with the author mcp.
func TestMCPMapEditor(t *testing.T) {
	e := newEditor(t)
	c := mcpFor(t, e.s, false)
	if out := c.call("map_edit", map[string]any{"action": "status"}); !strings.Contains(out["output"].(string), "no map is being edited") || out["session"] != nil {
		t.Fatalf("status: %v", out)
	}
	for _, tool := range []string{"map_op", "map_validate", "map_presence", "map_save", "map_log", "map_playtest", "map_undo"} {
		args := map[string]any{}
		if tool == "map_op" {
			args["ops"] = []any{map[string]any{"op": "box_delete", "i": 0}}
		}
		if out := c.call(tool, args, true); !strings.Contains(out["error"].(string), "no map is being edited") {
			t.Fatalf("%s without a session: %v", tool, out)
		}
	}
	c.call("map_edit", map[string]any{"action": "dance"}, true)
	c.call("map_edit", map[string]any{"action": "clone", "map": "nope"}, true)
	out := c.call("map_edit", map[string]any{"action": "clone", "map": "de_slop", "name": "de_mcp"})
	if out["session"].(map[string]any)["name"] != "de_mcp" {
		t.Fatalf("clone: %v", out)
	}
	msgs(e.p)
	n := len(e.s.Map.Boxes)
	out = c.call("map_op", map[string]any{"ops": []any{
		map[string]any{"op": "box_add", "box": map[string]any{"min": []float64{0, 0, 20}, "max": []float64{2, 2, 22}, "kind": "crate"}},
		map[string]any{"op": "box_move", "i": n, "by": []float64{0, 0, 2}},
	}})
	if out["applied"].(float64) != 2 || len(e.s.Map.Boxes) != n+1 || e.s.Map.Boxes[n].Min != [3]float64{0, 0, 22} {
		t.Fatalf("ops: %v", out)
	}
	deltas := 0
	for _, m := range msgs(e.p) {
		if m["t"] == "mapdelta" {
			deltas++
		}
	}
	if deltas != 2 {
		t.Fatalf("the admin got %d deltas", deltas)
	}
	if l := e.s.editors(); len(l) != 2 || !l[0].MCP || len(l[0].Sel) != 1 || *l[0].Sel[0].I != n || *l[0].At != [3]float64{1, 1, 23} {
		t.Fatalf("the agent's selection after its ops: %+v", l)
	}
	if l := e.s.editors(); l[1].ID != e.p.ID || l[1].Age != -1 || l[1].At != nil || l[1].Pos == nil { // an admin in game with no editor UI: listed all the same
		t.Fatalf("the admin without a presence: %+v", l[1])
	}
	if out := c.call("map_op", map[string]any{"ops": []any{map[string]any{"op": "box_delete", "i": n}, map[string]any{"op": "box_delete", "i": 9999}}}); out["applied"].(float64) != 1 || out["failed"].(map[string]any)["index"].(float64) != 1 {
		t.Fatalf("a refused second op: %v", out)
	}
	c.call("map_op", map[string]any{"ops": []any{map[string]any{"op": "box_delete", "i": 9999}}}, true)
	c.call("map_op", map[string]any{"ops": []any{map[string]any{"op": "box_delete", "colour": "red"}}}, true)
	c.call("map_op", map[string]any{"ops": []any{}}, true)
	if out := c.call("map_undo", map[string]any{"n": 5}); len(out["outputs"].([]any)) != 3 || len(e.s.Map.Boxes) != n {
		t.Fatalf("undo 5 (3 to undo): %v", out)
	}
	c.call("map_undo", nil, true) // nothing left
	if out := c.call("map_undo", map[string]any{"redo": true, "n": 2}); len(out["outputs"].([]any)) != 2 || len(e.s.Map.Boxes) != n+1 {
		t.Fatalf("redo 2: %v", out)
	}
	// The map.
	out = c.call("map_get", map[string]any{"fields": []string{"boxes", "materials", "fog"}})
	part := out["map"].(map[string]any)
	if len(part["boxes"].([]any)) != n+1 || len(part["materials"].([]any)) != len(content.Materials) || part["fog"] != nil || out["rev"].(float64) != float64(e.s.mapRev()) {
		t.Fatalf("map_get fields: %v", out["session"])
	}
	if out := c.call("map_get", nil); out["map"].(map[string]any)["name"] != "de_mcp" {
		t.Fatal("map_get")
	}
	// What the admin is on: their selection, with the objects; their aim
	// from the server's ray when the client sent none.
	i5 := 5
	e.s.handle(e.p, inbound{T: "editpresence", Sel: []editRef{{Kind: "box", I: &i5}}, Hover: &editRef{Kind: "zone", Zone: "site", Name: "A"}, Tool: "move"})
	ed := c.call("map_editors", nil)["editors"].([]any)
	admin := ed[1].(map[string]any)
	if len(ed) != 2 || admin["name"] != e.p.Name || admin["selObjects"].([]any)[0].(map[string]any)["kind"] != e.s.Map.Boxes[5].Kind ||
		admin["hoverObject"].(map[string]any)["name"] != "A" || admin["aim"] == nil {
		t.Fatalf("map_editors: %v", ed)
	}
	// The agent's presence, named.
	out = c.call("map_presence", map[string]any{"at": []float64{1, 0, 1}, "sel": []any{map[string]any{"kind": "spawn", "team": "vibe", "i": 0}}, "tool": "thinking", "name": "claude"})
	if l := e.s.editors(); l[0].Name != "claude" || l[0].Tool != "thinking" || l[0].Sel[0].Kind != "spawn" {
		t.Fatalf("map_presence: %+v", l)
	}
	c.call("map_presence", map[string]any{"tool": "placing cover"}) // the rest stays
	if l := e.s.editors(); l[0].Tool != "placing cover" || len(l[0].Sel) != 1 || l[0].At == nil {
		t.Fatalf("map_presence with one field: %+v", l[0])
	}
	c.call("map_presence", map[string]any{"sel": []any{}, "hover": map[string]any{"kind": "box", "i": 1}})
	if l := e.s.editors(); len(l[0].Sel) != 0 || l[0].Hover == nil || l[0].Tool != "placing cover" {
		t.Fatalf("map_presence clearing sel: %+v", l[0])
	}
	// A picture: an image with a caption naming both editors.
	res := c.rpc("tools/call", map[string]any{"name": "map_render", "arguments": map[string]any{"size": 300}})["result"].(map[string]any)["content"].([]any)
	caption, img := res[0].(map[string]any)["text"].(string), res[1].(map[string]any)
	b, _ := base64.StdEncoding.DecodeString(img["data"].(string))
	pic, err := png.Decode(bytes.NewReader(b))
	if err != nil || img["type"] != "image" || pic.Bounds().Dx() != 300 || !strings.Contains(caption, "top view") || !strings.Contains(caption, "claude #00e6ff") || !strings.Contains(caption, e.p.Name) {
		t.Fatalf("map_render: %v %v %q", err, pic.Bounds(), caption)
	}
	c.rpc("tools/call", map[string]any{"name": "map_render", "arguments": map[string]any{"view": "side", "editors": false, "labels": false}})
	c.call("map_render", map[string]any{"view": "iso"}, true)
	// Validation with reasons, playtest, save.
	if out := c.call("map_validate", nil); out["ok"] != true {
		t.Fatalf("de_slop with a crate: %v", out["problems"])
	}
	c.call("map_op", map[string]any{"ops": []any{map[string]any{"op": "zone_delete", "kind": "buy", "team": "artisan", "i": 0}}})
	if out := c.call("map_validate", map[string]any{"name": "de_mcp2"}); out["ok"] != false || !strings.Contains(out["problems"].([]any)[0].(string), "buy zone") {
		t.Fatalf("no artisan buy zone: %v", out)
	}
	c.call("map_save", nil, true) // refused: the same reasons
	c.call("map_undo", nil)
	if out := c.call("map_playtest", map[string]any{"on": true}); out["output"] != "playtest on" {
		t.Fatalf("playtest: %v", out)
	}
	c.call("map_op", map[string]any{"ops": []any{map[string]any{"op": "box_delete", "i": 0}}}, true)
	c.call("map_playtest", nil)   // toggles off
	c.call("map_save", nil, true) // no maps dir
	e.s.MapsDir = t.TempDir()
	if out := c.call("map_save", map[string]any{"name": "de_mcp_saved"}); !strings.Contains(out["output"].(string), "de_mcp_saved.json") {
		t.Fatalf("save: %v", out)
	}
	res = c.rpc("tools/call", map[string]any{"name": "map_log", "arguments": map[string]any{"n": 3}})["result"].(map[string]any)["content"].([]any)
	if log := res[0].(map[string]any)["text"].(string); strings.Count(log, "\n") != 2 || !strings.Contains(log, "mcp: save de_mcp_saved") {
		t.Fatalf("log: %q", log)
	}
	if out := c.call("map_edit", map[string]any{"action": "stop"}); out["session"] != nil {
		t.Fatalf("stop: %v", out)
	}
	if out := c.call("map_edit", map[string]any{"action": "new", "name": "de_mcp_blank"}); out["session"].(map[string]any)["boxes"].(float64) != 1 {
		t.Fatalf("new: %v", out)
	}
}

// What each op touched, and what each ref names (with its extent).
func TestMCPRefs(t *testing.T) {
	e := newEditor(t)
	e.run("map_edit", "new", "de_refs")
	e.op(`{"op":"nav_set","name":"n1","pos":[0,0,0]}`)
	e.op(`{"op":"item_add","weapon":"prompt","pos":[1,0,1]}`)
	e.op(`{"op":"hint_add","pos":[2,0,2]}`)
	e.op(`{"op":"zone_set","kind":"pit","name":"lava","min":[-2,-1,-2],"max":[2,0,2]}`)
	i := func(n int) *int { return &n }
	for _, c := range []struct {
		op   EditOp
		want *editRef
	}{
		{EditOp{Op: "box_add"}, &editRef{Kind: "box", I: i(0)}},
		{EditOp{Op: "box_ramp", I: 0}, &editRef{Kind: "box", I: i(0)}},
		{EditOp{Op: "spawn_add", Team: "vibe"}, &editRef{Kind: "spawn", Team: "vibe", I: i(0)}},
		{EditOp{Op: "spawn_move", Team: "artisan", I: 0}, &editRef{Kind: "spawn", Team: "artisan", I: i(0)}},
		{EditOp{Op: "item_add"}, &editRef{Kind: "item", I: i(0)}},
		{EditOp{Op: "item_move", I: 0}, &editRef{Kind: "item", I: i(0)}},
		{EditOp{Op: "nav_set", Name: "n1"}, &editRef{Kind: "nav", Name: "n1"}},
		{EditOp{Op: "hint_add"}, &editRef{Kind: "hint", I: i(0)}},
		{EditOp{Op: "hint_lane", I: 0}, &editRef{Kind: "hint", I: i(0)}},
		{EditOp{Op: "zone_set", Kind: "buy", Team: "vibe", I: 5}, &editRef{Kind: "zone", Zone: "buy", Team: "vibe", I: i(0)}},
		{EditOp{Op: "zone_set", Kind: "site", Name: "A"}, &editRef{Kind: "zone", Zone: "site", Name: "A"}},
		{EditOp{Op: "zone_set", Kind: "pit", Name: "lava"}, &editRef{Kind: "zone", Zone: "pit", Name: "lava"}},
		{EditOp{Op: "box_delete"}, nil},
		{EditOp{Op: "rule_set"}, nil},
	} {
		got := e.s.touchedBy(c.op)
		if (got == nil) != (c.want == nil) || got != nil && (got.Kind != c.want.Kind || got.Zone != c.want.Zone || got.Name != c.want.Name || got.Team != c.want.Team || (got.I == nil) != (c.want.I == nil) || got.I != nil && *got.I != *c.want.I) {
			t.Fatalf("%s: %+v, want %+v", c.op.Op, got, c.want)
		}
		if got != nil {
			if _, lo, hi, ok := e.s.refObject(*got); !ok || lo[0] > hi[0] {
				t.Fatalf("%s: %+v names nothing", c.op.Op, got)
			}
		}
	}
	for _, r := range []editRef{{Kind: "box", I: i(9)}, {Kind: "nav", Name: "x"}, {Kind: "zone", Zone: "site", Name: "C"}, {Kind: "zone", Zone: "buy", Team: "vibe", I: i(3)}, {Kind: "box"}} {
		if _, _, _, ok := e.s.refObject(r); ok {
			t.Fatalf("%+v names something", r)
		}
	}
	e.s.mcpSelect(nil)
	e.s.mcpSelect(&editRef{Kind: "box", I: i(99)})
	if l := e.s.editors(); len(l) != 1 || l[0].MCP {
		t.Fatalf("selecting nothing made the agent an editor: %+v", l)
	}
	if hexColor("#ff8000") != (hexColor("#FF8000")) || hexColor("red").R != 255 || hexColor("#ff8000").G != 0x80 {
		t.Fatal("hexColor")
	}
	if e.s.aimPoint(nil) != nil {
		t.Fatal("aim of nobody")
	}
	e.p.Pitch = 1.5 // straight up from the floor of a blank map: nothing
	if e.s.aimPoint(e.p) != nil {
		t.Fatal("aim at the sky")
	}
}
