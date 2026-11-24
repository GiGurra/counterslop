package game

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gorilla/websocket"
)

// msgs pops every queued message for p, decoded.
func msgs(p *Player) []map[string]any {
	var out []map[string]any
	for {
		select {
		case b := <-p.send:
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			out = append(out, m)
		default:
			return out
		}
	}
}

// ofType returns the queued messages of type typ (the rest are discarded).
func ofType(p *Player, typ string) []map[string]any {
	var out []map[string]any
	for _, m := range msgs(p) {
		if m["t"] == typ {
			out = append(out, m)
		}
	}
	return out
}

func TestVec3MarshalsMillimetres(t *testing.T) {
	for _, tc := range []struct {
		v    Vec3
		want string
	}{
		{Vec3{}, "[0,0,0]"},
		{Vec3{1.23456, -0.0004, 12}, "[1.235,-0,12]"},
		{Vec3{-3.5, 1e-9, 100.0005}, "[-3.5,0,100.001]"},
	} {
		b, err := json.Marshal(tc.v)
		if err != nil || string(b) != tc.want {
			t.Errorf("%v: got %s (%v), want %s", tc.v, b, err, tc.want)
		}
	}
}

func TestSnapBytesIsValidJSON(t *testing.T) {
	for _, me := range [][]byte{nil, []byte(`{"ack":7}`)} {
		b := snapBytes(1234, 99, []byte(`[{"id":1}]`), []byte(`[]`), me, []byte(`{"ph":"live"}`), []byte(`"nades":[{"id":3}]`), nil, []byte(`"spot":[2]`))
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("invalid snapshot %s: %v", b, err)
		}
		if m["t"] != "snap" || m["time"] != 1234.0 || m["tick"] != 99.0 || len(m["players"].([]any)) != 1 {
			t.Fatalf("snapshot %s", b)
		}
		if r, _ := m["round"].(map[string]any); r["ph"] != "live" {
			t.Fatalf("round missing in %s", b)
		}
		if _, has := m["me"]; has != (me != nil) {
			t.Fatalf("me present=%v with me=%s", has, me)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"bob", "bob"},
		{"<script>x</script>", "scriptx/script"},
		{"tab\there\x7f", "tabhere"},
		{"abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnop"},
		{"åäö日本語", "åäö日本語"},
		{"\x00\x01", ""},
	} {
		if got := sanitizeName(tc.in); got != tc.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAllowedOrigin(t *testing.T) {
	for _, tc := range []struct {
		origin, host string
		ok           bool
	}{
		{"", "anything", true},
		{"https://counterslop.gigurra.se", "x", true},
		{"https://counterslop-test.gigurra.se", "x", true},
		{"http://localhost:8080", "x", true},
		{"http://127.0.0.1:5173", "x", true},
		{"http://[::1]:80", "x", true},
		{"https://evil.example", "counterslop.gigurra.se", false},
		{"https://my.host:9000", "my.host:9000", true},
		{"https://my.host", "my.host", true},
		{"://bad", "x", false},
	} {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = tc.host
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if got := allowedOrigin(r); got != tc.ok {
			t.Errorf("origin %q host %q: %v, want %v", tc.origin, tc.host, got, tc.ok)
		}
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/ws", nil)
	r.RemoteAddr = "10.0.0.5:4321"
	if ip := clientIP(r); ip != "10.0.0.5" {
		t.Fatalf("remote addr: %q", ip)
	}
	r.RemoteAddr = "no-port"
	if ip := clientIP(r); ip != "no-port" {
		t.Fatalf("unparseable remote addr: %q", ip)
	}
	r.Header.Set("CF-Connecting-IP", "9.9.9.9")
	if ip := clientIP(r); ip != "9.9.9.9" {
		t.Fatalf("cloudflare header ignored: %q", ip)
	}
}

func TestSetCvarAndDebugState(t *testing.T) {
	s := NewServer()
	s.SetCvar("mp_friendlyfire", "1")
	if !s.friendlyFire() {
		t.Fatal("SetCvar didn't enable friendly fire")
	}
	if defaultCvars["mp_friendlyfire"] != "0" {
		t.Fatal("SetCvar leaked into the defaults")
	}
	p := s.join("1.1.1.1")
	p.bytesOut, p.bytesIn = 2048, 1024
	s.addBot()
	st := s.DebugState()
	if st["map"] != "de_slop" || len(st["players"].([]Player)) != 2 {
		t.Fatalf("debug state: %+v", st)
	}
	net := st["net"].(map[int]map[string]float64)
	if len(net) != 1 || net[p.ID]["downKBps"] != 2 || net[p.ID]["upKBps"] != 1 {
		t.Fatalf("net rates (joined <1 s ago, so per 1 s): %+v", net)
	}
	s.leave(p)
	if s.players[p.ID] != nil {
		t.Fatal("leave kept the player")
	}
	// A stale leave (the id now belongs to someone else) must not remove them.
	q := s.join("2.2.2.2")
	s.leave(&Player{ID: q.ID})
	if s.players[q.ID] != q {
		t.Fatal("leave removed a different player with the same id")
	}
}

func TestTrySendDropsWhenFull(t *testing.T) {
	p := &Player{send: make(chan []byte, 1)}
	trySend(p, []byte("ab"))
	trySend(p, []byte("cde")) // full: dropped, not counted
	if len(p.send) != 1 || p.bytesOut != 2 {
		t.Fatalf("queued %d, bytesOut %d", len(p.send), p.bytesOut)
	}
	trySend(&Player{}, []byte("x")) // bots have no channel: must not block
}

func TestHelloSetsNameAndReady(t *testing.T) {
	s := NewServer()
	p := s.join("1.1.1.1")
	s.handle(p, inbound{T: "hello", Name: "<>"}) // sanitizes to nothing: keep the default
	if !p.ready || p.Name != "slopper1" {
		t.Fatalf("ready=%v name=%q", p.ready, p.Name)
	}
	s.handle(p, inbound{T: "hello", Name: "neo"})
	if p.Name != "neo" {
		t.Fatalf("name %q", p.Name)
	}
	s.handle(p, inbound{T: "ping", Seq: 42})
	if m := reply(t, p); m["t"] != "pong" || m["seq"] != 42.0 {
		t.Fatalf("pong: %v", m)
	}
	s.handle(p, inbound{T: "nonsense"}) // ignored
	if len(p.send) != 0 {
		t.Fatal("unknown message got a reply")
	}
}

func TestLegacyStateSpeedCheck(t *testing.T) {
	for _, tc := range []struct {
		name    string
		test    bool
		to      Vec3
		moved   bool
		correct bool
	}{
		{"walk", false, Vec3{1, 0, 0}, true, false},
		{"teleport", false, Vec3{50, 0, 0}, false, true},
		{"fall too fast", false, Vec3{0, 30, 0}, false, true},
		{"teleport in test mode", true, Vec3{50, 0, 0}, true, false},
		{"NaN", false, Vec3{math.NaN(), 0, 0}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer()
			s.Test = tc.test
			p := s.join("1.1.1.1")
			p.Team = content.Vibe
			p.Pos = Vec3{}
			p.lastUpdate = s.clock()
			s.handle(p, inbound{T: "state", Pos: tc.to, Yaw: 1, Pitch: 5, C: true})
			if moved := p.Pos == tc.to; moved != tc.moved {
				t.Fatalf("moved=%v pos=%v", moved, p.Pos)
			}
			if tc.moved && (p.Pitch != 1.6 || p.Yaw != 1 || !p.Crouch) {
				t.Fatalf("pitch %v (want clamped 1.6), yaw %v, crouch %v", p.Pitch, p.Yaw, p.Crouch)
			}
			if got := len(ofType(p, "correct")) == 1; got != tc.correct {
				t.Fatalf("correct sent=%v", got)
			}
		})
	}
}

func TestStateIgnoredForCommandDrivenPlayers(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	p.Team = content.Vibe
	s.queueCmds(p, []Cmd{{Seq: 1}})
	pos := p.Pos
	s.handle(p, inbound{T: "state", Pos: Vec3{pos[0] + 1, pos[1], pos[2]}})
	if p.Pos != pos {
		t.Fatal("a legacy state moved a command-driven player")
	}
}

func TestTestOnlyMessages(t *testing.T) {
	for _, test := range []bool{false, true} {
		s := authServer(t)
		s.Test = test
		p := s.join("1.1.1.1")
		p.Team = content.Vibe
		s.handle(p, inbound{T: "tp", Pos: Vec3{3, 4, 5}, Yaw: 0.5, C: true, Seq: 9})
		s.handle(p, inbound{T: "paced", C: true})
		if test {
			if p.body == nil || p.body.Pos != [3]float64{3, 4, 5} || !p.body.Crouched || p.ackSeq != 9 || p.lastQueued != 9 || !p.paced {
				t.Fatalf("tp in test mode: body %+v ack %d queued %d paced %v", p.body, p.ackSeq, p.lastQueued, p.paced)
			}
			if p.lastCmd.B != BtnCrouch || p.Pos != (Vec3{3, 4, 5}) || !p.Crouch {
				t.Fatalf("tp crouch state: lastCmd %+v pos %v", p.lastCmd, p.Pos)
			}
			s.handle(p, inbound{T: "tp", Pos: Vec3{math.Inf(1), 0, 0}})
			if p.Pos != (Vec3{3, 4, 5}) {
				t.Fatal("tp to infinity accepted")
			}
		} else if p.body != nil || p.paced {
			t.Fatal("test-only messages worked outside test mode")
		}
	}
}

// Legacy clients (no commands) send shots; the server checks fire timing and
// ray-traces bots only, replying with what was hit.
func TestLegacyShotHitsBots(t *testing.T) {
	s := authServer(t)
	shooter, other := s.join("1.1.1.1"), s.join("2.2.2.2")
	shooter.Team, other.Team = content.Vibe, content.Artisan
	shooter.Pos = Vec3{-14, 0, 6}
	bot := s.addBot()
	bot.Pos, bot.Yaw = Vec3{-14, 0, 0}, 0
	eye := Vec3{-14, 1.4, 6}
	chest := Vec3{0, 0.9 - 1.4, -6}
	l := math.Sqrt(chest[1]*chest[1] + 36)
	dir := Vec3{0, chest[1] / l, chest[2] / l}

	s.handle(shooter, inbound{T: "shot", O: eye, D: dir})
	hits := ofType(shooter, "hit")
	if len(hits) != 1 || hits[0]["id"] != float64(bot.ID) || hits[0]["part"] != "body" || hits[0]["dmg"] != float64(registry(t).Get("prompt").Damage) {
		t.Fatalf("hits %v", hits)
	}
	if shots := ofType(other, "shot"); len(shots) != 1 || shots[0]["id"] != float64(shooter.ID) {
		t.Fatalf("other player heard %v", shots)
	}

	for _, tc := range []struct {
		name string
		o, d Vec3
	}{
		{"origin far from eye", Vec3{-14, 1.4, 20}, dir},
		{"direction not unit", eye, Vec3{0, 0, -5}},
		{"NaN direction", eye, Vec3{math.NaN(), 0, -1}},
		{"aimed away", eye, Vec3{0, 0, 1}},
	} {
		shooter.inv.nextShot = 0
		s.handle(shooter, inbound{T: "shot", O: tc.o, D: tc.d})
		if h := ofType(shooter, "hit"); len(h) != 0 {
			t.Errorf("%s: hit %v", tc.name, h)
		}
	}

	// Firing again at once is too fast: not believed, nothing relayed.
	shooter.inv.nextShot = 0
	s.handle(shooter, inbound{T: "shot", O: eye, D: dir})
	msgs(other)
	msgs(shooter)
	s.handle(shooter, inbound{T: "shot", O: eye, D: dir})
	if len(ofType(other, "shot")) != 0 || len(ofType(shooter, "hit")) != 0 {
		t.Fatal("a shot inside the fire interval was believed")
	}

	// Spectators and command-driven players can't send shots.
	spec := s.join("3.3.3.3")
	s.handle(spec, inbound{T: "shot", O: eye, D: dir})
	if len(ofType(other, "shot")) != 0 {
		t.Fatal("spectator shot relayed")
	}
	// A bot on the shooter's team is skipped with friendly fire off.
	bot.Team = content.Vibe
	shooter.inv.nextShot = 0
	s.handle(shooter, inbound{T: "shot", O: eye, D: dir})
	if h := ofType(shooter, "hit"); len(h) != 0 {
		t.Fatalf("team bot hit with ff off: %v", h)
	}
}

func TestHandleDropBotAndAdminMessages(t *testing.T) {
	s := authServer(t)
	s.AdminPassword = "pw"
	p := s.join("1.1.1.1")
	s.handle(p, inbound{T: "drop"})
	if p.inv.Slots["primary"] != nil || len(s.items) != 1 {
		t.Fatal("drop message didn't drop")
	}
	s.handle(p, inbound{T: "bot", Cmd: "add"})
	if n := countBots(s); n != 0 {
		t.Fatal("non-admin added a bot")
	}
	s.handle(p, inbound{T: "admin_login", Pw: "pw"})
	if !p.admin {
		t.Fatal("login failed")
	}
	s.handle(p, inbound{T: "bot", Cmd: "add"})
	s.handle(p, inbound{T: "rcon", Cmd: "SV_GRAVITY", Args: []string{"9"}})
	if countBots(s) != 1 || s.cvars["sv_gravity"] != "9" {
		t.Fatalf("bots %d gravity %s", countBots(s), s.cvars["sv_gravity"])
	}
	msgs(p)
	s.handle(p, inbound{T: "admin_logout"})
	if p.admin || !strings.Contains(ofType(p, "print")[0]["text"].(string), "logout") {
		t.Fatal("logout")
	}
}

func countBots(s *Server) int {
	n := 0
	for _, p := range s.players {
		if p.Bot {
			n++
		}
	}
	return n
}

func TestBroadcastSendsSnapshotsWithSelfState(t *testing.T) {
	s := authServer(t)
	s.broadcast() // no players: nothing to do
	a, b, hidden := s.join("1.1.1.1"), s.join("2.2.2.2"), s.join("3.3.3.3")
	a.ready, b.ready = true, true
	a.Team, b.Team = content.Vibe, content.Artisan
	a.HP = 77
	s.queueCmds(a, []Cmd{{Seq: 1, F: 1}, {Seq: 2, F: 1}, {Seq: 3, F: 1}})
	stepSim(s) // runs one command: ack 1, two waiting
	bot := s.addBot()
	s.broadcastFull()

	snapOf := func(p *Player) map[string]any {
		t.Helper()
		ss := ofType(p, "snap")
		if len(ss) != 1 {
			t.Fatalf("player %d got %d snapshots", p.ID, len(ss))
		}
		return ss[0]
	}
	sa := snapOf(a)
	ids := map[float64]map[string]any{}
	for _, x := range sa["players"].([]any) {
		m := x.(map[string]any)
		ids[m["id"].(float64)] = m
	}
	if len(ids) != 3 || ids[float64(hidden.ID)] != nil || ids[float64(bot.ID)]["bot"] != true {
		t.Fatalf("players in snapshot: %v", ids)
	}
	if pa := ids[float64(a.ID)]; pa["w"] != "prompt" || pa["ammo"] != 30.0 || pa["res"] != 90.0 || pa["team"] != content.Vibe {
		t.Fatalf("a in snapshot: %v", pa)
	}
	me, ok := sa["me"].(map[string]any)
	if !ok || me["ack"] != 1.0 || me["q"] != 2.0 || me["hp"] != 77.0 || me["g"] == nil {
		t.Fatalf("me: %v", sa["me"])
	}
	if me["p"].([]any)[2].(float64) != a.body.Pos[2] {
		t.Fatalf("me.p %v not the exact body position %v", me["p"], a.body.Pos)
	}
	if _, has := snapOf(b)["me"]; has {
		t.Fatal("legacy player got a me state")
	}
	snapOf(hidden) // not ready players still get snapshots (they just aren't in them)
	if sa["tick"] != float64(s.tick) {
		t.Fatalf("tick %v want %d", sa["tick"], s.tick)
	}
}

// End to end over a real websocket: welcome, motd, hello/ping, snapshots from
// Run, and leaving on disconnect.
func TestWebsocketSession(t *testing.T) {
	s := authServer(t)
	s.Bots = 1
	s.Version, s.Build = "v1", "b1"
	stop := make(chan struct{})
	defer close(stop)
	go s.Run(stop)
	hs := httptest.NewServer(http.HandlerFunc(s.HandleWS))
	defer hs.Close()
	url := "ws" + strings.TrimPrefix(hs.URL, "http")

	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	read := func() map[string]any {
		t.Helper()
		var m map[string]any
		if err := c.ReadJSON(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	w := read()
	if w["t"] != "welcome" || w["version"] != "v1" || w["build"] != "b1" || w["simHz"] != float64(SimHz) || w["team"] != content.Spectator {
		t.Fatalf("welcome %v", w)
	}
	if m := w["map"].(map[string]any); m["name"] != "de_slop" {
		t.Fatalf("welcome map %v", m["name"])
	}
	want := []string{"inv", "cvars", "print"}
	for _, typ := range want {
		if m := read(); m["t"] != typ {
			t.Fatalf("got %v, want %s", m, typ)
		}
	}
	_ = c.WriteMessage(websocket.TextMessage, []byte("not json")) // ignored
	_ = c.WriteJSON(map[string]any{"t": "hello", "name": "wsplayer"})
	_ = c.WriteJSON(map[string]any{"t": "ping", "seq": 5})
	var snap map[string]any
	gotPong := false
	for snap == nil || !gotPong {
		m := read()
		switch m["t"] {
		case "pong":
			gotPong = m["seq"] == 5.0
		case "snap":
			if gotPong && m["pose"] == nil { // a full one (pose snapshots carry the poses only)
				snap = m
			}
		}
	}
	names := map[string]bool{}
	for _, x := range snap["players"].([]any) {
		names[x.(map[string]any)["name"].(string)] = true
	}
	if !names["wsplayer"] || len(names) != 2 {
		t.Fatalf("snapshot players %v, want wsplayer and a bot", names)
	}
	if items := snap["items"].([]any); len(items) != 1 || items[0].(map[string]any)["w"] != "codereview" {
		t.Fatalf("map item not placed: %v", snap["items"])
	}

	s.mu.Lock()
	if n := len(s.players); n != 2 {
		t.Fatalf("%d players on the server", n)
	}
	var me *Player
	for _, p := range s.players {
		if !p.Bot {
			me = p
		}
	}
	if me.bytesIn == 0 || me.bytesOut == 0 {
		t.Fatalf("traffic not counted: in %d out %d", me.bytesIn, me.bytesOut)
	}
	s.mu.Unlock()

	c.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		n := len(s.players)
		s.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("player not removed after disconnect")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWebsocketKickAndBan(t *testing.T) {
	s := authServer(t)
	hs := httptest.NewServer(http.HandlerFunc(s.HandleWS))
	defer hs.Close()
	url := "ws" + strings.TrimPrefix(hs.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var m map[string]any
	_ = c.ReadJSON(&m) // welcome
	admin := s.join("9.9.9.9")
	s.mu.Lock()
	var p *Player
	for _, o := range s.players {
		if o != admin {
			p = o
		}
	}
	s.adminCommand(admin, "ban", []string{"#" + strconv.Itoa(p.ID)})
	s.mu.Unlock()
	kicked := false
	for {
		var m map[string]any
		if err := c.ReadJSON(&m); err != nil {
			break // the server closed the socket
		}
		kicked = kicked || m["t"] == "kicked"
	}
	if !kicked {
		t.Fatal("no kicked message before the close")
	}
	// The banned IP (127.0.0.1) can't reconnect.
	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("banned reconnect: err %v resp %v", err, resp)
	}
	// A plain HTTP request (no upgrade) fails the upgrade and returns.
	delete(s.bans, "127.0.0.1")
	r, err := http.Get(hs.URL)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-websocket request: %d", r.StatusCode)
	}
}

// A 10-player snapshot is ~2 KB of JSON 33 times a second: browsers offer
// permessage-deflate and the server must accept it (and still talk JSON).
func TestWebsocketNegotiatesCompression(t *testing.T) {
	s := authServer(t)
	stop := make(chan struct{})
	defer close(stop)
	go s.Run(stop)
	hs := httptest.NewServer(http.HandlerFunc(s.HandleWS))
	defer hs.Close()
	d := websocket.Dialer{EnableCompression: true}
	c, resp, err := d.Dial("ws"+strings.TrimPrefix(hs.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if ext := resp.Header.Get("Sec-WebSocket-Extensions"); !strings.Contains(ext, "permessage-deflate") {
		t.Fatalf("no compression negotiated (extensions %q)", ext)
	}
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, b, err := c.ReadMessage()
	if err != nil || !strings.Contains(string(b), `"t":"welcome"`) {
		t.Fatalf("first message over a compressed socket: %q %v", b, err)
	}
}

// Every player's ping reaches everyone's scoreboard as state: b's reported
// round trips (smoothed) are in a's snapshot; bad reports and test barriers
// (no rtt) change nothing.
func TestPingInSnapshot(t *testing.T) {
	s := NewServer()
	a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
	a.ready, b.ready = true, true
	s.handle(b, inbound{T: "ping", Seq: 1, Rtt: 40.4})
	s.handle(b, inbound{T: "ping", Seq: -1})                 // barrier: no rtt
	s.handle(b, inbound{T: "ping", Seq: 2, Rtt: math.NaN()}) // ignored
	s.handle(b, inbound{T: "ping", Seq: 3, Rtt: 1e9})        // ignored
	if b.Ping != 40 {
		t.Fatalf("first report: ping %d, want 40", b.Ping)
	}
	s.handle(b, inbound{T: "ping", Seq: 4, Rtt: 80})
	if b.Ping != 50 { // (3*40 + 80) / 4
		t.Fatalf("smoothed: ping %d, want 50", b.Ping)
	}
	s.handle(b, inbound{T: "ping", Seq: 5, Rtt: 0.2}) // a fast LAN: at least 1 ms
	msgs(a)
	s.broadcastFull()
	ss := ofType(a, "snap")
	if len(ss) != 1 {
		t.Fatalf("%d snapshots", len(ss))
	}
	for _, x := range ss[0]["players"].([]any) {
		m := x.(map[string]any)
		if m["id"] == float64(b.ID) {
			if m["ping"] != 38.0 { // (3*50 + 1 + 2) / 4
				t.Fatalf("b's ping in a's snapshot: %v", m)
			}
			return
		}
	}
	t.Fatal("b not in a's snapshot")
}

// A page that connects while lockstep is off (booting) and then goes quiet
// once it is on (its keepalive runs on the virtual clock, stopped between
// harness steps) keeps its socket: SetLockstep moves the deadline of the read
// already waiting. It used to keep the idle limit it got when it connected:
// on a loaded CI box sim scenarios lost players (name-tags, teams, team-cap).
func TestLockstepKeepsQuietConnections(t *testing.T) {
	s := authServer(t)
	s.idleAfter = 300 * time.Millisecond
	hs := httptest.NewServer(http.HandlerFunc(s.HandleWS))
	defer hs.Close()
	url := "ws" + strings.TrimPrefix(hs.URL, "http")
	connected := func() int {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.players)
	}
	dial := func() *websocket.Conn {
		c, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(2 * time.Second); connected() == 0; time.Sleep(time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal("never connected")
			}
		}
		return c
	}
	// Outside lockstep a silent client is dropped after the idle limit.
	c := dial()
	for deadline := time.Now().Add(2 * time.Second); connected() > 0; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("a silent client outside lockstep was never dropped")
		}
	}
	c.Close()
	// Connected before lockstep, silent after it is on: kept.
	c = dial()
	defer c.Close()
	time.Sleep(s.idleAfter / 2) // its first read is waiting, with the deadline it got then
	s.SetLockstep(true)
	defer s.SetLockstep(false)
	time.Sleep(4 * s.idleAfter)
	if connected() != 1 {
		t.Fatal("lockstep dropped a client that connected before it was on")
	}
}

// Redeploy (a deploy, for the test harness): the announced build changes and
// every connection drops without a kick notice, so clients reconnect and meet
// the new build in their welcome.
func TestRedeployDropsConnections(t *testing.T) {
	s := authServer(t)
	s.Version, s.Build = "v1", "b1"
	stop := make(chan struct{})
	defer close(stop)
	go s.Run(stop)
	hs := httptest.NewServer(http.HandlerFunc(s.HandleWS))
	defer hs.Close()
	url := "ws" + strings.TrimPrefix(hs.URL, "http")
	dial := func() (*websocket.Conn, map[string]any) {
		t.Helper()
		c, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		var w map[string]any
		if err := c.ReadJSON(&w); err != nil || w["t"] != "welcome" {
			t.Fatalf("welcome: %v %v", err, w)
		}
		return c, w
	}
	c, w := dial()
	defer c.Close()
	if w["build"] != "b1" {
		t.Fatalf("build %v", w["build"])
	}
	s.Redeploy("b2")
	for {
		var m map[string]any
		if err := c.ReadJSON(&m); err != nil {
			break // dropped
		}
		if m["t"] == "kicked" {
			t.Fatal("a redeploy must not look like a kick (the client wouldn't reconnect)")
		}
	}
	c2, w2 := dial()
	defer c2.Close()
	if w2["build"] != "b2" {
		t.Fatalf("after redeploy: build %v", w2["build"])
	}
}

// Between full snapshots (every fullEvery ticks) the server sends pose
// snapshots: each player's [id, x, y, z, yaw, pitch, flags] (1 crouched, 2 not
// a body) and the client's own full state, nothing else.
func TestPoseSnapshot(t *testing.T) {
	s := authServer(t)
	a, b, c := s.join("1.1.1.1"), s.join("2.2.2.2"), s.join("3.3.3.3")
	for _, p := range []*Player{a, b, c} {
		p.ready, p.Team = true, content.Vibe
		s.respawn(p)
		s.teleport(p, p.Pos, 0, false, 0) // a body: its full state goes in every snapshot
		p.inv = s.Weapons.Loadout(p.Team)
		drain(p)
	}
	b.Crouch, b.Pos, b.Yaw = true, Vec3{1.2345, 0.5, -2}, 1.23456
	c.Dead = true
	s.tick = 1
	s.broadcast()
	ss := ofType(a, "snap")
	if len(ss) != 1 || ss[0]["pose"] != 1.0 {
		t.Fatalf("tick 1: want one pose snapshot, got %v", ss)
	}
	m := ss[0]
	for _, k := range []string{"items", "round", "nades"} {
		if _, ok := m[k]; ok {
			t.Errorf("a pose snapshot carries %s", k)
		}
	}
	if me, ok := m["me"].(map[string]any); !ok || me["ack"] == nil || me["w"] == nil {
		t.Errorf("a pose snapshot must carry the full self-state: %v", m["me"])
	}
	rows := map[float64][]any{}
	for _, x := range m["players"].([]any) {
		r := x.([]any)
		rows[r[0].(float64)] = r
	}
	if r := rows[float64(b.ID)]; fmt.Sprint(r[1:]) != "[1.235 0.5 -2 1.2346 0 1]" {
		t.Errorf("crouched b: %v", r)
	}
	if r := rows[float64(c.ID)]; r[6] != 2.0 {
		t.Errorf("dead c: flags %v, want 2 (not a body)", r[6])
	}
	s.tick = fullEvery
	s.broadcast()
	ss = ofType(a, "snap")
	if len(ss) != 1 {
		t.Fatalf("tick %d: %d snapshots", fullEvery, len(ss))
	}
	if _, items := ss[0]["items"]; ss[0]["pose"] != nil || !items {
		t.Fatalf("tick %d: want a full snapshot, got %v", fullEvery, ss)
	}
}
