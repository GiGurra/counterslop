// Package content holds level data shared with clients: boxes with a material
// name, spawns, and the map container. Maps are built in content/maps from
// prefabs (content/prefabs) and layout helpers (content/mapgen).
package content

import "fmt"

// Box is an axis-aligned solid block. Kind names a client material in
// web/assets/materials (sand, brick, crate, hazard, ...).
//
// Ramp > 0 makes it a wedge: a right triangular prism filling the lower half
// of the box, its sloped face rising from the bottom edge on one side to the
// top edge on the other (RampPX: rises towards +x, ...). A surf ramp is two
// wedges back to back. Code that only knows boxes treats a wedge as its box
// (conservative); movement, rays, nades and the renderer know the slope
// (internal/sim/wedge.go, docs/physics.md). In JS a box without a ramp has
// no `ramp` field: test it with Ramp > 0, never Ramp == 0.
type Box struct {
	Min  [3]float64 `json:"min"`
	Max  [3]float64 `json:"max"`
	Kind string     `json:"kind"`
	Ramp int        `json:"ramp,omitempty"`
}

// Wedge directions (Box.Ramp): the slope rises towards this side.
const (
	RampPX = 1 // low at Min[0], high at Max[0]
	RampNX = 2 // low at Max[0], high at Min[0]
	RampPZ = 3 // low at Min[2], high at Max[2]
	RampNZ = 4 // low at Max[2], high at Min[2]
)

// W makes a wedge (Box.Ramp) from min and max corners, rising towards ramp.
func W(x0, y0, z0, x1, y1, z1 float64, kind string, ramp int) Box {
	return Box{Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}, Kind: kind, Ramp: ramp}
}

// Map is the level geometry, sent to clients on join so client-side
// collision matches what the server expects.
type Map struct {
	Name   string       `json:"name"`
	Boxes  []Box        `json:"boxes"`
	Spawns [][3]float64 `json:"spawns"`          // every spawn, any team
	Items  []Item       `json:"items,omitempty"` // weapons placed on the map (respawn after pickup)
	// TeamSpawns lists each team's spawns by team id (see Teams).
	TeamSpawns map[string][][3]float64 `json:"teamSpawns"`

	// Mode is the game mode (Modes): "" is Big Diff defusal (de_ maps, Sites
	// A and B), ModeGPUDrop is GPU Drop (gs_ maps, GPUs and Exits, no Sites).
	Mode string `json:"mode,omitempty"`
	// Sites are the bomb-plant volumes ("A", "B"); defusal only.
	Sites []Zone `json:"sites,omitempty"`
	// GPUs are the boxed GPUs on the stockroom shelves (GPU Drop: 4 by
	// default); a restocked GPU goes back to its spot.
	GPUs []GPUSpot `json:"gpus,omitempty"`
	// Exits are where a GPU carrier secures it (GPU Drop: 2 or more, far
	// apart; e.g. "dock", "entrance").
	Exits []Zone `json:"exits,omitempty"`
	// Points are the capture points (Mode "tickets", docs/starslop.md): held
	// by a team or neutral, flipped by standing in them; a spawn point lets
	// its owner respawn there. Each team's main is a locked spawn point.
	Points []CapturePoint `json:"points,omitempty"`
	// BuyZones are where each team may buy, by team id.
	BuyZones map[string][]Zone `json:"buyZones,omitempty"`
	// Nav is a waypoint graph over the walkable routes (spawns, chokes, sites):
	// the map tests walk it, and bots can path over it.
	Nav []NavNode `json:"nav,omitempty"`
	// NavHints steer a nav graph built from the geometry (a map without Nav,
	// or bot_autonav; internal/content/navgen): each is a point the graph
	// passes through, a lane if marked (docs/navgen.md).
	NavHints []NavHint `json:"navHints,omitempty"`
	// Movers are moving solids (escalator treads, Mover): a player standing on
	// one is carried along (internal/sim), and the client draws them.
	Movers []Mover `json:"movers,omitempty"`
	// Water is the map's water volumes (Kind Water): a box's top is the
	// surface; bodies swim in them (internal/sim water.go), the client draws
	// the surface. Bullets and sight pass through (v1).
	Water []Box `json:"water,omitempty"`
	// Boats are where the map's boats (inflatable dinghies, internal/sim
	// boat.go) start each round: on the water. Optional.
	Boats []BoatSpawn `json:"boats,omitempty"`
	// Cars are where the map's cars (internal/sim car.go, web/assets/cars.json
	// types) stand each round. Optional (docs/racing.md).
	Cars []CarSpawn `json:"cars,omitempty"`
	// Checkpoints are a race track's gates in driving order (Mode "race",
	// docs/racing.md): the first is the start/finish line. A lap passes
	// every one in order; a wrecked racer comes back at the last one passed.
	Checkpoints []Checkpoint `json:"checkpoints,omitempty"`
	// PitLanes repair a car driving through them (internal/game carcombat.go:
	// carPitRepair HP a second): the race's only repair (docs/racing.md).
	PitLanes []Zone `json:"pitLanes,omitempty"`
	// RaceLine is the racing line the bots drive (internal/game ai_race.go):
	// points on the road surface in driving order, a closed loop through every
	// gate (a spline is fitted through them: one every 10-30 m, more in the
	// corners). Optional; a race map without one has bots aim gate to gate.
	RaceLine [][3]float64 `json:"raceLine,omitempty"`
	// Lights are the map's local lights (lamps, ceiling panels, screen glow); the
	// renderer adds them to the sun and the sky ambient (web/engine/lights.js).
	Lights []Light `json:"lights,omitempty"`

	// Fog, when set, is the map's own distance fog in place of the palette's
	// warm outdoor haze (an enclosed map: inside a server case, a cave).
	Fog *Fog `json:"fog,omitempty"`

	// Pits are deadly volumes: a body whose feet go in dies, the world its
	// killer, the pit's name its weapon in the kill feed (he_merge_hell's
	// "main"). Bodies only: grenades fall through.
	Pits []Zone `json:"pits,omitempty"`

	// GravZones are boxes with their own gravity (the Moon, space; internal/sim
	// zones.go, docs/starslop.md): the last one holding a body's centre wins.
	GravZones []GravZone `json:"gravZones,omitempty"`
	// Launchers are launch pads: a body on the ground with its feet in one is
	// thrown at its velocity (internal/sim zones.go).
	Launchers []Launcher `json:"launchers,omitempty"`
	// Departures are ships leaving on a timetable (MVP-1; Departure).
	Departures []Departure `json:"departures,omitempty"`
	// TicketGates: walking through one scans your respawn ticket (the tickets
	// mode's "tickets please" gag: an event and a voice line, once per life).
	TicketGates []Zone `json:"ticketGates,omitempty"`
	// Signs are text drawn on the map (departure boards, posters, the
	// rocket's name): the client's alone, no collision (Sign).
	Signs []Sign `json:"signs,omitempty"`
	// Scanners: walk-through security scanners (cq_moonshot's terminal), a
	// gag: the client beeps and the TSA voice asks for your opinions.
	Scanners []Zone `json:"scanners,omitempty"`
	// Sky is the sky the client draws ("" the day sky, "space": stars, the
	// Earth and the Moon; web/assets/sky.js).
	Sky string `json:"sky,omitempty"`
	// SkyZones: boxes with a sky of their own, by name (SkyEarth, SkyMoon):
	// the client draws the first one holding the eye, Sky elsewhere (Star
	// Slop: from the Earth you see the Moon, from the Moon the Earth).
	SkyZones []Zone `json:"skyZones,omitempty"`

	// Cut, when set, takes an enclosed map's lid off for a view from above it:
	// with the eye higher than Cut the renderer leaves out the boxes wholly
	// above it (the Maps screen's preview, the spectators' overview). Inside,
	// nothing changes. Drawing only: the boxes stay solid.
	Cut *float64 `json:"cut,omitempty"`

	// Rules are the server settings this map pins, by cvar name (only those in
	// RuleCvars): while the map runs they override the server's (a surf map
	// plays with source movement whatever the server runs), and the client
	// reads them from the map as the server does (docs/physics.md).
	Rules map[string]string `json:"rules,omitempty"`

	// For the map selector and vote cards.
	Desc       string `json:"desc,omitempty"`
	MaxPlayers int    `json:"maxPlayers,omitempty"` // recommended total, both teams
	// Category groups the selector's cards (Categories, in order).
	Category string `json:"category"`
}

// RuleCvars are the server cvars a map may pin (Map.Rules).
var RuleCvars = []string{"sv_movement", "sv_airaccelerate", "sv_autobunnyhopping", "mp_gravityboots",
	"mp_respawn", "mp_respawntime", "mp_loadout", "mp_tickets", "mp_race_laps", "mp_race_races", "mp_respawn_delay", "mp_nade_refill", "mp_buy_guns",
	"sv_hook_range", "sv_hook_energy", "sv_hook_recharge", "sv_hook_drain", "sv_hook_cost", "sv_hook_pull"}

// Map categories, in the selector's order: small training maps, then full-size
// competitive ones.
const (
	Training    = "training"
	Competitive = "competitive"
)

var Categories = []string{Training, Competitive, GPUDrop, Conquest, Racing, Fun}

// Fun is the house-rules maps' category (he_merge_hell: grenades only), last.
const Fun = "fun"

// GPUDrop is the GPU Drop maps' category (gs_, Mode ModeGPUDrop), after the
// defusal ones.
const GPUDrop = "gpudrop"

// Game modes (Map.Mode). See docs/gpu-drop.md for GPU Drop's rules.
const (
	ModeDefusal = ""
	ModeGPUDrop = "gpudrop"
	// ModeTickets is the ticket-based conquest mode (Star Slop,
	// docs/starslop.md): respawn tickets, capture points, no rounds to buy in.
	ModeTickets = "tickets"
	// ModeRace is Slop n Roll Racing (docs/racing.md): cars on a grid, a
	// countdown, laps through the checkpoints in order, positions, results.
	ModeRace = "race"
)

// Racing is the race maps' category (rc_*, Mode ModeRace), after Conquest.
const Racing = "racing"

// Checkpoint is one gate of a race track (Map.Checkpoints): a racer whose
// feet (the driver's seat) are inside the volume passes it. Spawn and Yaw
// are where a wrecked racer who passed it last comes back, in a fresh car
// facing along the track.
type Checkpoint struct {
	Name  string     `json:"name,omitempty"`
	Min   [3]float64 `json:"min"`
	Max   [3]float64 `json:"max"`
	Spawn [3]float64 `json:"spawn"`
	Yaw   float64    `json:"yaw"`
}

// In: feet (or a seat) at p are inside the gate.
func (c Checkpoint) In(p [3]float64) bool {
	return Zone{Min: c.Min, Max: c.Max}.Contains(p)
}

// Conquest is the tickets maps' category (Mode ModeTickets), after GPU Drop.
const Conquest = "conquest"

// CapturePoint is one capture point of a tickets map (Map.Points).
type CapturePoint struct {
	ID   string `json:"id"`             // short and unique: "A", "B", "earth", ...
	Name string `json:"name,omitempty"` // shown on the HUD and the spawn screen ("" : the ID)
	// Pos is the centre on the floor; a body whose feet are within Radius
	// (across) and PointHeight (up or down) of it stands in the point.
	Pos    [3]float64 `json:"pos"`
	Radius float64    `json:"r"`
	Owner  string     `json:"owner,omitempty"`  // who holds it when the round starts ("": neutral)
	Locked bool       `json:"locked,omitempty"` // can't be captured (a team's main)
	Spawn  bool       `json:"spawn,omitempty"`  // its owner may spawn here
	// Spawns are the spots a player spawns on at this point (spawn points
	// only; mapgen.Point lays a ring of them inside the radius).
	Spawns [][3]float64 `json:"spawns,omitempty"`
}

// GravZone is a box with its own gravity (Map.GravZones): G is the fraction of
// the world's gravity (0: space, where bodies coast and thrust; 1/6: the
// Moon), Jump the fraction of the jump speed (0: unchanged). Mover (1-based,
// 0: none) carries the box with that mover (a ship's cabin keeps its gravity
// through space); such zones win over fixed ones. No omitempty: the generated
// sim reads every field.
type GravZone struct {
	Name  string     `json:"name"`
	Min   [3]float64 `json:"min"`
	Max   [3]float64 `json:"max"`
	G     float64    `json:"g"`
	Jump  float64    `json:"jump"`
	Mover int        `json:"mover"`
}

// Launcher is a launch pad (Map.Launchers): a body on the ground with its
// feet in the box gets velocity Vel (m/s). Mover (1-based, 0: none) carries
// the box with that mover; From..To (seconds into the mover's cycle; both 0:
// always) is when it throws; Air throws a body off the ground too (a ship's
// drop pods). No omitempty: the generated sim reads every field.
type Launcher struct {
	Name  string     `json:"name"`
	Min   [3]float64 `json:"min"`
	Max   [3]float64 `json:"max"`
	Vel   [3]float64 `json:"vel"`
	Mover int        `json:"mover"`
	From  float64    `json:"from"`
	To    float64    `json:"to"`
	Air   bool       `json:"air"`
}

// Departure is a ship that leaves on a timetable (Star Slop's MVP-1 rocket,
// docs/starslop.md): its cabin floor is mover Mover (1-based, a MoverPath);
// while it is docked (its cycle before Close seconds) a body walks in from
// Door (outside it, the end of the boarding bridge) to Board (inside). Bots
// board by it; the HUD counts down to Close.
type Departure struct {
	Name  string     `json:"name"`
	Mover int        `json:"mover"`
	Close float64    `json:"close"`
	Door  [3]float64 `json:"door"`
	Board [3]float64 `json:"board"`
}

// Sign is text on the map, in the client's own 5x7 dot font (web/assets/font.js):
// lines of Text ("\n" between) centred on Pos, a surface the letters stand
// out of by a few centimetres towards Face (SignPX .. SignNZ: the side they
// are read from), letters Size m tall, in material Mat (SignLed, SignInk or
// any map material). Mover > 0 (1-based): the sign rides that mover (the
// rocket's name). Live: the text is a template the client fills every frame
// ("{backlog:vibe}", "{departure:MVP-1}"; web/assets/models/signs.js) on
// split-flap tiles. Signs are drawn only: the server never sees them.
type Sign struct {
	Text  string     `json:"text"`
	Pos   [3]float64 `json:"pos"`
	Face  string     `json:"face"`
	Size  float64    `json:"size"`
	Mat   string     `json:"mat"`
	Mover int        `json:"mover,omitempty"`
	Live  bool       `json:"live,omitempty"`
}

// Sign faces (Sign.Face): the direction the text is read from.
const (
	SignPX = "+x"
	SignNX = "-x"
	SignPZ = "+z"
	SignNZ = "-z"
)

// OutOfWorldY: a body below it has fallen out of the world (the server puts
// it back at a spawn; internal/game outOfWorld). A pit must reach above it.
const OutOfWorldY = -20.0

// Skies a map may ask for (Map.Sky).
const (
	SkySpace = "space"
	SkyEarth = "earth" // a blue day sky with the Moon in it, fading to space with height
	SkyMoon  = "moon"  // space with the Earth in it (and no Moon: you are on it)
	SkyDusk  = "dusk"  // the sun just down: orange low in the west, indigo overhead, first stars (rc_hotfix_highway)
)

// Skies: every sky the client can draw ("" the day sky; web/assets/sky.js SKIES).
var Skies = []string{"", SkySpace, SkyEarth, SkyMoon, SkyDusk}

// PointHeight: how far above or below a point's floor a body still stands in it.
const PointHeight = 3.0

// In: feet at p stand in the point.
func (c CapturePoint) In(p [3]float64) bool {
	dx, dz := p[0]-c.Pos[0], p[2]-c.Pos[2]
	dy := p[1] - c.Pos[1]
	return dx*dx+dz*dz <= c.Radius*c.Radius && dy <= PointHeight && dy >= -PointHeight
}

// Label is the name shown for the point.
func (c CapturePoint) Label() string {
	if c.Name != "" {
		return c.Name
	}
	return c.ID
}

// GPUSpot is a boxed GPU's shelf spot: Pos is the box's floor point (where it
// stands), Yaw its facing.
type GPUSpot struct {
	Pos [3]float64 `json:"pos"`
	Yaw float64    `json:"yaw"`
}

// Site is a bomb site: a Zone the planter must stand in.
type Site = Zone

// Zone is a named axis-aligned volume (a bomb site, a buy zone).
// BoatSpawn is a boat's start: its waterline centre and heading (as a player's yaw).
type BoatSpawn struct {
	Pos [3]float64 `json:"pos"`
	Yaw float64    `json:"yaw"`
}

// CarSpawn is a car's start: the centre of its footprint on the ground, its
// heading (as a player's yaw) and its type (a cars.json id).
type CarSpawn struct {
	Pos  [3]float64 `json:"pos"`
	Yaw  float64    `json:"yaw"`
	Type string     `json:"type"`
}

type Zone struct {
	Name string     `json:"name"`
	Min  [3]float64 `json:"min"`
	Max  [3]float64 `json:"max"`
}

// Contains reports whether the point p (feet position) is inside the zone.
func (z Zone) Contains(p [3]float64) bool {
	for a := range 3 {
		if p[a] < z.Min[a] || p[a] > z.Max[a] {
			return false
		}
	}
	return true
}

// Center is the middle of the zone's floor (min y).
func (z Zone) Center() [3]float64 {
	return [3]float64{(z.Min[0] + z.Max[0]) / 2, z.Min[1], (z.Min[2] + z.Max[2]) / 2}
}

// NavNode is one waypoint; Links name the nodes reachable from it by walking
// (straight line, stairs included). Links are listed once and work both ways.
type NavNode struct {
	Name  string     `json:"name"`
	Pos   [3]float64 `json:"pos"`
	Links []string   `json:"links,omitempty"`
	// Lane marks a route choice: attacker bots each pick one lane node a
	// round (seeded) and walk spawn -> lane -> site, so they use more than the
	// shortest route (internal/game/ai_lanes.go).
	Lane bool `json:"lane,omitempty"`
}

// Team ids. Display names, colours and flavour text live in web/assets/teams.js.
const (
	Vibe    = "vibe"    // Vibe Coders, attackers
	Artisan = "artisan" // Artisans, defenders
	// Spectator is not a side: no body, can't shoot or be shot. New players
	// start here until they pick a team.
	Spectator = "spec"
)

// Teams in a fixed order.
var Teams = []string{Vibe, Artisan}

// B makes a box from min and max corners.
func B(x0, y0, z0, x1, y1, z1 float64, kind string) Box {
	return Box{Min: [3]float64{x0, y0, z0}, Max: [3]float64{x1, y1, z1}, Kind: kind}
}

// Material names known to the client (web/assets/materials).
const (
	Sand      = "sand"
	Brick     = "brick"
	Crate     = "crate"
	Hazard    = "hazard"
	Concrete  = "concrete"  // tiled floors, panelled walls
	Carpet    = "carpet"    // office carpet tiles
	Plaster   = "plaster"   // painted office walls with a skirting stripe
	Metal     = "metal"     // diamond-plate decks and steps
	Desk      = "desk"      // laminate desks, counters, whiteboards
	Container = "container" // corrugated shipping containers; colour from the footprint (prefabs.Container)
	Stucco    = "stucco"    // sun-baked render over sandstone blocks (old town walls)
	Rack      = "rack"      // server racks: perforated doors, blinking status LEDs
	Awning    = "awning"    // striped market cloth, its colour picked by where it hangs
	Lamp      = "lamp"      // light fixture: glowing diffuser in the hue of the Light under it

	// The hardware superstore (gs_ maps). Per-box looks are seeded by where the box stands.
	StoreTile = "storetile" // polished floor tiles, ceiling strips mirrored in them
	Shelf     = "shelf"     // gondola shelving stocked with products; narrow ends are SALE end caps
	Carton    = "carton"    // cardboard stock cartons (a big box reads as a stacked pallet)
	Checkout  = "checkout"  // checkout counter: blue sides, a lane light, a moving belt on top
	Screen    = "screen"    // monitors and TVs: big faces glow (code, a demo game or a sale ad)
	PCTower   = "pctower"   // gaming PC: glass side with RGB fans, mesh front
	Escalator = "escalator" // escalator steps (drawn moving from Map.Movers)
	Glass     = "glass"     // tinted glass panes and balustrades (opaque for now), steel frame
	Handrail  = "handrail"  // black rubber handrail on top of an escalator balustrade
	Neon      = "neon"      // a sign with a neon word (SALE, GPU, OPEN, 9090, DEAL)
	LanCarpet = "lancarpet" // dark LAN cafe carpet with glowing arcade shapes

	Car = "car" // a parked car's painted body; colour from the footprint (prefabs.Car)

	// surf_scope_creep
	Gantt      = "gantt"      // surf ramps: a roadmap's Gantt chart (task bars, slipped milestones, today line)
	Deprecated = "deprecated" // the void far below: struck-through code glowing in a dark editor
	// lab_harbor and the boarding map
	Water = "water" // a water volume's surface (Map.Water): animated waves

	// inside a server, scaled up (de_rackmount)
	PCB   = "pcb"   // motherboard: solder mask, traces, vias, silkscreen
	RAM   = "ram"   // RAM sticks: heat spreader, gold contacts, an RGB strip
	Fan   = "fan"   // case fans: spinning rotors in a square frame
	Cable = "cable" // cable looms: coloured wires, zip ties

	// he_merge_hell
	Diff = "diff" // a branch as a bridge: a diff's red (-) and green (+) lines, +/- in the gutter
	LGTM = "lgtm" // padded review-approval walls: grenades bounce off them (grenades.go lgtmBounce)
	// Star Slop (docs/starslop.md)
	Regolith = "regolith" // Moon dust with small craters (lab_moon)
	// Slop n Roll Racing (rc_ci_pipeline)
	Tarmac   = "tarmac"   // dark road with faint pulsing circuit traces; neon underglow on the sides
	Curb     = "curb"     // red/white kerb rails with a neon tube on top
	DataSea  = "datasea"  // the data sea far below: a glowing grid streaming packets
	RaceGate = "racegate" // a gate's banner: chequers between chasing neon tubes
	Chevron  = "chevron"  // a corner sign: neon chevrons chasing left (as read)
	ChevronR = "chevronr" // the same, chasing right
	// Star Slop signs (cq_moonshot)
	SignLed = "led"    // lit amber letters and digits (departure boards, clocks): signs
	SignInk = "ink"    // matte dark lettering (posters, wall signs): signs
	Flap    = "flap"   // split-flap tiles: dark plates split across the middle (boards' backs)
	Poster  = "poster" // a printed poster: paper with a coloured header band and a halftone picture
	// rc_merge_train
	Conflict     = "conflict"     // the sea far below: molten merge conflicts, markers drifting
	ConflictSign = "conflictsign" // a board of <<<<<<< ======= >>>>>>> in amber
	// rc_hotfix_highway
	Highway   = "highway"   // asphalt with edge lines and a dashed centre line; marker lights on the sides
	NightCity = "nightcity" // the city far below at dusk: roofs, lit windows, streets of lights
	ExitSign  = "exitsign"  // a green highway sign: white border, arrows straight on
)

// Materials lists every material name above (the client must define each).
var Materials = []string{Sand, Brick, Crate, Hazard, Concrete, Carpet, Plaster, Metal, Desk, Container, Stucco, Rack, Awning, Lamp,
	StoreTile, Shelf, Carton, Checkout, Screen, PCTower, Escalator, Glass, Handrail, Neon, LanCarpet, Car, Gantt, Deprecated, Water,
	PCB, RAM, Fan, Cable, Diff, LGTM, Regolith, Tarmac, Curb, DataSea, RaceGate, Chevron, ChevronR, SignLed, SignInk, Flap, Poster,
	Conflict, ConflictSign, Highway, NightCity, ExitSign}

// Fog is a map's distance fog: density per metre (the renderer's default is
// 0.014) and colour (linear RGB 0..1).
type Fog struct {
	Density float64    `json:"density"`
	Color   [3]float64 `json:"color"`
}

// Mover is a moving solid (kinematic): its boxes are a pure function of time
// (seconds on the clock of the command being simulated, internal/sim), so the
// server and every client's prediction put it in the same place. A body
// standing on one is carried with it, one rising into a body lifts it, one
// moving into it shoves it (internal/sim movers.go); the client draws it
// (web/engine/conveyor.js).
//
//   - MoverEscalator: the treads of an escalator flight in the volume Min..Max,
//     Rise high and Run deep, rising towards Up (+1/-1) along Axis (0 x, 2 z)
//     from Min[1] to Max[1], moving at Speed m/s along the run (> 0 up, < 0
//     down). The treads are level with the floor for Flat treads at the foot,
//     climb the rest of the flight in equal steps (n*Rise/(n-2*Flat) each for
//     n = (Max-Min)[1]/Rise treads) and are level with the upper floor for
//     Flat treads at the head, sliding under the comb plates at both ends, so a
//     rider steps off without a bump.
//     The map's static tread boxes inside the volume stay for bullets and
//     sight lines but not for movement.
//   - MoverSlide: the box Min..Max moving to Min+Move..Max+Move and back,
//     linearly, once per Period seconds, starting Phase (0..1) into its cycle.
//   - MoverPath: the box Min..Max moved by an offset along Path, keys
//     [t, dx, dy, dz] (t: seconds into the cycle, ascending, the first at 0),
//     eased (smoothstep) from key to key, holding the last key to the end of
//     the Period, then back at the first (a ship's departure, docs/starslop.md).
//   - Hide (any kind): [from, to) seconds into the cycle when the box is not
//     there (a trapdoor; to < from wraps round the cycle's end; [0, 0]: never).
//
// No omitempty: the generated sim reads every field from the map JSON.
type Mover struct {
	Kind   string       `json:"kind"`
	Mat    string       `json:"mat"`
	Min    [3]float64   `json:"min"`
	Max    [3]float64   `json:"max"`
	Axis   int          `json:"axis"`
	Up     int          `json:"up"`
	Rise   float64      `json:"rise"`
	Run    float64      `json:"run"`
	Speed  float64      `json:"speed"`
	Flat   float64      `json:"flat"`
	Move   [3]float64   `json:"move"`
	Period float64      `json:"period"`
	Phase  float64      `json:"phase"`
	Path   [][4]float64 `json:"path"`
	Hide   [2]float64   `json:"hide"`
}

const (
	MoverEscalator = "escalator"
	MoverSlide     = "slide"
	MoverPath      = "path"
)

// NavHint is a point a generated nav graph must pass through (a node there,
// linked like any other); Lane: also a lane, an attacker bot's route choice
// (with lane hints on a map, they are its only lanes).
type NavHint struct {
	Pos  [3]float64 `json:"pos"`
	Lane bool       `json:"lane,omitempty"`
}

// Item is a weapon lying on the map; Weapon is an id from web/assets/weapons/weapons.json.
type Item struct {
	Weapon string     `json:"weapon"`
	Pos    [3]float64 `json:"pos"`
}

// Light kinds.
const (
	PointLight = "point"
	SpotLight  = "spot"
)

// Light is a local light source on the map (purely visual: no gameplay reads it).
// Light reaches as far as Range, falling off smoothly to zero there, and only
// through open space: the client floods it cell by cell, so it never leaks
// through a wall into the next room. Shadow asks for a real shadow map; the
// client gives one to the r_light_shadows most important shadowed lights in view.
type Light struct {
	Kind      string     `json:"kind"`  // PointLight or SpotLight
	Pos       [3]float64 `json:"pos"`   // metres
	Color     [3]float64 `json:"color"` // linear RGB, 0..1
	Intensity float64    `json:"intensity"`
	Range     float64    `json:"range"`           // metres
	Dir       [3]float64 `json:"dir,omitempty"`   // spot: where it points (any length)
	Cone      float64    `json:"cone,omitempty"`  // spot: outer half-angle, degrees
	Inner     float64    `json:"inner,omitempty"` // spot: full-strength half-angle, degrees (0: 0.7 x Cone)
	Shadow    bool       `json:"shadow,omitempty"`
	// Flicker, when non-zero, seeds a flicker (a dying fluorescent tube, a neon sign).
	Flicker uint32 `json:"flicker,omitempty"`
}

// Point makes an unshadowed point light.
func Point(pos [3]float64, color [3]float64, intensity, rng float64) Light {
	return Light{Kind: PointLight, Pos: pos, Color: color, Intensity: intensity, Range: rng}
}

// Spot makes a spot light pointing along dir with an outer half-angle cone (degrees).
func Spot(pos, dir [3]float64, color [3]float64, intensity, rng, cone float64) Light {
	return Light{Kind: SpotLight, Pos: pos, Dir: dir, Color: color, Intensity: intensity, Range: rng, Cone: cone}
}

// Check reports what is wrong with a light, or nil.
func (l Light) Check() error {
	switch {
	case l.Kind != PointLight && l.Kind != SpotLight:
		return fmt.Errorf("kind %q is not point or spot", l.Kind)
	case l.Range <= 0 || l.Range > 40:
		return fmt.Errorf("range %v outside (0, 40] m", l.Range)
	case l.Intensity <= 0:
		return fmt.Errorf("intensity %v must be > 0", l.Intensity)
	}
	for _, c := range l.Color {
		if c < 0 || c > 1 {
			return fmt.Errorf("color %v outside 0..1", l.Color)
		}
	}
	if l.Kind == SpotLight {
		if l.Dir == ([3]float64{}) {
			return fmt.Errorf("spot without a dir")
		}
		if l.Cone <= 0 || l.Cone > 89 || l.Inner < 0 || l.Inner > l.Cone {
			return fmt.Errorf("spot cone %v / inner %v: want 0 < inner <= cone < 90", l.Cone, l.Inner)
		}
	}
	return nil
}
