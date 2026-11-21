// Shared uniforms, hemisphere ambient (dimmed where the sky is hidden: indoors) +
// shadowed sun + local lights, and distance fog. Colours come from the palette constants.
import { SHADOW } from "./shadow.js";
import { LIGHTS } from "./lights.js";

export const LIGHTING = `
uniform vec3 uEye; uniform vec3 uSun; uniform vec3 uFog; uniform float uTime; uniform float uFogK;
uniform int uLightDebug; // 1: local lights only (no sky, no sun): for checking lamp shadows
${SHADOW}
${LIGHTS}
// Contact shadows: a soft dark patch on the ground under each nearby player (xyz the
// feet, w the radius), so a player stands on the floor even where many overlapping
// lamps wash out their real shadow. Darkens the ambient and lamp light, not the sun
// (which casts real shadows).
#define MAX_BLOBS 8
uniform vec4 uBlobs[MAX_BLOBS]; uniform int uBlobN;
float contact(vec3 world, vec3 n){
  if (n.y < 0.3) return 1.0;
  float c = 1.0;
  for (int i = 0; i < MAX_BLOBS; i++) {
    if (i >= uBlobN) break;
    vec4 b = uBlobs[i];
    float h = world.y - b.y;
    if (h < -0.15 || h > 0.6) continue;
    float d = length(world.xz - b.xz) / b.w;
    c *= 1.0 - 0.75 * (1.0 - smoothstep(0.2, 1.0, d)) * (1.0 - smoothstep(0.05, 0.6, h));
  }
  return c;
}
// Ambient (sky/ground hemisphere x sky visibility) + sun diffuse x shadow + local
// lights; world is the shaded point.
vec3 light(vec3 col, vec3 world, vec3 n, float ao){
  float cs = contact(world, n);
  vec3 local = localLight(world, n) + dynLight(world, n);
  if (uLightDebug == 1) return col * local * cs;
  float diff = max(dot(n, uSun), 0.0);
  float sh = diff > 0.0 ? shadow(world, n) : 1.0;
  vec3 hemi = mix(PAL_HEMI_GROUND, PAL_HEMI_SKY, n.y*0.5 + 0.5);
  float sky = mix(uIndoor, 1.0, skyVis(world, n));
  return col * (hemi*0.6*ao*sky*cs + PAL_SUN_LIGHT*diff*sh*0.95 * mix(1.0, ao, 0.35)
    + local * mix(1.0, ao, 0.5) * cs);
}
vec3 fog(vec3 col, vec3 world){
  float d = length(world - uEye);
  return mix(col, uFog, (1.0 - exp(-d*uFogK)) * 0.85);
}
`;
