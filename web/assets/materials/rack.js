// Server racks: dark perforated cabinet doors in 0.6 m bays, a status LED
// column per bay blinking at its own seeded rate (green, now and then amber
// or blue), and a plain dark top.
export default {
  name: "rack",
  glsl: `vec3 mat_rack(Surf s){
    vec3 col = PAL_RACK * (0.9 + 0.15*fbm(s.wuv*3.0));
    if (s.top) return col*0.8;
    float bayW = 0.6;
    float bay = floor(s.uv.x/bayW);
    vec2 cell = vec2(fract(s.uv.x/bayW), s.uv.y);
    col = mix(col, col*0.55, gridLine(s.uv.x/bayW, 0.03));
    vec2 holes = fract(s.uv*vec2(28.0, 28.0)) - 0.5;
    col *= 0.88 + 0.12*smoothstep(0.18, 0.28, length(holes));
    float row = floor(s.uv.y/0.09);
    vec2 led = vec2(cell.x - 0.12, fract(s.uv.y/0.09) - 0.5);
    float on = step(0.45, fract(uTime*(0.7 + 2.5*hash(vec2(bay, row))) + hash(vec2(row, bay))));
    float h = hash(vec2(bay*7.0, row*3.0));
    vec3 ledCol = h > 0.92 ? PAL_LED_AMBER : (h > 0.8 ? PAL_LED_BLUE : PAL_LED_GREEN);
    float dot = 1.0 - smoothstep(0.02, 0.04, length(led*vec2(1.0, 0.25)));
    float inBand = step(0.3, s.uv.y) * step(s.uv.y, min(2.0, s.sz.y - 0.1));
    return mix(col, ledCol*(0.35 + 1.1*on), dot*inBand);
  }`,
};
