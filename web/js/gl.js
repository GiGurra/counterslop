// WebGL2 renderer: everything is procedural shading on boxes. No textures.
import { perspective, mul, translate, scale, rotY, rotX, fpsView, basis } from "./math.js";

const KIND = { floor: 0, wall: 1, crate: 2, trim: 3 };
export const SUN = norm([0.45, 0.8, 0.3]);
const FOG = [1.0, 0.84, 0.66];

const COMMON = `
uniform vec3 uEye; uniform vec3 uSun; uniform vec3 uFog; uniform float uTime;
float hash(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }
float noise(vec2 p){
  vec2 i = floor(p), f = fract(p); f = f*f*(3.0-2.0*f);
  return mix(mix(hash(i), hash(i+vec2(1,0)), f.x), mix(hash(i+vec2(0,1)), hash(i+vec2(1,1)), f.x), f.y);
}
float fbm(vec2 p){ float s = 0.0, a = 0.5; for (int i = 0; i < 4; i++) { s += a*noise(p); p = p*2.03 + 7.1; a *= 0.5; } return s; }
// 1 on grid lines (integer crossings of x), antialiased; w is the half width in x units.
float gridLine(float x, float w){ float d = abs(fract(x - 0.5) - 0.5); float fw = fwidth(x); return 1.0 - smoothstep(w - fw, w + fw, d); }
vec3 light(vec3 col, vec3 n, float ao){
  float diff = max(dot(n, uSun), 0.0);
  vec3 hemi = mix(vec3(0.42, 0.36, 0.30), vec3(0.55, 0.65, 0.85), n.y*0.5 + 0.5);
  return col * (hemi*0.75 + vec3(1.0, 0.92, 0.78)*diff*0.8) * ao;
}
vec3 fog(vec3 col, vec3 world){
  float d = length(world - uEye);
  return mix(col, uFog, (1.0 - exp(-d*0.014)) * 0.85);
}`;

const MAP_VS = `#version 300 es
layout(location=0) in vec3 aPos; layout(location=1) in vec3 aNrm;
layout(location=2) in vec4 aFace; layout(location=3) in float aKind;
uniform mat4 uVP;
out vec3 vWorld; out vec3 vNrm; out vec4 vFace; flat out int vKind;
void main(){ vWorld = aPos; vNrm = aNrm; vFace = aFace; vKind = int(aKind + 0.5); gl_Position = uVP * vec4(aPos, 1.0); }`;

const MAP_FS = `#version 300 es
precision highp float;
in vec3 vWorld; in vec3 vNrm; in vec4 vFace; flat in int vKind;
out vec4 outCol;
${COMMON}
void main(){
  vec3 n = normalize(vNrm);
  vec2 uv = vFace.xy, sz = vFace.zw;
  // world-aligned uv so patterns line up across neighbouring boxes
  vec2 wuv = abs(n.x) > 0.5 ? vWorld.zy : abs(n.y) > 0.5 ? vWorld.xz : vWorld.xy;
  bool top = n.y > 0.5;
  vec3 col; float ao = 1.0;
  if (!top) ao = mix(0.55, 1.0, smoothstep(0.0, 1.6, vWorld.y));

  if (vKind == 0) { // sand floor + tiles + puddles of slop
    float g = fbm(wuv*0.7);
    col = vec3(0.86, 0.72, 0.48) * (0.82 + 0.35*g);
    float tile = max(gridLine(wuv.x/4.0, 0.012), gridLine(wuv.y/4.0, 0.012));
    col = mix(col, col*0.7, tile);
    float slop = smoothstep(0.66, 0.70, fbm(wuv*0.11 + 3.0));
    float wobble = 0.5 + 0.5*sin(uTime*2.0 + fbm(wuv*0.8)*6.0);
    col = mix(col, vec3(0.45, 0.85, 0.15) + 0.12*wobble, slop);
  } else if (vKind == 1) { // bricks
    if (top) {
      col = vec3(0.72, 0.70, 0.66) * (0.85 + 0.25*fbm(wuv*2.0));
    } else {
      float row = floor(wuv.y / 0.5);
      float u = wuv.x + mod(row, 2.0)*0.5;
      vec2 cell = vec2(floor(u), row);
      float h = hash(cell);
      vec3 brick = mix(vec3(0.74, 0.40, 0.30), vec3(0.92, 0.62, 0.42), h) * (0.88 + 0.2*fbm(wuv*5.0));
      float mortar = max(gridLine(u, 0.03), gridLine(wuv.y/0.5, 0.07));
      col = mix(brick, vec3(0.82, 0.78, 0.70), mortar);
    }
  } else if (vKind == 2) { // crates: planks, frame, diagonal brace
    float edge = min(min(uv.x, sz.x - uv.x), min(uv.y, sz.y - uv.y));
    float grain = fbm(vec2(uv.x*1.5, uv.y*14.0));
    vec3 wood = vec3(0.74, 0.50, 0.27) * (0.8 + 0.35*grain);
    float planks = gridLine(uv.y/0.4, 0.04);
    col = mix(wood, wood*0.55, planks);
    float diag = abs(uv.x/sz.x - uv.y/sz.y) * min(sz.x, sz.y);
    float frame = max(1.0 - smoothstep(0.16, 0.18, edge), 1.0 - smoothstep(0.10, 0.12, diag));
    col = mix(col, vec3(0.58, 0.38, 0.20) * (0.85 + 0.3*grain), frame);
    col *= mix(0.6, 1.0, smoothstep(0.0, 0.05, edge));
  } else { // concrete trim with hazard stripes along the top edge
    col = vec3(0.56, 0.61, 0.70) * (0.85 + 0.25*fbm(wuv*1.5));
    if (top) {
      col = mix(col, col*0.75, max(gridLine(wuv.x, 0.015), gridLine(wuv.y, 0.015)));
    } else if (sz.y - uv.y < 0.35) {
      float s = step(0.5, fract((uv.x + uv.y) * 1.6));
      col = mix(vec3(0.08), vec3(1.0, 0.82, 0.1), s);
    }
  }
  vec3 c = fog(light(col, n, ao), vWorld);
  outCol = vec4(c, 1.0);
}`;

const SOLID_VS = `#version 300 es
layout(location=0) in vec3 aPos; layout(location=1) in vec3 aNrm;
uniform mat4 uVP; uniform mat4 uModel;
out vec3 vWorld; out vec3 vNrm;
void main(){ vec4 w = uModel * vec4(aPos, 1.0); vWorld = w.xyz; vNrm = mat3(uModel) * aNrm; gl_Position = uVP * w; }`;

const SOLID_FS = `#version 300 es
precision highp float;
in vec3 vWorld; in vec3 vNrm;
uniform vec3 uColor; uniform float uEmit;
out vec4 outCol;
${COMMON}
void main(){
  vec3 n = normalize(vNrm);
  vec3 v = normalize(uEye - vWorld);
  float rim = pow(1.0 - max(dot(n, v), 0.0), 3.0);
  vec3 c = light(uColor, n, 1.0) + rim*0.35*uColor;
  c = mix(c, uColor*1.4, uEmit);
  outCol = vec4(fog(c, vWorld), 1.0);
}`;

const SKY_VS = `#version 300 es
layout(location=0) in vec2 aPos;
out vec2 vNdc;
void main(){ vNdc = aPos; gl_Position = vec4(aPos, 0.0, 1.0); }`;

const SKY_FS = `#version 300 es
precision highp float;
in vec2 vNdc;
uniform vec3 uFwd, uRight, uUp; uniform vec2 uTan;
out vec4 outCol;
${COMMON}
void main(){
  vec3 d = normalize(uFwd + vNdc.x*uTan.x*uRight + vNdc.y*uTan.y*uUp);
  vec3 zen = vec3(0.30, 0.52, 0.92);
  vec3 col = mix(uFog, zen, smoothstep(-0.02, 0.55, d.y));
  float s = max(dot(d, uSun), 0.0);
  col += vec3(1.0, 0.9, 0.6)*smoothstep(0.9985, 0.9992, s)*1.5 + vec3(1.0, 0.6, 0.3)*pow(s, 8.0)*0.35;
  if (d.y > 0.0) {
    vec2 p = d.xz / (d.y + 0.08) * 0.9 + vec2(uTime*0.015, 0.0);
    float c = smoothstep(0.5, 0.78, fbm(p));
    col = mix(col, vec3(1.0, 0.97, 0.94), c * smoothstep(0.0, 0.25, d.y) * 0.85);
  }
  outCol = vec4(col, 1.0);
}`;

export function createRenderer(canvas) {
  const gl = canvas.getContext("webgl2", { antialias: true });
  if (!gl) throw new Error("WebGL2 not supported");

  const mapProg = program(gl, MAP_VS, MAP_FS);
  const solidProg = program(gl, SOLID_VS, SOLID_FS);
  const skyProg = program(gl, SKY_VS, SKY_FS);

  const skyVao = gl.createVertexArray();
  gl.bindVertexArray(skyVao);
  attrib(gl, 0, 2, new Float32Array([-1, -1, 3, -1, -1, 3]));

  const cube = boxMesh([{ min: [-0.5, -0.5, -0.5], max: [0.5, 0.5, 0.5], kind: "trim" }], true);
  const cubeVao = gl.createVertexArray();
  gl.bindVertexArray(cubeVao);
  attrib(gl, 0, 3, cube.pos);
  attrib(gl, 1, 3, cube.nrm);
  indexBuf(gl, cube.idx);

  let mapVao = null, mapCount = 0;
  let vp = null, fov = 1.3, aspect = 1;

  function setMap(map) {
    const m = boxMesh(map.boxes, false);
    if (mapVao) gl.deleteVertexArray(mapVao);
    mapVao = gl.createVertexArray();
    gl.bindVertexArray(mapVao);
    attrib(gl, 0, 3, m.pos);
    attrib(gl, 1, 3, m.nrm);
    attrib(gl, 2, 4, m.face);
    attrib(gl, 3, 1, m.kind);
    indexBuf(gl, m.idx);
    mapCount = m.idx.length;
  }

  function resize() {
    const dpr = Math.min(window.devicePixelRatio || 1, 1.5);
    const w = Math.floor(canvas.clientWidth * dpr), h = Math.floor(canvas.clientHeight * dpr);
    if (canvas.width !== w || canvas.height !== h) { canvas.width = w; canvas.height = h; }
    aspect = w / Math.max(h, 1);
  }

  function common(p, eye, time) {
    gl.useProgram(p);
    u3(gl, p, "uEye", eye);
    u3(gl, p, "uSun", SUN);
    u3(gl, p, "uFog", FOG);
    gl.uniform1f(loc(gl, p, "uTime"), time);
  }

  function drawCube(model, color, emit = 0) {
    gl.uniformMatrix4fv(loc(gl, solidProg, "uModel"), false, model);
    u3(gl, solidProg, "uColor", color);
    gl.uniform1f(loc(gl, solidProg, "uEmit"), emit);
    gl.drawElements(gl.TRIANGLES, cube.idx.length, gl.UNSIGNED_INT, 0);
  }

  // f: {eye, yaw, pitch, time, players, puffs, viewmodel}
  function frame(f) {
    resize();
    gl.viewport(0, 0, canvas.width, canvas.height);
    gl.clearColor(...FOG, 1);
    gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
    const proj = perspective(fov, aspect, 0.05, 400);
    vp = mul(proj, fpsView(f.eye, f.yaw, f.pitch));

    // sky
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
    for (const p of f.players || []) drawPlayer(p, f.time);
    for (const p of f.puffs || []) drawCube(M(translate(...p.p), scale(p.size, p.size, p.size)), p.color, 1);

    if (f.viewmodel) {
      gl.clear(gl.DEPTH_BUFFER_BIT);
      drawViewmodel(f.eye, f.yaw, f.pitch, f.viewmodel);
    }
  }

  function drawPlayer(p, time) {
    const c = p.rgb;
    const waddle = p.moving ? Math.sin(p.phase) * 0.12 : 0;
    const base = M(translate(...p.p), rotY(p.yaw), rotZ(waddle));
    drawCube(M(base, translate(0, 0.7, 0), scale(0.72, 1.3, 0.46)), c);
    drawCube(M(base, translate(-0.2, 0.08, 0), scale(0.26, 0.16, 0.5)), scaleRgb(c, 0.5));
    drawCube(M(base, translate(0.2, 0.08, 0), scale(0.26, 0.16, 0.5)), scaleRgb(c, 0.5));
    const head = M(base, translate(0, 1.58, 0), rotX(p.pitch * 0.6));
    drawCube(M(head, scale(0.52, 0.48, 0.52)), c);
    drawCube(M(head, translate(0, 0.03, -0.25), scale(0.44, 0.16, 0.06)), [0.2, 1, 0.95], 0.9);
    const gun = M(base, translate(0.32, 1.12, -0.3), rotX(p.pitch));
    drawCube(M(gun, translate(0, 0, -0.1), scale(0.1, 0.13, 0.5)), [0.15, 0.75, 0.7]);
    drawCube(M(gun, translate(0, 0, -0.38), scale(0.07, 0.07, 0.08)), [1, 0.45, 0.1], 0.6);
    if (p.flash > 0) drawCube(M(gun, translate(0, 0, -0.5), scale(0.22, 0.22, 0.22)), [1, 0.9, 0.3], 1);
  }

  function drawViewmodel(eye, yaw, pitch, v) {
    const cam = M(translate(...eye), rotY(yaw), rotX(pitch));
    const dip = Math.sin(Math.min(v.reload, 1) * Math.PI); // reload dip 0..1..0
    const g = M(cam,
      translate(0.24 + v.bobX, -0.22 + v.bobY - dip * 0.25, -0.5 + v.kick * 0.08),
      rotX(v.kick * 0.25 - dip * 0.9), rotZ(dip * 0.6), rotY(0.04), scale(0.8, 0.8, 0.8));
    drawCube(M(g, scale(0.09, 0.11, 0.38)), [0.15, 0.75, 0.7]);
    drawCube(M(g, translate(0, 0.075, -0.05), scale(0.03, 0.04, 0.2)), [0.95, 0.3, 0.6]);
    drawCube(M(g, translate(0, -0.12 - dip * 0.15, 0.04), scale(0.055, 0.15, 0.07)), [0.95, 0.85, 0.2]);
    drawCube(M(g, translate(0, -0.1, 0.16), scale(0.07, 0.16, 0.08)), [0.12, 0.55, 0.5]);
    drawCube(M(g, translate(0, 0.01, -0.3), scale(0.04, 0.04, 0.22)), [0.2, 0.2, 0.25]);
    drawCube(M(g, translate(0, 0.01, -0.43), scale(0.05, 0.05, 0.05)), [1, 0.45, 0.1], 0.6);
    if (v.flash > 0) drawCube(M(g, translate(0, 0.01, -0.52), rotZ(v.flash * 7), scale(0.12, 0.12, 0.05)), [1, 0.95, 0.4], 1);
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

  return { setMap, frame, project };
}

// ---- mesh building ----

const FACES = [
  { n: [1, 0, 0], ax: 0, hi: 1, u: 2, v: 1 }, { n: [-1, 0, 0], ax: 0, hi: 0, u: 2, v: 1 },
  { n: [0, 1, 0], ax: 1, hi: 1, u: 0, v: 2 }, { n: [0, -1, 0], ax: 1, hi: 0, u: 0, v: 2 },
  { n: [0, 0, 1], ax: 2, hi: 1, u: 0, v: 1 }, { n: [0, 0, -1], ax: 2, hi: 0, u: 0, v: 1 },
];

function boxMesh(boxes, allFaces) {
  const pos = [], nrm = [], face = [], kind = [], idx = [];
  for (const b of boxes) {
    const k = KIND[b.kind] ?? 3;
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
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(s));
    gl.attachShader(p, s);
  }
  gl.linkProgram(p);
  if (!gl.getProgramParameter(p, gl.LINK_STATUS)) throw new Error(gl.getProgramInfoLog(p));
  p.locs = {};
  return p;
}

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

function M(...ms) { return ms.reduce((a, b) => mul(a, b)); }

function rotZ(a) {
  const c = Math.cos(a), s = Math.sin(a);
  return new Float32Array([c, s, 0, 0, -s, c, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]);
}

function norm(v) { const l = Math.hypot(...v); return v.map((x) => x / l); }
function scaleRgb(c, k) { return c.map((x) => x * k); }

export function hexRgb(hex) {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16 & 255) / 255, (n >> 8 & 255) / 255, (n & 255) / 255];
}
