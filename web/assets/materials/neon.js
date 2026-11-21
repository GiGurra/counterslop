// Store signage: a dark panel with a neon word in seven-segment tubes (SALE, GPU, OPEN,
// 9090, DEAL), one colour per sign, a soft glow around the tubes and a rare flicker.
// The word and colour come from where the sign stands. Back and edges: plain panel.
export default {
  name: "neon",
  glsl: `int neonGlyph(int w, int i){
    // SALE GPU OPEN 9090 DEAL; segment bits a1 b2 c4 d8 e16 f32 g64 (0 = blank)
    if (w == 0) return i == 0 ? 109 : i == 1 ? 119 : i == 2 ? 56 : i == 3 ? 121 : 0;
    if (w == 1) return i == 0 ? 61 : i == 1 ? 115 : i == 2 ? 62 : 0;
    if (w == 2) return i == 0 ? 63 : i == 1 ? 115 : i == 2 ? 121 : i == 3 ? 55 : 0;
    if (w == 3) return i == 0 ? 111 : i == 1 ? 63 : i == 2 ? 111 : i == 3 ? 63 : 0;
    return i == 0 ? 94 : i == 1 ? 121 : i == 2 ? 119 : i == 3 ? 56 : 0;
  }
  vec3 mat_neon(Surf s){
    vec3 col = PAL_SIGN_BACK * (0.9 + 0.2*fbm(s.wuv*5.0));
    if (s.top || min(s.sz.x, s.sz.y) < 0.15) return col;
    float h = faceHash(s.world, s.n, s.uv, 5.0);
    int w = int(floor(h*5.0)), n = w == 1 ? 3 : 4;
    vec3 tube = h < 0.3 ? PAL_NEON_MAGENTA : h < 0.55 ? PAL_NEON_CYAN : h < 0.8 ? PAL_NEON_LIME : PAL_TOY_YELLOW;
    // glyph cells: height 70% of the face, width 1.3 cells per glyph, centred
    float ch = s.sz.y*0.7, cw = ch*0.5*1.3;
    float x0 = (s.sz.x - cw*float(n))*0.5;
    vec2 r = readUV(s.n, s.uv, s.sz);
    vec2 p = vec2((r.x - x0)/(ch*0.5), (r.y - s.sz.y*0.15)/(ch*0.5));
    // (seg7 uses derivatives: call it for every pixel, mask outside the word after)
    float cell = floor(p.x/1.3), inWord = step(0.0, cell) * step(cell, float(n) - 1.0);
    int m = neonGlyph(w, int(clamp(cell, 0.0, 3.0)));
    vec2 q = vec2(p.x - cell*1.3, p.y);
    float on = seg7(q, m, 0.07) * inWord, glow = seg7(q, m, 0.25) * 0.35 * inWord;
    // the odd flicker: a sign drops out for a few frames every ~10 s
    float fl = step(0.97, fract(uTime*0.1 + h*7.0)) * step(0.5, fract(uTime*13.0));
    float lit = (1.0 - fl);
    gEmit = clamp(on + glow, 0.0, 1.0) * lit;
    return mix(col + tube*glow*lit, tube*1.2 + 0.25, on*lit);
  }`,
};
