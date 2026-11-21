// Checkout counter: blue laminate sides with a light stripe and a glowing lane light
// (green, now and then red: closed); the top is a rubber conveyor belt down the middle,
// ribs sliding along the long axis, between stainless steel edges.
export default {
  name: "checkout",
  glsl: `vec3 mat_checkout(Surf s){
    float edge = edgeDist(s.uv, s.sz);
    if (s.top) {
      bool alongU = s.sz.x >= s.sz.y;
      float along = alongU ? s.uv.x : s.uv.y, across = alongU ? s.uv.y : s.uv.x, width = alongU ? s.sz.y : s.sz.x;
      vec3 steel = PAL_SHELF_METAL * (0.95 + 0.1*sin(along*200.0 + fbm(s.uv*2.0)*3.0));
      float belt = step(0.12, across) * step(across, width - 0.12);
      float rib = gridLine((along + uTime*0.35)/0.09, 0.08);
      vec3 b = PAL_BELT * (1.0 + 0.6*rib) * (0.9 + 0.2*fbm(vec2(along - uTime*0.35, across)*6.0));
      return mix(steel, b, belt);
    }
    vec3 col = PAL_CHECKOUT_BODY * (0.92 + 0.12*fbm(s.uv*4.0));
    col = mix(col, vec3(0.9), step(s.sz.y - 0.2, s.uv.y) * step(s.uv.y, s.sz.y - 0.14));
    col = mix(col, col*0.5, step(s.uv.y, 0.1)); // kick plate
    if (s.sz.x > 0.6) { // the lane light near one end
      float closed = step(0.85, faceHash(s.world, s.n, s.uv, 2.0));
      vec2 l = s.uv - vec2(0.25, s.sz.y - 0.35);
      float lamp = 1.0 - smoothstep(0.05, 0.065, length(l));
      vec3 lc = mix(PAL_LED_GREEN, PAL_REVIEW_RED*1.3, closed);
      col = mix(col, lc, lamp);
      gEmit = lamp;
    }
    return mix(col*0.6, col, smoothstep(0.01, 0.025, edge));
  }`,
};
