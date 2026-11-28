// Package game is the authoritative-ish game server: it accepts websocket
// players, relays their movement to everyone at a fixed tick and keeps
// obviously impossible movement in check.
package game

import (
	"compress/flate"
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
	"sync/atomic"
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
		b = strconv.AppendFloat(b, sim.RoundMM(f), 'f', -1, 64) // = the hulls players collide with (sim.PeerBox)
	}
	return append(b, ']'), nil
}

// Angle is a view angle in radians, sent with 4 decimals (0.006°: invisible
// on screen, a third of a float64's bytes). Hit tests use the commands' own
// exact angles, never these.
type Angle float64

func (a Angle) MarshalJSON() ([]byte, error) {
	return strconv.AppendFloat(nil, math.Round(float64(a)*1e4)/1e4, 'f', -1, 64), nil
}

// MarshalJSON sends the view angles with 4 decimals (the outer fields shadow
// the embedded ones of the same name).
func (p *Player) MarshalJSON() ([]byte, error) {
	type plain Player
	return json.Marshal(struct {
		*plain
		Yaw   Angle `json:"yaw"`
		Pitch Angle `json:"pitch"`
	}{(*plain)(p), Angle(p.Yaw), Angle(p.Pitch)})
}

type Player struct {
	conn     *websocket.Conn `net:"-"` // its socket (nil: a bot); SetLockstep moves its read deadline
	slowRead SlowRead        `net:"-"` // test harness: some of its command packets read late (harness_slowread.go)
	ID       int             `json:"id" net:"cold"`
	Name     string          `json:"name" net:"cold"`
	Color    string          `json:"color" net:"cold"`
	Pos      Vec3            `json:"p" net:"hot,mm"`
	Yaw      float64         `json:"yaw" net:"hot,angle"`
	Pitch    float64         `json:"pitch" net:"hot,angle"`
	// Crouch is relayed for rendering and hit tests (hitbox 1.2 instead of 1.8).
	// It doesn't change the movement sanity checks: a crouch-jump lifts the feet
	// 0.6 instantly, well inside MaxFallSpeed's per-update budget.
	Crouch bool `json:"c" net:"hot"`
	// Gait is the walk cycle (gait_shared.go), stepped by every command and
	// replicated so clients draw, and the server hits, the same pose.
	Gait Gait `json:"g" net:"hot,gait"`
	// Grav: the gravity the player stands in (sim.Grav*: the Gravity Override
	// boots); drawn, collided with and hit turned with it.
	Grav int `json:"gv,omitempty" net:"hot"`
	// Frame: the mover whose frame of reference it rides in (1-based; 0: the
	// world; sim.FrameOf: a ship's cabin, a lift's top) and Local its place in
	// that frame (sim.ToFrameLocal at its body's time). Clients interpolate a
	// rider in its frame and place it where they draw that mover; lag
	// compensation does the same (docs/physics.md "Frames of reference").
	Frame int   `json:"fr,omitempty" net:"hot"`
	Local *Vec3 `json:"fl,omitempty" net:"hot,mm"`
	// Hook: where its Gravity Override hook is latched (nil: not hooked): the rope.
	Hook    *Vec3  `json:"hk,omitempty" net:"hot,mm"`
	Bot     bool   `json:"bot,omitempty" net:"cold"` // target practice: invincible, not in the match
	AI      bool   `json:"ai,omitempty" net:"cold"`  // a playing bot (ai.go): a real player driven by the server
	Weapon  string `json:"w,omitempty" net:"cold"`   // id of the weapon in hand, for remote viewmodels
	Ammo    int    `json:"ammo" net:"cold"`          // in the weapon in hand (spectators' first-person HUD)
	Reserve int    `json:"res" net:"cold"`
	HP      int    `json:"hp" net:"cold"`
	Dead    bool   `json:"dead,omitempty" net:"cold"`
	// The corpse (client visuals, web/game/corpse.js): the server tick of the
	// death and the way the body falls (a yaw: away from the killer, else
	// backwards). Carried in every snapshot, so a client that joins late
	// draws it already lying down.
	DeathTick int64   `json:"dk,omitempty" net:"cold"`
	FallYaw   float64 `json:"fy,omitempty" net:"cold,exact"`
	// Vanish: the body goes at once (warmup, timed respawns) instead of lying
	// there for the rest of the round.
	Vanish  bool   `json:"dv,omitempty" net:"cold"`
	Kills   int    `json:"k" net:"cold"`
	Deaths  int    `json:"d" net:"cold"`
	Assists int    `json:"a,omitempty" net:"cold"`
	Points  int    `json:"sp,omitempty" net:"cold"` // score points (match.go): kills, assists, plants, defuses
	Team    string `json:"team" net:"cold"`
	// Ping: the round trip in ms this client reports with each keepalive
	// (smoothed here), shown on everyone's scoreboard. 0 = not measured yet
	// (bots). Self-reported: a liar only fakes a number on the scoreboard.
	Ping int `json:"ping,omitempty" net:"cold"`

	// spawnSeq counts server-side respawns (team switches, deaths). Legacy
	// states the client sent before it saw the newest spawn carry an older
	// seq and are dropped, so they can't drag the player back.
	spawnSeq int `net:"-"`
	// teamSeq orders team joins: a smaller map moves the latest joiners to
	// spectators first (teamcap.go).
	teamSeq int `net:"-"`

	lastUpdate time.Time   `net:"-"`
	send       chan []byte `net:"-"` // nil for bots (trySend drops)
	ready      bool        `net:"-"` // said hello; hidden from snapshots until then
	// Test mode (noteContact): when Pos last changed, and when p last touched a
	// moving player.
	lastPos                Vec3          `net:"-"`
	movedTick, contactTick int64         `net:"-"`
	helloSeq               int           `net:"-"` // order of the first hello (page loads connect before PLAY, so IDs aren't join order)
	brain                  *botBrain     `net:"-"`
	ai                     *aiBrain      `net:"-"` // playing bots (ai.go)
	chatTokens             float64       `net:"-"` // chat/radio rate limit (chat.go)
	chatAt                 time.Time     `net:"-"`
	inv                    *Inventory    `net:"-"`
	admin                  bool          `net:"-"` // logged in with the admin password
	ip                     string        `net:"-"` // for bans, rate limits and admin status
	kick                   chan struct{} `net:"-"` // closed to drop the connection

	// Authoritative sim (once the client sends commands; before that the
	// legacy client-moved path applies).
	body          *sim.Body        `net:"-"`
	cmds          []Cmd            `net:"-"`
	lastQueued    int              `net:"-"` // highest command seq received
	ackSeq        int              `net:"-"` // last command applied (sent back in snapshots)
	lastCmd       Cmd              `net:"-"`
	cmdTicks      int64            `net:"-"`          // commands applied: the clock weapon timers run on
	nadeGone      map[string]int64 `net:"-"`          // mp_nade_refill: the server tick each grenade slot went empty (nades.go)
	tickBase      int64            `net:"-"`          // command seq N runs at tick tickBase+N on the movers' clock (cmdTime)
	cmdBudget     float64          `net:"-"`          // commands this player may still run (see simTick)
	backlogSince  int64            `net:"-"`          // tick its queue went above backlogTarget (0: not; trimBacklog)
	Money         int              `json:"-" net:"-"` // server-owned; in the self-snapshot only
	Armor         int              `json:"-" net:"-"` // 0..100 (Senior-dev hoodie)
	Helmet        bool             `json:"-" net:"-"` // the self-snapshot only (meState)
	Kit           bool             `json:"-" net:"-"` // Rollback Kit: defuse in half the time
	Shades        bool             `json:"-" net:"-"` // Senior-dev Sunglasses: shorter flash blindness
	Boots         bool             `json:"-" net:"-"` // Gravity Override (bought): every body this player gets wears it (fitBoots); the body's Boots is the self-snapshot's bt
	cmdsDropped   int              `net:"-"`          // commands dropped on queue overflow (cl_netdebug)
	skipped       int              `net:"-"`          // commands dropped or trimmed (acknowledged, not run) since the last one run: coast steps the body through them
	paced         bool             `net:"-"`          // test mode: this player gets the real command budget (harness pacing tests)
	respawnAt     time.Time        `net:"-"`
	spawnPick     string           `net:"-"` // tickets mode: the chosen spawn point id ("" auto; tickets.go)
	scanned       bool             `net:"-"` // tickets mode: a ticket gate scanned this life's ticket (tickets.go tickGates)
	kit           string           `net:"-"` // tickets mode: the chosen kit id ("" the default)
	bytesOut      int64            `net:"-"`
	delta         deltaConn        `net:"-"` // the delta codec's state for this connection (netdelta_codec.go)
	snapSent      int64            `net:"-"` // the tick of the last snapshot queued to this connection (0: none; SnapSent)
	bytesIn       int64            `net:"-"`
	joined        time.Time        `net:"-"` // when it joined, on the game clock (s.clock()): rates, who joined last
	activeRound   int              `net:"-"` // the last round with input from this player (afk.go)
	playedFrom    int              `net:"-"` // the round this player spawned in at its start
	idleRounds    int              `net:"-"` // rounds in a row played without input
	lastLook      [2]float64       `net:"-"` // the last command's yaw and pitch
	noPickupUntil time.Time        `net:"-"`
	flashUntil    time.Time        `net:"-"` // flashbanged until (Stack Trace)
	flashDur      float64          `net:"-"` // the blindness's full length, seconds
	burn          float64          `net:"-"` // fire damage owed, not yet dealt (fire.go: dealt in chunks)
	puppet        bool             `net:"-"` // driven through the MCP (puppet_add/drive): no connection
	ride          int              `net:"-"` // the boat it rides (Boat.ID; 0: none), in seat (boats.go): sent in its meState
	seat          int              `net:"-"` // SeatDeck: standing on its deck, at deck
	deck          sim.Deck         `net:"-"` // where on the boat it stands (seat SeatDeck), in the boat's frame
	leftBoat      int64            `net:"-"` // the command (cmdTicks) it got out of a boat: that E press boards nothing
	car           int              `net:"-"` // the car it drives (Car.ID; 0: none; cars.go): sent in its meState
	leftCar       int64            `net:"-"` // the command (cmdTicks) it got out of a car: that E press boards nothing
	// The pit stop (pitstop.go): in the pit (car parked, driver held), whether
	// they went in alive, and the client's newest wish ({t:"pit"}: in or out, its seq).
	pit, pitAlive, pitWant bool    `net:"-"`
	pitSeq                 int     `net:"-"`
	garage                 *garage `net:"-"` // the race shop's: its car and upgrades, kept for the match (carshop.go)
	script                 []Cmd   `net:"-"` // a puppet's commands still to run, one a tick (tickPuppets)
}

type Server struct {
	customSrc []byte       // the custom map being played, as loaded (MapsReloaded); nil: a built-in map
	edit      *editSession // the map being edited (mapedit.go); nil: none
	MapsDir   string       // where map_save writes (--maps-dir); "" = map_save refuses
	// OnMapSaved runs after map_save wrote a map (the app reloads the custom
	// maps from MapsDir). Called without s.mu held.
	OnMapSaved func()
	prof       tickProf      // sv_prof: where the tick's time goes (tickprof.go); prof.now: a test's clock
	hellos     int           // first hellos so far (Player.helloSeq)
	idleAfter  time.Duration // idleLimit outside lockstep (0 = idleTimeout; tests shorten it)
	worldCache struct {
		w      *sim.World
		boxes  []content.Box
		movers []content.Mover
	} // simWorld: the movement world of the current map
	// rng: the server's own randomness (bot wander, Big Diff carrier, team
	// tie-break). Seeded at random; tests seed it (Seed) so a match replays.
	rng        *rand.Rand
	Economy    *Economy       // web/assets/economy.json (nil = DefaultEconomy)
	lossStreak map[string]int // consecutive round losses per team (loss bonus)
	lastTickAt time.Time      // when simTick last ran (command budgets)
	started    time.Time      // when Run started (clock before the first tick); zero in tests
	history    History        // poses sent in recent snapshots (lag compensation)
	Map        content.Map
	navAuto    bool     // Map.Nav was built from the geometry (autoNav)
	islands    struct { // navIslands' cache (ai_board.go)
		of   *content.NavNode
		comp []int
	}
	bounds struct { // outOfWorld's cache of MapBounds(Map.Boxes) and the movers' sweeps
		of     *content.Box
		n, mv  int // boxes, movers (their sweeps count too)
		lo, hi [3]float64
	}
	Version    string      // git SHA of this binary, for display
	Build      string      // hash of the client assets; clients reload when theirs differs
	Test       bool        // test harness mode: no speed checks, so scenarios can teleport
	codec      snapCodec   // how snapshots go on the wire (netcodec.go; nil = json)
	tv         *tvRecorder // the replay recorder (tv.go; nil = off)
	mcpS       *mcpState   // the MCP's events and snapshots (mcp_tools.go; nil = no MCP)
	lockstep   bool        // test harness: Run's ticker pauses; the harness Steps the server
	load       *loadState  // a map load under way (maploading.go; nil: none)
	loadSeq    int         // the last load's id
	navHold    int64       // lockstep: ticks the next load's nav step lasts at least (HoldNav)
	lockstepOn atomic.Bool // lockstep, readable without s.mu (socket reads)
	lastSnap   int64       // lockstep: the tick of the last snapshot Step broadcast (SnapTick)
	// Bots is how many target-practice bots start with the server (admins add more).
	Bots int
	// AdminPassword unlocks admin commands; empty means everyone is admin.
	AdminPassword string
	// Weapons is the shared registry (web/assets/weapons/weapons.json).
	Weapons   *Registry
	spawnNext map[string]int
	placing   map[*Player]bool // about to be respawned together: their old spot doesn't take a spawn (teamcap.go)
	teamSeqN  int              // team joins so far (Player.teamSeq)
	round     roundState
	now       time.Time // the current tick's time (simTick)
	worked    bool      // roundUse: this command advanced a plant/defuse

	mu      sync.Mutex
	nextID  int
	players map[int]*Player

	items       []*Item
	nextItem    int
	itemRespawn map[int]time.Time    // map item index -> when it comes back
	bests       map[string][]float64 // parkour: each player's best run's splits by course (race.go bestKey)

	vote *mapVote // the end-of-match map vote, while one is open
	// sidesSwapped: the players are on the other side since a halftime (until the next match).
	sidesSwapped bool
	warmupEnd    bool // admin mp_warmup_end: start the match on the next tick

	dmgLedger  roundLedger                 // this round's hits and kills (mvp.go)
	aiRaceLine *raceLine                   // the map's racing line, sampled (ai_race.go raceLineOf)
	aiPeace    bool                        // tests: racing bots drive and never fire (ai_race.go)
	aiIntel    map[string]aiIntel          // per team: the latest enemy a bot saw (ai_defend.go)
	aiEntrance map[string]Vec3             // "map/site": where the attackers' route enters it
	aiHold     map[string]Vec3             // "map/site": where its defenders stand
	aiSpots    map[string][]Vec3           // "map/site": its defenders' hold spots (ai_defmix.go)
	aiForward  map[string][2]Vec3          // "map/site": a forward spot and where it looks (ai_defmix.go)
	aiDefense  aiDefPlan                   // the defenders' plan this round (ai_defmix.go)
	aiAttDist  map[string]map[Vec3]float64 // per map: the attackers' walk to each nav node
	aiGPUWatch map[string]Vec3             // per map and GPU spot: where its guard looks (ai_gpudrop.go)
	aiPosts    map[string][]Vec3           // per map and GPU spot: the staff's forward posts (ai_gpudrop.go)
	gridCache  map[gridKey]gridResult      // last legs off the nav graph (ai_grid.go)
	plantCheck plantCheck                  // the last plant spot's defusable answer (plant_reach.go)
	aiAttack   struct {                    // the attackers' site this round (pickSite)
		round int
		site  string
	}

	nades     []*Nade     // grenades in flight and standing smokes
	boats     []*Boat     // the map's boats (boats.go)
	cars      []*Car      // the map's cars (cars.go)
	gear      []*carGear  // the cars' live shots and drops (carcombat.go)
	deltaPrev *deltaWorld // the delta codec's last tick's world: the next one shares its unchanged parts (shareNV)
	fullsNow  bool        // test only: this snapshot is a full for every connection (SendSetupSnapshot)
	gearSeq   int
	nextNade  int

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
		bests:       map[string][]float64{},
		cvars:       copyCvars(),
		loginFails:  map[string]*loginFails{},
		bans:        map[string]time.Time{},
		spawnNext:   map[string]int{},
		round:       newRound(),
		rng:         rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// Per-message deflate (no context takeover): a 10-player snapshot
			// is ~2 KB of JSON at 33/s (~70 KB/s per client); deflate at
			// BestSpeed makes it ~0.7 KB in ~25 µs. Transport-level only.
			EnableCompression: true,
			CheckOrigin:       allowedOrigin,
		},
	}
}

// inbound is every client->server message; T selects which fields matter.
type inbound struct {
	Text     string   `json:"text"` // say
	TeamOnly bool     `json:"tm"`   // say_team
	Radio    string   `json:"rid"`  // radio callout id
	Item     string   `json:"item"` // buy
	T        string   `json:"t"`
	Name     string   `json:"name"`
	Pos      Vec3     `json:"p"`
	Yaw      float64  `json:"yaw"`
	Pitch    float64  `json:"pitch"`
	Seq      float64  `json:"seq"`
	C        bool     `json:"c"`
	Gv       *int     `json:"gv"`  // tp: the body's gravity (sim.Grav*), nil: as it was
	Bt       *bool    `json:"bt"`  // tp: wears the Gravity Override (a full meter), nil: as it was
	O        Vec3     `json:"o"`   // shot origin (eye)
	D        Vec3     `json:"d"`   // shot direction
	Cmd      string   `json:"cmd"` // bot command
	Arg      string   `json:"arg"`
	Slot     string   `json:"slot"` // switch
	Pw       string   `json:"pw"`   // admin_login
	Args     []string `json:"args"` // rcon
	Cmds     []Cmd    `json:"cmds"` // input commands
	Sa       int64    `json:"sa"`   // cmd, sack: the newest snapshot tick the client decoded (delta codec bases)
	Team     string   `json:"team"`
	Sp       int      `json:"sp"`
	Map      string   `json:"map"` // vote
	Rtt      float64  `json:"rtt"` // ping: the client's last measured round trip (ms)
	// editpresence: the map editor's aim point, hover, selection, tool (mapedit_presence.go)
	At    *[3]float64 `json:"at"`
	Hover *editRef    `json:"hover"`
	Sel   []editRef   `json:"sel"`
	Tool  string      `json:"tool"`
	// deploy (tickets mode): the spawn point id ("auto") and the kit id
	Pt  string `json:"pt"`
	Kit string `json:"kit"`
	// loading: a client's map load progress against load lid (maploading.go)
	LoadID int    `json:"lid"`
	Step   string `json:"step"`
	Pct    int    `json:"pct"`
}

type welcomeMsg struct {
	T       string      `json:"t" net:"cold"`
	ID      int         `json:"id" net:"cold,self"`
	Color   string      `json:"color" net:"cold,self"`
	Spawn   Vec3        `json:"spawn" net:"cold,self,mm"`
	Map     content.Map `json:"map" net:"cold"`
	Tick    float64     `json:"tick" net:"cold,exact"`
	SimHz   int         `json:"simHz" net:"cold"`
	Version string      `json:"version" net:"cold"`
	Build   string      `json:"build" net:"cold"`
	Team    string      `json:"team" net:"cold,self"`
	Codec   string      `json:"codec,omitempty" net:"cold"`  // the snapshot codec (netcodec.go); absent = json
	Schema  string      `json:"schema,omitempty" net:"cold"` // the delta codec's schema hash (netSchemaHash); the client refuses another
}

// meState is the recipient's own authoritative body in a snapshot.
type meState struct {
	Ack      int          `json:"ack" net:"hot,self"`
	TB       int64        `json:"tb" net:"cold,self"`     // tick base: command seq N runs at tick tb+N on the movers' clock
	Pos      [3]float64   `json:"p" net:"hot,self,exact"` // exact: the client replays from here
	Vel      [3]float64   `json:"v" net:"hot,self,exact"`
	OnGround bool         `json:"g" net:"hot,self"`
	Grav     int          `json:"gv,omitempty" net:"hot,self"`       // the body's gravity (sim.Body.Grav)
	Boots    bool         `json:"bt,omitempty" net:"cold,self"`      // wears the Gravity Override (sim.Body.Boots)
	Noclip   bool         `json:"nc,omitempty" net:"cold,self"`      // flies through walls (sim.Body.Noclip: editing the map)
	MapRev   int          `json:"mr,omitempty" net:"cold,self"`      // the map's edit revision (mapedit.go): behind it, the client asks for the map
	Energy   float64      `json:"en,omitempty" net:"hot,self,exact"` // its hook's energy
	Hook     *hookState   `json:"hk,omitempty" net:"hot,self"`       // its hook, latched (nil: not)
	Crouched bool         `json:"c" net:"hot,self"`
	HP       int          `json:"hp" net:"cold,self"`
	Dead     bool         `json:"dead,omitempty" net:"cold,self"`
	Queue    int          `json:"q" net:"hot,self"`               // commands waiting on the server (cl_netdebug)
	Dropped  int          `json:"drop,omitempty" net:"cold,self"` // commands dropped on overflow, cumulative
	Team     string       `json:"team" net:"cold,self"`
	Money    int          `json:"money" net:"cold,self"`
	Armor    int          `json:"armor" net:"cold,self"`
	Helmet   bool         `json:"helmet,omitempty" net:"cold,self"`
	Kit      bool         `json:"kit,omitempty" net:"cold,self"`
	Shades   bool         `json:"shades,omitempty" net:"cold,self"`
	Buy      bool         `json:"buy,omitempty" net:"cold,self"`       // may buy right now (in the buy zone, in buy time)
	Weapons  *weaponState `json:"w,omitempty" net:"hot,self"`          // the full weapon state after Ack (reconciled like the body)
	Flash    float64      `json:"fl,omitempty" net:"hot,self,exact"`   // flashbanged: seconds of blindness left
	FlashDur float64      `json:"fld,omitempty" net:"cold,self,exact"` // ... out of this many
	// Riding a boat (boats.go, ride_shared.go): its id (0: on foot), the seat
	// (0 drives), and the boat's exact state after Ack (the driver predicts it;
	// a passenger sits on it).
	Ride int        `json:"ride,omitempty" net:"hot,self"`
	Seat int        `json:"seat,omitempty" net:"hot,self"`
	Boat *boatExact `json:"rb,omitempty" net:"hot,self"`
	Deck *deckExact `json:"dk,omitempty" net:"hot,self"` // standing on the deck (seat SeatDeck): where, in the boat's frame
	// Driving a car (cars.go, car_shared.go): its id (0: on foot) and its
	// exact state after Ack (the driver predicts it).
	Car      int       `json:"car,omitempty" net:"hot,self"`
	CarState *carExact `json:"cb,omitempty" net:"hot,self"`
	CarLock  bool      `json:"cl,omitempty" net:"hot,self"`  // can't get out (a race is on: race.go)
	Garage   *garage   `json:"gar,omitempty" net:"hot,self"` // a race map: the car and upgrades bought (carshop.go)
	Pit      bool      `json:"pit,omitempty" net:"hot,self"` // in the pit stop (pitstop.go): car parked, out of the race until you leave the garage
	PitSeq   int       `json:"ps,omitempty" net:"hot,self"`  // the newest {t:"pit"} seq taken (the client resends while ahead)
	// Tickets mode (tickets.go): the chosen spawn point ("" auto) and kit,
	// the point the next spawn would use, and seconds until the respawn.
	SpawnPick string  `json:"spk,omitempty" net:"cold,self"`
	KitID     string  `json:"kt,omitempty" net:"cold,self"`
	SpawnAt   string  `json:"spAt,omitempty" net:"cold,self"`
	Respawn   float64 `json:"rsp,omitempty" net:"hot,self,exact"`
}

// deckExact is a body's place on a boat's deck (sim.Deck), exact.
type deckExact struct {
	Pos [3]float64 `json:"p" net:"hot,self,exact"`
	Vel [3]float64 `json:"v" net:"hot,self,exact"`
}

// boatExact is a boat's simulated state, exact (sim.Boat), for its riders' prediction.
type boatExact struct {
	Pos    [3]float64 `json:"p" net:"hot,self,exact"`
	Vel    [3]float64 `json:"v" net:"hot,self,exact"`
	Yaw    float64    `json:"y" net:"hot,self,exact"`
	YawVel float64    `json:"yv" net:"hot,self,exact"`
	Driven bool       `json:"rd,omitempty" net:"hot,self"` // someone is at the tiller (else a rider's client steps it with no input: it drifts, as tickBoats)
}

// hookState is a latched hook in the self-snapshot (sim.Hook): reconciled
// like the body.
type hookState struct {
	At   [3]float64 `json:"at" net:"hot,self,exact"`
	N    [3]float64 `json:"n" net:"hot,self,exact"`
	Lo   [3]float64 `json:"lo" net:"hot,self,exact"`
	Hi   [3]float64 `json:"hi" net:"hot,self,exact"`
	Flip bool       `json:"f,omitempty" net:"hot,self"`
	Rope float64    `json:"r" net:"hot,self,exact"`
	// Latched to a mover (sim.Hook.Mover: its index + 1; 0: a static box),
	// the tread (U) and the anchor from its origin (Local).
	Mover int        `json:"mv,omitempty" net:"hot,self"`
	U     float64    `json:"u,omitempty" net:"hot,self,exact"`
	Local [3]float64 `json:"lc" net:"hot,self,exact"`
}

// hookStateOf: h on the wire, nil when not latched.
func hookStateOf(h *sim.Hook) *hookState {
	if !h.On {
		return nil
	}
	return &hookState{At: h.At, N: h.N, Lo: h.Lo, Hi: h.Hi, Flip: h.Flip, Rope: h.Rope, Mover: h.Mover, U: h.U, Local: h.Local}
}

// meBytes is p's self-snapshot: the authoritative state of its own body
// (nil without one), for reconciliation. Callers hold s.mu.
func (s *Server) meBytes(p *Player) []byte {
	m := s.meStateOf(p)
	if m == nil {
		return nil
	}
	me, _ := json.Marshal(m)
	return me
}

// meStateOf is p's self-state (nil without a body). Callers hold s.mu.
func (s *Server) meStateOf(p *Player) *meState {
	if p.body == nil {
		return nil
	}
	b := p.body
	return &meState{Ack: p.ackSeq, TB: p.tickBase, Pos: b.Pos, Vel: b.Vel, OnGround: b.OnGround, Grav: b.Grav, Boots: b.Boots, Noclip: b.Noclip, MapRev: s.mapRev(), Energy: b.Energy, Hook: hookStateOf(&b.Hook), Crouched: b.Crouched, HP: p.HP, Dead: p.Dead, Queue: len(p.cmds), Dropped: p.cmdsDropped, Team: p.Team, Money: p.Money, Armor: p.Armor, Helmet: p.Helmet, Kit: p.Kit, Shades: p.Shades, Buy: canBuyNow(s, p), Weapons: weaponStateOf(p),
		Flash: p.flashLeft(s.clock()), FlashDur: p.flashDur, Ride: p.ride, Seat: p.seat, Boat: s.rideExact(p), Deck: s.deckExact(p), Car: p.car, CarState: s.carExactOf(p), CarLock: s.carLocked(p), Garage: s.garageMsg(p), Pit: p.pit, PitSeq: p.pitSeq,
		SpawnPick: p.spawnPick, KitID: p.kit, SpawnAt: s.deployAuto(p), Respawn: s.respawnLeft(p)}
}

// snapBytes assembles a snapshot from pre-marshalled parts (the player list is
// shared; only "me" differs per recipient).
// extra: more fields as `"key":value` fragments (live grenades, your team's spotted enemies).
// posesJSON: every listed player's pose for a pose snapshot, as an array
// [id, x, y, z, yaw, pitch, flags] (mm, 4 decimals; flags: 1 crouched, 2
// not a body: dead or spectating, which the lag-compensation history and
// the client's collisions leave out; flags>>2: the gravity, Grav), then for
// a rider of a mover [..., fr, lx, ly, lz] (Frame, Local). No gait: it travels in full snapshots
// only, interpolated between them (GaitBetween).
func (s *Server) posesJSON(list []*Player) []byte {
	b := make([]byte, 0, 40*len(list)+2)
	b = append(b, '[')
	for i, p := range list {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '[')
		b = strconv.AppendInt(b, int64(p.ID), 10)
		pos, _ := p.Pos.MarshalJSON()
		b = append(b, ',')
		b = append(b, pos[1:len(pos)-1]...) // x,y,z without the brackets
		y, _ := Angle(p.Yaw).MarshalJSON()
		b = append(b, ',')
		b = append(b, y...)
		pt, _ := Angle(p.Pitch).MarshalJSON()
		b = append(b, ',')
		b = append(b, pt...)
		f := 0
		if p.Crouch {
			f |= 1
		}
		if !solid(p) {
			f |= 2
		}
		f |= p.Grav << 2 // the gravity (0..5) in bits 2-4
		b = append(b, ',')
		b = strconv.AppendInt(b, int64(f), 10)
		if p.Frame > 0 && p.Local != nil { // riding a mover: its frame and place in it
			b = append(b, ',')
			b = strconv.AppendInt(b, int64(p.Frame), 10)
			l, _ := p.Local.MarshalJSON()
			b = append(b, ',')
			b = append(b, l[1:len(l)-1]...)
		}
		b = append(b, ']')
	}
	return append(b, ']')
}

// poseBytes: a pose snapshot ({"pose":1}): the tick, the players' poses and
// the client's own full state. Like every snapshot it stands on its own.
func poseBytes(ms, tick int64, poses, me []byte) []byte {
	b := make([]byte, 0, len(poses)+len(me)+80)
	b = append(b, `{"t":"snap","pose":1,"time":`...)
	b = strconv.AppendInt(b, ms, 10)
	b = append(b, `,"tick":`...)
	b = strconv.AppendInt(b, tick, 10)
	b = append(b, `,"players":`...)
	b = append(b, poses...)
	if me != nil {
		b = append(b, `,"me":`...)
		b = append(b, me...)
	}
	return append(b, '}')
}

func snapBytes(ms int64, tick int64, players, items, me, round []byte, extra ...[]byte) []byte {
	n := len(players) + len(items) + len(me) + len(round) + 80
	for _, e := range extra {
		n += len(e) + 1
	}
	b := make([]byte, 0, n)
	b = append(b, `{"t":"snap","time":`...)
	b = strconv.AppendInt(b, ms, 10)
	b = append(b, `,"tick":`...)
	b = strconv.AppendInt(b, tick, 10)
	b = append(b, `,"players":`...)
	b = append(b, players...)
	b = append(b, `,"items":`...)
	b = append(b, items...)
	for _, e := range extra {
		if len(e) > 0 {
			b = append(b, ',')
			b = append(b, e...)
		}
	}
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
	_ = conn.SetCompressionLevel(flate.BestSpeed)
	// Disable Nagle: every snapshot and input should leave immediately
	// rather than wait to be coalesced with the next one.
	if tcp, ok := conn.NetConn().(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	defer conn.Close()

	p := s.join(ip)
	defer s.leave(p)
	s.mu.Lock()
	p.conn = conn
	s.mu.Unlock()
	slog.Info("player joined", "id", p.ID, "remote", r.RemoteAddr)

	s.mu.Lock()
	welcome, _ := json.Marshal(welcomeMsg{T: "welcome", ID: p.ID, Color: p.Color, Spawn: p.Pos, Map: s.Map, Tick: float64(SimHz) / snapEvery, SimHz: SimHz, Version: s.Version, Build: s.Build, Team: p.Team, Codec: s.welcomeCodec(), Schema: s.welcomeSchema()})
	p.send <- welcome
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
		_ = conn.SetReadDeadline(wallNow().Add(s.idleLimit()))
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
		s.readLate(p, m)
		s.handle(p, m)
	}
}

// Redeploy switches the client build the server announces and drops every
// connection without a kick notice, as a restart onto a new build does: each
// client reconnects, meets the new build in its welcome and reloads. Test
// harness only (app.Redeploy).
func (s *Server) Redeploy(build string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Build = build
	for _, p := range s.players {
		if p.kick != nil {
			close(p.kick)
			p.kick = nil
		}
	}
}

func (s *Server) writeLoop(conn *websocket.Conn, p *Player, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case msg := <-p.send:
			_ = conn.SetWriteDeadline(wallNow().Add(writeWait))
			kind := websocket.TextMessage
			if len(msg) > 0 && msg[0] != '{' { // binary codecs' snapshots (every JSON message is an object)
				kind = websocket.BinaryMessage
			}
			// Deflate gains ~10% on a delta snapshot for the CPU of a JSON one: binary goes as it is.
			conn.EnableWriteCompression(kind == websocket.TextMessage)
			if err := conn.WriteMessage(kind, msg); err != nil {
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
		lastUpdate: s.clock(),
		send:       make(chan []byte, sendQueue),
		ip:         ip,
		joined:     s.clock(),
		HP:         maxHP,
		Money:      s.startMoney(),
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
			down, up := p.rates(s.clock())
			net[p.ID] = map[string]float64{"downKBps": down, "upKBps": up}
		}
	}
	return map[string]any{"map": s.Map.Name, "players": list, "net": net, "nades": s.nades}
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

// teamFull reports whether p joining team would leave it more than
// mp_limitteams players bigger than the other side (0: no limit). Callers
// hold s.mu.
func (s *Server) teamFull(team string, p *Player) bool {
	limit := s.limitTeams()
	if limit <= 0 {
		return false
	}
	n := s.teamCounts(p)
	if c := s.teamCap(); c > 0 && p.ai == nil {
		if all, bots := s.onTeam(team, p); all >= c && bots > 0 {
			n[team]-- // a bot will make room (makeRoom)
		}
	}
	for _, o := range content.Teams {
		if o != team && TeamOverLimit(n[team], n[o], limit) {
			return true
		}
	}
	return false
}

// limitTeams is mp_limitteams: how many players one side may have over the
// other after a join (CS: mp_limitteams; 0 or less: no limit). Callers hold s.mu.
func (s *Server) limitTeams() int {
	v, err := strconv.ParseFloat(s.cvars["mp_limitteams"], 64)
	if err != nil {
		v, _ = strconv.ParseFloat(defaultCvars["mp_limitteams"], 64)
	}
	return int(math.Floor(v))
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
	case s.Test || s.rng.IntN(2) == 0: // test servers: ties go to the first team, so scenarios replay
		return a
	default:
		return b
	}
}

func (s *Server) leave(p *Player) {
	s.mu.Lock()
	if s.players[p.ID] == p {
		s.dropRacer(p)
		s.dropBomb(p)
		delete(s.players, p.ID)
		s.mcpNote(map[string]any{"t": "leave", "id": p.ID, "name": p.Name})
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
		if !p.ready {
			s.hellos++
			p.helloSeq = s.hellos
			s.mcpNote(map[string]any{"t": "join", "id": p.ID, "name": p.Name, "ip": p.ip})
		}
		p.ready = true
	case "team":
		team := m.Team
		if team == "auto" {
			if team = s.autoTeam(p); team == "" {
				if p.Team == content.Spectator {
					s.refuseTeam(p, "", true) // both sides full
				}
				return
			}
		}
		if team == p.Team || (team != content.Spectator && !slices.Contains(content.Teams, team)) {
			return // already there (no free respawn), or unknown
		}
		if team != content.Spectator {
			if capped := s.capFull(team, p); capped || s.teamFull(team, p) {
				s.refuseTeam(p, team, capped)
				return
			}
			s.makeRoom(team, p) // a bot gives up its place
			s.teamSeqN++
			p.teamSeq = s.teamSeqN
		}
		s.dropBomb(p)
		p.Team = team
		s.mcpNote(map[string]any{"t": "team", "id": p.ID, "name": p.Name, "team": team})
		s.respawn(p) // a new side respawns you there; spectators lose their body
		if team != content.Spectator && s.roundsOn() && !s.frozen() {
			s.lateJoin(p) // mid-round: you play from the next round on (tickets: after the respawn delay)
		}
	case "deploy": // tickets mode: where to respawn, with which kit (in the self-snapshot)
		s.setDeploy(p, m.Pt, m.Kit)
	case "cmd":
		if m.Sa > 0 {
			p.ackSnap(s.tick, m.Sa)
		}
		s.queueCmds(p, m.Cmds)
	case "sack": // a snapshot ack on its own (no commands going out: dead, spectating, the menus)
		p.ackSnap(s.tick, m.Sa)
	case "tp": // test harness only: teleport the server body, resync commands
		if !s.Test || !finite(m.Pos) {
			return
		}
		s.teleport(p, m.Pos, m.Yaw, m.C, int(m.Seq))
		if m.Gv != nil && *m.Gv >= sim.GravDown && *m.Gv <= sim.GravPZ {
			p.body.Grav, p.Grav = *m.Gv, *m.Gv
		}
		if m.Bt != nil {
			s.setBoots(p, *m.Bt)
		}
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
	case "buy":
		s.active(p)
		s.buy(p, m.Item)
	case "pit": // the race garage open (c) or not: the pit stop mid-race (pitstop.go)
		s.active(p)
		s.pitMsg(p, m.C, int(m.Seq))
	case "say":
		s.active(p)
		s.say(p, m.Text, m.TeamOnly)
	case "radio":
		s.active(p)
		s.radio(p, m.Radio)
	case "drop":
		if p.inv != nil {
			s.drop(p)
		}
	case "bot": // console bot_* commands: admin commands like any other
		s.adminCommand(p, "bot_"+m.Cmd, []string{m.Arg})
	case "vote": // map vote
		s.castVote(p, m.Map)
	case "admin_login":
		s.adminLogin(p, m.Pw)
		s.refitNoclip() // an admin flies while the map is edited
	case "admin_logout":
		p.admin = false
		s.print(p, "admin logout")
		trySend(p, s.cvarsMsg(p))
		s.refitNoclip()
	case "mapget": // the map editor: our map's revision fell behind (a lost mapdelta)
		s.sendMapSync(p)
	case "editpresence": // the map editor: where this admin aims, what they hover and selected
		if s.isAdmin(p) {
			s.setPresence(p.ID, editPresence{At: m.At, Hover: m.Hover, Sel: m.Sel, Tool: m.Tool})
		}
	case "rcon":
		s.adminCommand(p, m.Cmd, m.Args)
	case "loading": // a client's map load progress (maploading.go)
		s.loadProgress(p, loadMsgIn{ID: m.LoadID, Step: m.Step, Pct: m.Pct})
	case "full": // delta codec: the client has no usable base, or a checksum failed (netdelta_codec.go)
		p.requestFull()
	case "ping":
		p.reportPing(m.Rtt)
		b, _ := json.Marshal(map[string]any{"t": "pong", "seq": m.Seq})
		trySend(p, b)
	case "state":
		if p.body != nil || m.Sp != p.spawnSeq || p.Team == content.Spectator {
			return // the server moves this player now, a state from before the latest respawn, or a spectator
		}
		now := s.clock()
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

// idleLimit is how long a silent client keeps its socket. In lockstep the
// clients' clocks (and keepalive pings) stop between harness steps, however
// long the machine takes over one: no limit worth having then.
// SetLockstep(true) also moves the deadline of every read already waiting
// (a page that connected while it was off).
// SetIdleTimeout sets how long a client may stay silent outside lockstep
// before it is dropped (0: idleTimeout). Set before the server runs.
func (s *Server) SetIdleTimeout(d time.Duration) { s.idleAfter = d }

func (s *Server) idleLimit() time.Duration {
	if s.lockstepOn.Load() {
		return time.Hour
	}
	if s.idleAfter > 0 {
		return s.idleAfter
	}
	return idleTimeout
}

// wallNow is the wall clock, for what is not gameplay: the ticker that drives
// the game clock (Run), socket deadlines, traffic stats, test harness
// timeouts. Gameplay reads s.clock(). TestGameClockOnly keeps it that way.
func wallNow() time.Time { return time.Now() }

// gameEpoch is the game clock of a server that has never ticked and was never
// started by Run (tests): a fixed time, so nothing a test builds before its
// first tick depends on when it ran.
var gameEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// clock is the game clock: the time of the last server tick. Item and
// respawn timers run on it, so they follow the ticks (a stepped test server
// too), not the wall clock. Before the first tick it is the time Run started
// (production), else gameEpoch: never the wall clock.
func (s *Server) clock() time.Time {
	if !s.lastTickAt.IsZero() {
		return s.lastTickAt
	}
	if !s.started.IsZero() {
		return s.started
	}
	return gameEpoch
}

// advance runs one server tick at `now`: queued commands, bots, items on the
// floor, and the lag-compensation history on snapshot ticks. Run calls it on
// its ticker; tests (sim/cases) call it directly. Reports whether this tick
// broadcasts a snapshot. Callers hold s.mu.
func (s *Server) advance(now time.Time) bool {
	// A finished vote changes the map (everyone respawns) before this tick's
	// commands run: they move the respawned bodies, so the state this tick
	// reports is post-physics, as the client predicts it (maps-vote-console).
	pf := &s.prof // sv_prof: each step's time (tickprof.go)
	pf.start()
	s.tickLoad(now) // a map load: the nav graph in, the match starting (maploading.go)
	loading := s.loading()
	if !loading { // a map load freezes the world: only the players' own commands run (frozen)
		s.tickVote(now)
		pf.mark(secVote)
		s.tickNades(1.0 / SimHz)
		s.tickNadeRefill()
		pf.mark(secNades)
		s.tickEditors()
		pf.mark(secEditors)
		s.tickAI() // playing bots queue this tick's command, then it runs like anyone's
		pf.mark(secAI)
		s.tickPuppets()
		pf.mark(secPuppets)
	}
	s.simTick(now)
	pf.mark(secCmds)
	s.tickBoats(1.0 / SimHz)
	s.tickCars(1.0 / SimHz)
	if s.onFoot() {
		s.tickStations(now) // a parkour course's free cars and boats back to their stations
	}
	pf.mark(secBoats)
	if !loading {
		s.tickRound(now)
		pf.mark(secRound)
		s.tickBots(1.0 / SimHz)
		pf.mark(secBots)
		if s.Weapons != nil {
			s.tickItems(now)
		}
	}
	pf.mark(secItems)
	snap := s.tick%snapEvery == 0
	s.updateFrames()
	if snap {
		s.recordHistory()
	}
	pf.mark(secHistory)
	return snap
}

// Run ticks bots and broadcasts snapshots at TickRate until stop is closed.
func (s *Server) Run(stop <-chan struct{}) {
	s.mu.Lock()
	// The game clock until the first tick (clock): joins in the first 10 ms
	// stamp the real start, not gameEpoch, so the first tick's interval is
	// one tick, not years.
	s.started = wallNow()
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
			snap := s.advance(wallNow())
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
	Armor  bool    `json:"ar,omitempty"` // armour soaked part of it (the shooter hears a muffled thunk)
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
		if s.spared(shooter, p) {
			continue // mp_friendlyfire 0: shots pass through teammates (a race without combat: everyone)
		}
		if h, t, ok := hitPlayer(p, o, d, s.poseTime()); ok && t < best {
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
				if frozen { // a frozen target stands (or holds bot_crouch), never stuck mid wander-crouch
					b.brain.crouchLeft = 0
					b.Crouch = b.brain.holdCrouch
				}
			}
		}
		s.notice(p, "bot_freeze "+map[bool]string{true: "1", false: "0"}[frozen])
	}
}

func (s *Server) notice(p *Player, text string) {
	b, _ := json.Marshal(map[string]string{"t": "notice", "text": text})
	trySend(p, b)
}

// broadcast sends this tick's snapshot: a full one every fullEvery ticks, a
// pose snapshot between.
func (s *Server) broadcast() { s.sendSnapshot(s.tick%fullEvery == 0) }

// broadcastFull sends a full snapshot now (tests; broadcast on its ticks).
func (s *Server) broadcastFull() { s.sendSnapshot(true) }

func (s *Server) sendSnapshot(full bool) {
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
	s.prof.resume()
	s.netCodec().send(s, list, full)
	s.prof.mark(secSnapshot)
	s.tvFrame(list)
	s.prof.mark(secTV)
	s.mcpFrame(list)
	s.prof.mark(secMCP)
}

// trySend never blocks the tick: a client too slow to drain its queue just
// misses snapshots.
func trySend(p *Player, b []byte) bool {
	select {
	case p.send <- b:
		p.bytesOut += int64(len(b))
		return true
	default:
		return false
	}
}

// ordered lists the players by ID. Gameplay walks them in this order, not
// the map's random one: which bot thinks (and draws from the seeded rng)
// first, or whose shot lands first in a tick, then repeats run to run.
func (s *Server) ordered() []*Player {
	ps := make([]*Player, 0, len(s.players))
	for _, p := range s.players {
		ps = append(ps, p)
	}
	slices.SortFunc(ps, func(a, b *Player) int { return a.ID - b.ID })
	return ps
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
func (s *Server) friendlyFire() bool { return s.cvars["mp_friendlyfire"] == "1" || s.raceMode() } // a race is free for all

func copyCvars() map[string]string {
	m := make(map[string]string, len(defaultCvars))
	for k, v := range defaultCvars {
		m[k] = v
	}
	return m
}

// rates is p's average traffic since joining, in KB/s (down = server to client),
// over game time (now: s.clock()): a lockstep run counts the seconds it played,
// not how long the machine took.
func (p *Player) rates(now time.Time) (down, up float64) {
	secs := now.Sub(p.joined).Seconds()
	if secs < 1 {
		secs = 1
	}
	return float64(p.bytesOut) / 1024 / secs, float64(p.bytesIn) / 1024 / secs
}

// reportPing folds a client's measured round trip (ms) into its scoreboard
// ping: the first report as is, then a running average (3/4 old) so one slow
// pong doesn't jump the number. Missing, absurd or non-finite reports (test
// barriers send none) are ignored.
func (p *Player) reportPing(rtt float64) {
	if !finiteF(rtt) || rtt <= 0 || rtt > 10000 {
		return
	}
	r := int(math.Round(rtt))
	if r < 1 {
		r = 1
	}
	if p.Ping == 0 {
		p.Ping = r
		return
	}
	p.Ping = (3*p.Ping + r + 2) / 4
}
