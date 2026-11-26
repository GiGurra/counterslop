package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// editor is an admin at the console of a server, for the map editor's tests.
type editor struct {
	t      *testing.T
	s      *Server
	p      *Player
	deltas []map[string]any // the mapdelta messages the admin got
}

func newEditor(t *testing.T) *editor {
	s := authServer(t)
	p := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-26, 0, 0})
	s.AdminPassword = "hunter2" // an admin is one who logged in (no password: everyone is)
	p.admin = true
	return &editor{t: t, s: s, p: p}
}

// run is one console command; it returns what was printed back.
func (e *editor) run(cmd string, args ...string) string {
	e.s.adminCommand(e.p, cmd, args)
	var out []string
	for _, m := range msgs(e.p) {
		switch m["t"] {
		case "print":
			out = append(out, m["text"].(string))
		case "mapdelta":
			e.deltas = append(e.deltas, m)
		}
	}
	return strings.Join(out, "\n")
}

// op is one map_op; it fails the test if the op is refused.
func (e *editor) op(js string) string {
	e.t.Helper()
	out := e.run("map_op", js)
	if !strings.Contains(out, "rev ") {
		e.t.Fatalf("map_op %s: %s", js, out)
	}
	return out
}

func mapJSON(m content.Map) string { b, _ := json.Marshal(m); return string(b) }

// Every op edits the map in play, a bad one is refused and changes nothing,
// every edit goes out as a delta, and undo takes the map back to the clone
// step by step; redo brings every edit back.
func TestMapEditOps(t *testing.T) {
	e := newEditor(t)
	if out := e.run("map_edit", "clone", "de_slop"); !strings.Contains(out, "editing de_myslop") {
		t.Fatalf("clone: %s", out)
	}
	start := mapJSON(e.s.Map)
	if e.s.Map.Name != "de_myslop" || !e.p.body.Noclip {
		t.Fatalf("editing %s, noclip %v", e.s.Map.Name, e.p.body.Noclip)
	}
	nb := len(e.s.Map.Boxes)
	if out := e.op(`{"op":"box_add","box":{"min":[0,0,40],"max":[2,1,42],"kind":"crate"}}`); !strings.Contains(out, "box ") {
		t.Fatalf("box_add: %s", out)
	}
	i := nb
	ops := []string{
		`{"op":"box_move","i":` + itoa(i) + `,"by":[1,0,0]}`,
		`{"op":"box_resize","i":` + itoa(i) + `,"min":[1,0,40],"max":[5,2,44]}`,
		`{"op":"box_material","i":` + itoa(i) + `,"kind":"metal"}`,
		`{"op":"box_ramp","i":` + itoa(i) + `,"ramp":3}`,
		`{"op":"box_set","i":0,"box":{"min":[-40,-1,-40],"max":[40,0,40],"kind":"concrete"}}`,
		`{"op":"box_delete","i":1}`,
		`{"op":"spawn_add","team":"vibe","pos":[-24,0,6]}`,
		`{"op":"spawn_move","team":"artisan","i":0,"pos":[22,0,1]}`,
		`{"op":"spawn_delete","team":"artisan","i":1}`,
		`{"op":"zone_set","kind":"site","name":"A","min":[10,-0.5,10],"max":[20,3,20]}`,
		`{"op":"zone_set","kind":"buy","team":"vibe","i":1,"min":[-30,-0.5,4],"max":[-20,3,8]}`,
		`{"op":"zone_set","kind":"pit","name":"main","min":[-2,-10,-2],"max":[2,-1,2]}`,
		`{"op":"zone_delete","kind":"buy","team":"vibe","i":0}`,
		`{"op":"item_add","weapon":"prompt","pos":[0,0,30]}`,
		`{"op":"item_move","i":0,"pos":[1,0,30]}`,
		`{"op":"nav_set","name":"hint","pos":[0,0,20],"links":["vibe"]}`,
		`{"op":"nav_set","name":"vibe","pos":[-26,0,0],"links":["hint"]}`,
		`{"op":"rule_set","key":"sv_movement","value":"source"}`,
		`{"op":"rule_set","key":"mp_respawn","value":"1"}`,
		`{"op":"rule_delete","key":"mp_respawn"}`,
		`{"op":"meta_set","desc":"de_slop, but mine","maxPlayers":12,"category":"competitive"}`,
		`{"op":"nav_delete","name":"hint"}`,
		`{"op":"item_delete","i":0}`,
	}
	steps := []string{start, mapJSON(e.s.Map)} // the clone, then with the box
	for _, o := range ops {
		e.op(o)
		steps = append(steps, mapJSON(e.s.Map))
	}
	m := e.s.Map
	i-- // box_delete 1 moved it down one
	if m.Boxes[i].Kind != content.Metal || m.Boxes[i].Ramp != 3 || m.Boxes[i].Min != [3]float64{1, 0, 40} || len(m.Boxes) != nb {
		t.Fatalf("the box: %+v (%d boxes)", m.Boxes[i], len(m.Boxes))
	}
	if len(m.TeamSpawns[content.Vibe]) != len(maps.DeSlop().TeamSpawns[content.Vibe])+1 ||
		len(m.Spawns) != len(m.TeamSpawns[content.Vibe])+len(m.TeamSpawns[content.Artisan]) {
		t.Fatalf("spawns: %d vibe, %d artisan, %d all", len(m.TeamSpawns[content.Vibe]), len(m.TeamSpawns[content.Artisan]), len(m.Spawns))
	}
	if m.Rules["sv_movement"] != "source" || m.Rules["mp_respawn"] != "" || m.Desc != "de_slop, but mine" || m.Category != content.Competitive {
		t.Fatalf("rules %v, meta %q %s", m.Rules, m.Desc, m.Category)
	}
	if len(m.Pits) != 1 || len(m.BuyZones[content.Vibe]) != 1 || m.BuyZones[content.Vibe][0].Min[2] != 4 {
		t.Fatalf("zones: pits %v, vibe buy %v", m.Pits, m.BuyZones[content.Vibe])
	}
	for _, n := range m.Nav {
		for _, l := range n.Links {
			if l == "hint" {
				t.Fatalf("a link to the deleted node: %+v", n)
			}
		}
	}
	if e.s.mapRev() != len(ops)+1 {
		t.Fatalf("rev %d after %d ops", e.s.mapRev(), len(ops)+1)
	}

	// Refused: changes nothing, no delta.
	for _, bad := range []string{
		`{"op":"box_add","box":{"min":[0,0,0],"max":[1,1,1],"kind":"unobtainium"}}`,
		`{"op":"box_add","box":{"min":[0,0,0],"max":[0,1,1],"kind":"crate"}}`,
		`{"op":"box_add","box":{"min":[0,0,0],"max":[1e9,1,1],"kind":"crate"}}`,
		`{"op":"box_move","i":99999,"by":[1,0,0]}`,
		`{"op":"spawn_add","team":"spectator","pos":[0,0,0]}`,
		`{"op":"zone_set","kind":"site","name":"C","min":[0,0,0],"max":[1,1,1]}`,
		`{"op":"item_add","weapon":"bfg","pos":[0,0,0]}`,
		`{"op":"nav_set","name":"x","pos":[0,0,0],"links":["nowhere"]}`,
		`{"op":"rule_set","key":"rcon_password","value":"x"}`,
		`{"op":"rule_set","key":"sv_movement","value":"warp"}`,
		`{"op":"meta_set","maxPlayers":99}`,
		`{"op":"teleport"}`,
		`{"op":"box_add","bogus":1}`,
		`not json`,
	} {
		before := mapJSON(e.s.Map)
		if out := e.run("map_op", bad); strings.Contains(out, "rev ") || !strings.Contains(out, "map_op: ") {
			t.Errorf("map_op %s: %s", bad, out)
		}
		if mapJSON(e.s.Map) != before {
			t.Errorf("map_op %s changed the map", bad)
		}
	}

	// Undo, one step at a time, to the clone; redo all the way.
	for k := len(steps) - 1; k > 0; k-- {
		if out := e.run("map_undo"); !strings.Contains(out, "undo ") {
			t.Fatalf("undo: %s", out)
		}
		if mapJSON(e.s.Map) != steps[k-1] {
			a, b := mapJSON(e.s.Map), steps[k-1]
			n := 0
			for n < len(a) && n < len(b) && a[n] == b[n] {
				n++
			}
			t.Fatalf("undo to step %d: not the map it was at %d: %.120s vs %.120s", k-1, n, a[max(0, n-40):], b[max(0, n-40):])
		}
	}
	if out := e.run("map_undo"); !strings.Contains(out, "nothing to undo") {
		t.Fatalf("past the start: %s", out)
	}
	for k := 1; k < len(steps); k++ {
		e.run("map_redo")
		if mapJSON(e.s.Map) != steps[k] {
			t.Fatalf("redo to step %d: not the map it was", k)
		}
	}
	if out := e.run("map_redo"); !strings.Contains(out, "nothing to redo") {
		t.Fatalf("past the end: %s", out)
	}
	// A new op after an undo drops the redo.
	e.run("map_undo")
	e.op(`{"op":"box_material","i":0,"kind":"crate"}`)
	if out := e.run("map_redo"); !strings.Contains(out, "nothing to redo") {
		t.Fatalf("redo after a new op: %s", out)
	}
	if log := e.run("map_edit", "log", "3"); strings.Count(log, "rev ") != 3 || !strings.Contains(log, e.p.Name+": ") {
		t.Fatalf("log: %s", log)
	}
	if st := e.run("map_edit"); !strings.Contains(st, "editing de_myslop") {
		t.Fatalf("status: %s", st)
	}
	if h := e.run("map_op", "help"); !strings.Contains(h, "box_add") || !strings.Contains(h, "nav_set") {
		t.Fatalf("help: %s", h)
	}
}

// Every edit reaches every player as a delta with the next revision, and
// applying the deltas to the map they had gives the server's map (the JS
// applier does the same: TestMapPatchGolden); a player who lost one asks
// (mapget) and gets the whole map at its revision; every self-snapshot
// carries the revision.
func TestMapEditDeltas(t *testing.T) {
	e := newEditor(t)
	other := cmdPlayer(t, e.s, "2.2.2.2", content.Artisan, Vec3{26, 0, 0})
	e.run("map_edit", "new", "de_blank")
	var doc map[string]any
	for _, m := range msgs(other) {
		if m["t"] == "map" {
			b, _ := json.Marshal(m["map"])
			_ = json.Unmarshal(b, &doc)
		}
	}
	if doc == nil || doc["name"] != "de_blank" {
		t.Fatalf("no map message: %v", doc)
	}
	e.op(`{"op":"box_add","box":{"min":[0,0,0],"max":[2,1,2],"kind":"crate"}}`)
	e.op(`{"op":"rule_set","key":"sv_movement","value":"source"}`)
	e.op(`{"op":"spawn_add","team":"artisan","pos":[24,0,4]}`)
	e.run("map_undo")
	rev := 0
	for _, m := range msgs(other) {
		if m["t"] != "mapdelta" {
			continue
		}
		if int(m["rev"].(float64)) != rev+1 {
			t.Fatalf("delta rev %v after %d", m["rev"], rev)
		}
		rev++
		b, _ := json.Marshal(m["patch"])
		var ps []mapPatch
		_ = json.Unmarshal(b, &ps)
		for _, p := range ps {
			if _, err := patchDoc(doc, p); err != nil {
				t.Fatal(err)
			}
		}
	}
	var got content.Map
	b, _ := json.Marshal(doc)
	_ = json.Unmarshal(b, &got)
	if rev != 4 || mapJSON(got) != mapJSON(e.s.Map) {
		t.Fatalf("after %d deltas the player's map isn't the server's", rev)
	}
	if ms := e.s.meStateOf(other); ms.MapRev != 4 {
		t.Fatalf("snapshot map rev %d", ms.MapRev)
	}
	e.s.sendMapSync(other)
	ms := msgs(other)
	if len(ms) != 1 || ms[0]["t"] != "mapsync" || ms[0]["rev"].(float64) != 4 {
		t.Fatalf("mapsync: %v", ms)
	}
}

// An admin flies through walls while editing (others don't), bots stand
// still; playtest plays the map (no noclip, bots on, an auto nav on a map
// without one, gone again after); stop ends it; changelevel ends it.
func TestMapEditNoclipAndPlaytest(t *testing.T) {
	e := newEditor(t)
	other := cmdPlayer(t, e.s, "2.2.2.2", content.Artisan, Vec3{26, 0, 0})
	bot := e.s.addAI(content.Artisan)
	e.run("map_edit", "new", "de_blank")
	stepSim(e.s)
	if !e.p.body.Noclip || other.body.Noclip || bot.body != nil && bot.body.Noclip {
		t.Fatalf("noclip: admin %v, other %v", e.p.body.Noclip, other.body.Noclip)
	}
	if !e.s.meStateOf(e.p).Noclip {
		t.Fatal("the self-snapshot doesn't say noclip")
	}
	botAt := bot.Pos
	e.op(`{"op":"box_add","box":{"min":[-10,0,-4],"max":[10,4,-3],"kind":"concrete"}}`) // a wall ahead
	put(e.s, e.p, Vec3{0, 0, 0})
	r := &runner{s: e.s, p: e.p}
	for range 100 {
		r.cmd(Cmd{F: 1}) // ahead (yaw 0: -z), level: through the wall
	}
	if e.p.Pos[2] > -6 || e.p.Pos[1] != 0 {
		t.Fatalf("flew to %v", e.p.Pos)
	}
	for range 100 {
		stepSim(e.s)
	}
	if bot.Pos != botAt {
		t.Fatalf("a bot walked while the map is edited: %v -> %v", botAt, bot.Pos)
	}
	if out := e.run("map_playtest"); out != "playtest on" {
		t.Fatalf("playtest: %s", out)
	}
	if e.p.body.Noclip || len(e.s.Map.Nav) == 0 || !e.s.navAuto || e.s.editing() {
		t.Fatalf("playtest: noclip %v, nav %d nodes", e.p.body.Noclip, len(e.s.Map.Nav))
	}
	if out := e.run("map_op", `{"op":"box_delete","i":0}`); !strings.Contains(out, "map_playtest 0 first") {
		t.Fatalf("an edit while playtesting: %s", out)
	}
	if out := e.run("map_undo"); !strings.Contains(out, "map_playtest 0 first") {
		t.Fatalf("undo while playtesting: %s", out)
	}
	if out := e.run("map_playtest", "0"); out != "playtest off" || !e.p.body.Noclip || e.s.Map.Nav != nil {
		t.Fatalf("back to editing: %s, noclip %v, nav %v", out, e.p.body.Noclip, e.s.Map.Nav)
	}
	if out := e.run("map_edit", "stop"); out != "stopped" || e.p.body.Noclip || e.s.mapRev() != 0 {
		t.Fatalf("stop: %s", out)
	}
	e.run("map_edit", "new", "de_blank")
	e.run("changelevel", "de_slop")
	if e.s.edit != nil || e.p.body.Noclip {
		t.Fatal("changelevel left the session on")
	}
	// Not admin: nothing.
	e.p.admin = false
	if out := e.run("map_edit", "new", "de_x"); !strings.Contains(out, "not admin") {
		t.Fatalf("not admin: %s", out)
	}
}

// map_save checks the map as a custom map file and refuses it with the
// reasons; a good map is written to the maps dir and loads (OnMapSaved). The
// names: not a built-in's, a file name.
func TestMapEditSave(t *testing.T) {
	e := newEditor(t)
	if out := e.run("map_save"); !strings.Contains(out, "no map is being edited") {
		t.Fatalf("no session: %s", out)
	}
	e.run("map_edit", "new", "de_blank")
	if out := e.run("map_save"); !strings.Contains(out, "no maps dir") {
		t.Fatalf("no dir: %s", out)
	}
	dir := t.TempDir()
	e.s.MapsDir = dir
	saved := make(chan struct{}, 1)
	e.s.OnMapSaved = func() { saved <- struct{}{} }
	e.op(`{"op":"zone_delete","kind":"buy","team":"artisan","i":0}`)
	if out := e.run("map_save"); !strings.Contains(out, "not saved") || !strings.Contains(out, "buy zones") {
		t.Fatalf("a spawn outside the buy zones: %s", out)
	}
	e.run("map_undo")
	if out := e.run("map_save", "de_blank_saved"); !strings.Contains(out, "saved") { // no nav: mapcheck builds one to walk
		t.Fatalf("a blank map: %s", out)
	}
	<-saved
	e.run("map_edit", "clone", "de_slop", "de_slop_mine")
	if out := e.run("map_save", "de_slop"); !strings.Contains(out, "built-in") {
		t.Fatalf("over a built-in: %s", out)
	}
	if out := e.run("map_save"); !strings.Contains(out, "saved") {
		t.Fatalf("save: %s", out)
	}
	<-saved
	if _, err := os.Stat(filepath.Join(dir, "de_slop_mine.json")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"new", "De Bad!"}, {"clone", "de_slop", "de_standup"}, {"clone", "de_nope"}, {"new"}, {"frobnicate", "x"}} {
		if out := e.run("map_edit", bad...); !strings.Contains(out, "map_edit: ") {
			t.Errorf("map_edit %v: %s", bad, out)
		}
	}
}

// The golden patches for web/game/mappatch.js (mappatch.test.mjs): a small
// map, the patches a run of ops and undos sent, and the map after each, as
// the server's patchDoc leaves it. UPDATE=1 rewrites it.
func TestMapPatchGolden(t *testing.T) {
	e := newEditor(t)
	e.run("map_edit", "new", "de_golden")
	b, _ := json.Marshal(e.s.Map)
	var start any
	_ = json.Unmarshal(b, &start)
	e.deltas = nil
	for _, o := range []string{
		`{"op":"box_add","box":{"min":[0,0,0],"max":[2,1,2],"kind":"crate"}}`,
		`{"op":"box_add","box":{"min":[4,0,0],"max":[6,2,3],"kind":"metal","ramp":2}}`,
		`{"op":"box_delete","i":1}`,
		`{"op":"box_move","i":1,"by":[0.5,0,-0.25]}`,
		`{"op":"spawn_add","team":"vibe","pos":[-24,0,3]}`,
		`{"op":"spawn_delete","team":"vibe","i":0}`,
		`{"op":"zone_set","kind":"pit","name":"main","min":[-2,-10,-2],"max":[2,-1,2]}`,
		`{"op":"zone_set","kind":"buy","team":"artisan","i":1,"min":[20,-0.5,5],"max":[28,3,9]}`,
		`{"op":"rule_set","key":"sv_movement","value":"source"}`,
		`{"op":"rule_set","key":"mp_respawn","value":"1"}`,
		`{"op":"rule_delete","key":"sv_movement"}`,
		`{"op":"nav_set","name":"a","pos":[0,0,10]}`,
		`{"op":"nav_set","name":"b","pos":[0,0,12],"links":["a"]}`,
		`{"op":"nav_delete","name":"a"}`,
		`{"op":"item_add","weapon":"prompt","pos":[0,0,5]}`,
		`{"op":"meta_set","desc":"golden","maxPlayers":4}`,
	} {
		e.op(o)
	}
	for range 5 {
		e.run("map_undo")
	}
	e.run("map_redo")
	type step struct {
		Patch []mapPatch `json:"patch"`
		After any        `json:"after"`
	}
	var steps []step
	doc := map[string]any{}
	_ = json.Unmarshal(b, &doc)
	for _, m := range e.deltas {
		pb, _ := json.Marshal(m["patch"])
		var ps []mapPatch
		_ = json.Unmarshal(pb, &ps)
		for _, p := range ps {
			if _, err := patchDoc(doc, p); err != nil {
				t.Fatal(err)
			}
		}
		ab, _ := json.Marshal(doc)
		var after any
		_ = json.Unmarshal(ab, &after)
		steps = append(steps, step{ps, after})
	}
	got, _ := json.MarshalIndent(map[string]any{"start": start, "steps": steps}, "", " ")
	got = append(got, '\n')
	const file = "testdata/mappatch_golden.json"
	if os.Getenv("UPDATE") != "" {
		if err := os.WriteFile(file, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(file)
	if err != nil || string(want) != string(got) {
		t.Fatalf("%s is stale (UPDATE=1 go test ./internal/game -run TestMapPatchGolden): %v", file, err)
	}
}

// Every way an op, a patch or a command is refused: it says why and nothing changes.
func TestMapEditRefusals(t *testing.T) {
	e := newEditor(t)
	for _, c := range [][]string{{"map_op", `{"op":"box_delete","i":0}`}, {"map_undo"}, {"map_redo"}, {"map_playtest"}, {"map_edit", "stop"}, {"map_edit", "log"}} {
		if out := e.run(c[0], c[1:]...); !strings.Contains(out, "no map is being edited") {
			t.Errorf("%v without a session: %s", c, out)
		}
	}
	if out := e.run("map_edit"); !strings.Contains(out, "no map is being edited") {
		t.Errorf("status: %s", out)
	}
	e.run("map_edit", "new", "de_blank")
	e.op(`{"op":"item_add","weapon":"prompt","pos":[0,0,5]}`)
	e.op(`{"op":"nav_set","name":"a","pos":[0,0,10]}`)
	e.op(`{"op":"nav_set","name":"a","pos":[0,0,11]}`) // replaces
	before := mapJSON(e.s.Map)
	for _, bad := range []string{
		`{"op":"box_add"}`,
		`{"op":"box_set","i":0}`,
		`{"op":"box_set","i":5,"box":{"min":[0,0,0],"max":[1,1,1],"kind":"crate"}}`,
		`{"op":"box_move","i":0}`,
		`{"op":"box_resize","i":0,"min":[0,0,0]}`,
		`{"op":"box_resize","i":7,"min":[0,0,0],"max":[1,1,1]}`,
		`{"op":"box_material","i":9,"kind":"crate"}`,
		`{"op":"box_ramp","i":0}`,
		`{"op":"box_ramp","i":0,"ramp":7}`,
		`{"op":"box_ramp","i":3,"ramp":1}`,
		`{"op":"box_delete","i":-1}`,
		`{"op":"box_add","box":{"min":[0,0,0],"max":[1,1,1e9],"kind":"crate"}}`,
		`{"op":"spawn_move","team":"vibe","i":4,"pos":[0,0,0]}`,
		`{"op":"spawn_move","team":"vibe","i":0}`,
		`{"op":"spawn_delete","team":"vibe","i":0}`,
		`{"op":"spawn_add","team":"vibe","pos":[0,0,1e9]}`,
		`{"op":"zone_set","kind":"lava","name":"x","min":[0,0,0],"max":[1,1,1]}`,
		`{"op":"zone_set","kind":"pit","min":[0,0,0],"max":[1,1,1]}`,
		`{"op":"zone_set","kind":"buy","team":"nobody","i":0,"min":[0,0,0],"max":[1,1,1]}`,
		`{"op":"zone_set","kind":"buy","team":"vibe","i":5,"min":[0,0,0],"max":[1,1,1]}`,
		`{"op":"zone_set","kind":"site","name":"A"}`,
		`{"op":"zone_set","kind":"site","name":"A","min":[0,0,0],"max":[0,1,1]}`,
		`{"op":"zone_delete","kind":"pit","name":"none"}`,
		`{"op":"zone_delete","kind":"buy","team":"vibe","i":1}`,
		`{"op":"item_move","i":3,"pos":[0,0,0]}`,
		`{"op":"item_move","i":0}`,
		`{"op":"item_add","weapon":"prompt"}`,
		`{"op":"nav_set","pos":[0,0,0]}`,
		`{"op":"nav_set","name":"b"}`,
		`{"op":"nav_set","name":"a","pos":[0,0,0],"links":["a"]}`,
		`{"op":"nav_delete","name":"zz"}`,
		`{"op":"rule_delete","key":"sv_movement"}`,
		`{"op":"meta_set"}`,
		`{"op":"meta_set","category":"vibes"}`,
		`{"op":"meta_set","desc":"` + strings.Repeat("x", 401) + `"}`,
	} {
		if out := e.run("map_op", bad); strings.Contains(out, "rev ") || !strings.Contains(out, "map_op: ") {
			t.Errorf("map_op %s: %s", bad, out)
		}
	}
	if mapJSON(e.s.Map) != before {
		t.Fatal("a refused op changed the map")
	}
	e.op(`{"op":"zone_delete","kind":"site","name":"B"}`)
	e.op(`{"op":"item_delete","i":0}`)
	if out := e.run("map_playtest", "0"); !strings.Contains(out, "already") {
		t.Fatalf("playtest off twice: %s", out)
	}
	// Patches the server would never send itself: refused, the map as it was.
	i := func(n int) *int { return &n }
	for _, p := range []mapPatch{
		{P: nil, A: "set"},
		{P: []string{"boxes"}, A: "frob", I: i(0)},
		{P: []string{"boxes"}, A: "set", I: i(99)},
		{P: []string{"boxes"}, A: "ins", I: i(99)},
		{P: []string{"boxes"}, A: "del", I: i(-1)},
		{P: []string{"rules"}, A: "frob", K: "x"},
		{P: []string{"desc"}, A: "frob"},
		{P: []string{"boxes"}, A: "set", V: json.RawMessage(`"not boxes"`)},
		{P: []string{"desc"}, A: "set", V: json.RawMessage(`{bad`)},
	} {
		before := mapJSON(e.s.Map)
		if _, err := e.s.applyPatches([]mapPatch{p}); err == nil {
			t.Errorf("patch %+v applied", p)
		}
		if mapJSON(e.s.Map) != before {
			t.Errorf("patch %+v changed the map", p)
		}
	}
	// Key and field patches: their inverses put things back.
	before = mapJSON(e.s.Map)
	undo, err := e.s.applyPatches([]mapPatch{
		{P: []string{"rules"}, A: "set", K: "sv_movement", V: json.RawMessage(`"source"`)},
		{P: []string{"rules"}, A: "del", K: "sv_movement"},
		{P: []string{"desc"}, A: "del"},
		{P: []string{"cut"}, A: "set", V: json.RawMessage(`9`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.applyPatches(undo); err != nil || mapJSON(e.s.Map) != before {
		t.Fatalf("the inverse didn't put it back: %v", err)
	}
}
