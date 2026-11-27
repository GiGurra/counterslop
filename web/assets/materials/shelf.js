// Store gondola shelving: on the long faces, grey uprights every metre, shelf lips with
// white price strips (tiny orange tags) at five heights, and between them rows of
// seeded product boxes in loud packaging colours against a dark pegboard. Narrow end
// faces are end caps with a red SALE header; tops are bare shelf metal.
export default {
  name: "shelf",
  glsl: `vec3 shelfProduct(float k){
    float i = floor(k*7.0);
    return i < 1.0 ? PAL_NEON_CYAN*0.8 : i < 2.0 ? PAL_REVIEW_RED : i < 3.0 ? vec3(0.92) : i < 4.0 ? PAL_VIBE_DARK*2.0
      : i < 5.0 ? PAL_NEON_LIME*0.75 : i < 6.0 ? PAL_NEON_MAGENTA*0.8 : PAL_TOY_YELLOW;
  }
  vec3 mat_shelf(Surf s){
    vec3 metal = PAL_SHELF_METAL * (0.9 + 0.12*fbm(s.wuv*3.0));
    if (s.top || s.sz.y < 0.3) return metal;
    float edge = edgeDist(s.uv, s.sz);
    if (s.sz.x < 1.3) { // end cap
      vec3 col = mix(metal*0.8, PAL_REVIEW_RED, aaBand(s.uv.y, s.sz.y - 0.45, s.sz.y - 0.08));
      col = mix(col, vec3(0.95), aaBand(s.uv.y, s.sz.y - 0.33, s.sz.y - 0.2) * aaBand(s.uv.x, 0.15, s.sz.x - 0.15));
      return mix(metal*0.6, col, smoothstep(0.02, 0.04, edge));
    }
    // Lines antialiased (gen/glsl/patterns.js), on the continuous row coordinate Y
    // (y wraps per row) and with no per-pixel branch around them.
    float lip = 0.0, Y = (s.uv.y - 0.12) / 0.45, row = floor(Y);
    vec3 col = PAL_PEGBOARD * (0.9 + 0.1*fbm(s.uv*5.0));
    vec2 holes = fract(s.uv/0.05) - 0.5;
    col *= 0.85 + 0.15*smoothstep(0.12, 0.2, length(holes));
    // products on this shelf: slots of seeded width and height
    {
      float shelved = aaStep(0.12, s.uv.y) * (1.0 - aaStep(s.sz.y - 0.2, s.uv.y));
      float slot = floor(s.uv.x/0.22);
      float h1 = hash(vec2(slot, row) + faceHash(s.world, s.n, s.uv, 0.0)*97.0), hgt = 0.18 + 0.2*hash(vec2(row, slot*1.7));
      float inBox = aaPulse(s.uv.x/0.22, 0.06, 0.94) * aaPulse(Y, 0.04/0.45, (0.04 + hgt)/0.45) * step(0.12, h1) * shelved;
      vec3 p = shelfProduct(hash(vec2(slot*3.1, row + 4.0)));
      p = mix(p, vec3(0.95), aaPulse(Y, (0.04 + hgt*0.55)/0.45, (0.04 + hgt*0.7)/0.45) * 0.8); // label band
      col = mix(col, p * (0.9 + 0.1*fbm(s.uv*20.0)), inBox);
    }
    // shelf lip with the price strip and its tags
    lip = aaPulse(Y, 0.0, 0.05/0.45) * aaStep(0.12, s.uv.y);
    vec3 strip = mix(PAL_PRICE_STRIP, PAL_TOY_YELLOW*vec3(1.0, 0.6, 0.2), aaPulse(s.uv.x/0.22, 0.8, 1.0) * aaPulse(Y, 0.0, 0.035/0.45));
    col = mix(col, strip, lip);
    // uprights every metre
    col = mix(col, metal*0.7, gridLine(s.uv.x, 0.03));
    return mix(metal*0.6, col, smoothstep(0.015, 0.03, edge));
  }`,
};
