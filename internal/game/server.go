// Package game is the authoritative-ish game server: it accepts websocket
// players, relays their movement to everyone at a fixed tick and keeps
// obviously impossible movement in check.
package game

import (
	"encoding/json"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/sim"
	"github.com/gorilla/websocket"
)

const (
	TickRate     = 30               // snapshots per second
	MaxSpeed     = 12.0             // units/s, a bit above client run speed
	MaxFallSpeed = 40.0             // units/s vertical
	sendQueue    = 64               // buffered outbound messages per player
	writeWait    = 2 * time.Second  // per write deadline
	idleTimeout  = 15 * time.Second // drop silent clients
)

var palette = []string{"#ff4f8b", "#4fd1ff", "#ffd84f", "#7dff4f", "#b84fff", "#ff8a4f", "#4fffc3", "#ffffff"}

type Vec3 [3]float64

// MarshalJSON sends millimetres: plenty for rendering, half the bytes of
// full float64s. (Reconciliation state uses plain [3]float64: exact.)
func (v Vec3) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, 32)
	b = append(b, '[')
	for i, f := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendFloat(b, math.Round(f*1000)/1000, 'f', -1, 64)
	}
	return append(b, ']'), nil
}

type Player struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Color string  `json:"color"`
	Pos   Vec3    `json:"p"`
	Yaw   float64 `json:"yaw"`
	Pitch float64 `json:"pitch"`
	// Crouch is relayed for rendering and hit tests (hitbox 1.2 instead of 1.8).
	// It doesn't change the movement sanity checks: a crouch-jump lifts the feet
	// 0.6 instantly, well inside MaxFallSpeed's per-update budget.
	Crouch  bool   `json:"c"`
	Bot     bool   `json:"bot,omitempty"`
	Weapon  string `json:"w,omitempty"` // id of the weapon in hand, for remote viewmodels
	Ammo    int    `json:"ammo"`        // in the weapon in hand (spectators' first-person HUD)
	Reserve int    `json:"res"`
	HP      int    `json:"hp"`
	Dead    bool   `json:"dead,omitempty"`
	Kills   int    `json:"k"`
	Deaths  int    `json:"d"`
	Team    string `json:"team"`

	// spawnSeq counts server-side respawns (team switches, deaths). Legacy
	// states the client sent before it saw the newest spawn carry an older
	// seq and are dropped, so they can't drag the player back.
	spawnSeq int

	lastUpdate time.Time
	send       chan []byte // nil for bots (trySend drops)
	ready      bool        // said hello; hidden from snapshots until then
	brain      *botBrain
	inv        *Inventory
	admin      bool          // logged in with the admin password
	ip         string        // for bans, rate limits and admin status
	kick       chan struct{} // closed to drop the connection

	// Authoritative sim (once the client sends commands; before that the
	// legacy client-moved path applies).
	body          *sim.Body
	cmds          []Cmd
	lastQueued    int // highest command seq received
	ackSeq        int // last command applied (sent back in snapshots)
	lastCmd       Cmd
	cmdTicks      int64   // commands applied: the clock weapon timers run on
	cmdBudget     float64 // commands this player may still run (see simTick)
	cmdsDropped   int     // commands dropped on queue overflow (cl_netdebug)
	paced         bool    // test mode: this player gets the real command budget (harness pacing tests)
	respawnAt     time.Time
	bytesOut      int64
	bytesIn       int64
	joinedAt      time.Time
	noPickupUntil time.Time
}

type Server struct {
	lastTickAt time.Time // when simTick last ran (command budgets)
	history    History   // poses sent in recent snapshots (lag compensation)
	Map        content.Map
	Version    string // git SHA of this binary, for display
	Build      string // hash of the client assets; clients reload when theirs differs
	Test       bool   // test harness mode: no speed checks, so scenarios can teleport
	lockstep   bool   // test harness: Run's ticker pauses; the harness Steps the server
	// Bots is how many target-practice bots start with the server (admins add more).
	Bots int
	// AdminPassword unlocks admin commands; empty means everyone is admin.
	AdminPassword string
	// Weapons is the shared registry (web/assets/weapons/weapons.json).
	Weapons   *Registry
	spawnNext map[string]int
	round     roundState
	now       time.Time // the current tick's time (simTick)
	worked    bool      // roundUse: this command advanced a plant/defuse

	mu      sync.Mutex
	nextID  int
	players map[int]*Player

	items       []*Item
	nextItem    int
	itemRespawn map[int]time.Time // map item index -> when it comes back

	cvars      map[string]string
	tick       int64
	loginFails map[string]*loginFails // by IP
	bans       map[string]time.Time   // IP -> until

	upgrader websocket.Upgrader
}

func NewServer() *Server {
	return &Server{
		Map:         maps.DeSlop(),
		players:     map[int]*Player{},
		itemRespawn: map[int]time.Time{},
		cvars:       copyCvars(),
		loginFails:  map[string]*loginFails{},
		bans:        map[string]time.Time{},
		spawnNext:   map[string]int{},
		round:       newRound(),
		upgrader: websocket.Upgrader{
			ReadBufferSize:    4096,
			WriteBufferSize:   4096,
			EnableCompression: false, // compression adds latency; payloads are tiny
			CheckOrigin:       allowedOrigin,
		},
	}
}

// inbound is every client->server message; T selects which fields matter.
type inbound struct {
	T     string   `json:"t"`
	Name  string   `json:"name"`
	Pos   Vec3     `json:"p"`
	Yaw   float64  `json:"yaw"`
	Pitch float64  `json:"pitch"`
	Seq   float64  `json:"seq"`
	C     bool     `json:"c"`
	O     Vec3     `json:"o"`   // shot origin (eye)
	D     Vec3     `json:"d"`   // shot direction
	Cmd   string   `json:"cmd"` // bot command
	Arg   string   `json:"arg"`
	Slot  string   `json:"slot"` // switch
	Pw    string   `json:"pw"`   // admin_login
	Args  []string `json:"args"` // rcon
	Cmds  []Cmd    `json:"cmds"` // input commands
	Team  string   `json:"team"`
	Sp    int      `json:"sp"`
}

type welcomeMsg struct {
	T       string      `json:"t"`
	ID      int         `json:"id"`
	Color   string      `json:"color"`
	Spawn   Vec3        `json:"spawn"`
	Map     content.Map `json:"map"`
	Tick    float64     `json:"tick"`
	SimHz   int         `json:"simHz"`
	Version string      `json:"version"`
	Build   string      `json:"build"`
	Team    string      `json:"team"`
}

// meState is the recipient's own authoritative body in a snapshot.
type meState struct {
	Ack      int        `json:"ack"`
	Pos      [3]float64 `json:"p"` // exact: the client replays from here
	Vel      [3]float64 `json:"v"`
	OnGround bool       `json:"g"`
	Crouched bool       `json:"c"`
	HP       int        `json:"hp"`
	Dead     bool       `json:"dead,omitempty"`
	Queue    int        `json:"q"`              // commands waiting on the server (cl_netdebug)
	Dropped  int        `json:"drop,omitempty"` // commands dropped on overflow, cumulative
}

// snapBytes assembles a snapshot from pre-marshalled parts (the player list is
// shared; only "me" differs per recipient).
func snapBytes(ms int64, tick int64, players, items, me, round []byte) []byte {
	b := make([]byte, 0, len(players)+len(items)+len(me)+len(round)+64)
	b = append(b, `{"t":"snap","time":`...)
	b = strconv.AppendInt(b, ms, 10)
	b = append(b, `,"tick":`...)
	b = strconv.AppendInt(b, tick, 10)
	b = append(b, `,"players":`...)
	b = append(b, players...)
	b = append(b, `,"items":`...)
	b = append(b, items...)
	if round != nil {
		b = append(b, `,"round":`...)
		b = append(b, round...)
	}
	if me != nil {
		b = append(b, `,"me":`...)
		b = append(b, me...)
	}
	return append(b, '}')
}

type correctMsg struct {
	T   string `json:"t"`
	Pos Vec3   `json:"p"`
}

// HandleWS upgrades the request and runs the player until it disconnects.
func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	s.mu.Lock()
	banned := s.banned(ip)
	s.mu.Unlock()
	if banned {
		http.Error(w, "banned", http.StatusForbidden)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("upgrade failed", "err", err)
		return
	}
	// Disable Nagle: every snapshot and input should leave immediately
	// rather than wait to be coalesced with the next one.
	if tcp, ok := conn.NetConn().(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	defer conn.Close()

	p := s.join(ip)
	defer s.leave(p)
	slog.Info("player joined", "id", p.ID, "remote", r.RemoteAddr)

	welcome, _ := json.Marshal(welcomeMsg{T: "welcome", ID: p.ID, Color: p.Color, Spawn: p.Pos, Map: s.Map, Tick: float64(SimHz) / snapEvery, SimHz: SimHz, Version: s.Version, Build: s.Build, Team: p.Team})
	p.send <- welcome
	s.mu.Lock()
	s.sendInv(p)
	s.motd(p)
	kick := p.kick
	s.mu.Unlock()

	done := make(chan struct{})
	go s.writeLoop(conn, p, done)
	defer close(done)
	go func() {
		select {
		case <-kick:
			time.Sleep(200 * time.Millisecond) // let the notice go out
			conn.Close()
		case <-done:
		}
	}()

	for {
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
		_, data, err := conn.ReadMessage()
		if err != nil {
			slog.Info("player left", "id", p.ID, "err", err)
			return
		}
		s.mu.Lock()
		p.bytesIn += int64(len(data))
		s.mu.Unlock()
		var m inbound
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		s.handle(p, m)
	}
}

func (s *Server) writeLoop(conn *websocket.Conn, p *Player, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case msg := <-p.send:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				conn.Close() // unblocks the read loop
				return
			}
		}
	}
}

func (s *Server) join(ip string) *Player {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := s.nextID
	// Everyone starts spectating; the client's team-select screen picks a side.
	p := &Player{
		ID:         id,
		Name:       "slopper" + strconv.Itoa(id),
		Color:      palette[(id-1)%len(palette)],
		Team:       content.Spectator,
		Pos:        Vec3(s.Map.Spawns[0]),
		lastUpdate: time.Now(),
		send:       make(chan []byte, sendQueue),
		ip:         ip,
		joinedAt:   time.Now(),
		HP:         maxHP,
		kick:       make(chan struct{}),
	}
	if s.Weapons != nil {
		p.inv = s.Weapons.Loadout("")
	}
	s.players[id] = p
	return p
}

// DebugState is the server's view of the world, for the test harness.
func (s *Server) DebugState() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Player, 0, len(s.players))
	net := map[int]map[string]float64{}
	for _, p := range s.players {
		list = append(list, *p)
		if !p.Bot {
			down, up := p.rates()
			net[p.ID] = map[string]float64{"downKBps": down, "upKBps": up}
		}
	}
	return map[string]any{"map": s.Map.Name, "players": list, "net": net}
}

// teamCounts counts players per team, leaving out except. Callers hold s.mu.
func (s *Server) teamCounts(except *Player) map[string]int {
	n := map[string]int{}
	for _, o := range s.players {
		if o != except {
			n[o.Team]++
		}
	}
	return n
}

// teamFull reports whether p joining team would leave it 2+ players bigger
// than the other side. Callers hold s.mu.
func (s *Server) teamFull(team string, p *Player) bool {
	n := s.teamCounts(p)
	for _, o := range content.Teams {
		if o != team && n[team]+1-n[o] >= 2 {
			return true
		}
	}
	return false
}

// smallerTeam picks the team with fewer players (ignoring except), random on ties.
// Callers hold s.mu.
func (s *Server) smallerTeam(except *Player) string {
	n := s.teamCounts(except)
	a, b := content.Teams[0], content.Teams[1]
	switch {
	case n[a] < n[b]:
		return a
	case n[b] < n[a]:
		return b
	case rand.IntN(2) == 0:
		return a
	default:
		return b
	}
}

// spawnFor cycles through the team's spawns. Callers hold s.mu.
func (s *Server) spawnFor(team string) Vec3 {
	list := s.Map.TeamSpawns[team]
	if len(list) == 0 {
		list = s.Map.Spawns
	}
	i := s.spawnNext[team]
	s.spawnNext[team] = i + 1
	return Vec3(list[i%len(list)])
}

func (s *Server) leave(p *Player) {
	s.mu.Lock()
	if s.players[p.ID] == p {
		s.dropBomb(p)
		delete(s.players, p.ID)
	}
	s.mu.Unlock()
}

func (s *Server) handle(p *Player, m inbound) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch m.T {
	case "hello":
		if n := sanitizeName(m.Name); n != "" {
			p.Name = n
		}
		p.ready = true
	case "team":
		team := m.Team
		if team == "auto" {
			team = s.smallerTeam(p)
		}
		if team == p.Team || (team != content.Spectator && !slices.Contains(content.Teams, team)) {
			return // already there (no free respawn), or unknown
		}
		if team != content.Spectator && s.teamFull(team, p) {
			b, _ := json.Marshal(map[string]any{"t": "teamfull", "team": team})
			trySend(p, b)
			return
		}
		s.dropBomb(p)
		p.Team = team
		s.respawn(p) // a new side respawns you there; spectators lose their body
		if team != content.Spectator && s.roundsOn() && !s.frozen() {
			p.Dead, p.HP = true, 0 // mid-round: you play from the next round on
		}
	case "cmd":
		s.queueCmds(p, m.Cmds)
	case "tp": // test harness only: teleport the server body, resync commands
		if !s.Test || !finite(m.Pos) {
			return
		}
		s.teleport(p, m.Pos, m.Yaw, m.C, int(m.Seq))
	case "paced": // test harness only: run this player's commands at real-time pace, as in prod
		if s.Test {
			p.paced = m.C
		}
	case "shot":
		if p.Team == content.Spectator || p.inv == nil || p.body != nil { // command-driven players fire inside the sim
			return
		}
		w, _ := s.Weapons.fire(p.inv, p.cmdTicks)
		if w == nil {
			return // too fast, empty, reloading: the server doesn't believe this shot
		}
		// Relay to everyone else so they hear the pew and see the flash.
		b, _ := json.Marshal(map[string]any{"t": "shot", "id": p.ID, "w": w.ID})
		for _, o := range s.players {
			if o != p {
				trySend(o, b)
			}
		}
		s.shotHits(p, w, m.O, m.D)
	case "drop":
		if p.inv != nil {
			s.drop(p)
		}
	case "bot": // console bot_* commands: admin commands like any other
		s.adminCommand(p, "bot_"+m.Cmd, []string{m.Arg})
	case "admin_login":
		s.adminLogin(p, m.Pw)
	case "admin_logout":
		p.admin = false
		s.print(p, "admin logout")
	case "rcon":
		s.adminCommand(p, m.Cmd, m.Args)
	case "ping":
		b, _ := json.Marshal(map[string]any{"t": "pong", "seq": m.Seq})
		trySend(p, b)
	case "state":
		if p.body != nil || m.Sp != p.spawnSeq || p.Team == content.Spectator {
			return // the server moves this player now, a state from before the latest respawn, or a spectator
		}
		now := time.Now()
		dt := now.Sub(p.lastUpdate).Seconds()
		p.lastUpdate = now
		if !finite(m.Pos) || !finiteF(m.Yaw) || !finiteF(m.Pitch) {
			return
		}
		// Generous budget: allow for jitter by padding dt.
		budget := dt + 0.1
		dx, dz := m.Pos[0]-p.Pos[0], m.Pos[2]-p.Pos[2]
		dy := math.Abs(m.Pos[1] - p.Pos[1])
		if !s.Test && (math.Hypot(dx, dz) > MaxSpeed*budget || dy > MaxFallSpeed*budget) {
			// Too fast: snap the client back to the last good position.
			b, _ := json.Marshal(correctMsg{T: "correct", Pos: p.Pos})
			trySend(p, b)
			return
		}
		p.Pos = m.Pos
		p.Yaw = m.Yaw
		p.Pitch = math.Max(-1.6, math.Min(1.6, m.Pitch))
		p.Crouch = m.C
	}
}

// clock is the game clock: the time of the last server tick. Item and
// respawn timers run on it, so they follow the ticks (a stepped test server
// too), not the wall clock. Before the first tick it is the wall clock.
func (s *Server) clock() time.Time {
	if s.lastTickAt.IsZero() {
		return time.Now()
	}
	return s.lastTickAt
}

// advance runs one server tick at `now`: queued commands, bots, items on the
// floor, and the lag-compensation history on snapshot ticks. Run calls it on
// its ticker; tests (sim/cases) call it directly. Reports whether this tick
// broadcasts a snapshot. Callers hold s.mu.
func (s *Server) advance(now time.Time) bool {
	s.simTick(now)
	s.tickRound(now)
	s.tickBots(1.0 / SimHz)
	if s.Weapons != nil {
		s.tickItems(now)
	}
	snap := s.tick%snapEvery == 0
	if snap {
		s.recordHistory()
	}
	return snap
}

// Run ticks bots and broadcasts snapshots at TickRate until stop is closed.
func (s *Server) Run(stop <-chan struct{}) {
	s.mu.Lock()
	for range max(s.Bots, 0) {
		s.addBot()
	}
	for i := range s.Map.Items {
		s.itemRespawn[i] = time.Time{} // due now: placed on the first tick
	}
	s.mu.Unlock()
	t := time.NewTicker(time.Second / SimHz)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.mu.Lock()
			if s.lockstep {
				s.mu.Unlock()
				continue
			}
			snap := s.advance(time.Now())
			s.mu.Unlock()
			if snap {
				s.broadcast()
			}
		}
	}
}

type hitMsg struct {
	T      string  `json:"t"`
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Part   string  `json:"part"`
	Damage float64 `json:"dmg"`
	Weapon string  `json:"w"`
	At     Vec3    `json:"at"`
}

// shotHits is the server-side hit test (the seed of server-authoritative
// combat): the shooter's ray against the map and every bot's hitboxes. The
// shooter gets the part hit and the damage it would deal; bots never die.
// Players aren't damaged yet. Callers hold s.mu.
func (s *Server) shotHits(shooter *Player, w *Weapon, o, d Vec3) {
	l := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
	if !finite(o) || !finite(d) || l < 0.5 || l > 1.5 {
		return
	}
	d = Vec3{d[0] / l, d[1] / l, d[2] / l}
	// The ray must start at the shooter's eye, give or take crouch and latency.
	eye := Vec3{shooter.Pos[0], shooter.Pos[1] + 1.4, shooter.Pos[2]}
	if math.Hypot(math.Hypot(o[0]-eye[0], o[1]-eye[1]), o[2]-eye[2]) > 2 {
		return
	}
	wall := math.Min(mapDistance(&s.Map, o, d), w.Range)
	var target *Player
	var part Hitbox
	best := wall
	for _, p := range s.players {
		if !p.Bot || p == shooter || p.Team == content.Spectator {
			continue
		}
		if !s.friendlyFire() && p.Team != "" && p.Team == shooter.Team {
			continue // mp_friendlyfire 0: shots pass through teammates
		}
		if h, t, ok := hitPlayer(p, o, d); ok && t < best {
			target, part, best = p, h, t
		}
	}
	if target == nil {
		return
	}
	at := Vec3{o[0] + d[0]*best, o[1] + d[1]*best, o[2] + d[2]*best}
	b, _ := json.Marshal(hitMsg{T: "hit", ID: target.ID, Name: target.Name, Part: part.Part, Weapon: w.ID, Damage: math.Round(w.Damage * part.Mult), At: at})
	trySend(shooter, b)
}

// botCommand handles console bot_add / bot_kick / bot_freeze. Callers hold s.mu.
func (s *Server) botCommand(p *Player, cmd, arg string) {
	switch cmd {
	case "add":
		s.notice(p, s.addBot().Name+" joined")
	case "kick":
		s.notice(p, "kicked "+strconv.Itoa(s.kickBots())+" bot(s)")
	case "crouch": // bot_crouch 0|1: hold every bot crouched (hit tests)
		for _, b := range s.players {
			if b.Bot {
				b.Crouch = arg == "1"
				b.brain.holdCrouch = arg == "1"
			}
		}
		s.notice(p, "bot_crouch "+arg)
	case "freeze":
		frozen := arg == "1"
		for _, b := range s.players {
			if b.Bot {
				b.brain.frozen = frozen
			}
		}
		s.notice(p, "bot_freeze "+map[bool]string{true: "1", false: "0"}[frozen])
	}
}

func (s *Server) notice(p *Player, text string) {
	b, _ := json.Marshal(map[string]string{"t": "notice", "text": text})
	trySend(p, b)
}

func (s *Server) broadcast() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.players) == 0 {
		return
	}
	list := make([]*Player, 0, len(s.players))
	for _, p := range s.players {
		if p.ready {
			list = append(list, p)
		}
	}
	for _, p := range list {
		if p.inv != nil && p.inv.Slots[p.inv.Current] != nil {
			h := p.inv.Slots[p.inv.Current]
			p.Weapon, p.Ammo, p.Reserve = h.ID, h.Ammo, h.Reserve
		}
	}
	players, err := json.Marshal(list)
	if err != nil {
		return
	}
	items, _ := json.Marshal(s.items)
	round := s.roundJSON(time.Now())
	ms := time.Now().UnixMilli()
	for _, p := range s.players {
		if p.Bot {
			continue
		}
		var me []byte
		if p.body != nil { // the authoritative state of your own body, for reconciliation
			b := p.body
			me, _ = json.Marshal(meState{Ack: p.ackSeq, Pos: b.Pos, Vel: b.Vel, OnGround: b.OnGround, Crouched: b.Crouched, HP: p.HP, Dead: p.Dead, Queue: len(p.cmds), Dropped: p.cmdsDropped})
		}
		trySend(p, snapBytes(ms, s.tick, players, items, me, round))
	}
}

// trySend never blocks the tick: a client too slow to drain its queue just
// misses snapshots.
func trySend(p *Player, b []byte) {
	select {
	case p.send <- b:
		p.bytesOut += int64(len(b))
	default:
	}
}

func finiteF(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
func finite(v Vec3) bool     { return finiteF(v[0]) && finiteF(v[1]) && finiteF(v[2]) }

func sanitizeName(n string) string {
	out := make([]rune, 0, 16)
	for _, r := range n {
		if len(out) >= 16 {
			break
		}
		if r >= 32 && r != '<' && r != '>' && r != 127 {
			out = append(out, r)
		}
	}
	return string(out)
}

// allowedOrigin keeps other sites from opening sockets with a visitor's
// browser. Non-browser clients (no Origin header) are let through.
func allowedOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	switch h := u.Hostname(); h {
	case "counterslop.gigurra.se", "counterslop-test.gigurra.se", "localhost", "127.0.0.1", "::1":
		return true
	default:
		return h == r.Host || u.Host == r.Host
	}
}

// SetCvar sets a server cvar at startup (e.g. -friendlyfire -> mp_friendlyfire 1).
func (s *Server) SetCvar(k, v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cvars[k] = v
}

// friendlyFire is mp_friendlyfire: do shots hit teammates. Callers hold s.mu.
func (s *Server) friendlyFire() bool { return s.cvars["mp_friendlyfire"] == "1" }

func copyCvars() map[string]string {
	m := make(map[string]string, len(defaultCvars))
	for k, v := range defaultCvars {
		m[k] = v
	}
	return m
}

// rates is p's average traffic since joining, in KB/s (down = server to client).
func (p *Player) rates() (down, up float64) {
	secs := time.Since(p.joinedAt).Seconds()
	if secs < 1 {
		secs = 1
	}
	return float64(p.bytesOut) / 1024 / secs, float64(p.bytesIn) / 1024 / secs
}
