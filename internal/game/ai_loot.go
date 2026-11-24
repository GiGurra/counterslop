package game

import (
	"fmt"
	"math"
)

// Bots pick up dropped primaries (a dead player's, or one thrown down): out
// of combat, a bot without a primary, or with a cheaper one, walks over to
// one lying within aiLootRange and takes it (swapping its own down).

const aiLootRange = 6.0 // metres from the bot, about the floor it stands on

// aiLoot: the dropped primary worth a detour for p (nil: none). Callers hold s.mu.
func (s *Server) aiLoot(p *Player) *Item {
	if p.inv == nil || s.Weapons == nil || !s.roundsOn() || s.frozen() {
		return nil
	}
	price := -1 // no primary: any will do
	if h := p.inv.Slots["primary"]; h != nil {
		if w := s.Weapons.Get(h.ID); w != nil {
			price = w.Price
		}
	}
	var best *Item
	bestD := aiLootRange
	for _, it := range s.items {
		w := s.Weapons.Get(it.Weapon)
		if w == nil || w.Slot != "primary" || w.Price <= price || it.thrownBy == p.ID || it.held.Ammo+it.held.Reserve == 0 {
			continue
		}
		if math.Abs(it.Pos[1]-p.Pos[1]) > 1.5 {
			continue
		}
		if d := math.Hypot(it.Pos[0]-p.Pos[0], it.Pos[2]-p.Pos[2]); d < bestD {
			best, bestD = it, d
		}
	}
	return best
}

// aiLootGoal: where p walks for loot, and its path key (ok false: nothing to
// fetch). Standing on it with a cheaper primary, p swaps it (tickItems hands
// over only into an empty slot). Callers hold s.mu.
func (s *Server) aiLootGoal(p *Player) (Vec3, string, bool) {
	it := s.aiLoot(p)
	if it == nil {
		return Vec3{}, "", false
	}
	if p.inv.Slots["primary"] != nil &&
		math.Hypot(p.Pos[0]-it.Pos[0], p.Pos[2]-it.Pos[2]) < pickupRadius && math.Abs(p.Pos[1]-it.Pos[1]) < 1.5 {
		s.aiSwap(p, it)
		return Vec3{}, "", false
	}
	return it.Pos, fmt.Sprintf("loot:%d", it.ID), true
}

// aiSwap: p's primary goes down where it stands, it (a better one) goes into
// the slot. Callers hold s.mu.
func (s *Server) aiSwap(p *Player, it *Item) {
	inv := p.inv
	s.Weapons.settle(inv, p.cmdTicks)
	old := *inv.Slots["primary"]
	h := it.held
	inv.Slots["primary"] = &h
	kept := s.items[:0]
	for _, o := range s.items {
		if o != it {
			kept = append(kept, o)
		}
	}
	clear(s.items[len(kept):])
	s.items = kept
	d := s.spawnItem(old, it.Pos, -1)
	d.thrownBy, d.noPickupUntil = p.ID, s.clock().Add(throwGuard)
	s.acquired(p, "primary", true)
	s.sendInv(p)
}

// lootPath: straight to the gun when nothing stands in the way (it lies a
// few metres off), else over the nav graph.
func (s *Server) lootPath(from, to Vec3) []Vec3 {
	d := math.Hypot(to[0]-from[0], to[2]-from[2])
	n := int(d/0.25) + 1
	for i := 1; i <= n; i++ {
		f := float64(i) / float64(n)
		if s.solidNear(Vec3{from[0] + (to[0]-from[0])*f, from[1], from[2] + (to[2]-from[2])*f}) {
			return s.navPath(from, to)
		}
	}
	return []Vec3{to}
}
