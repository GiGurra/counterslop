// The city far under rc_hotfix_highway at dusk: blocks of dark roofs in a
// street grid, lit windows and rooftop lights, street lamps along the
// avenues and headlights / tail lights streaming along them. Read from
// above (world xz).
export default {
  name: "nightcity",
  glsl: `vec3 mat_nightcity(Surf s){
    vec2 p = s.wuv;
    vec2 blk = floor(p/24.0), f = fract(p/24.0)*24.0;
    float street = step(f.x, 4.0) + step(f.y, 4.0);
    vec3 col;
    if (street > 0.5) {
      col = vec3(0.03, 0.03, 0.04);
      // lamps along both sides, cars: white one way, red the other
      float lampx = step(abs(f.y - 0.4), 0.15) * step(fract(p.x/8.0), 0.08);
      float lampz = step(abs(f.x - 0.4), 0.15) * step(fract(p.y/8.0), 0.08);
      float sp = 6.0 + 6.0*hash(blk);
      float carx = step(f.x, 4.0) < 0.5 ? step(0.85, hash(vec2(floor((p.x + uTime*sp)/5.0), blk.y))) * step(abs(f.y - 1.3), 0.35) : 0.0;
      float tail = step(f.x, 4.0) < 0.5 ? step(0.85, hash(vec2(floor((p.x - uTime*sp)/5.0), blk.y + 9.0))) * step(abs(f.y - 2.7), 0.35) : 0.0;
      float carz = step(0.88, hash(vec2(floor((p.y + uTime*sp)/5.0), blk.x + 3.0))) * step(abs(f.x - 2.0), 0.35) * step(f.y, 4.0 + 20.0);
      col += PAL_LED_AMBER * (lampx + lampz) * 0.9 + vec3(1.0, 0.95, 0.8) * (carx + carz*step(4.0, f.y)) + PAL_REVIEW_RED * 1.3 * tail;
      gEmit = clamp(lampx + lampz + carx + tail + carz, 0.0, 1.0);
      return col;
    }
    // a block: a few buildings, each its own roof tone, windows lit on a grid
    vec2 b = floor((f - 4.0)/10.0), bf = fract((f - 4.0)/10.0)*10.0;
    float h = hash(blk*7.0 + b);
    col = mix(vec3(0.05, 0.05, 0.08), vec3(0.12, 0.1, 0.16), h);
    float edge = step(bf.x, 0.4) + step(9.6, bf.x) + step(bf.y, 0.4) + step(9.6, bf.y);
    col *= 1.0 - 0.5*clamp(edge, 0.0, 1.0);
    vec2 wcell = floor(bf/1.2);
    float lit = step(0.62, hash(wcell + blk*13.0 + b*5.0)) * step(abs(fract(bf.x/1.2) - 0.5), 0.25) * step(abs(fract(bf.y/1.2) - 0.5), 0.25);
    vec3 warm = mix(vec3(1.0, 0.8, 0.45), vec3(0.6, 0.8, 1.0), step(0.7, hash(b + blk)));
    float beacon = step(0.9, h) * step(length(bf - 5.0), 0.5) * step(0.5, fract(uTime*0.5 + h*3.0));
    col = mix(col, warm, lit*0.85) + PAL_REVIEW_RED*beacon*1.5;
    gEmit = clamp(lit*0.8 + beacon, 0.0, 1.0);
    return col;
  }`,
};
