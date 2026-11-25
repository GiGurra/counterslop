// The surf ramps of surf_scope_creep: every slope is a Gantt chart of a roadmap
// that never ends. Chart paper with week lines every 2 m along the ramp, two or
// three task lanes of coloured bars (each lane's colour and its bars' lengths
// seeded by where it is), the done part of each bar darker, a red "today" line
// that is always behind, and "+scope" diamonds (milestones that moved). Narrow
// faces, walls and the wedges' ends: the dark project-tool panel.
export default {
  name: "gantt",
  glsl: `vec3 mat_gantt(Surf s){
    if (abs(s.n.y) < 0.05 || s.n.y < -0.5 || min(s.sz.x, s.sz.y) < 1.0)
      return PAL_SIGN_BACK * (1.2 + 0.3*fbm(s.wuv*3.0));
    bool alongX = s.sz.x >= s.sz.y;
    float a = alongX ? s.uv.x : s.uv.y, w = alongX ? s.sz.y : s.sz.x, c = alongX ? s.uv.y : s.uv.x;
    vec3 col = mix(vec3(0.93, 0.93, 0.9), vec3(0.85, 0.87, 0.9), step(0.5, fract(a/4.0)));
    col *= 0.97 + 0.03*fbm(s.wuv*4.0);
    col = mix(col, vec3(0.62, 0.66, 0.72), (1.0 - smoothstep(0.0, 0.03, abs(fract(a/2.0 + 0.5) - 0.5)*2.0)) * 0.6);
    float lanes = clamp(floor(w/2.5), 2.0, 3.0), lane = floor(c/w*lanes), lc = fract(c/w*lanes);
    float seed = hash(vec2(lane, floor(s.world.x/7.0) + 13.0*floor(s.world.z/7.0)));
    vec3 bar = seed < 0.25 ? PAL_TOY_TEAL : seed < 0.5 ? PAL_TOY_PINK : seed < 0.75 ? PAL_TOY_YELLOW : PAL_NEON_CYAN;
    // bars: tasks of 3..9 m with 1 m gaps, each its own length
    float cell = floor(a/10.0), ca = fract(a/10.0)*10.0, len = 3.0 + 6.0*hash(vec2(cell, seed*31.0));
    float inBar = step(0.5, ca) * step(ca, 0.5 + len) * step(0.22, lc) * step(lc, 0.78);
    float done = step(ca, 0.5 + len*hash(vec2(seed, cell + 7.0)));
    col = mix(col, bar * mix(1.0, 0.62, done), inBar);
    // milestones that slipped: a diamond after each bar
    vec2 d = vec2((ca - (len + 1.2))/0.35, (lc - 0.5)/(0.35*lanes/w));
    col = mix(col, PAL_REVIEW_RED, step(abs(d.x) + abs(d.y), 1.0));
    // today: a red line, always behind the plan
    float today = 1.0 - smoothstep(0.03, 0.06, abs(fract(a/23.0 + hash(vec2(seed, 2.0))) - 0.3)*23.0);
    col = mix(col, PAL_REVIEW_RED, today*0.85);
    float edge = 1.0 - smoothstep(0.05, 0.1, edgeDist(s.uv, s.sz));
    return mix(col, PAL_SIGN_BACK, edge);
  }`,
};
