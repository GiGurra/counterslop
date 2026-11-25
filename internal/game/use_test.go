package game

import (
	"testing"
	"time"
)

// E (+use): defuse > take the floor weapon in sight > plant (draws The Big
// Diff in a site; letting go puts the gun back). Fire-to-plant with it in
// hand stays (rounds_test.go TestPlantExplodeAndDefuse).

// onSiteA: a live round with the carrier v standing on site A's platform
// (as TestPlantExplodeAndDefuse), landed.
func onSiteA(t *testing.T) (*Server, *Player, *Player, *time.Time) {
	t.Helper()
	s, v, a := roundServer(t)
	now := time.Now()
	ticks(s, 2, &now, nil)
	site := s.Map.Sites[0]
	at := Vec3{(site.Min[0]+site.Max[0])/2 - 3, 1.5, (site.Min[2] + site.Max[2]) / 2}
	s.teleport(v, at, 0, false, v.lastQueued)
	ticks(s, 30, &now, map[*Player]Cmd{v: {}})
	if !s.inSite(v.Pos) || v.inv.Slots[BombSlot] == nil {
		t.Fatalf("carrier at %v slots %v: want in site A with The Big Diff", v.Pos, v.inv.Slots)
	}
	return s, v, a, &now
}

func TestUsePlantsInASite(t *testing.T) {
	s, v, _, now := onSiteA(t)
	held := v.inv.Current
	ticks(s, 1, now, map[*Player]Cmd{v: {B: BtnUse}})
	if v.inv.Current != BombSlot {
		t.Fatalf("E in a site: holding %q, want The Big Diff drawn", v.inv.Current)
	}
	ticks(s, int(plantSecs*SimHz)+2, now, map[*Player]Cmd{v: {B: BtnUse}})
	if s.round.Phase != PhasePlanted || s.round.Bomb.State != BombPlanted {
		t.Fatalf("phase %q bomb %+v after holding E, want planted", s.round.Phase, s.round.Bomb)
	}
	if v.inv.Current != held || v.inv.useBack != "" {
		t.Fatalf("after the plant: holding %q (back %q), want %q", v.inv.Current, v.inv.useBack, held)
	}
	ticks(s, 2, now, map[*Player]Cmd{v: {}}) // letting go after: nothing more
	if v.inv.Current != held {
		t.Fatalf("after letting go: holding %q, want %q", v.inv.Current, held)
	}
}

func TestUseReleasedCancelsThePlant(t *testing.T) {
	s, v, _, now := onSiteA(t)
	held := v.inv.Current
	ticks(s, int(plantSecs*SimHz)/2, now, map[*Player]Cmd{v: {B: BtnUse}})
	if s.round.Bomb.Progress < 0.4 {
		t.Fatalf("bomb %+v: holding E isn't planting", s.round.Bomb)
	}
	ticks(s, 1, now, map[*Player]Cmd{v: {}})
	if s.round.Bomb.State != BombCarried || s.round.Bomb.Progress != 0 || s.round.Bomb.Worker != 0 {
		t.Fatalf("bomb %+v after letting go of E, want the plant cancelled", s.round.Bomb)
	}
	if v.inv.Current != held || v.inv.Slots[BombSlot] == nil {
		t.Fatalf("after letting go: holding %q slots %v, want %q back and The Big Diff in slot 5", v.inv.Current, v.inv.Slots, held)
	}
}

func TestUseOutsideASiteDoesNothing(t *testing.T) {
	s, v, _ := roundServer(t)
	now := time.Now()
	ticks(s, 2, &now, nil)
	ticks(s, 30, &now, map[*Player]Cmd{v: {}})
	if s.inSite(v.Pos) {
		t.Fatalf("spawn %v is in a site", v.Pos)
	}
	held := v.inv.Current
	ticks(s, int(plantSecs*SimHz)+2, &now, map[*Player]Cmd{v: {B: BtnUse}})
	if v.inv.Current != held || s.round.Bomb.State != BombCarried || s.round.Bomb.Progress != 0 {
		t.Fatalf("E outside a site: holding %q bomb %+v, want nothing", v.inv.Current, s.round.Bomb)
	}
}

// Drawn with 5 (not by E): E plants too, and letting go keeps it in hand.
func TestUseWithTheBigDiffInHandKeepsIt(t *testing.T) {
	s, v, _, now := onSiteA(t)
	ticks(s, 1, now, map[*Player]Cmd{v: {W: BombSlot}})
	ticks(s, 20, now, map[*Player]Cmd{v: {B: BtnUse}})
	ticks(s, 1, now, map[*Player]Cmd{v: {}})
	if v.inv.Current != BombSlot {
		t.Fatalf("holding %q, want The Big Diff still drawn (5 drew it, not E)", v.inv.Current)
	}
}

// itemAhead: a floor weapon 1 m in front of p's feet (p looks along yaw 0, down a bit).
func itemAhead(s *Server, p *Player, h Held) *Item {
	return s.spawnItem(h, Vec3{p.Pos[0], p.Pos[1], p.Pos[2] - 1}, -1)
}

const lookDown = -0.9 // pitch at a weapon on the floor 1 m ahead

func TestUseTakesTheWeaponInSightSwappingYours(t *testing.T) {
	s, v, _, now := liveRound(t)
	v.inv.Slots["primary"] = &Held{ID: "prompt", Ammo: 7, Reserve: 20}
	v.inv.Current = "sidearm"
	ticks(s, 30, now, map[*Player]Cmd{v: {Pitch: lookDown}})
	it := itemAhead(s, v, Held{ID: "codereview", Ammo: 5, Reserve: 10})
	ticks(s, 1, now, map[*Player]Cmd{v: {B: BtnUse, Pitch: lookDown}})
	if h := v.inv.Slots["primary"]; h == nil || h.ID != "codereview" || h.Ammo != 5 || v.inv.Current != "primary" {
		t.Fatalf("after E: primary %+v current %q, want the Code Review in hand", h, v.inv.Current)
	}
	got := droppedItems(s)
	if len(got) != 1 || got[0].Weapon != "prompt" || got[0].held.Ammo != 7 || got[0].Pos != it.Pos {
		t.Fatalf("on the floor %+v, want the Prompt (7/20) where the Code Review lay", got)
	}
	ticks(s, 5, now, map[*Player]Cmd{v: {B: BtnUse, Pitch: lookDown}}) // held E: one take per press
	if v.inv.Slots["primary"].ID != "codereview" {
		t.Fatal("holding E swapped back")
	}
}

func TestUseIgnoresAWeaponOutOfReachOrSight(t *testing.T) {
	s, v, _, now := liveRound(t)
	v.inv.Slots["primary"] = s.Weapons.newHeld("prompt")
	ticks(s, 30, now, map[*Player]Cmd{v: {}})
	before := v.inv.Slots["primary"].ID
	s.spawnItem(Held{ID: "codereview", Ammo: 5}, Vec3{v.Pos[0], v.Pos[1], v.Pos[2] - 3}, -1) // 3 m: too far
	s.spawnItem(Held{ID: "codereview", Ammo: 5}, Vec3{v.Pos[0], v.Pos[1], v.Pos[2] + 1}, -1) // behind
	ticks(s, 1, now, map[*Player]Cmd{v: {B: BtnUse, Pitch: lookDown}})
	if v.inv.Slots["primary"].ID != before || len(droppedItems(s)) != 2 {
		t.Fatalf("took %q; floor %v", v.inv.Slots["primary"].ID, droppedItems(s))
	}
}

// Take before plant: E in a site aimed at a weapon takes it, The Big Diff stays put.
func TestUseTakesBeforePlanting(t *testing.T) {
	s, v, _, now := onSiteA(t)
	ticks(s, 1, now, map[*Player]Cmd{v: {Pitch: lookDown}})
	itemAhead(s, v, Held{ID: "codereview", Ammo: 5, Reserve: 10})
	ticks(s, 1, now, map[*Player]Cmd{v: {B: BtnUse, Pitch: lookDown}})
	if h := v.inv.Slots["primary"]; v.inv.Current != "primary" || h == nil || h.ID != "codereview" {
		t.Fatalf("holding %q %+v, want the Code Review taken (not The Big Diff drawn)", v.inv.Current, v.inv.Slots["primary"])
	}
	ticks(s, 20, now, map[*Player]Cmd{v: {B: BtnUse, Pitch: lookDown}})
	if s.round.Bomb.Progress != 0 || v.inv.Current == BombSlot {
		t.Fatalf("bomb %+v holding %q: the take's E press went on to plant", s.round.Bomb, v.inv.Current)
	}
}

// Defuse before take: an Artisan by the planted Big Diff reverts it with E,
// the weapon in sight stays on the floor.
func TestUseDefusesBeforeTaking(t *testing.T) {
	s, v, a, now := onSiteA(t)
	ticks(s, int(plantSecs*SimHz)+2, now, map[*Player]Cmd{v: {B: BtnUse}})
	if s.round.Phase != PhasePlanted {
		t.Fatalf("phase %q, want planted", s.round.Phase)
	}
	b := s.round.Bomb.Pos
	s.teleport(a, Vec3{b[0], b[1], b[2] + 1.5}, 0, false, a.lastQueued)
	ticks(s, 30, now, map[*Player]Cmd{a: {Pitch: lookDown}})
	itemAhead(s, a, Held{ID: "codereview", Ammo: 5, Reserve: 10})
	n := len(droppedItems(s))
	ticks(s, 1, now, map[*Player]Cmd{a: {B: BtnUse, Pitch: lookDown}})
	if h := a.inv.Slots["primary"]; len(droppedItems(s)) != n || h != nil && h.ID == "codereview" {
		t.Fatalf("an Artisan on the Big Diff took the weapon instead of defusing")
	}
	if s.round.Bomb.Worker != a.ID {
		t.Fatalf("bomb %+v: E isn't defusing", s.round.Bomb)
	}
}

// A map-placed gun taken with E comes back after mapItemRespawn, as when walked over.
func TestUseTakesAMapGunWhichRespawns(t *testing.T) {
	s, v, _, now := liveRound(t)
	ticks(s, 30, now, map[*Player]Cmd{v: {Pitch: lookDown}})
	s.items = nil
	s.spawnItem(Held{ID: "codereview", Ammo: 5}, Vec3{v.Pos[0], v.Pos[1], v.Pos[2] - 1}, 0)
	ticks(s, 1, now, map[*Player]Cmd{v: {B: BtnUse, Pitch: lookDown}})
	if h := v.inv.Slots["primary"]; h == nil || h.ID != "codereview" {
		t.Fatalf("didn't take the map's gun: %+v", v.inv.Slots["primary"])
	}
	if _, ok := s.itemRespawn[0]; !ok {
		t.Fatal("the map's gun isn't scheduled to come back")
	}
}

// The shared rule on its own (the client runs the same, shared.gen.js).
func TestUseDrawRule(t *testing.T) {
	r := registry(t)
	fresh := func() *Inventory {
		inv := r.PistolLoadout()
		inv.Slots[BombSlot] = r.newHeld("bigdiff")
		return inv
	}
	inv := fresh()
	if r.UseDraw(inv, BtnUse, 0, false, false, 1) || inv.Current != "sidearm" {
		t.Fatalf("outside a site: %q", inv.Current)
	}
	if r.UseDraw(inv, BtnUse, 0, true, true, 1) || inv.Current != "sidearm" {
		t.Fatalf("aimed at a floor weapon: %q", inv.Current)
	}
	if r.UseDraw(inv, BtnUse, BtnUse, true, false, 1) || inv.Current != "sidearm" {
		t.Fatalf("held, not pressed: %q", inv.Current)
	}
	if !r.UseDraw(inv, BtnUse, 0, true, false, 1) || inv.Current != BombSlot || inv.useBack != "sidearm" {
		t.Fatalf("pressed in a site: %q back %q", inv.Current, inv.useBack)
	}
	if r.UseDraw(inv, BtnUse, BtnUse, true, false, 2) || inv.Current != BombSlot {
		t.Fatalf("still held: %q", inv.Current)
	}
	if !r.UseDraw(inv, 0, BtnUse, true, false, 3) || inv.Current != "sidearm" || inv.useBack != "" {
		t.Fatalf("let go: %q back %q", inv.Current, inv.useBack)
	}
	inv = fresh()
	r.UseDraw(inv, BtnUse, 0, true, false, 1)
	r.switchTo(inv, "melee", 2) // a switch of your own wins
	r.switchTo(inv, BombSlot, 3)
	if r.UseDraw(inv, 0, BtnUse, true, false, 4) || inv.Current != BombSlot {
		t.Fatalf("let go after drawing it with 5: %q", inv.Current)
	}
	inv = fresh()
	delete(inv.Slots, BombSlot)
	if r.UseDraw(inv, BtnUse, 0, true, false, 1) || inv.Current != "sidearm" {
		t.Fatalf("no Big Diff carried: %q", inv.Current)
	}
}

func TestAimedItem(t *testing.T) {
	eye := [3]float64{0, 1.6, 0}
	items := [][3]float64{{0, 0, 3}, {0, 0, -1.4}, {0.6, 0, -1.4}}
	if got := AimedItem(eye, 0, -0.85, items); got != 1 { // yaw 0 looks along -z
		t.Fatalf("looking down at the one 1.4 m ahead: %d", got)
	}
	if got := AimedItem(eye, 0, 0, items); got != -1 {
		t.Fatalf("looking ahead, over them: %d", got)
	}
	if got := AimedItem(eye, 3.14159, -0.5, items); got != -1 {
		t.Fatalf("the one behind is 3 m away: %d", got)
	}
}
