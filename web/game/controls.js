// Gameplay controls: how bound keys become the per-tick command (engine/cmd.js).
// Held actions ("+forward", "+duck", ...) are sampled into each command; a press
// of a latched action (+jump, +attack, +use) is queued so a tap shorter than the
// gap between two commands (a tick, or a stalled frame) still reaches one; run-once
// actions (reload, slotN, lastinv, invnext/invprev, drop) ask the arsenal,
// whose request rides in the next command. Pure: main.js wires it to the real
// input and console, web/game/controls.test.mjs drives it directly.
import { BTN } from "../engine/cmd.js";

// Held actions whose press must never be lost between two commands: a jump, a
// shot (a semi-auto click is an edge), a use (plant/defuse start).
export const LATCHED = ["+jump", "+attack", "+use", "+zoom"]; // +zoom: attack2, a grenade's lob

// One latched button. sample(held) gives its state for one command: a press since
// the last command shows up even if already released (a wheel notch, a click
// shorter than the gap between commands), for that one command only; a re-press
// while the last command still had it set gets one released command first, so
// the press is an edge there too. Held, it stays set exactly while held.
export function createLatch() {
  let queued = false, bit = false;
  return {
    press() { queued = true; },
    sample(held) {
      if (queued) {
        if (bit) { bit = false; return false; }
        queued = false;
        return (bit = true);
      }
      return (bit = held);
    },
    reset() { queued = bit = false; },
  };
}

export function createControls({ binds, isDown, arsenal, playing = () => true, zoomMode = () => "toggle" }) {
  const latches = Object.fromEntries(LATCHED.map((c) => [c, createLatch()]));

  // Whether any key bound to cmd is held.
  function down(cmd) {
    for (const [code, c] of Object.entries(binds())) if (c === cmd && isDown(code)) return true;
    return false;
  }
  // A latched button for one command. Always sampled, so a press made where it
  // does not count (frozen, not playing) is used up there instead of firing later.
  const latched = (cmd, counts) => latches[cmd].sample(down(cmd)) && counts;

  return {
    down,
    // A held ("+") action's key went down: latch it, toggle the zoom.
    pressHeld(cmd) {
      if (latches[cmd]) latches[cmd].press();
      if (cmd === "+zoom" && playing() && zoomMode() === "toggle") arsenal.toggleScope();
    },
    queueJump() { latches["+jump"].press(); },
    // Run-once game commands (bound keys go through the console to here).
    action(cmd) {
      if (!playing()) return false;
      const a = {
        reload: () => arsenal.reload(), drop: () => arsenal.drop(),
        slot1: () => arsenal.switchTo("primary"), slot2: () => arsenal.switchTo("sidearm"), slot3: () => arsenal.switchTo("melee"), slot4: () => arsenal.nextGrenade(), slot5: () => arsenal.switchTo("bomb"),
        lastinv: () => arsenal.lastWeapon(), invnext: () => arsenal.cycle(1), invprev: () => arsenal.cycle(-1),
      }[cmd];
      if (!a) return false;
      a();
      return true;
    },
    // Sample input into one command. play: input counts (in the match, no overlay).
    // frozen: round freeze time, look around only (the server applies the same: no move, jump, fire or use).
    buildCmd({ play, me, seq, dt, frozen = false }) {
      const d = (cmd) => (play && down(cmd) ? 1 : 0);
      const jump = latched("+jump", play && !frozen), fire = latched("+attack", play && !frozen), use = latched("+use", play && !frozen), alt = latched("+zoom", play && !frozen);
      const crouch = play ? d("+duck") : me.crouched;
      const wc = arsenal.takeCmd(); // weapon switch / reload asked for since the last tick
      const b = (jump ? BTN.JUMP : 0) | (crouch ? BTN.CROUCH : 0) | (d("+speed") ? BTN.WALK : 0) | (fire ? BTN.FIRE : 0)
        | (arsenal.scoped ? BTN.SCOPE : 0) | (wc.reload ? BTN.RELOAD : 0) | (use ? BTN.USE : 0) | (alt ? BTN.ALT : 0);
      const f = frozen ? 0 : d("+forward") - d("+back"), r = frozen ? 0 : d("+moveright") - d("+moveleft");
      return { seq, ms: Math.round(dt * 1000), f, r, yaw: me.yaw, pitch: me.pitch, b, w: wc.w || undefined };
    },
    reset() { for (const l of Object.values(latches)) l.reset(); },
  };
}
