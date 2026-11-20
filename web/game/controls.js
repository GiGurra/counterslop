// Gameplay controls: how bound keys become the per-tick command (engine/cmd.js).
// Held actions ("+forward", "+attack", ...) are sampled into each command; a
// jump press is queued so a tap shorter than a tick still jumps; run-once
// actions (reload, slotN, lastinv, invnext/invprev, drop) ask the arsenal,
// whose request rides in the next command. Pure: main.js wires it to the real
// input and console, web/game/controls.test.mjs drives it directly.
import { BTN } from "../engine/cmd.js";

export function createControls({ binds, isDown, arsenal, playing = () => true, zoomMode = () => "toggle" }) {
  let jumpQueued = false, jumpBit = false;

  // Whether any key bound to cmd is held.
  function down(cmd) {
    for (const [code, c] of Object.entries(binds())) if (c === cmd && isDown(code)) return true;
    return false;
  }
  // Jump button state for one command. A press since the last command shows up as
  // a down edge even if already released (a wheel notch, a tap shorter than a tick);
  // a re-press while the bit is still set gets one released command first.
  function jumpButton() {
    if (jumpQueued) {
      if (jumpBit) { jumpBit = false; return false; }
      jumpQueued = false;
      return (jumpBit = true);
    }
    return (jumpBit = down("+jump"));
  }

  return {
    down,
    // A held ("+") action's key went down: queue a jump, toggle the zoom.
    pressHeld(cmd) {
      if (cmd === "+jump") jumpQueued = true;
      else if (cmd === "+zoom" && playing() && zoomMode() === "toggle") arsenal.toggleScope();
    },
    queueJump() { jumpQueued = true; },
    // Run-once game commands (bound keys go through the console to here).
    action(cmd) {
      if (!playing()) return false;
      const a = {
        reload: () => arsenal.reload(), drop: () => arsenal.drop(),
        slot1: () => arsenal.switchTo("primary"), slot2: () => arsenal.switchTo("sidearm"), slot3: () => arsenal.switchTo("melee"),
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
      const jump = play && !frozen && jumpButton(), crouch = play ? d("+duck") : me.crouched;
      const wc = arsenal.takeCmd(); // weapon switch / reload asked for since the last tick
      const b = (jump ? BTN.JUMP : 0) | (crouch ? BTN.CROUCH : 0) | (d("+speed") ? BTN.WALK : 0) | (d("+attack") && !frozen ? BTN.FIRE : 0)
        | (arsenal.scoped ? BTN.SCOPE : 0) | (wc.reload ? BTN.RELOAD : 0) | (d("+use") && !frozen ? BTN.USE : 0);
      const f = frozen ? 0 : d("+forward") - d("+back"), r = frozen ? 0 : d("+moveright") - d("+moveleft");
      return { seq, ms: Math.round(dt * 1000), f, r, yaw: me.yaw, pitch: me.pitch, b, w: wc.w || undefined };
    },
    reset() { jumpQueued = jumpBit = false; },
  };
}
