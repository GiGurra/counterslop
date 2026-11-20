// Shared uniforms, hemisphere ambient + shadowed sun, and distance fog. Colours come from the palette constants.
import { SHADOW } from "./shadow.js";

export const LIGHTING = `
uniform vec3 uEye; uniform vec3 uSun; uniform vec3 uFog; uniform float uTime;
${SHADOW}
// Ambient (sky/ground hemisphere) + sun diffuse x shadow; world is the shaded point.
vec3 light(vec3 col, vec3 world, vec3 n, float ao){
  float diff = max(dot(n, uSun), 0.0);
  float sh = diff > 0.0 ? shadow(world, n) : 1.0;
  vec3 hemi = mix(PAL_HEMI_GROUND, PAL_HEMI_SKY, n.y*0.5 + 0.5);
  return col * (hemi*0.6*ao + PAL_SUN_LIGHT*diff*sh*0.95 * mix(1.0, ao, 0.35));
}
vec3 fog(vec3 col, vec3 world){
  float d = length(world - uEye);
  return mix(col, uFog, (1.0 - exp(-d*0.014)) * 0.85);
}
`;
