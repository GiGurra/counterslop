package game

import (
	"fmt"
	"math"
)

// Bots out of ammo: when no gun has a round left (in the mag or in reserve),
// a bot draws the Mechanical Keyboard and charges the enemy it is fighting,
// swinging once it is within the keyboard's reach. A gun that can still
// reload is still a gun: the bot reloads it (or switches to a loaded one).
// There are no dropped weapons to pick up in this game, so the keyboard is
// the last resort.

// aiMeleeChase: an enemy farther than this isn't worth a charge with the
// keyboard; the bot keeps to its objective (aiming at them) instead.
const aiMeleeChase = 25.0

var aiGunSlots = []string{"primary", "sidearm"}

// aiGunSlot: the gun slot to fight with: the first with a loaded mag, else
// the first that can reload, else "" (out of ammo: the keyboard).
func aiGunSlot(p *Player) string {
	for _, sl := range aiGunSlots {
		if h := p.inv.Slots[sl]; h != nil && h.Ammo > 0 {
			return sl
		}
	}
	for _, sl := range aiGunSlots {
		if h := p.inv.Slots[sl]; h != nil && h.Reserve > 0 {
			return sl
		}
	}
	return ""
}

// aiArm picks the weapon for a fight (or for after one) and sets c.W when it
// isn't in hand. Returns whether the bot fights with the keyboard.
func (s *Server) aiArm(p *Player, c *Cmd) (melee bool) {
	sl := aiGunSlot(p)
	if sl == "" {
		if p.inv.Slots["melee"] == nil {
			return false
		}
		sl = "melee"
	}
	if p.inv.Current != sl {
		c.W = sl
	}
	return sl == "melee"
}

// aiCharge: run at tgt (dist away, eye to chest) with the keyboard out, the
// view on them (aiThink aims), jinking a little, and swing within reach.
func (s *Server) aiCharge(p *Player, c *Cmd, tgt *Player, dist float64) {
	b := p.ai
	reach := 2.0
	if w := s.Weapons.Get("keyboard"); w != nil {
		reach = w.Range
	}
	if dist > aiMeleeChase {
		s.aiMove(p, c, true) // too far to run at: keep to the objective
		return
	}
	if dist <= reach && p.inv.Current == "melee" {
		c.B |= BtnFire
		c.VT = float64(s.tick)
	}
	// Straight at them when the way is clear, else over the nav graph.
	goal := tgt.Pos
	next := goal
	if !s.segmentClear(p.Pos, goal, 0.3) {
		key := fmt.Sprintf("melee:%d", tgt.ID)
		if b.goalKey != key || p.cmdTicks >= b.repathAt || len(b.path) == 0 {
			b.goalKey, b.goal = key, goal
			b.path = s.navPath(p.Pos, goal)
			b.repathAt = p.cmdTicks + SimHz/2
		}
		for len(b.path) > 1 && math.Hypot(b.path[0][0]-p.Pos[0], b.path[0][2]-p.Pos[2]) < 0.7 {
			b.path = b.path[1:]
		}
		next = b.path[0]
	} else if b.goalKey != "" {
		b.goalKey, b.path = "", nil // back to the objective's route afterwards
	}
	dx, dz := next[0]-p.Pos[0], next[2]-p.Pos[2]
	if math.Hypot(dx, dz) > 0.4 {
		b.steer(c, math.Atan2(-dx, -dz), true)
	}
	// A jink now and then while closing (harder to hit than a straight line).
	if b.r.Float() < 0.03 {
		b.strafe = -b.strafe
	}
	if b.strafe == 0 {
		b.strafe = 1
	}
	if dist > reach {
		c.R = math.Max(-1, math.Min(1, c.R+0.4*b.strafe))
	}
	// Stuck on something: hop.
	if p.cmdTicks-b.stuckAt >= SimHz/2 {
		if math.Hypot(p.Pos[0]-b.lastPos[0], p.Pos[2]-b.lastPos[2]) < 0.3 && dist > reach {
			c.B |= BtnJump
			b.repathAt = p.cmdTicks
		}
		b.lastPos, b.stuckAt = p.Pos, p.cmdTicks
	}
}
