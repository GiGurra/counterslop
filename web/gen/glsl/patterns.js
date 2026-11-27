// Reusable 2D patterns for materials. All take surface uv in world units.
//
// Antialiasing inside a face: MSAA (r_aa msaa*) smooths only where a triangle's coverage
// ends; the fragment shader still runs once per pixel, so a hard step() a material draws
// inside a face (a neon trim, a stripe, a seam, a glyph) stays as jagged as with AA off.
// aaStep/aaBand/aaLine are step() and its bands box-filtered over the pixel's footprint
// (fwidth of the coordinate): the coverage of the pixel, as MSAA would have given it.
// Up close they are a one-pixel ramp; a band thinner than a pixel fades to its share of
// the pixel instead of breaking up. Call them in uniform control flow per face (a branch
// on s.top or the material is fine: a 2x2 pixel quad is one triangle), not under a branch
// on a per-pixel value (derivatives there are undefined).
// aaStepRef/aaBandRef/aaPulseRef below are the same maths in JS for the unit tests.
export const PATTERNS = `
// step(e, x), antialiased: the share of the pixel where x >= e.
float aaStep(float e, float x){ float fw = max(fwidth(x), 1e-6); return clamp((x - e) / fw + 0.5, 0.0, 1.0); }
vec2 aaStep(vec2 e, vec2 x){ return vec2(aaStep(e.x, x.x), aaStep(e.y, x.y)); }

// 1 where lo <= x <= hi, antialiased: the share of the pixel's footprint inside the band.
float aaBand(float x, float lo, float hi){ float fw = max(fwidth(x), 1e-6); return clamp((min(x + 0.5*fw, hi) - max(x - 0.5*fw, lo)) / fw, 0.0, 1.0); }

// 1 where abs(x) <= w (a line of half width w centred on x = 0), antialiased.
float aaLine(float x, float w){ return aaBand(x, -w, w); }

// A repeating band: 1 where a <= fract(t) <= b (0 <= a < b <= 1), antialiased. Use it for
// stripes, ribs, seams and scrolling arrows instead of step(.., fract(t)): it filters t
// itself (no blur at fract's wrap) and a band finer than a pixel averages out (no moire).
float aaPulseInt(float t, float a, float b){ return floor(t) * (b - a) + clamp(fract(t), a, b) - a; }
float aaPulse(float t, float a, float b){ float fw = max(fwidth(t), 1e-6); return clamp((aaPulseInt(t + 0.5*fw, a, b) - aaPulseInt(t - 0.5*fw, a, b)) / fw, 0.0, 1.0); }

// 1 on grid lines (integer crossings of x), antialiased; w is the half width in x units.
float gridLine(float x, float w){ float d = abs(fract(x - 0.5) - 0.5); float fw = fwidth(x); return 1.0 - smoothstep(w - fw, w + fw, d); }

// Grid lines along both axes every 'size' units.
float tiles(vec2 uv, float size, float w){ return max(gridLine(uv.x/size, w), gridLine(uv.y/size, w)); }

// Running-bond bricks. Returns (mortar mask, per-brick random 0..1).
vec2 bricks(vec2 uv, vec2 brick, float mortarU, float mortarV){
  float row = floor(uv.y / brick.y);
  float u = uv.x / brick.x + mod(row, 2.0)*0.5;
  float mortar = max(gridLine(u, mortarU), gridLine(uv.y/brick.y, mortarV));
  return vec2(mortar, hash(vec2(floor(u), row)));
}

// Distance to the nearest edge of a face (uv in 0..sz).
float edgeDist(vec2 uv, vec2 sz){ return min(min(uv.x, sz.x - uv.x), min(uv.y, sz.y - uv.y)); }

// The world position of a face's uv origin (the box corner the face uv counts from), so a
// material can tell faces (and boxes) apart: hash(faceOrigin(...).xz) seeds per-box looks.
// Face uv axes (engine/gl.js boxMesh): x faces u=z v=y, y faces u=x v=z, z faces u=x v=y.
vec3 faceOrigin(vec3 world, vec3 n, vec2 uv){
  vec3 ax = abs(n);
  if (ax.x > 0.5) return world - vec3(0.0, uv.y, uv.x);
  if (ax.y > 0.5) return world - vec3(uv.x, 0.0, uv.y);
  return world - vec3(uv.x, uv.y, 0.0);
}

// Face uv as read by someone looking at the face: on +x and -z faces the u axis runs
// right to left for the viewer, so text and pictures would come out mirrored.
vec2 readUV(vec3 n, vec2 uv, vec2 sz){ return (n.x > 0.5 || n.z < -0.5) ? vec2(sz.x - uv.x, uv.y) : uv; }

// A stable 0..1 per face (seed a per-box look with it). The origin is rounded to 5 cm
// first: recomputed per pixel it jitters around the box corner, and floor() of a value
// sitting on an integer would flip between neighbouring pixels.
float faceHash(vec3 world, vec3 n, vec2 uv, float salt){
  vec3 o = floor(faceOrigin(world, n, uv)*20.0 + 0.5);
  return hash(o.xz*0.37 + o.y*1.71 + salt);
}

// Seven-segment glyph mask at p (cell 0..1 x 0..2), segments as bits a=1 b=2 c=4 d=8 e=16 f=32 g=64;
// w = stroke half width. Returns the stroke coverage 0..1 (antialiased).
float seg7(vec2 p, int m, float w){
  float d = 1e3;
  if ((m & 1) != 0) d = min(d, length(vec2(max(abs(p.x - 0.5) - 0.35, 0.0), p.y - 1.9)));
  if ((m & 8) != 0) d = min(d, length(vec2(max(abs(p.x - 0.5) - 0.35, 0.0), p.y - 0.1)));
  if ((m & 64) != 0) d = min(d, length(vec2(max(abs(p.x - 0.5) - 0.35, 0.0), p.y - 1.0)));
  if ((m & 2) != 0) d = min(d, length(vec2(p.x - 0.9, max(abs(p.y - 1.45) - 0.4, 0.0))));
  if ((m & 4) != 0) d = min(d, length(vec2(p.x - 0.9, max(abs(p.y - 0.55) - 0.4, 0.0))));
  if ((m & 16) != 0) d = min(d, length(vec2(p.x - 0.1, max(abs(p.y - 0.55) - 0.4, 0.0))));
  if ((m & 32) != 0) d = min(d, length(vec2(p.x - 0.1, max(abs(p.y - 1.45) - 0.4, 0.0))));
  float fw = fwidth(d) + 1e-4;
  return 1.0 - smoothstep(w - fw, w + fw, d);
}

// Diagonal stripes, 0/1, 'freq' stripes per unit.
float stripes(vec2 uv, float freq){ return step(0.5, fract((uv.x + uv.y) * freq)); }
`;

// JS twins of aaStep/aaBand for tests: fw is fwidth(x), the pixel's footprint in x units.
export function aaStepRef(e, x, fw) { fw = Math.max(fw, 1e-6); return Math.min(1, Math.max(0, (x - e) / fw + 0.5)); }
export function aaBandRef(x, lo, hi, fw) { fw = Math.max(fw, 1e-6); return Math.min(1, Math.max(0, (Math.min(x + 0.5 * fw, hi) - Math.max(x - 0.5 * fw, lo)) / fw)); }
function pulseInt(t, a, b) { const f = t - Math.floor(t); return Math.floor(t) * (b - a) + Math.min(b, Math.max(a, f)) - a; }
export function aaPulseRef(t, a, b, fw) { fw = Math.max(fw, 1e-6); return Math.min(1, Math.max(0, (pulseInt(t + 0.5 * fw, a, b) - pulseInt(t - 0.5 * fw, a, b)) / fw)); }
