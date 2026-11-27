// de_serverless's floating platforms (functions with no server under them):
// dark slate panels in a 2 m grid, a neon edge strip round every top, a
// faint orange lambda now and then in a panel; the sides ribbed, lit at the top.
export default {
  name: "lambda",
  glsl: `vec3 mat_lambda(Surf s){
    vec2 r = readUV(s.n, s.uv, s.sz);
    vec3 base = vec3(0.13, 0.14, 0.19);
    if (s.top) {
      vec2 p = fract(r / 2.0), cell = floor(r / 2.0);
      float seam = 1.0 - step(0.03, p.x) * step(0.03, p.y);
      float d = min(min(r.x, s.sz.x - r.x), min(r.y, s.sz.y - r.y));
      float edge = step(d, 0.18);
      // A lambda: two strokes, in about one panel of fourteen.
      vec2 q = p - 0.5;
      float lam = step(hash(cell + floor(s.world.y)), 0.07) * (
        step(abs(q.x - q.y * 0.45), 0.05) * step(abs(q.y), 0.32) +
        step(abs(q.x + (q.y + 0.32) * 0.9 - 0.02), 0.05) * step(q.y, 0.0) * step(-0.32, q.y));
      lam = clamp(lam, 0.0, 1.0);
      gEmit = clamp(edge + lam * 0.55, 0.0, 1.0);
      vec3 col = mix(base * (0.9 + 0.2 * hash(cell)), base * 0.6, seam);
      col = mix(col, PAL_NEON_CYAN, edge);
      return mix(col, vec3(1.0, 0.55, 0.12), lam * 0.8);
    }
    float rib = step(0.5, fract(r.x / 0.8));
    float lit = step(s.sz.y - 0.15, r.y);
    gEmit = lit * 0.8;
    return mix(base * (0.75 + 0.15 * rib), PAL_NEON_CYAN, lit);
  }`,
};
