package game

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
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
		if c.Seq <= p.lastQueued || !finiteF(c.F) || !finiteF(c.R) || !finiteF(c.Yaw) || !finiteF(c.Pitch) {
			continue
		}
		p.lastQueued = c.Seq
		c.F, c.R = clamp1(c.F), clamp1(c.R)
		c.Pitch = math.Max(-1.6, math.Min(1.6, c.Pitch))
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
	for _, p := range s.players {
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
	w.Step(p.body, sim.Input{F: c.F, R: c.R, Walk: c.B&BtnWalk != 0, Jump: jumpEdge, Crouch: crouch}, 1.0/SimHz)
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
	if c.B&BtnFire == 0 && p.inv != nil {
		p.inv.release()
	}
	if c.B&BtnFire != 0 && p.inv != nil {
		wpn, why := s.Weapons.fire(p.inv, p.cmdTicks)
		res := why
		if wpn != nil {
			res = s.serverShot(p, wpn, c)
		}
		// Tell the shooter what became of each shot (cl_netdebug): every fired
		// one, and a rejected trigger press (not every held tick of cooldown).
		if wpn != nil || p.lastCmd.B&BtnFire == 0 {
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
	r := sp.Base + math.Hypot(b.Vel[0], b.Vel[2])*sp.Move
	if !b.OnGround {
		r += sp.Air
	}
	if sp.Unscoped > 0 && c.B&BtnScope == 0 {
		r += sp.Unscoped
	}
	if b.Crouched {
		r *= sp.Crouch
	}
	return r
}

// serverShot fires one shot from p's eye along its view (with seeded spread)
// and applies damage to the first player or bot hit before a wall.
// It returns the result for the shooter's debug readout: "miss" or "<part> <dmg>".
func (s *Server) serverShot(p *Player, w *Weapon, c Cmd) string {
	b, _ := json.Marshal(map[string]any{"t": "shot", "id": p.ID, "w": w.ID})
	for _, o := range s.players {
		if o != p {
			trySend(o, b)
		}
	}
	r := rng.New(SpreadSeed(p.ID, c.Seq))
	spread := s.spread(p, w, c)
	yaw := c.Yaw + (r.Float()-0.5)*spread
	pitch := c.Pitch + (r.Float()-0.5)*spread
	cp := math.Cos(pitch)
	d := Vec3{-math.Sin(yaw) * cp, math.Sin(pitch), -math.Cos(yaw) * cp}
	o := Vec3{p.body.Pos[0], p.body.Pos[1] + p.body.EyeH + p.body.EyeOff, p.body.Pos[2]}

	best := math.Min(mapDistance(&s.Map, o, d), w.Range)
	var target *Player
	var part Hitbox
	rewound := s.posesAt(c.VT) // other players where the shooter saw them
	if s.cvars["sv_showimpacts"] == "1" {
		defer func() { s.sendImpacts(p, c, o, d, best, rewound) }()
	}
	for _, q := range s.players {
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
	dmg := math.Round(w.Damage * part.Mult)
	at := Vec3{o[0] + d[0]*best, o[1] + d[1]*best, o[2] + d[2]*best}
	hm, _ := json.Marshal(hitMsg{T: "hit", ID: target.ID, Name: target.Name, Part: part.Part, Weapon: w.ID, Damage: dmg, At: at})
	trySend(p, hm)
	res := fmt.Sprintf("%s %s %.0f", target.Name, part.Part, dmg)
	if target.Bot {
		return res // target practice: never dies
	}
	target.HP -= int(dmg)
	hurt, _ := json.Marshal(map[string]any{"t": "hurt", "dmg": dmg, "from": p.ID, "hp": max(target.HP, 0)})
	trySend(target, hurt)
	if target.HP <= 0 {
		s.kill(p, target, w, part.Part == "head")
	}
	return res
}

func (s *Server) kill(killer, victim *Player, w *Weapon, headshot bool) {
	s.killBy(killer, victim, w.ID, headshot)
}

// killBy kills victim (weapon id, e.g. "bigdiff" for the explosion): the
// Big Diff drops, the kill feed hears about it. Callers hold s.mu.
func (s *Server) killBy(killer, victim *Player, weapon string, headshot bool) {
	victim.HP, victim.Dead = 0, true
	s.dropBomb(victim)
	victim.Deaths++
	victim.respawnAt = time.Now().Add(s.respawnDelay())
	if killer != victim {
		killer.Kills++
	}
	s.moneyOnKill(killer, victim, weapon)
	b, _ := json.Marshal(killMsg{T: "kill", Killer: killer.ID, Victim: victim.ID, Weapon: weapon, Headshot: headshot})
	for _, o := range s.players {
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
	p.Dead, p.body = false, nil
	if p.Team != content.Spectator {
		p.HP = maxHP
		p.Pos = s.spawnPoint(p)
		p.Yaw = math.Atan2(p.Pos[0], p.Pos[2]) // face the middle
		if cmdDriven {
			p.body = sim.NewBody(sim.Vec3(p.Pos))
			p.body.Yaw = p.Yaw
		}
		if s.Weapons != nil {
			p.inv = s.Weapons.Loadout("")
			s.sendInv(p)
		}
	}
	b, _ := json.Marshal(map[string]any{"t": "spawn", "p": p.Pos, "yaw": p.Yaw, "hp": p.HP, "team": p.Team, "sp": p.spawnSeq})
	trySend(p, b)
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
