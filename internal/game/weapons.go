package game

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/gigurra/counterslop/internal/sim"
)

func LoadWeapons(b []byte) (*Registry, error) {
	var r Registry
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("weapons.json: %w", err)
	}
	return NewRegistry(r.Weapons, r.Slots), nil // as the client builds it (arsenal.js)
}

func (r *Registry) newHeld(id string) *Held {
	w := r.Get(id)
	if w == nil {
		return nil
	}
	return &Held{ID: id, Ammo: w.Mag, Reserve: w.Reserve}
}

// Loadout until the buy menu exists: the Stub, the keyboard and the team rifle
// (the Prompt for Vibe Coders and anyone without a team, the Linter for Artisans).
func (r *Registry) Loadout(team string) *Inventory {
	rifle := "prompt"
	if team == "artisan" {
		rifle = "linter"
	}
	inv := &Inventory{Slots: map[string]*Held{}, Current: "primary"}
	for _, id := range []string{rifle, "stub", "keyboard"} {
		if h := r.newHeld(id); h != nil {
			inv.Slots[r.Get(id).Slot] = h
		}
	}
	return inv
}

// PistolLoadout is what a round starts with once the buy menu decides the
// rest: the Stub and the keyboard, the Stub in hand.
func (r *Registry) PistolLoadout() *Inventory {
	inv := &Inventory{Slots: map[string]*Held{}, Current: "sidearm"}
	for _, id := range []string{"stub", "keyboard"} {
		if h := r.newHeld(id); h != nil {
			inv.Slots[r.Get(id).Slot] = h
		}
	}
	return inv
}

// Item is a weapon lying on the ground (dropped, or placed by the map).
type Item struct {
	ID      int    `json:"id" net:"cold"`
	Weapon  string `json:"w" net:"cold"`
	Pos     Vec3   `json:"p" net:"cold,mm"`
	held    Held   `net:"-"`
	mapSlot int    `net:"-"` // index into Map.Items, or -1 for dropped weapons
	// The thrower can't re-grab it until the guard expires or they step out of
	// pickup range; everyone else can take it at once (hand-overs).
	thrownBy      int       `net:"-"`
	noPickupUntil time.Time `net:"-"`
	throwerNear   bool      `net:"-"` // the thrower has been within reach since the throw
}

const (
	pickupRadius   = 1.1
	throwGuard     = 750 * time.Millisecond
	maxItems       = 32
	mapItemRespawn = 20 * time.Second
)

// drop puts the current weapon (not melee) on the ground in front of p and
// switches to the next slot. Callers hold s.mu.
func (s *Server) drop(p *Player) {
	inv := p.inv
	s.Weapons.settle(inv, p.cmdTicks) // a finished reload goes to the ground full
	h := inv.Slots[inv.Current]
	if h == nil || inv.Current == "melee" {
		// GPU Drop: G with nothing droppable in hand puts the GPU down in front
		fwd := Vec3{-math.Sin(p.Yaw), 0, -math.Cos(p.Yaw)}
		s.dropGPUAt(p, Vec3{p.Pos[0] + fwd[0]*1.4, p.Pos[1], p.Pos[2] + fwd[2]*1.4})
		return
	}
	if inv.Current == BombSlot { // The Big Diff goes down in front of you, like a weapon
		fwd := Vec3{-math.Sin(p.Yaw), 0, -math.Cos(p.Yaw)}
		s.dropBombAt(p, Vec3{p.Pos[0] + fwd[0]*1.4, p.Pos[1], p.Pos[2] + fwd[2]*1.4})
		return
	}
	delete(inv.Slots, inv.Current)
	fwd := Vec3{-math.Sin(p.Yaw), 0, -math.Cos(p.Yaw)}
	it := s.spawnItem(*h, Vec3{p.Pos[0] + fwd[0]*1.4, p.Pos[1], p.Pos[2] + fwd[2]*1.4}, -1)
	it.thrownBy, it.noPickupUntil = p.ID, s.clock().Add(throwGuard)
	for _, slot := range s.Weapons.Slots {
		if inv.Slots[slot] != nil {
			inv.Current = slot
			break
		}
	}
	inv.drawn(p.cmdTicks)
	s.sendInv(p)
	s.tickItems(s.clock()) // standing on another weapon: pick it up right away
}

// aimedItem: the index in s.items of the floor weapon p's view points at
// within TakeRange (AimedItem, shared with the client's prompt), or -1.
func (s *Server) aimedItem(p *Player) int {
	if p.body == nil || len(s.items) == 0 {
		return -1
	}
	at := make([][3]float64, len(s.items))
	for i, it := range s.items {
		at[i] = it.Pos
	}
	return AimedItemG(p.body.Grav, [3]float64(sim.EyePos(p.body)), p.body.Yaw, p.body.Pitch, at)
}

// takeItem: E on a floor weapon. It goes into its slot and in hand; what
// was in that slot goes down where it lay (the taker can't walk-over grab it
// back for throwGuard). Map-placed guns respawn as when walked over. Callers
// hold s.mu.
func (s *Server) takeItem(p *Player, it *Item) {
	w := s.Weapons.Get(it.Weapon)
	if w == nil || p.inv == nil || w.Slot == BombSlot {
		return
	}
	inv := p.inv
	s.Weapons.settle(inv, p.cmdTicks) // a finished reload goes to the ground full
	old := inv.Slots[w.Slot]
	kept := s.items[:0]
	for _, o := range s.items {
		if o != it {
			kept = append(kept, o)
		}
	}
	clear(s.items[len(kept):])
	s.items = kept
	if it.mapSlot >= 0 {
		s.itemRespawn[it.mapSlot] = s.clock().Add(mapItemRespawn)
	}
	if old != nil {
		d := s.spawnItem(*old, it.Pos, -1)
		d.thrownBy, d.noPickupUntil = p.ID, s.clock().Add(throwGuard)
	}
	h := it.held
	inv.Slots[w.Slot] = &h
	if inv.Current != w.Slot {
		inv.prev, inv.Current = inv.Current, w.Slot
	}
	inv.useBack = ""
	inv.drawn(p.cmdTicks)
	s.sendInv(p)
}

// deathDropToss: how far along the victim's velocity the primary lands at
// most (metres), and per m/s of it.
const (
	deathDropToss  = 1.0
	deathDropPerMS = 0.12
	deathDropClear = 0.25 // the item keeps this far out of walls
)

// deathDrop puts the victim's primary weapon (whatever is in hand) on the
// floor at the body, with its ammo, like a manual drop: anyone may take it,
// and the next round start clears it. Nothing else drops; no primary,
// nothing. Warmup and deathmatch bodies vanish and drop nothing. Callers hold s.mu.
func (s *Server) deathDrop(p *Player) {
	if p.Vanish || p.inv == nil || s.Weapons == nil {
		return
	}
	inv := p.inv
	s.Weapons.settle(inv, p.cmdTicks) // a finished reload goes to the ground full
	h := inv.Slots["primary"]
	if h == nil {
		return
	}
	delete(inv.Slots, "primary")
	if inv.Current == "primary" {
		for _, slot := range s.Weapons.Slots {
			if inv.Slots[slot] != nil {
				inv.Current = slot
				break
			}
		}
	}
	var vel Vec3
	if p.body != nil {
		vel = Vec3(p.body.Vel)
	}
	s.spawnItem(*h, s.dropSpot(p.Pos, vel), -1)
	s.sendInv(p)
}

// dropSpot: where a weapon dropped at feet with velocity vel comes to rest:
// tossed a little along the horizontal velocity, stopped short of any wall,
// settled on the highest floor under it (not inside a box, not floating).
func (s *Server) dropSpot(feet, vel Vec3) Vec3 {
	dx, dz := vel[0]*deathDropPerMS, vel[2]*deathDropPerMS
	if l := math.Hypot(dx, dz); l > deathDropToss {
		dx, dz = dx*deathDropToss/l, dz*deathDropToss/l
	}
	at := Vec3{feet[0], feet[1], feet[2]}
	const steps = 10
	for i := 1; i <= steps; i++ {
		f := float64(i) / steps
		q := Vec3{feet[0] + dx*f, feet[1], feet[2] + dz*f}
		if s.solidNear(q) {
			break
		}
		at = q
	}
	at[1] = s.floorBelow(at)
	return at
}

// solidNear: a box of the map takes up the space within deathDropClear of q
// horizontally, between a step over its height and a player's height.
func (s *Server) solidNear(q Vec3) bool {
	for i := range s.Map.Boxes {
		b := &s.Map.Boxes[i]
		if q[0]+deathDropClear > b.Min[0] && q[0]-deathDropClear < b.Max[0] &&
			q[2]+deathDropClear > b.Min[2] && q[2]-deathDropClear < b.Max[2] &&
			q[1]+1.6 > b.Min[1] && q[1]+0.3 < b.Max[1] {
			return true
		}
	}
	return false
}

// floorBelow: the top of the highest box under q at most a step over its
// height (q itself when there is none).
func (s *Server) floorBelow(q Vec3) float64 {
	top := math.Inf(-1)
	for i := range s.Map.Boxes {
		b := &s.Map.Boxes[i]
		if q[0] >= b.Min[0] && q[0] <= b.Max[0] && q[2] >= b.Min[2] && q[2] <= b.Max[2] && sim.TopAt(b, q[0], q[2]) <= q[1]+0.3 && sim.TopAt(b, q[0], q[2]) > top {
			top = sim.TopAt(b, q[0], q[2])
		}
	}
	if math.IsInf(top, -1) {
		return q[1]
	}
	return top
}

func (s *Server) spawnItem(h Held, pos Vec3, mapSlot int) *Item {
	s.nextItem++
	it := &Item{ID: s.nextItem, Weapon: h.ID, Pos: pos, held: h, mapSlot: mapSlot}
	s.items = append(s.items, it)
	if len(s.items) > maxItems {
		s.items = s.items[1:]
	}
	return it
}

// clearDropped takes every dropped weapon off the floor (a new round or match,
// CS); the map's own items stay. Callers hold s.mu.
func (s *Server) clearDropped() {
	kept := s.items[:0]
	for _, it := range s.items {
		if it.mapSlot >= 0 {
			kept = append(kept, it)
		}
	}
	clear(s.items[len(kept):])
	s.items = kept
}

// tickItems hands items to players walking over them (if that slot is free)
// and respawns map-placed items. Callers hold s.mu.
func (s *Server) tickItems(now time.Time) {
	for i, mi := range s.Map.Items {
		if at, ok := s.itemRespawn[i]; ok && now.After(at) {
			delete(s.itemRespawn, i)
			if h := s.Weapons.newHeld(mi.Weapon); h != nil {
				s.spawnItem(*h, Vec3(mi.Pos), i)
			}
		}
	}
	kept := s.items[:0]
	for _, it := range s.items {
		taken := false
		w := s.Weapons.Get(it.Weapon)
		for _, p := range s.ordered() {
			if p.Bot || p.Dead || p.inv == nil || w == nil {
				continue
			}
			near := math.Hypot(p.Pos[0]-it.Pos[0], p.Pos[2]-it.Pos[2]) < pickupRadius && math.Abs(p.Pos[1]-it.Pos[1]) < 1.5
			if it.thrownBy == p.ID {
				if !now.Before(it.noPickupUntil) || (!near && it.throwerNear) {
					it.thrownBy = 0 // guard over: time's up, or they stepped away and back
				}
				it.throwerNear = it.throwerNear || near
				continue // either way, not this tick
			}
			if p.inv.Slots[w.Slot] != nil {
				continue
			}
			if near {
				h := it.held
				s.Weapons.settle(p.inv, p.cmdTicks) // finish a reload that is due before putting the gun away
				p.inv.Slots[w.Slot] = &h
				s.acquired(p, w.Slot, false) // only into an empty slot: nothing replaced
				s.sendInv(p)
				if it.mapSlot >= 0 {
					s.itemRespawn[it.mapSlot] = now.Add(mapItemRespawn)
				}
				taken = true
				break
			}
		}
		if !taken {
			kept = append(kept, it)
		}
	}
	s.items = kept
}

type invMsg struct {
	T   string     `json:"t" net:"cold"`
	Inv *Inventory `json:"inv" net:"cold,self"`
}

func (s *Server) sendInv(p *Player) {
	b, _ := json.Marshal(invMsg{T: "inv", Inv: p.inv})
	trySend(p, b)
}

// weaponState is a player's whole weapon state as of its last applied command,
// in every self-snapshot: the client compares it with its own prediction at
// that command and, if they differ, adopts it and replays the commands since
// (web/game/arsenal.js wstate/setWstate). Timers are ticks from that command.
type weaponState struct {
	Slots   map[string]*Held `json:"slots" net:"cold,self"`
	Current string           `json:"cur" net:"cold,self"`
	Next    int64            `json:"ns" net:"hot,self"`                  // ticks until the next shot may go off (0 = ready)
	Cycle   int64            `json:"ce" net:"hot,self"`                  // ticks of bolt cycling left
	Reload  int64            `json:"re" net:"hot,self"`                  // ticks until the reload completes (0 = not reloading)
	Trigger bool             `json:"trig" net:"cold,self"`               // semi/bolt/burst: this trigger press has been used
	Burst   int              `json:"bl" net:"cold,self"`                 // burst: rounds of the current burst still to fire
	Boost   int64            `json:"bo,omitempty" net:"hot,self"`        // energy drink: commands left that move faster
	Recoil  float64          `json:"ri,omitempty" net:"cold,self,exact"` // spray index as of the last shot (recoil.go)
	Since   int64            `json:"ls,omitempty" net:"hot,self"`        // ticks since the last shot (with ri)
	Carry   bool             `json:"ca,omitempty" net:"cold,self"`       // GPU Drop: carrying a GPU (moves slower)
	UseBack string           `json:"ub,omitempty" net:"cold,self"`       // E drew The Big Diff: the slot letting go of E goes back to (UseDraw)
	Pin     int              `json:"pin,omitempty" net:"cold,self"`      // grenade: pin pulled, the throw buttons held (throwTick)
}

func weaponStateOf(p *Player) *weaponState {
	inv := p.inv
	if inv == nil {
		return nil
	}
	t := p.cmdTicks
	w := &weaponState{Slots: inv.Slots, Current: inv.Current, Next: max(0, inv.nextShot-t), Cycle: max(0, inv.cycleEnd-t), Trigger: inv.trigger, Burst: inv.burstLeft, Boost: max(0, inv.boostEnd-t), Carry: inv.carry, UseBack: inv.useBack, Pin: inv.pin}
	if inv.reloadEnd != 0 {
		w.Reload = max(0, inv.reloadEnd-t)
	}
	if inv.recoil > 0 {
		w.Recoil, w.Since = inv.recoil, t-inv.lastShot
	}
	return w
}
