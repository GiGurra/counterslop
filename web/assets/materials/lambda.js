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
      // All antialiased (aaStep & co, gen/glsl/patterns.js): MSAA alone leaves lines
      // drawn inside a face jagged, the trim's bright edge most of all.
      float seam = max(aaPulse(r.x / 2.0, 0.0, 0.03), aaPulse(r.y / 2.0, 0.0, 0.03));
      float edge = 1.0 - aaStep(0.18, edgeDist(r, s.sz));
      // A lambda: two strokes, in about one panel of fourteen.
      vec2 q = p - 0.5;
      float lam = step(hash(cell + floor(s.world.y)), 0.07) * (
        aaLine(q.x - q.y * 0.45, 0.05) * aaLine(q.y, 0.32) +
        aaLine(q.x + (q.y + 0.32) * 0.9 - 0.02, 0.05) * aaBand(q.y, -0.32, 0.0));
      lam = clamp(lam, 0.0, 1.0);
      gEmit = clamp(edge + lam * 0.55, 0.0, 1.0);
      vec3 col = mix(base * (0.9 + 0.2 * hash(cell)), base * 0.6, seam);
      col = mix(col, PAL_NEON_CYAN, edge);
      return mix(col, vec3(1.0, 0.55, 0.12), lam * 0.8);
    }
    float rib = aaPulse(r.x / 0.8, 0.5, 1.0);
    float lit = aaStep(s.sz.y - 0.15, r.y);
    gEmit = lit * 0.8;
    return mix(base * (0.75 + 0.15 * rib), PAL_NEON_CYAN, lit);
  }`,
};
