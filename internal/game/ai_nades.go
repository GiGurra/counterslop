package game

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// Bots and grenades. A throw is an ordinary command (draw the slot, then the
// fire bit), so it goes through the same sim path as a player's.
//
//   - Vibe Coders on their way to a site stop where they can see into it,
//     lob an HE into it (whoever holds the corner), then a flash, turn away
//     until it pops and push.
//   - Artisans throw their HE at an enemy they see at mid range.
//
// Bots buy them with what is left after guns and armor (aiBuy).

const (
	aiNadeTimeout  = 300 // command ticks: give up a throw that never happens
	aiAvertTicks   = 170 // turned away from our own flash until it has popped
	aiSiteNadeMin  = 8.0 // metres from the site's middle: close enough to lob into it ...
	aiSiteNadeMax  = 22.0
	aiEnemyNadeMin = 10.0 // ... and the range an Artisan throws an HE at an enemy
	aiEnemyNadeMax = 25.0
)

// aiNadeState is one throw in progress (or the look-away after a flash).
type aiNadeState struct {
	slot    string // he | flash | smoke while drawing / aiming / throwing
	at      Vec3   // where it should land
	enemy   bool   // thrown at an enemy (keeps going while engaged)
	ammo    int    // carried when the throw started (fewer = it left the hand)
	started int64
	avert   int64 // look away (from our own flash) until this tick
	avYaw   float64
	flash   int             // the flash we look away from (it may bounce back toward us)
	round   int             // the round `thrown` is for
	thrown  map[string]bool // slots already used this round
}

// lobPitch is the pitch that lands a throw from eye o at about t (no walls),
// on the lower arc; ok=false if it is out of reach.
func (s *Server) lobPitch(o, t Vec3) (float64, bool) {
	g := s.cvarF("sv_gravity", 22)
	dh := math.Hypot(t[0]-o[0], t[2]-o[2]) * 0.85 // it bounces and rolls on a bit
	dy := t[1] - o[1]
	best, bestErr := 0.0, math.Inf(1)
	for th := -0.5; th <= 0.9; th += 0.005 {
		vx, vy := nadeSpeed*math.Cos(th), nadeSpeed*math.Sin(th)+2
		disc := vy*vy - 2*g*dy
		if disc < 0 {
			continue
		}
		reach := vx * (vy + math.Sqrt(disc)) / g
		if e := math.Abs(reach - dh); e < bestErr {
			best, bestErr = th, e
		}
		if reach > dh+1 { // past it: the lower arc is the one already found
			break
		}
	}
	return best, bestErr < 1.5
}

// aiStartNade begins a throw of slot at `at` if the bot carries one it hasn't used this round.
func (s *Server) aiStartNade(p *Player, slot string, at Vec3, enemy bool) bool {
	n := &p.ai.nade
	if n.round != s.round.Number || n.thrown == nil { // warmup is round 0 too
		n.round, n.thrown = s.round.Number, map[string]bool{}
	}
	h := p.inv.Slots[slot]
	if h == nil || h.Ammo <= 0 || n.thrown[slot] || n.slot != "" {
		return false
	}
	if _, ok := s.lobPitch(s.eyeOf(p), at); !ok {
		return false
	}
	n.slot, n.at, n.enemy, n.ammo, n.started = slot, at, enemy, h.Ammo, p.cmdTicks
	return true
}

// aiDodgeFlash: a teammate's flash in the air ("flash out!") that would go
// off in front of p makes p look away until it has popped (as after its own).
func (s *Server) aiDodgeFlash(p *Player) {
	n := &p.ai.nade
	if n.slot != "" || n.avert > p.cmdTicks {
		return
	}
	o := s.eyeOf(p)
	for _, g := range s.nades {
		if g.Kind != "flash" || g.State != "fly" || g.by == nil || g.by.Team != p.Team || g.w.Grenade == nil {
			continue
		}
		d := math.Hypot(g.Pos[0]-o[0], g.Pos[2]-o[2])
		toward := math.Atan2(-(g.Pos[0] - o[0]), -(g.Pos[2] - o[2]))
		if d > g.w.Grenade.Radius || math.Abs(angleDiff(toward, p.ai.yaw)) > 1.6 {
			continue
		}
		left := int64(math.Ceil((g.w.Grenade.Fuse-g.age)*SimHz)) + 15
		n.avert, n.avYaw, n.flash = p.cmdTicks+left, toward+math.Pi, g.ID
		return
	}
}

// aiNadeBusy: a throw or the look-away after one is in progress.
func (b *aiBrain) aiNadeBusy(now int64) bool { return b.nade.slot != "" || b.nade.avert > now }

// aiNadeStop abandons a throw (an enemy showed up): back to the rifle.
func (s *Server) aiNadeStop(p *Player, c *Cmd) {
	n := &p.ai.nade
	if n.slot != "" && p.inv.Current == n.slot {
		c.W = s.aiBestGun(p)
	}
	n.slot, n.avert = "", 0
}

// aiNade writes this tick's command for the throw in progress. Callers checked aiNadeBusy.
func (s *Server) aiNade(p *Player, c *Cmd) {
	b, n := p.ai, &p.ai.nade
	sk := aiSkill[s.aiDifficulty()]
	if n.slot == "" { // looking away until the flash has popped, from wherever it is now
		for _, g := range s.nades {
			if g.ID == n.flash {
				o := s.eyeOf(p)
				n.avYaw = math.Atan2(-(g.Pos[0]-o[0]), -(g.Pos[2]-o[2])) + math.Pi
			}
		}
		b.turnTo(n.avYaw, 0.3, sk.turn) // and a little up, off the floor it lands on
		return
	}
	h := p.inv.Slots[n.slot]
	if h == nil || h.Ammo < n.ammo { // it left the hand
		n.thrown[n.slot] = true
		if WeaponKind(s.Weapons.Get(p.ai.nadeWeapon(n.slot, s))) == "flash" {
			n.avert, n.avYaw, n.flash = p.cmdTicks+aiAvertTicks, b.yaw+math.Pi, 0
			for _, g := range s.nades { // the one that just left our hand
				if g.by == p && g.Kind == "flash" && g.ID > n.flash {
					n.flash = g.ID
				}
			}
		}
		if p.inv.Current == n.slot || h == nil {
			c.W = s.aiBestGun(p)
		}
		n.slot = ""
		return
	}
	if p.cmdTicks-n.started > aiNadeTimeout {
		s.aiNadeStop(p, c)
		return
	}
	o := s.eyeOf(p)
	yaw := math.Atan2(-(n.at[0] - o[0]), -(n.at[2] - o[2]))
	pitch, _ := s.lobPitch(o, n.at)
	b.turnTo(yaw, pitch, sk.turn)
	if p.inv.Current != n.slot {
		c.W = n.slot
		return
	}
	aimed := math.Abs(angleDiff(yaw, b.yaw)) < 0.03 && math.Abs(pitch-b.pitch) < 0.03
	if aimed && p.inv.nextShot <= p.cmdTicks+1 && !b.lastFire {
		c.B |= BtnFire
	}
}

// nadeWeapon is the id of the grenade in slot (remembered when it is gone).
func (b *aiBrain) nadeWeapon(slot string, s *Server) string {
	for i := range s.Weapons.Weapons {
		if w := &s.Weapons.Weapons[i]; w.Slot == slot && w.Grenade != nil {
			return w.ID
		}
	}
	return ""
}

// WeaponKind is a grenade's kind (he | flash | smoke), "" for guns.
func WeaponKind(w *Weapon) string {
	if w == nil || w.Grenade == nil {
		return ""
	}
	return w.Grenade.Kind
}

// aiBestGun: the slot to go back to after a throw.
// A gun with ammo (or a reload) first, then any weapon at all.
func (s *Server) aiBestGun(p *Player) string {
	if sl := aiGunSlot(p); sl != "" {
		return sl
	}
	for _, sl := range []string{"melee", "primary", "sidearm"} {
		if p.inv.Slots[sl] != nil {
			return sl
		}
	}
	return p.inv.Current
}

// aiSiteNades: a Vibe Coder near their site with a clear view into it starts
// the next grenade: HE first, then the flash. Returns whether one started.
func (s *Server) aiSiteNades(p *Player) bool {
	if p.Team != content.Vibe || s.frozen() {
		return false
	}
	z := s.siteZone(p.ai.site)
	if z == nil {
		return false
	}
	o := s.eyeOf(p)
	if d := math.Hypot(z.Center()[0]-o[0], z.Center()[2]-o[2]); d < aiSiteNadeMin || d > aiSiteNadeMax {
		return false
	}
	// The first open spot of the site we can see: its middle, then its quarters
	// (a site's middle is often a crate stack).
	for _, f := range [][2]float64{{0.5, 0.5}, {0.25, 0.25}, {0.75, 0.25}, {0.25, 0.75}, {0.75, 0.75}} {
		x, zz := z.Min[0]+(z.Max[0]-z.Min[0])*f[0], z.Min[2]+(z.Max[2]-z.Min[2])*f[1]
		top := s.groundTop(x, zz)
		if top > z.Min[1]+2 { // a crate stack, not the floor of the site
			continue
		}
		at := Vec3{x, top + 0.2, zz}
		if !s.lineClear(o, Vec3{at[0], at[1] + 1, at[2]}) {
			continue
		}
		return s.aiStartNade(p, "he", at, false) || s.aiStartNade(p, "flash", at, false)
	}
	return false
}

// groundTop is the height of the highest box top over (x, z): where a grenade lands.
func (s *Server) groundTop(x, z float64) float64 {
	top := math.Inf(-1)
	for i := range s.Map.Boxes {
		b := &s.Map.Boxes[i]
		if x >= b.Min[0] && x <= b.Max[0] && z >= b.Min[2] && z <= b.Max[2] && b.Max[1] > top {
			top = b.Max[1]
		}
	}
	return top
}

// aiBuyNades spends what is left on grenades (after guns and armor).
func (s *Server) aiBuyNades(p *Player) {
	first, second := "stacktrace", "mergeconflict" // Vibe Coders open sites with flashes
	if p.Team == content.Artisan {
		first, second = "mergeconflict", "stacktrace"
	}
	for _, id := range []string{first, second} {
		w := s.Weapons.Get(id)
		if w != nil && p.Money >= w.Price+200 && p.ai.r.Float() < 0.7 {
			s.purchase(p, id)
		}
	}
}
