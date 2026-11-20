// Wooden crate: planks, a frame around each face, a diagonal brace, darkened edges.
export default {
  name: "crate",
  glsl: `vec3 mat_crate(Surf s){
    float edge = edgeDist(s.uv, s.sz);
    float grain = fbm(vec2(s.uv.x*1.5, s.uv.y*14.0));
    vec3 wood = PAL_WOOD * (0.8 + 0.35*grain);
    vec3 col = mix(wood, wood*0.55, gridLine(s.uv.y/0.4, 0.04));
    float diag = abs(s.uv.x/s.sz.x - s.uv.y/s.sz.y) * min(s.sz.x, s.sz.y);
    float frame = max(1.0 - smoothstep(0.16, 0.18, edge), 1.0 - smoothstep(0.10, 0.12, diag));
    col = mix(col, PAL_WOOD_FRAME * (0.85 + 0.3*grain), frame);
    return col * mix(0.6, 1.0, smoothstep(0.0, 0.05, edge));
  }`,
};
