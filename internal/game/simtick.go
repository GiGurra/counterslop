package game

import (
	"encoding/json"
	"math"
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
	SimHz       = 100 // shared with the client (sent in welcome)
	snapEvery   = 3   // 100/3 ≈ 33 snapshots per second
	maxQueued   = 32  // commands buffered per player; more is a speedhack or a stall
	respawnTime = 3 * time.Second
	maxHP       = 100
)

// Command buttons.
const (
	BtnJump = 1 << iota
	BtnCrouch
	BtnWalk
	BtnFire
	BtnScope
)

// Cmd is one client tick of input.
type Cmd struct {
	Seq   int     `json:"s"`
	F     float64 `json:"f"`
	R     float64 `json:"r"`
	Yaw   float64 `json:"y"`
	Pitch float64 `json:"p"`
	B     int     `json:"b"`
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
		if len(p.cmds) >= maxQueued {
			continue // running ahead of real time: drop
		}
		c.F, c.R = clamp1(c.F), clamp1(c.R)
		c.Pitch = math.Max(-1.6, math.Min(1.6, c.Pitch))
		p.cmds = append(p.cmds, c)
	}
}

func clamp1(v float64) float64 { return math.Max(-1, math.Min(1, v)) }

// simTick advances every command-driven player by one tick. Callers hold s.mu.
func (s *Server) simTick(now time.Time) {
	s.tick++
	w := &sim.World{Boxes: s.Map.Boxes}
	for _, p := range s.players {
		if p.body == nil || p.Bot {
			continue
		}
		if p.Dead {
			p.cmds = p.cmds[:0]
			if now.After(p.respawnAt) {
				s.respawn(p)
			}
			continue
		}
		var c Cmd
		if len(p.cmds) > 0 {
			c = p.cmds[0]
			p.cmds = p.cmds[1:]
			p.ackSeq = c.Seq
		} else {
			// Starved (packet loss, stall): repeat the last intent without edges.
			c = p.lastCmd
			c.B &^= BtnJump | BtnFire
		}
		// Jump on the button's down edge only, and never while crouched on the
		// ground (as web/engine/cmd.js moveInput); the air crouch-jump is untouched.
		crouch := c.B&BtnCrouch != 0
		jumpEdge := c.B&BtnJump != 0 && p.lastCmd.B&BtnJump == 0 &&
			!(p.body.OnGround && (crouch || p.body.Crouched))
		p.body.Yaw, p.body.Pitch = c.Yaw, c.Pitch
		ev := w.Step(p.body, sim.Input{F: c.F, R: c.R, Walk: c.B&BtnWalk != 0, Jump: jumpEdge, Crouch: crouch}, 1.0/SimHz)
		_ = ev
		if p.body.Pos[1] < -20 { // fell out of the world
			p.body = sim.NewBody(sim.Vec3(s.spawnPoint(p)))
		}
		p.Pos, p.Yaw, p.Pitch, p.Crouch = Vec3(p.body.Pos), c.Yaw, c.Pitch, p.body.Crouched
		if c.B&BtnFire != 0 && p.inv != nil {
			if wpn := s.Weapons.fire(p.inv, s.simTime()); wpn != nil {
				s.serverShot(p, wpn, c)
			}
		}
		p.lastCmd = c
	}
}

// simTime is the server's sim clock (weapon timers run on ticks, not wall time).
func (s *Server) simTime() time.Time {
	return time.Unix(0, 0).Add(time.Duration(s.tick) * time.Second / SimHz)
}

// SpreadSeed is shared with the client (web/game/arsenal.js) so predicted and
// authoritative shots go the same way.
func SpreadSeed(playerID, seq int) uint32 { return uint32(playerID)*2654435761 ^ uint32(seq) }

func (s *Server) spread(p *Player, w *Weapon, c Cmd) float64 {
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
func (s *Server) serverShot(p *Player, w *Weapon, c Cmd) {
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
	for _, q := range s.players {
		if q == p || q.Dead || !q.ready || q.Team == content.Spectator {
			continue
		}
		if !s.friendlyFire() && q.Team != "" && q.Team == p.Team {
			continue // mp_friendlyfire 0: shots pass through teammates
		}
		if h, t, ok := hitPlayer(q, o, d); ok && t < best {
			target, part, best = q, h, t
		}
	}
	if target == nil {
		return
	}
	dmg := math.Round(w.Damage * part.Mult)
	at := Vec3{o[0] + d[0]*best, o[1] + d[1]*best, o[2] + d[2]*best}
	hm, _ := json.Marshal(hitMsg{T: "hit", ID: target.ID, Name: target.Name, Part: part.Part, Weapon: w.ID, Damage: dmg, At: at})
	trySend(p, hm)
	if target.Bot {
		return // target practice: never dies
	}
	target.HP -= int(dmg)
	hurt, _ := json.Marshal(map[string]any{"t": "hurt", "dmg": dmg, "from": p.ID, "hp": max(target.HP, 0)})
	trySend(target, hurt)
	if target.HP <= 0 {
		s.kill(p, target, w, part.Part == "head")
	}
}

func (s *Server) kill(killer, victim *Player, w *Weapon, headshot bool) {
	victim.HP, victim.Dead = 0, true
	victim.Deaths++
	victim.respawnAt = time.Now().Add(respawnTime)
	if killer != victim {
		killer.Kills++
	}
	b, _ := json.Marshal(killMsg{T: "kill", Killer: killer.ID, Victim: victim.ID, Weapon: w.ID, Headshot: headshot})
	for _, o := range s.players {
		trySend(o, b)
	}
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
