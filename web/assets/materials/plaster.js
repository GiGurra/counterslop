// Painted office walls: faint roller texture, a dark skirting board at the
// floor, a chair rail at 1 m; tops are plain.
export default {
  name: "plaster",
  glsl: `vec3 mat_plaster(Surf s){
    vec3 col = PAL_PLASTER * (0.94 + 0.08*fbm(s.wuv*4.0));
    if (s.top) return col*0.92;
    if (s.uv.y < 0.12) return PAL_SKIRTING * (0.9 + 0.1*fbm(s.wuv*10.0));
    return mix(col, col*0.82, 1.0 - smoothstep(0.015, 0.025, abs(s.uv.y - 1.0)));
  }`,
};
