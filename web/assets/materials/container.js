// Corrugated steel shipping container: vertical ribs on the sides, locking
// bars on the doors (the short ends), a ribbed roof. The colour comes from the
// box's footprint (mapgen/prefabs Container encodes it): width 2.40/2.45/2.50/
// 2.55 m, length 6.0/6.1/6.2/6.3 m (or 12.0/12.2/... for a 40 ft one), so every
// face of one container agrees without a per-box attribute.
export default {
  name: "container",
  glsl: `vec3 mat_container(Surf s){
    float w = s.top ? min(s.sz.x, s.sz.y) : s.sz.x;
    float idx = w < 4.0 ? (w - 2.4)/0.05 : w < 9.0 ? (w - 6.0)/0.1 : (w - 12.0)/0.2;
    int i = int(mod(floor(idx + 0.5), 4.0));
    vec3 base = i == 0 ? PAL_CONTAINER_RED : i == 1 ? PAL_CONTAINER_BLUE : i == 2 ? PAL_CONTAINER_GREEN : PAL_CONTAINER_YELLOW;
    float edge = edgeDist(s.uv, s.sz);
    float grime = fbm(s.wuv*0.8 + vec2(float(i)*7.0));
    vec3 col = base * (0.82 + 0.25*grime);
    if (s.top) {
      col *= 0.9 + 0.1*sin(s.uv.x*20.0);
    } else {
      float rib = 0.5 + 0.5*sin(s.uv.x*6.2832/0.3);
      col *= 0.84 + 0.22*rib;
      if (w < 4.0) { // the doors: a seam down the middle and four locking bars
        float seam = 1.0 - smoothstep(0.02, 0.04, abs(s.uv.x - s.sz.x*0.5));
        float bars = 0.0;
        for (int k = 0; k < 4; k++) {
          float bx = s.sz.x*(0.15 + 0.233*float(k));
          bars = max(bars, 1.0 - smoothstep(0.025, 0.04, abs(s.uv.x - bx)));
        }
        col = mix(col, PAL_STEEL*0.6, max(seam, bars*step(0.15, s.uv.y)*step(s.uv.y, s.sz.y - 0.15)));
      }
      // rust streaks running down from the top rail
      float streak = smoothstep(0.62, 0.9, noise(vec2(s.uv.x*3.0, 0.0) + grime)) * smoothstep(0.0, 1.0, s.uv.y/s.sz.y);
      col = mix(col, PAL_CONTAINER_RED*0.45, streak*0.35);
    }
    // corner posts and rails
    col = mix(col*0.55, col, smoothstep(0.08, 0.12, edge));
    return col;
  }`,
};
