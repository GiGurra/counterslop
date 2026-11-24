package game

import (
	"encoding/json"
	"math"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// Grenades: Merge Conflict (HE), Smoke Test (smoke), Stack Trace (flash).
//
// A grenade is a weapon with action "throw" in its own slot (he / flash /
// smoke); its ammo is how many you carry. Firing it is the throw (the fire bit,
// like any shot, so prediction and the weapon clock stay as for guns); throwing
// the last one empties the slot and draws the best weapon left (both twins:
// Registry.fire, arsenal.js). The projectile is the server's: it flies, bounces
// off map boxes and goes off, and every snapshot carries the live ones ("nades").
//
// weapons.json "grenade": {kind he|flash|smoke, fuse s, radius m, duration s (smoke)}.

const (
	nadeSpeed       = 16.0 // m/s at full throw
	nadeRadius      = 0.08 // collision radius
	nadeBounce      = 0.45 // velocity kept along the axis it hits
	nadeFriction    = 0.75 // velocity kept along the floor per bounce
	nadeRollDrag    = 2.5  // 1/s: how fast a rolling grenade slows
	smokeMaxFuse    = 3.0  // a smoke pops when it settles, or after this long
	flashMaxBlind   = 4.5  // seconds, facing it point blank
	nadeCarryTotal  = 4    // CS: at most four grenades
	nadeExplodeLift = 0.2
)

// GrenadeSpec is a grenade's behaviour (weapons.json "grenade").
type GrenadeSpec struct {
	Kind     string  `json:"kind"` // he | flash | smoke
	Fuse     float64 `json:"fuse"`
	Radius   float64 `json:"radius"`
	Duration float64 `json:"duration"` // smoke: how long the cloud stands
}

// Nade is a grenade in the world: flying, or a standing smoke cloud.
type Nade struct {
	ID      int     `json:"id" net:"cold"`
	Weapon  string  `json:"w" net:"cold"`
	Kind    string  `json:"k" net:"cold"`
	Pos     Vec3    `json:"p" net:"hot,mm"`
	Vel     Vec3    `json:"v" net:"hot,mm"`
	State   string  `json:"st" net:"cold"`                  // fly | smoke
	Left    float64 `json:"left,omitempty" net:"hot,exact"` // smoke: seconds the cloud still stands
	by      *Player `net:"-"`
	age     float64 `net:"-"`
	w       *Weapon `net:"-"`
	resting bool    `net:"-"`
}

// throwNade launches p's grenade from the eye along the command's view. Callers hold s.mu.
func (s *Server) throwNade(p *Player, w *Weapon, c Cmd) {
	if w.Grenade == nil {
		return
	}
	cp := math.Cos(c.Pitch)
	d := Vec3{-math.Sin(c.Yaw) * cp, math.Sin(c.Pitch), -math.Cos(c.Yaw) * cp}
	b := p.body
	o := Vec3{b.Pos[0], b.Pos[1] + b.EyeH + b.EyeOff - 0.1, b.Pos[2]}
	v := Vec3{d[0]*nadeSpeed + b.Vel[0]*0.6, d[1]*nadeSpeed + 2 + b.Vel[1]*0.6, d[2]*nadeSpeed + b.Vel[2]*0.6}
	s.nextNade++
	s.nades = append(s.nades, &Nade{ID: s.nextNade, Weapon: w.ID, Kind: w.Grenade.Kind, Pos: o, Vel: v, State: "fly", by: p, w: w})
	b2, _ := json.Marshal(map[string]any{"t": "nade", "ev": "throw", "id": s.nextNade, "by": p.ID, "w": w.ID, "p": o})
	for _, q := range s.ordered() {
		trySend(q, b2)
	}
}

// nadeSolid reports whether a grenade at p overlaps a map box.
func (s *Server) nadeSolid(p Vec3) bool {
	for i := range s.Map.Boxes {
		b := &s.Map.Boxes[i]
		if p[0]+nadeRadius > b.Min[0] && p[0]-nadeRadius < b.Max[0] &&
			p[1]+nadeRadius > b.Min[1] && p[1]-nadeRadius < b.Max[1] &&
			p[2]+nadeRadius > b.Min[2] && p[2]-nadeRadius < b.Max[2] {
			return true
		}
	}
	return false
}

// stepNade moves one grenade by dt: gravity, then each axis on its own, bouncing
// off what it hits. Returns whether it bounced.
func (s *Server) stepNade(n *Nade, dt float64) bool {
	if n.resting {
		return false
	}
	n.Vel[1] -= s.cvarF("sv_gravity", 22) * dt
	bounced := false
	for _, ax := range []int{0, 2, 1} {
		old := n.Pos[ax]
		n.Pos[ax] += n.Vel[ax] * dt
		if !s.nadeSolid(n.Pos) {
			continue
		}
		n.Pos[ax] = old
		hard := math.Abs(n.Vel[ax]) > 1.5
		n.Vel[ax] = -n.Vel[ax] * nadeBounce
		if ax == 1 { // the floor (or a ceiling): a bounce scrubs speed, rolling slows it gently
			f := math.Exp(-nadeRollDrag * dt)
			if hard {
				f = nadeFriction
			}
			n.Vel[0] *= f
			n.Vel[2] *= f
			if math.Abs(n.Vel[1]) < 0.6 && math.Hypot(n.Vel[0], n.Vel[2]) < 0.3 {
				n.Vel, n.resting = Vec3{}, true
			}
		}
		bounced = bounced || hard
	}
	if n.Pos[1] < -20 { // out of the world
		n.Vel, n.resting = Vec3{}, true
	}
	return bounced
}

// tickNades flies the grenades and sets them off. Callers hold s.mu.
func (s *Server) tickNades(dt float64) {
	kept := s.nades[:0]
	for _, n := range s.nades {
		n.age += dt
		if n.State == "smoke" {
			n.Left -= dt
			if n.Left > 0 {
				kept = append(kept, n)
			}
			continue
		}
		bounced := false
		for range 2 {
			bounced = s.stepNade(n, dt/2) || bounced
		}
		if bounced {
			s.sendAll(map[string]any{"t": "nade", "ev": "bounce", "id": n.ID, "p": n.Pos})
		}
		g := n.w.Grenade
		switch {
		case g.Kind == "smoke" && (n.resting && n.age >= 0.5 || n.age >= smokeMaxFuse):
			n.State, n.Left, n.Vel = "smoke", g.Duration, Vec3{}
			s.sendAll(map[string]any{"t": "nade", "ev": "smoke", "id": n.ID, "w": n.Weapon, "p": n.Pos, "dur": g.Duration})
			kept = append(kept, n)
		case g.Kind != "smoke" && n.age >= g.Fuse:
			s.detonate(n)
		default:
			kept = append(kept, n)
		}
	}
	s.nades = kept
}

// lineClear: nothing solid between a and b.
func (s *Server) lineClear(a, b Vec3) bool {
	d := Vec3{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	l := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
	if l < 1e-6 {
		return true
	}
	d = Vec3{d[0] / l, d[1] / l, d[2] / l}
	return mapClear(&s.Map, a, d, l-1e-3)
}

// detonate sets off an HE or a flash. Callers hold s.mu.
func (s *Server) detonate(n *Nade) {
	g := n.w.Grenade
	at := Vec3{n.Pos[0], n.Pos[1] + nadeExplodeLift, n.Pos[2]}
	s.sendAll(map[string]any{"t": "nade", "ev": g.Kind, "id": n.ID, "w": n.Weapon, "p": at})
	for _, q := range s.ordered() {
		if q.Dead || q.Team == content.Spectator || !q.ready {
			continue
		}
		switch g.Kind {
		case "he":
			chest := Vec3{q.Pos[0], q.Pos[1] + 1.0, q.Pos[2]}
			dist := math.Sqrt(sq(chest[0]-at[0]) + sq(chest[1]-at[1]) + sq(chest[2]-at[2]))
			if dist >= g.Radius || !s.lineClear(at, chest) {
				continue
			}
			if q != n.by && !s.friendlyFire() && q.Team == n.by.Team {
				continue
			}
			dmg := heDamage(n.w, dist)
			if q.Bot || dmg <= 0 {
				continue // target practice bots never die
			}
			dmg = math.Round(absorb(q, n.w, "body", dmg))
			s.noteDamage(n.by, q, int(dmg), q.HP)
			q.HP -= int(dmg)
			hurt, _ := json.Marshal(map[string]any{"t": "hurt", "dmg": dmg, "from": n.by.ID, "hp": max(q.HP, 0)})
			trySend(q, hurt)
			if q != n.by {
				hm, _ := json.Marshal(hitMsg{T: "hit", ID: q.ID, Name: q.Name, Part: "body", Weapon: n.Weapon, Damage: dmg, At: chest})
				trySend(n.by, hm)
			}
			if q.HP <= 0 {
				s.killBy(n.by, q, n.Weapon, false)
			}
		case "flash":
			if q.Bot || q.body == nil {
				continue
			}
			eye := Vec3{q.Pos[0], q.Pos[1] + q.body.EyeH, q.Pos[2]}
			to := Vec3{at[0] - eye[0], at[1] - eye[1], at[2] - eye[2]}
			dist := math.Sqrt(sq(to[0]) + sq(to[1]) + sq(to[2]))
			if dist >= g.Radius || !s.lineClear(at, eye) {
				continue
			}
			cp := math.Cos(q.Pitch)
			view := Vec3{-math.Sin(q.Yaw) * cp, math.Sin(q.Pitch), -math.Cos(q.Yaw) * cp}
			dot := (view[0]*to[0] + view[1]*to[1] + view[2]*to[2]) / math.Max(dist, 1e-6)
			facing := 0.35 // behind you: a short white-out still
			if dot > 0.5 {
				facing = 1
			} else if dot > -0.2 {
				facing = 0.7
			}
			dur := flashMaxBlind * facing * (1 - 0.6*dist/g.Radius)
			if q.Shades {
				if k, _, _ := s.econ().gearSpec("shades"); k > 0 {
					dur *= k
				}
			}
			if dur > 0.25 {
				q.flashDur = math.Max(q.flashLeft(s.clock()), dur)
				q.flashUntil = s.clock().Add(time.Duration(q.flashDur * float64(time.Second)))
			}
		}
	}
}

func sq(v float64) float64 { return v * v }

// flashLeft is how many seconds of blindness p has left.
func (p *Player) flashLeft(now time.Time) float64 {
	return math.Max(0, p.flashUntil.Sub(now).Seconds())
}

// grenadeCount is how many grenades p carries in total.
func (s *Server) grenadeCount(inv *Inventory) int {
	n := 0
	for _, h := range inv.Slots {
		if w := s.Weapons.Get(h.ID); w != nil && w.Grenade != nil {
			n += h.Ammo
		}
	}
	return n
}

// buyGrenade adds one grenade of w to p (CS limits: the per-type carry in
// "mag", four in all). Returns why not, or "". Callers hold s.mu.
func (s *Server) buyGrenade(p *Player, w *Weapon) string {
	h := p.inv.Slots[w.Slot]
	if h != nil && h.ID == w.ID && h.Ammo >= w.Mag {
		return "can't carry more"
	}
	if s.grenadeCount(p.inv) >= nadeCarryTotal {
		return "can't carry more grenades"
	}
	cost := s.price(p, w.Price)
	if p.Money < cost {
		return "can't afford it"
	}
	p.Money -= cost
	if h != nil && h.ID == w.ID {
		h.Ammo++
	} else {
		p.inv.Slots[w.Slot] = &Held{ID: w.ID, Ammo: 1}
	}
	s.sendInv(p)
	return ""
}

// nadesJSON is the live grenades for the snapshot.
func (s *Server) nadesJSON() []byte {
	if len(s.nades) == 0 {
		return nil
	}
	b, _ := json.Marshal(s.nades)
	return append([]byte(`"nades":`), b...)
}
