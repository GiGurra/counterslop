// Direction towards the sun (normalized by the renderer).
export const SUN_DIR = [0.42, 0.86, 0.3]; // ~59° elevation: noon-ish, shadows read well

// Sky: horizon-to-zenith gradient, sun disc + glow, drifting fbm clouds.
// Body of the sky fragment shader; d is the normalized view direction.
export const SKY = `vec3 sky(vec3 d){
  vec3 col = mix(uFog, PAL_ZENITH, smoothstep(-0.02, 0.55, d.y));
  float s = max(dot(d, uSun), 0.0);
  col += PAL_SUN_CORE*smoothstep(0.9985, 0.9992, s)*1.5 + PAL_SUN_GLOW*pow(s, 8.0)*0.35;
  if (d.y > 0.0) {
    vec2 p = d.xz / (d.y + 0.08) * 0.9 + vec2(uTime*0.015, 0.0);
    float c = smoothstep(0.5, 0.78, fbm(p));
    col = mix(col, PAL_CLOUD, c * smoothstep(0.0, 0.25, d.y) * 0.85);
  }
  return col;
}`;
