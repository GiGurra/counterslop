// Instanced batches: surface decals (one quad per record, the look in GLSL)
// and small rigid bits (casings: one prism per record). One draw call each,
// however many there are. Game-agnostic: the caller hands in rings of float
// records ({data, count, version}, layouts below) and the kinds' look.
import { prelude } from "../gen/glsl/index.js";

// Decal record (8 floats): pos.xyz, seed (0..1), normal.xyz, size (radius, m).
const DECAL_VS = `#version 300 es
layout(location=0) in vec2 aCorner;
layout(location=1) in vec4 aA; layout(location=2) in vec4 aB;
uniform mat4 uVP;
out vec3 vWorld; out vec3 vNrm; out vec2 vUv; flat out float vSeed;
void main(){
  vec3 n = normalize(aB.xyz);
  vec3 t = abs(n.y) > 0.5 ? vec3(1.0, 0.0, 0.0) : normalize(cross(vec3(0.0, 1.0, 0.0), n));
  vec3 b = cross(n, t);
  float a = aA.w * 6.2831853, c = cos(a), s = sin(a); // each hole turned its own way
  vec2 q = vec2(c*aCorner.x - s*aCorner.y, s*aCorner.x + c*aCorner.y);
  vec3 w = aA.xyz + n * 0.002 + (t*q.x + b*q.y) * aB.w;
  vWorld = w; vNrm = n; vUv = aCorner; vSeed = aA.w;
  gl_Position = uVP * vec4(w, 1.0);
}`;

// A bullet hole: a near-black ragged pit, a scorched ring and chipped,
// lighter dust around it (alpha blended, lit like the surface it sits on).
function decalFS(pal, defs) {
  return `${prelude(pal, defs)}
in vec3 vWorld; in vec3 vNrm; in vec2 vUv; flat in float vSeed;
out vec4 outCol;
void main(){
  float r = length(vUv);
  float ang = atan(vUv.y, vUv.x);
  vec2 sd = vec2(vSeed * 91.7, vSeed * 37.3);
  // Ragged edges: the radius wobbles with angle (seeded), plus fine grain.
  float wob = noise(vec2(ang * 1.6, 0.0) + sd) - 0.5;
  float grain = noise(vUv * 9.0 + sd) - 0.5;
  float rr = r * (1.0 + wob * 0.45 + grain * 0.12);
  float pit = 1.0 - smoothstep(0.13, 0.2, rr);
  float burn = 1.0 - smoothstep(0.2, 0.36, rr);
  // Chipped surface: a pale ring of broken material, with spokes of dust out to the edge.
  float spokes = smoothstep(0.5, 0.9, noise(vec2(ang * 3.0, 3.0) + sd * 1.7));
  float ring = smoothstep(0.22, 0.3, rr) * (1.0 - smoothstep(0.42, 0.55 + 0.35 * spokes, rr));
  float dust = ring * (0.55 + 0.45 * grain + 0.3 * spokes);
  if (pit + burn + dust < 0.01) discard;
  vec3 col = mix(vec3(0.8, 0.76, 0.68), vec3(0.1, 0.09, 0.08), burn);
  col = mix(col, vec3(0.015, 0.013, 0.012), pit);
  float alpha = clamp(max(max(pit, burn * 0.8), dust * 0.6), 0.0, 1.0);
  vec3 n = normalize(vNrm);
  float ao = n.y > 0.5 ? 1.0 : mix(0.55, 1.0, smoothstep(0.0, 1.6, vWorld.y)); // as the map's faces
  vec3 lit = light(col, vWorld, n, ao * (1.0 - 0.6 * pit));
  outCol = vec4(fog(lit, vWorld), alpha);
}`;
}

// Bit record (8 floats): pos.xyz, kind, yaw, tilt, roll, unused. A kind is a
// prism along x: {len, rad, color, base (share of the length in brass)}.
const MAX_KINDS = 8;
const BIT_VS = `#version 300 es
layout(location=0) in vec3 aPos; layout(location=1) in vec3 aNrm;
layout(location=2) in vec4 aA; layout(location=3) in vec4 aB;
uniform mat4 uVP; uniform vec2 uKDim[${MAX_KINDS}];
out vec3 vWorld; out vec3 vNrm; out float vX; flat out int vKind;
mat3 rx(float a){ float c = cos(a), s = sin(a); return mat3(1.0,0.0,0.0, 0.0,c,s, 0.0,-s,c); }
mat3 ry(float a){ float c = cos(a), s = sin(a); return mat3(c,0.0,-s, 0.0,1.0,0.0, s,0.0,c); }
mat3 rz(float a){ float c = cos(a), s = sin(a); return mat3(c,s,0.0, -s,c,0.0, 0.0,0.0,1.0); }
void main(){
  int k = int(aA.w + 0.5);
  vec2 dim = uKDim[k];
  mat3 R = ry(aB.x) * rz(aB.y) * rx(aB.z);
  vec3 w = aA.xyz + R * (aPos * vec3(dim.x, dim.y, dim.y));
  vWorld = w; vNrm = R * aNrm; vX = aPos.x + 0.5; vKind = k;
  gl_Position = uVP * vec4(w, 1.0);
}`;
function bitFS(pal, defs) {
  return `${prelude(pal, defs)}
in vec3 vWorld; in vec3 vNrm; in float vX; flat in int vKind;
uniform vec3 uKCol[${MAX_KINDS}]; uniform float uKBase[${MAX_KINDS}];
out vec4 outCol;
void main(){
  vec3 n = normalize(vNrm), v = normalize(uEye - vWorld);
  vec3 brass = vec3(0.86, 0.66, 0.3);
  vec3 col = vX > 1.0 - uKBase[vKind] ? (uKBase[vKind] >= 1.0 ? uKCol[vKind] : brass) : uKCol[vKind];
  float rim = pow(1.0 - max(dot(n, v), 0.0), 3.0);
  vec3 c = light(col, vWorld, n, 1.0) + rim * 0.3 * col;
  outCol = vec4(fog(c, vWorld), 1.0);
}`;
}

// An octagonal prism along x: length 1 (x in -0.5..0.5), radius 1.
function prism(sides = 8) {
  const pos = [], nrm = [];
  const tri = (a, b, c, n) => { for (const p of [a, b, c]) { pos.push(...p); nrm.push(...(n || [0, p[1], p[2]])); } };
  for (let i = 0; i < sides; i++) {
    const a0 = (i / sides) * Math.PI * 2, a1 = ((i + 1) / sides) * Math.PI * 2;
    const y0 = Math.cos(a0), z0 = Math.sin(a0), y1 = Math.cos(a1), z1 = Math.sin(a1);
    tri([-0.5, y0, z0], [0.5, y0, z0], [0.5, y1, z1]);
    tri([-0.5, y0, z0], [0.5, y1, z1], [-0.5, y1, z1]);
    tri([0.5, 0, 0], [0.5, y0, z0], [0.5, y1, z1], [1, 0, 0]);
    tri([-0.5, 0, 0], [-0.5, y1, z1], [-0.5, y0, z0], [-1, 0, 0]);
  }
  return { pos: new Float32Array(pos), nrm: new Float32Array(nrm), n: pos.length / 3 };
}

// gl, pal; helpers {program, loc} from the renderer. Programs per lighting
// variant (the renderer's defs), compiled on first use.
export function createInstanced(gl, pal, { program, loc, samplers }) {
  const progs = {};
  function progsFor(defs) {
    const key = defs.join(",");
    if (!progs[key]) {
      progs[key] = { decal: program(gl, DECAL_VS, decalFS(pal, defs)), bit: program(gl, BIT_VS, bitFS(pal, defs)) };
      samplers(progs[key].decal); samplers(progs[key].bit);
    }
    return progs[key];
  }

  const buf = (data, usage = gl.STATIC_DRAW) => { const b = gl.createBuffer(); gl.bindBuffer(gl.ARRAY_BUFFER, b); gl.bufferData(gl.ARRAY_BUFFER, data, usage); return b; };
  const inst = (b, i, size, stride, off) => {
    gl.bindBuffer(gl.ARRAY_BUFFER, b);
    gl.enableVertexAttribArray(i);
    gl.vertexAttribPointer(i, size, gl.FLOAT, false, stride * 4, off * 4);
    gl.vertexAttribDivisor(i, 1);
  };
  function batch(setup) {
    const vao = gl.createVertexArray(), ib = gl.createBuffer();
    gl.bindVertexArray(vao);
    setup(ib);
    gl.bindVertexArray(null);
    return { vao, ib, version: -1, cap: 0 };
  }
  const decals = batch((ib) => {
    buf(new Float32Array([-1, -1, 1, -1, 1, 1, -1, -1, 1, 1, -1, 1]));
    gl.enableVertexAttribArray(0);
    gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
    gl.bindBuffer(gl.ARRAY_BUFFER, ib);
    inst(ib, 1, 4, 8, 0); inst(ib, 2, 4, 8, 4);
  });
  const mesh = prism();
  const bits = batch((ib) => {
    buf(mesh.pos); gl.enableVertexAttribArray(0); gl.vertexAttribPointer(0, 3, gl.FLOAT, false, 0, 0);
    buf(mesh.nrm); gl.enableVertexAttribArray(1); gl.vertexAttribPointer(1, 3, gl.FLOAT, false, 0, 0);
    gl.bindBuffer(gl.ARRAY_BUFFER, ib);
    inst(ib, 2, 4, 8, 0); inst(ib, 3, 4, 8, 4);
  });
  // Upload a ring's records when they changed (the whole valid range: slots 0..count).
  function upload(b, ring) {
    if (b.version === ring.version) return;
    gl.bindBuffer(gl.ARRAY_BUFFER, b.ib);
    if (b.cap !== ring.data.length) { gl.bufferData(gl.ARRAY_BUFFER, ring.data.byteLength, gl.DYNAMIC_DRAW); b.cap = ring.data.length; }
    if (ring.count) gl.bufferSubData(gl.ARRAY_BUFFER, 0, ring.data, 0, ring.count * ring.stride);
    b.version = ring.version;
  }

  const stats = { decals: 0, bits: 0, draws: 0 };
  // m: {decals?: ring, bits?: ring, kinds?: [{len, rad, color, base}]}; common(p) sets the
  // shared lighting uniforms on a program (the renderer's), vp the view-projection.
  function draw(m, defs, vp, common) {
    stats.decals = stats.bits = stats.draws = 0;
    if (!m) return;
    const p = progsFor(defs);
    if (m.bits?.count && m.kinds) {
      upload(bits, m.bits);
      common(p.bit);
      gl.uniformMatrix4fv(loc(gl, p.bit, "uVP"), false, vp);
      const dims = new Float32Array(MAX_KINDS * 2), cols = new Float32Array(MAX_KINDS * 3), base = new Float32Array(MAX_KINDS);
      m.kinds.slice(0, MAX_KINDS).forEach((k, i) => { dims.set([k.len, k.rad], i * 2); cols.set(k.color, i * 3); base[i] = k.base ?? 1; });
      gl.uniform2fv(loc(gl, p.bit, "uKDim[0]"), dims);
      gl.uniform3fv(loc(gl, p.bit, "uKCol[0]"), cols);
      gl.uniform1fv(loc(gl, p.bit, "uKBase[0]"), base);
      gl.bindVertexArray(bits.vao);
      gl.drawArraysInstanced(gl.TRIANGLES, 0, mesh.n, m.bits.count);
      stats.bits = m.bits.count; stats.draws++;
    }
    if (m.decals?.count) {
      upload(decals, m.decals);
      common(p.decal);
      gl.uniformMatrix4fv(loc(gl, p.decal, "uVP"), false, vp);
      gl.enable(gl.BLEND);
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
      gl.depthMask(false);
      gl.enable(gl.POLYGON_OFFSET_FILL);
      gl.polygonOffset(-1, -4); // on the face, never z-fighting it
      gl.bindVertexArray(decals.vao);
      gl.drawArraysInstanced(gl.TRIANGLES, 0, 6, m.decals.count);
      gl.disable(gl.POLYGON_OFFSET_FILL);
      gl.depthMask(true);
      gl.disable(gl.BLEND);
      stats.decals = m.decals.count; stats.draws++;
    }
    gl.bindVertexArray(null);
  }
  return { draw, stats: () => ({ ...stats }) };
}
