// Asset gallery: renders every registered material and model with the real
// renderer and plays every sound recipe. A test harness for gen/ + assets/.
import { createRenderer } from "../engine/gl.js";
import { PALETTE } from "../assets/palette.js";
import { MATERIALS } from "../assets/materials/index.js";
import { SKY, SUN_DIR } from "../assets/sky.js";
import { playerParts } from "../assets/models/player.js";
import { itemParts } from "../assets/models/weapons.js";
import { WEAPONS } from "../assets/weapons/index.js";
import { CATALOG, initAudio } from "../assets/sounds/index.js";
import { hexRgb } from "../gen/color.js";

const canvas = document.getElementById("gl");
const gfx = createRenderer(canvas, { palette: PALETTE, materials: MATERIALS, sky: SKY, sun: SUN_DIR });

// A sand slab with one 2 m cube per material along x, plus a low 'step' so edges show.
const spacing = 4, x0 = -((MATERIALS.length - 1) * spacing) / 2;
const boxes = [{ min: [-20, -1, -12], max: [20, 0, 12], kind: "sand" }];
const labels = [];
MATERIALS.forEach((m, i) => {
  const x = x0 + i * spacing;
  boxes.push({ min: [x - 1, 0, -4], max: [x + 1, 2, -2], kind: m.name });
  boxes.push({ min: [x - 1, 0, -1.5], max: [x + 1, 0.5, -0.5], kind: m.name });
  labels.push({ text: m.name, p: [x, 2.4, -3] });
});
gfx.setMap({ boxes });

const players = [
  { label: "player", p: [-4, 0, 3], crouch: 0, color: "#ff4f8b" },
  { label: "crouched", p: [-1.5, 0, 3], crouch: 1, color: "#4fd1ff" },
  { label: "firing", p: [1, 0, 3], crouch: 0, color: "#ffd84f", flash: 1 },
];
for (const pl of players) labels.push({ text: pl.label, p: [pl.p[0], 2.3, pl.p[2]] });
// Weapons (registry order), in a row in front of the players.
const weaponRow = Object.values(WEAPONS).map((w, i, all) => ({ id: w.id, p: [(i - (all.length - 1) / 2) * 1.8, 0.6, 4] }));
for (const w of weaponRow) labels.push({ text: WEAPONS[w.id].name, p: [w.p[0], 1.6, w.p[2]] });

const labelEl = document.getElementById("labels");
for (const l of labels) { l.el = document.createElement("div"); l.el.textContent = l.text; labelEl.appendChild(l.el); }

const sounds = document.getElementById("sounds");
for (const [name, play] of Object.entries(CATALOG)) {
  const b = document.createElement("button");
  b.textContent = name;
  b.onclick = () => { initAudio(); setTimeout(play, 30); };
  sounds.appendChild(b);
}

let yaw = 0, pitch = -0.25, dist = 14, drag = null;
canvas.addEventListener("pointerdown", (e) => { drag = [e.clientX, e.clientY]; canvas.setPointerCapture(e.pointerId); });
canvas.addEventListener("pointerup", () => { drag = null; });
canvas.addEventListener("pointermove", (e) => {
  if (!drag) return;
  yaw -= (e.clientX - drag[0]) * 0.005;
  pitch = Math.max(-1.2, Math.min(0.3, pitch - (e.clientY - drag[1]) * 0.005));
  drag = [e.clientX, e.clientY];
});
canvas.addEventListener("wheel", (e) => { dist = Math.max(5, Math.min(40, dist + e.deltaY * 0.01)); }, { passive: true });

function frame(now) {
  requestAnimationFrame(frame);
  const time = now / 1000;
  if (!drag) yaw += 0.002;
  const target = [0, 1, 0];
  const eye = [
    target[0] + Math.sin(yaw) * Math.cos(pitch) * dist,
    target[1] - Math.sin(pitch) * dist,
    target[2] + Math.cos(yaw) * Math.cos(pitch) * dist,
  ];
  const solids = players.flatMap((pl) => playerParts({
    p: pl.p, yaw: Math.PI + Math.sin(time * 0.5) * 0.5, pitch: 0, rgb: hexRgb(pl.color),
    moving: true, phase: time * 6, crouch: pl.crouch, flash: pl.flash ? (Math.sin(time * 8) > 0 ? 1 : 0) : 0,
  }));
  for (const w of weaponRow) solids.push(...itemParts(w.p, w.id, time).map((p) => ({ ...p, m: scaleAbout(p.m, w.p, 1.6) })));
  gfx.frame({ eye, yaw, pitch, time, solids });
  for (const l of labels) {
    const s = gfx.project(l.p);
    l.el.style.display = s ? "" : "none";
    if (s) l.el.style.transform = `translate(${s[0]}px, ${s[1]}px) translate(-50%, -100%)`;
  }
}
requestAnimationFrame(frame);

// Scale a model matrix about a world point (makes the tiny viewmodel visible).
function scaleAbout(m, c, k) {
  const o = new Float32Array(m);
  for (let i = 0; i < 12; i++) o[i] *= k;
  for (let a = 0; a < 3; a++) o[12 + a] = c[a] + (m[12 + a] - c[a]) * k;
  return o;
}
