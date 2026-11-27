// RAM sticks, giant (de_rackmount): a heat spreader on the big faces with a
// lit RGB strip on top (they have RGB, of course), a row of gold contacts at
// the bottom edge, plain black on the narrow ends.
export default {
  name: "ram",
  glsl: `vec3 mat_ram(Surf s){
    vec3 col = PAL_RAM_HEAT * (0.85 + 0.2*fbm(s.wuv*4.0));
    float edge = edgeDist(s.uv, s.sz);
    if (min(s.sz.x, s.sz.y) < 0.5) return PAL_CHIP * 1.3; // the stick's thin sides and ends
    // the long axis runs along the face's longer side
    vec2 u = s.sz.x >= s.sz.y ? s.uv : s.uv.yx;
    vec2 z = s.sz.x >= s.sz.y ? s.sz : s.sz.yx;
    // fins pressed into the spreader
    col *= 0.85 + 0.15*aaPulse(u.x*2.5, 0.5, 1.0); // antialiased (aaStep & co, gen/glsl/patterns.js): MSAA leaves lines drawn inside a face jagged
    // gold contacts along one long edge
    float pins = (1.0 - aaStep(0.25, u.y)) * aaPulse(u.x*6.0, 0.35, 1.0);
    col = mix(col, PAL_GOLD, pins);
    // an RGB strip along the other long edge
    float strip = aaStep(z.y - 0.18, u.y);
    vec3 rgb = 0.5 + 0.5*cos(6.2832*(u.x*0.08 - uTime*0.2 + vec3(0.0, 0.33, 0.67)));
    gEmit = strip;
    col = mix(col, rgb, strip);
    // the label sticker in the middle: DDR-ish
    float lab = aaLine(u.x - z.x*0.5, z.x*0.18) * aaLine(u.y - z.y*0.5, z.y*0.16);
    return mix(col, vec3(0.85, 0.86, 0.9), lab * aaStep(0.02, edge));
  }`,
};
