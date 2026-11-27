// Parked car body: glossy paint, a dark sill along the bottom, door seams on
// the long sides, head- and tail-lamps on the ends. The paint colour comes from
// the body's footprint (prefabs.Car encodes it): width 1.80/1.82/1.84/1.86 m,
// length 4.2/4.3/4.4/4.5 m, so every face of one car agrees.
export default {
  name: "car",
  glsl: `vec3 mat_car(Surf s){
    float w = s.top ? max(s.sz.x, s.sz.y) : s.sz.x;
    float idx = w < 3.0 ? (w - 1.8)/0.02 : (w - 4.2)/0.1;
    int i = int(mod(floor(idx + 0.5), 4.0));
    vec3 paint = i == 0 ? PAL_CAR_RED : i == 1 ? PAL_CAR_BLUE : i == 2 ? PAL_CAR_SILVER : PAL_CAR_GREEN;
    vec3 col = paint * (0.9 + 0.12*fbm(s.wuv*1.5 + vec2(float(i)*5.0)));
    if (s.top) return col * 0.92;
    if (s.n.y < -0.5) return PAL_TYRE;
    float y = s.uv.y / s.sz.y;
    col = mix(PAL_TYRE, col, smoothstep(0.26, 0.3, y)); // the sill
    if (w >= 3.0) { // a long side: two door seams
      float u = s.uv.x / s.sz.x;
      float seam = max(1.0 - smoothstep(0.004, 0.008, abs(u - 0.36)), 1.0 - smoothstep(0.004, 0.008, abs(u - 0.62)));
      col = mix(col, col*0.35, seam*aaStep(0.3, y));
    } else { // an end: a lamp at each corner, white at +x/+z, red behind
      float lu = min(s.uv.x, s.sz.x - s.uv.x);
      float lamp = aaBand(lu, 0.12, 0.42)*aaBand(y, 0.58, 0.8); // antialiased (MSAA leaves in-face lines jagged)
      vec3 lc = s.n.x + s.n.z > 0.0 ? PAL_LAMP : PAL_CAR_RED*1.6;
      col = mix(col, lc, lamp);
    }
    return col;
  }`,
};
