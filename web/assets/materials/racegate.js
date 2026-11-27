// rc_ci_pipeline's gate banners: a chequered band between two neon tubes with a
// chase of lights running along them. Faces too thin for the band: just the glow.
export default {
  name: "racegate",
  glsl: `vec3 mat_racegate(Surf s){
    vec2 r = readUV(s.n, s.uv, s.sz);
    if (s.top || s.sz.y < 0.3) { gEmit = 0.8; return PAL_NEON_CYAN; }
    float y = r.y / s.sz.y;
    float tube = step(y, 0.16) + step(0.84, y);
    vec2 c = floor(vec2(r.x, r.y) / (s.sz.y * 0.17));
    vec3 col = mix(vec3(0.05), vec3(0.95), mod(c.x + c.y, 2.0));
    float chase = step(0.6, fract(r.x*0.5 - uTime*1.5));
    vec3 tc = mix(PAL_NEON_CYAN, PAL_NEON_LIME, chase);
    gEmit = tube;
    return mix(col, tc * 1.1, tube);
  }`,
};
