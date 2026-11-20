// Sandy floor: fbm-mottled sand, 4 m tile joints, puddles of wobbling green slop.
export default {
  name: "sand",
  glsl: `vec3 mat_sand(Surf s){
    float g = fbm(s.wuv*0.7);
    vec3 col = PAL_SAND * (0.82 + 0.35*g);
    col = mix(col, col*0.7, tiles(s.wuv, 4.0, 0.012));
    float slop = smoothstep(0.66, 0.70, fbm(s.wuv*0.11 + 3.0));
    float wobble = 0.5 + 0.5*sin(uTime*2.0 + fbm(s.wuv*0.8)*6.0);
    return mix(col, PAL_SLOP + 0.12*wobble, slop);
  }`,
};
