// Earth's assault banner (prefabs.CrestBanner, cq_moonshot): the facepalm
// crest, our own: a shield in hoodie purple with a neon rim, on it a head
// bowed into a palm, four fingers up the brow, the sleeve of the arm below.
// Cloth with a faint weave; thin faces (the cloth's edges) are plain.
export default {
  name: "crest",
  glsl: `float crestSeg(vec2 p, vec2 a, vec2 b){ vec2 pa = p - a, ba = b - a; return length(pa - ba*clamp(dot(pa, ba)/dot(ba, ba), 0.0, 1.0)); }
  float crestEll(vec2 p, vec2 c, vec2 r){ return length((p - c)/r) - 1.0; }
  vec3 mat_crest(Surf s){
    float weave = 0.93 + 0.07*step(0.5, fract(s.wuv.x*40.0 + step(0.5, fract(s.world.y*40.0))*0.5));
    if (s.top || min(s.sz.x, s.sz.y) < 0.3) return PAL_VIBE_DARK*weave;
    vec2 p = readUV(s.n, s.uv, s.sz) / s.sz;
    vec2 q = vec2((p.x - 0.5)*s.sz.x/s.sz.y, p.y); // across in units of the height, centred
    // The shield: straight sides from 0.3 up, a point at the bottom.
    float hw = 0.27 * (q.y > 0.3 ? 1.0 : clamp((q.y - 0.1)/0.2, 0.0, 1.0));
    float inShield = step(abs(q.x), hw) * step(q.y, 0.9) * step(0.1, q.y);
    float rim = inShield * (1.0 - step(abs(q.x), hw - 0.02) * step(q.y, 0.88) * step(0.12 + 0.02*step(q.y, 0.3), q.y));
    vec3 col = PAL_VIBE_DARK;
    if (inShield > 0.5) col = PAL_VIBE_HOODIE*1.3;
    // The figure: shoulders (a hoodie), the head bowed, its hair, a frown
    // under the palm across the eyes, the fingers over the brow, the arm up
    // from below (its sleeve).
    vec3 skin = PAL_CREST_SKIN, line = PAL_VIBE_DARK;
    if (inShield > 0.5) {
      if (crestEll(q, vec2(0.0, 0.24), vec2(0.23, 0.13)) < 0.0 && q.y > 0.12) col = PAL_VIBE_DARK*1.6; // shoulders
      float hair = crestEll(q, vec2(0.0, 0.62), vec2(0.15, 0.1));
      float head = crestEll(q, vec2(0.0, 0.5), vec2(0.13, 0.155));
      if (min(hair, head) < 0.08) col = line;
      if (hair < 0.0) col = PAL_VIBE_DARK*2.2;
      if (head < 0.0 && q.y < 0.6) col = skin;
      float frown = crestEll(q, vec2(0.0, 0.405), vec2(0.055, 0.03));
      if (abs(frown) < 0.3 && q.y > 0.405) col = line;
      float arm = crestSeg(q, vec2(0.24, 0.16), vec2(0.07, 0.52));
      if (arm < 0.058) col = line; if (arm < 0.042) col = PAL_VIBE_DARK*1.6; // the sleeve
      float palm = crestEll(q, vec2(0.0, 0.565), vec2(0.13, 0.05));
      float fingers = 1e3;
      for (int i = 0; i < 4; i++) {
        float x = -0.09 + 0.05*float(i);
        fingers = min(fingers, crestSeg(q, vec2(x, 0.58), vec2(x - 0.015, 0.665 - 0.012*abs(float(i) - 1.5))));
      }
      float thumb = crestSeg(q, vec2(0.1, 0.55), vec2(0.13, 0.48));
      float hand = min(min(palm*0.07, fingers - 0.019), thumb - 0.017);
      if (hand < 0.011) col = line; if (hand < 0.0) col = skin;
    }
    if (rim > 0.5) col = PAL_VIBE_NEON;
    return col*weave;
  }`,
};
