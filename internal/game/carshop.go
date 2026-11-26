package game

import (
	"strings"

	"github.com/gigurra/counterslop/internal/sim"
)

// The race shop (docs/racing.md, race phase 3): a racer's garage, kept for
// the match: the car type (a tier), the front and rear items swapped in, and
// upgrades by tier (cars.json "shop"). Bought like guns ({t:"buy", item}),
// on the grid (the freeze time) or in warmup, anywhere: items
//
//   - "car:<type>": the car (its price; the buggy is free);
//   - "front:<item>", "rear:<item>": swap a built-in for that item (the type's
//     own built-in back: free);
//   - "up:<charges|tank|plating|engine|tyres>": the next tier (at most 3;
//     no tyres for the hovercraft).
//
// On the grid the car is refitted at once (fitCar); every respawn and race
// fits it again. In the self-snapshot as me.gar.

// garage is a racer's car and upgrades.
type garage struct {
	Type    string `json:"car,omitempty" net:"hot,self"`
	Front   string `json:"f,omitempty" net:"hot,self"`
	Rear    string `json:"r,omitempty" net:"hot,self"`
	Charges int    `json:"ch,omitempty" net:"hot,self"`
	Tank    int    `json:"tk,omitempty" net:"hot,self"`
	Plating int    `json:"pl,omitempty" net:"hot,self"`
	Engine  int    `json:"en,omitempty" net:"hot,self"`
	Tyres   int    `json:"ty,omitempty" net:"hot,self"`
}

// garageOf is p's garage (the zero one: the map's car, stock).
func (s *Server) garageOf(p *Player) garage {
	if p == nil || p.garage == nil {
		return garage{}
	}
	return *p.garage
}

// garageMsg is p's garage for the self-snapshot (nil when not racing).
func (s *Server) garageMsg(p *Player) *garage {
	if !s.raceMode() || p.garage == nil {
		return nil
	}
	g := *p.garage
	return &g
}

// carShopOpen: p can buy car things now (a race map, on the grid or in
// warmup). "" or why not.
func (s *Server) carShopOpen(p *Player) string {
	if !s.raceMode() {
		return "no race on this map"
	}
	if p.Dead || (p.Team != "vibe" && p.Team != "artisan") {
		return "dead"
	}
	if ph := s.round.Phase; ph != PhaseFreeze && ph != PhaseWarmup {
		return "the shop is on the grid (before GO)"
	}
	return ""
}

// carPurchase buys car shop item for p; "" or why not.
func (s *Server) carPurchase(p *Player, item string) string {
	if why := s.carShopOpen(p); why != "" {
		return why
	}
	if p.garage == nil {
		p.garage = &garage{}
	}
	g := p.garage
	kind, id, _ := strings.Cut(item, ":")
	cur := s.garageCarSpec(p)
	cost := 0
	apply := func() {}
	switch kind {
	case "car":
		sp := CarSpecOf(id)
		if sp == nil {
			return "no such car"
		}
		if sp.ID == cur.ID {
			return "already yours"
		}
		cost = sp.Price
		apply = func() { g.Type, g.Front, g.Rear = sp.ID, "", "" } // the new car's own built-ins
	case "front", "rear":
		it := CarItemOf(id)
		if it == nil || it.Slot != kind {
			return "no such " + kind + " item"
		}
		have, builtin := g.Front, cur.Front
		if kind == "rear" {
			have, builtin = g.Rear, cur.Rear
		}
		if have == id || (have == "" && builtin == id) {
			return "already fitted"
		}
		if id != builtin {
			cost = CarShop.Swap
		}
		apply = func() {
			v := id
			if id == builtin {
				v = ""
			}
			if kind == "front" {
				g.Front = v
			} else {
				g.Rear = v
			}
		}
	case "up":
		prices, ok := CarShop.Upgrades[id]
		tier := map[string]*int{"charges": &g.Charges, "tank": &g.Tank, "plating": &g.Plating, "engine": &g.Engine, "tyres": &g.Tyres}[id]
		if !ok || tier == nil {
			return "no such upgrade"
		}
		if id == "tyres" && cur.Drive == sim.DriveHover {
			return "the hovercraft has no tyres"
		}
		if *tier >= len(prices) {
			return "maxed out"
		}
		cost = prices[*tier]
		apply = func() { *tier++ }
	default:
		return "no such item"
	}
	if p.Money < cost {
		return "can't afford it"
	}
	p.Money -= cost
	apply()
	if s.garageCarSpec(p).Drive == sim.DriveHover { // no tyres on a hovercraft (a tier bought for another car stays off it)
		g.Tyres = 0
	}
	s.refitOnGrid(p)
	return ""
}

// garageCarSpec is the car type p races in: their garage's, else the map's
// car they sit in, else the first type.
func (s *Server) garageCarSpec(p *Player) *sim.CarSpec {
	if p.garage != nil && p.garage.Type != "" {
		if sp := CarSpecOf(p.garage.Type); sp != nil {
			return sp
		}
	}
	if c := s.carByID(p.car); c != nil {
		return c.spec
	}
	if rs := s.racer(p); rs != nil {
		if c := s.carByID(rs.Car); c != nil {
			return c.spec
		}
	}
	if len(CarTypes) > 0 {
		return CarTypes[0]
	}
	return &sim.CarSpec{}
}

// refitOnGrid: p bought something on the grid: their car is refitted now.
func (s *Server) refitOnGrid(p *Player) {
	if rs := s.racer(p); rs != nil {
		if c := s.carByID(rs.Car); c != nil {
			s.fitCar(c, p)
		}
	}
}
