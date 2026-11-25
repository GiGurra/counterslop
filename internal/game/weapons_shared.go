//gojs:shared

package game

// The weapon rules shared with the client: cmd/gojs translates this file (and
// the other //gojs:shared files of the package) into web/game/shared.gen.js,
// which web/game/arsenal.js runs for its predicted shots. One copy of the
// rules, so predicted and authoritative shots line up by construction.

import (
	"math"

	"github.com/gigurra/counterslop/internal/detmath"
	"github.com/gigurra/counterslop/internal/sim"
)

// Weapon is one entry of web/assets/weapons/weapons.json, the registry shared
// with the client. The server uses it to validate fire and compute damage.
type Weapon struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Slot     string       `json:"slot"`  // primary | sidearm | melee | he | flash | smoke | bomb
	Class    string       `json:"class"` // pistol | smg | rifle | shotgun | sniper | heavy | melee (kill reward)
	Team     *string      `json:"team"`
	Damage   float64      `json:"damage"`
	Interval float64      `json:"interval"`
	Action   string       `json:"action"`   // auto | semi | bolt | burst | throw | plant (never fires: rounds.go plants it)
	Cycle    float64      `json:"cycle"`    // bolt-action: seconds working the bolt after a shot
	Burst    int          `json:"burst"`    // burst: rounds per trigger pull
	BurstGap float64      `json:"burstGap"` // burst: seconds between the rounds of one burst
	Pellets  int          `json:"pellets"`  // shotgun: rays per shot (0 = 1), each with its own seeded spread
	Grenade  *GrenadeSpec `json:"grenade"`  // action "throw": what it does once thrown (grenades.go)
	Mag      int          `json:"mag"`
	Reserve  int          `json:"reserve"`
	Reload   float64      `json:"reload"`
	Range    float64      `json:"range"`
	Price    int          `json:"price"`
	ArmorPen float64      `json:"armorPen"`    // fraction of damage through armor (CS armor ratio); 0 = 0.5
	Falloff  float64      `json:"falloff"`     // damage × falloff per 12.7 m (CS range modifier); 0 = none
	Pen      float64      `json:"penetration"` // fraction of damage kept after passing through a body (penetration.go); 0 = stops at the first
	Speed    float64      `json:"speed"`       // max move speed while held, a fraction of the knife's (CS2 / 250); 0 = 1
	Spread   Spread       `json:"spread"`
	Recoil   Recoil       `json:"recoil"` // spray pattern (recoil.go)
}

// Spread in radians: base + per m/s moving + in the air (+ unscoped) (+ belt ×
// the empty fraction of the mag), times crouch.
type Spread struct {
	Base     float64 `json:"base"`
	Move     float64 `json:"move"`
	Air      float64 `json:"air"`
	Crouch   float64 `json:"crouch"`
	Unscoped float64 `json:"unscoped"`
	Belt     float64 `json:"belt"` // heavy: accuracy degrades as the belt empties
}

type Registry struct {
	Weapons []Weapon `json:"weapons"`
	Slots   []string `json:"slots"`
	byID    map[string]*Weapon
}

// NewRegistry indexes a weapon list (the client builds its registry from
// weapons.json with it; LoadWeapons does the same).
func NewRegistry(weapons []Weapon, slots []string) *Registry {
	r := &Registry{Weapons: weapons, Slots: slots}
	r.index()
	return r
}

func (r *Registry) index() {
	r.byID = map[string]*Weapon{}
	for i := range r.Weapons {
		r.byID[r.Weapons[i].ID] = &r.Weapons[i]
	}
}

func (r *Registry) Get(id string) *Weapon { return r.byID[id] }

// Held is one weapon a player carries, with its ammo.
type Held struct {
	ID      string `json:"id" net:"cold,self"`
	Ammo    int    `json:"ammo" net:"cold,self"`
	Reserve int    `json:"reserve" net:"cold,self"`
}

// Inventory is a player's slots, current slot and fire/reload timing.
type Inventory struct {
	Slots   map[string]*Held `json:"slots" net:"cold,self"`
	Current string           `json:"cur" net:"cold,self"`
	// Timers in command ticks (Player.cmdTicks), mirrored exactly by
	// web/game/arsenal.js so predicted and authoritative shots line up.
	nextShot  int64   `net:"-"` // first tick the next shot may go off
	cycleEnd  int64   `net:"-"` // bolt: tick the bolt is worked (no shot, trigger not taken, before)
	reloadEnd int64   `net:"-"` // tick the reload completes (0 = not reloading)
	trigger   bool    `net:"-"` // semi/bolt/burst: this trigger press has been used
	boostEnd  int64   `net:"-"` // energy drink: commands up to this cmdTicks move faster (arsenal.js boostEnd)
	carry     bool    `net:"-"` // GPU Drop: carrying a GPU, moves at carrySpeed (syncCarry; arsenal.js carry)
	burstLeft int     `net:"-"` // burst: rounds of the current burst still to fire (they fire with the trigger released too)
	prev      string  `net:"-"` // the slot held before the last switch (server only: where a plant leaves you)
	recoil    float64 `net:"-"` // spray index as of the last shot (recoil.go)
	lastShot  int64   `net:"-"` // tick of the last shot
	shotIdx   float64 `net:"-"` // the spray index the last shot used
	useBack   string  `net:"-"` // +use drew The Big Diff (UseDraw): the slot to go back to on release
}

// BombSlot is the inventory slot of The Big Diff (key 5; weapons.json "bomb").
const BombSlot = "bomb"

// DeployTicks is how long a freshly drawn weapon can't fire (arsenal.js:
// RAISE_TIME * 0.6).
const DeployTicks = 21

// Ticks converts seconds to whole command ticks, rounding up (as arsenal.js ticks()).
func Ticks(sec float64) int64 { return int64(math.Ceil(float64(sec*SimHz) - 1e-6)) }

// settle finishes a reload whose time has come.
func (r *Registry) settle(inv *Inventory, tick int64) {
	if inv.reloadEnd == 0 || tick < inv.reloadEnd {
		return
	}
	inv.reloadEnd = 0
	h := inv.Slots[inv.Current]
	if h == nil {
		return
	}
	w := r.Get(h.ID)
	take := min(w.Mag-h.Ammo, h.Reserve)
	h.Ammo += take
	h.Reserve -= take
}

// fire runs one tick of a held trigger at command tick `tick`: it fires the
// held weapon, or says why not ("no weapon", "reloading", "cycling", "semi",
// "empty", "cooldown"). The rules and their order are arsenal.js a.tick's.
func (r *Registry) fire(inv *Inventory, tick int64) (*Weapon, string) {
	r.settle(inv, tick)
	h := inv.Slots[inv.Current]
	if h == nil {
		return nil, "no weapon"
	}
	if w := r.Get(h.ID); w != nil && w.Action == "plant" { // The Big Diff: the trigger plants (rounds.go), nothing fires
		return nil, "plant"
	}
	if inv.reloadEnd != 0 {
		inv.burstLeft = 0
		return nil, "reloading"
	}
	if tick < inv.cycleEnd {
		return nil, "cycling"
	}
	w := r.Get(h.ID)
	if inv.burstLeft > 0 { // the rest of a burst: on its own clock, trigger or not
		if tick < inv.nextShot {
			return nil, "cooldown"
		}
		if w.Mag > 0 && h.Ammo <= 0 {
			inv.burstLeft = 0
			return nil, "empty"
		}
		inv.burstLeft--
		inv.nextShot = tick + Ticks(w.Interval)
		if inv.burstLeft > 0 {
			inv.nextShot = tick + Ticks(w.BurstGap)
		}
		if w.Mag > 0 {
			h.Ammo--
		}
		inv.kick(w, tick)
		return w, ""
	}
	if w.Action != "auto" && inv.trigger {
		return nil, "semi"
	}
	inv.trigger = true
	if w.Mag > 0 && h.Ammo <= 0 {
		return nil, "empty"
	}
	if tick < inv.nextShot {
		return nil, "cooldown"
	}
	inv.nextShot = tick + Ticks(w.Interval)
	if w.Action == "burst" && w.Burst > 1 {
		inv.burstLeft = w.Burst - 1
		inv.nextShot = tick + Ticks(w.BurstGap)
	}
	if w.Action == "bolt" {
		cycle := w.Cycle
		if cycle == 0 {
			cycle = w.Interval
		}
		inv.cycleEnd = tick + Ticks(cycle)
	}
	if w.Mag > 0 {
		h.Ammo--
	}
	inv.kick(w, tick)
	if w.Action == "throw" && h.Ammo <= 0 { // the last grenade left the hand
		delete(inv.Slots, inv.Current)
		inv.Current = r.bestSlot(inv)
		inv.drawn(tick)
	}
	return w, ""
}

// bestSlot is the first slot (in registry order) that holds something.
func (r *Registry) bestSlot(inv *Inventory) string {
	for _, s := range r.Slots {
		if inv.Slots[s] != nil {
			return s
		}
	}
	return inv.Current
}

// release is a tick without the trigger held.
func (inv *Inventory) release() { inv.trigger = false }

// trigger is one command tick of the trigger (held or not), after switch,
// reload and settle: a released trigger resets, and the shot (if any) goes off,
// which for a burst weapon may be the rest of a burst with the trigger up.
// arsenal.js a.tick follows the same steps.
func (r *Registry) trigger(inv *Inventory, tick int64, firing bool) (*Weapon, string) {
	if !firing {
		inv.release()
		if inv.burstLeft == 0 {
			return nil, ""
		}
	}
	return r.fire(inv, tick)
}

func (r *Registry) reload(inv *Inventory, tick int64) {
	r.settle(inv, tick)
	h := inv.Slots[inv.Current]
	if h == nil || inv.reloadEnd != 0 {
		return
	}
	w := r.Get(h.ID)
	if w.Mag == 0 || h.Ammo >= w.Mag || h.Reserve == 0 {
		return
	}
	inv.reloadEnd = tick + Ticks(w.Reload)
}

func (r *Registry) switchTo(inv *Inventory, slot string, tick int64) bool {
	if inv.Slots[slot] == nil || inv.Current == slot {
		return false
	}
	r.settle(inv, tick) // a reload that finished stays finished (the client settles every tick)
	inv.prev, inv.Current = inv.Current, slot
	inv.useBack = "" // a switch of your own: letting go of E keeps it
	inv.drawn(tick)
	return true
}

// UseDraw is +use's part of the weapon rules, after the command's own
// switch: pressing E in a bomb site with The Big Diff in slot 5 draws it (the
// plant then runs while E is held, rounds.go roundUse), unless E is taking a
// floor weapon in sight (aimItem: AimedItem, which comes first); letting go
// of E puts back the weapon it replaced (a finished plant has taken The Big
// Diff away already, syncBombSlots). Returns whether the slot changed.
func (r *Registry) UseDraw(inv *Inventory, b, prevB int, inSite, aimItem bool, tick int64) bool {
	if b&BtnUse == 0 {
		back := inv.useBack
		inv.useBack = ""
		if back == "" || inv.Current != BombSlot {
			return false
		}
		return r.switchTo(inv, back, tick)
	}
	if prevB&BtnUse != 0 || !inSite || aimItem || inv.Current == BombSlot || inv.Slots[BombSlot] == nil {
		return false
	}
	back := inv.Current
	if !r.switchTo(inv, BombSlot, tick) {
		return false
	}
	inv.useBack = back
	return true
}

// TakeRange is how far from the eye a floor weapon can be taken with E (m:
// one dropped with G, 1.4 m ahead, is 2.05 m from a standing eye);
// takeCos: within about 20 degrees of the view.
const (
	TakeRange = 2.3
	takeCos   = 0.94
)

// AimedItem: of the floor weapons at items (their floor points), the index
// of the one the view from eye (yaw, pitch) points at, within TakeRange and
// takeCos (the nearest to the view's line wins), or -1. The server takes it
// on E (weapons.go takeItem); the client shows the prompt and predicts that
// this E press doesn't draw The Big Diff.
func AimedItem(eye [3]float64, yaw, pitch float64, items [][3]float64) int {
	cp := detmath.Cos(pitch)
	dx, dy, dz := -float64(detmath.Sin(yaw)*cp), detmath.Sin(pitch), -float64(detmath.Cos(yaw)*cp)
	best, bestCos := -1, takeCos
	for i := range items {
		vx, vy, vz := items[i][0]-eye[0], float64(items[i][1]+0.1)-eye[1], items[i][2]-eye[2]
		d := math.Sqrt(float64(vx*vx) + float64(vy*vy) + float64(vz*vz))
		if d > TakeRange || d < 1e-6 {
			continue
		}
		c := (float64(vx*dx) + float64(vy*dy) + float64(vz*dz)) / d
		if c > bestCos {
			best, bestCos = i, c
		}
	}
	return best
}

// drawn: a weapon was just taken in hand: no reload, no bolt, a short deploy.
func (inv *Inventory) drawn(tick int64) {
	inv.reloadEnd, inv.cycleEnd, inv.burstLeft = 0, 0, 0
	inv.recoil, inv.shotIdx = 0, 0 // a fresh spray
	inv.nextShot = max(inv.nextShot, tick+DeployTicks)
}

// accurateFrac: below this fraction of the weapon's max speed a shot has no
// movement inaccuracy (CS: 34%), so stopping before you shoot (counter-
// strafing) matters. arsenal.js spread uses the same rule.
const accurateFrac = 0.34

// movePenalty: the m/s of horizontal speed that count toward the movement
// spread: everything above the accurate speed.
func movePenalty(speed float64, w *Weapon) float64 {
	return math.Max(0, speed-float64(accurateFrac*sim.Run*speedOr1(w.Speed)))
}

func speedOr1(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}

// HoldBoost: a frozen command (round freeze time) doesn't use up the energy
// drink, so one bought before the round starts lasts its full length from the
// first command that can move (server runCmd, arsenal.js tick with frozen).
// tick is the command's number; a drink still running at it is pushed back one.
func HoldBoost(inv *Inventory, tick int64) {
	if inv.boostEnd >= tick {
		inv.boostEnd++
	}
}

// MoveSpeed is the max move-speed scale: the weapon in hand's (0 or none =
// 1), times the energy drink's while boosted and the GPU carry's (0 = none).
// The order of the products is part of the rule (the client must get the
// same bits).
func MoveSpeed(w *Weapon, boosted bool, drink float64, carry bool, carrySpeed float64) float64 {
	k := 1.0
	if boosted && drink > 0 {
		k = drink
	}
	if carry && carrySpeed > 0 {
		k *= carrySpeed
	}
	sp := 1.0
	if w != nil && w.Speed > 0 {
		sp = w.Speed
	}
	return k * sp
}

// ShotSpread is the spread in radians of a shot by w (held: h, after this
// shot's round left the mag) from body: base, plus movement above the accurate
// speed, in the air, unscoped (scoped: the scope is up), and a belt weapon's
// empty fraction, times crouch.
func ShotSpread(w *Weapon, h *Held, body *sim.Body, scoped bool) float64 {
	sp := w.Spread
	r := sp.Base + float64(movePenalty(detmath.Hypot(body.Vel[0], body.Vel[2]), w)*sp.Move)
	if !body.OnGround {
		r += sp.Air
	}
	if sp.Unscoped > 0 && !scoped {
		r += sp.Unscoped
	}
	if sp.Belt > 0 && w.Mag > 0 && h != nil {
		r += float64(sp.Belt * (1 - float64(h.Ammo)/float64(w.Mag)))
	}
	if body.Crouched {
		r *= sp.Crouch
	}
	return r
}
