// Lit letters and digits (content.Sign with mat "led": departure boards, the
// T-minus clock, seatbelt signs): warm amber dots that glow in the shade, each
// dot a little brighter at its middle, a faint scan flicker.
export default {
  name: "led",
  glsl: `vec3 mat_led(Surf s){
    gEmit = 0.9;
    float flick = 0.94 + 0.06*step(0.5, fract(uTime*0.7 + hash(floor(s.world.xz*3.0))));
    return PAL_LED_AMBER * 1.25 * flick;
  }`,
};
