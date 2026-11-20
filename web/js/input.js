// Keyboard + pointer-lock mouse. Movement keys use e.code so layouts don't matter.
export function createInput(canvas, wantRaw = () => true) {
  const keys = new Set();
  const st = { dx: 0, dy: 0, fire: false, locked: false, jumpQueued: false, onKey: null, onLock: null };

  addEventListener("keydown", (e) => {
    if (e.target.tagName === "INPUT") return;
    if (e.code === "Tab") e.preventDefault();
    if (!e.repeat) {
      if (e.code === "Space") st.jumpQueued = true;
      st.onKey?.(e.code, true);
    }
    keys.add(e.code);
  });
  addEventListener("keyup", (e) => { keys.delete(e.code); st.onKey?.(e.code, false); });
  addEventListener("blur", () => { keys.clear(); st.fire = false; });
  addEventListener("mousemove", (e) => {
    if (!st.locked) return;
    st.dx += e.movementX; st.dy += e.movementY;
  });
  addEventListener("mousedown", (e) => { if (st.locked && e.button === 0) st.fire = true; });
  addEventListener("mouseup", (e) => { if (e.button === 0) st.fire = false; });
  canvas.addEventListener("click", () => st.locked || (st.onClickLock ? st.onClickLock() : st.lock()));
  document.addEventListener("pointerlockchange", () => {
    st.locked = document.pointerLockElement === canvas;
    if (!st.locked) { keys.clear(); st.fire = false; }
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
      await navigator.keyboard?.lock?.(["KeyW", "KeyA", "KeyS", "KeyD", "KeyC", "KeyR", "Space", "ControlLeft", "ControlRight", "Tab"]);
      st.kbLocked = !!navigator.keyboard?.lock;
    } catch { st.kbLocked = false; }
  };
  document.addEventListener("fullscreenchange", () => { if (!document.fullscreenElement) st.kbLocked = false; });
  // Ctrl crouches only while the keyboard is locked; in a window Ctrl+W would close the tab.
  st.crouching = (key) => keys.has(key) || (st.kbLocked && (keys.has("ControlLeft") || keys.has("ControlRight")));
  // Movement axes for physics; jump is consumed once per press (hold to bhop).
  st.move = (crouchKey) => {
    const k = (c) => (keys.has(c) ? 1 : 0);
    const jump = st.jumpQueued || keys.has("Space");
    st.jumpQueued = false;
    return {
      f: k("KeyW") - k("KeyS"),
      r: k("KeyD") - k("KeyA"),
      walk: keys.has("ShiftLeft") || keys.has("ShiftRight"),
      jump,
      crouch: st.crouching(crouchKey),
    };
  };
  st.takeMouse = () => { const d = [st.dx, st.dy]; st.dx = st.dy = 0; return d; };
  return st;
}
