package game

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
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
	ID      int    `json:"id"`
	Weapon  string `json:"w"`
	Pos     Vec3   `json:"p"`
	held    Held
	mapSlot int // index into Map.Items, or -1 for dropped weapons
	// The thrower can't re-grab it until the guard expires or they step out of
	// pickup range; everyone else can take it at once (hand-overs).
	thrownBy      int
	noPickupUntil time.Time
	throwerNear   bool // the thrower has been within reach since the throw
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

func (s *Server) spawnItem(h Held, pos Vec3, mapSlot int) *Item {
	s.nextItem++
	it := &Item{ID: s.nextItem, Weapon: h.ID, Pos: pos, held: h, mapSlot: mapSlot}
	s.items = append(s.items, it)
	if len(s.items) > maxItems {
		s.items = s.items[1:]
	}
	return it
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
	T   string     `json:"t"`
	Inv *Inventory `json:"inv"`
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
	Slots   map[string]*Held `json:"slots"`
	Current string           `json:"cur"`
	Next    int64            `json:"ns"`           // ticks until the next shot may go off (0 = ready)
	Cycle   int64            `json:"ce"`           // ticks of bolt cycling left
	Reload  int64            `json:"re"`           // ticks until the reload completes (0 = not reloading)
	Trigger bool             `json:"trig"`         // semi/bolt/burst: this trigger press has been used
	Burst   int              `json:"bl"`           // burst: rounds of the current burst still to fire
	Boost   int64            `json:"bo,omitempty"` // energy drink: commands left that move faster
	Recoil  float64          `json:"ri,omitempty"` // spray index as of the last shot (recoil.go)
	Since   int64            `json:"ls,omitempty"` // ticks since the last shot (with ri)
	Carry   bool             `json:"ca,omitempty"` // GPU Drop: carrying a GPU (moves slower)
}

func weaponStateOf(p *Player) *weaponState {
	inv := p.inv
	if inv == nil {
		return nil
	}
	t := p.cmdTicks
	w := &weaponState{Slots: inv.Slots, Current: inv.Current, Next: max(0, inv.nextShot-t), Cycle: max(0, inv.cycleEnd-t), Trigger: inv.trigger, Burst: inv.burstLeft, Boost: max(0, inv.boostEnd-t), Carry: inv.carry}
	if inv.reloadEnd != 0 {
		w.Reload = max(0, inv.reloadEnd-t)
	}
	if inv.recoil > 0 {
		w.Recoil, w.Since = inv.recoil, t-inv.lastShot
	}
	return w
}
