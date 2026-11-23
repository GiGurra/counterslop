package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Balance: time-to-kill tables computed from the weapon data (weapons.json,
// the player model's region multipliers, armor absorb), the same damage path
// the server applies to a hit. testdata/balance_bands.json pins target bands;
// TestBalanceBands fails when a data edit leaves one, and cmd/balance prints
// the table (CI job summary).

// heldSpeed: the move-speed scale of p's weapon in hand, times the energy
// drink's while it lasts and GPU Drop's while carrying (shared MoveSpeed). runCmd calls it after
// counting the command, so command number cmdTicks is boosted while <= boostEnd.
func (s *Server) heldSpeed(p *Player) float64 {
	if p.inv == nil || s.Weapons == nil {
		return 1
	}
	_, drink, _ := s.econ().gearSpec("drink")
	var w *Weapon
	if h := p.inv.Slots[p.inv.Current]; h != nil {
		w = s.Weapons.Get(h.ID)
	}
	return MoveSpeed(w, p.inv.boostEnd > 0 && p.cmdTicks <= p.inv.boostEnd, drink, p.inv.carry, s.econ().GPUDrop.CarrySpeed)
}

// falloffMetres is the distance over which a weapon's falloff factor applies
// once (CS: the range modifier per 500 units, about 12.7 m).
const falloffMetres = 12.7

// DamageAt is the damage of one hit at dist metres on a region with
// multiplier mult, before armor: damage × mult × falloff^(dist/12.7 m).
func (w *Weapon) DamageAt(mult, dist float64) float64 {
	f := w.Falloff
	if f <= 0 || f > 1 {
		f = 1
	}
	return w.Damage * mult * math.Pow(f, dist/falloffMetres)
}

// heDamage: an HE blast's damage (before armor) dist metres from the blast:
// full at the centre, linear to 0 at the radius. grenades.go applies it.
func heDamage(w *Weapon, dist float64) float64 {
	if w.Grenade == nil || w.Grenade.Radius <= 0 || dist >= w.Grenade.Radius {
		return 0
	}
	return math.Round(w.Damage * (1 - dist/w.Grenade.Radius))
}

// heTaken: what an HE takes off a full-HP player dist metres away, with or
// without a vest (the blast hits the body).
func heTaken(w *Weapon, dist float64, vest bool) int {
	p := &Player{HP: maxHP}
	if vest {
		p.Armor = 100
	}
	return int(math.Round(absorb(p, w, "body", heDamage(w, dist))))
}

// Armor states and the parts and distances the tables cover.
var (
	BalanceArmor = []string{"none", "vest", "helmet"} // helmet = vest + helmet
	BalanceParts = []string{"head", "body", "legs"}
	BalanceDists = []float64{5, 15, 30}
	// BalanceThrough: cells for a target behind another body (penetration.go):
	// the bullet passed through one player first.
	BalanceThrough = []int{0, 1}
)

// TTKCell: hits to kill a full-HP player and the time they take (first shot at 0).
type TTKCell struct {
	Weapon, Armor, Part string
	Dist                float64
	Through             int     // bodies the bullet went through first (penetration)
	Shots               int     // 0: can't kill (out of range, no damage)
	TTK                 float64 // seconds
	OverMag             bool    // needs more than one magazine
}

// Key: "part/armor/dist", plus "/through1" behind one body.
func (c TTKCell) Key() string {
	k := fmt.Sprintf("%s/%s/%g", c.Part, c.Armor, c.Dist)
	if c.Through > 0 {
		k += fmt.Sprintf("/through%d", c.Through)
	}
	return k
}

// shotsToKill plays hits on a fresh 100 HP player through absorb, behind
// `through` other bodies (each passed keeps the weapon's penetration).
func shotsToKill(w *Weapon, mult float64, armor, part string, dist float64, through int) int {
	p := &Player{HP: maxHP}
	if armor != "none" {
		p.Armor, p.Helmet = 100, armor == "helmet"
	}
	if w.Range > 0 && dist > w.Range {
		return 0
	}
	keep := 1.0
	for range through {
		keep *= penOf(w)
	}
	pellets := max(w.Pellets, 1) // a shotgun shot: every pellet on target (the best case), each absorbed on its own
	for n := 1; n <= 200; n++ {
		for range pellets {
			raw := w.DamageAt(mult, dist) * keep
			if through > 0 && raw < 1 {
				return 0 // spent before it gets here (penetrate)
			}
			dmg := math.Round(absorb(p, w, part, math.Round(raw)))
			if dmg <= 0 {
				return 0
			}
			p.HP -= int(dmg)
			if p.HP <= 0 {
				return n
			}
		}
	}
	return 0
}

// shotTime is the time between two shots (a bolt works the bolt in between).
func shotTime(w *Weapon) float64 {
	if w.Action == "bolt" {
		return math.Max(w.Interval, w.Cycle)
	}
	return w.Interval
}

// timeOfShot: when shot k (0-based) goes off with the trigger held. A burst
// fires its rounds burstGap apart, then waits interval for the next burst.
func timeOfShot(w *Weapon, k int) float64 {
	if w.Action == "burst" && w.Burst > 1 {
		cycle := float64(w.Burst-1)*w.BurstGap + w.Interval
		return float64(k/w.Burst)*cycle + float64(k%w.Burst)*w.BurstGap
	}
	return float64(k) * shotTime(w)
}

// rpm: rounds per minute with the trigger held.
func rpm(w *Weapon) float64 {
	if w.Action == "burst" && w.Burst > 1 {
		return 60 * float64(w.Burst) / (float64(w.Burst-1)*w.BurstGap + w.Interval)
	}
	return 60 / shotTime(w)
}

// BalanceTable: every weapon × armor × part × distance.
func BalanceTable(r *Registry, mult map[string]float64) []TTKCell {
	var out []TTKCell
	for i := range r.Weapons {
		w := &r.Weapons[i]
		if w.Grenade != nil || w.Action == "plant" {
			continue // thrown, not hitscan: no hit table (grenades.go has their damage); The Big Diff never fires
		}
		for _, through := range BalanceThrough {
			for _, a := range BalanceArmor {
				for _, part := range BalanceParts {
					for _, d := range BalanceDists {
						m, ok := mult[part]
						if !ok {
							m = 1
						}
						dist := d
						if w.Class == "melee" {
							dist = 0 // in reach: melee rows read "within its range"
						}
						n := shotsToKill(w, m, a, part, dist, through)
						c := TTKCell{Weapon: w.ID, Armor: a, Part: part, Dist: d, Through: through, Shots: n}
						if n > 0 {
							c.TTK = timeOfShot(w, n-1)
							c.OverMag = w.Mag > 0 && n > w.Mag
						}
						out = append(out, c)
					}
				}
			}
		}
	}
	return out
}

// Band: a target range for one cell's shots ([min, max]) and/or TTK in ms.
type Band struct {
	Shots []int     `json:"shots,omitempty"`
	TTKms []float64 `json:"ttkMs,omitempty"`
	Why   string    `json:"why,omitempty"`
}

// CheckBands returns one line per cell outside its band, and per weapon without bands.
func CheckBands(table []TTKCell, bands map[string]map[string]Band) []string {
	by := map[string]TTKCell{}
	weapons := map[string]bool{}
	for _, c := range table {
		by[c.Weapon+" "+c.Key()] = c
		weapons[c.Weapon] = true
	}
	var bad []string
	for w := range weapons {
		if len(bands[w]) == 0 {
			bad = append(bad, fmt.Sprintf("%s: no balance bands (add them to internal/game/testdata/balance_bands.json)", w))
		}
	}
	for w, bs := range bands {
		if !weapons[w] {
			bad = append(bad, fmt.Sprintf("%s: bands for a weapon that doesn't exist", w))
			continue
		}
		for key, b := range bs {
			c, ok := by[w+" "+key]
			if !ok {
				bad = append(bad, fmt.Sprintf("%s %s: no such cell (part/armor/dist[/through1]: head|body|legs / none|vest|helmet / 5|15|30, through1 = behind one body)", w, key))
				continue
			}
			if len(b.Shots) == 2 && (c.Shots < b.Shots[0] || c.Shots > b.Shots[1]) {
				bad = append(bad, fmt.Sprintf("%s %s: %d shots to kill, band %v (%s)", w, key, c.Shots, b.Shots, b.Why))
			}
			if ms := c.TTK * 1000; len(b.TTKms) == 2 && (c.Shots == 0 || ms < b.TTKms[0]-1e-6 || ms > b.TTKms[1]+1e-6) {
				bad = append(bad, fmt.Sprintf("%s %s: TTK %.0f ms, band %v ms (%s)", w, key, ms, b.TTKms, b.Why))
			}
		}
	}
	sort.Strings(bad)
	return bad
}

// BalanceMarkdown: per weapon, its archetype stats and a shots-to-kill (TTK)
// grid: rows part × armor, columns distance.
func BalanceMarkdown(r *Registry, e *Economy, table []TTKCell) string {
	by := map[string]TTKCell{}
	for _, c := range table {
		by[c.Weapon+" "+c.Key()] = c
	}
	var b strings.Builder
	b.WriteString("### Balance: hits to kill (time to kill) from 100 HP\n\n")
	b.WriteString("Generated from weapons.json, the player model's region multipliers and armor. `helmet` = vest + helmet. ⚠ = more than one magazine. Shotguns: every pellet on target (best case). Melee: in reach, whatever the column. *through a body*: the target stands behind another player the bullet went through first (penetration: the damage left).\n\n")
	var nades []*Weapon
	for i := range r.Weapons {
		w := &r.Weapons[i]
		if w.Grenade != nil {
			nades = append(nades, w)
			continue
		}
		if w.Action == "plant" {
			continue // The Big Diff: no damage table
		}
		reward := 0
		if e != nil {
			reward = e.KillReward[w.Class]
		}
		pen := armorRatio(w)
		dmgText := fmt.Sprintf("%g dmg", w.Damage)
		if w.Pellets > 1 {
			dmgText = fmt.Sprintf("%g dmg × %d pellets", w.Damage, w.Pellets)
		}
		if w.Action == "burst" {
			dmgText += fmt.Sprintf(" (%d-round bursts)", w.Burst)
		}
		fmt.Fprintf(&b, "**%s** (`%s`, %s): %s, %.0f RPM, %d/%d, reload %gs, armor pen %.0f%%, penetration %.0f%%, falloff %g/12.7 m, speed %.0f%%, $%d, kill $%d\n\n",
			w.Name, w.ID, w.Class, dmgText, rpm(w), w.Mag, w.Reserve, w.Reload, pen*100, penOf(w)*100, fallOr1(w.Falloff), speedOr1(w.Speed)*100, w.Price, reward)
		b.WriteString("| | 5 m | 15 m | 30 m |\n|---|---|---|---|\n")
		type row struct {
			part, armor string
			through     int
		}
		var rows []row
		for _, part := range BalanceParts {
			for _, a := range BalanceArmor {
				rows = append(rows, row{part, a, 0})
			}
		}
		if penOf(w) > 0 && w.Class != "melee" { // behind a body: the body rows the bands pin
			rows = append(rows, row{"body", "none", 1}, row{"body", "vest", 1})
		}
		for _, r := range rows {
			part, a := r.part, r.armor
			label := fmt.Sprintf("%s, %s", part, a)
			if r.through > 0 {
				label += ", through a body"
			}
			fmt.Fprintf(&b, "| %s |", label)
			for _, d := range BalanceDists {
				c := by[w.ID+" "+TTKCell{Part: part, Armor: a, Dist: d, Through: r.through}.Key()]
				if c.Shots == 0 {
					b.WriteString(" — |")
					continue
				}
				warn := ""
				if c.OverMag {
					warn = " ⚠"
				}
				fmt.Fprintf(&b, " %d (%.0f ms)%s |", c.Shots, c.TTK*1000, warn)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("**Grenades** (damage taken from 100 HP at a distance from the blast; flash and smoke do none)\n\n| | radius | $ | 1 m | 3 m | 6 m | 1 m, vest | 3 m, vest |\n|---|---|---|---|---|---|---|---|\n")
	for _, w := range nades {
		g := w.Grenade
		if g.Kind != "he" {
			fmt.Fprintf(&b, "| %s (%s) | %g m | %d | | | | | |\n", w.Name, g.Kind, g.Radius, w.Price)
			continue
		}
		fmt.Fprintf(&b, "| %s (he) | %g m | %d | %d | %d | %d | %d | %d |\n", w.Name, g.Radius, w.Price,
			heTaken(w, 1, false), heTaken(w, 3, false), heTaken(w, 6, false), heTaken(w, 1, true), heTaken(w, 3, true))
	}
	return b.String()
}

func fallOr1(f float64) float64 {
	if f <= 0 || f > 1 {
		return 1
	}
	return f
}
