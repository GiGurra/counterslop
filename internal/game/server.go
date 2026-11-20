// Package game is the authoritative-ish game server: it accepts websocket
// players, relays their movement to everyone at a fixed tick and keeps
// obviously impossible movement in check.
package game

import (
	"encoding/json"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
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
	Crouch bool   `json:"c"`
	Bot    bool   `json:"bot,omitempty"`
	Weapon string `json:"w,omitempty"` // id of the weapon in hand, for remote viewmodels

	lastUpdate    time.Time
	send          chan []byte // nil for bots (trySend drops)
	ready         bool        // said hello; hidden from snapshots until then
	brain         *botBrain
	inv           *Inventory
	admin         bool          // logged in with the admin password
	ip            string        // for bans, rate limits and admin status
	kick          chan struct{} // closed to drop the connection
	noPickupUntil time.Time
}

type Server struct {
	Map     content.Map
	Version string // git SHA of this binary, for display
	Build   string // hash of the client assets; clients reload when theirs differs
	Test    bool   // test harness mode: no speed checks, so scenarios can teleport
	// Bots is how many target-practice bots start with the server (admins add more).
	Bots int
	// AdminPassword unlocks admin commands; empty means everyone is admin.
	AdminPassword string
	// Weapons is the shared registry (web/assets/weapons/weapons.json).
	Weapons *Registry

	mu      sync.Mutex
	nextID  int
	players map[int]*Player

	items       []*Item
	nextItem    int
	itemRespawn map[int]time.Time // map item index -> when it comes back

	cvars      map[string]string
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
}

type welcomeMsg struct {
	T        string      `json:"t"`
	ID       int         `json:"id"`
	Color    string      `json:"color"`
	Spawn    Vec3        `json:"spawn"`
	Map      content.Map `json:"map"`
	Tick     float64     `json:"tick"`
	Version  string      `json:"version"`
	Build    string      `json:"build"`
	Hitboxes HitboxSet   `json:"hitboxes"`
}

type snapMsg struct {
	T       string    `json:"t"`
	Time    int64     `json:"time"`
	Players []*Player `json:"players"`
	Items   []*Item   `json:"items"`
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

	welcome, _ := json.Marshal(welcomeMsg{T: "welcome", ID: p.ID, Color: p.Color, Spawn: p.Pos, Map: s.Map, Tick: TickRate, Version: s.Version, Build: s.Build, Hitboxes: Hitboxes})
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
	sp := s.Map.Spawns[(id-1)%len(s.Map.Spawns)]
	p := &Player{
		ID:         id,
		Name:       "slopper" + strconv.Itoa(id),
		Color:      palette[(id-1)%len(palette)],
		Pos:        Vec3(sp),
		lastUpdate: time.Now(),
		send:       make(chan []byte, sendQueue),
		ip:         ip,
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
	for _, p := range s.players {
		list = append(list, *p)
	}
	return map[string]any{"map": s.Map.Name, "players": list}
}

func (s *Server) leave(p *Player) {
	s.mu.Lock()
	if s.players[p.ID] == p {
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
	case "shot":
		if p.inv == nil {
			return
		}
		w := s.Weapons.fire(p.inv, time.Now())
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
	case "reload":
		if p.inv != nil {
			s.Weapons.reload(p.inv, time.Now())
		}
	case "switch":
		if p.inv != nil && s.Weapons.switchTo(p.inv, m.Slot) {
			s.sendInv(p)
		}
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
	t := time.NewTicker(time.Second / TickRate)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.mu.Lock()
			s.tickBots(1.0 / TickRate)
			if s.Weapons != nil {
				s.tickItems(time.Now())
			}
			s.mu.Unlock()
			s.broadcast()
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
		if !p.Bot || p == shooter {
			continue
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
			p.Weapon = p.inv.Slots[p.inv.Current].ID
		}
	}
	b, err := json.Marshal(snapMsg{T: "snap", Time: time.Now().UnixMilli(), Players: list, Items: s.items})
	if err != nil {
		return
	}
	for _, p := range s.players {
		trySend(p, b)
	}
}

// trySend never blocks the tick: a client too slow to drain its queue just
// misses snapshots.
func trySend(p *Player, b []byte) {
	select {
	case p.send <- b:
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

func copyCvars() map[string]string {
	m := make(map[string]string, len(defaultCvars))
	for k, v := range defaultCvars {
		m[k] = v
	}
	return m
}
