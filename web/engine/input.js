// Keyboard + pointer-lock mouse, as plain codes: KeyboardEvent.code for keys
// (layout independent), Mouse0..MouseN (raw event.button) for buttons and MWheelUp/MWheelDown for
// wheel notches (a press and release at once). What a code does is up to the
// game (its binds); onKey(code, down) reports every edge.
// Mouse buttons are bound by the raw DOM event.button the browser reports
// (0 primary, 1 middle, 2 secondary, 3/4 back/forward on most mice): no
// remapping of our own, so whatever the player presses is what gets bound.
// (Never event.buttons: that bitmask orders secondary/middle differently.)
const MOUSE = (b) => `Mouse${b}`;

export function createInput(canvas, wantRaw = () => true) {
  const keys = new Set();
  const st = { dx: 0, dy: 0, locked: false, onKey: null, onLock: null };
  const press = (code) => { if (!keys.has(code)) { keys.add(code); st.onKey?.(code, true); } };
  const release = (code) => { if (keys.delete(code)) st.onKey?.(code, false); };

  addEventListener("keydown", (e) => {
    if (e.target.tagName === "INPUT") return;
    if (e.code === "Tab") e.preventDefault();
    // In game, Ctrl/Meta+key is crouch-strafing, not a shortcut (Ctrl+D bookmark, Ctrl+S save...).
    // Reserved ones (Ctrl+W/T/N) ignore this in a window; fullscreen keyboard lock and beforeunload cover them.
    if (st.locked && (e.ctrlKey || e.metaKey)) e.preventDefault();
    // With the whole keyboard locked, Esc reaches the page instead of the browser
    // (holding it still leaves fullscreen), so release the mouse ourselves.
    if (e.code === "Escape" && st.locked) document.exitPointerLock();
    press(e.code);
  });
  addEventListener("keyup", (e) => release(e.code));
  const releaseAll = () => { for (const c of [...keys]) release(c); };
  addEventListener("blur", releaseAll);
  // Drop every held code (opening a menu or the console must not leave W stuck).
  st.releaseKeys = releaseAll;
  // Test harness: hold exactly these codes (edges are reported like real ones).
  st.setHeld = (codes) => {
    for (const c of [...keys]) if (!codes.includes(c)) release(c);
    for (const c of codes) press(c);
  };
  addEventListener("mousemove", (e) => {
    if (!st.locked) return;
    st.dx += e.movementX; st.dy += e.movementY;
  });
  addEventListener("mousedown", (e) => { if (st.locked) press(MOUSE(e.button)); });
  addEventListener("mouseup", (e) => release(MOUSE(e.button)));
  // No browser context menu anywhere in the game (right-click is a game
  // button, also while binding it in Controls), except in text fields.
  addEventListener("contextmenu", (e) => { if (!e.target?.closest?.("input, textarea")) e.preventDefault(); });
  // Wheel: one notch per ~50 units (trackpads send many small deltas).
  let wheelAcc = 0;
  addEventListener("wheel", (e) => {
    if (!st.locked) return;
    wheelAcc += e.deltaY;
    if (Math.abs(wheelAcc) >= 50) {
      const code = wheelAcc > 0 ? "MWheelDown" : "MWheelUp";
      wheelAcc = 0;
      st.onKey?.(code, true); st.onKey?.(code, false);
    }
  }, { passive: true });
  canvas.addEventListener("click", () => st.locked || (st.onClickLock ? st.onClickLock() : st.lock()));
  document.addEventListener("pointerlockchange", () => {
    st.locked = document.pointerLockElement === canvas;
    if (!st.locked) for (const c of [...keys]) if (c.startsWith("Mouse")) release(c); // keys stay: keep walking while re-aiming
    st.onLock?.(st.locked);
  });

  // Raw (unaccelerated) counts make CS-style sensitivity values mean the same thing here.
  st.lock = () => {
    if (document.pointerLockElement === canvas) return;
    let p;
    try { p = canvas.requestPointerLock(wantRaw() ? { unadjustedMovement: true } : undefined); } catch { p = null; }
    p?.catch?.((e) => { if (e?.name === "NotSupportedError") canvas.requestPointerLock()?.catch?.(() => {}); });
  };

  // Fullscreen + Keyboard Lock (Chromium): lets Ctrl crouch without Ctrl+W closing the tab.
  st.kbLocked = false;
  st.enterFullscreen = async () => {
    try {
      if (!document.fullscreenElement) await document.documentElement.requestFullscreen({ navigationUI: "hide" });
      // Whole keyboard: every Ctrl+key (W, D, S, R, ...) reaches the game instead of the browser.
      await navigator.keyboard?.lock?.();
      st.kbLocked = !!navigator.keyboard?.lock;
    } catch { st.kbLocked = false; }
    return st.kbLocked;
  };
  document.addEventListener("fullscreenchange", () => { if (!document.fullscreenElement) st.kbLocked = false; });
  st.held = () => [...keys];
  // Whether a code is held. A Ctrl/Shift/Alt code accepts either side's key.
  st.down = (code) => {
    if (keys.has(code)) return true;
    const m = /^(Control|Shift|Alt)(Left|Right)$/.exec(code);
    return !!m && (keys.has(m[1] + "Left") || keys.has(m[1] + "Right"));
  };
  st.takeMouse = () => { const d = [st.dx, st.dy]; st.dx = st.dy = 0; return d; };
  return st;
}
