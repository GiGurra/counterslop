//gojs:shared

package game

import (
	"math"

	"github.com/gigurra/counterslop/internal/content/rng"
	"github.com/gigurra/counterslop/internal/detmath"
)

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
// it decays exponentially (CS2: recoil and inaccuracy recover by a per-weapon
// recovery time, not at a fixed rate): recoil.recoveryTime is the seconds it
// takes to fall to 10% of where it was, whatever its size, so a 30-round spray
// resets about as fast as a tap: a rifle (0.43 s) is at 10% 0.43 s after the
// rest starts, 1% after 0.86 s.

// Recoil is a weapon's spray pattern.
type Recoil struct {
	Pattern      [][3]float64 `json:"pattern"`      // keyframes [shot index, up, right] (radians), ascending
	RecoveryTime float64      `json:"recoveryTime"` // seconds to recover 90% of the spray index, after one interval's rest (0 = at once)
}

// ln10: RecoveryTime is the time to 10%, so idx × 10^(-rest/T) = idx × e^(-ln10·rest/T).
const ln10 = 2.302585092994046

// recoilSnap: below this the index is 0 (an exponential never gets there).
const recoilSnap = 0.01

// RecoilIndex is the spray index at tick (fractional on the client's render
// clock): idx as of the shot at lastShot, decaying from one interval after it
// to 10% every recoveryTime seconds.
func RecoilIndex(idx float64, lastShot, tick, interval float64, recoveryTime float64) float64 {
	rest := tick - lastShot - interval
	if rest <= 0 || idx <= 0 {
		return max(idx, 0)
	}
	if recoveryTime <= 0 {
		return 0
	}
	r := idx * detmath.Exp(-ln10*rest/float64(recoveryTime*SimHz))
	if r < recoilSnap {
		return 0
	}
	return r
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
			return [2]float64{prev[1] + float64((k[1]-prev[1])*f), prev[2] + float64((k[2]-prev[2])*f)}
		}
		prev = k
	}
	return [2]float64{prev[1], prev[2]}
}

// recoilAngles turns an offset into the shot's [yaw, pitch]: up raises the
// pitch, right turns right (yaw decreases: forward is (-sin yaw, -cos yaw)).
func recoilAngles(yaw, pitch float64, off [2]float64, scale float64) (float64, float64) {
	return yaw - float64(off[1]*scale), math.Max(-1.6, math.Min(1.6, pitch+float64(off[0]*scale)))
}

// kick: a shot of w went off at tick. The shot uses the recovered index (kept
// in inv.shotIdx for serverShot), and the spray moves one shot on.
func (inv *Inventory) kick(w *Weapon, tick int64) {
	idx := RecoilIndex(inv.recoil, float64(inv.lastShot), float64(tick), float64(Ticks(w.Interval)), w.Recoil.RecoveryTime)
	inv.shotIdx, inv.recoil, inv.lastShot = idx, idx+1, tick
}

// aiRecoilControl is how much of the spray offset a bot pulls against, by
// bot_difficulty: the easy ones spray into the ceiling.

// SpreadSeed is shared with the client (web/game/arsenal.js) so predicted and
// authoritative shots go the same way.
func SpreadSeed(playerID, seq int) uint32 { return uint32(playerID)*2654435761 ^ uint32(seq) }

// RNG (= arsenal.js shotAngles). spread is the full cone width in radians.
func ShotAngles(yaw, pitch, spread float64, seed uint32, pellets int) [][2]float64 {
	r := rng.New(seed)
	out := make([][2]float64, max(1, pellets))
	for i := range out {
		// float64() is Go's rounding barrier (no FMA on any CPU, so JS gets
		// the same bits): around the products, and around Float() so its
		// inlined x*2^-32 isn't fused with the -0.5 (exact either way, but
		// TestNoFusedMultiplyAdd can't know that).
		dy := float64((float64(r.Float()) - 0.5) * spread)
		dp := float64((float64(r.Float()) - 0.5) * spread)
		out[i] = [2]float64{yaw + dy, pitch + dp}
	}
	return out
}

// ray traces one ray of a shot from o and applies its damage to the first
// player or bot hit before a wall. Returns "miss" or "<name> <part> <dmg>".
