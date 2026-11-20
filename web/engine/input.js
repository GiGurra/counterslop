// Keyboard + pointer-lock mouse. Movement keys use e.code so layouts don't matter.
export function createInput(canvas, wantRaw = () => true) {
  const keys = new Set();
  const st = { dx: 0, dy: 0, fire: false, locked: false, jumpQueued: false, onKey: null, onLock: null };

  addEventListener("keydown", (e) => {
    if (e.target.tagName === "INPUT") return;
    if (e.code === "Tab") e.preventDefault();
    // In game, Ctrl/Meta+key is crouch-strafing, not a shortcut (Ctrl+D bookmark, Ctrl+S save...).
    // Reserved ones (Ctrl+W/T/N) ignore this in a window; fullscreen keyboard lock and beforeunload cover them.
    if (st.locked && (e.ctrlKey || e.metaKey)) e.preventDefault();
    // With the whole keyboard locked, Esc reaches the page instead of the browser
    // (holding it still leaves fullscreen), so release the mouse ourselves.
    if (e.code === "Escape" && st.locked) document.exitPointerLock();
    if (!e.repeat) {
      if (e.code === "Space") st.jumpQueued = true;
      st.onKey?.(e.code, true);
    }
    keys.add(e.code);
  });
  addEventListener("keyup", (e) => { keys.delete(e.code); st.onKey?.(e.code, false); });
  addEventListener("blur", () => { keys.clear(); st.fire = false; });
  // Test harness: hold exactly these key codes (no DOM events involved).
  st.setHeld = (codes) => { keys.clear(); for (const c of codes) keys.add(c); };
  // Drop every held key (opening a menu or the console must not leave W stuck).
  st.releaseKeys = () => { keys.clear(); st.fire = false; st.jumpQueued = false; };
  addEventListener("mousemove", (e) => {
    if (!st.locked) return;
    st.dx += e.movementX; st.dy += e.movementY;
  });
  addEventListener("mousedown", (e) => { if (st.locked && e.button === 0) st.fire = true; });
  addEventListener("mouseup", (e) => { if (e.button === 0) st.fire = false; });
  canvas.addEventListener("click", () => st.locked || (st.onClickLock ? st.onClickLock() : st.lock()));
  document.addEventListener("pointerlockchange", () => {
    st.locked = document.pointerLockElement === canvas;
    if (!st.locked) st.fire = false; // keys stay: you can keep walking while re-aiming
    st.onLock?.(st.locked);
  });

  // Raw (unaccelerated) counts make CS-style sensitivity values mean the same thing here.
  st.lock = () => {
    if (document.pointerLockElement === canvas) return;
    let p;
    try { p = canvas.requestPointerLock(wantRaw() ? { unadjustedMovement: true } : undefined); } catch { p = null; }
    p?.catch?.((e) => { if (e?.name === "NotSupportedError") canvas.requestPointerLock()?.catch?.(() => {}); });
  };
  st.down = (code) => keys.has(code);

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
  // A Ctrl bind accepts either Ctrl key. In a window Ctrl+W can still close the
  // tab; the beforeunload confirm in main.js is the safety net there.
  const held = (k) => keys.has(k) || (/^Control/.test(k) && (keys.has("ControlLeft") || keys.has("ControlRight")));
  st.crouching = (...ks) => ks.some((k) => k && held(k));
  // Movement axes for physics; jump is consumed once per press (hold to bhop).
  st.move = (...crouchKeys) => {
    const k = (c) => (keys.has(c) ? 1 : 0);
    const jump = st.jumpQueued || keys.has("Space");
    st.jumpQueued = false;
    return {
      f: k("KeyW") - k("KeyS"),
      r: k("KeyD") - k("KeyA"),
      walk: keys.has("ShiftLeft") || keys.has("ShiftRight"),
      jump,
      crouch: st.crouching(...crouchKeys),
    };
  };
  st.takeMouse = () => { const d = [st.dx, st.dy]; st.dx = st.dy = 0; return d; };
  return st;
}
