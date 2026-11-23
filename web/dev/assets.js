// Asset gallery: renders every registered material and model with the real
// renderer and plays every sound recipe. A test harness for gen/ + assets/.
import { createRenderer } from "../engine/gl.js";
import { PALETTE } from "../assets/palette.js";
import { MATERIALS } from "../assets/materials/index.js";
import { SKY, SUN_DIR } from "../assets/sky.js";
import { playerParts, torsoFrame } from "../assets/models/player.js";
import { itemParts, viewmodelParts } from "../assets/models/weapons.js";
import { bigDiffParts, carriedParts } from "../assets/models/bigdiff.js";
import { gpuParts, gpuCarriedParts, gpuHandParts } from "../assets/models/gpu.js";
import { WEAPONS } from "../assets/weapons/index.js";
import { CATALOG, initAudio } from "../assets/sounds/index.js";
import { hexRgb } from "../gen/color.js";
import { newGait, gaitStep } from "../gen/gait.js";
import { LINES, VOICES, sayPhonemes } from "../assets/voice/index.js";
import { createMusic, LOOPS, STINGERS, loopTheme } from "../assets/music/index.js";

const canvas = document.getElementById("gl");
const gfx = createRenderer(canvas, { palette: PALETTE, materials: MATERIALS, sky: SKY, sun: SUN_DIR });

// A sand slab with one 2 m cube per material along x, plus a low 'step' so edges show.
const spacing = 4, x0 = -((MATERIALS.length - 1) * spacing) / 2;
const half = Math.max(20, -x0 + 4);
const boxes = [{ min: [-half, -1, -12], max: [half, 0, 16], kind: "sand" }];
const labels = [];
MATERIALS.forEach((m, i) => {
  const x = x0 + i * spacing;
  boxes.push({ min: [x - 1, 0, -4], max: [x + 1, 2, -2], kind: m.name });
  boxes.push({ min: [x - 1, 0, -1.5], max: [x + 1, 0.5, -0.5], kind: m.name });
  labels.push({ text: m.name, p: [x, 2.4, -3] });
});
// The store showroom: every GPU Drop prop (internal/content/prefabs/store.go), as the Go
// test TestShowroomGolden lays them out. ?target=0,1,-18&dist=16 looks at it.
try {
  const props = await (await fetch("/dev/storeprops.json")).json();
  boxes.push(...props.boxes);
  for (const [text, p] of [["gondolas", [-17.5, 3.2, -21]], ["stock shelf", [-11, 3.2, -21]], ["checkouts", [-6, 3.2, -19]],
    ["LAN desk", [0, 2, -20]], ["TV wall", [7, 3.8, -21]], ["kiosks + price sign", [6, 2.3, -19]], ["carts", [14.4, 1.6, -20]],
    ["escalator + glass", [20, 2.4, -20]]]) labels.push({ text, p });
} catch (e) { console.warn("no store showroom", e); }
gfx.setMap({ boxes });

// The lighting lab (maps.Labs lab_lights from the server): coloured lamps, panels,
// shadowed spots, a skylight. Toggles with the material slab; the orbit follows.
// /dev/assets.html?lab=1 opens on it.
let lab = null, view = { target: [0, 1, 0], min: 5, max: 40 };
document.getElementById("lab-toggle").onclick = async (e) => {
  if (!lab) lab = await (await fetch("/map.json?name=lab_lights")).json();
  const on = document.getElementById("lab-toggle").classList.toggle("on");
  gfx.setMap(on ? lab : { boxes });
  view = on ? { target: [0, 1, -6], min: 4, max: 60 } : { target: [0, 1, 0], min: 5, max: 40 };
  dist = on ? 30 : 14;
  labelEl.style.display = on ? "none" : "";
  const st = gfx.stats();
  document.getElementById("lab-info").textContent = on
    ? `${st.lights} lights, ${st.lightEntries} cell entries, built in ${st.lightBuildMs.toFixed(0)} ms, ${st.lightGpuKB} KB on the GPU` : "";
};

const players = [
  { label: "vibe coder", p: [-6.5, 0, 3], crouch: 0, color: "#ff4f8b", team: "vibe", id: 1 },
  { label: "artisan", p: [-4, 0, 3], crouch: 0, color: "#4fd1ff", team: "artisan", id: 2 },
  { label: "crouched", p: [-1.5, 0, 3], crouch: 1, color: "#9cff4f", team: "artisan", id: 5 },
  { label: "firing", p: [1, 0, 3], crouch: 0, color: "#ffd84f", team: "vibe", id: 4, flash: 1 },
];
for (const pl of players) pl.gait = newGait();
const animSpeed = document.getElementById("anim-speed"), animAir = document.getElementById("anim-air");
animSpeed.oninput = () => { document.getElementById("anim-speed-v").textContent = animSpeed.value; };
let lastFrame = null;
{ const q = new URLSearchParams(location.search); // ?speed=7&air=1 (screenshots)
  if (q.has("speed")) { animSpeed.value = q.get("speed"); animSpeed.oninput(); }
  if (q.has("air")) animAir.checked = q.get("air") === "1"; }
for (const pl of players) labels.push({ text: pl.label, p: [pl.p[0], 2.3, pl.p[2]] });
// Weapons (registry order), in a row in front of the players.
const weaponRow = Object.values(WEAPONS).map((w, i, all) => ({ id: w.id, p: [(i - (all.length - 1) / 2) * 1.8, 0.6, 4] }));
for (const w of weaponRow) labels.push({ text: WEAPONS[w.id].name, p: [w.p[0], 1.6, w.p[2]] });
// The Big Diff: idle (carried / dropped) and planted with the CI bar running and a revert in progress.
const diffRow = [{ p: [-1.8, 0, 7], planted: false }, { p: [1.8, 0, 7], planted: true }];
labels.push({ text: "The Big Diff", p: [-1.8, 2.3, 7] }, { text: "planted (CI running)", p: [1.8, 2.3, 7] });

// GPU Drop: the boxed GPU on its shelf, dropped (a grab in progress) and on a carrier's back.
// ?fp=1: a first-person view of a GPU carrier (the hand item next to the weapon).
// Seen best from ?target=0,0.9,12&dist=6&still=1.
const carrierGait = newGait();
const gpuRow = { shelf: [-3, 0, 12], dropped: [0, 0, 12], carrier: [3, 0, 12], diffCarrier: [6, 0, 12] };
labels.push({ text: "GPU (shelf)", p: [-3, 1.6, 12] }, { text: "GPU dropped (grab)", p: [0, 1.6, 12] }, { text: "GPU carried", p: [3, 2.5, 12] });

const labelEl = document.getElementById("labels");
for (const l of labels) { l.el = document.createElement("div"); l.el.textContent = l.text; labelEl.appendChild(l.el); }

const sounds = document.getElementById("sounds");
for (const [name, play] of Object.entries(CATALOG)) {
  const b = document.createElement("button");
  b.textContent = name;
  b.onclick = () => { initAudio(); setTimeout(play, 30); };
  sounds.appendChild(b);
}

// Music: loops toggle on the music engine (one at a time), stingers play once.
const music = createMusic(), musicInfo = document.getElementById("music-info");
const showLoop = (id) => {
  const t = id && loopTheme(id), p = t?.pattern;
  musicInfo.textContent = p ? `${t.name}: ${p.scale} at MIDI ${p.root}, ${p.bpm} bpm, ${p.beats} beats, ${p.events.length} notes, seed ${t.seed}` : "";
  for (const b of document.querySelectorAll("#music-loops button")) b.classList.toggle("on", b.dataset.id === id);
};
const toggleLoop = (id) => {
  initAudio();
  setTimeout(() => { const on = music.state().loop === id; music.setLoop(on ? null : id); showLoop(on ? null : id); }, 30);
};
for (const id of Object.keys(LOOPS)) {
  const b = document.createElement("button");
  b.textContent = `${LOOPS[id].name} (${id})`; b.dataset.id = id;
  b.onclick = () => toggleLoop(id);
  document.getElementById("music-loops").appendChild(b);
}
const stingers = [...Object.keys(STINGERS).filter((id) => id !== "kill").map((id) => [id, id]), ...[1, 2, 3, 4, 5].map((m) => [`kill x${m}`, "kill", m])];
for (const [label, id, arg] of stingers) {
  const b = document.createElement("button");
  b.textContent = label;
  b.onclick = () => { initAudio(); setTimeout(() => music.stinger(id, arg), 30); };
  document.getElementById("music-stingers").appendChild(b);
}
document.getElementById("music-mvp-play").onclick = () => toggleLoop(`mvp:${document.getElementById("music-mvp").value.trim() || "alice"}`);

// Camera: ?target=x,y,z&yaw=&pitch=&dist= frames something (e.g. the GPU row: ?target=0,1,10&dist=7);
// ?still=1 stops the slow auto-orbit.
const q = new URLSearchParams(location.search), num = (k, d) => (q.has(k) ? +q.get(k) : d);
const target = q.has("target") ? q.get("target").split(",").map(Number) : [0, 1, 0];
let yaw = num("yaw", 0), pitch = num("pitch", -0.25), dist = num("dist", 14), drag = null;
const still = q.has("still");
view.target = target;
canvas.addEventListener("pointerdown", (e) => { drag = [e.clientX, e.clientY]; canvas.setPointerCapture(e.pointerId); });
canvas.addEventListener("pointerup", () => { drag = null; });
canvas.addEventListener("pointermove", (e) => {
  if (!drag) return;
  yaw -= (e.clientX - drag[0]) * 0.005;
  pitch = Math.max(-1.2, Math.min(0.3, pitch - (e.clientY - drag[1]) * 0.005));
  drag = [e.clientX, e.clientY];
});
canvas.addEventListener("wheel", (e) => { dist = Math.max(view.min, Math.min(view.max, dist + e.deltaY * 0.01)); }, { passive: true });

function frame(now) {
  requestAnimationFrame(frame);
  const time = now / 1000;
  if (!drag && !still) yaw += 0.002;
  // Walk on the spot: the gait sees the slider's speed along the model's facing.
  const dt = lastFrame === null ? 0 : Math.min(0.1, time - lastFrame);
  lastFrame = time;
  const facing = Math.PI + Math.sin(time * 0.5) * 0.5, v = +animSpeed.value * dt;
  for (const pl of players) gaitStep(pl.gait, -Math.sin(facing) * v, 0, -Math.cos(facing) * v, dt, { crouch: !!pl.crouch, air: animAir.checked });
  const target = view.target;
  const eye = [
    target[0] + Math.sin(yaw) * Math.cos(pitch) * dist,
    target[1] - Math.sin(pitch) * dist,
    target[2] + Math.cos(yaw) * Math.cos(pitch) * dist,
  ];
  const solids = players.flatMap((pl) => playerParts({
    p: pl.p, team: pl.team, id: pl.id, time, yaw: facing, pitch: 0, rgb: hexRgb(pl.color),
    gait: pl.gait, crouch: pl.crouch, flash: pl.flash ? (Math.sin(time * 8) > 0 ? 1 : 0) : 0,
  }));
  for (const w of weaponRow) solids.push(...itemParts(w.p, w.id, time).map((p) => ({ ...p, m: scaleAbout(p.m, w.p, 1.6) })));
  for (const d of diffRow) {
    const parts = bigDiffParts({ pos: d.p, time, planted: d.planted, left: 40 - ((time * 4) % 40), total: 40, progress: d.planted ? (time * 0.2) % 1 : 0 });
    solids.push(...parts.map((q) => ({ ...q, m: scaleAbout(q.m, d.p, 2.5) })));
  }
  const k = 2;
  solids.push(...gpuParts({ pos: gpuRow.shelf, yaw: Math.PI + Math.sin(time * 0.4) * 0.6, time }).map((q) => ({ ...q, m: scaleAbout(q.m, gpuRow.shelf, k) })));
  solids.push(...gpuParts({ pos: gpuRow.dropped, yaw: 0.4, time, st: "dropped", progress: (time * 0.5) % 1 }).map((q) => ({ ...q, m: scaleAbout(q.m, gpuRow.dropped, k) })));
  const cy = Math.sin(time * 0.5) * 1.2; // back to the default camera
  const carrier = { p: gpuRow.carrier, team: "vibe", id: 7, time, yaw: cy, pitch: 0, rgb: hexRgb("#ff4f8b"), gait: carrierGait, crouch: 0, flash: 0 };
  gaitStep(carrierGait, -Math.sin(cy) * +animSpeed.value * dt, 0, -Math.cos(cy) * +animSpeed.value * dt, dt, {});
  solids.push(...playerParts(carrier));
  solids.push(...gpuCarriedParts(torsoFrame(carrier), false, time));
  // ...and a Big Diff carrier next to it, walking the same way (the diff sways with the torso).
  const diffCarrier = { ...carrier, p: gpuRow.diffCarrier, id: 8, rgb: hexRgb("#4fd1ff") };
  solids.push(...playerParts(diffCarrier), ...carriedParts(torsoFrame(diffCarrier), false));
  if (q.has("fp")) {
    const fe = [0, 1.6, 15], fy = 0, fpitch = -0.15, v = { bobX: Math.sin(time * 6) * 0.01, bobY: Math.abs(Math.cos(time * 6)) * 0.01, kick: 0, flash: 0, raise: 1 };
    const overlay = [...viewmodelParts(fe, fy, fpitch, v, "prompt"), ...gpuHandParts(fe, fy, fpitch, v, time)];
    gfx.frame({ eye: fe, yaw: fy, pitch: fpitch, time, solids, overlay });
  } else gfx.frame({ eye, yaw, pitch, time, solids });
  for (const l of labels) {
    const s = gfx.project(l.p);
    l.el.style.display = s ? "" : "none";
    if (s) l.el.style.transform = `translate(${s[0]}px, ${s[1]}px) translate(-50%, -100%)`;
  }
}
requestAnimationFrame(frame);
if (new URLSearchParams(location.search).has("lab")) document.getElementById("lab-toggle").onclick();

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
