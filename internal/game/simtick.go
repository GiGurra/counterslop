package game

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/rng"
	"github.com/gigurra/counterslop/internal/sim"
)

// The authoritative simulation. Clients send input commands (one per client
// sim tick, batched); the server runs one command per player per tick with the
// same movement code as the client (internal/sim, parity-tested against
// physics.js), fires weapons, applies damage, kills and respawns. Snapshots go
// out every snapEvery ticks and tell each client the last command it applied.

const (
	SimHz     = 100 // shared with the client (sent in welcome)
	snapEvery = 3   // 100/3 ≈ 33 snapshots per second
	maxQueued = 64  // commands buffered per player (0.64 s); beyond that the oldest are dropped
	// Command budget: a player may run commands as fast as real time passes
	// (plus cmdDrift for client clocks running fast). Budget unused while no
	// commands arrive (a frame hitch, a network burst) is banked up to cmdBank,
	// so the late commands run at once instead of becoming a backlog. A
	// speedhack gains at most cmdDrift-1 in speed plus the bank once.
	cmdDrift = 1.03
	cmdBank  = 50 // 0.5 s
	cmdBurst = 20 // server tick stalls caught up per tick, at most
	maxHP    = 100
)

// Command buttons.
const (
	BtnJump = 1 << iota
	BtnCrouch
	BtnWalk
	BtnFire
	BtnScope
	BtnReload // down edge starts a reload
	BtnUse    // plant / defuse The Big Diff (held)
)

// Cmd is one client tick of input.
type Cmd struct {
	Seq   int     `json:"s"`
	F     float64 `json:"f"`
	R     float64 `json:"r"`
	Yaw   float64 `json:"y"`
	Pitch float64 `json:"p"`
	B     int     `json:"b"`
	W     string  `json:"w,omitempty"`  // switch to this slot first
	VT    float64 `json:"vt,omitempty"` // fire commands: the server tick the shooter was seeing
}

type killMsg struct {
	T        string `json:"t"`
	Killer   int    `json:"k"`
	Victim   int    `json:"v"`
	Weapon   string `json:"w"`
	Headshot bool   `json:"hs,omitempty"`
	NoScope  bool   `json:"ns,omitempty"` // a scoped weapon fired unscoped (kill feed flair)
}

// queueCmds accepts a client's command batch (resends of old seqs are ignored).
// Callers hold s.mu.
func (s *Server) queueCmds(p *Player, cmds []Cmd) {
	if p.Team == content.Spectator {
		return // no body to move
	}
	if p.body == nil {
		p.body = sim.NewBody(sim.Vec3(p.Pos))
		p.body.Yaw = p.Yaw
	}
	for _, c := range cmds {
		// Already run (or acknowledged by a teleport), or not a number: drop.
		if c.Seq <= p.ackSeq || !finiteF(c.F) || !finiteF(c.R) || !finiteF(c.Yaw) || !finiteF(c.Pitch) {
			continue
		}
		c.F, c.R = clamp1(c.F), clamp1(c.R)
		c.Pitch = math.Max(-1.6, math.Min(1.6, c.Pitch))
		if c.Seq <= p.lastQueued {
			// Overtaken by newer packets (datagrams reorder) but not run yet:
			// slot it in by seq, unless it is already queued (a resend).
			at, found := slices.BinarySearchFunc(p.cmds, c.Seq, func(q Cmd, seq int) int { return q.Seq - seq })
			if !found {
				p.cmds = slices.Insert(p.cmds, at, c)
			}
			continue
		}
		p.lastQueued = c.Seq
		p.cmds = append(p.cmds, c)
		if len(p.cmds) > maxQueued && (!s.Test || p.paced) {
			// Far behind (stall or speedhack): drop the oldest, acknowledged as
			// done, so the newest input (and fire) is never the one lost; the
			// client's prediction corrects itself from the next snapshot.
			p.ackSeq = p.cmds[0].Seq
			p.lastCmd = p.cmds[0]
			p.cmds = p.cmds[1:]
			p.cmdsDropped++
		}
	}
}

func clamp1(v float64) float64 { return math.Max(-1, math.Min(1, v)) }

// simTick runs the commands that have arrived for every command-driven
// player, as many as its budget allows (real time since the last tick, so a
// late or skipped server tick catches up and a slightly fast client clock
// never builds a backlog). A player without commands is not simulated: no
// extrapolation, its next commands just run later (the client predicted them
// anyway). Test mode applies every queued command (harness runs faster than
// real time). Callers hold s.mu.
func (s *Server) simTick(now time.Time) {
	s.tick++
	elapsed := 1.0
	if !s.lastTickAt.IsZero() {
		elapsed = math.Max(0, math.Min(cmdBurst, now.Sub(s.lastTickAt).Seconds()*SimHz))
	}
	s.lastTickAt = now
	s.now = now
	w := &sim.World{Boxes: s.Map.Boxes}
	for _, p := range s.ordered() {
		if p.body == nil || p.Bot {
			continue
		}
		if p.Dead {
			p.cmds = p.cmds[:0]
			if s.timedRespawn() && now.After(p.respawnAt) {
				s.respawn(p)
			}
			continue
		}
		p.cmdBudget = math.Min(cmdBank, p.cmdBudget+elapsed*cmdDrift)
		for len(p.cmds) > 0 && !p.Dead && ((s.Test && !p.paced) || p.cmdBudget >= 1) {
			c := p.cmds[0]
			p.cmds = p.cmds[1:]
			p.ackSeq = c.Seq
			p.cmdBudget--
			s.runCmd(w, p, c)
		}
	}
}

// runCmd applies one command to p: movement, then fire. Callers hold s.mu.
func (s *Server) runCmd(w *sim.World, p *Player, c Cmd) {
	p.cmdTicks++
	s.noteInput(p, c)
	if s.frozen() { // freeze time: look around, but no moving, jumping, shooting or planting
		c.F, c.R = 0, 0
		c.B &^= BtnJump | BtnFire | BtnUse
	}
	// Jump on the button's down edge only, and never while crouched on the
	// ground (as web/engine/cmd.js moveInput); the air crouch-jump is untouched.
	crouch := c.B&BtnCrouch != 0
	jumpEdge := c.B&BtnJump != 0 && p.lastCmd.B&BtnJump == 0 &&
		!(p.body.OnGround && (crouch || p.body.Crouched))
	p.body.Yaw, p.body.Pitch = c.Yaw, c.Pitch
	w.Step(p.body, sim.Input{F: c.F, R: c.R, Walk: c.B&BtnWalk != 0, Jump: jumpEdge, Crouch: crouch, Speed: s.heldSpeed(p)}, 1.0/SimHz)
	if p.body.Pos[1] < -20 { // fell out of the world
		p.body = sim.NewBody(sim.Vec3(s.spawnPoint(p)))
	}
	p.Pos, p.Yaw, p.Pitch, p.Crouch = Vec3(p.body.Pos), c.Yaw, c.Pitch, p.body.Crouched
	// Weapon state changes in the order arsenal.js a.tick applies them.
	if p.inv != nil && c.W != "" && s.Weapons.switchTo(p.inv, c.W, p.cmdTicks) {
		s.sendInv(p)
	}
	if p.inv != nil && c.B&BtnReload != 0 {
		s.Weapons.reload(p.inv, p.cmdTicks)
	}
	if p.inv != nil {
		s.Weapons.settle(p.inv, p.cmdTicks) // as arsenal.js: every tick, before the trigger
	}
	if p.inv != nil {
		firing := c.B&BtnFire != 0
		wpn, why := s.Weapons.trigger(p.inv, p.cmdTicks, firing)
		res := why
		if wpn != nil && wpn.Action == "throw" {
			s.throwNade(p, wpn, c)
			res = "thrown"
		} else if wpn != nil {
			res = s.serverShot(p, wpn, c)
		}
		// Tell the shooter what became of each shot (cl_netdebug): every fired
		// one, and a rejected trigger press (not every held tick of cooldown).
		if wpn != nil || (firing && p.lastCmd.B&BtnFire == 0) {
			b, _ := json.Marshal(map[string]any{"t": "fired", "s": c.Seq, "r": res})
			trySend(p, b)
		}
	}
	s.roundUse(p, c)
	p.lastCmd = c
}

// SpreadSeed is shared with the client (web/game/arsenal.js) so predicted and
// authoritative shots go the same way.
func SpreadSeed(playerID, seq int) uint32 { return uint32(playerID)*2654435761 ^ uint32(seq) }

func (s *Server) spread(p *Player, w *Weapon, c Cmd) float64 {
	if s.cvars["weapon_spread"] == "0" {
		return 0
	}
	sp := w.Spread
	b := p.body
	r := sp.Base + movePenalty(math.Hypot(b.Vel[0], b.Vel[2]), w)*sp.Move
	if !b.OnGround {
		r += sp.Air
	}
	if sp.Unscoped > 0 && c.B&BtnScope == 0 {
		r += sp.Unscoped
	}
	if h := p.inv.Slots[p.inv.Current]; sp.Belt > 0 && w.Mag > 0 && h != nil {
		r += sp.Belt * (1 - float64(h.Ammo)/float64(w.Mag)) // after this shot's round left the belt
	}
	if b.Crouched {
		r *= sp.Crouch
	}
	return r
}

// recoilScale is weapon_recoil_scale (0 with weapon_spread 0: exact shots).
func (s *Server) recoilScale() float64 {
	if s.cvars["weapon_spread"] == "0" {
		return 0
	}
	v, err := strconv.ParseFloat(s.cvars["weapon_recoil_scale"], 64)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 1
	}
	return v
}

// serverShot fires one shot from p's eye along its view (with seeded spread)
// and applies damage to the first player or bot hit before a wall.
// It returns the result for the shooter's debug readout: "miss" or "<part> <dmg>".
func (s *Server) serverShot(p *Player, w *Weapon, c Cmd) string {
	b, _ := json.Marshal(map[string]any{"t": "shot", "id": p.ID, "w": w.ID})
	for _, o := range s.ordered() {
		if o != p {
			trySend(o, b)
		}
	}
	rewound := s.posesAt(c.VT) // other players where the shooter saw them
	o := Vec3{p.body.Pos[0], p.body.Pos[1] + p.body.EyeH + p.body.EyeOff, p.body.Pos[2]}
	var hits []string
	yaw, pitch := recoilAngles(c.Yaw, c.Pitch, RecoilOffset(w.Recoil.Pattern, p.inv.shotIdx), s.recoilScale())
	for i, a := range ShotAngles(yaw, pitch, s.spread(p, w, c), SpreadSeed(p.ID, c.Seq), w.Pellets) {
		if res := s.ray(p, w, c, o, a, rewound, i == 0); res != "miss" {
			hits = append(hits, res)
		}
	}
	if len(hits) == 0 {
		return "miss"
	}
	return strings.Join(hits, ", ")
}

// ShotAngles is the [yaw, pitch] of every ray of one shot: one, or `pellets`
// for a shotgun, each offset by its own pair of draws from the shot's seeded
// RNG (= arsenal.js shotAngles). spread is the full cone width in radians.
func ShotAngles(yaw, pitch, spread float64, seed uint32, pellets int) [][2]float64 {
	r := rng.New(seed)
	out := make([][2]float64, max(1, pellets))
	for i := range out {
		dy := (r.Float() - 0.5) * spread
		dp := (r.Float() - 0.5) * spread
		out[i] = [2]float64{yaw + dy, pitch + dp}
	}
	return out
}

// ray traces one ray of a shot from o and applies its damage to the first
// player or bot hit before a wall. Returns "miss" or "<name> <part> <dmg>".
func (s *Server) ray(p *Player, w *Weapon, c Cmd, o Vec3, a [2]float64, rewound map[int]PoseState, impacts bool) string {
	cp := math.Cos(a[1])
	d := Vec3{-math.Sin(a[0]) * cp, math.Sin(a[1]), -math.Cos(a[0]) * cp}
	best := math.Min(mapDistance(&s.Map, o, d), w.Range)
	var target *Player
	var part Hitbox
	if impacts && s.cvars["sv_showimpacts"] == "1" {
		defer func() { s.sendImpacts(p, c, o, d, best, rewound) }()
	}
	for _, q := range s.ordered() {
		if q == p || q.Dead || !q.ready || q.Team == content.Spectator {
			continue
		}
		if !s.friendlyFire() && q.Team != "" && q.Team == p.Team {
			continue // mp_friendlyfire 0: shots pass through teammates
		}
		pose := poseOf(q)
		if rp, ok := rewound[q.ID]; ok {
			pose = rp
		}
		if h, t, ok := RayModel(&PlayerModel, pose, o, d); ok && t < best {
			target, part, best = q, h, t
		}
	}
	if target == nil {
		return "miss"
	}
	dmg := math.Round(w.DamageAt(part.Mult, best)) // best: metres along the unit ray
	armorBefore := target.Armor
	if !target.Bot {
		dmg = math.Round(absorb(target, w, part.Part, dmg))
	}
	at := Vec3{o[0] + d[0]*best, o[1] + d[1]*best, o[2] + d[2]*best}
	hm, _ := json.Marshal(hitMsg{T: "hit", ID: target.ID, Name: target.Name, Part: part.Part, Weapon: w.ID, Damage: dmg, At: at})
	trySend(p, hm)
	res := fmt.Sprintf("%s %s %.0f", target.Name, part.Part, dmg)
	if target.Bot {
		return res // target practice: never dies
	}
	s.noteDamage(p, target, int(dmg), target.HP)
	target.HP -= int(dmg)
	// Optional (HP is in the snapshot): where it came from, for the damage
	// direction arc, and whether the armor took part of it (the clank).
	hurt, _ := json.Marshal(map[string]any{"t": "hurt", "dmg": dmg, "from": p.ID, "hp": max(target.HP, 0), "fp": o, "ar": target.Armor < armorBefore})
	trySend(target, hurt)
	if target.HP <= 0 {
		s.kill(p, target, w, part.Part == "head", w.Spread.Unscoped > 0 && c.B&BtnScope == 0)
	}
	return res
}

func (s *Server) kill(killer, victim *Player, w *Weapon, headshot bool, noscope ...bool) {
	s.killBy(killer, victim, w.ID, headshot, noscope...)
}

// killBy kills victim (weapon id, e.g. "bigdiff" for the explosion): the
// Big Diff drops, the kill feed hears about it. Callers hold s.mu.
// fallYaw: the way a corpse falls, as a yaw (its forward is the direction):
// away from the killer, or backwards when there is none (the world, a suicide).
func fallYaw(killer, victim *Player) float64 {
	if killer != nil && killer != victim {
		dx, dz := victim.Pos[0]-killer.Pos[0], victim.Pos[2]-killer.Pos[2]
		if dx != 0 || dz != 0 {
			return math.Atan2(-dx, -dz)
		}
	}
	return victim.Yaw + math.Pi
}

func (s *Server) killBy(killer, victim *Player, weapon string, headshot bool, noscope ...bool) {
	s.noteKill(killer, victim)
	victim.HP, victim.Dead = 0, true
	victim.DeathTick, victim.FallYaw = s.tick, fallYaw(killer, victim)
	victim.Vanish = s.round.Phase == PhaseWarmup || s.deathmatch()
	s.dropBomb(victim)
	victim.Deaths++
	victim.respawnAt = s.clock().Add(s.respawnDelay())
	kid := 0 // killer nil: the world (the Big Diff's blast), feed killer id 0
	if killer != nil {
		kid = killer.ID
	}
	switch {
	case killer == nil || killer == victim: // the world, or suicide (own HE): no kill
	case killer.Team != "" && killer.Team == victim.Team:
		killer.Kills-- // CS: a team kill costs a frag
	default:
		killer.Kills++
	}
	scoreKillPoints(killer, victim)
	s.scoreAssists(killer, victim)
	s.moneyOnKill(killer, victim, weapon)
	b, _ := json.Marshal(killMsg{T: "kill", Killer: kid, Victim: victim.ID, Weapon: weapon, Headshot: headshot, NoScope: len(noscope) > 0 && noscope[0]})
	for _, o := range s.ordered() {
		trySend(o, b)
	}
}

// respawnDelay is mp_respawntime (seconds, default 3). Callers hold s.mu.
func (s *Server) respawnDelay() time.Duration {
	v, err := strconv.ParseFloat(s.cvars["mp_respawntime"], 64)
	if err != nil || v < 0 {
		v = 3
	}
	return time.Duration(v * float64(time.Second))
}

func (s *Server) spawnPoint(p *Player) Vec3 {
	if len(s.Map.TeamSpawns[p.Team]) > 0 {
		return s.spawnFor(p.Team)
	}
	return Vec3(s.Map.Spawns[(p.ID-1)%len(s.Map.Spawns)])
}

// respawn puts p at one of its team's spawns with full HP and a fresh
// loadout (after a death or a team switch); a spectator gets no body. Only
// command-driven players get a sim body: legacy clients move themselves.
// The spawn message carries the team and spawn seq. Callers hold s.mu.
func (s *Server) respawn(p *Player) {
	cmdDriven := p.body != nil || p.lastQueued > 0
	p.spawnSeq++
	p.lastUpdate = time.Now()
	p.cmds, p.lastCmd = p.cmds[:0], Cmd{}
	p.Dead, p.body, p.DeathTick, p.FallYaw, p.Vanish = false, nil, 0, 0, false
	if p.Team != content.Spectator {
		p.HP = maxHP
		p.Pos = s.spawnPoint(p)
		p.Yaw = math.Atan2(p.Pos[0], p.Pos[2]) // face the middle
		if cmdDriven {
			p.body = sim.NewBody(sim.Vec3(p.Pos))
			p.body.Yaw = p.Yaw
			p.body.OnGround = true // spawns stand on the floor; the client starts its body the same way (main.js "spawn")
		}
		if s.Weapons != nil {
			p.inv = s.Weapons.Loadout("")
			if s.roundsOn() {
				p.inv = s.Weapons.PistolLoadout() // rounds: buy the rest
			} else {
				p.Money = s.econ().MaxMoney // warmup: full money every spawn
			}
			s.sendInv(p)
		}
	}
	b, _ := json.Marshal(map[string]any{"t": "spawn", "p": p.Pos, "yaw": p.Yaw, "hp": p.HP, "team": p.Team, "sp": p.spawnSeq})
	trySend(p, b)
}

// setPos moves a live player (admin setpos) between two of their commands:
// {t:"setpos"} is the body as of the last command run (ack), which the client
// reconciles to (replaying its later commands from there) and older snapshots
// can't undo. A lost one is caught by the next snapshot like any correction.
func (s *Server) setPos(p *Player, pos Vec3, yaw float64) {
	b := p.body
	b.Pos, b.Vel, b.Yaw, b.OnGround = sim.Vec3(pos), sim.Vec3{}, yaw, false
	p.Pos, p.Yaw = pos, yaw
	m, _ := json.Marshal(map[string]any{"t": "setpos", "ack": p.ackSeq, "p": b.Pos, "v": b.Vel, "g": false, "c": b.Crouched, "yaw": yaw})
	trySend(p, m)
}

// teleport (test mode) puts p's body at pos; commands up to seq are dropped
// and acknowledged, so the client's prediction restarts from here too.
func (s *Server) teleport(p *Player, pos Vec3, yaw float64, crouch bool, seq int) {
	if p.body == nil {
		p.body = sim.NewBody(sim.Vec3(pos))
	}
	b := p.body
	b.Pos, b.Vel, b.Yaw, b.OnGround = sim.Vec3(pos), sim.Vec3{}, yaw, false
	b.Crouched, b.H = crouch, sim.H
	if crouch {
		b.H = sim.HCrouch
	}
	p.cmds = p.cmds[:0]
	p.lastQueued = max(p.lastQueued, seq)
	p.ackSeq = seq
	p.lastCmd = Cmd{}
	if crouch {
		p.lastCmd.B = BtnCrouch
	}
	p.Pos, p.Yaw, p.Crouch = pos, yaw, crouch
}
