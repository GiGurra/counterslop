// Asset gallery: renders every registered material and model with the real
// renderer and plays every sound recipe. A test harness for gen/ + assets/.
import { createRenderer } from "../engine/gl.js";
import { PALETTE } from "../assets/palette.js";
import { MATERIALS } from "../assets/materials/index.js";
import { SKY, SUN_DIR } from "../assets/sky.js";
import { playerParts } from "../assets/models/player.js";
import { itemParts } from "../assets/models/weapons.js";
import { bigDiffParts } from "../assets/models/bigdiff.js";
import { WEAPONS } from "../assets/weapons/index.js";
import { CATALOG, initAudio } from "../assets/sounds/index.js";
import { hexRgb } from "../gen/color.js";
import { LINES, VOICES, sayPhonemes } from "../assets/voice/index.js";

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
  { label: "vibe coder", p: [-6.5, 0, 3], crouch: 0, color: "#ff4f8b", team: "vibe", id: 1 },
  { label: "artisan", p: [-4, 0, 3], crouch: 0, color: "#4fd1ff", team: "artisan", id: 2 },
  { label: "crouched", p: [-1.5, 0, 3], crouch: 1, color: "#9cff4f", team: "artisan", id: 5 },
  { label: "firing", p: [1, 0, 3], crouch: 0, color: "#ffd84f", team: "vibe", id: 4, flash: 1 },
];
for (const pl of players) labels.push({ text: pl.label, p: [pl.p[0], 2.3, pl.p[2]] });
// Weapons (registry order), in a row in front of the players.
const weaponRow = Object.values(WEAPONS).map((w, i, all) => ({ id: w.id, p: [(i - (all.length - 1) / 2) * 1.8, 0.6, 4] }));
for (const w of weaponRow) labels.push({ text: WEAPONS[w.id].name, p: [w.p[0], 1.6, w.p[2]] });
// The Big Diff: idle (carried / dropped) and planted with the CI bar running and a revert in progress.
const diffRow = [{ p: [-1.8, 0, 7], planted: false }, { p: [1.8, 0, 7], planted: true }];
labels.push({ text: "The Big Diff", p: [-1.8, 2.3, 7] }, { text: "planted (CI running)", p: [1.8, 2.3, 7] });

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
    p: pl.p, team: pl.team, id: pl.id, time, yaw: Math.PI + Math.sin(time * 0.5) * 0.5, pitch: 0, rgb: hexRgb(pl.color),
    moving: true, phase: time * 6, crouch: pl.crouch, flash: pl.flash ? (Math.sin(time * 8) > 0 ? 1 : 0) : 0,
  }));
  for (const w of weaponRow) solids.push(...itemParts(w.p, w.id, time).map((p) => ({ ...p, m: scaleAbout(p.m, w.p, 1.6) })));
  for (const d of diffRow) {
    const parts = bigDiffParts({ pos: d.p, time, planted: d.planted, left: 40 - ((time * 4) % 40), total: 40, progress: d.planted ? (time * 0.2) % 1 : 0 });
    solids.push(...parts.map((q) => ({ ...q, m: scaleAbout(q.m, d.p, 2.5) })));
  }
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

// ---- SlopVox: every announcer line with live pitch / rate / formant / robot sliders ----
const $ = (id) => document.getElementById(id);
const voiceSel = $("vox-voice"), phIn = $("vox-ph"), info = $("vox-info"), linesEl = $("vox-lines");
for (const v of Object.keys(VOICES)) voiceSel.add(new Option(v, v));
let current = { ph: LINES.monster_slop.ph, voice: LINES.monster_slop.voice, text: LINES.monster_slop.text, color: LINES.monster_slop.color };
const SLIDERS = ["vox-pitch", "vox-rate", "vox-shift", "vox-robot", "vox-fx"];
phIn.value = current.ph;

function tweaks() {
  const robot = +$("vox-robot").value, fxMix = +$("vox-fx").value, set = {};
  for (const id of SLIDERS) {
    const el = $(id); el.nextElementSibling.textContent = +el.value < 0 ? "voice" : (+el.value).toFixed(2);
  }
  if (robot >= 0) set.robot = robot;
  if (fxMix >= 0) set.fxMix = fxMix;
  return { mul: { pitch: +$("vox-pitch").value, rate: +$("vox-rate").value, shift: +$("vox-shift").value }, set };
}

function speakCurrent() {
  initAudio();
  const { mul, set } = tweaks(), v = voiceSel.value || current.voice || "announcer";
  setTimeout(() => {
    const t0 = performance.now();
    let r;
    try { r = sayPhonemes(phIn.value, v, mul, set); } catch (e) { info.textContent = String(e.message || e); return; }
    const cap = $("vox-caption");
    cap.textContent = current.text || phIn.value; cap.style.color = current.color || "#eee";
    info.textContent = `${v} · ${(r.samples.length / r.sampleRate).toFixed(2)} s · rendered in ${(performance.now() - t0).toFixed(0)} ms (cached after first play)`;
    drawSpectrogram($("vox-spec"), r.samples, r.sampleRate);
  }, 30);
}

for (const [id, l] of Object.entries(LINES)) {
  const b = document.createElement("button");
  b.textContent = l.text; b.title = `${id}: ${l.ph}`;
  b.onclick = () => {
    linesEl.querySelectorAll("button").forEach((x) => x.classList.toggle("on", x === b));
    current = { ph: l.ph, voice: l.voice, text: l.text, color: l.color }; phIn.value = l.ph;
    speakCurrent();
  };
  linesEl.appendChild(b);
}
$("vox-say").onclick = () => { current = { ph: phIn.value, voice: undefined }; speakCurrent(); };
phIn.addEventListener("keydown", (e) => { if (e.key === "Enter") $("vox-say").onclick(); });
for (const id of SLIDERS) {
  $(id).addEventListener("input", tweaks);
  $(id).addEventListener("change", speakCurrent);
}
voiceSel.addEventListener("change", speakCurrent);
tweaks();

// Spectrogram (0-8 kHz, log magnitude) with the waveform on top.
function drawSpectrogram(cv, x, sr) {
  const g = cv.getContext("2d"), W = cv.width, H = cv.height, N = 512, img = g.createImageData(W, H);
  const hop = Math.max(1, (x.length - N) / W), bins = Math.floor((8000 / sr) * N), re = new Float64Array(N), im = new Float64Array(N);
  for (let c = 0; c < W; c++) {
    const o = Math.floor(c * hop);
    for (let i = 0; i < N; i++) { re[i] = (x[o + i] || 0) * (0.5 - 0.5 * Math.cos((2 * Math.PI * i) / N)); im[i] = 0; }
    fft(re, im);
    for (let y = 0; y < H; y++) {
      const k = Math.floor(((H - 1 - y) / H) * bins), db = 10 * Math.log10(re[k] * re[k] + im[k] * im[k] + 1e-9);
      const v = Math.max(0, Math.min(1, (db + 10) / 50)), p = (y * W + c) * 4;
      img.data[p] = 255 * v; img.data[p + 1] = 255 * v * v * 0.9 + 20 * v; img.data[p + 2] = 80 + 100 * (1 - v); img.data[p + 3] = 255;
    }
  }
  g.putImageData(img, 0, 0);
  g.strokeStyle = "#8bff4faa"; g.beginPath();
  for (let c = 0; c < W; c++) {
    let m = 0;
    for (let i = Math.floor(c * (x.length / W)); i < (c + 1) * (x.length / W); i++) m = Math.max(m, Math.abs(x[i]));
    g.moveTo(c, H - 2); g.lineTo(c, H - 2 - m * 30);
  }
  g.stroke();
}

function fft(re, im) { // in-place radix-2
  const n = re.length;
  for (let i = 1, j = 0; i < n; i++) {
    let bit = n >> 1;
    for (; j & bit; bit >>= 1) j ^= bit;
    j ^= bit;
    if (i < j) { [re[i], re[j]] = [re[j], re[i]]; [im[i], im[j]] = [im[j], im[i]]; }
  }
  for (let len = 2; len <= n; len <<= 1) {
    const a = (-2 * Math.PI) / len, wr = Math.cos(a), wi = Math.sin(a);
    for (let i = 0; i < n; i += len) {
      let cr = 1, ci = 0;
      for (let k = 0; k < len / 2; k++) {
        const ur = re[i + k], ui = im[i + k], j = i + k + len / 2;
        const vr = re[j] * cr - im[j] * ci, vi = re[j] * ci + im[j] * cr;
        re[i + k] = ur + vr; im[i + k] = ui + vi; re[j] = ur - vr; im[j] = ui - vi;
        [cr, ci] = [cr * wr - ci * wi, cr * wi + ci * wr];
      }
    }
  }
}
