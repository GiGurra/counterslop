# Slop n Roll Racing

The living design doc of the race mode. The design (operator + lead) comes
first; what is built so far is under "Built" at the end.


A per-map game mode (map mode "race"), a silly combat-racing parody inside counterslop: you still play CS (your gun on the mouse) while driving a parody vehicle. Everything original: no characters, catchphrases, music, car designs or track layouts copied from Rock n' Roll Racing.

## Vehicles (cars.json, shared like weapons.json; data-driven sim)
Four tiers, bought in the pre-race shop, kept for the match (respawn in the same type; upgrades carry over). Top speed strictly increasing per tier, small steps ("it's a race after all"); tiers otherwise differ by handling / car HP / feel. Pin the ordering in a test.
- T1 buggy (starter, free, light, bouncy). Default front weapon: laser (Linter Beam, hitscan).
- T2 racecar (low, grippy, fragile-ish). Default front weapon: unguided rockets (Hotfix Missiles).
- T3 tank (tracks, heavy, most car HP, rams hard; still faster than the racecar). Default front weapon: rockets too (heavier/bigger splash than the racecar's, fewer charges is fine).
- T4 hovercraft (most expensive; no wheels: air cushion, drifty: almost no grip, side thrusters (cars.json "thrust", m/s²) push against the slide so it still steers, and the handbrake cuts them; ignores spikes/oil/water, mines still hit it). Default front weapon: guided energy balls (homing orbs, e.g. "Synergy Orbs"; slower projectiles that steer toward the nearest car ahead / the aimed target).
Each vehicle's built-in front weapon comes with it; the shop sells charges/tiers and rear items.
Parody names on top allowed (Intern Buggy, Senior Racecar, Legacy Tank, Series-A Hovercraft).

## Health
Car HP separate from and shown next to driver HP (two bars). Car body hits/explosions/rams -> car HP; hits on the exposed driver (head/torso leaning out) -> driver HP (armor applies). Car 0 -> explodes, driver dies. Driver 0 -> car wrecks. Low car HP: smoke, then fire. Repair via a pit-lane drive-through (costs time); laps do not repair.

## Controls (rebindable)
W/S throttle, brake/reverse; A/D steer; mouse aims the CS gun 360 independent of the car (lean out the window), LMB fire, R reload, wheel/1-5 switch. Space handbrake/drift. Shift boost. Q front car weapon (fires along the car's heading). C rear drop. E hop. V first-person / chase cam. A small heading arrow by the crosshair.

## Equipment (shop; charges; all refilled each lap; tiers add charges/damage)
FRONT (Q): Linter Beam (laser, hitscan; the buggy's default), Hotfix Missiles (rockets, splash), Force-Push Ram plate.
REAR (C): Tech Debt mines, Breaking Changes spikes (shredded tires: slow + wobble), Spaghetti Code oil slick (spin out).
BOOST (Shift): Hotfix boost tank.
Plus CS guns/grenades bought as usual. Each lap reloads ammo/nades/boost/charges.

## Race rules
Race = round: grid, countdown, 4 laps via checkpoints (no cutting), positions, finish + results. Killed: auto-respawn after ~3 s at the last checkpoint, keeping the lap. Money from kills, laps led, placement; score = placement points + kills. FFA; cars coloured by team for flavour.

## Presentation (phase 4)
Animated vehicles (not static models): wheels spin with speed and steer, tracks scroll per side, suspension bob/body roll, hover skirt wobble + fans, jet/boost flames. All from replicated state (speed, steer, per-wheel contact/compression). Procedural race-rock music tracks through the race, SlopVox announcer (original lines: "Merge conflict in turn three!", "Rolled back to the last checkpoint!"), wrecks/explosions.

## Phases (board)
1 tk_bae4d2f1 car sim + driving + lab track + HUD speedo/car HP
2 tk_aa50e8c7 race rules + first original track
3 tk_f74db548 combat while driving, shop, equipment, per-lap refill, ram damage
4 tk_bac31d10 presentation: animation, music, announcer, models
5 tk_7f4ccb50 bots that race and shoot, tracks 2 and 3

## Built-in rear (secondary) per vehicle (operator + lead)
- buggy: spikes (Breaking Changes): a car that hits them drifts uncontrollably for a few seconds (lost grip, wobbling steer), no tyre damage needed.
- racecar: oil slick (Spaghetti Code): spin-out.
- tank: mines (Tech Debt): damage + knock up.
- hovercraft: sticky proximity mines (Scope Creep) that drift toward the nearest car within a few metres.
Front/rear built-ins refill every lap.

## Shop (pre-race / between races), on top of CS guns + grenades
- Swap a built-in for another front or rear type (e.g. mines on a buggy instead of spikes).
- Bigger charge packs: more charges per lap for front / rear (tiers 1-3).
- Boost tank size (charges per lap), plating (car HP + ram damage), engine (accel, a little top speed), tyres/grip (handling; useless on the hovercraft, so not sold for it).
- Repairs are not sold: pit lane only.

# Built

## Phase 1: the car sim (internal/sim car.go)

`web/assets/cars.json` holds the four types (`CarSpec`: the server loads it,
the client passes the same objects to the generated sim; its keys are the
fields' JS names). Each has its tier, price, drive type, mass, top speed
(buggy 24, racecar 25.5, tank 27, hovercraft 28.5 m/s: strictly rising in
small steps, `TestCarTiers`), reverse, accel, brake, drag, grip and handbrake
grip, turn rate, car HP, size (the hitbox and the walls' discs), wheelbase and
track (the ground probes), ride height and suspension travel (drawn), the
driver's seat, and its built-in front weapon and rear item (ids for phase 3:
buggy `linter_beam` + `breaking_changes` spikes, racecar `hotfix_missiles` +
`spaghetti_code` oil, tank `rollback_rockets` + `tech_debt` mines, hovercraft
`synergy_orbs` + `sticky_tech_debt` sticky proximity mines).

`World.StepCar(spec, car, input, dt)` is arcade physics, deterministic and
generated to the client like the rest of internal/sim (`TestCarParityWithJS`:
the same bits in JS over seeded runs of every type around lab_kart):

- State `Car`: pos (the footprint's centre on the ground), vel, yaw, yaw rate,
  pitch and roll (from the ground under it), steer (smoothed), on the ground,
  per-wheel contact bits and suspension compression (drawn), HP, wet.
- Input `CarInput`: throttle, steer, handbrake.
- Steering first: the heading turns, the velocity stays; then the grip takes
  the sideways part away (gripping it turns into speed along the heading: it
  carves; the handbrake swaps grip for slide grip and brakes: it drifts).
  Wheels don't turn standing and turn less at speed; tracks turn at any speed
  and pivot on the spot; the hover's turn rate follows slowly and it barely
  grips.
- Ground: the highest top (a wedge's slope) no more than 0.6 m over the car
  under each corner; the car sits at their mean and tilts to them. Following it
  up a ramp gives the climb rate as vertical speed, so off a ramp's end it
  flies (gravity 20 m/s²) and lands. The slope pulls it back down (a banked
  turn holds it in). Over nothing it falls.
- Water: wheels and tracks in it bog down (30% engine, heavy drag, sink onto the
  bed); to the hover, water is ground: it floats over it.
- Walls: three discs along its length from 0.6 m over its ground to its height
  against the map's boxes (pushed out, the speed into them removed); the other
  cars' discs (`World.Cars`) the same.

Tests: `internal/sim/car_test.go` (tiers, top speed and coasting to a stop per
type, turning, the tank's pivot, the handbrake slide, water, the ramp jump, the
banked turn, a wall, off the edge, another car, JS parity).

**lab_kart** (`changelevel lab_kart`): a loop round a low island (driven with
the island on your left: +x along the north straight), a ramp ending in a 2.2 m
drop on the north straight, a bank rising 4 m to the wall round the east end,
and a 1.6 m deep water basin across the south straight; one car of each type
on the grid at the north straight's west end, the spawns between them.

## Phase 1: driving (internal/game cars.go, car_shared.go; web/game/cars.js)

**The server** loads `cars.json` (`LoadCars`), puts the map's cars (`Map.Cars`,
`CarSpawn`: pos, yaw, type; mapgen `Builder.Car`) on their starts at every round
start and map change (`resetCars`), and every tick steps each car nobody drives
with no input (`tickCars`: it rolls to a stop; one that fell out of the world goes
back to its start). A driven car moves only by its driver's commands, however
they arrive, exactly as the driver's client predicts it. Every full snapshot
carries the cars (`"cars"`: id, type `k`, `p` mm, `y`/`pt`/`rl` angles, `v` mm,
`f` speed along the heading, `st` steering, `wc` wheel contact bits, `wz` the
four suspension values a byte each (`CarComp`), `hp`, `mhp`, `d` the driver, `wet`;
both codecs, replays). Phase 4 animates wheels, tracks and the skirt from these.

**Driving** (`car_shared.go`, shared with the client): E within 3.2 m of a free
car (feet, x and z) takes its driver's seat (`boardCar`; before a boat). The
driver's body stays a world body, crouched in the seat (`CarSpec.Seat`, turned
and tilted with the car: `CarSeatWorld`), so shots, hits and bots see it there;
the head and torso stick out. `DriveStep` is the driver's command: W/S throttle
and brake/reverse, A/D steering, jump (Space) the handbrake, the other cars'
discs to bump into. The self-snapshot carries the car (`me.car`, `me.cb`: its
exact state); the client adopts it and predicts it (`predict.js adoptCar`,
`localsim.js moveCmd`). E gets out (`CarLeaves`, `CarLeave`: standing on the
roof over the seat, moving with the car; it isn't solid to bodies, so you drop
beside it), predicted on both sides; that E boards nothing. Death, a respawn, a
teleport and the round's reset put the driver out. The mouse aims the gun as on
foot, 360 degrees; the view turns with the car (`carView` in main.js) so W stays
"ahead".

**The client**: `web/game/cars.js` (the snapshot's cars drawn ahead by their
velocity, ours at its predicted pose; the HUD's state), placeholder box models
`web/assets/models/car.js` (body, cockpit, wheels that steer, spin and ride their
suspension; the tank's tracks scroll; the hovercraft's skirt and fan). The chase
camera (`cl_carcam 1`, the default) sits behind and over the car looking where
you aim, your body drawn in the seat; `cl_carcam 0` is first person from the
seat; V (`carcam`) toggles. The HUD: the car's HP next to yours, the speedometer
(km/h) and the car's name with its keys, or "E drive the ..." on foot by a free
car.

**MCP debugger**: `car_set` (move a car, put it home, spawn one of a type, list
them all with speed, ground, wet and driver).

(Phase 3 fixed what phase 1 left: a seated driver's body is at rest in the
seat, so the spread no longer counts the car's speed; under the chase camera
the command aims from the seat at what the crosshair is on: see below.)

Tests: `internal/game/cars_test.go` (the grid, the snapshot, `CarComp`,
boarding and driving and getting out, tickCars, death and the round reset, out
of the world, `car_set`), `web/game/cars.test.mjs` (models, drawing, the HUD,
the replay with a car, stepLocal), `web/engine/carsim.test.mjs` (the generated
sim per type), sim/cases `car-buggy`, `car-racecar-jump`, `car-tank-pivot`,
`car-hover-water` (go, js, e2e: server and client agree every tick, cases start
in a car with `"car": n` and optionally `"carAt"`), `car-board-leave` (go, e2e),
`car-camera` (e2e: V and `cl_carcam`), scenario `kart-drive` (the grid, the
buggy in chase and first person, the tank pivoting).

## Phase 2: the race rules (internal/game race.go) and the first track

**A race map** has `Mode: "race"`, is named `rc_*` (or `lab_*` for a test map)
in the Racing category, and has (`content.Map.ModeProblems`):
`Checkpoints` (the gates in driving order, at least 3, the first the
start/finish line; each a volume a racer's seat passes through, with a respawn
spot and heading after it), a grid (`Map.Cars`, one slot per racer, at least
2) and a pit under the track (falling off is a wreck). Built with
`mapgen.Builder.Checkpoint` and `.Car`. A race map is driven, not walked: the
map checks build no nav graph for it; `internal/content/maps/race_test.go`
drives it instead (`TestDriveTheTracks`: an autopilot laps every track in every
car type with the real car physics, every gate in order, never into the pit;
a new track adds its centre line to `raceLines`; `TestRaceRespawnSpots`: every
gate's respawn spot holds a car on the road, and the jump is cleared from its
gate's spot from a standing start).

**The rules** (server authoritative, `internal/game/race.go`):

- One race is the match's one round. Free for all: anyone on a team races (the
  teams only colour the cars); one racer is a race (a time trial). Playing
  bots race too (phase 5, `ai_race.go`); with more players than cars the
  rest watch.
- The round's freeze time is the grid (5 s): every racer in a car on their
  slot, the countdown holding the cars still (the freeze zeroes the throttle
  and strips E). Then GO.
- Laps: the gates in order (`racerState.CP`: gates passed since the start; a
  gate out of order counts for nothing, no cutting). The grid is behind the
  line: crossing it starts lap 1, crossing it after the last lap
  (`mp_race_laps`, default 4, a map may pin it) finishes. A gate is swept, not
  sampled: every race tick checks the path each command of that tick drove
  (`Player.racePath`, `Checkpoint.Crossed`), so a fast car or a burst of
  commands in one tick (a client hitch, a stalled socket: up to 64 at once)
  can't jump a 2 m gate. A sampled line missed that way left the racer a
  whole lap behind (gates count in order only). Moved without a command (a
  respawn, a teleport): a point check, nothing swept along the jump. Every
  race map's grid is behind its line, facing it
  (`TestRaceGridBehindTheLine`).
- Positions: the finished by place, then the most gates passed, then the
  nearest to the next gate's middle.
- Wrecks: a racer who dies (into the pit, shot, the car at 0 HP) is back after
  3 s in their own car, repaired, on the last gate passed (its `Spawn`,
  facing its `Yaw`; the grid slot before the line), keeping the lap. During
  the race nobody gets out of a car (`me.cl`, shared `CarLeaves(b, prevB,
  locked)`: the client predicts it).
- A player who joins during the race spawns on the timer and races from a free
  grid slot, lap 0, in that slot's stock car fitted from their own garage (its
  item cooldowns reset to their command clock). With every slot taken they
  free-roam until the next race's grid (the HUD says so: `noSlot`).
- A racer who leaves (disconnects, kicked, a bot removed, or off to
  spectators), in the pit or not, drops out at once (`dropRacer`; no ghost in
  the standings) and their car is its grid slot's stock car again, back on the
  slot: never a car upgraded from a gone player's garage. A reload is a clean
  fresh start (a new player: start money, an empty garage), not a restore.
- The end: everyone finished, 30 s after the winner crossed the line, or the
  10-minute limit. Placement points (10, 8, 6, 5, 4, 3, 2, then 1) go to each
  finisher's score; the winner's team (the leader's if nobody finished) takes
  the round; the results, then the map vote.
- **State**: the round state of every snapshot carries `laps`, `gates`, `race`
  (the racers in position order: `id`, `cp`, `pl` place, `t` finish time, `wr`
  wreck countdown, `car`) and `rt` (the race clock while live). The finish
  event (`{t:"race", ev:"finish"}`) is optional.

**The client**: `web/game/race.js` (`raceHud`: lap, position, the standings,
the next gate, the grid's countdown, GO!, the wreck countdown, the finish, the
results; the race clock counted locally between snapshots like the round
timer) and `web/ui/race.js` (the top bar `LAP 2/4 · P1/3 · 1:23.4`, the
standings on the right, the big 3-2-1-GO!, WRECKED, FINISHED, the results under
the winner's banner). The Maps screen has a Racing section.

**Tracks**:

- `lab_race`: the rules' test track, a flat oval plate over the void, four
  wide gates, four buggies on the grid.
- `rc_ci_pipeline` (in the rotation): a figure eight of floating roads on
  pillars over the void, a motherboard glowing far below. The start straight
  west under the flyover; a banked hairpin into the west leg and its jump (a
  2.2 m ramp, an 8 m gap, 40 m to brake on the landing); a narrow bridge (7 m)
  on the south leg; the flyover leg up onto a deck 7 m over the start straight
  (the crossover) and down; the north leg; a banked corner into the east leg;
  back onto the start straight. Low rails (0.7 m: they stop a car running
  along them; a jump or a bad line goes over), six gates with neon arches,
  an 8-slot grid of buggies (tiers come with the shop in phase 3). A lap is
  about 33 s (buggy) to 43 s (hovercraft) for the autopilot.

**Tests**: `internal/game/race_test.go` (the grid and countdown, gates in order,
positions, a fall into the void and the respawn at the last gate, a car at 0
HP, locked in, the end and the points, the time limit, a late joiner, the
snapshot), `race_reset_test.go` (race 2 starts everyone level, whoever was
in the pit, wrecked or finished at the end of race 1; a gate through a burst of
commands), `internal/content/race_test.go` (the mode's map rules),
`internal/content/maps/race_test.go` (driving the tracks, the respawn spots),
`web/game/race.test.mjs` (the HUD state), sim/cases `race-lap` (two racers a
lap each through the real keys, the results; `mp_race_laps`) and `race-wreck`
(off the plate's edge, back on the last gate 3 s later) and `race-two-races`
(race 1 ends with a racer in the pit; race 2 starts both level) at e2e, scenarios
`race-hud` (grid, GO!, racing, wrecked) and `map-rc_ci_pipeline`.

## Phase 3: combat while driving (internal/game carcombat.go, carshop.go)

**Keys** (a layer of the binds only while driving: `car:<code>` in the same
binds store, web/ui/binds.js): Q `+carfront` fires the front item while
held, C `+carrear` drops the rear one per press, Shift `+boost` (full throttle,
1.25x the top speed, 1.8x the push, the tank drains a second a second). All
three are command bits (BtnCarFront, BtnCarRear, BtnBoost); the freeze strips
them. Boost, slip, engine and tyres are internal/sim Car state, so the
prediction runs them too.

**Items** (cars.json `equipment`; each type's `front` / `rear` are its
built-ins; charges per lap):

| id | name | kind | what |
|---|---|---|---|
| linter_beam | Linter Beam | beam | hitscan, 5 HP a tenth of a second, 70 m |
| hotfix_missiles | Hotfix Missiles | rocket | unguided, 35 dmg, 3.5 m blast |
| rollback_rockets | Rollback Rockets | rocket | heavier: 60 dmg, 5 m blast, slower |
| synergy_orbs | Synergy Orbs | orb | home on the nearest driven car ahead |
| force_push | Force Push | ram | your rams hit twice as hard |
| breaking_changes | Breaking Changes | spikes | an uncontrollable drift for 2.5 s |
| spaghetti_code | Spaghetti Code | oil | a slide and a spin |
| tech_debt | Tech Debt | mine | proximity, 45 dmg, a knock up |
| sticky_tech_debt | Scope Creep | mine | creeps after the nearest car, 35 dmg |

Shots and drops are server state (`gear` in every snapshot, at most 64 live;
web/game/cargear.js draws them and the beams and blasts from `car` events).
A drop arms after 0.6 s and spares its own car until that car has once been
clear of it. The hovercraft floats over spikes and oil.

**Damage**: bullets hit the car's body box (bulletCar) unless they hit the
exposed driver first (the player hit model in the seat); the car takes the
weapon's damage x carBulletK. Rams pay by the closing speed, split by mass and
the other car's ram factor (plating, Force Push); a car over 7 m/s hurts a
body on foot it hits. Cars are solid to bodies on foot (internal/sim
peers.go: a car's disc as a box; a body already inside one walks out).
Car HP 0: it explodes, its driver dies (credit to the last hitter); a dead
driver wrecks the car. Friendly fire is on in a race.

**Laps** refill the charges and the boost tank, never the car's HP. The pit
lane (Map.PitLanes; rc_ci_pipeline's beside the start straight, in off the
last corner, out onto the straight) repairs a car in it 35 HP a second: the
only repair. The HUD shows PIT LANE while in it.

**The shop** (the buy menu's 7 Cars, 8 Car weapons, 9 Upgrades on a race
map: open in the warmup, on the grid and over the results of a race
another follows, the buy timer counting down to GO; ui/carshop.js, server carshop.go): a car
type for the next race, a front or rear swap ($500; a type's own built-ins
are free), and three tiers each of charge packs, the boost tank, plating
(car HP and ram damage), engine and tyres (not the hovercraft). The garage
(`me.gar`) is the player's, kept for the match; CS guns and grenades are
bought as ever.

**Money** (economy.json `race`): $400 a kill (gun, car item, ram or wreck), $300
a lap led, place money $3000 / 2200 / 1600 / 1200 and $800 for any other
finisher. No round-end money in a race. A match is `mp_race_races` races
(default 3), no halftime.

**Chase camera aim** (web/game/carcam.js): the command's angles go from the
seat's eye to the point the camera's centre ray hits, so the shot lands on the
crosshair however close.

**Tests**: internal/game/carcombat_test.go (built-ins, the beam, a rocket's
blast, the orb homing, spikes and oil, mines, bullets on a car, a wreck,
rams, the shop, a lap's refill and pay, a match of races, a car solid on foot,
the driver's spread, the pit lane), internal/sim/car_test.go (boost, slip,
upgrades, a body against a car; JS parity), internal/content/maps
TestPitLaneDrivable, web tests (carshop, carcam, cargear, controls,
cars), sim/case `car-combat` (Q, C and Shift through the real keys at e2e) and
`race-lap` (`mp_race_races`), scenario `race-combat`.

## Phase 4: presentation (web/assets/models/car.js, sounds/cars.js, game/cars.js, cargear.js, raceannouncer.js)

Everything here is drawn and heard from the replicated state; it adds no
gameplay state. The car's snapshot carries two presentation fields:
`yv` (yaw rate) and `fx` (bits: throttle, boost, slip, brake; cars.go carFx).

**Models** (models/car.js carParts, one per type, animated by stepCarAnim
from the car's speed, yaw rate, steering and fx): wheels spin with the ground
speed and the front ones steer; the buggy (an office chair on a frame) swivels
its casters; the tank's tracks scroll per side (a turn runs them apart) and
its turret follows the driver's aim; the hovercraft's skirt wobbles and its
fans spin; the body bobs, rolls and pitches on a spring; the racecar's exhaust
flames grow with throttle and boost. The test API's `state().cars` reports
the animation (wheel, trackL/R, swivel, lean, flame).

**Damage** (damageParts): grey smoke under half HP, black smoke and fire under
a quarter, a charred burning wreck. A car exploding (cargear.js wreckParts):
a fireball, a shockwave ring, bits flung up, a smoke column. The items:
rockets with fins and a smoke trail, orbs with satellites, oil with a sheen,
mines pulsing, Scope Creep on legs; flashes on launches and rams.

**Sounds** (sounds/cars.js, in the gallery): an engine per type that revs
with the speed and roars with the boost (one per driven car within 45 m), and
one-shots for every item, booms, wrecks, rams, skids, the boost and the pit.

**Music** (assets/music/themes.js RACE_TRACKS; game/music.js loopFor, the
only loop that plays under live play): every race map has its own track
(RACE_MAP_TRACKS, by map id; a client table, as the music is the client's own):
race rock (power chords on an overdriven guitar, a lead, driving drums) for
rc_ci_pipeline (Green Build Getaway), rc_merge_train (Merge Conflict Motorway),
rc_hotfix_highway (Hotfix Highway) and lab_race / any other car map (Friday
Deploy Drift); breakbeat chiptune for parkour (pk_ maps, lab_parkour, any
point-to-point course: Happy Path Sprint, an acid bass). Races 2 and 3 of a
match play variations (`<track>:<1..3>`: a new melody, the progression turned,
another arrangement). Our final lap (a course: its last stretch) lifts it
(`:lift`: 7 % faster, a whole step up, the heaviest drums, an extra arp), cut in
at the next bar line. Stingers come from the race state, once per change: `lap`
over the line into a new lap, `finalLap` into the last one (or a course's last
stretch), `finish` over the finish line (podium or not). All in the gallery;
`play_music <id>` plays one.

**The race caller** (game/raceannouncer.js, SlopVox voice `caller`, lines
`race_*`): the grid's countdown and GO, the lead taken/lost, a new leader,
wrecks, the last lap, the finish by place, a pit stop; each once per change
of the race HUD state. `state().raceSaid` lists what it said.

**rc_ci_pipeline's dressing**: tarmac roads with pulsing circuit traces and a
neon underglow, red/white kerb rails with a neon top, chequered gate banners
between chasing neon tubes, chevron boards at every corner pointing the way
(materials chevron/chevronr), and the data sea far below (a grid streaming
packets).

**Admin**: `car_hp <id> <hp>` sets a car's HP (0 wrecks it); sim/case
`car-damage`, scenarios `car-damage`, `car-models`.

## The garage (web/game/garage.js, garagescene.js; ui/garage.js; assets/garage.js, models/carkit.js)

A bay to see and shop for what you own (the operator asked for it: what you
own lived only in `me.gar`, shown nowhere until the grid). On a car race map
it opens **at any time and from anywhere**: **B** (the buy key: on a car race
map B is the garage) or **H** (`garage`, rebindable; also a console command),
in every phase, on foot or in a car, mid-air, wrecked or not; no buy zones,
no distance checks. The HUD says so: "B / H garage" over the HP, always, and
"press B for the garage: buy and change your car" in the warmup and on the
grid. Only the phase decides what it is (the operator: "players should be
allowed to reach the garage at any time, even if it means they are left
behind in the race"):

- **Before GO** (warmup, the grid) and **over the results**: the shop.
  Presentation only: your body stays where it is, game input is off while it
  is open (as in the buy menu), and a buy is the same `{t:"buy", item}` the
  buy menu sends (the server checks and charges; the model changes when its
  snapshot says so). What you buy or pick is the car you drive at once. GO
  closes the garage by itself (open it again for a pit stop).
- **During the race: a pit stop** (server-owned, internal/game pitstop.go).
  The client says what it wants, `{t:"pit", c: open, seq}`, level-triggered:
  the server keeps the newest seq's wish (a repeat or an older one is
  dropped), acknowledges it in every self-snapshot (`me.ps`) and the client
  resends while its seq is ahead, so a lost, duplicated or reordered message
  is harmless. A car racer whose wish is "in" leaves the track: the car is
  parked out of the world (not sent in `cars`, not driven, nothing hits it,
  no collisions) and the driver is held like a body that vanished (`me.dead`,
  no corpse, no death counted, no commands run: there is nothing to predict);
  `me.pit` and the race list's `pit` say so (PIT in the standings). The race
  clock runs on and no wreck countdown starts. The shop is open (`me.buy`).
  Leaving the garage puts them back on the last gate they passed (a wreck's
  respawn place; the grid slot before the line) in a car built from their
  garage, repaired and refilled, their guns, armour and HP kept: the time in
  the pit is the penalty. A racer wrecked when they went in still waits out
  what is left of the wreck's countdown. The race ending takes everyone out
  of the pit onto their gate; GO drops the wishes made on the grid. Mid-race,
  someone not racing a car (no grid slot) can shop without a pit stop.
- **Changing car**: every car type bought this match stays yours
  (`me.gar.own`; the free buggy always is): picking one you own again is free
  ("car:<type>" costs nothing). A bay's label says SELECTED (the one you
  drive), OWNED or the price; an owned car on the turntable has DRIVE THIS on
  the spec card. A new match starts everyone stock with nothing owned.
- **Guns**: G in the garage opens the CS buy menu (guns, gear, grenades) in its
  place; closing it comes back to the garage (still the pit stop). Its 7 Cars,
  8 Car weapons, 9 Upgrades tabs stay too.
- **Parkour** (pk_ maps, Map.OnFoot): no cars, so no garage; B is the plain
  buy menu there.
- `mp_startmoney` (CS) sets the match's start money (-1: economy.json's).

- **The bay** (assets/garage.js): its own renderer on a canvas over the
  game's, made on first use and kept (the match's map, lights and programs are
  never touched; leaving is instant). Concrete floor and walls, a roller door
  under the sign "GARAGE · NO REFUNDS", a tool wall and bench, ceiling lamps,
  a turntable, and a row of four bays behind it, one per car type.
- **Your car on the turntable**, the real animated model (models/car.js) with
  its kit (models/carkit.js): the front item on the nose (a lens emitter, a
  rocket pod, an orb cradle, a ram plate), the rear one on a hitch (a spike
  hopper, an oil drum, a mine rack), plating panels down the flanks, a blower
  and exhaust stacks for the engine, fatter treads (grousers on a tank), a
  boost tank cylinder on the flank, ammo crates for charge packs; more of each
  per tier. The spec card: tier, top speed, car HP, boost tank, the items and
  their charges a lap, every upgrade's tier: the server's own numbers
  (internal/game car_shared.go `Fit*`, which fitCar uses too).
- **Shopping on the car**: every part has a target on the model (front and
  rear mount, charges, boost tank, plating, engine, tyres); pointing at one
  lights the part up and says what it does; a click opens just that slot's
  options with prices, bought in place, and the table turns the part to face
  you. A parked car's label shows its price; a click rolls it onto the
  turntable for a look (as it would come: its own built-ins, your upgrade
  tiers, which carry over), its label (or BUY on the card) then buys it.
  OWNED is green, a price you can afford white, one you can't red with
  "can't afford"; with the shop closed (over the last race's results) it is
  look only. The parts list on the right is the docked shop (every slot, what
  is fitted, the next price).
- **Keys**: arrows walk the bays (each rolls onto the table), Down to the
  parts and into one's options, Enter buys / opens, Backspace and Up step back,
  Esc steps back then leaves, H leaves. Drag to orbit, wheel to zoom.
- The warmup's buys are the warmup's: a new match starts everyone stock
  (match.go), and the garage says so in the warmup.

Tests: web/game/garage.test.mjs (when it opens, what it shows, owned and
selected cars, the card's numbers, keys, the kit, the room), internal/game
TestFitNumbers and pitstop_test.go (the pit parks the car and keeps the
clock, leaving at the last gate in the garage's car with the guns kept,
resend/dup/reorder of `{t:"pit"}`, a wreck into the pit, the race ending in
it, the garage from anywhere: far away, mid-air, on foot and in a car, owned
cars free, reset at a new match, mp_startmoney), sim/cases `race-garage` (H
through the real key: no movement while open, the grid's hint, GO closes it,
a pit stop in the live race) and `race-pitstop` (B mid-race: the pit, 4 s in
it without a wreck, the racecar bought there, out on the start line's spot in
it, back to the owned buggy for free), scenarios `race-pitstop` (the grid's
hint, the pit stop, back on the track behind, DRIVE THIS) and `race-garage` (the stock buggy, the
tank with plating, tank, engine, tyres, charges and the ram plate, a part's
options, a car rolling in, the bays, the grid with the countdown, driving
off after GO).

## Phase 5: bots that race (internal/game ai_race.go)

A racing bot drives with the same commands a player's keys make (throttle
and steering are analog for it):

- **The racing line**: `Map.RaceLine`, hand-placed points on the road in
  driving order (wide into a corner, the apex inside, out wide), a closed
  Catmull-Rom spline sampled every 1.5 m with its curvature. A map without
  one has bots drive gate to gate. A bot keeps its place on the line by
  searching a window around its last place (so it never jumps to the road
  above or below at a crossover: height counts triple); off the line by
  14 m (a wreck brought it back at a gate) it searches the whole line.
- **Steering**: for the line point 4 m + 0.45 s of speed ahead.
- **Speed**: what the curves ahead allow, the lower of the car's turn rate
  (times a margin per drive: wheels 0.55, tracks 0.6, hover 0.5) over the
  curvature and its grip (a sideways acceleration of 8 m/s² per unit of grip:
  the hovercraft, grip 1.2, slides out of a corner however fast its nose
  comes round), braking for each in time (at 0.6 of the car's brake), times
  its skill's share of top speed (bot_difficulty: 0.8 .. 1). Boost on
  straights. Sliding (the nose 0.22 rad off the velocity) it eases off until
  the slide settles, except with a jump ahead (the line climbing 1 m within
  30 m): a jump it takes at 85% of top speed at least.
- **Stuck** (under 1.5 m/s with the throttle on for 1.5 s): it backs off
  for 1.2 s on the other lock.
- **Items**: the front item at a car straight ahead and in sight (orbs: any
  car ahead, they home), the rear one on a car close behind.
- **Guns**: the driver shoots the nearest enemy racer within 25 m (its view
  is free while the car steers), with three times its standing aim error,
  not in the first 5 s after GO.
- **The shop**: on the grid, the best car it can afford but the hovercraft
  (a bot takes it slowly), then a tier of each upgrade in turn while the
  money lasts.

Tests (ai_race_test.go): bots race two laps of every race map
(TestBotsRaceEveryTrack); one of each car type on every rc_ track, peaceful
(`aiPeace`: nobody fires), never falls off it and all but the hovercraft
finish two laps (TestBotsDriveEveryCar); a bot race replays the same; the
shop; backing off a rail; finding its place on the line (the crossover's
levels). Scenario `race-bots`.

**More tracks** (the floating-roads style; `maps/racekit.go` builds them:
decks, low rails, pillars, arches, signs, gates with each track's own look):

- `rc_merge_train`: diffs paved on pillars over a sea of molten merge
  conflicts (materials `diff`, `conflict`, `conflictsign`; a red haze). A
  banked hairpin, 26 m to straighten up and a jump over an 8 m gap, a narrow
  bridge, the middle leg diving into a trench and under the start straight
  (the underpass, 6 m down), the merge (the road splits round a slot of void
  into two 4.5 m lanes and joins again), banked right-handers, a pit lane.
  About 840 m a lap.
- `rc_hotfix_highway`: an elevated highway at dusk (sky `dusk`) over a
  city's lights (`highway`: lane paint and amber marker lights; `nightcity`;
  green gantries, `exitsign`; concrete barriers). A banked corner, a narrow
  bridge, a jump, the interchange flying 8 m over the start straight, and the
  road works: the highway detours round a closed block (four corners), or,
  for the brave, straight on up a narrow ramp and over a 9 m gap (no gate on
  either way). The longest: about 1.1 km a lap. The bots go round.

Every rotation race map has a racing line (`TestDriveTheTracks` drives it in
every car with a simple autopilot; `lab_race`, without one, has a line in
the test). Scenarios `map-rc_merge_train`, `map-rc_hotfix_highway`.

## Power-ups (internal/game pickups.go; web/game/pickups.js)

Pickups you drive through on an rc_ track (`content.Map.Pickups`,
`mapgen.Pickup(kind, x, y, z)`; mapcheck checks the kind and the respawn):

| kind       | name           | does                                                     | back after |
|------------|----------------|----------------------------------------------------------|-----------:|
| `bonus`    | Bonus Approved | money to the driver (economy.json `race.bonus`, $500)    | 30 s |
| `hotfix`   | Hotfix Patch   | repairs 40% of the car's max HP                          | 20 s |
| `reorg`    | Reorg Budget   | the front and rear items' charges full again             | 20 s |
| `caffeine` | Caffeine       | the boost tank full                                      | 15 s |
| `freeze`   | Code Freeze    | 4 s of no damage (`damageCar` ignores it; not a fall)    | 25 s |
| `flag`     | Feature Flag   | one of the others at random (the server's seeded rng)    | 20 s |

- **Server-owned**: each pickup's kind, place and whether it is up ride in
  every snapshot's round (`pu`: `{k, p, up}`). The effects land in the car's
  state (HP, boost, charges, `sh`: the shield) and the driver's money, so a
  lost event changes nothing; the `car` event `pickup` (`k`: what it did,
  `via: "flag"`: it was a Feature Flag) only plays the chime, the flash, the
  caller's line (`pu_<kind>`, ours only) and the feed.
- A car whose middle is within 2.5 m across (and 2.5 m up or down) takes
  it, in a live race only; a new race puts every one back.
- **Placement**: on the line, the riskier spots the better: in the air over
  each jump's gap (a car flies through it), on the narrow bridges, on the
  flyover/interchange/underpass; on rc_merge_train a Feature Flag in the
  merge's other lane, on rc_hotfix_highway one over the shortcut's gap (the
  bots don't drive either: they're for players). None of the useful ones
  right after the line: a lap refills the boost and the charges.
- **Bots** (`raceDetour`): a pickup up ahead (6..45 m, within 0.45 rad of
  the nose, at most 7 m off the line, not on a jump's ramp) that is worth it
  (`pickupWorth`: a repair when under 75% HP, caffeine under half a tank,
  the charges when any are spent; money, the freeze and the flag always):
  they steer through the nearest.
- **Drawn**: a crystal in the kind's colour round a white core, a halo
  turning round it and a ring of dashes on the road, bobbing and pulsing; a
  shielded car gets two turning rings of ice-blue dashes and the HUD's car
  badge says CODE FREEZE. Sound: `carPickup` (a two-note chime by kind,
  coins, a ratchet, fizz, a glassy ring; `pickup_<kind>` in the gallery).
- Tests: `TestPickupsEachKind`, `TestPickupRespawnsAndOneCarTakesIt`,
  `TestPickupsOnlyInALiveRace`, `TestFeatureFlagIsEachOfTheOthers`,
  `TestPickupWorth`, `TestBotDetoursForAPickup`,
  `TestBotsTakePickupsEveryTrack` (3 laps of 2 v 2 on each track: every
  pickup on the bots' line taken); web `pickups.test.mjs`, `cars.test.mjs`
  (the badge); scenario `race-pickups`.

## Parkour: a race on foot (internal/game race.go, parkour.go)

A pk_ map (category Parkour) is a race map with `Map.OnFoot`
(`mapgen.OnFoot(p2p)`): the race rules above, run on foot.

- **The start**: the team spawns (both teams need some); the freeze time is
  the countdown (bodies are held, as in any freeze). Nobody is put in a car;
  any number race (no grid slots).
- **Point to point** (`Map.PointToPoint`): the gates once, the first the
  start, the last the finish (`round.p2p`; laps forced to 1). Without it a
  course laps like a track.
- **Falls**: into a pit (under each section of the course) is a respawn on
  the last gate passed at once, facing along the course, keeping the
  progress (`raceSpawn`; no death); out of the world the body is put back
  there too. A death (combat, a grenade) is back there after the respawn
  time.
- **Splits**: every gate passed sets the racer's split (seconds from GO,
  `sp`) and, once they have a best run of this course, its delta against
  the best's split at that gate (`dl`, `hd`). A finish faster than the best
  (or the first) is the new best (`best`; `pb` the best time). The bests are
  the server's memory (by course and player name) while it runs. The HUD:
  GATE n/N, the clock, the split with its delta (green ahead, red behind),
  BEST; the finish says NEW BEST!.
- **Combat**: `mp_race_combat` (default 1: the race's free for all; a map may
  pin it, lab_parkour pins 0): 0 spares everyone (`spared`: bullets, melee,
  grenades, fire, car items, a car's damage by a racer); your own still hurts.
- **Vehicles**: `Map.Cars` and `Map.Boats` are free vehicles at stations for
  the course's legs, not a grid: anyone gets in and out (no lock); one left
  alone more than 4 m from its station for 6 s goes back there, a car
  repaired (`tickStations`), for the next racer.
- **Bots** run a course that has a route (`Map.Route`, `mapgen.Route`;
  internal/game ai_parkour.go), and aren't racers on one without. A route is
  steps in order, grouped by the gate they start from (`from`): `run` at a
  point, `jump`/`hop` at an edge (a hop lets go of the keys in the air: onto
  a lift), `back` (a run-up), `rest` (a pad chain's flight, a skid), `wait`
  (the movers' clock at a phase: a lift), `leap` + `hook` + `wall` (hook a
  face, walk up and over it), `board`/`drive`/`leave` (a car or boat at a
  station), `swim` (out of the water onto a ledge). The bot sends real
  commands (runCmd, the same sim as players); the edge and water guards of
  the other modes are off. A fall puts it back on its gate (the race's rule)
  and it starts that gate's steps again; a step not done in 25 s puts it
  back there too. `TestBotRunsTheHappyPath` (both movement models: bots
  finish pk_happy_path), `TestParkourBotStuckGoesBack`; scenario `pk-bots`.
- **The ghost of your best run** (web/game/ghost.js; any race map, tracks
  too): the client samples your own run on the race clock (10 Hz: where you
  are, your facing, on foot / in a car / a boat); a finish faster than the
  best kept for the map (or the first) becomes the map's best, in
  localStorage (`counterslop.ghost.<map>`, every access in try/catch: a
  private window keeps it for the page only). The next race plays it back
  on the race clock: a see-through, pale cyan copy of the model you were in
  (the renderer draws solids with `alpha` < 1 last, blended, no depth
  writes or shadows). `cl_ghost` (default 1) hides it; it still records.
  Client only: the server never sees it. Tests: web `ghost.test.mjs`, case
  `parkour-course` (it records; `cl_ghost`), scenario `race-ghost`.
- mapcheck (`raceProblems`): pk_* (or lab_*), category Parkour, team spawns,
  a pit; point to point only on foot.
- Tests: `TestParkourOnFoot` (on foot, the gates, a fall back on the last
  gate, a death back there, the finish, out of order), `TestParkourSplitsAgainstTheBest`,
  `TestRaceCombatOff`, `TestParkourStations`, `TestParkourNoBots`, `TestBotRunsTheHappyPath`,
  `TestParkourModeProblems`; web `race.test.mjs`; case `parkour-course`
  (lab_parkour: on foot through the start gate, `mp_race_combat`).

### pk_happy_path: localhost to prod

The first course (internal/content/maps/pk_happy_path.go), ~760 m east along
a deployment pipeline floating in space, ten gates, a section each: lint
(stepping stones), build (a chain of jump pads), staging (three lifts out of
step), the void (10 m gaps between tall blocks: hook the next one's face and
walk up it), wall run (a 60 m rack of servers: hook it, walk up and over),
canary (0.3 g, long hops down glass slabs), deploy (a station of buggies, a
road with a 10 m jump), rollout (dinghies across a basin, a water jump out)
and the finish at prod. Rules: `mp_race_combat 0`, `mp_gravityboots 2`
(everyone wears the boots), `sv_hook_recharge 40` (the void and the wall hook
four times in a row), three races a map.

`TestHappyPathRuns` runs every section with the real movement code in both
movement models (the stones jumped, the pads flown, the lifts timed, the
hooks and wall walks, the low-gravity hops), the buggy over the jump with the
car physics and a dinghy across the basin, then the swim and the water jump
out, each from its gate's spawn to the next gate, never into a pit. Change a
section, run it.
