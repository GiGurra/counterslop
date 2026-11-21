// Laminate furniture (desks, counters, shelves, whiteboards): a pale wood-look
// top with a grey edge band; sides are the grey carcass.
export default {
  name: "desk",
  glsl: `vec3 mat_desk(Surf s){
    float edge = edgeDist(s.uv, s.sz);
    if (s.top) {
      vec3 wood = PAL_LAMINATE * (0.88 + 0.18*fbm(vec2(s.uv.x*0.8, s.uv.y*12.0)));
      return mix(PAL_LAMINATE_EDGE, wood, smoothstep(0.03, 0.04, edge));
    }
    vec3 col = PAL_LAMINATE_EDGE * (1.1 + 0.1*fbm(s.uv*6.0));
    if (s.sz.y > 1.5) col = mix(vec3(0.96), col, 1.0 - smoothstep(0.06, 0.08, edge)); // whiteboard face
    return col;
  }`,
};
