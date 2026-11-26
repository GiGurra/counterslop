package game

import (
	"encoding/json"
	"fmt"

	"github.com/gigurra/counterslop/internal/content"
)

// Economy is web/assets/economy.json: CS2 competitive money rules, shared with
// the client (buy menu, HUD). The server owns every dollar; money rides in
// every self-snapshot and each change is announced as {t:"money"} with a reason.
type Economy struct {
	StartMoney      int            `json:"startMoney"`
	MaxMoney        int            `json:"maxMoney"`
	KillReward      map[string]int `json:"killReward"` // by weapon class
	TeamKillPenalty int            `json:"teamKillPenalty"`
	RoundWin        map[string]int `json:"roundWin"` // by end reason
	LossBonus       struct {
		Base      int `json:"base"`
		Step      int `json:"step"`
		Max       int `json:"max"`
		MaxStreak int `json:"maxStreak"`
		// Start: each team's loss counter at the start of a match or half
		// (CS2: 1, so the pistol-round loser gets base+step).
		Start int `json:"start"`
	} `json:"lossBonus"`
	PlantReward      int `json:"plantReward"`
	PlantedLossBonus int `json:"plantedLossBonus"`
	DefuseReward     int `json:"defuseReward"`
	Gear             []struct {
		ID      string  `json:"id"`
		Price   int     `json:"price"`
		Upgrade int     `json:"upgrade"` // helmet: price when the vest is already on
		Team    string  `json:"team"`
		Flash   float64 `json:"flash"` // shades: flash blindness scale
		Speed   float64 `json:"speed"` // drink: move speed scale while it lasts
		Secs    float64 `json:"secs"`  // drink: how long
		Rule    string  `json:"rule"`  // a server cvar that must be "1" to sell it (boots: mp_gravityboots)
	} `json:"gear"`
	// GPUDrop: GPU Drop's objective money and the carrier's speed scale.
	GPUDrop struct {
		Grab       int     `json:"grab"`
		Secure     int     `json:"secure"`     // the carrier
		SecureTeam int     `json:"secureTeam"` // every other Vibe Coder
		Restock    int     `json:"restock"`
		CarrySpeed float64 `json:"carrySpeed"`
	} `json:"gpudrop"`
	// Tickets: the tickets mode's kits (tickets.go).
	Tickets struct {
		Kits []Kit `json:"kits"`
	} `json:"tickets"`
	// Ammo: what refilling a gun's reserve costs, by slot (CS 1.6's buyammo1 /
	// buyammo2 on , and .).
	Ammo map[string]int `json:"ammo"`
	// Race: the race mode's money (race.go): a kill with a car item, a lap
	// finished in the lead, and the finish by place (then Finish for the rest).
	Race struct {
		Kill   int   `json:"kill"`
		LapLed int   `json:"lapLed"`
		Place  []int `json:"place"`
		Finish int   `json:"finish"`
	} `json:"race"`
}

// gear: price, upgrade price and team of a gear item, ok=false if unknown.
func (e *Economy) gear(id string) (price, upgrade int, team string, ok bool) {
	for _, g := range e.Gear {
		if g.ID == id {
			return g.Price, g.Upgrade, g.Team, true
		}
	}
	return 0, 0, "", false
}

// gearSpec is a gear item's effect numbers (zero values if unknown).
func (e *Economy) gearSpec(id string) (flash, speed, secs float64) {
	for _, g := range e.Gear {
		if g.ID == id {
			return g.Flash, g.Speed, g.Secs
		}
	}
	return 0, 0, 0
}

// DefaultEconomy is used when no economy.json is loaded (tests, tools).
var DefaultEconomy = func() *Economy {
	e, _ := LoadEconomy([]byte(`{"startMoney":800,"maxMoney":16000,
		"killReward":{"pistol":300,"rifle":300,"smg":600,"shotgun":900,"sniper":100,"melee":1500,"grenade":300},
		"teamKillPenalty":-300,"roundWin":{"elimination":3250,"time":3250,"exploded":3500,"defused":3500,"secured":3500},
		"lossBonus":{"base":1400,"step":500,"max":3400,"maxStreak":5,"start":1},
		"plantReward":300,"plantedLossBonus":800,"defuseReward":300,
		"gear":[{"id":"vest","price":650},{"id":"helmet","price":1000,"upgrade":350},{"id":"kit","price":400,"team":"artisan"},
			{"id":"shades","price":200,"flash":0.4},{"id":"drink","price":300,"speed":1.1,"secs":10},
			{"id":"boots","price":1000,"rule":"mp_gravityboots"}],
		"ammo":{"primary":100,"sidearm":50},
		"gpudrop":{"grab":200,"secure":1000,"secureTeam":500,"restock":300,"carrySpeed":0.8},
		"tickets":{"kits":[{"id":"intern","name":"Intern Rifleman","weapons":["rifle","stub","keyboard","smoketest"]},
			{"id":"senior","name":"Senior Sniper","weapons":["codereview","hotfix","keyboard"]},
			{"id":"staff","name":"Staff Engineer","weapons":["tokenburner","stub","keyboard","mergeconflict","stacktrace"]},
			{"id":"pm","name":"Product Manager","weapons":["autocomplete","rubberduck","keyboard","smoketest"]}]}}`))
	return e
}()

func LoadEconomy(b []byte) (*Economy, error) {
	var e Economy
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("economy.json: %w", err)
	}
	return &e, nil
}

// econ is the loaded economy (or the defaults).
func (s *Server) econ() *Economy {
	if s.Economy != nil {
		return s.Economy
	}
	return DefaultEconomy
}

// economyOn: money only moves in real rounds, not in warmup.
func (s *Server) economyOn() bool { return s.roundsOn() }

// pay changes p's money by d (clamped to 0..maxMoney) and tells them why.
// Callers hold s.mu.
func (s *Server) pay(p *Player, d int, why string) {
	if p == nil || p.Bot || d == 0 {
		return
	}
	before := p.Money
	p.Money = max(0, min(s.econ().MaxMoney, p.Money+d))
	if p.Money == before {
		return
	}
	b, _ := json.Marshal(map[string]any{"t": "money", "d": p.Money - before, "why": why, "total": p.Money})
	trySend(p, b)
}

// resetMoney gives everyone the start money (match start, halftime).
func (s *Server) resetMoney() {
	for _, p := range s.ordered() {
		p.Money = s.econ().StartMoney
	}
	s.lossStreak = map[string]int{content.Vibe: s.econ().LossBonus.Start, content.Artisan: s.econ().LossBonus.Start}
}

// moneyOnKill: the kill reward by weapon class, or the team-kill penalty.
func (s *Server) moneyOnKill(killer, victim *Player, weapon string) {
	if !s.economyOn() || killer == nil || killer == victim || weapon == "bigdiff" {
		return // the Big Diff's blast pays nothing and fines nothing (CS: C4 kills), teammates included
	}
	if killer.Team != "" && killer.Team == victim.Team && !s.raceMode() { // (a race is free for all)
		s.pay(killer, s.econ().TeamKillPenalty, "team kill")
		return
	}
	class, name := "", weapon
	if w := s.Weapons.Get(weapon); w != nil {
		class, name = w.Class, w.Name
	} else if weapon == "bigdiff" {
		class, name = "bomb", "The Big Diff"
	} else if it := CarItemOf(weapon); it != nil || weapon == "ram" || weapon == "wreck" {
		s.pay(killer, s.econ().Race.Kill, "wrecked a car") // a car item, a ram
		return
	}
	s.pay(killer, s.econ().KillReward[class], "kill: "+name)
}

func (s *Server) moneyOnPlant(p *Player)  { s.pay(p, s.econ().PlantReward, "planted the Big Diff") }
func (s *Server) moneyOnDefuse(p *Player) { s.pay(p, s.econ().DefuseReward, "reverted the Big Diff") }

// moneyOnRoundEnd: the win reward for the winners; the loss bonus (growing
// with the loss streak) for the losers, plus the planted-loss bonus for Vibe
// Coders who got the Big Diff down. Vibe Coders still alive when the time
// ran out without a plant get no loss bonus (CS: no saving for free).
func (s *Server) moneyOnRoundEnd(winner, reason string, planted bool) {
	e := s.econ()
	if s.lossStreak == nil {
		s.lossStreak = map[string]int{}
	}
	loser := content.Vibe
	if winner == content.Vibe {
		loser = content.Artisan
	}
	s.lossStreak[winner] = max(0, s.lossStreak[winner]-1)
	s.lossStreak[loser] = min(e.LossBonus.MaxStreak, s.lossStreak[loser]+1)
	bonus := min(e.LossBonus.Max, e.LossBonus.Base+e.LossBonus.Step*(s.lossStreak[loser]-1))
	for _, p := range s.ordered() {
		switch p.Team {
		case winner:
			s.pay(p, e.RoundWin[reason], "round won ("+reason+")")
		case loser:
			if loser == content.Vibe && reason == "time" && !planted && !p.Dead {
				continue
			}
			d := bonus
			why := fmt.Sprintf("round lost (loss bonus x%d)", s.lossStreak[loser])
			if loser == content.Vibe && planted {
				d += e.PlantedLossBonus
				why += " + planted"
			}
			s.pay(p, d, why)
		}
	}
}
