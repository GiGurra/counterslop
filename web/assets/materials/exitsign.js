// rc_hotfix_highway's gantry signs: a highway sign's green board with a
// white border and a big white arrow pointing up (straight on), lit from
// below so it reads at dusk. Back and edges: grey steel.
export default {
  name: "exitsign",
  glsl: `vec3 mat_exitsign(Surf s){
    vec3 steel = vec3(0.32, 0.34, 0.36);
    if (s.top || min(s.sz.x, s.sz.y) < 0.3) return steel;
    vec2 r = readUV(s.n, s.uv, s.sz);
    vec3 col = vec3(0.02, 0.36, 0.18) * (0.9 + 0.1*fbm(s.wuv*4.0));
    float b = min(min(r.x, s.sz.x - r.x), min(r.y, s.sz.y - r.y));
    float border = step(abs(b - 0.12), 0.05);
    // arrows: one per 3 m of board, pointing up
    float cw = s.sz.x / max(1.0, floor(s.sz.x/3.0));
    vec2 q = vec2(fract(r.x/cw)*cw - cw*0.5, r.y - s.sz.y*0.15) / (s.sz.y*0.7);
    float shaft = step(abs(q.x), 0.07) * step(0.0, q.y) * step(q.y, 0.6);
    float head = step(0.45, q.y) * step(q.y, 0.95) * step(abs(q.x), (0.95 - q.y)*0.6);
    float on = clamp(border + shaft + head, 0.0, 1.0);
    gEmit = 0.35 + on*0.4;
    return mix(col, vec3(0.95), on);
  }`,
};
