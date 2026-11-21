// WebGL2 renderer core. Game-agnostic: the scene's look (palette, materials,
// sky) is passed in, and everything besides the map is drawn as "solids":
// unit cubes with a model matrix, a colour and an emissive amount. No textures.
import { perspective, mul, fpsView, basis, ortho, lookAt, identity, fogFor } from "./math.js";
import { prelude } from "../gen/glsl/index.js";
import { FXAA } from "../gen/glsl/fxaa.js";
import { aaPlan, scaledSize } from "./aa.js";

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

function mapFS(pal, materials) {
  const dispatch = materials.map((m, i) => `if (k == ${i}) return mat_${m.name}(s);`).join("\n  ");
  return `${prelude(pal)}
in vec3 vWorld; in vec3 vNrm; in vec4 vFace; flat in int vKind;
out vec4 outCol;
${SURF}
${materials.map((m) => m.glsl).join("\n")}
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
  outCol = vec4(fog(light(shade(s, vKind), vWorld, s.n, ao), vWorld), 1.0);
}`;
}

const SOLID_VS = `#version 300 es
layout(location=0) in vec3 aPos; layout(location=1) in vec3 aNrm;
uniform mat4 uVP; uniform mat4 uModel;
out vec3 vWorld; out vec3 vNrm;
void main(){ vec4 w = uModel * vec4(aPos, 1.0); vWorld = w.xyz; vNrm = mat3(uModel) * aNrm; gl_Position = uVP * w; }`;

function solidFS(pal) {
  return `${prelude(pal)}
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

  const mapProg = program(gl, MAP_VS, mapFS(pal, look.materials));
  const solidProg = program(gl, SOLID_VS, solidFS(pal));
  const skyProg = program(gl, SKY_VS, skyFS(pal, look.sky));
  const depthProg = program(gl, DEPTH_VS, DEPTH_FS);
  const postProg = program(gl, SKY_VS, POST_FS);
  const ID = identity();

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
  const opts = { shadowSize: 2048, shadowQuality: 2, maxInFlight: 0, finish: false, fog: 0.014, aa: "auto", renderScale: 1 };
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

  function pollTimers() {
    if (!timer) return;
    if (gl.getParameter(timer.GPU_DISJOINT_EXT)) { queries.forEach((e) => gl.deleteQuery(e.q)); queries.length = 0; return; }
    while (queries.length && gl.getQueryParameter(queries[0].q, gl.QUERY_RESULT_AVAILABLE)) {
      const e = queries.shift(), ms = gl.getQueryParameter(e.q, gl.QUERY_RESULT) / 1e6;
      gl.deleteQuery(e.q);
      if (e.kind === "shadow") sh.gpuMs = ema(sh.gpuMs, ms);
      else gpuFrameMs = ema(gpuFrameMs, ms);
    }
  }

  // Costs and queue state for the perf overlay.
  function stats() {
    const q = mapVao ? opts.shadowQuality : 0;
    return {
      shadowQuality: q, shadowSize: sh.size, shadowCpuMs: q ? sh.cpuMs : 0, shadowGpuMs: q ? sh.gpuMs : null,
      aa: lastPlan.label, renderScale: lastPlan.scale, target: lastPlan.direct ? null : [tg.w, tg.h],
      gpuFrameMs, inFlight: fences.length, maxInFlight: opts.maxInFlight, finish: opts.finish,
      latchToGpuMs: lat.latchToGpu, submitToGpuMs: lat.submitToGpu, skipped: lat.skipped,
    };
  }

  function setMap(map) {
    const unknown = new Set();
    const m = boxMesh(map.boxes, (b) => {
      if (b.kind in kindIndex) return kindIndex[b.kind];
      unknown.add(b.kind);
      return -1; // magenta: the shader's fallback
    }, false);
    if (unknown.size) console.warn(`map uses unknown materials: ${[...unknown].join(", ")}`);
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
  //     solids: [{m, color, emit}], overlay: [...] drawn on top (viewmodel)}
  let lastPlan = { label: "auto", scale: 1, direct: true };
  function frame(f) {
    pollTimers();
    pollFences();
    frameNo++;
    let fq = null;
    if (timer && queries.length < 4 && !(frameNo % 2)) { fq = gl.createQuery(); gl.beginQuery(timer.TIME_ELAPSED_EXT, fq); }
    if (mapVao && opts.shadowQuality > 0) shadowPass(f.solids || []);
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, sh.tex); // uShadowMap = unit 0
    resize();
    fov = f.fov ?? 1.3; // vertical, radians; scopes zoom by passing less
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
    const proj = perspective(fov, aspect, 0.05, 400);
    vp = mul(proj, fpsView(f.eye, f.yaw, f.pitch));

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

  return { setMap, setOptions, frame, project, stats, canRender, info };
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
