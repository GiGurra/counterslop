package game

import (
	"encoding/json"
	"math"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// Gear (vest, helmet, kit) comes from web/assets/economy.json "gear"; weapons
// from the weapon registry (price, team).

// canBuy says whether p may buy right now: alive, on a team, in its buy zone,
// and within mp_buytime of the round start (freeze time included). In warmup
// buying is free and has no time limit, but still only in your buy zone.
func (s *Server) canBuy(p *Player) (bool, string) {
	if p.Dead || p.inv == nil || (p.Team != content.Vibe && p.Team != content.Artisan) {
		return false, "dead"
	}
	if s.roundsOn() && s.round.Phase != PhaseFreeze && (s.round.Phase != PhaseLive || s.clock().After(s.round.buyUntil)) {
		return false, "buy time is over"
	}
	zones := s.Map.BuyZones[p.Team]
	if len(zones) == 0 {
		return true, "" // a map without buy zones: buy anywhere
	}
	for _, z := range zones {
		if z.Contains(p.Pos) {
			return true, ""
		}
	}
	return false, "not in your buy zone"
}

// buy handles {t:"buy", item}. The result goes back as {t:"bought", item, ok,
// why}; the money and the inventory are in the next snapshot regardless.
// Callers hold s.mu.
func (s *Server) buy(p *Player, item string) {
	ok, why := s.canBuy(p)
	if ok {
		why = s.purchase(p, item)
		ok = why == ""
	}
	b, _ := json.Marshal(map[string]any{"t": "bought", "item": item, "ok": ok, "why": why})
	trySend(p, b)
}

// price: the real price, warmup included (CS2: warmup money is full and
// refilled on every spawn, so the menu shows what things will cost).
func (s *Server) price(p *Player, n int) int { return n }

func (s *Server) purchase(p *Player, item string) string {
	if gp, upgrade, team, ok := s.econ().gear(item); ok {
		if team != "" && team != p.Team {
			return "Artisans only"
		}
		switch item {
		case "vest":
			if p.Armor >= 100 {
				return "already wearing it"
			}
		case "helmet":
			if p.Helmet && p.Armor >= 100 {
				return "already wearing it"
			}
			if p.Armor >= 100 && upgrade > 0 {
				gp = upgrade
			} else if vp, _, _, ok := s.econ().gear("vest"); ok && p.Helmet {
				gp = vp // the helmet is on, only the vest is worn: CS charges the vest
			}
		case "kit":
			if p.Kit {
				return "already have one"
			}
		case "shades":
			if p.Shades {
				return "already wearing them"
			}
		case "boots":
			if s.cvar("mp_gravityboots") != "1" {
				return "not on this server (mp_gravityboots)"
			}
			if p.Boots {
				return "already wearing them"
			}
		case "drink":
			if p.inv == nil {
				return "no hands"
			}
			if p.inv.boostEnd >= p.cmdTicks+1 {
				return "still buzzing"
			}
		}
		cost := s.price(p, gp)
		if p.Money < cost {
			return "can't afford it"
		}
		p.Money -= cost
		switch item {
		case "vest":
			p.Armor = 100
		case "helmet":
			p.Armor, p.Helmet = 100, true
		case "kit":
			p.Kit = true
		case "shades":
			p.Shades = true
		case "boots":
			s.setBoots(p, true)
		case "drink":
			// Commands after the current one move faster for secs (in command
			// ticks, like the weapon timers; in the self-snapshot as weaponState "bo").
			_, _, secs := s.econ().gearSpec("drink")
			p.inv.boostEnd = p.cmdTicks + Ticks(secs)
		}
		return ""
	}
	if slot, ok := ammoSlot[item]; ok {
		return s.buyAmmo(p, slot)
	}
	w := s.Weapons.Get(item)
	if w == nil || w.Slot == "melee" || w.Slot == BombSlot { // no melee, no Big Diff (the round hands it out)
		return "no such item"
	}
	if w.Team != nil && *w.Team != p.Team {
		return "the other team's weapon"
	}
	if w.Grenade != nil {
		return s.buyGrenade(p, w)
	}
	if h := p.inv.Slots[w.Slot]; h != nil && h.ID == w.ID {
		return "already have one"
	}
	cost := s.price(p, w.Price)
	if p.Money < cost {
		return "can't afford it"
	}
	p.Money -= cost
	s.Weapons.settle(p.inv, p.cmdTicks)
	replaced := p.inv.Slots[w.Slot] != nil
	if old := p.inv.Slots[w.Slot]; old != nil {
		// CS: buying into an occupied slot drops what was there.
		fwd := Vec3{-math.Sin(p.Yaw), 0, -math.Cos(p.Yaw)}
		it := s.spawnItem(*old, Vec3{p.Pos[0] + fwd[0]*1.0, p.Pos[1], p.Pos[2] + fwd[2]*1.0}, -1)
		it.thrownBy, it.noPickupUntil = p.ID, s.clock().Add(throwGuard)
	}
	p.inv.Slots[w.Slot] = s.Weapons.newHeld(w.ID)
	s.acquired(p, w.Slot, replaced)
	s.sendInv(p)
	return ""
}

// acquired: a gun just went into slot (bought or picked up); replaced: the
// slot held one before. The operator's rule:
//   - any primary goes to hand (a first one or a replacement);
//   - a pistol that replaces your pistol goes to hand, unless your primary is
//     in hand;
//   - nothing else moves the hand (a first pistol, and armor, grenades, kits,
//     ammo, The Big Diff, which never come here).
//
// A new gun in the slot you hold is drawn there (deploy delay, nothing
// carries over).
func (s *Server) acquired(p *Player, slot string, replaced bool) {
	switch {
	case slot == "primary",
		slot == "sidearm" && replaced && p.inv.Current != "primary":
		p.inv.Current = slot
		p.inv.drawn(p.cmdTicks)
	case slot == p.inv.Current:
		p.inv.drawn(p.cmdTicks)
	}
}

// armorRatio is the fraction of damage that gets through armor (CS armor
// ratio); weapons may set "armorPen", otherwise 0.5.
func armorRatio(w *Weapon) float64 {
	if w != nil && w.ArmorPen > 0 {
		return w.ArmorPen
	}
	return 0.5
}

// absorb applies p's armor to a hit of dmg on part: the vest covers the body
// and arms, the helmet the head, nothing covers the legs. The armor loses
// half of what it stopped. Returns the damage that gets through.
func absorb(p *Player, w *Weapon, part string, dmg float64) float64 {
	covered := p.Armor > 0 && (part == "body" || part == "head" && p.Helmet)
	if !covered {
		return dmg
	}
	through := dmg * armorRatio(w)
	stopped := dmg - through
	if spend := stopped / 2; spend > float64(p.Armor) {
		through = dmg - float64(p.Armor)*2 // ran out: the rest goes through
		p.Armor = 0
	} else {
		p.Armor -= int(math.Round(spend))
	}
	if p.Armor <= 0 {
		p.Armor, p.Helmet = 0, false
	}
	return through
}

// keepForNextRound: CS rules for what a survivor takes into the next round
// (weapons, armor, kit); the dead start over with the pistol loadout.
type carried struct {
	inv    *Inventory
	armor  int
	helmet bool
	kit    bool
	shades bool
	boots  bool
}

func (s *Server) carryOver(p *Player) *carried {
	if p.Dead || p.inv == nil || !s.roundsOn() || s.round.Number <= 1 || s.round.pistol { // round 1 and after halftime: everyone starts fresh
		return nil
	}
	// A reload still in progress when the round ends finishes now (the held gun
	// tops up from its reserve, as settle does), so restoreCarried's reset can't drop it.
	s.Weapons.settle(p.inv, max(p.cmdTicks, p.inv.reloadEnd))
	return &carried{inv: p.inv, armor: p.Armor, helmet: p.Helmet, kit: p.Kit, shades: p.Shades, boots: p.Boots}
}

func (s *Server) restoreCarried(p *Player, c *carried) {
	if c == nil {
		p.Armor, p.Helmet, p.Kit, p.Shades = 0, false, false, false
		s.setBoots(p, false)
		return
	}
	inv := c.inv
	inv.nextShot, inv.cycleEnd, inv.reloadEnd, inv.trigger, inv.boostEnd = 0, 0, 0, false, 0 // the drink wears off between rounds
	p.inv = inv
	p.inv.drawn(p.cmdTicks)
	p.Armor, p.Helmet, p.Kit, p.Shades = c.armor, c.helmet, c.kit, c.shades
	s.setBoots(p, c.boots)
	s.sendInv(p)
}

// setBoots puts the Gravity Override on p (or takes it off): the player keeps
// it for every body it gets (fitBoots). Callers hold s.mu.
func (s *Server) setBoots(p *Player, on bool) {
	p.Boots = on
	s.fitBoots(p)
}

// fitBoots dresses p's body as p.Boots says, the meter full (a new body, a
// purchase, a new round); off: no hook, the world's gravity. Callers hold s.mu.
func (s *Server) fitBoots(p *Player) {
	b := p.body
	if b == nil {
		return
	}
	b.Boots, b.Energy, b.Hook = p.Boots, 0, sim.Hook{}
	if p.Boots {
		b.Energy = s.moveRules().Hook.Energy
	}
}

// canBuyNow is canBuy for the snapshot flag (the HUD's buy-zone icon).
func canBuyNow(s *Server, p *Player) bool { ok, _ := s.canBuy(p); return ok }

// ammoSlot: the ammo items (buyammo1 on ",", buyammo2 on ".") and the slot each refills.
var ammoSlot = map[string]string{"ammo1": "primary", "ammo2": "sidearm"}

// buyAmmo fills the reserve of the gun in slot back to what a new one comes
// with, for the economy's ammo price. Callers hold s.mu.
func (s *Server) buyAmmo(p *Player, slot string) string {
	s.Weapons.settle(p.inv, p.cmdTicks)
	h := p.inv.Slots[slot]
	if h == nil {
		return "no " + map[string]string{"primary": "primary", "sidearm": "pistol"}[slot] + " to load"
	}
	w := s.Weapons.Get(h.ID)
	if w == nil || w.Mag == 0 || h.Reserve >= w.Reserve {
		return "ammo is full"
	}
	cost := s.price(p, s.econ().Ammo[slot])
	if p.Money < cost {
		return "can't afford it"
	}
	p.Money -= cost
	h.Reserve = w.Reserve
	s.sendInv(p)
	return ""
}
