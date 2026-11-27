// The city far under rc_hotfix_highway at dusk: blocks of dark roofs in a
// street grid, lit windows and rooftop lights, street lamps along the
// avenues and headlights / tail lights streaming along them. Read from
// above (world xz).
export default {
  name: "nightcity",
  glsl: `vec3 mat_nightcity(Surf s){
    // Streets and blocks are both worked out and mixed (no branch per pixel: the
    // antialiased lines, gen/glsl/patterns.js aaPulse, take derivatives).
    vec2 p = s.wuv;
    vec2 blk = floor(p/24.0), f = fract(p/24.0)*24.0;
    float stX = aaPulse(p.x/24.0, 0.0, 4.0/24.0), stY = aaPulse(p.y/24.0, 0.0, 4.0/24.0);
    float street = max(stX, stY);
    // lamps along both sides, cars: white one way, red the other
    vec3 sc = vec3(0.03, 0.03, 0.04);
    float lampx = aaPulse(p.y/24.0, 0.25/24.0, 0.55/24.0) * aaPulse(p.x/8.0, 0.0, 0.08);
    float lampz = aaPulse(p.x/24.0, 0.25/24.0, 0.55/24.0) * aaPulse(p.y/8.0, 0.0, 0.08);
    float sp = 6.0 + 6.0*hash(blk);
    float carx = (1.0 - stX) * step(0.85, hash(vec2(floor((p.x + uTime*sp)/5.0), blk.y))) * aaPulse(p.y/24.0, 0.95/24.0, 1.65/24.0);
    float tail = (1.0 - stX) * step(0.85, hash(vec2(floor((p.x - uTime*sp)/5.0), blk.y + 9.0))) * aaPulse(p.y/24.0, 2.35/24.0, 3.05/24.0);
    float carz = step(0.88, hash(vec2(floor((p.y + uTime*sp)/5.0), blk.x + 3.0))) * aaPulse(p.x/24.0, 1.65/24.0, 2.35/24.0);
    sc += PAL_LED_AMBER * (lampx + lampz) * 0.9 + vec3(1.0, 0.95, 0.8) * (carx + carz*(1.0 - stY)) + PAL_REVIEW_RED * 1.3 * tail;
    float se = clamp(lampx + lampz + carx + tail + carz, 0.0, 1.0);
    // a block: a few buildings, each its own roof tone, windows lit on a grid
    vec2 b = floor((f - 4.0)/10.0), bf = fract((f - 4.0)/10.0)*10.0, u = (f - 4.0)/10.0;
    float h = hash(blk*7.0 + b);
    vec3 col = mix(vec3(0.05, 0.05, 0.08), vec3(0.12, 0.1, 0.16), h);
    float edge = aaPulse(u.x, 0.0, 0.04) + aaPulse(u.x, 0.96, 1.0) + aaPulse(u.y, 0.0, 0.04) + aaPulse(u.y, 0.96, 1.0);
    col *= 1.0 - 0.5*clamp(edge, 0.0, 1.0);
    vec2 wcell = floor(bf/1.2);
    float lit = step(0.62, hash(wcell + blk*13.0 + b*5.0)) * aaPulse(bf.x/1.2, 0.25, 0.75) * aaPulse(bf.y/1.2, 0.25, 0.75);
    vec3 warm = mix(vec3(1.0, 0.8, 0.45), vec3(0.6, 0.8, 1.0), step(0.7, hash(b + blk)));
    float beacon = step(0.9, h) * (1.0 - aaStep(0.5, length(bf - 5.0))) * step(0.5, fract(uTime*0.5 + h*3.0));
    col = mix(col, warm, lit*0.85) + PAL_REVIEW_RED*beacon*1.5;
    gEmit = mix(clamp(lit*0.8 + beacon, 0.0, 1.0), se, street);
    return mix(col, sc, street);
  }`,
};
