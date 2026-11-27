// rc_ci_pipeline's rails: red and white kerb blocks (2 m each) on the sides, a neon
// magenta tube along the top so the road's edge reads at speed and at night.
export default {
  name: "curb",
  glsl: `vec3 mat_curb(Surf s){
    if (s.top) { gEmit = 1.0; return PAL_NEON_MAGENTA * 1.1 + 0.15; }
    float k = step(0.5, fract((s.wuv.x + s.wuv.y) / 4.0));
    vec3 col = mix(PAL_REVIEW_RED, vec3(0.92), k) * (0.85 + 0.2*fbm(s.wuv*3.0));
    float tube = step(s.sz.y - s.uv.y, 0.08);
    gEmit = tube;
    return mix(col, PAL_NEON_MAGENTA * 1.1 + 0.15, tube);
  }`,
};
