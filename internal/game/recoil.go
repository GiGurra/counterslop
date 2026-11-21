package game

import "math"

// Recoil: CS-style spray patterns. Each gun has a fixed pattern of aim offsets
// by shot index in the spray (weapons.json recoil.pattern: keyframes
// [shot, up, right] in radians, linear between them, the last one held), so
// spray control is learnable: pull against the same shape every time. The
// offset moves where the shot goes, not the view: the client shows a view
// punch of a part of it (web/game/arsenal.js). Server and client run the same
// rules (= arsenal.js recoilIndex/recoilOffset); the index travels in the
// weapon state, and weapons_golden.json pins both sides.
//
// The spray index counts shots; once the trigger has rested for one interval
// it recovers at recoil.recover shots per second, so a short pause resets a
// tap and a long spray climbs back down in about recover⁻¹ × its length.

// Recoil is a weapon's spray pattern.
type Recoil struct {
	Pattern [][3]float64 `json:"pattern"` // keyframes [shot index, up, right] (radians), ascending
	Recover float64      `json:"recover"` // spray index recovered per second after one interval's rest
}

// RecoilIndex is the spray index at tick: idx as of the shot at lastShot,
// recovered at recover/s from one interval after it.
func RecoilIndex(idx float64, lastShot, tick, interval int64, recover float64) float64 {
	rest := tick - lastShot - interval
	if rest <= 0 || idx <= 0 {
		return max(idx, 0)
	}
	return max(0, idx-float64(rest)*recover/SimHz)
}

// RecoilOffset is the [up, right] aim offset at spray index idx.
func RecoilOffset(pattern [][3]float64, idx float64) [2]float64 {
	if len(pattern) == 0 || idx <= 0 {
		return [2]float64{}
	}
	prev := [3]float64{0, 0, 0}
	for _, k := range pattern {
		if idx <= k[0] {
			span := k[0] - prev[0]
			if span <= 0 {
				return [2]float64{k[1], k[2]}
			}
			f := (idx - prev[0]) / span
			return [2]float64{prev[1] + (k[1]-prev[1])*f, prev[2] + (k[2]-prev[2])*f}
		}
		prev = k
	}
	return [2]float64{prev[1], prev[2]}
}

// recoilAngles turns an offset into the shot's [yaw, pitch]: up raises the
// pitch, right turns right (yaw decreases: forward is (-sin yaw, -cos yaw)).
func recoilAngles(yaw, pitch float64, off [2]float64, scale float64) (float64, float64) {
	return yaw - off[1]*scale, math.Max(-1.6, math.Min(1.6, pitch+off[0]*scale))
}

// kick: a shot of w went off at tick. The shot uses the recovered index (kept
// in inv.shotIdx for serverShot), and the spray moves one shot on.
func (inv *Inventory) kick(w *Weapon, tick int64) {
	idx := RecoilIndex(inv.recoil, inv.lastShot, tick, Ticks(w.Interval), w.Recoil.Recover)
	inv.shotIdx, inv.recoil, inv.lastShot = idx, idx+1, tick
}

// aiRecoilControl is how much of the spray offset a bot pulls against, by
// bot_difficulty: the easy ones spray into the ceiling.
var aiRecoilControl = [4]float64{0.4, 0.65, 0.85, 0.95}

// aiPullDown aims a firing bot's command against the offset its next shot
// will get, by frac of it (as a player pulls the mouse down in a spray).
func (s *Server) aiPullDown(p *Player, c *Cmd, frac float64) {
	inv := p.inv
	h := inv.Slots[inv.Current]
	if h == nil {
		return
	}
	w := s.Weapons.Get(h.ID)
	if w == nil || len(w.Recoil.Pattern) == 0 {
		return
	}
	idx := RecoilIndex(inv.recoil, inv.lastShot, p.cmdTicks+1, Ticks(w.Interval), w.Recoil.Recover)
	off := RecoilOffset(w.Recoil.Pattern, idx)
	k := frac * s.recoilScale()
	c.Yaw, c.Pitch = c.Yaw+off[1]*k, math.Max(-1.5, math.Min(1.5, c.Pitch-off[0]*k))
}
