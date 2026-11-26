// Cable looms (de_rackmount): a bundle of coloured wires running the box's
// long way, a zip tie every 1.5 m.
export default {
  name: "cable",
  glsl: `vec3 mat_cable(Surf s){
    vec2 u = s.sz.x >= s.sz.y ? s.uv : s.uv.yx; // along, across
    float k = floor(u.y / 0.12);
    float h = hash(vec2(k, floor(s.world.x + s.world.y*3.0 + s.world.z*7.0)));
    vec3 col = h < 0.35 ? PAL_CABLE_BLUE : (h < 0.6 ? PAL_CABLE_YELLOW : (h < 0.8 ? PAL_CABLE_RED : PAL_CHIP*2.0));
    float roundness = 0.6 + 0.4*sin(fract(u.y/0.12)*3.1416);
    col *= roundness;
    float tie = step(abs(fract(u.x/1.5) - 0.5), 0.03);
    return mix(col, vec3(0.92), tie);
  }`,
};
