package game

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
)

func TestSpottedByFacingWallsAndSmoke(t *testing.T) {
	s := authServer(t)
	alice := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-20, 0, 0})
	bob := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-12, 0, 0})  // in the open, east of alice
	carol := cmdPlayer(t, s, "3.3.3.3", content.Artisan, Vec3{0, 0, 0})  // inside mid (walls at x = ±6)
	dave := cmdPlayer(t, s, "4.4.4.4", content.Artisan, Vec3{-28, 0, 0}) // behind alice
	for _, p := range []*Player{bob, carol, dave} {
		p.Yaw = math.Pi / 2 // the Artisans look west: bob and carol face alice
	}
	alice.Yaw, alice.Pitch = -math.Pi/2, 0 // alice looks east
	sp := s.spotted()
	if !slices.Equal(sp[content.Vibe], []int{bob.ID}) {
		t.Fatalf("vibe sees %v, want only bob %d (carol behind a wall, dave behind alice)", sp[content.Vibe], bob.ID)
	}
	if !slices.Contains(sp[content.Artisan], alice.ID) {
		t.Fatalf("bob faces alice in the open: artisans see %v", sp[content.Artisan])
	}
	nadeAt(s, alice, "smoketest", Vec3{-16, nadeRadius, 0}, 0)
	s.tickNades(0.6)
	if got := s.spotted()[content.Vibe]; len(got) != 0 {
		t.Fatalf("a smoke between them: vibe still sees %v", got)
	}
	bob.Dead = true
	dave.Yaw = -math.Pi / 2 // dave turns east: alice's back, nothing in between
	if got := s.spotted()[content.Artisan]; !slices.Equal(got, []int{alice.ID}) {
		t.Fatalf("dave behind alice: artisans see %v", got)
	}
}

func TestSnapshotCarriesOnlyYourTeamsSpots(t *testing.T) {
	s := authServer(t)
	alice := cmdPlayer(t, s, "1.1.1.1", content.Vibe, Vec3{-20, 0, 0})
	bob := cmdPlayer(t, s, "2.2.2.2", content.Artisan, Vec3{-12, 0, 0})
	alice.Yaw, bob.Yaw = -math.Pi/2, -math.Pi/2 // alice looks at bob's back; bob doesn't see her
	msgs(alice)
	msgs(bob)
	s.broadcast()
	read := func(p *Player) []int {
		for _, m := range msgs(p) {
			if m["t"] == "snap" {
				var ids []int
				b, _ := json.Marshal(m["spot"])
				json.Unmarshal(b, &ids)
				return ids
			}
		}
		t.Fatalf("no snapshot for %s", p.Name)
		return nil
	}
	if got := read(alice); !slices.Equal(got, []int{bob.ID}) {
		t.Fatalf("alice's snapshot spots %v", got)
	}
	if got := read(bob); len(got) != 0 {
		t.Fatalf("bob's snapshot spots %v", got)
	}
}
