package game

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/rng"
	"github.com/gigurra/counterslop/internal/sim"
)

// Playing bots ("AI players"): real players to the server, not the
// invincible target-practice bots. Each sim tick their brain writes one
// command into their queue, which then goes through exactly the path a
// client's commands take (budget, physics, weapons, rounds, economy). They
// buy at round start, walk the map's nav graph to a site, carry/plant the
// Big Diff (Vibe Coders) or hold/retake/defuse (Artisans), and shoot enemies
// they can see after a reaction time, with aim error by bot_difficulty.
// Everything random comes from a per-bot seeded rng, so a scenario replays.

type aiBrain struct {
	r        *rng.Rand
	seq      int
	path     []Vec3
	goal     Vec3
	goalKey  string
	repathAt int64
	lastPos  Vec3
	stuckAt  int64
	strafe   float64
	site     string // the site this bot plays this round
	round    int    // round number the site and the buy were decided for
	target   int    // enemy being engaged
	seenAt   int64  // tick the target was first seen
	aimErr   [2]float64
	yaw      float64
	pitch    float64
	lastFire bool
	nade     aiNadeState // a grenade throw in progress (ai_nades.go)
	rotated  int         // the round this bot last rotated to the other site (ai_defend.go)
	lane     aiLane      // the route this attacker takes this round (ai_lanes.go)
	roam     Vec3        // warmup on a map without sites: the nav node walked to (aiRoam)
	roamAt   int64       // the tick roam was picked (0: none yet)
	post     aiPost      // GPU Drop staff: the forward post this shift (ai_gpudrop.go)
	leg      [2]Vec3     // the last waypoint reached and the one after it (aimOnLeg)
	way      aiWay       // giving way to players in its path (ai_way.go)
	goTo     *Vec3       // tests: walk here, nothing else
	isle     aiIsle      // the nav islands it and its goal are on (ai_board.go aiAcross)
	race     aiRace      // driving a race (ai_race.go)
	pk       aiParkour   // running a parkour course (ai_parkour.go)
	tk       aiTk        // its point in tickets mode (ai_tickets.go)
}

// difficulty: reaction ticks, aim error (radians), turn speed (radians/tick).
var aiSkill = [4]struct {
	react int64
	err   float64
	turn  float64
}{
	{60, 0.07, 0.06},
	{40, 0.04, 0.09},
	{25, 0.022, 0.14},
	{15, 0.01, 0.22},
}

var aiNames = []string{
	"Copilot-ish", "StackOverflower", "Hand-typed Hank", "Tab Complete", "Senior Prompt Engineer",
	"10x Tim", "Rubber Duck Debbie", "Merge Mona", "LGTM Larry", "Yak Shaver",
	"Off-by-One Olga", "Regex Rick", "Null Pointer Pat", "Semver Sam", "Flaky Fiona",
	"Legacy Lars", "Hotfix Hilda", "Monorepo Mo", "Bikeshed Bob", "Cargo Cult Carl",
}

func (s *Server) aiDifficulty() int {
	return max(0, min(3, int(s.cvarF("bot_difficulty", 1))))
}

// addAI adds a playing bot to team ("" = the smaller side with room); nil if
// that side (or, for "", both) is at the map's cap. Callers hold s.mu.
func (s *Server) addAI(team string) *Player {
	if team != content.Vibe && team != content.Artisan {
		n := s.teamCounts(nil)
		team = content.Vibe // deterministic tie-break (scenarios replay)
		if n[content.Artisan] < n[content.Vibe] {
			team = content.Artisan
		}
		if s.capFull(team, nil) {
			team = otherTeam(team)
		}
	}
	if s.capFull(team, nil) {
		return nil
	}
	s.nextID++
	id := s.nextID
	name := aiNames[(id*7)%len(aiNames)]
	for _, p := range s.ordered() {
		if p.Name == name {
			name += " " + strconv.Itoa(id)
			break
		}
	}
	p := &Player{
		ID: id, Name: name + " [bot]", Color: "#9aa0a6", ready: true,
		lastUpdate: s.clock(), joined: s.clock(),
		HP: maxHP, Money: s.startMoney(), Team: team, AI: true,
		ai:         &aiBrain{r: rng.New(uint32(id)*2654435761 + 12345)},
		lastQueued: 1, // command-driven from the start: respawn gives it a sim body
	}
	p.ai.seq = 1
	s.players[id] = p
	s.respawn(p)
	if s.roundsOn() && !s.frozen() {
		s.lateJoin(p) // mid-round: plays from the next round (tickets: after the respawn delay)
	}
	return p
}

// kickAI removes playing bots: n of them from team ("" = any), or all if n < 0.
func (s *Server) kickAI(team string, n int) int {
	ids := []int{}
	for _, p := range s.ordered() {
		if p.ai != nil && (team == "" || p.Team == team) {
			ids = append(ids, p.ID)
		}
	}
	slices.Sort(ids)
	slices.Reverse(ids) // newest first
	k := 0
	for _, id := range ids {
		if n >= 0 && k >= n {
			break
		}
		s.dropBomb(s.players[id])
		delete(s.players, id)
		k++
	}
	return k
}

// fillQuota: bot_quota N (bot_quota_mode fill): playing bots fill the teams
// up to N players in total; humans replace them as they join.
func (s *Server) fillQuota() {
	quota := int(s.cvarF("bot_quota", 0))
	humans, bots := 0, 0
	for _, p := range s.ordered() {
		switch {
		case p.ai != nil:
			bots++
		case !p.Bot && p.ready && (p.Team == content.Vibe || p.Team == content.Artisan):
			humans++
		}
	}
	want := max(0, quota-humans)
	if quota <= 0 {
		want = bots // no quota: leave hand-added bots alone
	}
	for ; bots < want; bots++ {
		if s.addAI("") == nil {
			break // both sides at the map's cap
		}
	}
	if bots > want {
		// take them from the bigger team
		for ; bots > want; bots-- {
			v, _ := s.fighters(content.Vibe)
			a, _ := s.fighters(content.Artisan)
			team := content.Vibe
			if a > v {
				team = content.Artisan
			}
			if s.kickAI(team, 1) == 0 {
				s.kickAI("", 1)
			}
		}
	}
}

// tickAI queues this tick's command for every playing bot. Runs before
// simTick, so the commands run this tick. Callers hold s.mu.
func (s *Server) tickAI() {
	if s.editing() {
		return // the map is being edited: bots stand still (playtest plays them)
	}
	if s.tick%SimHz == 0 {
		s.fillQuota()
	}
	ids := make([]int, 0, len(s.players))
	for _, p := range s.ordered() {
		if p.ai != nil {
			ids = append(ids, p.ID)
		}
	}
	slices.Sort(ids) // deterministic order
	for _, id := range ids {
		p := s.players[id]
		if p.Dead || p.body == nil || p.Team == content.Spectator {
			continue
		}
		c := s.aiThink(p)
		if !s.onFoot() { // a parkour course's route jumps off edges and swims on purpose
			s.aiEdgeGuard(p, &c)
			s.aiKeepDry(p, &c) // never off a quay into the water, whatever moved it
		}
		p.ai.seq++
		c.Seq = p.ai.seq
		s.queueCmds(p, []Cmd{c})
	}
}

// above: the point h up from p's feet along its up (its gravity, sim.Above).
func above(p *Player, h float64) Vec3 { return Vec3(sim.Above(sim.Vec3(p.Pos), p.Grav, h)) }

// look: the world direction p's view (yaw, pitch, in its gravity's frame) points along.
func look(g int, yaw, pitch float64) Vec3 {
	cp := math.Cos(pitch)
	return Vec3(sim.FromFrame(g, sim.Vec3{-math.Sin(yaw) * cp, math.Sin(pitch), -math.Cos(yaw) * cp}))
}

func (s *Server) eyeOf(p *Player) Vec3 {
	if p.body != nil {
		return Vec3(sim.EyePos(p.body))
	}
	return above(p, 1.6)
}

// seenCrouched: q's chest is in sight from p's eye crouched where it stands.
func (s *Server) seenCrouched(p, q *Player) bool {
	return s.lineClear(above(p, sim.EyeCrouch), above(q, 1.2))
}

// visible: a clear line from p's eye to q's chest within p's field of view.
func (s *Server) visible(p, q *Player, fov float64) (dist float64, ok bool) {
	if p.flashLeft(s.clock()) > 0.4 { // flashed: sees nothing (a Stack Trace works on bots too)
		return 0, false
	}
	o := s.eyeOf(p)
	t := above(q, 1.2)
	if q.Crouch {
		t = above(q, 0.8)
	}
	d := Vec3{t[0] - o[0], t[1] - o[1], t[2] - o[2]}
	dist = math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
	if dist < 1e-6 {
		return 0, true
	}
	yawTo := math.Atan2(-d[0], -d[2])
	if math.Abs(angleDiff(yawTo, p.ai.yaw)) > fov/2 {
		return dist, false
	}
	n := Vec3{d[0] / dist, d[1] / dist, d[2] / dist}
	return dist, mapClear(&s.Map, o, n, dist-0.2)
}

func angleDiff(a, b float64) float64 {
	d := math.Mod(a-b+math.Pi, 2*math.Pi)
	if d < 0 {
		d += 2 * math.Pi
	}
	return d - math.Pi
}

// turnTo moves the bot's view toward (yaw, pitch) by at most step radians.
func (b *aiBrain) turnTo(yaw, pitch, step float64) {
	dy := angleDiff(yaw, b.yaw)
	b.yaw += math.Max(-step, math.Min(step, dy))
	b.pitch += math.Max(-step, math.Min(step, pitch-b.pitch))
}

// aiThink decides one command. Callers hold s.mu.
func (s *Server) aiThink(p *Player) Cmd {
	if s.onFoot() { // a parkour course: its route (ai_parkour.go)
		return s.aiParkourCmd(p)
	}
	if s.raceMode() { // bots race (ai_race.go); one without a car (no grid slot) stands
		return s.aiRace(p)
	}
	b := p.ai
	sk := aiSkill[s.aiDifficulty()]
	r := &s.round
	if b.round != r.Number || b.site == "" {
		b.round, b.site, b.target = r.Number, s.pickSite(p), 0
		b.path, b.goalKey = nil, ""
		b.yaw = p.body.Yaw
		if s.roundsOn() {
			s.aiBuy(p)
		}
	}
	var c Cmd

	// Enemies: keep the current one while visible, else the closest visible.
	var tgt *Player
	best := math.Inf(1)
	for _, q := range s.ordered() {
		if q == p || q.Dead || !q.ready || q.Bot || q.Team == p.Team || (q.Team != content.Vibe && q.Team != content.Artisan) {
			continue
		}
		fov := 2.2
		if q.ID == b.target {
			fov = 2 * math.Pi
		}
		if d, ok := s.visible(p, q, fov); ok && (d < best || q.ID == b.target && d < best+5) {
			tgt, best = q, d
		}
	}
	held := p.inv.Slots[p.inv.Current]
	// Grenades (ai_nades.go): a throw in progress owns the command; an enemy
	// showing up cancels a site throw, not one aimed at that enemy.
	s.aiDodgeFlash(p)
	busy := b.aiNadeBusy(p.cmdTicks)
	if busy && tgt != nil && !b.nade.enemy && b.nade.slot != "" { // not while our own flash is in the air: keep looking away
		s.aiNadeStop(p, &c)
		busy = false
	}
	if !busy && !s.frozen() {
		seen := tgt != nil && tgt.ID == b.target && p.cmdTicks-b.seenAt >= sk.react
		if seen && s.nadesOnly() && best >= aiNadeFightMin && best <= aiEnemyNadeMax {
			// Grenades are all there is (mp_loadout nades): lob whatever is
			// back in the hand at them, HE first; closer, the keyboard.
			busy = s.aiStartNade(p, "he", tgt.Pos, true) || s.aiStartNade(p, "fire", tgt.Pos, true) || s.aiStartNade(p, "flash", tgt.Pos, true)
		} else if seen && p.Team == content.Artisan && best >= aiEnemyNadeMin && best <= aiEnemyNadeMax && !s.facing(tgt, p) {
			busy = s.aiStartNade(p, "he", tgt.Pos, true) || s.aiStartNade(p, "fire", tgt.Pos, true)
		} else if tgt == nil {
			busy = s.aiSiteNades(p)
		}
	}
	if busy && !s.frozen() {
		s.aiNade(p, &c)
		b.lastFire = c.B&BtnFire != 0
		c.Yaw, c.Pitch = b.yaw, math.Max(-1.5, math.Min(1.5, b.pitch))
		return c
	}
	if tgt != nil {
		s.noteIntel(p.Team, tgt.Pos)
	}
	if tgt != nil && !s.frozen() {
		if tgt.ID != b.target {
			b.target, b.seenAt = tgt.ID, p.cmdTicks
			b.aimErr = [2]float64{b.r.Range(-1, 1) * sk.err * 3, b.r.Range(-1, 1) * sk.err * 2}
		}
		age := p.cmdTicks - b.seenAt
		decay := math.Max(0.25, 1-float64(age)/120) // the aim settles with time on target
		o := s.eyeOf(p)
		at := above(tgt, 1.25) // along its up: a player on a wall too
		if tgt.Crouch {
			at = above(tgt, 0.85)
		}
		d := Vec3{at[0] - o[0], at[1] - o[1], at[2] - o[2]}
		yaw := math.Atan2(-d[0], -d[2]) + b.aimErr[0]*decay
		pitch := math.Atan2(d[1], math.Hypot(d[0], d[2])) + b.aimErr[1]*decay
		b.turnTo(yaw, pitch, sk.turn)
		off := math.Hypot(angleDiff(yaw, b.yaw), pitch-b.pitch)
		melee := false
		switch {
		case p.inv.Current == BombSlot: // a fight with The Big Diff in hand: a gun first
			c.W = s.aiBestGun(p)
		case held == nil:
		case p.inv.Current == "melee" || held.Ammo == 0:
			// Empty mag: a loaded gun, else reload, else the keyboard
			// (ai_melee.go). Keyboard in hand: back to a gun with ammo.
			if melee = s.aiArm(p, &c); !melee && c.W == "" {
				c.B |= BtnReload
			}
		case age >= sk.react && off < 0.06+math.Atan2(0.3, best):
			w := s.Weapons.Get(held.ID)
			fire := true
			if w != nil && w.Action != "auto" {
				fire = !b.lastFire // semi/bolt: a fresh press each shot
			}
			if fire {
				c.B |= BtnFire
				c.VT = float64(s.tick)
			}
		}
		if melee {
			s.aiCharge(p, &c, tgt, best)
		} else {
			// Far away: keep pushing to the objective while shooting (CS bots
			// do; two teams standing at their spawns trading shots across the
			// map is no game). Mid range: stop and crouch for accuracy.
			// GPU Drop's scalpers always push for the stock or the exit while
			// shooting: standing to trade shots through the open stock racking
			// (a clear sight line, shots into the shelves) runs the clock out.
			switch {
			case best > aiHoldRange || (s.gpuMode() && p.Team == content.Vibe):
				s.aiMove(p, &c, true)
			case best > 18 && s.seenCrouched(p, tgt):
				c.B |= BtnCrouch // steadier at range (unless crouching hides the target: a railing, a low wall)
			}
			// Stand still to shoot (counter-strafe); strafe a little up close.
			if best < 10 {
				if b.r.Float() < 0.02 {
					b.strafe = -b.strafe
				}
				if b.strafe == 0 {
					b.strafe = 1
				}
				c.R = b.strafe
			}
		}
	} else {
		b.target = 0
		if held != nil && held.Ammo < max(1, s.magOf(held)/3) && held.Reserve > 0 {
			c.B |= BtnReload // reload when safe
		} else if held != nil && (p.inv.Current == "melee" || held.Ammo == 0 && (p.inv.Current == "primary" || p.inv.Current == "sidearm")) {
			s.aiArm(p, &c) // a dry gun or the keyboard in hand: the best weapon left
		}
		s.aiMove(p, &c, false)
	}
	s.aiAvoidFire(p, &c)
	b.lastFire = c.B&BtnFire != 0
	c.Yaw, c.Pitch = b.yaw, math.Max(-1.5, math.Min(1.5, b.pitch))
	if c.B&BtnFire != 0 && c.W == "" {
		s.aiPullDown(p, &c, aiRecoilControl[s.aiDifficulty()])
	}
	return c
}

func (s *Server) magOf(h *Held) int {
	if w := s.Weapons.Get(h.ID); w != nil {
		return w.Mag
	}
	return 0
}

// aiLurk: the chance an attacker (other than the one picking the team's site)
// plays any site instead of the team's.
const aiLurk = 0.25

// pickSite: the site this bot plays this round (A or B), seeded. In a live
// round the attackers all go for one site (the first of them to think picks
// it): split over two sites, they arrive a few at a time and lose both.
func (s *Server) pickSite(p *Player) string {
	names := s.objectiveNames() // the bomb sites, or in GPU Drop the GPUs
	if len(names) == 0 {
		return "-"
	}
	if p.Team == content.Vibe && s.roundsOn() {
		if s.aiAttack.round != s.round.Number || s.aiAttack.site == "" {
			s.aiAttack.round, s.aiAttack.site = s.round.Number, names[p.ai.r.Int(0, len(names)-1)]
		} else if p.ai.r.Float() < aiLurk {
			return names[p.ai.r.Int(0, len(names)-1)] // a lurker: anywhere
		}
		return s.aiAttack.site
	}
	if p.Team == content.Artisan && s.roundsOn() && !s.gpuMode() {
		if post, ok := s.defPlan(p).posts[p.ID]; ok {
			return post.site
		}
	}
	return names[p.ai.r.Int(0, len(names)-1)]
}

// aiRoamTicks: a roaming bot picks a new nav node at the latest after this
// long (the old one may be out of reach).
const aiRoamTicks = 20 * SimHz

// aiRoam: where a warmup bot on a map without bomb sites walks: a seeded
// random nav node, a new one on arrival (or after aiRoamTicks). The teams
// cross the map and run into each other: a deathmatch.
func (s *Server) aiRoam(p *Player) (Vec3, bool) {
	b := p.ai
	nav := s.Map.Nav
	if len(nav) == 0 {
		return Vec3{}, false
	}
	arrived := b.roamAt != 0 && b.goalKey != "" && len(b.path) == 0 && math.Hypot(b.roam[0]-p.Pos[0], b.roam[2]-p.Pos[2]) < 3
	if b.roamAt == 0 || arrived || p.cmdTicks-b.roamAt >= aiRoamTicks || p.cmdTicks < b.roamAt {
		b.roam, b.roamAt = Vec3(nav[b.r.Int(0, len(nav)-1)].Pos), max(p.cmdTicks, 1)
	}
	return b.roam, true
}

func (s *Server) siteZone(name string) *content.Zone {
	for i := range s.Map.Sites {
		if s.Map.Sites[i].Name == name {
			return &s.Map.Sites[i]
		}
	}
	return nil
}

// aiMove: objectives and walking. Vibe Coders take the Big Diff to their
// site and plant it; the rest go to the same site (or the dropped Big
// Diff). Artisans hold their site; once it is planted, they go to it and
// revert it.
// aiHoldRange: closer than this a bot stops to fight; farther it keeps
// walking its route (aiming at the enemy all the while).
const aiHoldRange = 28.0

// steer walks toward moveYaw: turning the view there, or, while aiming at an
// enemy (engaged), strafing/walking relative to the view instead.
func (b *aiBrain) steer(c *Cmd, moveYaw float64, engaged bool) {
	if !engaged {
		b.turnTo(moveYaw, 0, 0.25)
		c.F = 1
		return
	}
	rel := moveYaw - b.yaw
	c.F, c.R = math.Cos(rel), -math.Sin(rel)
}

func (s *Server) aiMove(p *Player, c *Cmd, engaged bool) {
	b := p.ai
	r := &s.round
	bomb := &r.Bomb
	var goal Vec3
	key := ""
	use, plant, defending := false, false, false
	siteCenter := func(name string) (Vec3, bool) {
		z := s.siteZone(name)
		if z == nil {
			return Vec3{}, false
		}
		return Vec3{(z.Min[0] + z.Max[0]) / 2, z.Min[1] + 0.5, (z.Min[2] + z.Max[2]) / 2}, true
	}
	var loot Vec3
	lootKey := ""
	if b.goTo == nil && !engaged {
		loot, lootKey, _ = s.aiLootGoal(p) // a dropped primary close by (ai_loot.go)
	}
	switch {
	case b.goTo != nil:
		goal, key = *b.goTo, fmt.Sprintf("goto:%.1f,%.1f", b.goTo[0], b.goTo[2])
	case lootKey != "":
		goal, key = loot, lootKey
	case !s.roundsOn() || s.frozen():
		// warmup: roam between the sites (or, without sites, the nav
		// graph); freeze: stand (can't move anyway)
		if s.frozen() {
			break
		}
		if g, ok := siteCenter(b.site); ok {
			goal, key = g, "site:"+b.site
		} else if g, ok := s.aiRoam(p); ok {
			goal, key = g, fmt.Sprintf("roam:%.1f,%.1f,%.1f", g[0], g[1], g[2])
		}
	case s.ticketMode(): // tickets: capture and hold points (ai_tickets.go)
		if g, k, ok := s.aiTicketGoal(p); ok {
			goal, key = g, k
		} else if g, ok := s.aiRoam(p); ok {
			goal, key = g, fmt.Sprintf("roam:%.1f,%.1f,%.1f", g[0], g[1], g[2])
		}
	case s.gpuMode(): // GPU Drop (ai_gpudrop.go)
		var hold bool
		goal, key, use, hold = s.aiGPU(p)
		if hold && !engaged && len(b.path) == 0 {
			if w, ok := s.gpuWatch(goal); ok {
				s.lookAt(p, w, 0.2)
				engaged = true // hold the angle: walk relative to the view
			}
		}
	case p.Team == content.Vibe && bomb.State == BombCarried && bomb.Carrier == p.ID:
		if z := s.siteZone(b.site); z != nil && z.Contains(p.Pos) {
			plant = true
			break
		}
		goal, key = s.mustSite(b.site), "plant:"+b.site
	case p.Team == content.Vibe && bomb.State == BombDropped:
		goal, key = bomb.Pos, fmt.Sprintf("bomb:%.0f,%.0f", bomb.Pos[0], bomb.Pos[2])
	case bomb.State == BombPlanted && p.Team == content.Artisan:
		// Within the reach defusable() promises a plant (its grid path may end
		// that far off: a bomb against a wall), inside the server's own range.
		if math.Hypot(p.Pos[0]-bomb.Pos[0], p.Pos[2]-bomb.Pos[2]) <= defuseRange-0.1 && math.Abs(p.Pos[1]-bomb.Pos[1]) < 1.2 {
			use = true
			break
		}
		goal, key = bomb.Pos, "defuse"
		if !engaged && s.aiWaitForRetake(p) {
			s.lookAt(p, Vec3{bomb.Pos[0], bomb.Pos[1] + 1, bomb.Pos[2]}, 0.2)
			return
		}
	case bomb.State == BombPlanted:
		goal, key = bomb.Pos, "guard"
	case p.Team == content.Artisan:
		if !engaged {
			s.aiRotate(p)
		}
		post := s.defPost(p)
		goal, key = post.at, fmt.Sprintf("site:%s:%.1f,%.1f", b.site, post.at[0], post.at[2])
		if !engaged {
			defending = true
			if post.forward && len(b.path) == 0 { // out front: watch their way in
				s.lookAt(p, Vec3{post.watch[0], post.watch[1] + 1.0, post.watch[2]}, 0.2)
				engaged, defending = true, false
			}
		}
	default:
		goal, key = s.mustSite(b.site), "site:"+b.site
	}
	if key != "" && !plant && !use {
		goal, key = s.aiAcross(p, goal, key) // on another nav island: by ship (ai_board.go)
	}
	if !engaged && !defending && p.Team == content.Vibe && key != "" {
		// On the move: pre-aim where a teammate just saw someone, if it's ahead.
		if at, ok := s.intel(p.Team); ok && math.Hypot(at[0]-p.Pos[0], at[2]-p.Pos[2]) < aiIntelRange &&
			math.Abs(angleDiff(math.Atan2(-(at[0]-p.Pos[0]), -(at[2]-p.Pos[2])), math.Atan2(-(goal[0]-p.Pos[0]), -(goal[2]-p.Pos[2])))) < 1.6 {
			s.lookAt(p, Vec3{at[0], at[1] + 1.25, at[2]}, 0.2)
			engaged = true
		}
	}
	if defending { // hold an angle: look where they come from, walk relative to the view
		if at, ok := s.aiWatch(p); ok {
			s.lookAt(p, at, 0.2)
			engaged = true
		}
	}
	if use {
		c.B |= BtnUse
		b.path = nil
		return
	}
	if plant { // draw The Big Diff (slot 5) and hold fire, standing still
		if p.inv != nil && p.inv.Current != BombSlot {
			c.W = BombSlot
		}
		c.B |= BtnFire
		b.path = nil
		return
	}
	if key == "" {
		return
	}
	s.laneProgress(p)
	if key != b.goalKey || p.cmdTicks >= b.repathAt {
		b.goalKey, b.goal = key, goal
		b.path = s.attackPath(p, key, goal)
		b.repathAt = math.MaxInt64 // until the goal changes or the bot gets stuck
	}
	// The route ran out short of the goal (it wasn't a node): walk the rest.
	if len(b.path) == 0 && math.Hypot(goal[0]-p.Pos[0], goal[2]-p.Pos[2]) > 1.5 && key != "" && !strings.HasPrefix(key, "site:") {
		b.path = []Vec3{goal}
	}
	// Right below (or above) the goal, on another floor (de_stack's sites sit
	// on top of rooms): route there again from this floor.
	if p.body != nil && p.body.OnGround && b.r.Float() < 0.05 && math.Abs(goal[1]-p.Pos[1]) > 2.5 &&
		(len(b.path) == 0 || math.Hypot(b.path[0][0]-p.Pos[0], b.path[0][2]-p.Pos[2]) < 1.2 && math.Abs(b.path[0][1]-p.Pos[1]) > 2.5) {
		b.path = s.navPath(p.Pos, goal)
	}
	// Arrived at a holding spot: stand and watch, turning now and then.
	if len(b.path) == 0 {
		if !engaged && !defending && b.r.Float() < 0.01 {
			b.yaw += b.r.Range(-1.2, 1.2)
		}
		return
	}
	if s.aiThrown(p) {
		return
	}
	next := b.path[0]
	dx, dz := next[0]-p.Pos[0], next[2]-p.Pos[2]
	reach := 0.7
	if len(b.path) == 1 && !strings.HasPrefix(key, "loot:") { // loot: walk right onto it (pickupRadius)
		reach = 1.2
	}
	// Arrived: close by, and on its floor (not passing over or under it:
	// de_stack's roof spawn sits right above the escalator's foot).
	if math.Hypot(dx, dz) < reach && math.Abs(next[1]-p.Pos[1]) < aiArriveDY {
		b.leg = [2]Vec3{next, {}}
		if len(b.path) > 1 {
			b.leg[1] = b.path[1]
		}
		b.path = b.path[1:]
		b.strafe = 0 // moving again: no more stuck sidestep (it would carry the bot off the next leg)
		return
	}
	if b.leg[1] == next && math.Abs(next[1]-b.leg[0][1]) > aiClimb {
		// Up or down a floor (stairs, an escalator between handrails): keep
		// to the line between the waypoints, aiming a little ahead on it.
		dx, dz = aimOnLeg(b.leg[0], next, p.Pos)
	}
	b.steer(c, math.Atan2(-dx, -dz), engaged)
	// A waypoint higher than a step, close by (a crate top on a grid leg): jump.
	if p.body != nil && p.body.OnGround && next[1]-p.Pos[1] > aiStepUp && next[1]-p.Pos[1] < aiJumpUp+0.2 && math.Hypot(dx, dz) < 1.2 {
		c.B |= BtnJump
	}
	if s.aiGiveWay(p, c, next, engaged) {
		return
	}
	// Stuck (a corner, a crate edge): jump and sidestep for a moment.
	if p.cmdTicks-b.stuckAt >= SimHz {
		if math.Hypot(p.Pos[0]-b.lastPos[0], p.Pos[2]-b.lastPos[2]) < 0.5 {
			c.B |= BtnJump
			b.strafe = s.sidestep(p, float64(b.r.Int(0, 1)*2-1))
			b.repathAt = p.cmdTicks
		} else {
			b.strafe = 0
		}
		b.lastPos, b.stuckAt = p.Pos, p.cmdTicks
	}
	if b.strafe != 0 {
		c.R = b.strafe // stuck: sidestep
	}
}

// aiThrown: bot p is flying a jump pad's throw (in the air, faster than a
// run): it drops the waypoints behind it (its pad's) and coasts, the arc
// lands it on the next one (sim.PadVel). Its keys would only steer it off.
func (s *Server) aiThrown(p *Player) bool {
	bd := p.body
	if bd == nil || bd.OnGround || math.Hypot(bd.Vel[0], bd.Vel[2]) <= sim.Run*1.2 {
		return false
	}
	b := p.ai
	for len(b.path) > 1 && (b.path[0][0]-p.Pos[0])*bd.Vel[0]+(b.path[0][2]-p.Pos[2])*bd.Vel[2] < 0 {
		b.path = b.path[1:]
	}
	b.stuckAt, b.lastPos, b.strafe = p.cmdTicks, p.Pos, 0 // flying: not stuck
	return true
}

// sidestep is the stuck sidestep's side (1 right, -1 left): the one asked
// for, else the other, where there is floor to step onto (not off a bridge
// into a pit: he_merge_hell); 0 if neither.
func (s *Server) sidestep(p *Player, side float64) float64 {
	right := Vec3{math.Cos(p.ai.yaw), 0, -math.Sin(p.ai.yaw)}
	for _, sd := range []float64{side, -side} {
		to := Vec3{p.Pos[0] + right[0]*sd*1.2, p.Pos[1], p.Pos[2] + right[2]*sd*1.2}
		if s.floorAlong(p.Pos, to) {
			return sd
		}
	}
	return 0
}

// aiEdgeGuard keeps a bot out of a map's pits (Map.Pits: he_merge_hell's
// main): a command that would walk it (or jump it) off the floor stops it
// instead. Every other move (chasing, strafing in a fight, the stuck
// sidestep and jump) went over the bridges' edges and their rails. People
// fall in; bots only when a grenade throws them.
func (s *Server) aiEdgeGuard(p *Player, c *Cmd) {
	if len(s.Map.Pits) == 0 || p.body == nil || !p.body.OnGround || c.F == 0 && c.R == 0 {
		return
	}
	fwd := Vec3{-math.Sin(c.Yaw), 0, -math.Cos(c.Yaw)}
	right := Vec3{math.Cos(c.Yaw), 0, -math.Sin(c.Yaw)}
	dx, dz := float64(c.F*fwd[0])+float64(c.R*right[0]), float64(c.F*fwd[2])+float64(c.R*right[2])
	l := math.Hypot(dx, dz)
	if l < 1e-9 {
		return
	}
	ahead := aiEdgeAhead
	if c.B&BtnJump != 0 {
		ahead = aiEdgeAheadJump
	}
	to := Vec3{p.Pos[0] + dx/l*ahead, p.Pos[1], p.Pos[2] + dz/l*ahead}
	if !s.floorAlong(p.Pos, to) {
		c.F, c.R = 0, 0
		c.B &^= BtnJump
	}
}

// aiEdgeAhead: metres of floor a bot on a pit map wants ahead of a step (a
// run stops in about this); aiEdgeAheadJump: ahead of a jump.
const (
	aiEdgeAhead     = 1.2
	aiEdgeAheadJump = 2.5
)

// aiArriveDY: a waypoint counts as reached within this height of it.
const aiArriveDY = 1.5

// aiClimb: a leg between waypoints rising or falling more than this changes
// floors (stairs, an escalator): the bot keeps to its line (aimOnLeg).
const aiClimb = 1.0

// aiLegAhead: how far ahead along the leg's line a bot on it aims.
const aiLegAhead = 1.0

// aimOnLeg: the way (dx, dz) from pos to the point aiLegAhead past pos's
// projection on the leg a -> b (at most b): pure pursuit, so a bot off the
// line steers back onto it instead of cutting the corner into a handrail.
func aimOnLeg(a, b, pos Vec3) (float64, float64) {
	lx, lz := b[0]-a[0], b[2]-a[2]
	l := math.Hypot(lx, lz)
	if l < 1e-9 {
		return b[0] - pos[0], b[2] - pos[2]
	}
	t := math.Min(l, ((pos[0]-a[0])*lx+(pos[2]-a[2])*lz)/l+aiLegAhead)
	return a[0] + lx/l*t - pos[0], a[2] + lz/l*t - pos[2]
}

func (s *Server) mustSite(name string) Vec3 {
	if z := s.siteZone(name); z != nil {
		return Vec3{(z.Min[0] + z.Max[0]) / 2, z.Min[1] + 0.5, (z.Min[2] + z.Max[2]) / 2}
	}
	return Vec3{}
}

// navPath: waypoints from 'from' to 'to' over the map's nav graph (A*,
// straight-line distance), ending at 'to'. Without a graph: straight there.
func (s *Server) navPath(from, to Vec3) []Vec3 {
	nav := s.Map.Nav
	if len(nav) == 0 {
		return []Vec3{to}
	}
	idx := map[string]int{}
	for i, n := range nav {
		idx[n.Name] = i
	}
	// A jump pad's node is no waypoint to walk on through: stepping on it
	// throws you (its only way on is its throw, a one-way link), and it is no
	// goal's node unless the goal is on that pad (de_serverless: defenders
	// walked to their spot by A's pad down and were thrown back to the hub).
	pad := make([]bool, len(nav))
	for i, n := range nav {
		pad[i] = s.onLauncher(Vec3(n.Pos)) && !s.onLauncher(to)
	}
	adj := make([][]int, len(nav))
	for i, n := range nav {
		for _, l := range n.Links {
			if j, ok := idx[l]; ok {
				if !pad[i] {
					adj[i] = append(adj[i], j)
				}
				if !pad[j] {
					adj[j] = append(adj[j], i)
				}
			}
		}
		for _, l := range n.OneWay { // onto a jump pad, its throw
			if j, ok := idx[l]; ok {
				adj[i] = append(adj[i], j)
			}
		}
	}
	dist := func(a, b [3]float64) float64 {
		return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
	}
	nearest := func(p Vec3) int {
		bi, bd := 0, math.Inf(1)
		for i, n := range nav {
			d := dist(n.Pos, p)
			if !s.segmentClear(p, Vec3(n.Pos), 0.3) || math.Abs(n.Pos[1]-p[1]) > 2 {
				d += 1000 // prefer one we can walk to straight, on our floor (de_stack: not the site above us)
			}
			if d < bd {
				bi, bd = i, d
			}
		}
		return bi
	}
	start := nearest(from)
	// The goal's node: the one nearest the goal among those reachable from
	// the start (a map's graph may come in pieces; walking at a node in
	// another piece means walking into walls).
	reach := map[int]bool{start: true}
	for queue := []int{start}; len(queue) > 0; queue = queue[1:] {
		for _, j := range adj[queue[0]] {
			if !reach[j] {
				reach[j] = true
				queue = append(queue, j)
			}
		}
	}
	end, bestEnd := start, math.Inf(1)
	for i := range nav {
		if !reach[i] {
			continue
		}
		d := dist(nav[i].Pos, to)
		if !s.segmentClear(Vec3(nav[i].Pos), to, 0.3) {
			d += 1000
		}
		if pad[i] {
			d += 2000 // not onto a pad to stop by it
		}
		if d < bestEnd {
			end, bestEnd = i, d
		}
	}
	g := make([]float64, len(nav))
	prev := make([]int, len(nav))
	open := map[int]bool{start: true}
	for i := range g {
		g[i], prev[i] = math.Inf(1), -1
	}
	g[start] = 0
	for len(open) > 0 {
		cur, bf := -1, math.Inf(1)
		for i := range open {
			if f := g[i] + dist(nav[i].Pos, nav[end].Pos); f < bf || f == bf && i < cur {
				cur, bf = i, f
			}
		}
		delete(open, cur)
		if cur == end {
			break
		}
		for _, j := range adj[cur] {
			if ng := g[cur] + dist(nav[cur].Pos, nav[j].Pos); ng < g[j] {
				g[j], prev[j] = ng, cur
				open[j] = true
			}
		}
	}
	var path []Vec3
	for i := end; i != -1; i = prev[i] {
		path = append([]Vec3{Vec3(nav[i].Pos)}, path...)
		if i == start {
			break
		}
	}
	// Skip the first waypoint if we can already walk past it toward the second
	// (nothing in the way, and floor all the way: no cutting a corner over a
	// pit, he_merge_hell's bridges).
	if len(path) > 1 && s.segmentClear(from, path[1], 0.7) && math.Abs(path[1][1]-from[1]) < 0.5 && s.floorAlong(from, path[1]) {
		path = path[1:]
	}
	// The last leg, off the graph: straight to 'to' if nothing is in the way,
	// else around (or up onto) the crates on a local walk grid (ai_grid.go).
	last := path[len(path)-1]
	if !s.segmentClear(last, to, 0.3) || to[1]-last[1] > aiStepUp {
		if leg, ok := s.gridPath(last, to); ok {
			return append(path, leg...)
		}
	}
	return append(path, to)
}

// aiBuy: CS-ish buy logic. Rifle when the money allows a rifle and a
// vest, else armor and save; the kit for Artisans with money to spare.
func (s *Server) aiBuy(p *Player) {
	if ok, _ := s.canBuy(p); !ok || p.inv == nil {
		return
	}
	rifle := "prompt"
	if p.Team == content.Artisan {
		rifle = "linter"
	}
	if s.round.Number > 1 && p.Money >= 4750+1000 && p.ai.r.Float() < 0.25 {
		rifle = "codereview"
	}
	w := s.Weapons.Get(rifle)
	have := p.inv.Slots["primary"] != nil
	if !have && w != nil && p.Money >= w.Price+650 {
		s.purchase(p, rifle)
	}
	if p.Money >= 1000 && p.Armor < 100 {
		s.purchase(p, "helmet")
	} else if p.Money >= 650 && p.Armor < 100 {
		s.purchase(p, "vest")
	}
	if p.Team == content.Artisan && p.Money >= 400 && p.inv.Slots["primary"] != nil {
		s.purchase(p, "kit")
	}
	s.aiBuyNades(p)
}

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
