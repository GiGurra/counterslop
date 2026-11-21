// Store signage: a dark panel with a neon word in seven-segment tubes (WORDS), one
// colour per sign, a soft glow around the tubes and a rare flicker. The word and colour
// come from where the sign stands. Back and edges: plain panel.

// Seven-segment shapes (bits a1 b2 c4 d8 e16 f32 g64) of the letters that read as
// themselves on a sign. Not D (the usual "d" reads as a backwards E, a full one as O) or T (a "t" reads as E).
export const SEGMENTS = {
  A: 119, C: 57, E: 121, G: 61, H: 118, L: 56, N: 55, O: 63, P: 115, S: 109, U: 62, Y: 110,
  0: 63, 9: 111,
};
// The words a sign can show (at most 4 cells), picked per sign by its hash.
export const WORDS = ["SALE", "GPU", "OPEN", "9090", "SHOP"];

const glyphFn = () => {
  const word = (w) => [...w].map((ch, i) => `i == ${i} ? ${SEGMENTS[ch]} : `).join("") + "0";
  return `int neonGlyph(int w, int i){
    ${WORDS.map((w, k) => (k < WORDS.length - 1 ? `if (w == ${k}) return ${word(w)};` : `return ${word(w)};`)).join("\n    ")}
  }
  int neonLen(int w){ ${WORDS.map((w, k) => `if (w == ${k}) return ${w.length};`).join(" ")} return 0; }`;
};

export default {
  name: "neon",
  glsl: `${glyphFn()}
  vec3 mat_neon(Surf s){
    vec3 col = PAL_SIGN_BACK * (0.9 + 0.2*fbm(s.wuv*5.0));
    if (s.top || min(s.sz.x, s.sz.y) < 0.15) return col;
    float h = faceHash(s.world, s.n, s.uv, 5.0);
    int w = int(floor(h*${WORDS.length}.0)), n = neonLen(w);
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
