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
      vec3 col = mix(metal*0.8, PAL_REVIEW_RED, step(s.sz.y - 0.45, s.uv.y) * step(s.uv.y, s.sz.y - 0.08));
      col = mix(col, vec3(0.95), step(s.sz.y - 0.33, s.uv.y) * step(s.uv.y, s.sz.y - 0.2) * step(0.15, s.uv.x) * step(s.uv.x, s.sz.x - 0.15));
      return mix(metal*0.6, col, smoothstep(0.02, 0.04, edge));
    }
    float lip = 0.0, row = floor((s.uv.y - 0.12) / 0.45), y = s.uv.y - 0.12 - row*0.45;
    vec3 col = PAL_PEGBOARD * (0.9 + 0.1*fbm(s.uv*5.0));
    vec2 holes = fract(s.uv/0.05) - 0.5;
    col *= 0.85 + 0.15*smoothstep(0.12, 0.2, length(holes));
    // products on this shelf: slots of seeded width and height
    if (row >= 0.0 && s.uv.y < s.sz.y - 0.2) {
      float slot = floor(s.uv.x/0.22), fx = fract(s.uv.x/0.22);
      float h1 = hash(vec2(slot, row) + faceHash(s.world, s.n, s.uv, 0.0)*97.0), hgt = 0.18 + 0.2*hash(vec2(row, slot*1.7));
      float inBox = step(0.06, fx) * step(fx, 0.94) * step(0.04, y) * step(y, 0.04 + hgt) * step(0.12, h1);
      vec3 p = shelfProduct(hash(vec2(slot*3.1, row + 4.0)));
      p = mix(p, vec3(0.95), step(0.04 + hgt*0.55, y) * step(y, 0.04 + hgt*0.7) * 0.8); // label band
      col = mix(col, p * (0.9 + 0.1*fbm(s.uv*20.0)), inBox);
    }
    // shelf lip with the price strip and its tags
    lip = step(y, 0.05) * step(0.0, s.uv.y - 0.12);
    vec3 strip = mix(PAL_PRICE_STRIP, PAL_TOY_YELLOW*vec3(1.0, 0.6, 0.2), step(0.8, fract(s.uv.x/0.22)) * step(y, 0.035));
    col = mix(col, strip, lip);
    // uprights every metre
    col = mix(col, metal*0.7, gridLine(s.uv.x, 0.03));
    return mix(metal*0.6, col, smoothstep(0.015, 0.03, edge));
  }`,
};
