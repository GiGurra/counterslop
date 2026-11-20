// Keyboard + pointer-lock mouse. Movement keys use e.code so layouts don't matter.
export function createInput(canvas) {
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
  canvas.addEventListener("click", () => st.locked || st.lock());
  document.addEventListener("pointerlockchange", () => {
    st.locked = document.pointerLockElement === canvas;
    if (!st.locked) { keys.clear(); st.fire = false; }
    st.onLock?.(st.locked);
  });

  st.lock = () => canvas.requestPointerLock?.()?.catch?.(() => {});
  st.down = (code) => keys.has(code);
  // Movement axes for physics; jump is consumed once per press (hold to bhop).
  st.move = () => {
    const k = (c) => (keys.has(c) ? 1 : 0);
    const jump = st.jumpQueued || keys.has("Space");
    st.jumpQueued = false;
    return {
      f: k("KeyW") - k("KeyS"),
      r: k("KeyD") - k("KeyA"),
      walk: keys.has("ShiftLeft") || keys.has("ShiftRight"),
      jump,
    };
  };
  st.takeMouse = () => { const d = [st.dx, st.dy]; st.dx = st.dy = 0; return d; };
  return st;
}
