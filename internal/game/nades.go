package game

import (
	"strconv"

	"github.com/gigurra/counterslop/internal/content"
)

// Grenades-only play (he_merge_hell's Rules, any server's cvars):
//
//   - mp_loadout nades: a spawn gets the keyboard and one of every grenade
//     (the HE, the flash, the smoke and the team's fire), the HE in hand.
//   - mp_nade_refill N: a grenade slot gone empty (thrown) fills again N
//     seconds later, server-side like a purchase (the client adopts the
//     inventory from the next snapshot).
//   - mp_buy_guns 0: the shop sells grenades and gear, no guns (buy.go).

// nadesOnly: mp_loadout nades is in force. Callers hold s.mu.
func (s *Server) nadesOnly() bool { return s.cvar("mp_loadout") == "nades" }

// nadeRefill is mp_nade_refill in server ticks (0: never). Callers hold s.mu.
func (s *Server) nadeRefill() int64 {
	v, err := strconv.ParseFloat(s.cvar("mp_nade_refill"), 64)
	if err != nil || v <= 0 || v != v {
		return 0
	}
	return int64(v * SimHz)
}

// NadeLoadout: the keyboard and one of every grenade kind (the fire one this
// team may carry: each side has its own), the HE in hand.
func (r *Registry) NadeLoadout(team string) *Inventory {
	inv := &Inventory{Slots: map[string]*Held{}, Current: "melee"}
	if h := r.newHeld("keyboard"); h != nil {
		inv.Slots[r.Get("keyboard").Slot] = h
	}
	for i := range r.Weapons {
		w := &r.Weapons[i]
		if w.Grenade == nil || inv.Slots[w.Slot] != nil || (w.Team != nil && *w.Team != team && team != content.Spectator) {
			continue
		}
		inv.Slots[w.Slot] = &Held{ID: w.ID, Ammo: 1}
	}
	if inv.Slots["he"] != nil {
		inv.Current = "he"
	}
	return inv
}

// tickNadeRefill gives each living player back the grenades they threw,
// mp_nade_refill after each slot went empty. Callers hold s.mu.
func (s *Server) tickNadeRefill() {
	every := s.nadeRefill()
	if every == 0 || !s.nadesOnly() || s.Weapons == nil {
		return
	}
	for _, p := range s.ordered() {
		if p.Dead || p.inv == nil || p.Team == content.Spectator {
			continue
		}
		full := s.Weapons.NadeLoadout(p.Team)
		changed := false
		for slot, want := range full.Slots {
			if slot == "melee" {
				continue
			}
			if h := p.inv.Slots[slot]; h != nil && h.Ammo > 0 {
				delete(p.nadeGone, slot)
				continue
			}
			if p.nadeGone == nil {
				p.nadeGone = map[string]int64{}
			}
			since, ok := p.nadeGone[slot]
			if !ok {
				p.nadeGone[slot] = s.tick
				continue
			}
			if s.tick-since >= every {
				p.inv.Slots[slot] = &Held{ID: want.ID, Ammo: 1}
				delete(p.nadeGone, slot)
				changed = true
			}
		}
		if changed {
			s.sendInv(p)
		}
	}
}
