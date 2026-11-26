// Moon dust (Star Slop, lab_moon): grey fbm regolith pocked with small craters
// (one per jittered 3 m cell: a dark bowl, a pale rim), a few boot prints of slop.
export default {
  name: "regolith",
  glsl: `vec3 mat_regolith(Surf s){
    float g = fbm(s.wuv*0.9);
    vec3 col = PAL_REGOLITH * (0.78 + 0.4*g);
    vec2 cell = floor(s.wuv/3.0);
    vec2 c = (cell + 0.25 + 0.5*vec2(hash(cell), hash(cell + 17.0)))*3.0;
    float r = 0.35 + 0.9*hash(cell + 5.0);
    float d = length(s.wuv - c)/r;
    float bowl = 1.0 - smoothstep(0.55, 0.95, d);
    float rim = smoothstep(0.75, 0.95, d) * (1.0 - smoothstep(0.95, 1.2, d));
    col *= 1.0 - 0.32*bowl;
    col += 0.12*rim;
    float slop = smoothstep(0.78, 0.8, fbm(s.wuv*0.2 + 9.0));
    return mix(col, PAL_SLOP*0.8, slop*0.6);
  }`,
};
