package game

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// Weapon is one entry of web/assets/weapons/weapons.json, the registry shared
// with the client. The server uses it to validate fire and compute damage.
type Weapon struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Slot     string  `json:"slot"` // primary | sidearm | melee
	Team     *string `json:"team"`
	Damage   float64 `json:"damage"`
	Interval float64 `json:"interval"`
	Mag      int     `json:"mag"`
	Reserve  int     `json:"reserve"`
	Reload   float64 `json:"reload"`
	Range    float64 `json:"range"`
	Spread   Spread  `json:"spread"`
}

// Spread in radians: base + per m/s moving + in the air (+ unscoped), times crouch.
type Spread struct {
	Base     float64 `json:"base"`
	Move     float64 `json:"move"`
	Air      float64 `json:"air"`
	Crouch   float64 `json:"crouch"`
	Unscoped float64 `json:"unscoped"`
}

type Registry struct {
	Weapons []Weapon `json:"weapons"`
	Slots   []string `json:"slots"`
	byID    map[string]*Weapon
}

func LoadWeapons(b []byte) (*Registry, error) {
	var r Registry
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("weapons.json: %w", err)
	}
	r.byID = map[string]*Weapon{}
	for i := range r.Weapons {
		r.byID[r.Weapons[i].ID] = &r.Weapons[i]
	}
	return &r, nil
}

func (r *Registry) Get(id string) *Weapon { return r.byID[id] }

// Held is one weapon a player carries, with its ammo.
type Held struct {
	ID      string `json:"id"`
	Ammo    int    `json:"ammo"`
	Reserve int    `json:"reserve"`
}

// Inventory is a player's slots, current slot and fire/reload timing.
type Inventory struct {
	Slots       map[string]*Held `json:"slots"`
	Current     string           `json:"cur"`
	lastShot    time.Time
	reloadUntil time.Time
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

// settle finishes a reload whose time has come.
func (r *Registry) settle(inv *Inventory, now time.Time) {
	if inv.reloadUntil.IsZero() || now.Before(inv.reloadUntil) {
		return
	}
	inv.reloadUntil = time.Time{}
	h := inv.Slots[inv.Current]
	if h == nil {
		return
	}
	w := r.Get(h.ID)
	take := min(w.Mag-h.Ammo, h.Reserve)
	h.Ammo += take
	h.Reserve -= take
}

// fire validates a shot with the current weapon (fire rate, ammo, reload) and
// consumes ammo. It returns the weapon, or nil if the shot isn't allowed.
func (r *Registry) fire(inv *Inventory, now time.Time) *Weapon {
	r.settle(inv, now)
	h := inv.Slots[inv.Current]
	if h == nil || !inv.reloadUntil.IsZero() {
		return nil
	}
	w := r.Get(h.ID)
	// 20% slack for network jitter between the client's shot packets.
	if !inv.lastShot.IsZero() && now.Sub(inv.lastShot).Seconds() < w.Interval*0.8 {
		return nil
	}
	if w.Mag > 0 {
		if h.Ammo <= 0 {
			return nil
		}
		h.Ammo--
	}
	inv.lastShot = now
	return w
}

func (r *Registry) reload(inv *Inventory, now time.Time) {
	r.settle(inv, now)
	h := inv.Slots[inv.Current]
	if h == nil || !inv.reloadUntil.IsZero() {
		return
	}
	w := r.Get(h.ID)
	if w.Mag == 0 || h.Ammo >= w.Mag || h.Reserve == 0 {
		return
	}
	inv.reloadUntil = now.Add(time.Duration(w.Reload * float64(time.Second)))
}

func (r *Registry) switchTo(inv *Inventory, slot string) bool {
	if inv.Slots[slot] == nil || inv.Current == slot {
		return false
	}
	inv.Current = slot
	inv.reloadUntil = time.Time{} // switching cancels a reload, like CS
	return true
}

// Item is a weapon lying on the ground (dropped, or placed by the map).
type Item struct {
	ID      int    `json:"id"`
	Weapon  string `json:"w"`
	Pos     Vec3   `json:"p"`
	held    Held
	mapSlot int // index into Map.Items, or -1 for dropped weapons
}

const (
	pickupRadius   = 1.1
	maxItems       = 32
	mapItemRespawn = 20 * time.Second
)

// drop puts the current weapon (not melee) on the ground in front of p and
// switches to the next slot. Callers hold s.mu.
func (s *Server) drop(p *Player) {
	inv := p.inv
	h := inv.Slots[inv.Current]
	if h == nil || inv.Current == "melee" {
		return
	}
	delete(inv.Slots, inv.Current)
	fwd := Vec3{-math.Sin(p.Yaw), 0, -math.Cos(p.Yaw)}
	s.spawnItem(*h, Vec3{p.Pos[0] + fwd[0]*1.4, p.Pos[1], p.Pos[2] + fwd[2]*1.4}, -1)
	for _, slot := range s.Weapons.Slots {
		if inv.Slots[slot] != nil {
			inv.Current = slot
			break
		}
	}
	inv.reloadUntil = time.Time{}
	p.noPickupUntil = time.Now().Add(time.Second) // don't instantly re-grab what you threw
	s.sendInv(p)
}

func (s *Server) spawnItem(h Held, pos Vec3, mapSlot int) {
	s.nextItem++
	s.items = append(s.items, &Item{ID: s.nextItem, Weapon: h.ID, Pos: pos, held: h, mapSlot: mapSlot})
	if len(s.items) > maxItems {
		s.items = s.items[1:]
	}
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
		for _, p := range s.players {
			if p.Bot || p.inv == nil || w == nil || now.Before(p.noPickupUntil) || p.inv.Slots[w.Slot] != nil {
				continue
			}
			if math.Hypot(p.Pos[0]-it.Pos[0], p.Pos[2]-it.Pos[2]) < pickupRadius && math.Abs(p.Pos[1]-it.Pos[1]) < 1.5 {
				h := it.held
				p.inv.Slots[w.Slot] = &h
				p.inv.Current = w.Slot // pick up = hold it, like CS
				p.inv.reloadUntil = time.Time{}
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
