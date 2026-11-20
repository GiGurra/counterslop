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
	Crouch bool `json:"c"`

	lastUpdate time.Time
	send       chan []byte
	ready      bool // said hello; hidden from snapshots until then
}

type Server struct {
	Map     content.Map
	Version string // git SHA of this binary, for display
	Build   string // hash of the client assets; clients reload when theirs differs
	Test    bool   // test harness mode: no speed checks, so scenarios can teleport

	mu      sync.Mutex
	nextID  int
	players map[int]*Player

	upgrader websocket.Upgrader
}

func NewServer() *Server {
	return &Server{
		Map:     maps.DeSlop(),
		players: map[int]*Player{},
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
	T     string  `json:"t"`
	Name  string  `json:"name"`
	Pos   Vec3    `json:"p"`
	Yaw   float64 `json:"yaw"`
	Pitch float64 `json:"pitch"`
	Seq   float64 `json:"seq"`
	C     bool    `json:"c"`
}

type welcomeMsg struct {
	T       string      `json:"t"`
	ID      int         `json:"id"`
	Color   string      `json:"color"`
	Spawn   Vec3        `json:"spawn"`
	Map     content.Map `json:"map"`
	Tick    float64     `json:"tick"`
	Version string      `json:"version"`
	Build   string      `json:"build"`
}

type snapMsg struct {
	T       string    `json:"t"`
	Time    int64     `json:"time"`
	Players []*Player `json:"players"`
}

type correctMsg struct {
	T   string `json:"t"`
	Pos Vec3   `json:"p"`
}

// HandleWS upgrades the request and runs the player until it disconnects.
func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
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

	p := s.join()
	defer s.leave(p)
	slog.Info("player joined", "id", p.ID, "remote", r.RemoteAddr)

	welcome, _ := json.Marshal(welcomeMsg{T: "welcome", ID: p.ID, Color: p.Color, Spawn: p.Pos, Map: s.Map, Tick: TickRate, Version: s.Version, Build: s.Build})
	p.send <- welcome

	done := make(chan struct{})
	go s.writeLoop(conn, p, done)
	defer close(done)

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

func (s *Server) join() *Player {
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
	delete(s.players, p.ID)
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
		// Relay to everyone else so they hear the pew and see the flash.
		b, _ := json.Marshal(map[string]any{"t": "shot", "id": p.ID})
		for _, o := range s.players {
			if o != p {
				trySend(o, b)
			}
		}
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

// Run broadcasts snapshots at TickRate until stop is closed.
func (s *Server) Run(stop <-chan struct{}) {
	t := time.NewTicker(time.Second / TickRate)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.broadcast()
		}
	}
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
	b, err := json.Marshal(snapMsg{T: "snap", Time: time.Now().UnixMilli(), Players: list})
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
