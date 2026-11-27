// rc_ci_pipeline's gate banners: a chequered band between two neon tubes with a
// chase of lights running along them. Faces too thin for the band: just the glow.
export default {
  name: "racegate",
  glsl: `vec3 mat_racegate(Surf s){
    vec2 r = readUV(s.n, s.uv, s.sz);
    if (s.top || s.sz.y < 0.3) { gEmit = 0.8; return PAL_NEON_CYAN; }
    float y = r.y / s.sz.y;
    float tube = 1.0 - aaBand(y, 0.16, 0.84); // antialiased (MSAA leaves in-face lines jagged)
    // the checker: odd cell along x xor odd cell along y, each a filtered pulse
    vec2 c = vec2(r.x, r.y) / (s.sz.y * 0.17);
    float ox = aaPulse(c.x * 0.5, 0.5, 1.0), oy = aaPulse(c.y * 0.5, 0.5, 1.0);
    vec3 col = mix(vec3(0.05), vec3(0.95), ox + oy - 2.0*ox*oy);
    float chase = aaPulse(r.x*0.5 - uTime*1.5, 0.6, 1.0);
    vec3 tc = mix(PAL_NEON_CYAN, PAL_NEON_LIME, chase);
    gEmit = tube;
    return mix(col, tc * 1.1, tube);
  }`,
};
