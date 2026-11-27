// The data sea far under rc_ci_pipeline: a dark ocean of glowing grid lines with
// packets of bits streaming along them and slow swells of light rolling across.
// Read from above (world xz), it tiles without seams.
export default {
  name: "datasea",
  glsl: `vec3 mat_datasea(Surf s){
    vec2 p = s.wuv;
    float swell = fbm(p*0.03 + vec2(uTime*0.03, -uTime*0.02));
    vec3 col = mix(vec3(0.01, 0.02, 0.06), vec3(0.03, 0.08, 0.2), swell);
    // the grid: 4 m cells, fine lines
    vec2 g = abs(fract(p/4.0) - 0.5);
    float line = 1.0 - smoothstep(0.0, 0.02, min(g.x, g.y) - 0.0);
    // packets: lanes along x (rows) and z (columns) stream bits, each lane its own speed
    float row = floor(p.y/4.0 + 0.5), colm = floor(p.x/4.0 + 0.5);
    float sx = 4.0 + 8.0*hash(vec2(row, 7.0)), sz = 4.0 + 8.0*hash(vec2(colm, 13.0));
    float bx = step(0.93, hash(vec2(floor((p.x + uTime*sx)/3.0), row))) * step(g.y, 0.06);
    float bz = step(0.95, hash(vec2(floor((p.y - uTime*sz)/3.0), colm + 50.0))) * step(g.x, 0.06);
    vec3 lineC = mix(PAL_NEON_CYAN, PAL_NEON_MAGENTA, step(0.75, hash(vec2(row, colm))));
    float bits = clamp(bx + bz, 0.0, 1.0);
    col = mix(col, lineC * (0.25 + 0.5*swell), line * 0.6);
    col = mix(col, lineC * 1.2 + 0.2, bits);
    gEmit = clamp(line*0.5 + bits + swell*0.3, 0.0, 1.0);
    return col;
  }`,
};
