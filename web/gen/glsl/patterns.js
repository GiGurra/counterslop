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

// Diagonal stripes, 0/1, 'freq' stripes per unit.
float stripes(vec2 uv, float freq){ return step(0.5, fract((uv.x + uv.y) * freq)); }
`;
