package game

import (
	"encoding/json"
	"math"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

// Grenades: Merge Conflict (HE), Smoke Test (smoke), Stack Trace (flash),
// Hot Reload / Burndown Chart (fire, fire.go).
//
// A grenade is a weapon with action "throw" in its own slot (he / flash /
// smoke); its ammo is how many you carry. CS throwing (Registry.throwTick,
// shared with the client's prediction): attack / attack2 pull the pin, letting
// go throws it: attack the full throw, attack2 the underhand lob, both the
// medium one (NadeLaunch). The fuse starts at the throw; throwing the last one
// empties the slot and draws the best weapon left. The projectile is the
// server's: it flies (at nadeGravity, CS: 0.4 of a player's), bounces off map
// boxes and goes off, and every snapshot carries the live ones ("nades").
//
// weapons.json "grenade": {kind he|flash|smoke|fire, fuse s (fire: the most
// it flies before it bursts), radius m, duration s (smoke, fire)}.

const (
	nadeGravityK    = 0.4  // a grenade falls at this much of sv_gravity (CS), so a full throw carries ~40 m
	nadeRadius      = 0.08 // collision radius
	nadeBounce      = 0.45 // velocity kept along the axis it hits (CS elasticity)
	nadeFriction    = 0.65 // velocity kept along the floor per bounce
	nadeRollDrag    = 3.5  // 1/s: how fast a rolling grenade slows (it stops about where it lands)
	smokeMaxFuse    = 6.0  // a smoke pops when it settles (CS), or after this long (a long throw flies 3 s)
	flashMaxBlind   = 4.5  // seconds, facing it point blank
	nadeCarryTotal  = 4    // CS: at most four grenades
	nadeExplodeLift = 0.2
)

// GrenadeSpec is a grenade's behaviour (weapons.json "grenade").
type GrenadeSpec struct {
	Kind     string  `json:"kind"` // he | flash | smoke | fire
	Fuse     float64 `json:"fuse"`
	Radius   float64 `json:"radius"`
	Duration float64 `json:"duration"` // smoke: how long the cloud stands; fire: how long the pool burns
}

// Nade is a grenade in the world: flying, a standing smoke cloud or a fire pool.
type Nade struct {
	ID     int     `json:"id" net:"cold"`
	Weapon string  `json:"w" net:"cold"`
	Kind   string  `json:"k" net:"cold"`
	Pos    Vec3    `json:"p" net:"hot,mm"`
	Vel    Vec3    `json:"v" net:"hot,mm"`
	State  string  `json:"st" net:"cold"`                  // fly | smoke | fire
	Left   float64 `json:"left,omitempty" net:"hot,exact"` // smoke, fire: seconds it still stands / burns
	// Flames: a fire pool's burning floor points (fire.go; each burns
	// flameReach around it). A smoke puts out the ones under it.
	Flames  []Vec3  `json:"fl,omitempty" net:"cold,mm"`
	by      *Player `net:"-"`
	age     float64 `net:"-"`
	w       *Weapon `net:"-"`
	resting bool    `net:"-"`
	floor   bool    `net:"-"` // it has touched a floor (a fire grenade bursts there)
}

// nadeGravity: how fast grenades fall (m/s²).
func (s *Server) nadeGravity() float64 { return s.cvarF("sv_gravity", 22) * nadeGravityK }

// throwNade launches p's grenade along the command's view at strength
// (ThrowStrength: 1 full, 0.5 medium, 0 lob; NadeLaunch). From the eye
// when the hand would be inside a wall. Callers hold s.mu.
func (s *Server) throwNade(p *Player, w *Weapon, strength float64, c Cmd) {
	if w.Grenade == nil {
		return
	}
	b := p.body
	eye := [3]float64{b.Pos[0], b.Pos[1] + b.EyeH + b.EyeOff, b.Pos[2]}
	o3, v3 := NadeLaunch(eye, c.Yaw, c.Pitch, strength, [3]float64(b.Vel))
	o, v := Vec3(o3), Vec3(v3)
	if s.nadeSolid(o) || !s.lineClear(Vec3(eye), o) {
		o = Vec3{eye[0], eye[1] - 0.1, eye[2]}
	}
	s.nextNade++
	s.nades = append(s.nades, &Nade{ID: s.nextNade, Weapon: w.ID, Kind: w.Grenade.Kind, Pos: o, Vel: v, State: "fly", by: p, w: w})
	b2, _ := json.Marshal(map[string]any{"t": "nade", "ev": "throw", "id": s.nextNade, "by": p.ID, "w": w.ID, "p": o})
	for _, q := range s.ordered() {
		trySend(q, b2)
	}
}

// nadeSolid reports whether a grenade at p overlaps a map solid.
func (s *Server) nadeSolid(p Vec3) bool { return s.nadeHit(p) >= 0 }

// nadeHit is the first map solid a grenade at p overlaps (-1: none).
func (s *Server) nadeHit(p Vec3) int {
	for i := range s.Map.Boxes {
		b := &s.Map.Boxes[i]
		if p[0]+nadeRadius > b.Min[0] && p[0]-nadeRadius < b.Max[0] &&
			p[1]+nadeRadius > b.Min[1] && p[1]-nadeRadius < b.Max[1] &&
			p[2]+nadeRadius > b.Min[2] && p[2]-nadeRadius < b.Max[2] &&
			(!sim.IsWedge(b) || !sim.AboveSlope(b, p[0], p[1]-nadeRadius, p[2], nadeRadius)) {
			return i
		}
	}
	return -1
}

// stepNade moves one grenade by dt: gravity, then each axis on its own, bouncing
// off what it hits (a wedge's slope: mirrored about its normal). Returns
// whether it bounced (hard) and whether it came down on a floor (n.floor: any
// touch, even a soft one).
func (s *Server) stepNade(n *Nade, dt float64) bool {
	if n.resting {
		return false
	}
	n.Vel[1] -= s.nadeGravity() * dt
	bounced := false
	for _, ax := range []int{0, 2, 1} {
		old := n.Pos
		n.Pos[ax] += n.Vel[ax] * dt
		i := s.nadeHit(n.Pos)
		if i < 0 {
			continue
		}
		n.Pos = old
		if b := &s.Map.Boxes[i]; sim.IsWedge(b) && sim.AboveSlope(b, old[0], old[1]-nadeRadius, old[2], nadeRadius) {
			bounced = s.slopeBounce(n, b, dt) || bounced
			continue
		}
		if ax == 1 && n.Vel[1] < 0 {
			n.floor = true
		}
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

// slopeBounce bounces a grenade off a wedge's slope: the part of its velocity
// into the slope comes back × nadeBounce; along a walkable slope it is a floor
// (friction, rolling, resting; downhill, gravity keeps it rolling).
func (s *Server) slopeBounce(n *Nade, b *content.Box, dt float64) bool {
	nn := sim.SlopeNormal(b)
	vn := n.Vel[0]*nn[0] + n.Vel[1]*nn[1] + n.Vel[2]*nn[2]
	if vn >= 0 {
		return false
	}
	hard := -vn > 1.5
	tan := Vec3{n.Vel[0] - vn*nn[0], n.Vel[1] - vn*nn[1], n.Vel[2] - vn*nn[2]}
	if sim.Walkable(b) {
		n.floor = true
		f := math.Exp(-nadeRollDrag * dt)
		if hard {
			f = nadeFriction
		}
		tan = Vec3{tan[0] * f, tan[1] * f, tan[2] * f}
		if -vn < 0.6 && math.Sqrt(tan[0]*tan[0]+tan[1]*tan[1]+tan[2]*tan[2]) < 0.3 && nn[1] > 0.95 {
			n.Vel, n.resting = Vec3{}, true
			return hard
		}
	}
	out := -vn * nadeBounce
	n.Vel = Vec3{tan[0] + out*nn[0], tan[1] + out*nn[1], tan[2] + out*nn[2]}
	return hard
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
		if n.State == "fire" {
			if s.burnTick(n, dt) {
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
			s.smokeOutFires(n)
			kept = append(kept, n)
		case g.Kind == "fire" && (n.floor || n.age >= g.Fuse):
			if s.ignite(n) {
				kept = append(kept, n)
			}
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
