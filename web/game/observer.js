// Observer cameras for spectators and the dead: follow live players in first
// person or a chase cam, fly around (free roam) or look down on the map
// (overview), plus the death cam (a freeze on your killer, then follow).
// Pure camera logic over the sampled remote players; main.js feeds input and
// draws what view() returns.
//   Fire / zoom (left / right click): next / previous live player
//   Jump (Space): next camera mode
import { basis } from "../engine/math.js";

export const MODES = ["first", "chase", "free", "overview"];
export const MODE_NAMES = { first: "first person", chase: "chase cam", free: "free roam", overview: "overview" };
const EYE = 1.62, EYE_CROUCH = 1.05;
const CHASE_DIST = 3.2, CHASE_HEIGHT = 1.7;
const FLY_SPEED = 9, FLY_FAST = 3;
export const FREEZE_TIME = 2; // seconds on the killer before following someone

// raycast(o, d, maxT) -> {t} | null against the map (keeps the chase cam out of walls).
export function createObserver({ raycast }) {
  const o = {
    mode: "first",
    target: 0, // followed player id (0 = nobody)
    free: { p: [0, 12, 30], yaw: 0, pitch: -0.3 },
    chase: { yaw: 0, pitch: -0.25, set: false }, // orbit angles, world space
    freeze: null, // death cam: {killer, at: [x,y,z] eye, left: s, weapon}
    team: "", // prefer teammates when choosing whom to follow (the dead)
  };

  // Live players that can be followed.
  const live = (players) => players.filter((r) => !r.dead && r.team !== "spec");

  function pick(players, dir) {
    const list = live(players);
    if (!list.length) { o.target = 0; return; }
    const ids = list.map((r) => r.id).sort((a, b) => a - b);
    const i = ids.indexOf(o.target);
    o.target = i < 0 ? ids[0] : ids[(i + dir + ids.length) % ids.length];
    o.chase.set = false;
  }
  o.next = (players) => { o.freeze = null; pick(players, 1); };
  o.prev = (players) => { o.freeze = null; pick(players, -1); };
  o.cycleMode = () => {
    o.freeze = null;
    o.mode = MODES[(MODES.indexOf(o.mode) + 1) % MODES.length];
    o.chase.set = false;
  };
  o.setMode = (m) => { if (MODES.includes(m)) { o.mode = m; o.freeze = null; o.chase.set = false; } };

  // Death cam: freeze looking at the killer from where you died.
  // killer 0 (suicide, the world): keep looking where you looked (yaw, pitch).
  o.died = ({ killer, eye, yaw = 0, pitch = 0, weapon, team }) => {
    o.team = team || "";
    o.freeze = { killer, at: [...eye], yaw, pitch, left: FREEZE_TIME, weapon };
    o.mode = "first";
    o.target = 0;
  };
  // Back to playing (respawn) or to plain spectating.
  o.reset = () => { o.freeze = null; o.team = ""; };

  // The followed player, choosing one if the current one is gone or dead
  // (teammates first when o.team is set, then anyone).
  function followed(players) {
    let t = players.find((r) => r.id === o.target && !r.dead && r.team !== "spec");
    if (!t) {
      const list = live(players);
      t = (o.team && list.find((r) => r.team === o.team)) || list[0] || null;
      o.target = t ? t.id : 0;
      o.chase.set = false;
    }
    return t;
  }

  // Per frame: mouse look (radians) and free-roam movement {f, r, up, fast}.
  o.update = (dt, look, move) => {
    if (o.freeze) {
      o.freeze.left -= dt;
      if (o.freeze.left <= 0) o.freeze = null;
      return;
    }
    const [dyaw, dpitch] = look;
    if (o.mode === "free") {
      const f = o.free;
      f.yaw += dyaw; f.pitch = Math.max(-1.55, Math.min(1.55, f.pitch + dpitch));
      const b = basis(f.yaw, f.pitch), sp = FLY_SPEED * (move.fast ? FLY_FAST : 1) * dt;
      for (let a = 0; a < 3; a++) f.p[a] += (b.fwd[a] * move.f + b.right[a] * move.r) * sp + (a === 1 ? move.up * sp : 0);
    } else if (o.mode === "chase") {
      o.chase.yaw += dyaw;
      o.chase.pitch = Math.max(-1.2, Math.min(0.8, o.chase.pitch + dpitch));
    }
  };

  // The camera for this frame: {eye, yaw, pitch, follow (player | null), first (bool), killer?}.
  // Callers hide `follow`'s body in first person.
  o.view = (players, map) => {
    if (o.freeze) {
      const k = players.find((r) => r.id === o.freeze.killer);
      const at = o.freeze.at;
      let { yaw, pitch } = o.freeze;
      if (k) {
        const d = [k.p[0] - at[0], k.p[1] + (k.c ? EYE_CROUCH : EYE) - 0.3 - at[1], k.p[2] - at[2]];
        yaw = Math.atan2(-d[0], -d[2]);
        pitch = Math.atan2(d[1], Math.hypot(d[0], d[2]));
      }
      return { eye: at, yaw, pitch, follow: null, first: false, killer: k || null };
    }
    if (o.mode === "overview") return { ...overview(map), follow: null, first: false };
    if (o.mode === "free") return { eye: [...o.free.p], yaw: o.free.yaw, pitch: o.free.pitch, follow: null, first: false };
    const t = followed(players);
    if (!t) return { ...overview(map), follow: null, first: false };
    const head = [t.p[0], t.p[1] + (t.c ? EYE_CROUCH : EYE), t.p[2]];
    if (o.mode === "first") {
      // Free roam starts from wherever you last watched.
      o.free.p = [...head]; o.free.yaw = t.yaw; o.free.pitch = t.pitch;
      return { eye: head, yaw: t.yaw, pitch: t.pitch, follow: t, first: true };
    }
    // chase: orbit behind the player, pulled in front of walls
    if (!o.chase.set) { o.chase.yaw = t.yaw; o.chase.pitch = -0.25; o.chase.set = true; }
    const pivot = [t.p[0], t.p[1] + CHASE_HEIGHT - (t.c ? 0.5 : 0), t.p[2]];
    const back = basis(o.chase.yaw, o.chase.pitch).fwd.map((v) => -v);
    const hit = raycast(pivot, back, CHASE_DIST);
    const dist = hit ? Math.max(0.3, hit.t - 0.25) : CHASE_DIST;
    const eye = pivot.map((v, i) => v + back[i] * dist);
    o.free.p = [...eye]; o.free.yaw = o.chase.yaw; o.free.pitch = o.chase.pitch;
    return { eye, yaw: o.chase.yaw, pitch: o.chase.pitch, follow: t, first: false };
  };

  return o;
}

// Top-down view of the whole map: centred on its boxes, high enough to see them all.
export function overview(map) {
  if (!map?.boxes?.length) return { eye: [0, 75, 0.01], yaw: 0, pitch: -1.5 };
  let x0 = Infinity, x1 = -Infinity, z0 = Infinity, z1 = -Infinity;
  for (const b of map.boxes) {
    x0 = Math.min(x0, b.min[0]); x1 = Math.max(x1, b.max[0]);
    z0 = Math.min(z0, b.min[2]); z1 = Math.max(z1, b.max[2]);
  }
  const size = Math.max(x1 - x0, z1 - z0);
  return { eye: [(x0 + x1) / 2, size * 0.95, (z0 + z1) / 2 + 0.01], yaw: 0, pitch: -1.5 };
}
