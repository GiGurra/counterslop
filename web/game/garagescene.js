// The garage drawn (game/garage.js has what it shows, assets/garage.js the
// room, assets/models/carkit.js the kit): a renderer of its own on a canvas
// over the game's, made the first time the garage opens and kept, so the
// match's map, lights and programs are never touched (leaving is instant).
// The camera orbits the turntable (drag to turn, wheel to zoom); the car you
// look at turns on it, or turns to show the part you picked. The click
// targets are HTML (ui/garage.js): every frame each element with a
// data-ganchor ("slot:<id>", "bay:<type>", "card") is moved to where its
// anchor is on screen (and marked "back" when it faces away).
//   createGarageScene({look, before, onPick})
//     .frame(view, {dt, glow, focusSlot, hold, noGL})  draw (view: garageView; dt: s since the last frame)
//     .show() / .hide()   the canvas on / off
//     .pick(x, y)         the anchor nearest a screen point ({kind, id} | null)
import { createRenderer } from "../engine/gl.js";
import { chain, translate, rotY } from "../engine/math.js";
import { GARAGE_ROOM, BAYS, TURNTABLE, garageProps } from "../assets/garage.js";
import { carParts, newCarAnim, stepCarAnim } from "../assets/models/car.js";
import { kitAnchors, kitParts } from "../assets/models/carkit.js";
import { signBoxes } from "../assets/models/signs.js";
import carsData from "../assets/cars.json" with { type: "json" };
import { SLOT_FACING, presentAngle, orbitEye, layoutLabels } from "./garage.js";

const CARS = carsData.cars;
const specOf = (id) => CARS.find((c) => c.id === id) || CARS[0];
const ROLL_S = 1.1; // a picked car rolls from its bay onto the turntable in this long

const angleDiff = (a, b) => Math.atan2(Math.sin(a - b), Math.cos(a - b));

export function createGarageScene({ look, before = null, onPick = null }) {
  let canvas = null, gfx = null, clock = 0; // s: the scene's own time, advanced by each frame's dt
  const cam = { yaw: 0.3, pitch: 0.3, dist: 8, target: [0, 0.8, 1.2] };
  let angle = 0.6, shownId = "", rollFrom = -1, rollAt = -Infinity;
  const anim = newCarAnim(), parkedAnim = newCarAnim();
  let anchors = []; // this frame's: {key, kind, id, x, y, back}
  let drag = null;

  function make() {
    canvas = document.createElement("canvas");
    canvas.className = "garage-view";
    (before?.parentNode || document.body).insertBefore(canvas, before);
    gfx = createRenderer(canvas, look);
    gfx.setOptions({ shadowQuality: 1, shadowSize: 2048 });
    gfx.setMap({ ...GARAGE_ROOM, boxes: [...GARAGE_ROOM.boxes, ...signBoxes(GARAGE_ROOM.signs)] });
    canvas.addEventListener("pointerdown", (e) => { drag = { x: e.clientX, y: e.clientY, moved: 0 }; canvas.setPointerCapture?.(e.pointerId); });
    canvas.addEventListener("pointermove", (e) => {
      if (!drag) return;
      const dx = e.clientX - drag.x, dy = e.clientY - drag.y;
      drag.moved += Math.abs(dx) + Math.abs(dy);
      drag.x = e.clientX; drag.y = e.clientY;
      cam.yaw -= dx * 0.006;
      cam.pitch = Math.max(0.08, Math.min(1.1, cam.pitch + dy * 0.004));
    });
    canvas.addEventListener("pointerup", (e) => {
      const click = drag && drag.moved < 6;
      drag = null;
      if (click) { const p = api.pick(e.clientX, e.clientY); if (p) onPick?.(p); }
    });
    canvas.addEventListener("wheel", (e) => { e.preventDefault(); cam.dist = Math.max(4.5, Math.min(14, cam.dist * (e.deltaY > 0 ? 1.08 : 1 / 1.08))); }, { passive: false });
  }

  // Where the shown car stands now: rolling in from its bay, then on the table.
  function shownPose(now) {
    const k = Math.min(1, (now - rollAt) / 1000 / ROLL_S), e = k * k * (3 - 2 * k);
    const t = [TURNTABLE.pos[0], TURNTABLE.pos[1], TURNTABLE.pos[2]];
    if (rollFrom < 0 || k >= 1) return { pos: t, yaw: angle, rolling: false };
    const b = BAYS[rollFrom];
    return { pos: [b.pos[0] + (t[0] - b.pos[0]) * e, b.pos[1] + (t[1] - b.pos[1]) * e, b.pos[2] + (t[2] - b.pos[2]) * e], yaw: b.yaw + angleDiff(angle, b.yaw) * e, rolling: true, fwd: 6 * (1 - Math.abs(2 * k - 1)) };
  }

  const api = {
    get canvas() { return canvas; },
    show() { if (!canvas) make(); canvas.style.display = ""; },
    hide() { if (canvas) canvas.style.display = "none"; drag = null; },
    camera: cam,
    get angle() { return angle; },
    // view: garageView; glow: the slot under the mouse / focus (lit up);
    // focusSlot: the part picked (the table turns it to the camera).
    frame(view, { dt = 0, glow = "", focusSlot = "", hold = false, noGL = false } = {}) {
      if (!canvas) make();
      dt = Math.max(0, Math.min(2, dt || 0)); // a stepped harness draws every n frames: the time still all counts
      clock += dt;
      const now = clock * 1000;
      if (view.shown !== shownId) { // a new car on the table: it rolls in from its bay
        rollFrom = shownId ? CARS.findIndex((c) => c.id === view.shown) : -1;
        rollAt = now; shownId = view.shown;
      }
      const want = presentAngle(focusSlot, cam.yaw);
      angle = want != null ? angle + angleDiff(want, angle) * Math.min(1, dt * 4) : hold ? angle : angle + dt * 0.22; // held still under the mouse: a target you point at stays put
      const time = clock, solids = garageProps(angle);
      // The parked cars: every type but the one on the table, in its bay.
      CARS.forEach((sp, i) => {
        if (sp.id === view.shown || !BAYS[i]) return;
        solids.push(...carParts({ spec: sp, pos: BAYS[i].pos, yaw: BAYS[i].yaw, time, id: 10 + i, anim: parkedAnim, comp: [0.5, 0.5, 0.5, 0.5] }));
      });
      // The car on the table, its kit fitted.
      const sp = specOf(view.shown), pose = shownPose(now);
      stepCarAnim(anim, { spec: sp, fwd: pose.fwd || 0, yawVel: 0, steer: 0, vel: [0, 0, 0], yaw: pose.yaw, fx: 0 }, dt);
      solids.push(...carParts({ spec: sp, pos: pose.pos, yaw: pose.yaw, time, id: 1, anim, comp: [0.5, 0.5, 0.5, 0.5], aim: pose.yaw }));
      const base = chain(translate(...pose.pos), rotY(pose.yaw));
      solids.push(...kitParts(sp, view.card, base, { time, glow }));
      const o = orbitEye(cam.target, cam.yaw, cam.pitch, cam.dist);
      gfx.frame({ eye: o.eye, yaw: o.yaw, pitch: o.pitch, time, solids, players: [], noGL });
      // The click targets this frame.
      anchors = [];
      const A = kitAnchors(sp), cy = Math.cos(pose.yaw), sy = Math.sin(pose.yaw);
      const world = (p) => [pose.pos[0] + p[0] * cy + p[2] * sy, pose.pos[1] + p[1], pose.pos[2] - p[0] * sy + p[2] * cy];
      for (const s of view.slots) {
        const f = SLOT_FACING[s.id] || [0, 1], q = gfx.project(world(A[s.id]));
        const back = Math.cos(pose.yaw + Math.atan2(f[0], f[1]) - cam.yaw) < -0.15;
        if (q) anchors.push({ key: `slot:${s.id}`, kind: "slot", id: s.id, x: q[0], y: q[1], back: back || pose.rolling });
      }
      CARS.forEach((c, i) => {
        if (!BAYS[i]) return;
        const q = gfx.project([BAYS[i].pos[0], 2.4, BAYS[i].pos[2]]);
        if (q) anchors.push({ key: `bay:${c.id}`, kind: "bay", id: c.id, x: q[0], y: q[1], back: false });
      });
      place();
    },
    // The anchor nearest screen point (x, y) within reach: a part of the car
    // on the table (the front ones first) or a bay.
    pick(x, y, reach = 70) {
      let best = null, bd = reach;
      for (const a of anchors) {
        const d = Math.hypot(a.x - x, a.y - y) + (a.back ? 40 : 0);
        if (d < bd) { bd = d; best = a; }
      }
      return best ? { kind: best.kind, id: best.id } : null;
    },
    anchors: () => anchors,
  };

  // The HTML targets onto their anchors; the bay labels laid out between the
  // side panels (garage.js layoutLabels), never under them nor on each other.
  function place() {
    if (typeof document === "undefined") return;
    const at = new Map(anchors.map((a) => [a.key, a]));
    const bays = [];
    for (const el of document.querySelectorAll("[data-ganchor]")) {
      const a = at.get(el.dataset.ganchor);
      if (!a) { el.style.visibility = "hidden"; continue; }
      el.style.visibility = "";
      if (a.kind === "bay") { bays.push({ el, a }); continue; }
      el.style.transform = `translate(${Math.round(a.x)}px, ${Math.round(a.y)}px)`;
      el.classList.toggle("back", !!a.back);
    }
    if (!bays.length) return;
    const left = document.querySelector(".g-card")?.getBoundingClientRect(), right = document.querySelector(".g-parts")?.getBoundingClientRect();
    const lo = left ? left.right + 8 : 8, hi = right ? right.left - 8 : innerWidth - 8;
    const laid = new Map(layoutLabels(bays.map(({ el, a }) => ({ key: a.key, x: a.x, w: el.offsetWidth })), lo, hi).map((l) => [l.key, l]));
    const y = Math.min(...bays.map(({ a }) => a.y));
    for (const { el, a } of bays) {
      const l = laid.get(a.key);
      el.style.transform = `translate(${Math.round(l.x)}px, ${Math.round(y - l.row * (el.offsetHeight + 6))}px)`;
    }
  }
  return api;
}
