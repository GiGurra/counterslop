// Every bindable action (ui/binds.js ACTIONS) reaches the command: press its
// default key, build the next command, and check the bit, axis, slot switch or
// message it produced. A new action without an entry in EXPECT fails here.
import test from "node:test";
import assert from "node:assert/strict";
import { ACTIONS, DEFAULT_BINDS } from "../ui/binds.js";
import { BTN } from "../engine/cmd.js";
import { createControls, createLatch, LATCHED } from "./controls.js";
import { createArsenal } from "./arsenal.js";

const SLOTS = { primary: { id: "prompt", ammo: 20, reserve: 90 }, sidearm: { id: "stub", ammo: 12, reserve: 48 }, melee: { id: "keyboard", ammo: 0, reserve: 0 } };
const still = { vel: [0, 0, 0], onGround: true, crouched: false, yaw: 0.25, pitch: -0.1 };

function rig({ cur = "primary", slots = SLOTS } = {}) {
  const held = new Set(), sent = [];
  const arsenal = createArsenal({ net: { send: (m) => sent.push(m) }, hud: { set() {} } });
  arsenal.setInv({ cur, slots: structuredClone(slots) });
  const c = createControls({ binds: () => DEFAULT_BINDS, isDown: (code) => held.has(code), arsenal });
  let seq = 0;
  // A key press as main.js routes it: held actions are sampled (and pressHeld on
  // the down edge); run-once actions go through the console to controls.action.
  const press = (code) => {
    const cmd = DEFAULT_BINDS[code];
    if (cmd[0] === "+") { held.add(code); c.pressHeld(cmd); } else c.action(cmd);
  };
  const next = () => c.buildCmd({ play: true, me: still, seq: ++seq, dt: 0.01 });
  return { c, arsenal, held, sent, press, next, release: (code) => held.delete(code) };
}

// action -> what pressing its default key does to the next command(s).
const EXPECT = {
  "+forward": (cmd) => assert.equal(cmd.f, 1),
  "+back": (cmd) => assert.equal(cmd.f, -1),
  "+moveleft": (cmd) => assert.equal(cmd.r, -1),
  "+moveright": (cmd) => assert.equal(cmd.r, 1),
  "+jump": (cmd) => assert.equal(cmd.b & BTN.JUMP, BTN.JUMP),
  "+duck": (cmd) => assert.equal(cmd.b & BTN.CROUCH, BTN.CROUCH),
  "+speed": (cmd) => assert.equal(cmd.b & BTN.WALK, BTN.WALK),
  "+attack": (cmd) => assert.equal(cmd.b & BTN.FIRE, BTN.FIRE),
  "+use": (cmd) => assert.equal(cmd.b & BTN.USE, BTN.USE),
  "+hook": (cmd) => assert.equal(cmd.b & BTN.HOOK, BTN.HOOK),
  "+zoom": (cmd, r) => { assert.equal(r.arsenal.zoomWant, true, "toggle mode: the press turns the zoom on (scoped once raised)"); assert.equal(cmd.b & BTN.ALT, BTN.ALT, "attack2 held (a grenade's lob)"); },
  reload: (cmd) => assert.equal(cmd.b & BTN.RELOAD, BTN.RELOAD),
  drop: (cmd, r) => assert.deepEqual(r.sent, [{ t: "drop" }]),
  slot1: (cmd) => assert.equal(cmd.w, "primary"),
  slot2: (cmd) => assert.equal(cmd.w, "sidearm"),
  slot3: (cmd) => assert.equal(cmd.w, "melee"),
  slot4: (cmd) => assert.equal(cmd.w, "flash"), // the first grenade carried
  slot5: (cmd) => assert.equal(cmd.w, "bomb"), // The Big Diff (carrier only)
  lastinv: (cmd) => assert.equal(cmd.w, "sidearm"), // last = sidearm on a fresh loadout
  invnext: (cmd) => assert.equal(cmd.w, "sidearm"),
  invprev: (cmd) => assert.equal(cmd.w, "melee"),
  // UI actions: no command; covered by lockstep cases (scoreboard, team, buy menu, console).
  "+showscores": "ui", teammenu: "ui", toggleconsole: "ui", buymenu: "ui", buyammo1: "ui", buyammo2: "ui", messagemode: "ui", messagemode2: "ui", radio: "ui",
};

test("every bindable action has an expectation", () => {
  const missing = ACTIONS.map((a) => a.cmd).filter((c) => !(c in EXPECT));
  assert.deepEqual(missing, [], "add these to EXPECT (and a sim/cases case if they change gameplay)");
});

// Starting loadout per action where the default doesn't show it.
const SETUP = {
  "+zoom": { slots: { primary: { id: "codereview", ammo: 5, reserve: 20 } } },
  slot1: { cur: "sidearm" },
  slot4: { slots: { flash: { id: "stacktrace", ammo: 2, reserve: 0 }, smoke: { id: "smoketest", ammo: 1, reserve: 0 } } },
  slot5: { slots: { bomb: { id: "bigdiff", ammo: 0, reserve: 0 } } },
};
const MOVES = ["+forward", "+back", "+moveleft", "+moveright"], SWITCHES = ["slot1", "slot2", "slot3", "slot4", "slot5", "lastinv", "invnext", "invprev"];

for (const a of ACTIONS) {
  if (EXPECT[a.cmd] === "ui") continue;
  test(`${a.label} (${a.def} -> ${a.cmd}) reaches the command`, () => {
    const r = rig(SETUP[a.cmd]);
    r.next(); // a baseline command with nothing held
    r.press(a.def);
    const cmd = r.next();
    EXPECT[a.cmd](cmd, r);
    // Nothing else leaks into the command.
    if (!MOVES.includes(a.cmd)) assert.deepEqual([cmd.f, cmd.r], [0, 0], "move axes leaked");
    if (!SWITCHES.includes(a.cmd)) assert.equal(cmd.w, undefined, "slot switch leaked");
  });
}

test("held actions stop with the key; run-once actions ride in exactly one command", () => {
  const r = rig();
  r.press("KeyW");
  assert.equal(r.next().f, 1);
  r.release("KeyW");
  assert.equal(r.next().f, 0);
  r.press("KeyR");
  assert.equal(r.next().b & BTN.RELOAD, BTN.RELOAD);
  assert.equal(r.next().b & BTN.RELOAD, 0);
  r.press("Digit2");
  assert.equal(r.next().w, "sidearm");
  assert.equal(r.next().w, undefined);
});

test("a jump tap shorter than a tick still jumps once; holding gives one edge", () => {
  const r = rig();
  r.press("Space"); r.release("Space"); // down and up between two commands
  assert.equal(r.next().b & BTN.JUMP, BTN.JUMP);
  assert.equal(r.next().b & BTN.JUMP, 0);
  r.press("Space");
  const a = r.next().b & BTN.JUMP, b = r.next().b & BTN.JUMP;
  assert.deepEqual([a, b], [BTN.JUMP, BTN.JUMP], "held: the bit stays set (the edge is the sim's business)");
});

test("latch: a press reaches one sample even if released first; held is held; a re-press while set gets a released sample first", () => {
  const l = createLatch();
  assert.equal(l.sample(false), false);
  l.press(); // down and up between two samples
  assert.deepEqual([l.sample(false), l.sample(false)], [true, false], "the tap shows up once, never longer");
  l.press(); // pressed and held
  assert.deepEqual([l.sample(true), l.sample(true), l.sample(false)], [true, true, false], "held: set exactly while held");
  l.sample(true);
  l.press(); // released and pressed again between two samples while set
  assert.deepEqual([l.sample(true), l.sample(true)], [false, true], "the re-press is an edge too");
  l.press(); l.reset();
  assert.equal(l.sample(false), false, "reset drops a queued press");
});

for (const [cmd, code, bit] of [["+attack", "Mouse0", BTN.FIRE], ["+use", "KeyE", BTN.USE], ["+jump", "Space", BTN.JUMP], ["+zoom", "Mouse2", BTN.ALT], ["+hook", "KeyF", BTN.HOOK]]) {
  test(`${cmd}: a click shorter than the gap between two commands still reaches the next one, once`, () => {
    assert.ok(LATCHED.includes(cmd));
    const r = rig();
    r.next();
    r.press(code); r.release(code); // a frame hitch longer than the click: no command saw it held
    assert.equal(r.next().b & bit, bit);
    assert.equal(r.next().b & bit, 0, "a tap never lasts longer than one command");
    r.press(code);
    assert.deepEqual([r.next().b & bit, r.next().b & bit], [bit, bit], "held: set while held");
    r.release(code);
    assert.equal(r.next().b & bit, 0, "released: clear at once");
  });
}

test("a click during code freeze or in a menu is used up there, not fired when it ends", () => {
  const r = rig();
  r.press("Mouse0"); r.release("Mouse0"); r.press("Space"); r.release("Space");
  assert.equal(r.c.buildCmd({ play: true, me: still, seq: 50, dt: 0.01, frozen: true }).b & (BTN.FIRE | BTN.JUMP), 0);
  assert.equal(r.next().b & (BTN.FIRE | BTN.JUMP), 0, "freeze over: no late shot or jump");
  r.press("Mouse0"); r.release("Mouse0");
  assert.equal(r.c.buildCmd({ play: false, me: still, seq: 60, dt: 0.01 }).b & BTN.FIRE, 0);
  assert.equal(r.next().b & BTN.FIRE, 0, "back in play: no late shot");
});

test("pressing the same slot, or an empty slot, sends no switch", () => {
  const r = rig({ slots: { primary: SLOTS.primary, melee: SLOTS.melee } });
  r.press("Digit1");
  assert.equal(r.next().w, undefined);
  r.press("Digit2");
  assert.equal(r.next().w, undefined);
});

test("nothing reaches the command when not playing (menu, team select)", () => {
  const held = new Set(["KeyW", "Mouse0"]);
  const arsenal = createArsenal({ net: { send() {} }, hud: { set() {} } });
  arsenal.setInv({ cur: "primary", slots: structuredClone(SLOTS) });
  const c = createControls({ binds: () => DEFAULT_BINDS, isDown: (k) => held.has(k), arsenal, playing: () => false });
  c.action("slot2");
  const cmd = c.buildCmd({ play: false, me: still, seq: 1, dt: 0.01 });
  assert.deepEqual([cmd.f, cmd.b, cmd.w], [0, 0, undefined]);
});

test("the command carries the view angles and the scope bit", () => {
  const r = rig({ slots: { primary: { id: "codereview", ammo: 5, reserve: 20 } } });
  const cmd = r.next();
  assert.deepEqual([cmd.yaw, cmd.pitch, cmd.ms], [0.25, -0.1, 10]);
  for (let i = 0; i < 40; i++) r.arsenal.tick(0.01, false, still, () => {});
  r.press("Mouse2");
  r.arsenal.tick(0.01, false, still, () => {});
  assert.equal(r.next().b & BTN.SCOPE, BTN.SCOPE);
});

test("code freeze (round freeze time): no moving, jumping, firing or using; looking and crouching still work", () => {
  const r = rig();
  for (const code of ["KeyW", "KeyD", "Space", "Mouse0", "KeyE", "ControlLeft"]) r.press(code);
  const cmd = r.c.buildCmd({ play: true, me: still, seq: 99, dt: 0.01, frozen: true });
  assert.equal(cmd.f, 0);
  assert.equal(cmd.r, 0);
  assert.equal(cmd.b & (BTN.JUMP | BTN.FIRE | BTN.USE), 0);
  assert.equal(cmd.b & BTN.CROUCH, BTN.CROUCH);
  assert.equal(cmd.yaw, still.yaw);
});
