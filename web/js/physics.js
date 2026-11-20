// Player movement against the map's AABBs. Position is the feet centre.
export const R = 0.4, H = 1.8, EYE = 1.62;
const STEP = 0.55, GRAV = 22, JUMP = 7.2, RUN = 7, WALK = 3.5;
const GROUND_ACCEL = 70, AIR_ACCEL = 14, STRIDE = 2.2, E = 1e-4;

export function newBody(spawn) {
  return { pos: [...spawn], vel: [0, 0, 0], yaw: 0, pitch: 0, onGround: false, eyeOff: 0, stride: 0 };
}

export function createPhysics() {
  let boxes = [];

  const overlaps = (x, y, z, b) =>
    x + R > b.min[0] + E && x - R < b.max[0] - E &&
    y + H > b.min[1] + E && y < b.max[1] - E &&
    z + R > b.min[2] + E && z - R < b.max[2] - E;

  const free = (x, y, z) => !boxes.some((b) => overlaps(x, y, z, b));

  function moveH(s, ax, d) {
    if (!d) return;
    s.pos[ax] += d;
    for (const b of boxes) {
      if (!overlaps(s.pos[0], s.pos[1], s.pos[2], b)) continue;
      const rise = b.max[1] - s.pos[1];
      if (s.onGround && rise > 0 && rise <= STEP && free(s.pos[0], b.max[1], s.pos[2])) {
        s.pos[1] = b.max[1];
        s.eyeOff -= rise; // smoothed back to 0 so stairs don't jolt the camera
        continue;
      }
      s.pos[ax] = d > 0 ? b.min[ax] - R : b.max[ax] + R;
      s.vel[ax] = 0;
    }
  }

  function moveV(s, d) {
    s.onGround = false;
    s.pos[1] += d;
    for (const b of boxes) {
      if (!overlaps(s.pos[0], s.pos[1], s.pos[2], b)) continue;
      if (d < 0) { s.pos[1] = b.max[1]; s.onGround = true; } else s.pos[1] = b.min[1] - H;
      s.vel[1] = 0;
    }
  }

  // inp: {f, r, walk, jump}; returns sound-worthy events.
  function step(s, inp, dt) {
    const ev = { jumped: false, landed: 0, steps: 0 };
    dt = Math.min(dt, 0.1);
    const n = Math.max(1, Math.ceil(dt / (1 / 120)));
    const h = dt / n;
    for (let i = 0; i < n; i++) sub(s, inp, h, ev);
    s.eyeOff *= Math.exp(-dt * 14);
    return ev;
  }

  function sub(s, inp, dt, ev) {
    const sy = Math.sin(s.yaw), cy = Math.cos(s.yaw);
    let wx = -sy * inp.f + cy * inp.r, wz = -cy * inp.f - sy * inp.r;
    const wl = Math.hypot(wx, wz);
    if (wl > 0) { wx /= wl; wz /= wl; }
    const max = inp.walk ? WALK : RUN;

    if (s.onGround || wl > 0) {
      const a = (s.onGround ? GROUND_ACCEL : AIR_ACCEL) * dt;
      s.vel[0] += clamp(wx * max - s.vel[0], a);
      s.vel[2] += clamp(wz * max - s.vel[2], a);
    }
    if (inp.jump && s.onGround) { s.vel[1] = JUMP; s.onGround = false; ev.jumped = true; }
    s.vel[1] -= GRAV * dt;

    const wasGround = s.onGround, fall = -s.vel[1];
    moveH(s, 0, s.vel[0] * dt);
    moveH(s, 2, s.vel[2] * dt);
    moveV(s, s.vel[1] * dt);
    if (s.onGround && !wasGround && fall > 3) ev.landed = Math.max(ev.landed, fall);

    const sp = Math.hypot(s.vel[0], s.vel[2]);
    if (s.onGround && sp > WALK + 0.5) {
      s.stride += sp * dt;
      if (s.stride > STRIDE) { s.stride -= STRIDE; ev.steps++; }
    }
  }

  return {
    setMap(map) { boxes = map.boxes; },
    step,
    // Ray vs boxes (slab test). Returns {t, n} for the nearest hit within maxT.
    raycast(o, d, maxT = 200) {
      let best = null;
      for (const b of boxes) {
        let t0 = 0, t1 = maxT, nAx = -1, nSign = 0;
        for (let a = 0; a < 3; a++) {
          if (Math.abs(d[a]) < 1e-9) { if (o[a] < b.min[a] || o[a] > b.max[a]) { t0 = Infinity; break; } continue; }
          let ta = (b.min[a] - o[a]) / d[a], tb = (b.max[a] - o[a]) / d[a];
          let sgn = -Math.sign(d[a]);
          if (ta > tb) [ta, tb] = [tb, ta];
          if (ta > t0) { t0 = ta; nAx = a; nSign = sgn; }
          if (tb < t1) t1 = tb;
          if (t0 > t1) break;
        }
        if (t0 <= t1 && t0 < (best?.t ?? Infinity) && nAx >= 0) {
          const n = [0, 0, 0]; n[nAx] = nSign;
          best = { t: t0, n };
        }
      }
      return best;
    },
  };
}

const clamp = (v, m) => Math.max(-m, Math.min(m, v));
