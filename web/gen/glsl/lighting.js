// Shared uniforms, hemisphere ambient (dimmed where the sky is hidden: indoors) +
// shadowed sun + local lights, and distance fog. Colours come from the palette constants.
import { SHADOW } from "./shadow.js";
import { LIGHTS } from "./lights.js";

export const LIGHTING = `
uniform vec3 uEye; uniform vec3 uSun; uniform vec3 uFog; uniform float uTime; uniform float uFogK;
uniform int uLightDebug; // 1: local lights only (no sky, no sun): for checking lamp shadows
${SHADOW}
${LIGHTS}
// Ambient (sky/ground hemisphere x sky visibility) + sun diffuse x shadow + local
// lights; world is the shaded point.
vec3 light(vec3 col, vec3 world, vec3 n, float ao){
  if (uLightDebug == 1) return col * localLight(world, n);
  float diff = max(dot(n, uSun), 0.0);
  float sh = diff > 0.0 ? shadow(world, n) : 1.0;
  vec3 hemi = mix(PAL_HEMI_GROUND, PAL_HEMI_SKY, n.y*0.5 + 0.5);
  float sky = mix(uIndoor, 1.0, skyVis(world, n));
  return col * (hemi*0.6*ao*sky + PAL_SUN_LIGHT*diff*sh*0.95 * mix(1.0, ao, 0.35)
    + localLight(world, n) * mix(1.0, ao, 0.5));
}
vec3 fog(vec3 col, vec3 world){
  float d = length(world - uEye);
  return mix(col, uFog, (1.0 - exp(-d*uFogK)) * 0.85);
}
`;
