// Running-bond brick walls with a concrete capstone on top faces.
export default {
  name: "brick",
  glsl: `vec3 mat_brick(Surf s){
    if (s.top) return PAL_CAPSTONE * (0.85 + 0.25*fbm(s.wuv*2.0));
    vec2 b = bricks(s.wuv, vec2(1.0, 0.5), 0.03, 0.07);
    vec3 brick = mix(PAL_BRICK_DARK, PAL_BRICK_LIGHT, b.y) * (0.88 + 0.2*fbm(s.wuv*5.0));
    return mix(brick, PAL_MORTAR, b.x);
  }`,
};
