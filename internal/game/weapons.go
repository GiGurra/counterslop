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
	Action   string  `json:"action"` // auto | semi | bolt
	Cycle    float64 `json:"cycle"`  // bolt-action: seconds working the bolt after a shot
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
	Slots   map[string]*Held `json:"slots"`
	Current string           `json:"cur"`
	// Timers in command ticks (Player.cmdTicks), mirrored exactly by
	// web/game/arsenal.js so predicted and authoritative shots line up.
	nextShot  int64 // first tick the next shot may go off
	cycleEnd  int64 // bolt: tick the bolt is worked (no shot, trigger not taken, before)
	reloadEnd int64 // tick the reload completes (0 = not reloading)
	trigger   bool  // semi/bolt: this trigger press has been used
}

// DeployTicks is how long a freshly drawn weapon can't fire (arsenal.js:
// RAISE_TIME * 0.6).
const DeployTicks = 21

// Ticks converts seconds to whole command ticks, rounding up (as arsenal.js ticks()).
func Ticks(sec float64) int64 { return int64(math.Ceil(sec*SimHz - 1e-6)) }

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
func (r *Registry) settle(inv *Inventory, tick int64) {
	if inv.reloadEnd == 0 || tick < inv.reloadEnd {
		return
	}
	inv.reloadEnd = 0
	h := inv.Slots[inv.Current]
	if h == nil {
		return
	}
	w := r.Get(h.ID)
	take := min(w.Mag-h.Ammo, h.Reserve)
	h.Ammo += take
	h.Reserve -= take
}

// fire runs one tick of a held trigger at command tick `tick`: it fires the
// held weapon, or says why not ("no weapon", "reloading", "cycling", "semi",
// "empty", "cooldown"). The rules and their order are arsenal.js a.tick's.
func (r *Registry) fire(inv *Inventory, tick int64) (*Weapon, string) {
	r.settle(inv, tick)
	h := inv.Slots[inv.Current]
	if h == nil {
		return nil, "no weapon"
	}
	if inv.reloadEnd != 0 {
		return nil, "reloading"
	}
	if tick < inv.cycleEnd {
		return nil, "cycling"
	}
	w := r.Get(h.ID)
	if w.Action != "auto" && inv.trigger {
		return nil, "semi"
	}
	inv.trigger = true
	if w.Mag > 0 && h.Ammo <= 0 {
		return nil, "empty"
	}
	if tick < inv.nextShot {
		return nil, "cooldown"
	}
	inv.nextShot = tick + Ticks(w.Interval)
	if w.Action == "bolt" {
		cycle := w.Cycle
		if cycle == 0 {
			cycle = w.Interval
		}
		inv.cycleEnd = tick + Ticks(cycle)
	}
	if w.Mag > 0 {
		h.Ammo--
	}
	return w, ""
}

// release is a tick without the trigger held.
func (inv *Inventory) release() { inv.trigger = false }

func (r *Registry) reload(inv *Inventory, tick int64) {
	r.settle(inv, tick)
	h := inv.Slots[inv.Current]
	if h == nil || inv.reloadEnd != 0 {
		return
	}
	w := r.Get(h.ID)
	if w.Mag == 0 || h.Ammo >= w.Mag || h.Reserve == 0 {
		return
	}
	inv.reloadEnd = tick + Ticks(w.Reload)
}

func (r *Registry) switchTo(inv *Inventory, slot string, tick int64) bool {
	if inv.Slots[slot] == nil || inv.Current == slot {
		return false
	}
	r.settle(inv, tick) // a reload that finished stays finished (the client settles every tick)
	inv.Current = slot
	inv.drawn(tick)
	return true
}

// drawn: a weapon was just taken in hand: no reload, no bolt, a short deploy.
func (inv *Inventory) drawn(tick int64) {
	inv.reloadEnd, inv.cycleEnd = 0, 0
	inv.nextShot = max(inv.nextShot, tick+DeployTicks)
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
		for _, p := range s.players {
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
				p.inv.Current = w.Slot // pick up = hold it, like CS
				p.inv.drawn(p.cmdTicks)
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
	Next    int64            `json:"ns"`   // ticks until the next shot may go off (0 = ready)
	Cycle   int64            `json:"ce"`   // ticks of bolt cycling left
	Reload  int64            `json:"re"`   // ticks until the reload completes (0 = not reloading)
	Trigger bool             `json:"trig"` // semi/bolt: this trigger press has been used
}

func weaponStateOf(p *Player) *weaponState {
	inv := p.inv
	if inv == nil {
		return nil
	}
	t := p.cmdTicks
	w := &weaponState{Slots: inv.Slots, Current: inv.Current, Next: max(0, inv.nextShot-t), Cycle: max(0, inv.cycleEnd-t), Trigger: inv.trigger}
	if inv.reloadEnd != 0 {
		w.Reload = max(0, inv.reloadEnd-t)
	}
	return w
}
