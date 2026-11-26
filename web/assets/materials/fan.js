// Case fans, giant (de_rackmount): a square black frame round a spinning
// seven-bladed rotor (seen through on any big face), a hub sticker; the
// thin sides are the frame's ribbed plastic.
export default {
  name: "fan",
  glsl: `vec3 mat_fan(Surf s){
    vec3 col = PAL_CHIP * 1.4;
    if (min(s.sz.x, s.sz.y) < 1.0) return col * (0.85 + 0.15*step(0.5, fract(s.uv.x*4.0 + s.uv.y*4.0)));
    // one fan per square cell of the face's shorter side
    float cs = min(s.sz.x, s.sz.y);
    vec2 cell = vec2(mod(s.uv.x, cs), mod(s.uv.y, cs)) - cs*0.5;
    float r = length(cell) / (cs*0.5);
    float a = atan(cell.y, cell.x);
    float spin = uTime * 9.0 + hash(floor(s.uv/cs)) * 6.0;
    float blade = step(0.5, fract((a + spin + r*1.2) * 7.0 / 6.2832));
    vec3 rotor = mix(PAL_FAN_BLADE, PAL_FAN_BLADE*2.2, blade);
    float inside = step(r, 0.92);
    col = mix(col, rotor * 0.9, inside);
    col = mix(col, vec3(0.75), step(r, 0.2));             // the hub sticker
    col = mix(col, PAL_CHIP, step(r, 0.08));
    float ring = 1.0 - smoothstep(0.02, 0.05, abs(r - 0.94)); // guard ring
    return mix(col, col*0.4, ring);
  }`,
};
