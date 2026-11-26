# Slop n Roll Racing

The living design doc of the race mode. The design (operator + lead) comes
first; what is built so far is under "Built" at the end.


A per-map game mode (map mode "race"), a silly combat-racing parody inside counterslop: you still play CS (your gun on the mouse) while driving a parody vehicle. Everything original: no characters, catchphrases, music, car designs or track layouts copied from Rock n' Roll Racing.

## Vehicles (cars.json, shared like weapons.json; data-driven sim)
Four tiers, bought in the pre-race shop, kept for the match (respawn in the same type; upgrades carry over). Top speed strictly increasing per tier, small steps ("it's a race after all"); tiers otherwise differ by handling / car HP / feel. Pin the ordering in a test.
- T1 buggy (starter, free, light, bouncy). Default front weapon: laser (Linter Beam, hitscan).
- T2 racecar (low, grippy, fragile-ish). Default front weapon: unguided rockets (Hotfix Missiles).
- T3 tank (tracks, heavy, most car HP, rams hard; still faster than the racecar). Default front weapon: rockets too (heavier/bigger splash than the racecar's, fewer charges is fine).
- T4 hovercraft (most expensive; no wheels: air cushion, drifty, ignores spikes/oil/water, mines still hit it). Default front weapon: guided energy balls (homing orbs, e.g. "Synergy Orbs"; slower projectiles that steer toward the nearest car ahead / the aimed target).
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
