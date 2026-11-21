// Reusable 2D patterns for materials. All take surface uv in world units.
export const PATTERNS = `
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
