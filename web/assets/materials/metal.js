// Diamond-plate steel on top (decks, steps), brushed panels with rivets on the
// sides; darker towards the edges.
export default {
  name: "metal",
  glsl: `vec3 mat_metal(Surf s){
    float edge = edgeDist(s.uv, s.sz);
    vec3 col = PAL_STEEL * (0.85 + 0.2*fbm(s.wuv*2.0));
    if (s.top) {
      vec2 q = mat2(0.7071, -0.7071, 0.7071, 0.7071) * s.wuv / 0.3;
      vec2 g = fract(q + vec2(0.5*mod(floor(q.y), 2.0), 0.0)) - 0.5;
      float d = abs(g.x)*0.35 + abs(g.y);
      float fw = fwidth(d);
      float bump = 1.0 - smoothstep(0.12 - fw, 0.12 + fw, d);
      col *= 0.92 + 0.16*bump;
      col = mix(col, col*0.75, tiles(s.wuv, 2.0, 0.008));
    } else {
      col *= 0.92 + 0.08*sin(s.uv.x*180.0 + fbm(s.uv*3.0)*4.0);
      vec2 r = fract(s.uv/0.4) - 0.5;
      float rivet = (1.0 - smoothstep(0.035, 0.05, length(r))) * (1.0 - smoothstep(0.1, 0.12, edge));
      col = mix(col, col*1.3, rivet);
    }
    return col * mix(0.7, 1.0, smoothstep(0.0, 0.04, edge));
  }`,
};
