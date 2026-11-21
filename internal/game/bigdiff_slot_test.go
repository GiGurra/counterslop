package game

import (
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// The Big Diff is an inventory slot (5) kept in step with the round: the
// carrier has it, undrawn on arrival; G with it drawn drops it in front; a
// pickup puts it back in slot 5 without drawing it; death takes it; nobody
// buys or gives it; a round with nobody carrying it leaves nobody holding it.
func TestBigDiffSlotFollowsTheRound(t *testing.T) {
	s, v, a := roundServer(t)
	v2 := s.join("3.3.3.3")
	v2.ready, v2.Team = true, content.Vibe
	now := time.Now()
	s.round.Phase = PhaseWarmup
	ticks(s, 2, &now, map[*Player]Cmd{v2: {}})
	carrier, other := v, v2
	if s.round.Bomb.Carrier == v2.ID {
		carrier, other = v2, v
	}
	if carrier.inv.Slots[BombSlot] == nil || carrier.inv.Current == BombSlot || other.inv.Slots[BombSlot] != nil || a.inv.Slots[BombSlot] != nil {
		t.Fatalf("round start: carrier %v (cur %q), other %v, artisan %v", carrier.inv.Slots[BombSlot], carrier.inv.Current, other.inv.Slots[BombSlot], a.inv.Slots[BombSlot])
	}
	// G with a gun drawn drops the gun, not The Big Diff; drawn, G drops it in front.
	ticks(s, 2, &now, map[*Player]Cmd{carrier: {W: BombSlot}, other: {}})
	if carrier.inv.Current != BombSlot {
		t.Fatalf("slot 5 didn't draw it: current %q", carrier.inv.Current)
	}
	carrier.Yaw = 0
	s.drop(carrier)
	b := s.round.Bomb
	if b.State != BombDropped || carrier.inv.Slots[BombSlot] != nil || carrier.inv.Current == BombSlot {
		t.Fatalf("after G: bomb %+v, carrier slot %v current %q", b, carrier.inv.Slots[BombSlot], carrier.inv.Current)
	}
	if d := math.Hypot(b.Pos[0]-carrier.Pos[0], b.Pos[2]-carrier.Pos[2]); d < 1 {
		t.Fatalf("dropped %.2f m away, want in front", d)
	}
	// Picking it up: slot 5, not drawn.
	s.teleport(other, b.Pos, 0, false, other.lastQueued)
	ticks(s, 1, &now, map[*Player]Cmd{carrier: {}, other: {}})
	if s.round.Bomb.Carrier != other.ID || other.inv.Slots[BombSlot] == nil || other.inv.Current == BombSlot {
		t.Fatalf("pickup: bomb %+v, slot %v current %q", s.round.Bomb, other.inv.Slots[BombSlot], other.inv.Current)
	}
	// Dying drops it and the slot goes.
	s.killBy(a, other, "prompt", false)
	if s.round.Bomb.State != BombDropped || other.inv.Slots[BombSlot] != nil {
		t.Fatalf("death: bomb %+v slot %v", s.round.Bomb, other.inv.Slots[BombSlot])
	}
	// Not for sale, not for give.
	if why := s.purchase(carrier, "bigdiff"); why == "" {
		t.Fatal("bought The Big Diff")
	}
	s.adminCommand(carrier, "give", []string{"bigdiff"})
	if carrier.inv.Slots[BombSlot] != nil {
		t.Fatal("give handed out The Big Diff")
	}
	// The round goes away (rounds off): nobody keeps it.
	s.round.Bomb = bombState{State: BombCarried, Carrier: carrier.ID}
	s.syncBombSlots()
	s.round.Bomb = bombState{}
	s.syncBombSlots()
	if carrier.inv.Slots[BombSlot] != nil {
		t.Fatal("slot 5 kept without a round")
	}
}

// The Big Diff never fires: no shot, no ammo, no trigger state (arsenal.js skips it the same way).
func TestBigDiffNeverFires(t *testing.T) {
	r := registry(t)
	inv := &Inventory{Slots: map[string]*Held{BombSlot: r.newHeld("bigdiff")}, Current: BombSlot}
	for tick := int64(1); tick < 200; tick++ {
		if w, why := r.trigger(inv, tick, true); w != nil || why != "plant" {
			t.Fatalf("tick %d: fired %v (%q)", tick, w, why)
		}
	}
	if inv.trigger || inv.nextShot != 0 {
		t.Fatalf("trigger %v nextShot %d, want untouched", inv.trigger, inv.nextShot)
	}
}
