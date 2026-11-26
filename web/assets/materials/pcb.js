// A motherboard, giant (de_rackmount): green solder mask, copper traces in
// right-angle runs, silver via dots and the odd silkscreen label block.
export default {
  name: "pcb",
  glsl: `vec3 mat_pcb(Surf s){
    vec2 w = s.wuv;
    vec3 col = PAL_PCB * (0.9 + 0.15*fbm(w*2.0));
    // traces: a 0.5 m grid of lanes, each cell lit along one axis by its hash
    vec2 cell = floor(w/0.5), f = fract(w/0.5);
    float hx = hash(cell), hy = hash(cell + 17.0);
    float tr = step(0.55, hx) * (1.0 - smoothstep(0.04, 0.07, abs(f.y - 0.5)))
             + step(0.6, hy) * (1.0 - smoothstep(0.04, 0.07, abs(f.x - 0.5)));
    col = mix(col, PAL_PCB_TRACE, clamp(tr, 0.0, 1.0)*0.8);
    // vias where traces turn
    float via = step(0.7, hash(cell + 3.0)) * (1.0 - smoothstep(0.06, 0.09, length(f - 0.5)));
    col = mix(col, PAL_SOLDER, via);
    // silkscreen: a white outlined block now and then (a 4 m grid)
    vec2 big = floor(w/4.0), g = fract(w/4.0)*4.0;
    float label = step(0.7, hash(big + 9.0)) * step(0.4, g.x) * step(g.x, 2.2) * step(0.4, g.y) * step(g.y, 1.0);
    float frame = label * (1.0 - step(0.06, min(min(g.x - 0.4, 2.2 - g.x), min(g.y - 0.4, 1.0 - g.y))));
    col = mix(col, vec3(0.9), frame*0.8);
    return col;
  }`,
};
