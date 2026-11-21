package game

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

func TestChangeLevelLoadsMapAndRespawnsBySide(t *testing.T) {
	s := authServer(t)
	a := s.join("1.1.1.1")
	a.Team = content.Artisan
	v := s.join("2.2.2.2")
	v.Team = content.Vibe
	msgs(a)
	s.adminCommand(a, "changelevel", []string{"de_stack"})
	if s.Map.Name != "de_stack" {
		t.Fatalf("map %s", s.Map.Name)
	}
	if got := ofType(v, "map"); len(got) != 1 {
		t.Fatalf("map messages to the other player: %d", len(got))
	}
	in := func(p *Player) bool {
		for _, sp := range s.Map.TeamSpawns[p.Team] {
			if Vec3(sp) == p.Pos {
				return true
			}
		}
		return false
	}
	if !in(a) || !in(v) {
		t.Fatalf("not at their side's spawns: artisan %v, vibe %v", a.Pos, v.Pos)
	}
}

func TestMapRotationCvar(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	if got := fmt.Sprint(s.rotation()); got != fmt.Sprint(maps.Order) {
		t.Fatalf("default rotation %s", got)
	}
	s.adminCommand(p, "mp_maprotation", []string{"de_stack", "de_slop"})
	if s.cvars["mp_maprotation"] != "de_stack,de_slop" {
		t.Fatalf("rotation %q", s.cvars["mp_maprotation"])
	}
	msgs(p)
	s.adminCommand(p, "mp_maprotation", []string{"de_nope"})
	if !hasText(texts(p), `unknown map "de_nope"`) || s.cvars["mp_maprotation"] != "de_stack,de_slop" {
		t.Fatalf("bad map accepted: %q", s.cvars["mp_maprotation"])
	}
	// Only one other map: the vote offers it and staying.
	if got := fmt.Sprint(s.voteOptions()); got != "[de_stack de_slop]" {
		t.Fatalf("options %s", got)
	}
	s.adminCommand(p, "mp_maprotation", []string{"all"})
	if got := fmt.Sprint(s.voteOptions()); got != "[de_monorepo de_standup de_stack]" {
		t.Fatalf("options %s", got)
	}
}

func TestMapVote(t *testing.T) {
	s := authServer(t)
	s.lastTickAt = time.Now()
	a, b, c := s.join("1.1.1.1"), s.join("2.2.2.2"), s.join("3.3.3.3")
	s.adminCommand(a, "mapvote", []string{"5"})
	if s.vote == nil {
		t.Fatal("no vote")
	}
	m := ofType(c, "mapvote")
	if len(m) != 1 || fmt.Sprint(m[0]["options"]) != "[de_monorepo de_standup de_stack]" || m[0]["endsIn"].(float64) != 5 {
		t.Fatalf("vote message %v", m)
	}
	s.handle(a, inbound{T: "vote", Map: "de_stack"})
	s.handle(b, inbound{T: "vote", Map: "de_standup"})
	s.handle(b, inbound{T: "vote", Map: "de_stack"}) // changed their mind
	s.handle(c, inbound{T: "vote", Map: "de_slop"})  // not an option: ignored
	m = ofType(c, "mapvote")
	last := m[len(m)-1]["votes"].(map[string]any)
	if last["de_stack"] != 2.0 || last["de_standup"] != 0.0 {
		t.Fatalf("votes %v", last)
	}
	s.tickVote(s.lastTickAt.Add(4 * time.Second))
	if s.Map.Name != "de_slop" {
		t.Fatal("vote ended early")
	}
	s.tickVote(s.lastTickAt.Add(5 * time.Second))
	if s.Map.Name != "de_stack" || s.vote != nil {
		t.Fatalf("map %s after the vote", s.Map.Name)
	}
	done := ofType(a, "mapvote")
	if len(done) == 0 || done[len(done)-1]["winner"] != "de_stack" {
		t.Fatalf("no result: %v", done)
	}
}

func TestMapVoteEndNow(t *testing.T) {
	s := authServer(t)
	s.lastTickAt = time.Now()
	p := s.join("1.1.1.1")
	s.adminCommand(p, "mapvote", []string{"60"})
	s.handle(p, inbound{T: "vote", Map: "de_standup"})
	s.adminCommand(p, "mapvote", []string{"end"})
	s.advance(s.lastTickAt.Add(time.Second / SimHz))
	if s.Map.Name != "de_standup" {
		t.Fatalf("map %s", s.Map.Name)
	}
}

func TestMapVoteNoVotesTakesTheNextMap(t *testing.T) {
	s := authServer(t)
	s.lastTickAt = time.Now()
	s.join("1.1.1.1")
	s.StartMapVote(time.Second)
	s.advance(s.lastTickAt.Add(2 * time.Second)) // the server tick ends it
	if s.Map.Name != "de_monorepo" {
		t.Fatalf("map %s", s.Map.Name)
	}
}

func TestMapCatalog(t *testing.T) {
	cat := MapCatalog()
	if len(cat) != len(maps.All) || cat[0].Name != maps.Order[0] {
		t.Fatalf("catalog %d maps, first %s", len(cat), cat[0].Name)
	}
	for _, m := range cat {
		if m.Size[0] <= 0 || m.Size[1] <= 0 || len(m.Boxes) == 0 || m.Desc == "" {
			t.Errorf("%s: size %v, %d boxes, desc %q", m.Name, m.Size, len(m.Boxes), m.Desc)
		}
	}
}

// The vote stays in the current map's category: a competitive match votes
// among competitive maps, a training one among training maps; a rotation
// with no other map of that category falls back to the whole rotation.
func TestVoteOptionsFollowTheCategory(t *testing.T) {
	s := authServer(t)
	cats := map[string][]string{}
	for _, n := range maps.Order {
		c := maps.All[n]().Category
		cats[c] = append(cats[c], n)
	}
	for cat, names := range cats {
		if len(names) < 2 {
			continue // nothing to vote between
		}
		s.Map = maps.All[names[0]]()
		opts := s.voteOptions()
		if len(opts) == 0 {
			t.Fatalf("%s: no options", names[0])
		}
		for _, o := range opts {
			if maps.All[o]().Category != cat {
				t.Errorf("%s (%s): option %s is %s", names[0], cat, o, maps.All[o]().Category)
			}
		}
	}
	// The only competitive map in a mixed rotation: the vote offers the rest.
	var comp string
	for _, n := range maps.Order {
		if maps.All[n]().Category == content.Competitive {
			comp = n
			break
		}
	}
	if comp == "" {
		t.Skip("no competitive map")
	}
	s.Map = maps.All[comp]()
	s.cvars["mp_maprotation"] = comp + ",de_slop,de_stack"
	if got := fmt.Sprint(s.voteOptions()); got != "[de_slop de_stack]" {
		t.Fatalf("mixed rotation from %s: %s", comp, got)
	}
}

// Lab maps (maps.Labs) load with changelevel, lights and all, but stay out of the
// rotation and the selector's catalog.
func TestChangeLevelToALab(t *testing.T) {
	s := authServer(t)
	a := s.join("1.1.1.1")
	a.Team = content.Artisan
	msgs(a)
	s.adminCommand(a, "changelevel", []string{"lab_lights"})
	if s.Map.Name != "lab_lights" || len(s.Map.Lights) == 0 {
		t.Fatalf("map %s with %d lights", s.Map.Name, len(s.Map.Lights))
	}
	if got := ofType(a, "map"); len(got) != 1 || !strings.Contains(fmt.Sprint(got[0]), "lights") {
		t.Fatalf("map message: %v", got)
	}
	for _, m := range MapCatalog() {
		if m.Name == "lab_lights" {
			t.Fatal("a lab is in the catalog")
		}
	}
	if slices.Contains(s.rotation(), "lab_lights") {
		t.Fatal("a lab is in the rotation")
	}
}
