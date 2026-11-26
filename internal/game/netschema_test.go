package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// The replication schema's guards (netschema.go, docs/replication.md): every
// field of every wire type has a valid net tag that agrees with its json, and
// every struct type a state message reaches is in the registry.

// marshalled: whether encoding/json writes f (embedded structs are flattened).
func marshalled(f reflect.StructField) bool {
	if f.Tag.Get("json") == "-" {
		return false
	}
	return f.IsExported() || f.Anonymous
}

// floatPrecisions is the precisions a field of type t may declare: nil when
// it carries no float (then it may declare none).
func floatPrecisions(t reflect.Type) []string {
	if p, ok := netLeaves[t]; ok {
		if p == "" {
			return nil
		}
		return []string{p}
	}
	switch t.Kind() {
	case reflect.Float64, reflect.Float32:
		return []string{"exact", "angle"} // angle: a MarshalJSON rounds it (Player's yaw)
	case reflect.Pointer, reflect.Slice, reflect.Map:
		return floatPrecisions(t.Elem())
	case reflect.Array:
		if t.Elem().Kind() == reflect.Float64 {
			return []string{"exact"}
		}
		return floatPrecisions(t.Elem())
	}
	return nil // structs: their own fields say
}

// checkNetType lists what is wrong with t's net tags.
func checkNetType(t reflect.Type) []error {
	var errs []error
	bad := func(f reflect.StructField, format string, a ...any) {
		errs = append(errs, fmt.Errorf("%s.%s: %s", t.Name(), f.Name, fmt.Sprintf(format, a...)))
	}
	for i := range t.NumField() {
		f := t.Field(i)
		raw, ok := f.Tag.Lookup("net")
		if !ok {
			bad(f, `no net tag: add net:"hot|cold[,self|team][,exact|mm|angle|gait]" or net:"-" (docs/replication.md)`)
			continue
		}
		tag, err := parseNetTag(raw)
		if err != nil {
			bad(f, "%v", err)
			continue
		}
		switch m := marshalled(f); {
		case tag.Sent() && !m:
			bad(f, `net:%q but json never writes it (unexported or json:"-")`, raw)
		case !tag.Sent() && m:
			bad(f, `net:"-" but json writes it: add json:"-" or tag what it is`)
		}
		if !tag.Sent() {
			continue
		}
		allowed := floatPrecisions(f.Type)
		switch {
		case len(allowed) == 0 && tag.Precision != "":
			bad(f, "precision %q on a field without floats", tag.Precision)
		case len(allowed) > 0 && tag.Precision == "":
			bad(f, "carries floats: needs one of %v", allowed)
		case len(allowed) > 0 && !slices.Contains(allowed, tag.Precision):
			bad(f, "precision %q, but its type encodes as %v", tag.Precision, allowed)
		}
	}
	return errs
}

// walkNet follows the sent fields from roots and lists every struct type it
// reaches outside the registry, and self/team scopes that don't nest: a field
// reached through a self field must be self itself (and the same for team),
// and a scoped field may sit only in a root message or under the same scope.
func walkNet(roots, registry []reflect.Type) (reached map[reflect.Type]bool, errs []error) {
	reg := map[reflect.Type]bool{}
	for _, t := range registry {
		reg[t] = true
	}
	reached = map[reflect.Type]bool{}
	type visit struct {
		t     reflect.Type
		scope string
	}
	seen := map[visit]bool{}
	var walk func(t reflect.Type, scope, via string, root bool)
	walk = func(t reflect.Type, scope, via string, root bool) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
			t = t.Elem()
		}
		if _, leaf := netLeaves[t]; leaf || t.Kind() != reflect.Struct {
			return
		}
		if !reg[t] {
			errs = append(errs, fmt.Errorf("%s (via %s) reaches the wire but is not in netTypes", t, via))
			return
		}
		reached[t] = true
		if seen[visit{t, scope}] {
			return
		}
		seen[visit{t, scope}] = true
		for i := range t.NumField() {
			f := t.Field(i)
			tag, err := parseNetTag(f.Tag.Get("net"))
			if err != nil || !tag.Sent() {
				continue // checkNetType reports bad tags
			}
			switch {
			case scope != "" && tag.Scope != scope:
				errs = append(errs, fmt.Errorf("%s.%s (via %s): inside a %s field, so it must be %s too", t.Name(), f.Name, via, scope, scope))
			case scope == "" && tag.Scope != "" && !root:
				errs = append(errs, fmt.Errorf("%s.%s (via %s): %s, but %s also goes to everyone", t.Name(), f.Name, via, tag.Scope, t.Name()))
			}
			walk(f.Type, tag.Scope, via+"."+f.Name, false)
		}
	}
	for _, r := range roots {
		walk(r, "", r.Name(), true)
	}
	return reached, errs
}

func TestNetTagsValid(t *testing.T) {
	for _, typ := range netTypes {
		for _, err := range checkNetType(typ) {
			t.Error(err)
		}
	}
}

func TestNetRegistryCoversTheWire(t *testing.T) {
	reached, errs := walkNet(netRoots, netTypes)
	for _, err := range errs {
		t.Error(err)
	}
	for _, typ := range netTypes {
		if !reached[typ] {
			t.Errorf("%s is in netTypes but no state message reaches it: drop it or add its root", typ)
		}
	}
}

// An exported field the server keeps off the wire says why.
func TestNetNeverHasReason(t *testing.T) {
	names := map[string]bool{}
	for _, typ := range netTypes {
		names[typ.Name()] = true
	}
	fset := token.NewFileSet()
	files, _ := filepath.Glob("*.go")
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || !names[ts.Name.Name] {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, fd := range st.Fields.List {
				if fd.Tag == nil || len(fd.Names) == 0 || !fd.Names[0].IsExported() {
					continue
				}
				if reflect.StructTag(strings.Trim(fd.Tag.Value, "`")).Get("net") == netNever && fd.Doc == nil && fd.Comment == nil {
					t.Errorf("%s: %s.%s is net:\"-\" without a comment saying why", fset.Position(fd.Pos()), ts.Name.Name, fd.Names[0].Name)
				}
			}
			return false
		})
	}
}

// What the precision words promise is what encoding/json writes: a probe
// value in every float field comes back rounded as its tag says.
func TestNetPrecisionMatchesEncoding(t *testing.T) {
	const p = 0.123456789123
	want := func(prec string, x float64) float64 {
		switch prec {
		case "mm":
			return math.Round(x*1e3) / 1e3
		case "angle":
			return math.Round(x*1e4) / 1e4
		}
		return x
	}
	checked := 0
	for _, typ := range netTypes {
		v := reflect.New(typ)
		type probe struct {
			f   reflect.StructField
			tag netTag
		}
		var probes []probe
		for i := range typ.NumField() {
			f := typ.Field(i)
			tag, _ := parseNetTag(f.Tag.Get("net"))
			if !tag.Sent() || f.Anonymous || tag.Precision == "" {
				continue
			}
			fv := v.Elem().Field(i)
			switch x := fv.Addr().Interface().(type) {
			case *float64:
				*x = p
			case *Vec3:
				*x = Vec3{p, -3 * p, 7 * p}
			case *[3]float64:
				*x = [3]float64{p, -3 * p, 7 * p}
			case *Gait:
				*x = Gait{p, p / 2, p / 3, p / 4}
			default:
				continue // containers: their element types are probed on their own
			}
			probes = append(probes, probe{f, tag})
		}
		if len(probes) == 0 {
			continue
		}
		b, err := json.Marshal(v.Interface())
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		for _, pr := range probes {
			name, _, _ := strings.Cut(pr.f.Tag.Get("json"), ",")
			raw := got[name]
			var ok bool
			switch pr.f.Type {
			case reflect.TypeFor[Gait]():
				var g Gait
				_ = json.Unmarshal(raw, &g)
				ok = pr.tag.Precision == "gait" && g == (Gait{p, p / 2, p / 3, p / 4}).wire()
			case reflect.TypeFor[Vec3](), reflect.TypeFor[[3]float64]():
				var a [3]float64
				_ = json.Unmarshal(raw, &a)
				ok = a == [3]float64{want(pr.tag.Precision, p), want(pr.tag.Precision, -3*p), want(pr.tag.Precision, 7*p)}
			default:
				var x float64
				_ = json.Unmarshal(raw, &x)
				ok = x == want(pr.tag.Precision, p)
			}
			if !ok {
				t.Errorf("%s.%s is net precision %q but encodes as %s", typ.Name(), pr.f.Name, pr.tag.Precision, raw)
			}
			checked++
		}
	}
	if checked < 20 {
		t.Fatalf("probed only %d float fields", checked)
	}
}

// The checker itself, on fixtures: each one breaks one rule.
type (
	fxOK struct {
		A int     `json:"a" net:"cold"`
		B Vec3    `json:"b" net:"hot,mm"`
		C float64 `json:"c,omitempty" net:"hot,self,exact"`
		d int     `net:"-"`
		E int     `json:"-" net:"-"` // why: test
	}
	fxUntagged    struct{ A int }
	fxUnknownWord struct {
		A int `net:"warm"`
	}
	fxTwoClasses struct {
		A int `net:"hot,cold"`
	}
	fxTwoPrecisions struct {
		A float64 `net:"hot,exact,mm"`
	}
	fxTwoScopes struct {
		A int `net:"cold,self,team"`
	}
	fxModsOnNever struct {
		A int `json:"-" net:"-,self"`
	}
	fxNeverButSent struct {
		A int `json:"a" net:"-"`
	}
	fxSentButHidden struct {
		A int `json:"-" net:"cold"`
	}
	fxSentUnexported struct {
		a int `net:"cold"`
	}
	fxNoPrecision struct {
		A float64 `net:"cold"`
	}
	fxWrongPrecision struct {
		A Vec3 `net:"cold,exact"`
	}
	fxPrecisionOnInt struct {
		A int `net:"cold,mm"`
	}
	fxFloatsInSlice struct {
		A []float64 `net:"cold"`
	}
)

func TestNetCheckerCatches(t *testing.T) {
	if errs := checkNetType(reflect.TypeFor[fxOK]()); len(errs) > 0 {
		t.Errorf("fxOK: %v", errs)
	}
	_ = fxOK{}.d
	_ = fxSentUnexported{}.a
	for _, typ := range []reflect.Type{
		reflect.TypeFor[fxUntagged](), reflect.TypeFor[fxUnknownWord](), reflect.TypeFor[fxTwoClasses](),
		reflect.TypeFor[fxTwoPrecisions](), reflect.TypeFor[fxTwoScopes](), reflect.TypeFor[fxModsOnNever](),
		reflect.TypeFor[fxNeverButSent](), reflect.TypeFor[fxSentButHidden](), reflect.TypeFor[fxSentUnexported](),
		reflect.TypeFor[fxNoPrecision](), reflect.TypeFor[fxWrongPrecision](), reflect.TypeFor[fxPrecisionOnInt](),
		reflect.TypeFor[fxFloatsInSlice](),
	} {
		errs := checkNetType(typ)
		if len(errs) != 1 {
			t.Errorf("%s: want 1 error, got %v", typ.Name(), errs)
		} else {
			t.Logf("%s: %v", typ.Name(), errs[0])
		}
	}
}

type (
	fxRoot struct {
		A fxRegistered   `json:"a" net:"cold"`
		B *fxUnlisted    `json:"b" net:"cold"`
		S []fxSelfChild  `json:"s" net:"cold,self"`
		M map[string]int `json:"m" net:"cold"`
	}
	fxRegistered struct {
		X int `json:"x" net:"cold,self"` // self, but fxRoot.A goes to everyone
	}
	fxUnlisted struct {
		Y int `net:"cold"`
	}
	fxSelfChild struct {
		Z int `json:"z" net:"cold"` // must be self: it sits in a self field
	}
)

func TestNetWalkCatches(t *testing.T) {
	_, errs := walkNet([]reflect.Type{reflect.TypeFor[fxRoot]()},
		[]reflect.Type{reflect.TypeFor[fxRoot](), reflect.TypeFor[fxRegistered](), reflect.TypeFor[fxSelfChild]()})
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{"fxUnlisted (via fxRoot.B) reaches the wire but is not in netTypes", "fxRegistered.X", "fxSelfChild.Z"} {
		if !strings.Contains(all, want) {
			t.Errorf("walk errors miss %q:\n%s", want, all)
		}
	}
	if len(errs) != 3 {
		t.Errorf("want 3 errors, got:\n%s", all)
	}
}

// snapMsg (the declared schema) and snapBytes (the hand-built snapshot) give
// the same bytes for a populated world.
func TestSnapMsgMatchesSnapBytes(t *testing.T) {
	s := authServer(t)
	s.Test = true
	s.lastTickAt = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) // game time: s.clock() stands still
	var ps []*Player
	for i := range 4 {
		p := s.join(fmt.Sprintf("10.0.%d.1", i))
		p.Name, p.ready = fmt.Sprintf("p%d", i), true
		p.Team = []string{content.Vibe, content.Artisan, content.Vibe, content.Spectator}[i]
		s.respawn(p)
		if p.Team != content.Spectator {
			p.inv = s.Weapons.Loadout(p.Team)
		}
		p.Yaw, p.Pitch = 0.123456789*float64(i+1), -0.2
		ps = append(ps, p)
	}
	ps[0].Money, ps[0].Armor, ps[0].Helmet = 1234, 55, true
	s.items = append(s.items, &Item{ID: 7, Weapon: "lint", Pos: Vec3{1.23456, 0.5, -2.0001}})
	s.nades = append(s.nades, &Nade{ID: 3, Weapon: "nade", Kind: "frag", Pos: Vec3{1, 2, 3.33333}, Vel: Vec3{0.1, -9.81, 0}, State: "fly"})
	s.boats = append(s.boats, &Boat{ID: 1, Pos: Vec3{-12.3456, -0.6, 4}, Yaw: 1.23456789, Vel: Vec3{3.3333, 0, -1}})
	drain := func(p *Player) []byte {
		var last []byte
		for {
			select {
			case b := <-p.send:
				if bytes.HasPrefix(b, []byte(`{"t":"snap"`)) {
					last = b
				}
			default:
				return last
			}
		}
	}
	for _, p := range ps {
		drain(p)
	}
	s.broadcastFull()
	for _, p := range ps {
		got := drain(p)
		if got == nil {
			t.Fatalf("player %d got no snapshot", p.ID)
		}
		// sendSnapshot lists the players in map order: take the order it sent.
		var order struct {
			Players []struct{ ID int } `json:"players"`
		}
		_ = json.Unmarshal(got, &order)
		var list []*Player
		for _, o := range order.Players {
			list = append(list, s.players[o.ID])
		}
		s.mu.Lock()
		m := snapMsg{T: "snap", Time: s.clock().UnixMilli(), Tick: s.tick, Players: list, Items: s.items, Nades: s.nades, Boats: s.boats, Cars: s.cars, Me: s.meStateOf(p)}
		if ids, ok := s.spotted()[p.Team]; ok {
			m.Spot = &ids
		}
		r := s.roundMsgOf(s.clock())
		m.Round = &r
		want, err := json.Marshal(m)
		s.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("player %d (%s):\nsnapBytes: %s\nsnapMsg:   %s", p.ID, p.Team, got, want)
		}
	}
}

// poseGaps: Player's hot fields that pose snapshots leave out today (docs/replication.md).
var poseGaps = map[string]string{
	"g":  "the gait rides full snapshots only; clients interpolate it between them (GaitBetween)",
	"hk": "the hook's rope (where it is latched) rides full snapshots only: drawn, not hit or collided with",
}

// A pose snapshot row ([id, x, y, z, yaw, pitch, flags], posesJSON) is
// Player's hot fields, encoded as the full snapshot encodes them.
func TestPoseRowsAreHotFields(t *testing.T) {
	var hot []string
	pt := reflect.TypeFor[Player]()
	for i := range pt.NumField() {
		f := pt.Field(i)
		tag, _ := parseNetTag(f.Tag.Get("net"))
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if tag.Class == netHot && poseGaps[name] == "" {
			hot = append(hot, name)
		}
	}
	slices.Sort(hot)
	if want := []string{"c", "gv", "p", "pitch", "yaw"}; !slices.Equal(hot, want) {
		t.Fatalf("Player's hot fields %v, pose rows carry %v: put a new hot field in posesJSON (or poseGaps, with why)", hot, want)
	}
	s := NewServer()
	p := &Player{ID: 9, Pos: Vec3{1.23456, -0.0004, 7.77777}, Yaw: 2.718281828, Pitch: -0.333333, Crouch: true, Grav: 5, ready: true, Team: content.Vibe}
	var rows [][]float64
	if err := json.Unmarshal(s.posesJSON([]*Player{p}), &rows); err != nil {
		t.Fatal(err)
	}
	var full struct {
		ID    int        `json:"id"`
		P     [3]float64 `json:"p"`
		Yaw   float64    `json:"yaw"`
		Pitch float64    `json:"pitch"`
		C     bool       `json:"c"`
		Gv    int        `json:"gv"`
	}
	b, _ := json.Marshal(p)
	_ = json.Unmarshal(b, &full)
	r := rows[0]
	if int(r[0]) != full.ID || r[1] != full.P[0] || r[2] != full.P[1] || r[3] != full.P[2] || r[4] != full.Yaw || r[5] != full.Pitch || (int(r[6])&1 == 1) != full.C || int(r[6])>>2 != full.Gv || full.Gv != 5 {
		t.Errorf("pose row %v, full snapshot %+v", r, full)
	}
}
