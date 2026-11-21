// Every bindable action (ui/binds.js ACTIONS) reaches the command: press its
// default key, build the next command, and check the bit, axis, slot switch or
// message it produced. A new action without an entry in EXPECT fails here.
import test from "node:test";
import assert from "node:assert/strict";
import { ACTIONS, DEFAULT_BINDS } from "../ui/binds.js";
import { BTN } from "../engine/cmd.js";
import { createControls } from "./controls.js";
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
  "+zoom": (cmd, r) => assert.equal(r.arsenal.zoomWant, true, "toggle mode: the press turns the zoom on (scoped once raised)"),
  reload: (cmd) => assert.equal(cmd.b & BTN.RELOAD, BTN.RELOAD),
  drop: (cmd, r) => assert.deepEqual(r.sent, [{ t: "drop" }]),
  slot1: (cmd) => assert.equal(cmd.w, "primary"),
  slot2: (cmd) => assert.equal(cmd.w, "sidearm"),
  slot3: (cmd) => assert.equal(cmd.w, "melee"),
  lastinv: (cmd) => assert.equal(cmd.w, "sidearm"), // last = sidearm on a fresh loadout
  invnext: (cmd) => assert.equal(cmd.w, "sidearm"),
  invprev: (cmd) => assert.equal(cmd.w, "melee"),
  // UI actions: no command; covered by scenarios (scoreboard, team menu, console).
  "+showscores": "ui", teammenu: "ui", toggleconsole: "ui",
};

test("every bindable action has an expectation", () => {
  const missing = ACTIONS.map((a) => a.cmd).filter((c) => !(c in EXPECT));
  assert.deepEqual(missing, [], "add these to EXPECT (and a sim/cases case if they change gameplay)");
});

// Starting loadout per action where the default doesn't show it.
const SETUP = {
  "+zoom": { slots: { primary: { id: "codereview", ammo: 5, reserve: 20 } } },
  slot1: { cur: "sidearm" },
};
const MOVES = ["+forward", "+back", "+moveleft", "+moveright"], SWITCHES = ["slot1", "slot2", "slot3", "lastinv", "invnext", "invprev"];

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
