// WebGL2 renderer core. Game-agnostic: the scene's look (palette, materials,
// sky) is passed in, and everything besides the map is drawn as "solids":
// unit cubes with a model matrix, a colour and an emissive amount. No textures.
import { perspective, mul, fpsView, basis, ortho, lookAt, identity, fogFor } from "./math.js";
import { prelude } from "../gen/glsl/index.js";
import { FXAA } from "../gen/glsl/fxaa.js";
import { aaPlan, scaledSize } from "./aa.js";
import {
  buildGrid, skyVisibility, buildLightLists, buildZones, packLights, activeLights, shadowViews, sphereInFrustum,
  atlasLayout, tileXY, tilesOf, dynamicScore, pickDynamic, allocTiles, fadeToward, contactBlobs,
} from "./lights.js";
import { createInstanced } from "./instanced.js";
import { LIST_W, MAX_SHADOW_SLOTS, MAX_DYN_LIGHTS, MASK } from "../gen/glsl/lights.js";
import { createLightPool, createMaskCache, planShadows } from "./dynlights.js";

const MAP_VS = `#version 300 es
layout(location=0) in vec3 aPos; layout(location=1) in vec3 aNrm;
layout(location=2) in vec4 aFace; layout(location=3) in float aKind;
uniform mat4 uVP;
out vec3 vWorld; out vec3 vNrm; out vec4 vFace; flat out int vKind;
void main(){ vWorld = aPos; vNrm = aNrm; vFace = aFace; vKind = int(aKind + 0.5); gl_Position = uVP * vec4(aPos, 1.0); }`;

// Surface handed to every material function.
const SURF = `struct Surf {
  vec3 world; vec3 n;
  vec2 uv;   // face-local, in metres from the box corner
  vec2 sz;   // face size in metres
  vec2 wuv;  // world-aligned uv, continuous across neighbouring boxes
  bool top;  // upward-facing
};`;

// used: the material indices this map's boxes use (null = all); only those compile and
// dispatch (a map pays for its own materials only; ids stay the registry's).
function mapFS(pal, materials, defs, used = null) {
  const mats = materials.map((m, i) => [m, i]).filter(([, i]) => !used || used.has(i));
  const dispatch = mats.map(([m, i]) => `if (k == ${i}) return mat_${m.name}(s);`).join("\n  ");
  return `${prelude(pal, defs)}
in vec3 vWorld; in vec3 vNrm; in vec4 vFace; flat in int vKind;
out vec4 outCol;
${SURF}
// Emissive amount a material may set (0..1): that much of its colour is shown unlit
// (screens, neon, LEDs glow in the shade). main() resets it before every shade().
float gEmit;
${mats.map(([m]) => m.glsl).join("\n")}
vec3 shade(Surf s, int k){
  ${dispatch}
  return vec3(1.0, 0.0, 1.0);
}
void main(){
  Surf s;
  s.world = vWorld; s.n = normalize(vNrm); s.uv = vFace.xy; s.sz = vFace.zw;
  s.wuv = abs(s.n.x) > 0.5 ? vWorld.zy : abs(s.n.y) > 0.5 ? vWorld.xz : vWorld.xy;
  s.top = s.n.y > 0.5;
  float ao = s.top ? 1.0 : mix(0.55, 1.0, smoothstep(0.0, 1.6, vWorld.y));
  gEmit = 0.0;
  vec3 col = shade(s, vKind);
  vec3 lit = light(col, vWorld, s.n, ao);
  outCol = vec4(fog(mix(lit, col, clamp(gEmit, 0.0, 1.0)), vWorld), 1.0);
}`;
}

const SOLID_VS = `#version 300 es
layout(location=0) in vec3 aPos; layout(location=1) in vec3 aNrm;
uniform mat4 uVP; uniform mat4 uModel;
out vec3 vWorld; out vec3 vNrm;
void main(){ vec4 w = uModel * vec4(aPos, 1.0); vWorld = w.xyz; vNrm = mat3(uModel) * aNrm; gl_Position = uVP * w; }`;

function solidFS(pal, defs) {
  return `${prelude(pal, defs)}
in vec3 vWorld; in vec3 vNrm;
uniform vec3 uColor; uniform float uEmit;
out vec4 outCol;
void main(){
  vec3 n = normalize(vNrm);
  vec3 v = normalize(uEye - vWorld);
  float rim = pow(1.0 - max(dot(n, v), 0.0), 3.0);
  vec3 c = light(uColor, vWorld, n, 1.0) + rim*0.35*uColor;
  c = mix(c, uColor*1.4, uEmit);
  outCol = vec4(fog(c, vWorld), 1.0);
}`;
}

// Depth-only pass from the sun: map (uModel = identity) and solids share it.
const DEPTH_VS = `#version 300 es
layout(location=0) in vec3 aPos;
uniform mat4 uVP; uniform mat4 uModel;
void main(){ gl_Position = uVP * uModel * vec4(aPos, 1.0); }`;
const DEPTH_FS = `#version 300 es
precision mediump float;
out vec4 o;
void main(){ o = vec4(1.0); }`;

const SKY_VS = `#version 300 es
layout(location=0) in vec2 aPos;
out vec2 vNdc;
void main(){ vNdc = aPos; gl_Position = vec4(aPos, 0.0, 1.0); }`;

function skyFS(pal, sky) {
  return `${prelude(pal)}
in vec2 vNdc;
uniform vec3 uFwd, uRight, uUp; uniform vec2 uTan;
out vec4 outCol;
${sky}
void main(){
  vec3 d = normalize(uFwd + vNdc.x*uTan.x*uRight + vNdc.y*uTan.y*uUp);
  outCol = vec4(sky(d), 1.0);
}`;
}

// Post pass: the offscreen target onto the canvas (upscaled when the render scale is
// below 100%), through FXAA when that is on.
const POST_FS = `#version 300 es
precision highp float;
uniform sampler2D uSrc; uniform vec2 uTexel; uniform int uFxaa;
in vec2 vNdc; out vec4 outCol;
${FXAA}
void main(){
  vec2 uv = vNdc * 0.5 + 0.5;
  outCol = vec4(uFxaa == 1 ? fxaa(uSrc, uv, uTexel) : texture(uSrc, uv).rgb, 1.0);
}`;

// look: {palette, materials, sky (GLSL defining vec3 sky(vec3 dir)), sun: [x,y,z]}
// Options (setOptions): shadowSize (texels, power of two), shadowQuality 0 off .. 3 softest,
// maxInFlight (0 = no limit: frames queued on the GPU before canRender() says no),
// finish (gl.finish() after every frame), aa (r_aa mode, engine/aa.js), renderScale (0.5..1).
// ctx: context creation flags {desynchronized, powerPreference, antialias (default true: the
// browser's MSAA, what aa "auto" uses)}; changing them needs a new renderer.
export function createRenderer(canvas, look, ctx = {}) {
  const gl = canvas.getContext("webgl2", {
    antialias: ctx.antialias ?? true, desynchronized: !!ctx.desynchronized, powerPreference: ctx.powerPreference || "default",
    preserveDrawingBuffer: !!ctx.preserveDrawingBuffer,
  });
  if (!gl) throw new Error("WebGL2 not supported");
  const pal = look.palette, sun = norm(look.sun), fogCol = pal.fog;
  const kindIndex = Object.fromEntries(look.materials.map((m, i) => [m.name, i]));

  // Lit programs come in variants (gen/glsl/lights.js): with map lights, and with lamp
  // shadows too; compiled on first use and kept.
  const variants = {};
  // The map program also depends on the map's material set (usedKinds, from setMap).
  let usedKinds = null, usedKey = "all";
  function variant(lights, shadows) {
    const key = `${+lights}${+shadows}`;
    if (!variants[key]) {
      const defs = [...(lights ? ["HAS_LIGHTS"] : []), ...(shadows ? ["HAS_LIGHT_SHADOWS"] : [])];
      variants[key] = { defs, maps: {}, solid: program(gl, SOLID_VS, solidFS(pal, defs)) };
      samplers(variants[key].solid);
    }
    const v = variants[key];
    if (usedKinds && !v.maps[usedKey]) { v.maps[usedKey] = program(gl, MAP_VS, mapFS(pal, look.materials, v.defs, usedKinds)); samplers(v.maps[usedKey]); }
    return { map: v.maps[usedKey] || null, solid: v.solid, defs: v.defs };
  }
  let mapProg, solidProg;
  const skyProg = program(gl, SKY_VS, skyFS(pal, look.sky));
  const depthProg = program(gl, DEPTH_VS, DEPTH_FS);
  const postProg = program(gl, SKY_VS, POST_FS);
  const ID = identity();
  // Texture units: 0 sun shadow map, 1 post source, 2-5 the light grid (lights.js),
  // 6 the lamp shadows' static atlas, 7 their view matrices, 8 the live (players) pool,
  // 9 the dynamic lights' cell masks, 10 their cube shadows, 11 those shadows' matrices.
  const UNITS = { uShadowMap: 0, uLightCells: 2, uLightList: 3, uLightData: 4, uSkyVis: 5, uLShadow: 6, uLSMat: 7, uLShadowDyn: 8, uDynMask: 9, uDynShadow: 10, uDynSMat: 11 };
  function samplers(p) {
    gl.useProgram(p);
    for (const [name, unit] of Object.entries(UNITS)) gl.uniform1i(loc(gl, p, name), unit);
  }
  samplers(skyProg);
  ({ map: mapProg, solid: solidProg } = variant(false, false));
  const lg = createLightGrid(gl, () => opts.lights);
  const inst = createInstanced(gl, pal, { program, loc, samplers });
  let litDefs = [];
  const fl = createDynLights(gl, lg, () => opts.lights);

  const skyVao = gl.createVertexArray();
  gl.bindVertexArray(skyVao);
  attrib(gl, 0, 2, new Float32Array([-1, -1, 3, -1, -1, 3]));

  const cube = boxMesh([{ min: [-0.5, -0.5, -0.5], max: [0.5, 0.5, 0.5] }], () => 0, true);
  const cubeVao = gl.createVertexArray();
  gl.bindVertexArray(cubeVao);
  attrib(gl, 0, 3, cube.pos);
  attrib(gl, 1, 3, cube.nrm);
  indexBuf(gl, cube.idx);

  let mapVao = null, mapCount = 0, mapBounds = null;
  let vp = null, fov = 1.3, aspect = 1;
  // fog: distance fog density (per metre); offscreen previews of a whole map use less.
  // lightDebug: 1 shows the local lights alone (testapi gfxOptions).
  // lights: false renders as before map lights (no local lights, sky everywhere): for A/B benchmarks.
  // lightShadows: how many shadowed local lights get a shadow map (r_light_shadows, 0-4);
  // lightShadowSize: texels per tile (a spot uses one tile, a point light six).
  // dynShadows: how many dynamic lights (muzzle flashes...) get a cube shadow (r_dynlight_shadows, 0-4),
  // dynShadowSize: texels per cube face.
  const opts = { shadowSize: 2048, shadowQuality: 2, maxInFlight: 0, finish: false, fog: 0.014, aa: "auto", renderScale: 1, lightShadows: 2, lightShadowSize: 512, lights: true, lightDebug: 0, dynShadows: 2, dynShadowSize: 128 };
  const sh = { tex: null, fbo: null, size: 0, vp: null, texel: 0, cpuMs: 0, gpuMs: null };
  const timer = gl.getExtension("EXT_disjoint_timer_query_webgl2");
  const queries = []; // {q, kind: "shadow" | "frame"}
  let frameNo = 0, gpuFrameMs = null;
  // GPU fences, one per frame: frames in flight and latch -> GPU-done latency.
  const fences = [];
  const lat = { latchToGpu: null, submitToGpu: null, skipped: 0 };
  const ema = (old, v) => (old == null ? v : old * 0.9 + v * 0.1);

  function pollFences() {
    const now = performance.now();
    while (fences.length) {
      const r = gl.clientWaitSync(fences[0].s, 0, 0);
      if (r !== gl.ALREADY_SIGNALED && r !== gl.CONDITION_SATISFIED) break;
      const f = fences.shift();
      gl.deleteSync(f.s);
      if (f.latch != null) lat.latchToGpu = ema(lat.latchToGpu, now - f.latch);
      lat.submitToGpu = ema(lat.submitToGpu, now - f.t);
    }
    return fences.length;
  }
  // Whether a new frame may be queued now (the frames-in-flight limit).
  function canRender() {
    if (!opts.maxInFlight) return true;
    const ok = pollFences() < opts.maxInFlight;
    if (!ok) lat.skipped++;
    return ok;
  }
  // What the browser actually granted, and which GPU.
  // What the context got. Read once: getParameter is a synchronous round trip
  // to the GPU process (it waits for the frames in flight: ~0.1-0.2 s a call
  // under SwiftShader), and none of it changes for the context's life.
  let infoCache = null;
  function info() {
    if (infoCache) return infoCache;
    const a = gl.getContextAttributes() || {};
    const dbg = gl.getExtension("WEBGL_debug_renderer_info");
    infoCache = {
      desynchronized: !!a.desynchronized, powerPreference: a.powerPreference || "default",
      gpu: String(gl.getParameter(dbg ? dbg.UNMASKED_RENDERER_WEBGL : gl.RENDERER)), timerQuery: !!timer,
      antialias: !!a.antialias, msaaSamples: msaaSamples(),
    };
    return infoCache;
  }

  function setOptions(o) { Object.assign(opts, o); }

  // Sample counts the GPU can do for an RGBA8 renderbuffer (descending), read once.
  let samplesCache = null;
  function msaaSamples() {
    if (!samplesCache) {
      try { samplesCache = [...(gl.getInternalformatParameter(gl.RENDERBUFFER, gl.RGBA8, gl.SAMPLES) || [])]; } catch { samplesCache = []; }
    }
    return samplesCache;
  }
  let granted = null; // the canvas's own MSAA, read once
  function plan() {
    if (granted == null) granted = !!(gl.getContextAttributes() || {}).antialias;
    return aaPlan(opts.aa, { builtin: granted, samples: msaaSamples() }, opts.renderScale);
  }

  // The offscreen target (any AA mode but the direct one): a colour texture + depth, and a
  // multisampled colour + depth pair in front of it when MSAA is on (blit-resolved into the texture).
  const tg = { w: 0, h: 0, samples: -1, fbo: null, tex: null, depth: null, msFbo: null, msColor: null, msDepth: null };
  function ensureTarget(w, h, samples) {
    if (tg.w === w && tg.h === h && tg.samples === samples) return;
    for (const r of [tg.depth, tg.msColor, tg.msDepth]) if (r) gl.deleteRenderbuffer(r);
    for (const f of [tg.fbo, tg.msFbo]) if (f) gl.deleteFramebuffer(f);
    if (tg.tex) gl.deleteTexture(tg.tex);
    const rb = (fmt, n) => {
      const r = gl.createRenderbuffer();
      gl.bindRenderbuffer(gl.RENDERBUFFER, r);
      if (n) gl.renderbufferStorageMultisample(gl.RENDERBUFFER, n, fmt, w, h);
      else gl.renderbufferStorage(gl.RENDERBUFFER, fmt, w, h);
      return r;
    };
    tg.tex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, tg.tex);
    gl.texStorage2D(gl.TEXTURE_2D, 1, gl.RGBA8, w, h);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    tg.fbo = gl.createFramebuffer();
    gl.bindFramebuffer(gl.FRAMEBUFFER, tg.fbo);
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tg.tex, 0);
    tg.depth = samples ? null : rb(gl.DEPTH_COMPONENT24, 0);
    if (tg.depth) gl.framebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, tg.depth);
    tg.msFbo = tg.msColor = tg.msDepth = null;
    if (samples) {
      tg.msColor = rb(gl.RGBA8, samples);
      tg.msDepth = rb(gl.DEPTH_COMPONENT24, samples);
      tg.msFbo = gl.createFramebuffer();
      gl.bindFramebuffer(gl.FRAMEBUFFER, tg.msFbo);
      gl.framebufferRenderbuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, tg.msColor);
      gl.framebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, tg.msDepth);
    }
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    Object.assign(tg, { w, h, samples });
  }
  function freeTarget() {
    if (!tg.fbo) return;
    for (const r of [tg.depth, tg.msColor, tg.msDepth]) if (r) gl.deleteRenderbuffer(r);
    for (const f of [tg.fbo, tg.msFbo]) if (f) gl.deleteFramebuffer(f);
    gl.deleteTexture(tg.tex);
    Object.assign(tg, { w: 0, h: 0, samples: -1, fbo: null, tex: null, depth: null, msFbo: null, msColor: null, msDepth: null });
  }

  function ensureShadowMap() {
    const size = opts.shadowSize;
    if (sh.size === size) return;
    if (sh.tex) { gl.deleteTexture(sh.tex); gl.deleteFramebuffer(sh.fbo); }
    sh.tex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, sh.tex);
    gl.texStorage2D(gl.TEXTURE_2D, 1, gl.DEPTH_COMPONENT24, size, size);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_COMPARE_MODE, gl.COMPARE_REF_TO_TEXTURE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_COMPARE_FUNC, gl.LEQUAL);
    sh.fbo = gl.createFramebuffer();
    gl.bindFramebuffer(gl.FRAMEBUFFER, sh.fbo);
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.TEXTURE_2D, sh.tex, 0);
    gl.drawBuffers([gl.NONE]);
    gl.readBuffer(gl.NONE);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    sh.size = size;
    sh.vp = null;
  }

  // One fixed orthographic frustum from the sun, fitted around the map's bounds
  // (plus headroom for players and jumps): stable, no shimmering.
  function fitShadow() {
    const [mn, mx] = mapBounds;
    const c = [(mn[0] + mx[0]) / 2, (mn[1] + mx[1]) / 2, (mn[2] + mx[2]) / 2];
    const up = Math.abs(sun[1]) > 0.99 ? [0, 0, 1] : [0, 1, 0];
    const view = lookAt([c[0] + sun[0], c[1] + sun[1], c[2] + sun[2]], c, up);
    let l = Infinity, r = -Infinity, b = Infinity, t = -Infinity, n = Infinity, f = -Infinity;
    for (let i = 0; i < 8; i++) {
      const p = [i & 1 ? mx[0] : mn[0], i & 2 ? mx[1] : mn[1], i & 4 ? mx[2] : mn[2]];
      const x = view[0] * p[0] + view[4] * p[1] + view[8] * p[2] + view[12];
      const y = view[1] * p[0] + view[5] * p[1] + view[9] * p[2] + view[13];
      const z = view[2] * p[0] + view[6] * p[1] + view[10] * p[2] + view[14];
      l = Math.min(l, x); r = Math.max(r, x); b = Math.min(b, y); t = Math.max(t, y);
      n = Math.min(n, -z); f = Math.max(f, -z);
    }
    sh.vp = mul(ortho(l, r, b, t, n - 1, f + 1), view);
    sh.texel = Math.max(r - l, t - b) / sh.size;
  }

  function shadowPass(solids) {
    ensureShadowMap();
    if (!sh.vp) fitShadow();
    const t0 = performance.now();
    let q = null;
    if (timer && queries.length < 4 && frameNo % 2) { q = gl.createQuery(); gl.beginQuery(timer.TIME_ELAPSED_EXT, q); }
    gl.bindFramebuffer(gl.FRAMEBUFFER, sh.fbo);
    gl.viewport(0, 0, sh.size, sh.size);
    gl.clear(gl.DEPTH_BUFFER_BIT);
    gl.enable(gl.DEPTH_TEST);
    gl.enable(gl.POLYGON_OFFSET_FILL);
    gl.polygonOffset(1.5, 2.0);
    gl.useProgram(depthProg);
    gl.uniformMatrix4fv(loc(gl, depthProg, "uVP"), false, sh.vp);
    gl.uniformMatrix4fv(loc(gl, depthProg, "uModel"), false, ID);
    gl.bindVertexArray(mapVao);
    gl.drawElements(gl.TRIANGLES, mapCount, gl.UNSIGNED_INT, 0);
    gl.bindVertexArray(cubeVao);
    for (const s of solids) {
      if (s.emit >= 1) continue; // glowing debug lines, muzzle flashes, puffs: no shadow
      gl.uniformMatrix4fv(loc(gl, depthProg, "uModel"), false, s.m);
      gl.drawElements(gl.TRIANGLES, cube.idx.length, gl.UNSIGNED_INT, 0);
    }
    gl.disable(gl.POLYGON_OFFSET_FILL);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    if (q) { gl.endQuery(timer.TIME_ELAPSED_EXT); queries.push({ q, kind: "shadow" }); }
    sh.cpuMs = sh.cpuMs * 0.9 + (performance.now() - t0) * 0.1;
  }

  // ---- shadowed local lights ----
  // Static: every Shadow light gets its tiles in one atlas (lights.js atlasLayout),
  // the map's depth rendered once after load, BAKE_TILES a frame, so walls and shelves
  // always cast. Live: the lights with players near (dynamicScore, the top
  // r_light_shadows) copy their tiles into a small pool and draw the players on top,
  // fading in and out over FADE_S. Only visibility is cached: the lights themselves
  // stay live (on/off, flicker, colour).
  const BAKE_TILES = 6, FADE_S = 0.3, MAX_BLOBS = 8;
  let blobs = { n: 0, data: new Float32Array(MAX_BLOBS * 4) };
  const ls = {
    lay: null, stat: null, statFbo: null, matTex: null, mats: null, queue: [], baked: 0,
    dyn: null, dynFbo: null, dynUsed: null, dynCols: 1, dynCap: 0, live: new Map(), // light -> {base, n, w, target, had}
    dummy: depthTex(1, 1, true), t: null, cpuMs: 0, gpuMs: null, bakeMs: 0, mapRenders: 0, solidRenders: 0, bytes: 0,
  };
  function depthTex(w, h, compare) {
    const t = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, t);
    gl.texStorage2D(gl.TEXTURE_2D, 1, gl.DEPTH_COMPONENT16, w, h);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, compare ? gl.LINEAR : gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, compare ? gl.LINEAR : gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    if (compare) {
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_COMPARE_MODE, gl.COMPARE_REF_TO_TEXTURE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_COMPARE_FUNC, gl.LEQUAL);
    }
    return t;
  }
  function depthFbo(t) {
    const f = gl.createFramebuffer();
    gl.bindFramebuffer(gl.FRAMEBUFFER, f);
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.TEXTURE_2D, t, 0);
    gl.drawBuffers([gl.NONE]);
    gl.readBuffer(gl.NONE);
    return f;
  }
  function freeLightShadows() {
    for (const t of [ls.stat, ls.matTex, ls.dyn]) if (t) gl.deleteTexture(t);
    for (const f of [ls.statFbo, ls.dynFbo]) if (f) gl.deleteFramebuffer(f);
    Object.assign(ls, { lay: null, stat: null, statFbo: null, matTex: null, mats: null, queue: [], baked: 0,
      dyn: null, dynFbo: null, dynUsed: null, dynCap: 0, live: new Map(), bytes: 0, bakeMs: 0, t: null });
  }
  // A new map: lay out the static atlas and queue every shadowed light for baking.
  function setupLightShadows(lights) {
    freeLightShadows();
    const lay = atlasLayout(lights);
    if (!lay.tiles) return;
    ls.lay = lay;
    ls.stat = depthTex(lay.width, lay.height, true);
    ls.statFbo = depthFbo(ls.stat);
    ls.mats = new Float32Array(lay.tiles * 20);
    ls.matTex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, ls.matTex);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, 5, lay.tiles, 0, gl.RGBA, gl.FLOAT, ls.mats);
    for (const k of [gl.TEXTURE_MIN_FILTER, gl.TEXTURE_MAG_FILTER]) gl.texParameteri(gl.TEXTURE_2D, k, gl.NEAREST);
    ls.queue = lights.map((l, i) => i).filter((i) => lay.base[i] >= 0);
    ls.bytes = lay.width * lay.height * 2 + lay.tiles * 80;
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
  }
  // The live pool: room for r_light_shadows lights of up to 6 tiles, plus one fading out.
  function ensureDynPool(n) {
    const cap = n ? (n + 1) * 6 : 0;
    if (ls.dynCap === cap) return;
    for (const [li] of ls.live) lg.setShadow(li, ls.lay.base[li], -1, 0);
    ls.live = new Map();
    if (ls.dyn) { gl.deleteTexture(ls.dyn); gl.deleteFramebuffer(ls.dynFbo); ls.dyn = ls.dynFbo = null; }
    ls.dynCap = cap;
    ls.dynUsed = new Uint8Array(cap);
    if (!cap) return;
    const T = ls.lay.tile;
    ls.dynCols = Math.min(cap, Math.floor(4096 / T));
    ls.dyn = depthTex(ls.dynCols * T, Math.ceil(cap / ls.dynCols) * T, true);
    ls.dynFbo = depthFbo(ls.dyn);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
  }
  function drawDepth(fbo, vp, x, y, size, map, solids) {
    gl.bindFramebuffer(gl.FRAMEBUFFER, fbo);
    gl.viewport(x, y, size, size);
    gl.uniformMatrix4fv(loc(gl, depthProg, "uVP"), false, vp);
    if (map) {
      gl.enable(gl.SCISSOR_TEST);
      gl.scissor(x, y, size, size);
      gl.clear(gl.DEPTH_BUFFER_BIT);
      gl.disable(gl.SCISSOR_TEST);
      gl.uniformMatrix4fv(loc(gl, depthProg, "uModel"), false, ID);
      gl.bindVertexArray(mapVao);
      gl.drawElements(gl.TRIANGLES, mapCount, gl.UNSIGNED_INT, 0);
    }
    if (solids?.length) {
      gl.bindVertexArray(cubeVao);
      for (const sd of solids) {
        gl.uniformMatrix4fv(loc(gl, depthProg, "uModel"), false, sd.m);
        gl.drawElements(gl.TRIANGLES, cube.idx.length, gl.UNSIGNED_INT, 0);
      }
    }
  }
  // Solids that cast into a light's shadow: not glowing, their centre within range
  // (plus their own size).
  function castersFor(l, solids) {
    const out = [];
    for (const sd of solids) {
      if (sd.emit >= 1) continue;
      const m = sd.m, r = Math.hypot(m[0], m[1], m[2]) + Math.hypot(m[4], m[5], m[6]) + Math.hypot(m[8], m[9], m[10]);
      const dx = m[12] - l.pos[0], dy = m[13] - l.pos[1], dz = m[14] - l.pos[2];
      if (dx * dx + dy * dy + dz * dz < (l.range + r) ** 2) out.push(sd);
    }
    return out;
  }
  function lightShadowPass(eye, solids, players) {
    const lights = lg.lights(), lay = ls.lay;
    if (!lay || !opts.lights) return;
    const now = performance.now(), dt = ls.t == null ? 0 : Math.min(0.1, (now - ls.t) / 1000);
    ls.t = now;
    const t0 = now, T = lay.tile;
    let q = null;
    if (timer && queries.length < 6 && frameNo % 2) { q = gl.createQuery(); gl.beginQuery(timer.TIME_ELAPSED_EXT, q); }
    gl.useProgram(depthProg);
    gl.enable(gl.DEPTH_TEST);
    gl.enable(gl.POLYGON_OFFSET_FILL);
    gl.polygonOffset(2.0, 4.0);
    // Bake: the map's depth into the next queued lights' static tiles.
    let budget = BAKE_TILES;
    const bakedNow = [];
    while (ls.queue.length && budget > 0) {
      const li = ls.queue.shift(), base = lay.base[li];
      const views = shadowViews(lights[li], T);
      views.forEach((v, f) => {
        const [tx, ty] = tileXY(base + f, lay.cols);
        ls.mats.set(v.vp, (base + f) * 20);
        ls.mats[(base + f) * 20 + 16] = v.texel;
        drawDepth(ls.statFbo, v.vp, tx * T, ty * T, T, true, null);
      });
      gl.bindTexture(gl.TEXTURE_2D, ls.matTex);
      gl.texSubImage2D(gl.TEXTURE_2D, 0, 0, base, 5, views.length, gl.RGBA, gl.FLOAT, ls.mats, base * 20);
      bakedNow.push(li);
      ls.baked++; ls.mapRenders += views.length; budget -= views.length;
    }
    for (const li of bakedNow) lg.setShadow(li, lay.base[li], -1, 0);
    if (bakedNow.length) ls.bakeMs += performance.now() - t0;
    // Live: score the baked lights by the players near them, keep the top n.
    const n = Math.min(MAX_SHADOW_SLOTS, opts.lightShadows | 0);
    ensureDynPool(n);
    if (n) {
      const onScreen = (p) => !vp || sphereInFrustum(vp, [p[0], p[1] + 0.9, p[2]], 1);
      const scores = lights.map((l, i) => (lay.base[i] >= 0 && !ls.queue.includes(i) ? dynamicScore(l, players, eye, onScreen) : 0));
      const picks = new Set(pickDynamic(scores, n, new Set([...ls.live].filter(([, e]) => e.target).map(([i]) => i))));
      for (const [li, e] of ls.live) e.target = picks.has(li) ? 1 : 0;
      for (const li of picks) {
        if (ls.live.has(li)) continue;
        const k = tilesOf(lights[li]), base = allocTiles(ls.dynUsed, k);
        if (base < 0) continue; // the pool is full of fading lights: next frame
        ls.live.set(li, { base, n: k, w: 0, target: 1, had: -1 });
      }
      for (const [li, e] of ls.live) {
        e.w = fadeToward(e.w, e.target, dt, FADE_S);
        if (!e.w && !e.target) { ls.dynUsed.fill(0, e.base, e.base + e.n); ls.live.delete(li); lg.setShadow(li, lay.base[li], -1, 0); continue; }
        const casters = castersFor(lights[li], solids);
        if (e.had === 0 && !casters.length) { lg.setShadow(li, lay.base[li], e.base, e.w); continue; } // still clean
        // Copy the static tiles in, draw the players on top.
        gl.bindFramebuffer(gl.READ_FRAMEBUFFER, ls.statFbo);
        gl.bindFramebuffer(gl.DRAW_FRAMEBUFFER, ls.dynFbo);
        for (let f = 0; f < e.n; f++) {
          const [sx, sy] = tileXY(lay.base[li] + f, lay.cols), [dx, dy] = tileXY(e.base + f, ls.dynCols);
          gl.blitFramebuffer(sx * T, sy * T, (sx + 1) * T, (sy + 1) * T, dx * T, dy * T, (dx + 1) * T, (dy + 1) * T, gl.DEPTH_BUFFER_BIT, gl.NEAREST);
        }
        if (casters.length) {
          for (let f = 0; f < e.n; f++) {
            const [dx, dy] = tileXY(e.base + f, ls.dynCols);
            drawDepth(ls.dynFbo, ls.mats.subarray((lay.base[li] + f) * 20, (lay.base[li] + f) * 20 + 16), dx * T, dy * T, T, false, casters);
          }
          ls.solidRenders++;
        }
        e.had = casters.length;
        lg.setShadow(li, lay.base[li], e.base, e.w);
      }
    }
    gl.disable(gl.POLYGON_OFFSET_FILL);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    if (q) { gl.endQuery(timer.TIME_ELAPSED_EXT); queries.push({ q, kind: "lshadow" }); }
    ls.cpuMs = ls.cpuMs * 0.9 + (performance.now() - t0) * 0.1;
  }
  // ---- cube shadows for dynamic lights ----
  // The dynShadows most important dynamic lights (engine/dynlights.js planShadows) get
  // 6 faces of dynShadowSize^2 in one atlas (a row per slot): the map (one draw a face)
  // and the solids near the light, rendered when the light starts (and every frame for
  // long-lived lights). Every dynamic light keeps its cell mask too.
  const ds = { tex: null, fbo: null, matTex: null, mats: null, size: 0, cap: 0, renders: 0, cpuMs: 0, gpuMs: null };
  function ensureDynShadows(n, size) {
    if (ds.cap === n && ds.size === size) return;
    if (ds.tex) { gl.deleteTexture(ds.tex); gl.deleteFramebuffer(ds.fbo); gl.deleteTexture(ds.matTex); }
    Object.assign(ds, { tex: null, fbo: null, matTex: null, mats: null, cap: n, size });
    if (!n) return;
    ds.tex = depthTex(6 * size, n * size, true);
    ds.fbo = depthFbo(ds.tex);
    ds.mats = new Float32Array(n * 6 * 20);
    ds.matTex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, ds.matTex);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, 5, n * 6, 0, gl.RGBA, gl.FLOAT, ds.mats);
    for (const k of [gl.TEXTURE_MIN_FILTER, gl.TEXTURE_MAG_FILTER]) gl.texParameteri(gl.TEXTURE_2D, k, gl.NEAREST);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
  }
  function dynShadowPass(solids) {
    const n = mapVao && opts.lights ? Math.max(0, Math.min(4, opts.dynShadows | 0)) : 0;
    ensureDynShadows(n, opts.dynShadowSize | 0 || 128);
    const onScreen = (l) => !vp || sphereInFrustum(vp, l.pos, l.range * 0.5);
    const jobs = fl.plan(n, onScreen);
    if (!jobs.length) return;
    const t0 = performance.now(), S = ds.size;
    let q = null;
    if (timer && queries.length < 6 && !(frameNo % 2)) { q = gl.createQuery(); gl.beginQuery(timer.TIME_ELAPSED_EXT, q); }
    gl.useProgram(depthProg);
    gl.enable(gl.DEPTH_TEST);
    gl.enable(gl.POLYGON_OFFSET_FILL);
    gl.polygonOffset(2.0, 4.0);
    for (const { light } of jobs) {
      const l = { kind: "point", pos: light.pos, range: light.range }, s = light.sh;
      // Not the solids the light sits in (the shooter's own gun): they'd shadow everything.
      const casters = castersFor(l, solids).filter((sd) => {
        const m = sd.m, r = 0.5 * Math.max(Math.hypot(m[0], m[1], m[2]), Math.hypot(m[4], m[5], m[6]), Math.hypot(m[8], m[9], m[10]));
        return Math.hypot(m[12] - l.pos[0], m[13] - l.pos[1], m[14] - l.pos[2]) > r + 0.15;
      });
      shadowViews(l, S).forEach((v, f) => {
        ds.mats.set(v.vp, (s * 6 + f) * 20);
        ds.mats[(s * 6 + f) * 20 + 16] = v.texel;
        drawDepth(ds.fbo, v.vp, f * S, s * S, S, true, casters);
      });
      ds.renders++;
    }
    gl.bindTexture(gl.TEXTURE_2D, ds.matTex);
    gl.texSubImage2D(gl.TEXTURE_2D, 0, 0, 0, 5, ds.cap * 6, gl.RGBA, gl.FLOAT, ds.mats);
    gl.disable(gl.POLYGON_OFFSET_FILL);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    if (q) { gl.endQuery(timer.TIME_ELAPSED_EXT); queries.push({ q, kind: "dshadow" }); }
    ds.cpuMs = ds.cpuMs * 0.9 + (performance.now() - t0) * 0.1;
  }
  function lightShadowUniforms(p) {
    const lay = ls.lay;
    gl.uniform1i(loc(gl, p, "uLSOn"), lay && ls.baked ? 1 : 0);
    if (!lay) return;
    gl.uniform1f(loc(gl, p, "uLSTile"), lay.tile);
    gl.uniform2f(loc(gl, p, "uLSAtlas"), lay.width, lay.height);
    gl.uniform1i(loc(gl, p, "uLSCols"), lay.cols);
    gl.uniform2f(loc(gl, p, "uLDAtlas"), ls.dynCols * lay.tile, Math.max(1, Math.ceil(ls.dynCap / ls.dynCols)) * lay.tile);
    gl.uniform1i(loc(gl, p, "uLDCols"), ls.dynCols);
  }

  function pollTimers() {
    if (!timer) return;
    if (gl.getParameter(timer.GPU_DISJOINT_EXT)) { queries.forEach((e) => gl.deleteQuery(e.q)); queries.length = 0; return; }
    while (queries.length && gl.getQueryParameter(queries[0].q, gl.QUERY_RESULT_AVAILABLE)) {
      const e = queries.shift(), ms = gl.getQueryParameter(e.q, gl.QUERY_RESULT) / 1e6;
      gl.deleteQuery(e.q);
      if (e.kind === "shadow") sh.gpuMs = ema(sh.gpuMs, ms);
      else if (e.kind === "lshadow") ls.gpuMs = ema(ls.gpuMs, ms);
      else if (e.kind === "dshadow") ds.gpuMs = ema(ds.gpuMs, ms);
      else gpuFrameMs = ema(gpuFrameMs, ms);
    }
  }

  // Costs and queue state for the perf overlay.
  function stats() {
    const q = mapVao ? opts.shadowQuality : 0;
    return {
      shadowQuality: q, shadowSize: sh.size, shadowCpuMs: q ? sh.cpuMs : 0, shadowGpuMs: q ? sh.gpuMs : null,
      mapPrograms: Object.values(variants).reduce((n, v) => n + Object.keys(v.maps).length, 0), lightGridBuilds: mapBuilds,
      aa: lastPlan.label, renderScale: lastPlan.scale, target: lastPlan.direct ? null : [tg.w, tg.h],
      gpuFrameMs, inFlight: fences.length, maxInFlight: opts.maxInFlight, finish: opts.finish,
      latchToGpuMs: lat.latchToGpu, submitToGpuMs: lat.submitToGpu, skipped: lat.skipped,
      ...lg.stats(), ...fl.stats(),
      dynShadows: fl.shadowed(), dynShadowBudget: ds.cap, dynShadowRenders: ds.renders, dynShadowCpuMs: ds.cpuMs, dynShadowGpuMs: ds.gpuMs,
      marks: inst.stats(), // {decals, bits, draws}
      // Lamp shadows: static tiles baked / to bake, their size, live ones (players drawn in) / budget.
      lightShadowTiles: ls.lay?.tiles ?? 0, lightShadowBaked: ls.baked, lightShadowQueued: ls.queue.length, lightShadowSize: ls.lay?.tile ?? 0,
      lightShadowBakeMs: ls.bakeMs, lightShadowKB: Math.round((ls.bytes + (ls.dyn ? ls.dynCols * ls.lay.tile * Math.ceil(ls.dynCap / ls.dynCols) * ls.lay.tile * 2 : 0)) / 1024),
      lightShadows: [...ls.live.values()].filter((e) => e.target).length, lightShadowBudget: ls.dynCap ? ls.dynCap / 6 - 1 : 0,
      lightShadowCpuMs: ls.lay ? ls.cpuMs : 0, lightShadowGpuMs: ls.lay ? ls.gpuMs : null,
      lightShadowMapRenders: ls.mapRenders, lightShadowSolidRenders: ls.solidRenders,
    };
  }

  // preview: a one-shot still (map thumbnails) with no lamp lighting, no
  // indoor sky grid and one all-materials shader shared by every map, so a
  // thumbnail costs a draw, not a light-grid build and a shader compile
  // (each preview froze the page for ~1 s).
  let mapBuilds = 0;
  function setMap(map, { preview = false } = {}) {
    const unknown = new Set();
    const m = boxMesh(map.boxes, (b) => {
      if (b.kind in kindIndex) return kindIndex[b.kind];
      unknown.add(b.kind);
      return -1; // magenta: the shader's fallback
    }, false);
    if (unknown.size) console.warn(`map uses unknown materials: ${[...unknown].join(", ")}`);
    usedKinds = preview ? new Set(Object.values(kindIndex)) : new Set(map.boxes.map((b) => kindIndex[b.kind]).filter((i) => i != null));
    usedKey = preview ? "all" : [...usedKinds].sort((a, b) => a - b).join(",");
    if (mapVao) gl.deleteVertexArray(mapVao);
    mapVao = gl.createVertexArray();
    gl.bindVertexArray(mapVao);
    attrib(gl, 0, 3, m.pos);
    attrib(gl, 1, 3, m.nrm);
    attrib(gl, 2, 4, m.face);
    attrib(gl, 3, 1, m.kind);
    indexBuf(gl, m.idx);
    mapCount = m.idx.length;
    const mn = [Infinity, Infinity, Infinity], mx = [-Infinity, -Infinity, -Infinity];
    for (const b of map.boxes) for (let a = 0; a < 3; a++) { mn[a] = Math.min(mn[a], b.min[a]); mx[a] = Math.max(mx[a], b.max[a]); }
    mx[1] = Math.max(mx[1], mn[1] + 1) + 4; // headroom for players above the tallest box
    mapBounds = [mn, mx];
    sh.vp = null;
    lg.set(preview ? { boxes: [], lights: [] } : map, opts);
    fl.reset();
    if (!preview) mapBuilds++;
    setupLightShadows(lg.lights());
  }

  function resize() {
    if (ctx.fixedSize) { aspect = canvas.width / Math.max(canvas.height, 1); return; } // offscreen: the caller sizes it
    const dpr = Math.min(window.devicePixelRatio || 1, 1.5);
    const w = Math.floor(canvas.clientWidth * dpr), h = Math.floor(canvas.clientHeight * dpr);
    if (canvas.width !== w || canvas.height !== h) { canvas.width = w; canvas.height = h; }
    aspect = w / Math.max(h, 1);
  }

  function common(p, eye, time) {
    gl.useProgram(p);
    u3(gl, p, "uEye", eye);
    u3(gl, p, "uSun", sun);
    u3(gl, p, "uFog", fogCol);
    gl.uniform1f(loc(gl, p, "uFogK"), fogFor(opts.fog, mapBounds));
    gl.uniform1f(loc(gl, p, "uTime"), time);
    lg.uniforms(p);
    fl.uniforms(p);
    gl.uniform1f(loc(gl, p, "uDSTile"), ds.size || 1);
    gl.uniform2f(loc(gl, p, "uDSAtlas"), ds.size * 6 || 1, ds.size * ds.cap || 1);
    lightShadowUniforms(p);
    gl.uniform1i(loc(gl, p, "uLightDebug"), opts.lightDebug | 0);
    gl.uniform1i(loc(gl, p, "uBlobN"), blobs.n);
    if (blobs.n) gl.uniform4fv(loc(gl, p, "uBlobs[0]"), blobs.data);
    const q = sh.vp && mapVao ? opts.shadowQuality : 0;
    gl.uniform1i(loc(gl, p, "uShadowQ"), q);
    if (q) {
      gl.uniformMatrix4fv(loc(gl, p, "uShadowVP"), false, sh.vp);
      gl.uniform1f(loc(gl, p, "uShadowTexel"), sh.texel);
    }
  }

  function drawSolids(list) {
    for (const s of list) {
      gl.uniformMatrix4fv(loc(gl, solidProg, "uModel"), false, s.m);
      u3(gl, solidProg, "uColor", s.color);
      gl.uniform1f(loc(gl, solidProg, "uEmit"), s.emit || 0);
      gl.drawElements(gl.TRIANGLES, cube.idx.length, gl.UNSIGNED_INT, 0);
    }
  }

  // f: {eye, yaw, pitch, time, fov?, latch? (performance.now() when input was sampled),
  //     solids: [{m, color, emit}], overlay: [...] drawn on top (viewmodel),
  //     marks?: {decals, bits, kinds}: instanced batches (engine/instanced.js), one draw each}
  let lastPlan = { label: "auto", scale: 1, direct: true };
  function frame(f) {
    pollTimers();
    pollFences();
    frameNo++;
    let fq = null;
    if (timer && queries.length < 4 && !(frameNo % 2)) { fq = gl.createQuery(); gl.beginQuery(timer.TIME_ELAPSED_EXT, fq); }
    if (mapVao && opts.shadowQuality > 0) shadowPass(f.solids || []);
    resize();
    fov = f.fov ?? 1.3; // vertical, radians; scopes zoom by passing less
    const proj = perspective(fov, aspect, 0.05, 400);
    vp = mul(proj, fpsView(f.eye, f.yaw, f.pitch));
    lg.cull(f.eye, vp);
    fl.update(f.dynNow, f.eye);
    if (mapVao) lightShadowPass(f.eye, f.solids || [], f.players || []);
    dynShadowPass(f.solids || []);
    fl.pack();
    blobs = contactBlobs(f.players || [], f.eye, MAX_BLOBS, blobs.data);
    ({ map: mapProg, solid: solidProg, defs: litDefs } = variant(opts.lights && lg.lights().length > 0, opts.lights && ls.baked > 0));
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, sh.tex); // uShadowMap = unit 0 (after the passes that bind textures)
    lg.bind();
    gl.activeTexture(gl.TEXTURE6);
    gl.bindTexture(gl.TEXTURE_2D, ls.stat || ls.dummy);
    gl.activeTexture(gl.TEXTURE7);
    gl.bindTexture(gl.TEXTURE_2D, ls.matTex || lg.dummyData());
    gl.activeTexture(gl.TEXTURE8);
    gl.bindTexture(gl.TEXTURE_2D, ls.dyn || ls.dummy);
    gl.activeTexture(gl.TEXTURE9);
    gl.bindTexture(gl.TEXTURE_2D_ARRAY, fl.tex);
    gl.activeTexture(gl.TEXTURE10);
    gl.bindTexture(gl.TEXTURE_2D, ds.tex || ls.dummy);
    gl.activeTexture(gl.TEXTURE11);
    gl.bindTexture(gl.TEXTURE_2D, ds.matTex || lg.dummyData());
    gl.activeTexture(gl.TEXTURE0);
    const pl = lastPlan = plan();
    if (pl.direct) {
      freeTarget();
      gl.viewport(0, 0, canvas.width, canvas.height);
    } else {
      const [w, h] = scaledSize(canvas.width, canvas.height, pl.scale);
      ensureTarget(w, h, pl.samples);
      gl.bindFramebuffer(gl.FRAMEBUFFER, pl.samples ? tg.msFbo : tg.fbo);
      gl.viewport(0, 0, w, h);
    }
    gl.clearColor(...fogCol, 1);
    gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);

    gl.disable(gl.DEPTH_TEST);
    common(skyProg, f.eye, f.time);
    const b = basis(f.yaw, f.pitch);
    u3(gl, skyProg, "uFwd", b.fwd);
    u3(gl, skyProg, "uRight", b.right);
    u3(gl, skyProg, "uUp", b.up);
    const ty = Math.tan(fov / 2);
    gl.uniform2f(loc(gl, skyProg, "uTan"), ty * aspect, ty);
    gl.bindVertexArray(skyVao);
    gl.drawArrays(gl.TRIANGLES, 0, 3);

    gl.enable(gl.DEPTH_TEST);
    gl.disable(gl.CULL_FACE);
    if (mapVao) {
      common(mapProg, f.eye, f.time);
      gl.uniformMatrix4fv(loc(gl, mapProg, "uVP"), false, vp);
      gl.bindVertexArray(mapVao);
      gl.drawElements(gl.TRIANGLES, mapCount, gl.UNSIGNED_INT, 0);
    }
    inst.draw(f.marks, litDefs, vp, (p) => common(p, f.eye, f.time)); // bullet holes, casings: one draw each

    common(solidProg, f.eye, f.time);
    gl.uniformMatrix4fv(loc(gl, solidProg, "uVP"), false, vp);
    gl.bindVertexArray(cubeVao);
    drawSolids(f.solids || []);
    if (f.overlay?.length) {
      gl.clear(gl.DEPTH_BUFFER_BIT);
      drawSolids(f.overlay);
    }
    if (!pl.direct) post(pl);
    if (fq) { gl.endQuery(timer.TIME_ELAPSED_EXT); queries.push({ q: fq, kind: "frame" }); }
    if (fences.length >= 8) gl.deleteSync(fences.shift().s); // never polled: don't leak
    fences.push({ s: gl.fenceSync(gl.SYNC_GPU_COMMANDS_COMPLETE, 0), latch: f.latch, t: performance.now() });
    gl.flush();
    if (opts.finish) gl.finish();
  }

  // Resolve the offscreen target onto the canvas: MSAA blit into the texture, then one
  // fullscreen triangle (plain copy or FXAA, linear filtered when upscaling).
  function post(pl) {
    if (pl.samples) {
      gl.bindFramebuffer(gl.READ_FRAMEBUFFER, tg.msFbo);
      gl.bindFramebuffer(gl.DRAW_FRAMEBUFFER, tg.fbo);
      gl.blitFramebuffer(0, 0, tg.w, tg.h, 0, 0, tg.w, tg.h, gl.COLOR_BUFFER_BIT, gl.NEAREST);
    }
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    gl.viewport(0, 0, canvas.width, canvas.height);
    gl.disable(gl.DEPTH_TEST);
    gl.useProgram(postProg);
    gl.activeTexture(gl.TEXTURE1);
    gl.bindTexture(gl.TEXTURE_2D, tg.tex);
    gl.uniform1i(loc(gl, postProg, "uSrc"), 1);
    gl.uniform2f(loc(gl, postProg, "uTexel"), 1 / tg.w, 1 / tg.h);
    gl.uniform1i(loc(gl, postProg, "uFxaa"), pl.fxaa ? 1 : 0);
    gl.bindVertexArray(skyVao);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
    gl.activeTexture(gl.TEXTURE0);
  }

  // World point -> CSS pixel coordinates, or null if behind the camera.
  function project(p) {
    if (!vp) return null;
    const x = vp[0] * p[0] + vp[4] * p[1] + vp[8] * p[2] + vp[12];
    const y = vp[1] * p[0] + vp[5] * p[1] + vp[9] * p[2] + vp[13];
    const w = vp[3] * p[0] + vp[7] * p[1] + vp[11] * p[2] + vp[15];
    if (w < 0.1) return null;
    return [(x / w * 0.5 + 0.5) * canvas.clientWidth, (1 - (y / w * 0.5 + 0.5)) * canvas.clientHeight, w];
  }

  // Wait for the GPU to finish everything queued (a 1-pixel read back): benchmarks only.
  const px = new Uint8Array(4);
  function sync() { gl.readPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, px); }

  // A dynamic light (engine/dynlights.js): {pos, color, intensity, range, life, curve} at now
  // (seconds, the clock frame()'s dynNow runs on). Returns its slot, or -1 when dropped.
  const addDynamicLight = (l, now) => fl.add(l, now);

  return { setMap, setOptions, frame, project, stats, canRender, info, sync, addDynamicLight };
}

// ---- dynamic lights (engine/dynlights.js on the GPU) ----

// A pool of MAX_DYN_LIGHTS point lights (muzzle flashes, later explosions and the like);
// each new one floods the light grid from its cell (cached per cell) and uploads its
// MASK^3 cell mask into its slot's layers. The grid exists on every map but previews.
function createDynLights(gl, lg, enabled) {
  const tex = gl.createTexture();
  gl.bindTexture(gl.TEXTURE_2D_ARRAY, tex);
  for (const k of [gl.TEXTURE_MIN_FILTER, gl.TEXTURE_MAG_FILTER]) gl.texParameteri(gl.TEXTURE_2D_ARRAY, k, gl.NEAREST);
  gl.texStorage3D(gl.TEXTURE_2D_ARRAY, 1, gl.R8, MASK, MASK, MAX_DYN_LIGHTS * MASK);
  const pool = createLightPool(MAX_DYN_LIGHTS), offs = Array.from({ length: MAX_DYN_LIGHTS }, () => [0, 0, 0]);
  const pos = new Float32Array(MAX_DYN_LIGHTS * 4), col = new Float32Array(MAX_DYN_LIGHTS * 4), off = new Int32Array(MAX_DYN_LIGHTS * 3);
  const shs = new Float32Array(MAX_DYN_LIGHTS * 4);
  // Per frame at most MAX_DYN_LIGHTS adds (more would only evict each other) and the
  // mask cache's flood budget (createMaskCache): the worst frame's cost is bounded
  // however many shooters there are. A slot already holding a mask skips its upload.
  const slotKey = new Float64Array(MAX_DYN_LIGHTS).fill(-1);
  let masks = null, masksOf = null, n = 0, eye = null, added = 0, dropped = 0, addMs = 0, peak = 0, live = [];
  let addsLeft = MAX_DYN_LIGHTS, frameMs = 0, frameMsMax = 0, uploads = 0;
  function add(f, now) {
    const g = lg.grid();
    if (!g || !enabled() || !(f.range > 0)) return -1;
    if (addsLeft <= 0 && !f.own) { dropped++; return -1; }
    const t0 = performance.now();
    if (masksOf !== g) { masks = createMaskCache(g); masksOf = g; }
    const m = masks.get(f.pos, Math.min(f.range, MASK / 2 * g.cell - g.cell), !!f.own);
    const slot = m ? pool.add(f, now, eye) : -1;
    if (slot < 0) { dropped++; frameMs += performance.now() - t0; return -1; }
    addsLeft--;
    offs[slot] = m.off;
    if (slotKey[slot] !== m.key) {
      slotKey[slot] = m.key;
      gl.bindTexture(gl.TEXTURE_2D_ARRAY, tex);
      gl.texSubImage3D(gl.TEXTURE_2D_ARRAY, 0, 0, 0, slot * MASK, MASK, MASK, MASK, gl.RED, gl.UNSIGNED_BYTE, m.data);
      uploads++;
    }
    added++;
    const ms = performance.now() - t0;
    addMs = addMs ? addMs * 0.9 + ms * 0.1 : ms;
    peak = Math.max(peak, ms);
    frameMs += ms;
    return slot;
  }
  // The lights lit at now (then plan(): their shadows; pack(): the uniform arrays).
  function update(now, e) {
    eye = e;
    addsLeft = MAX_DYN_LIGHTS; frameMsMax = Math.max(frameMsMax, frameMs); frameMs = 0;
    masks?.frame();
    live = now == null || !enabled() || !lg.grid() ? [] : pool.live(now);
  }
  // Shadow slots (budget of them) for the lit lights: the entries to render now.
  const plan = (budget, onScreen) => planShadows(live, budget, eye || [0, 0, 0], onScreen);
  function pack() {
    n = 0;
    for (const { slot, light, strength } of live) {
      pos.set([light.pos[0], light.pos[1], light.pos[2], light.range], n * 4);
      const c = light.color || [1, 1, 1];
      col.set([c[0] * strength, c[1] * strength, c[2] * strength, slot], n * 4);
      off.set(offs[slot], n * 3);
      shs[n * 4] = light.sh >= 0 ? light.sh : -1;
      n++;
    }
  }
  function uniforms(p) {
    gl.uniform1i(loc(gl, p, "uDynN"), n);
    if (!n) return;
    gl.uniform4fv(loc(gl, p, "uDynPos[0]"), pos);
    gl.uniform4fv(loc(gl, p, "uDynCol[0]"), col);
    gl.uniform3iv(loc(gl, p, "uDynOff[0]"), off);
    gl.uniform4fv(loc(gl, p, "uDynSh[0]"), shs);
  }
  // A new map: the flood's scratch (a few bytes per grid cell) is allocated now, not on the first shot.
  function reset() {
    pool.clear(); n = 0; live = []; slotKey.fill(-1);
    const g = lg.grid();
    masks = g ? createMaskCache(g) : null; masksOf = g;
  }
  // Perf overlay / benches: lit now, added / dropped (over the frame's budget, or weaker
  // than all lit), the add cost (mask + upload; EMA and peak), the worst frame's total,
  // floods / cache hits / skipped misses, mask uploads.
  const stats = () => ({
    dynLights: n, dynLightsAdded: added, dynLightsDropped: dropped, dynLightAddMs: addMs, dynLightAddPeakMs: peak,
    dynLightFrameMsMax: Math.max(frameMsMax, frameMs), dynLightUploads: uploads,
    ...(masks ? { dynLightFloods: masks.stats().floods, dynLightMaskHits: masks.stats().hits, dynLightMaskSkips: masks.stats().skipped } : {}),
  });
  return { add, update, plan, pack, uniforms, reset, stats, tex, shadowed: () => live.filter((e) => e.light.sh >= 0).length };
}

// ---- local lights and sky visibility (engine/lights.js on the GPU) ----

// Cull walk: zones (4 m) within this many open zone boundaries of the camera count.
const CULL_STEPS = 6;

function createLightGrid(gl, enabled) {
  const tex = (target, setup) => {
    const t = gl.createTexture();
    gl.bindTexture(target, t);
    gl.texParameteri(target, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
    gl.texParameteri(target, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
    gl.texParameteri(target, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(target, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    setup();
    return t;
  };
  gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
  // 1-texel placeholders, so every sampler always has a texture of its own type.
  const empty = {
    cells: tex(gl.TEXTURE_2D_ARRAY, () => gl.texImage3D(gl.TEXTURE_2D_ARRAY, 0, gl.R32UI, 1, 1, 1, 0, gl.RED_INTEGER, gl.UNSIGNED_INT, new Uint32Array(1))),
    list: tex(gl.TEXTURE_2D, () => gl.texImage2D(gl.TEXTURE_2D, 0, gl.R16UI, 1, 1, 0, gl.RED_INTEGER, gl.UNSIGNED_SHORT, new Uint16Array(1))),
    data: tex(gl.TEXTURE_2D, () => gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, 4, 1, 0, gl.RGBA, gl.FLOAT, new Float32Array(16))),
    sky: tex(gl.TEXTURE_2D_ARRAY, () => gl.texImage3D(gl.TEXTURE_2D_ARRAY, 0, gl.R8, 1, 1, 1, 0, gl.RED, gl.UNSIGNED_BYTE, new Uint8Array([255]))),
  };
  let cur = { ...empty }, grid = null, zones = null, lights = [], data = null;
  let cOff = [0, 0, 0], cDims = [1, 1, 1], buildMs = 0, entries = 0, gpuBytes = 0, active = [];
  const free = () => { for (const k of Object.keys(cur)) if (cur[k] !== empty[k]) gl.deleteTexture(cur[k]); cur = { ...empty }; };

  function set(map) {
    free();
    const t0 = performance.now();
    lights = map.lights || [];
    grid = null; zones = null; entries = 0; active = []; gpuBytes = 0;
    if (!map.boxes.length) { buildMs = 0; return; } // a preview: no grid
    grid = buildGrid(map.boxes);
    const sky = skyVisibility(grid, map.boxes);
    cur.sky = tex(gl.TEXTURE_2D_ARRAY, () => {
      gl.texImage3D(gl.TEXTURE_2D_ARRAY, 0, gl.R8, grid.nx, grid.nz, grid.ny, 0, gl.RED, gl.UNSIGNED_BYTE, sky);
      gl.texParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    });
    gpuBytes = grid.n;
    zones = null; entries = 0; active = [];
    if (lights.length) {
      const ll = buildLightLists(grid, lights);
      zones = buildZones(grid, ll.perLight);
      entries = ll.stats.entries;
      // Upload only the sub-box of cells any light reaches.
      const { nx, ny, nz } = grid;
      const lo = [nx, ny, nz], hi = [-1, -1, -1];
      for (let i = 0; i < grid.n; i++) {
        if (!ll.cells[i]) continue;
        const x = i % nx, z = ((i / nx) | 0) % nz, y = (i / (nx * nz)) | 0;
        if (x < lo[0]) lo[0] = x; if (y < lo[1]) lo[1] = y; if (z < lo[2]) lo[2] = z;
        if (x > hi[0]) hi[0] = x; if (y > hi[1]) hi[1] = y; if (z > hi[2]) hi[2] = z;
      }
      if (hi[0] >= 0) {
        cOff = lo; cDims = [hi[0] - lo[0] + 1, hi[1] - lo[1] + 1, hi[2] - lo[2] + 1];
        const sub = new Uint32Array(cDims[0] * cDims[1] * cDims[2]);
        for (let y = 0; y < cDims[1]; y++) for (let z = 0; z < cDims[2]; z++) {
          const src = lo[0] + nx * (lo[2] + z + nz * (lo[1] + y));
          sub.set(ll.cells.subarray(src, src + cDims[0]), cDims[0] * (z + cDims[2] * y));
        }
        cur.cells = tex(gl.TEXTURE_2D_ARRAY, () => gl.texImage3D(gl.TEXTURE_2D_ARRAY, 0, gl.R32UI, cDims[0], cDims[2], cDims[1], 0, gl.RED_INTEGER, gl.UNSIGNED_INT, sub));
        const rows = Math.max(1, Math.ceil(ll.list.length / LIST_W));
        const list = new Uint16Array(rows * LIST_W);
        list.set(ll.list);
        cur.list = tex(gl.TEXTURE_2D, () => gl.texImage2D(gl.TEXTURE_2D, 0, gl.R16UI, LIST_W, rows, 0, gl.RED_INTEGER, gl.UNSIGNED_SHORT, list));
        gpuBytes += sub.byteLength + list.byteLength;
      }
      data = packLights(lights);
      cur.data = tex(gl.TEXTURE_2D, () => gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, 4, lights.length, 0, gl.RGBA, gl.FLOAT, data));
      gpuBytes += data.byteLength;
    }
    buildMs = performance.now() - t0;
  }

  function bind() {
    const units = [[2, gl.TEXTURE_2D_ARRAY, cur.cells], [3, gl.TEXTURE_2D, cur.list], [4, gl.TEXTURE_2D, cur.data], [5, gl.TEXTURE_2D_ARRAY, cur.sky]];
    for (const [u, target, t] of units) { gl.activeTexture(gl.TEXTURE0 + u); gl.bindTexture(target, t); }
    gl.activeTexture(gl.TEXTURE0);
  }

  function uniforms(p) {
    const on = !!grid && enabled();
    gl.uniform1i(loc(gl, p, "uSkyOn"), on ? 1 : 0);
    gl.uniform1i(loc(gl, p, "uLightsOn"), on && cur.cells !== empty.cells ? 1 : 0);
    if (!on) return;
    u3(gl, p, "uLGOrigin", grid.origin);
    gl.uniform1f(loc(gl, p, "uLGCell"), grid.cell);
    gl.uniform3i(loc(gl, p, "uLGDims"), grid.nx, grid.ny, grid.nz);
    gl.uniform3i(loc(gl, p, "uLCOff"), cOff[0], cOff[1], cOff[2]);
    gl.uniform3i(loc(gl, p, "uLCDims"), cDims[0], cDims[1], cDims[2]);
    // Maps without lamps keep some sky ambient indoors (their interiors were made for it).
    gl.uniform1f(loc(gl, p, "uIndoor"), lights.length ? 0.3 : 0.55);
  }

  function cull(eye, vp) { active = zones ? activeLights(grid, zones, lights, eye, vp, CULL_STEPS) : []; return active; }

  const stats = () => ({ lights: lights.length, lightsActive: active.length, lightEntries: entries, lightBuildMs: buildMs, lightGpuKB: Math.round(gpuBytes / 1024) });

  // A light's shadow tiles: texel 3 = (cos inner, static base tile, live base tile, live weight); -1 none.
  function setShadow(li, base, dyn, w) {
    const o = li * 16 + 13;
    if (!data || (data[o] === base && data[o + 1] === dyn && data[o + 2] === w)) return;
    data[o] = base; data[o + 1] = dyn; data[o + 2] = w;
    gl.bindTexture(gl.TEXTURE_2D, cur.data);
    gl.texSubImage2D(gl.TEXTURE_2D, 0, 3, li, 1, 1, gl.RGBA, gl.FLOAT, data, li * 16 + 12);
  }

  return { set, bind, uniforms, cull, stats, setShadow, active: () => active, lights: () => lights, grid: () => grid, dummyData: () => empty.data };
}

// ---- mesh building ----

const FACES = [
  { n: [1, 0, 0], ax: 0, hi: 1, u: 2, v: 1 }, { n: [-1, 0, 0], ax: 0, hi: 0, u: 2, v: 1 },
  { n: [0, 1, 0], ax: 1, hi: 1, u: 0, v: 2 }, { n: [0, -1, 0], ax: 1, hi: 0, u: 0, v: 2 },
  { n: [0, 0, 1], ax: 2, hi: 1, u: 0, v: 1 }, { n: [0, 0, -1], ax: 2, hi: 0, u: 0, v: 1 },
];

function boxMesh(boxes, kindOf, allFaces) {
  const pos = [], nrm = [], face = [], kind = [], idx = [];
  for (const b of boxes) {
    const k = kindOf(b);
    for (const f of FACES) {
      if (!allFaces && f.n[1] < 0 && b.min[1] <= 0) continue; // nobody sees under the floor
      const base = pos.length / 3;
      const su = b.max[f.u] - b.min[f.u], sv = b.max[f.v] - b.min[f.v];
      for (const [i, j] of [[0, 0], [1, 0], [1, 1], [0, 1]]) {
        const p = [0, 0, 0];
        p[f.ax] = f.hi ? b.max[f.ax] : b.min[f.ax];
        p[f.u] = i ? b.max[f.u] : b.min[f.u];
        p[f.v] = j ? b.max[f.v] : b.min[f.v];
        pos.push(...p);
        nrm.push(...f.n);
        face.push(p[f.u] - b.min[f.u], p[f.v] - b.min[f.v], su, sv);
        kind.push(k);
      }
      idx.push(base, base + 1, base + 2, base, base + 2, base + 3);
    }
  }
  return {
    pos: new Float32Array(pos), nrm: new Float32Array(nrm), face: new Float32Array(face),
    kind: new Float32Array(kind), idx: new Uint32Array(idx),
  };
}

// ---- small GL helpers ----

function program(gl, vs, fs) {
  const p = gl.createProgram();
  for (const [type, src] of [[gl.VERTEX_SHADER, vs], [gl.FRAGMENT_SHADER, fs]]) {
    const s = gl.createShader(type);
    gl.shaderSource(s, src);
    gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(s) + "\n" + numbered(src));
    gl.attachShader(p, s);
  }
  gl.linkProgram(p);
  if (!gl.getProgramParameter(p, gl.LINK_STATUS)) throw new Error(gl.getProgramInfoLog(p));
  p.locs = {};
  return p;
}

const numbered = (src) => src.split("\n").map((l, i) => `${i + 1}: ${l}`).join("\n");

function loc(gl, p, name) {
  if (!(name in p.locs)) p.locs[name] = gl.getUniformLocation(p, name);
  return p.locs[name];
}

function u3(gl, p, name, v) { gl.uniform3f(loc(gl, p, name), v[0], v[1], v[2]); }

function attrib(gl, i, size, data) {
  gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer());
  gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW);
  gl.enableVertexAttribArray(i);
  gl.vertexAttribPointer(i, size, gl.FLOAT, false, 0, 0);
}

function indexBuf(gl, data) {
  gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, gl.createBuffer());
  gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, data, gl.STATIC_DRAW);
}

function norm(v) { const l = Math.hypot(...v); return v.map((x) => x / l); }
