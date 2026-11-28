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
	"github.com/gigurra/counterslop/internal/sim"
)

// The authoritative simulation. Clients send input commands (one per client
// sim tick, batched); the server runs one command per player per tick with the
// same movement code as the client (internal/sim, parity-tested against
// physics.js), fires weapons, applies damage, kills and respawns. Snapshots go
// out every snapEvery ticks and tell each client the last command it applied.

const (
	SimHz     = cmdHz // 100, shared with the client (sent in welcome)
	snapEvery = 1     // 100 snapshots per second (CS2: every tick)
	// fullEvery: every fullEvery-th snapshot is full (every player's status:
	// name, team, weapon, ammo, HP, score; items, round, grenades); the ones
	// between carry the players' poses and the client's own full state only
	// (pose snapshots: a third of the bytes). Each stands on its own.
	fullEvery = 3
	maxQueued = 64 // commands buffered per player (0.64 s); beyond that the oldest are dropped
	// Command budget: a player may run commands as fast as real time passes
	// (plus cmdDrift for client clocks running fast). Budget unused while no
	// commands arrive (a frame hitch, a network burst) is banked up to cmdBank,
	// so the late commands run at once instead of becoming a backlog. The bank
	// covers all the queue can hold: with less, a stall's burst left the rest
	// queued (maxQueued-cmdBank commands, 140 ms on every shot and pickup),
	// draining only at cmdDrift-1 a tick (seconds). A speedhack gains at most
	// cmdDrift-1 in speed plus the bank once.
	cmdDrift = 1.03
	cmdBank  = maxQueued // 0.64 s
	cmdBurst = 20        // server tick stalls caught up per tick, at most
	// A backlog that stays (the server's clock runs slower than the client's,
	// as WSL's can, or steps back: budget a burst can't make up) is cut:
	// a queue that hasn't drained for backlogHold ticks drops its oldest
	// commands down to backlogTarget (acknowledged as done, as an overflow
	// is), every tick it is above it, until it drains.
	// Speed stays honest and every action stays within backlogTarget ticks of
	// on time, whatever the clocks do; the client corrects its prediction.
	backlogTarget = 4
	backlogHold   = SimHz // 1 s
	maxHP         = 100
	// A command more than this many seqs past the last one run (more than a
	// client resends) doesn't move its player's command clock further than one
	// tick: the movers stay where real time has them.
	maxSeqGap = maxQueued
	// A command clock this many ticks off the server's is put back in step
	// (away from movers), so every player sees the movers about where the
	// others do (remote riders are drawn on the server's timeline).
	maxClockDrift = 25
)

// rebase starts p's command clock at server tick `tick` for the next command
// (on joining and every spawn; the spawn message carries it too): command seq N runs at tick
// tickBase+N (cmdTime), carried in every self-snapshot (me.tb) so the client's
// prediction and replays run the movers on the same clock.
func (p *Player) rebase(tick int64) { p.tickBase = tick - int64(p.ackSeq) }

// cmdTime is when command seq starts on p's command clock, in seconds.
func (p *Player) cmdTime(seq int) float64 { return CmdTime(p.tickBase, seq) }

// Cmd is one client tick of input.
type Cmd struct {
	Seq   int     `json:"s"`
	F     float64 `json:"f"`
	R     float64 `json:"r"`
	Yaw   float64 `json:"y"`
	Pitch float64 `json:"p"`
	B     int     `json:"b"`
	W     string  `json:"w,omitempty"`  // switch to this slot first
	VT    float64 `json:"vt,omitempty"` // the server tick the client was drawing the others at (shots and collisions are traced against that)
	// The view at the moment of the click (CS2 subtick), on the command that
	// carries the fire press: the shot goes there, not where the view is at
	// the end of the tick (a flick keeps moving after the click). ShotView.
	ShotView  bool    `json:"sv,omitempty"`
	ShotYaw   float64 `json:"sy,omitempty"`
	ShotPitch float64 `json:"sp,omitempty"`
}

// maxShotViewOff: a click-time view further than this from the command's own
// view (radians) is not believed (more than any mouse moves within one tick):
// the shot uses the command's view.
const maxShotViewOff = 1.0

type killMsg struct {
	T        string `json:"t"`
	Killer   int    `json:"k"`
	Victim   int    `json:"v"`
	Weapon   string `json:"w"`
	Headshot bool   `json:"hs,omitempty"`
	NoScope  bool   `json:"ns,omitempty"` // a scoped weapon fired unscoped (kill feed flair)
	Pen      bool   `json:"pn,omitempty"` // the bullet passed through another body first (kill feed flair)
}

// queueCmds accepts a client's command batch (resends of old seqs are ignored).
// Callers hold s.mu.
func (s *Server) queueCmds(p *Player, cmds []Cmd) {
	if p.Team == content.Spectator {
		// No body to move. Still count them as received: one sent just before
		// a switch to spectators is never resent, so nothing should wait for it.
		for _, c := range cmds {
			p.lastQueued = max(p.lastQueued, c.Seq)
		}
		return
	}
	if p.body == nil {
		p.body = sim.NewBody(sim.Vec3(p.Pos))
		p.body.Yaw = p.Yaw
		s.fitBoots(p)
		p.rebase(s.tick)
	}
	for _, c := range cmds {
		// Already run (or acknowledged by a teleport), or not a number: drop.
		if c.Seq <= p.ackSeq || !finiteF(c.F) || !finiteF(c.R) || !finiteF(c.Yaw) || !finiteF(c.Pitch) {
			continue
		}
		c.F, c.R = clamp1(c.F), clamp1(c.R)
		c.Pitch = math.Max(-1.6, math.Min(1.6, c.Pitch))
		if c.ShotView && (!finiteF(c.ShotYaw) || !finiteF(c.ShotPitch) || math.Abs(angleDiff(c.ShotYaw, c.Yaw)) > maxShotViewOff || math.Abs(c.ShotPitch-c.Pitch) > maxShotViewOff) {
			c.ShotView = false
		}
		if c.ShotView {
			c.ShotPitch = math.Max(-1.6, math.Min(1.6, c.ShotPitch))
		}
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
			p.skipped++
		}
	}
}

// trimBacklog: a queue that hasn't drained (to one command) for backlogHold
// ticks drops its oldest commands down to backlogTarget, every tick it is
// above it, until it drains.
func (p *Player) trimBacklog(tick int64) {
	if len(p.cmds) <= 1 { // drained: an honest client's queue, on time
		p.backlogSince = 0
		return
	}
	if p.backlogSince == 0 {
		p.backlogSince = tick
	}
	if len(p.cmds) <= backlogTarget || tick-p.backlogSince < backlogHold {
		return
	}
	n := len(p.cmds) - backlogTarget
	p.ackSeq = p.cmds[n-1].Seq
	p.lastCmd = p.cmds[n-1]
	p.cmds = slices.Delete(p.cmds, 0, n)
	p.cmdsDropped += n // still behind: trimmed again each tick until it is not
	p.skipped += n
}

func clamp1(v float64) float64 { return math.Max(-1, math.Min(1, v)) }

// simTick runs the commands that have arrived for every command-driven
// player, as many as its budget allows (real time since the last tick, so a
// late or skipped server tick catches up and a slightly fast client clock
// never builds a backlog). A player without commands is not simulated: no
// extrapolation, its next commands just run later (the client predicted them
// anyway). Test mode applies every queued command (harness runs faster than
// real time). Callers hold s.mu.
// testTickHook, when set (tests only: the delta wire watch,
// wirewatch_test.go), sees the server's state at the start of every tick.
var testTickHook func(s *Server)

func (s *Server) simTick(now time.Time) {
	if testTickHook != nil {
		testTickHook(s)
	}
	s.tick++
	elapsed := 1.0
	if !s.lastTickAt.IsZero() {
		elapsed = math.Max(0, math.Min(cmdBurst, now.Sub(s.lastTickAt).Seconds()*SimHz))
	}
	s.lastTickAt = now
	s.now = now
	w := s.simWorld()
	for _, p := range s.ordered() {
		if p.body == nil || p.Bot {
			continue
		}
		if p.Dead {
			p.cmds = p.cmds[:0]
			// A racer is back from a wreck through the race (race.go); one
			// who joined during it spawns on the timer and then races.
			if (s.timedRespawn() || s.raceLive() && s.racer(p) == nil) && now.After(p.respawnAt) {
				s.respawn(p)
			}
			continue
		}
		p.cmdBudget = math.Min(cmdBank, p.cmdBudget+elapsed*cmdDrift)
		for len(p.cmds) > 0 && !p.Dead && ((s.Test && !p.paced) || p.cmdBudget >= 1) {
			c := p.cmds[0]
			p.cmds = p.cmds[1:]
			skipped := p.skipped // dropped or trimmed since the last command run, and the seqs that never came
			if gap := c.Seq - p.ackSeq - 1; gap > maxSeqGap {
				// Seqs skipped (a client may not pick its movers' phase): the
				// clock runs on from the last command run.
				p.tickBase -= int64(gap)
			} else {
				skipped += gap
			}
			p.skipped = 0
			if d := s.tick - (p.tickBase + int64(c.Seq)); (d > maxClockDrift || d < -maxClockDrift) && !w.NearMovers(p.body) {
				// The command clock fell behind the server's (no commands while
				// a tab was hidden, a stall): back in step, where no mover is
				// near, so nothing the body touches moves (the client takes the
				// new base from the next snapshot; off movers its replay is the same).
				p.tickBase += d
			}
			s.coast(w, p, c.Seq, skipped)
			p.ackSeq = c.Seq
			if p.Dead { // crushed coasting (crush.go)
				break
			}
			p.cmdBudget--
			race := s.raceLive()
			if race && len(p.racePath) == 0 { // the path the gates are swept along (tickRace): from here
				p.racePath = append(p.racePath, p.Pos)
			}
			s.runCmd(w, p, c)
			if race {
				p.racePath = append(p.racePath, p.Pos)
			}
		}
		if s.Test && !p.paced {
			continue // the harness steps faster than real time: no budget, no backlog
		}
		p.trimBacklog(s.tick)
	}
}

// solidPlayers is mp_solid_players: players collide with each other.
func (s *Server) solidPlayers() bool { return s.cvars["mp_solid_players"] != "0" }

// solid: q's body is in the way of other players (alive and not spectating;
// target-practice bots too, which have no HP). Only players in the snapshot
// count (ready): the client predicts with those (predict.js peersFrom).
func solid(q *Player) bool { return q.ready && !q.Dead && q.Team != content.Spectator }

// peersOf appends the hulls p collides with (internal/sim/peers.go): every
// other solid player where p's client drew it when it sent the command
// (rewound: History.At(vt), interpolated exactly as net.js draws them, as for
// shots: lagcomp.go), or where it stands now without a view tick (AI bots,
// cases, sv_maxunlag 0). The client predicts the command against the same
// drawn poses (predict.js peersAt), so you bump into the model you see. A
// rewound player also has its core (sim.Peer Core) where it is now, so two
// players moving into each other can't merge or pass: centres stay R apart.
// (The client knows "now" only as its newest snapshot: a core block can
// mispredict a little.)
func (s *Server) peersOf(p *Player, out []sim.Peer, vt, ft float64) []sim.Peer {
	if !s.solidPlayers() {
		return out
	}
	var drawn map[int]PoseState
	if p.ai == nil {
		drawn = s.posesAt(vt, ft)
	}
	for _, id := range s.peerIDs(drawn) {
		q := s.players[id]
		if q == nil { // left since vt: only its hull where p's client still drew it
			d := drawn[id]
			dh := sim.H
			if d.Crouch {
				dh = sim.HCrouch
			}
			out = append(out, sim.Peer{Pos: sim.Vec3(d.Pos), H: dh, Lo: id < p.ID, Drawn: true, G: d.Grav})
			continue
		}
		if q == p {
			continue
		}
		h := sim.H
		if q.Crouch {
			h = sim.HCrouch
		}
		if drawn == nil {
			if solid(q) {
				out = append(out, sim.Peer{Pos: s.peerPos(q, ft), H: h, Lo: q.ID < p.ID, G: q.Grav})
			}
			continue
		}
		// The hull where p saw q, if q was a body then (the history keeps
		// only those, as net.js samples do), and q's core where q is now, if
		// it is one now (no walking through someone who is really there but
		// not drawn there yet).
		if d, ok := drawn[q.ID]; ok {
			dh := sim.H
			if d.Crouch {
				dh = sim.HCrouch
			}
			out = append(out, sim.Peer{Pos: sim.Vec3(d.Pos), H: dh, Lo: q.ID < p.ID, Drawn: true, G: d.Grav})
		}
		if solid(q) {
			out = append(out, sim.Peer{Pos: s.peerPos(q, ft), H: h, Lo: q.ID < p.ID, Core: true, G: q.Grav})
		}
	}
	return out
}

// peerIDs: every player now and every one in the rewound poses (who may have
// left since), in ID order (= predict.js peersAt).
func (s *Server) peerIDs(drawn map[int]PoseState) []int {
	ids := make([]int, 0, len(s.players)+len(drawn))
	for id := range s.players {
		ids = append(ids, id)
	}
	for id := range drawn {
		if s.players[id] == nil {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// runCmd applies one command to p: movement, then fire. Callers hold s.mu.
func (s *Server) runCmd(w *sim.World, p *Player, c Cmd) {
	p.cmdTicks++
	s.noteInput(p, c)
	if s.frozen() { // freeze time: look around, but no moving, jumping, shooting or planting
		c.F, c.R = 0, 0
		c.B &^= BtnJump | BtnFire | BtnUse | BtnAlt | BtnHook | BtnSeat | BtnCarFront | BtnCarRear | BtnBoost
		if p.inv != nil {
			HoldBoost(p.inv, p.cmdTicks) // the drink's countdown starts with the round
		}
	}
	in := MoveInput(c, p.lastCmd.B, p.body) // shared with the client (cmd_shared.go)
	in.Speed = s.heldSpeed(p)
	p.body.Yaw, p.body.Pitch = c.Yaw, c.Pitch
	w.T = p.cmdTime(c.Seq)
	w.Peers = s.peersOf(p, w.Peers[:0], c.VT, w.T)
	from := p.body.Pos
	if s.carCmd(w, p, c) { // driving a car (cars.go)
		from = p.body.Pos // no walk cycle from the car's motion
	} else if rode, moved := s.rideCmd(w, p, c, in); rode { // in a boat (boats.go): seated, driving it, or on its deck
		from = sim.Vec3{p.body.Pos[0] - moved[0], p.body.Pos[1] - moved[1], p.body.Pos[2] - moved[2]} // the walk cycle: only the steps on the deck, not the boat's motion
	} else {
		y0 := p.body.Pos[1]
		w.Cars = s.carDiscs(nil) // the cars are solid to a body on foot (sim peers.go)
		ev := w.Step(p.body, in, 1.0/SimHz)
		w.Cars = nil
		s.crush(p, ev.Crushed) // a mover pushed it into what won't give (crush.go)
		s.landOnBoats(p, y0)   // onto a boat's floor: on its deck (boats.go)
	}
	bd := p.body
	p.Gait.Step(bd.Pos[0]-from[0], bd.Pos[1]-from[1], bd.Pos[2]-from[2], bd.OnGround, bd.Crouched, GaitKOf(p.ID).Swing)
	if s.Test {
		s.noteContact(p, w.Peers)
	}
	if s.outOfWorld(p.body.Pos) && !p.body.Noclip { // fell out of the world (an editor may fly anywhere) (down, or up and sideways in a gravity of its own)
		at := s.spawnPoint(p)
		if g, _, ok := s.raceSpawn(p); ok {
			at = g // a parkour course: back on the last gate passed, at once
		}
		p.body = sim.NewBody(sim.Vec3(at))
		s.fitBoots(p) // the gear stays on
	}
	p.Pos, p.Yaw, p.Pitch, p.Crouch, p.Grav = Vec3(p.body.Pos), c.Yaw, c.Pitch, p.body.Crouched, p.body.Grav
	s.frameOf(p) // riding a mover: the others' commands bump into it in its frame (peerPos)
	p.Hook = nil
	if p.body.Hook.On {
		at := Vec3(p.body.Hook.At)
		p.Hook = &at // the rope, drawn by everyone
	}
	// E pressed on a floor weapon in sight takes it (defusing comes first):
	// server only, the client adopts the new inventory from the snapshot.
	aimed := -1
	if p.inv != nil && c.B&BtnUse != 0 && p.lastCmd.B&BtnUse == 0 {
		aimed = s.aimedItem(p)
		if aimed >= 0 && !s.canDefuse(p) {
			s.takeItem(p, s.items[aimed])
		} else if aimed < 0 && !s.canDefuse(p) && !s.inSite(p.Pos) && p.leftBoat != p.cmdTicks && p.leftCar != p.cmdTicks && p.car == 0 && !s.boardCar(p) {
			s.boardBoat(p) // E near a boat with a free seat, or on its deck (boats.go); a free car first (cars.go)
		}
	}
	if c.B&BtnSeat != 0 && p.lastCmd.B&BtnSeat == 0 && p.ride != 0 { // +seat: switch seats, or sit down from the deck (boats.go)
		if b := s.boatByID(p.ride); p.seat == SeatDeck {
			s.boardBoat(p)
		} else if b != nil {
			s.switchSeat(p, b)
		}
	}
	// Weapon state changes in the order arsenal.js a.tick applies them.
	if p.inv != nil && c.W != "" && s.Weapons.switchTo(p.inv, c.W, p.cmdTicks) {
		s.sendInv(p)
	}
	if p.inv != nil && s.Weapons.UseDraw(p.inv, c.B, p.lastCmd.B, s.inSite(p.Pos), aimed >= 0, p.cmdTicks) {
		s.sendInv(p) // E in a site drew The Big Diff, or letting go put it back
	}
	if p.inv != nil && c.B&BtnReload != 0 {
		s.Weapons.reload(p.inv, p.cmdTicks)
	}
	if p.inv != nil {
		s.Weapons.settle(p.inv, p.cmdTicks) // as arsenal.js: every tick, before the trigger
	}
	if p.inv != nil {
		firing := c.B&BtnFire != 0
		wpn, why := s.Weapons.trigger(p.inv, p.cmdTicks, firing, c.B&BtnAlt != 0, p.body.Crouched)
		res := why
		if wpn != nil && wpn.Action == "throw" {
			s.throwNade(p, wpn, ThrowStrength(p.inv.thrown), c)
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
	if z := s.pitAt(p.Pos); z != nil && !p.Dead && !p.body.Noclip {
		if s.onFoot() && s.raceLive() && s.racer(p) != nil {
			s.respawn(p) // a parkour course: back on the last gate at once, facing along the course (raceSpawn)
			return
		}
		s.killBy(nil, p, z.Name, false) // fell in (he_merge_hell: merged into main); the feed names the pit
	}
}

// pitAt is the map's deadly volume (Map.Pits) the feet at pos are in, or nil.
// Callers hold s.mu.
func (s *Server) pitAt(pos Vec3) *content.Zone {
	for i := range s.Map.Pits {
		if s.Map.Pits[i].Contains(pos) {
			return &s.Map.Pits[i]
		}
	}
	return nil
}

func (s *Server) spread(p *Player, w *Weapon, c Cmd) float64 {
	if s.cvars["weapon_spread"] == "0" {
		return 0
	}
	return ShotSpread(w, p.inv.Slots[p.inv.Current], p.body, c.B&BtnScope != 0)
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
	rewound := s.posesAt(c.VT, p.cmdTime(c.Seq)) // other players where the shooter saw them (riders: where it drew their movers)
	o := Vec3(sim.EyePos(p.body))
	var hits []string
	var walls shotMarks // where rays went into the map and out of a wall (not in a player): bullet holes, impact sounds
	defer func() { s.relayShot(p, w, o, walls) }()
	vy, vp := ShotView(c) // where the view was at the click (subtick)
	yaw, pitch := recoilAngles(vy, vp, RecoilOffset(w.Recoil.Pattern, p.inv.shotIdx), s.recoilScale())
	for i, a := range ShotAngles(yaw, pitch, s.spread(p, w, c), SpreadSeed(p.ID, c.Seq), w.Pellets) {
		if res := s.ray(p, w, c, o, a, rewound, i == 0, &walls); res != "miss" {
			hits = append(hits, res)
		}
	}
	if len(hits) == 0 {
		return "miss"
	}
	return strings.Join(hits, ", ")
}

// shotMarks: where a shot's rays went into the map (h: bullet holes, impact
// sounds) and came out of a wall they went through (x: exit holes).
type shotMarks struct{ h, x [][3]float64 }

// relayShot tells everyone else about p's shot (the pew, the flash) and,
// optionally, its eye "o", wall impacts "h" and wallbang exits "x" (bullet
// holes, impact sounds). Purely cosmetic: losing it loses a sound and a decal.
func (s *Server) relayShot(p *Player, w *Weapon, o Vec3, walls shotMarks) {
	m := map[string]any{"t": "shot", "id": p.ID, "w": w.ID}
	if len(walls.h) > 0 || len(walls.x) > 0 {
		m["o"] = [3]float64{mm(o[0]), mm(o[1]), mm(o[2])}
		m["h"] = walls.h
	}
	if len(walls.x) > 0 {
		m["x"] = walls.x
	}
	b, _ := json.Marshal(m)
	for _, q := range s.ordered() {
		if q != p {
			trySend(q, b)
		}
	}
	s.eventOut(b)
}

func mm(v float64) float64 { return math.Round(v*1000) / 1000 }

// wallPower is w's wallbang power (WallTrace): its wallPen, 0 under mp_wallbang 0.
func (s *Server) wallPower(w *Weapon) float64 {
	if s.cvars["mp_wallbang"] == "0" {
		return 0
	}
	return w.WallPen
}

// ray traces one ray of a shot from o through the walls it can get through
// (wallbang_shared.go) and every player or bot it crosses before the one
// that stops it (penetration.go: each takes what is left of the damage, the
// rest goes on x the weapon's penetration). Returns "miss" or "<name> <part>
// <dmg>" per body damaged, comma separated.
func (s *Server) ray(p *Player, w *Weapon, c Cmd, o Vec3, a [2]float64, rewound map[int]PoseState, impacts bool, walls *shotMarks) string {
	cp := math.Cos(a[1])
	d := Vec3{-math.Sin(a[0]) * cp, math.Sin(a[1]), -math.Cos(a[0]) * cp}
	if g := p.body.Grav; g != sim.GravDown { // the angles are in the shooter's frame
		d = Vec3(sim.FromFrame(g, sim.Vec3(d)))
	}
	lo, hi := MapBounds(s.Map.Boxes)
	passes := WallTrace(s.Map.Boxes, lo, hi, o, d, w.Range, s.wallPower(w))
	wall := WallStop(passes, w.Range)
	// A car's body stops the bullet (its driver's head and shoulders stick
	// out of it: hit on the model as anyone's): the car takes a share of it.
	hitCar, carT := s.bulletCar(p, o, d, wall)
	if hitCar != nil {
		wall = carT
		defer s.damageCar(hitCar, w.Damage*carBulletK, p, w.ID)
	}
	var bodies []bodyHit
	for _, q := range s.ordered() {
		if q == p || q.Dead || !q.ready || q.Team == content.Spectator {
			continue
		}
		pose := poseOf(q, s.poseTime())
		if rp, ok := rewound[q.ID]; ok {
			pose = rp
		}
		if h, t, ok := RayModel(&PlayerModel, pose, o, d); ok && t < wall {
			bodies = append(bodies, bodyHit{q: q, part: h, t: t})
		}
	}
	steps, stopped := penetrateWalls(w, bodies, passes, func(q *Player) bool {
		return s.spared(p, q) // mp_friendlyfire 0 (or a race without combat): no damage, but the body still slows the bullet
	})
	if impacts && s.cvars["sv_showimpacts"] == "1" {
		first := wall
		for _, st := range steps {
			if !st.spared {
				first = st.t
				break
			}
		}
		defer func() { s.sendImpacts(p, c, o, d, first, rewound) }()
	}
	if walls != nil {
		end := w.Range // how far the bullet got: the body that stopped it, else its range
		if stopped && len(steps) > 0 {
			end = steps[len(steps)-1].t
		}
		at := func(t float64) [3]float64 { return [3]float64{mm(o[0] + d[0]*t), mm(o[1] + d[1]*t), mm(o[2] + d[2]*t)} }
		for _, wp := range passes {
			if wp.In >= end {
				break
			}
			walls.h = append(walls.h, at(wp.In))
			if wp.Keep > 0 && wp.Out < end {
				walls.x = append(walls.x, at(wp.Out))
			}
		}
	}
	var res []string
	for _, st := range steps {
		if st.spared {
			continue
		}
		if r := s.applyHit(p, st, w, c, o, d); r != "" {
			res = append(res, r)
		}
	}
	if len(res) == 0 {
		return "miss"
	}
	return strings.Join(res, ", ")
}

// applyHit deals one body's share of a ray: armor, the hit marker, HP, the
// kill. Returns "<name> <part> <dmg>".
func (s *Server) applyHit(p *Player, st penStep, w *Weapon, c Cmd, o, d Vec3) string {
	target, part, dmg := st.q, st.part, st.dmg
	armorBefore := target.Armor
	if !target.Bot {
		dmg = math.Round(absorb(target, w, part.Part, dmg))
	}
	at := Vec3{o[0] + d[0]*st.t, o[1] + d[1]*st.t, o[2] + d[2]*st.t}
	hm, _ := json.Marshal(hitMsg{T: "hit", ID: target.ID, Name: target.Name, Part: part.Part, Weapon: w.ID, Damage: dmg, At: at, Armor: target.Armor < armorBefore})
	trySend(p, hm)
	s.tvHit(p, hm)
	res := fmt.Sprintf("%s %s %.0f", target.Name, part.Part, dmg)
	if target.Bot || target.Dead {
		return res // target practice: never dies (and a body already down takes nothing more)
	}
	s.noteDamage(p, target, int(dmg), target.HP)
	target.HP -= int(dmg)
	// Optional (HP is in the snapshot): where it came from, for the damage
	// direction arc, and whether the armor took part of it (the clank).
	hurt, _ := json.Marshal(map[string]any{"t": "hurt", "dmg": dmg, "from": p.ID, "hp": max(target.HP, 0), "fp": o, "ar": target.Armor < armorBefore})
	trySend(target, hurt)
	if target.HP <= 0 {
		s.killFlair(p, target, w.ID, part.Part == "head", w.Spread.Unscoped > 0 && c.B&BtnScope == 0, st.through > 0)
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
	s.killFlair(killer, victim, weapon, headshot, len(noscope) > 0 && noscope[0], false)
}

// killFlair is killBy with the kill feed's flair: noscope, and penetrated
// (the bullet went through another body first).
func (s *Server) killFlair(killer, victim *Player, weapon string, headshot, noscope, penetrated bool) {
	s.noteKill(killer, victim)
	victim.HP, victim.Dead = 0, true
	victim.ride, victim.seat, victim.car = 0, 0, 0 // the body falls out of the boat (the car)
	victim.DeathTick, victim.FallYaw = s.tick, fallYaw(killer, victim)
	victim.Vanish = s.round.Phase == PhaseWarmup || s.deathmatch()
	s.dropBomb(victim)
	s.deathDrop(victim)
	victim.Deaths++
	s.ticketDeath(victim) // tickets mode: one off the team's backlog
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
	b, _ := json.Marshal(killMsg{T: "kill", Killer: kid, Victim: victim.ID, Weapon: weapon, Headshot: headshot, NoScope: noscope, Pen: penetrated})
	for _, o := range s.ordered() {
		trySend(o, b)
	}
	s.eventOut(b)
}

// respawnDelay is mp_respawntime (seconds, default 3). Callers hold s.mu.
func (s *Server) respawnDelay() time.Duration {
	name, def := "mp_respawntime", 3.0
	if s.ticketsLive() { // tickets mode: mp_respawn_delay
		name, def = "mp_respawn_delay", defaultRespawnSecs
	}
	v, err := strconv.ParseFloat(s.cvar(name), 64)
	if err != nil || v < 0 {
		v = def
	}
	return time.Duration(v * float64(time.Second))
}

func (s *Server) spawnPoint(p *Player) Vec3 {
	if s.ticketsLive() && teamIdx(p.Team) >= 0 { // tickets: the chosen (or auto) spawn point
		return s.ticketSpawnPos(p)
	}
	if len(s.Map.TeamSpawns[p.Team]) > 0 {
		return s.spawnFor(p)
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
	p.lastUpdate = s.clock()
	p.cmds, p.lastCmd = p.cmds[:0], Cmd{}
	p.Dead, p.body, p.DeathTick, p.FallYaw, p.Vanish = false, nil, 0, 0, false
	p.Gait, p.Grav = Gait{}, sim.GravDown // a new body stands in the world's gravity
	p.ride, p.seat, p.car = 0, 0, 0       // and on its own feet
	s.raceJumped(p)                       // a racer: no gate passed on the way here
	if p.pit {                            // out of any pit stop (pitstop.go): a team switch, a new round
		s.unparkOf(p)
	}
	p.Boots = false // gear dies with you (a new round's survivors get theirs back: restoreCarried)
	if p.ai != nil {
		p.ai.path, p.ai.goalKey = nil, "" // a bot routes again from the spawn (the old route's next waypoint may be over a pit)
	}
	if p.Team != content.Spectator {
		p.HP = maxHP
		p.Pos = s.spawnPoint(p)
		p.Yaw = math.Atan2(p.Pos[0], p.Pos[2]) // face the middle
		if at, yaw, ok := s.raceSpawn(p); ok { // on a parkour course: back on the last gate passed
			p.Pos, p.Yaw = at, yaw
		}
		if cmdDriven {
			p.body = sim.NewBody(sim.Vec3(p.Pos))
			p.body.Yaw = p.Yaw
			p.body.OnGround = true // spawns stand on the floor; the client starts its body the same way (main.js "spawn")
			s.fitBoots(p)
			p.rebase(s.tick)
		}
		if s.Weapons != nil && s.nadesOnly() {
			p.inv = s.Weapons.NadeLoadout(p.Team) // mp_loadout nades: the keyboard and one of every grenade
			p.nadeGone = nil
			s.sendInv(p)
		} else if s.Weapons != nil {
			p.inv = s.Weapons.Loadout("")
			if s.ticketsLive() {
				p.inv = s.kitLoadout(p) // tickets: the chosen kit, no buying
			} else if s.roundsOn() {
				p.inv = s.Weapons.PistolLoadout() // rounds: buy the rest
			} else {
				p.Money = s.econ().MaxMoney // warmup: full money every spawn
			}
			s.sendInv(p)
		}
	}
	b, _ := json.Marshal(map[string]any{"t": "spawn", "p": p.Pos, "yaw": p.Yaw, "hp": p.HP, "team": p.Team, "sp": p.spawnSeq, "tb": p.tickBase})
	trySend(p, b)
}

// setPos moves a live player (admin setpos) between two of their commands:
// {t:"setpos"} is the body as of the last command run (ack), which the client
// reconciles to (replaying its later commands from there) and older snapshots
// can't undo. A lost one is caught by the next snapshot like any correction.
func (s *Server) setPos(p *Player, pos Vec3, yaw float64) {
	b := p.body
	b.Pos, b.Vel, b.Yaw, b.OnGround = sim.Vec3(pos), sim.Vec3{}, yaw, false
	p.Gait = Gait{}
	p.ride, p.seat, p.car = 0, 0, 0 // off any boat or car
	p.Pos, p.Yaw = pos, yaw
	s.raceJumped(p)
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
	p.Gait = Gait{}
	p.ride, p.seat, p.car = 0, 0, 0 // off any boat or car
	s.raceJumped(p)
	b.Crouched, b.H = crouch, sim.H
	if crouch {
		b.H = sim.HCrouch
	}
	p.cmds = p.cmds[:0]
	p.lastQueued = max(p.lastQueued, seq)
	p.ackSeq = seq // the command clock runs on: the client knows its tick base already
	p.skipped = 0
	p.lastCmd = Cmd{}
	if crouch {
		p.lastCmd.B = BtnCrouch
	}
	p.Pos, p.Yaw, p.Crouch = pos, yaw, crouch
}

// coast steps p's body, on foot near a mover, through the n commands
// before seq it never ran (lost past the resend window, dropped from an
// overflowing queue, trimmed from a backlog): no input, just time passing, so
// a ship carries its riders across them. Skipping them jumped the command
// clock (each command runs at its own time on the movers' clock) and a
// cabin climbing at 60-80 m/s left its rider metres below the floor:
// MVP-1 threw players out on a prod server (TestRideRealtime). Off movers
// nothing moves in the gap: skipped as ever (the client corrects itself).
// Callers hold s.mu.
func (s *Server) coast(w *sim.World, p *Player, seq, n int) {
	if n <= 0 || p.body == nil || p.car != 0 || p.ride != 0 || !w.NearMovers(p.body) {
		return
	}
	n = min(n, maxQueued)
	idle := Cmd{Yaw: p.lastCmd.Yaw, Pitch: p.lastCmd.Pitch, B: p.lastCmd.B & BtnCrouch} // crouched stays crouched
	in := MoveInput(idle, p.lastCmd.B, p.body)
	in.Speed = s.heldSpeed(p)
	for i := n; i >= 1; i-- {
		w.T = p.cmdTime(seq - i)
		w.Peers = s.peersOf(p, w.Peers[:0], 0, w.T)
		s.crush(p, w.Step(p.body, in, 1.0/SimHz).Crushed)
		if p.Dead {
			return
		}
		in = MoveInput(idle, idle.B, p.body)
		in.Speed = s.heldSpeed(p)
	}
	p.lastCmd.B = idle.B
}

// simWorld is a fresh movement world for this tick. Building one filters the
// map's boxes (the treads its movers replace), so the filtered boxes are kept
// for as long as the map is the same (its Boxes and Movers backing arrays):
// rebuilding them every tick was a fifth of a megabyte AI match's CPU.
func (s *Server) simWorld() *sim.World {
	c := &s.worldCache
	if c.w == nil || !sameSlice(c.boxes, s.Map.Boxes) || !sameSlice(c.movers, s.Map.Movers) {
		c.w, c.boxes, c.movers = sim.NewWorld(&s.Map), s.Map.Boxes, s.Map.Movers
	}
	w := *c.w // the per-step fields (T, Peers, ...) are the step's own
	w.Move = s.moveRules()
	return &w
}

// sameSlice: a and b are the same slice (backing array and length).
func sameSlice[T any](a, b []T) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// outOfWorldMargin: how far past the map's boxes a body may get before it is
// out of the world (and below y -20 always is, as ever).
const outOfWorldMargin = 30

// outOfWorld: pos has left the map: fallen below it, or (a body in a gravity
// of its own: the Gravity Override) up past its top or off its sides.
// Callers hold s.mu.
func (s *Server) outOfWorld(pos sim.Vec3) bool {
	if pos[1] < content.OutOfWorldY {
		return true
	}
	if len(s.Map.Boxes) == 0 {
		return false
	}
	if key := &s.Map.Boxes[0]; s.bounds.of != key || s.bounds.n != len(s.Map.Boxes) || s.bounds.mv != len(s.Map.Movers) { // the map changed
		s.bounds.lo, s.bounds.hi = MapBounds(s.Map.Boxes)
		for i := range s.Map.Movers { // a ship's flight is part of the world (its riders are in it)
			lo, hi := sim.MoverSweep(&s.Map.Movers[i])
			for a := range 3 {
				s.bounds.lo[a], s.bounds.hi[a] = math.Min(s.bounds.lo[a], lo[a]), math.Max(s.bounds.hi[a], hi[a])
			}
		}
		s.bounds.of, s.bounds.n, s.bounds.mv = key, len(s.Map.Boxes), len(s.Map.Movers)
	}
	for i := range 3 {
		if pos[i] < s.bounds.lo[i]-outOfWorldMargin || pos[i] > s.bounds.hi[i]+outOfWorldMargin {
			return true
		}
	}
	return false
}
