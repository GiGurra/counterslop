package game

import (
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

// addAI adds a playing bot to team ("" = the smaller side). Callers hold s.mu.
func (s *Server) addAI(team string) *Player {
	s.nextID++
	id := s.nextID
	if team != content.Vibe && team != content.Artisan {
		n := s.teamCounts(nil)
		team = content.Vibe // deterministic tie-break (scenarios replay)
		if n[content.Artisan] < n[content.Vibe] {
			team = content.Artisan
		}
	}
	name := aiNames[(id*7)%len(aiNames)]
	for _, p := range s.ordered() {
		if p.Name == name {
			name += " " + strconv.Itoa(id)
			break
		}
	}
	p := &Player{
		ID: id, Name: name + " [bot]", Color: "#9aa0a6", ready: true,
		lastUpdate: time.Now(), joinedAt: time.Now(),
		HP: maxHP, Money: s.econ().StartMoney, Team: team, AI: true,
		ai:         &aiBrain{r: rng.New(uint32(id)*2654435761 + 12345)},
		lastQueued: 1, // command-driven from the start: respawn gives it a sim body
	}
	p.ai.seq = 1
	s.players[id] = p
	s.respawn(p)
	if s.roundsOn() && !s.frozen() {
		p.Dead, p.HP = true, 0 // mid-round: plays from the next round
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
		s.addAI("")
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
		p.ai.seq++
		c.Seq = p.ai.seq
		s.queueCmds(p, []Cmd{c})
	}
}

func (s *Server) eyeOf(p *Player) Vec3 {
	if p.body != nil {
		return Vec3{p.body.Pos[0], p.body.Pos[1] + p.body.EyeH + p.body.EyeOff, p.body.Pos[2]}
	}
	return Vec3{p.Pos[0], p.Pos[1] + 1.6, p.Pos[2]}
}

// seenCrouched: q's chest is in sight from p's eye crouched where it stands.
func (s *Server) seenCrouched(p, q *Player) bool {
	eye := Vec3{p.Pos[0], p.Pos[1] + sim.EyeCrouch, p.Pos[2]}
	return s.lineClear(eye, Vec3{q.Pos[0], q.Pos[1] + 1.2, q.Pos[2]})
}

// visible: a clear line from p's eye to q's chest within p's field of view.
func (s *Server) visible(p, q *Player, fov float64) (dist float64, ok bool) {
	if p.flashLeft(s.clock()) > 0.4 { // flashed: sees nothing (a Stack Trace works on bots too)
		return 0, false
	}
	o := s.eyeOf(p)
	t := Vec3{q.Pos[0], q.Pos[1] + 1.2, q.Pos[2]}
	if q.Crouch {
		t[1] = q.Pos[1] + 0.8
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
	return dist, mapDistance(&s.Map, o, n) >= dist-0.2
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
		if tgt != nil && tgt.ID == b.target && p.Team == content.Artisan && best >= aiEnemyNadeMin && best <= aiEnemyNadeMax && p.cmdTicks-b.seenAt >= sk.react && !s.facing(tgt, p) {
			busy = s.aiStartNade(p, "he", tgt.Pos, true)
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
		aimY := tgt.Pos[1] + 1.25
		if tgt.Crouch {
			aimY = tgt.Pos[1] + 0.85
		}
		d := Vec3{tgt.Pos[0] - o[0], aimY - o[1], tgt.Pos[2] - o[2]}
		yaw := math.Atan2(-d[0], -d[2]) + b.aimErr[0]*decay
		pitch := math.Atan2(d[1], math.Hypot(d[0], d[2])) + b.aimErr[1]*decay
		b.turnTo(yaw, pitch, sk.turn)
		off := math.Hypot(angleDiff(yaw, b.yaw), pitch-b.pitch)
		if p.inv.Current == BombSlot { // a fight with The Big Diff in hand: a gun first
			c.W = s.aiBestGun(p)
		} else if held != nil && held.Ammo == 0 && held.ID != "keyboard" {
			if other := p.inv.Slots["sidearm"]; p.inv.Current == "primary" && other != nil && other.Ammo > 0 {
				c.W = "sidearm"
			} else {
				c.B |= BtnReload
			}
		} else if age >= sk.react && off < 0.06+math.Atan2(0.3, best) {
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
	} else {
		b.target = 0
		if held != nil && held.Ammo < max(1, s.magOf(held)/3) && held.Reserve > 0 {
			c.B |= BtnReload // reload when safe
		}
		s.aiMove(p, &c, false)
	}
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
	return names[p.ai.r.Int(0, len(names)-1)]
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
	switch {
	case !s.roundsOn() || s.frozen():
		// warmup: roam between the sites; freeze: stand (can't move anyway)
		if g, ok := siteCenter(b.site); ok && !s.frozen() {
			goal, key = g, "site:"+b.site
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
		if math.Hypot(p.Pos[0]-bomb.Pos[0], p.Pos[2]-bomb.Pos[2]) < defuseRange*0.7 && math.Abs(p.Pos[1]-bomb.Pos[1]) < 1.2 {
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
		goal, key = s.siteHold(b.site), "site:"+b.site
		if !engaged {
			defending = true
		}
	default:
		goal, key = s.mustSite(b.site), "site:"+b.site
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
	next := b.path[0]
	dx, dz := next[0]-p.Pos[0], next[2]-p.Pos[2]
	reach := 0.7
	if len(b.path) == 1 {
		reach = 1.2
	}
	if math.Hypot(dx, dz) < reach {
		b.path = b.path[1:]
		return
	}
	b.steer(c, math.Atan2(-dx, -dz), engaged)
	// Stuck (a corner, a crate edge): jump and sidestep for a moment.
	if p.cmdTicks-b.stuckAt >= SimHz {
		if math.Hypot(p.Pos[0]-b.lastPos[0], p.Pos[2]-b.lastPos[2]) < 0.5 {
			c.B |= BtnJump
			b.strafe = float64(b.r.Int(0, 1)*2 - 1)
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
	adj := make([][]int, len(nav))
	for i, n := range nav {
		for _, l := range n.Links {
			if j, ok := idx[l]; ok {
				adj[i] = append(adj[i], j)
				adj[j] = append(adj[j], i)
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
	// Skip the first waypoint if we can already walk past it toward the second.
	if len(path) > 1 && s.segmentClear(from, path[1], 0.7) && math.Abs(path[1][1]-from[1]) < 0.5 {
		path = path[1:]
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
