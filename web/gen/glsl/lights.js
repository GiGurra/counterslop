// Local lights and sky visibility from the per-map light grid (web/engine/lights.js
// builds it, the renderer uploads it). One grid cell per world cell:
//   uLightCells  (x, z, layer y) -> offset << 8 | count into uLightList (a sub-box of the grid)
//   uLightList   light indices, LIST_W per row
//   uLightData   4 texels per light (row = light): see packLights
//   uSkyVis      sky visibility 0..1 over the whole grid, filtered
// A shaded point looks up the cell half a cell out along its normal, so a wall's
// face reads the cell in front of it, not the wall's own.
export const LIST_W = 4096;

export const LIGHTS = `
uniform highp usampler2DArray uLightCells; uniform highp usampler2D uLightList;
uniform highp sampler2D uLightData; uniform highp sampler2DArray uSkyVis;
uniform vec3 uLGOrigin; uniform float uLGCell; uniform ivec3 uLGDims;
uniform ivec3 uLCOff; uniform ivec3 uLCDims; uniform int uLightsOn; uniform int uSkyOn;
uniform float uIndoor; // ambient kept where no sky is visible

float skyVis(vec3 world, vec3 n){
  if (uSkyOn == 0) return 1.0;
  vec3 p = (world + n*uLGCell*0.5 - uLGOrigin)/uLGCell - 0.5;
  if (any(lessThan(p, vec3(-0.5))) || any(greaterThan(p, vec3(uLGDims) - 0.5))) return 1.0;
  float y0 = floor(p.y), t = p.y - y0, top = float(uLGDims.y - 1);
  vec2 uv = (p.xz + 0.5) / vec2(uLGDims.xz);
  float a = texture(uSkyVis, vec3(uv, clamp(y0, 0.0, top))).r;
  float b = texture(uSkyVis, vec3(uv, clamp(y0 + 1.0, 0.0, top))).r;
  return mix(a, b, t);
}

// A dying tube: a steady hum with now and then a drop-out, seeded per light.
float flicker(float seed){
  float h = hash(vec2(seed, floor(uTime*7.0)));
  float hum = 0.93 + 0.07*noise(vec2(seed*1.7, uTime*23.0));
  return h > 0.88 ? 0.15 + 0.5*hash(vec2(seed, floor(uTime*31.0))) : hum;
}

float lightShadow(int slot, vec3 world, vec3 n, vec3 toLight, float dist);

// Sum of the local lights reaching this point (colour x intensity x N.L x falloff x cone).
vec3 localLight(vec3 world, vec3 n){
  if (uLightsOn == 0) return vec3(0.0);
  ivec3 c = ivec3(floor((world + n*uLGCell*0.5 - uLGOrigin)/uLGCell)) - uLCOff;
  if (any(lessThan(c, ivec3(0))) || any(greaterThanEqual(c, uLCDims))) return vec3(0.0);
  uint e = texelFetch(uLightCells, ivec3(c.x, c.z, c.y), 0).r;
  int cnt = int(e & 255u), off = int(e >> 8u);
  vec3 sum = vec3(0.0);
  for (int i = 0; i < cnt; i++) {
    int k = off + i;
    int li = int(texelFetch(uLightList, ivec2(k % ${LIST_W}, k / ${LIST_W}), 0).r);
    vec4 a = texelFetch(uLightData, ivec2(0, li), 0);
    vec3 L = a.xyz - world;
    float d2 = dot(L, L), r2 = a.w*a.w;
    if (d2 >= r2) continue;
    float d = sqrt(d2);
    L /= max(d, 1e-4);
    float ndl = dot(n, L);
    if (ndl <= 0.0) continue;
    vec4 b = texelFetch(uLightData, ivec2(1, li), 0);
    vec4 s = texelFetch(uLightData, ivec2(2, li), 0);
    vec4 x = texelFetch(uLightData, ivec2(3, li), 0);
    float w = 1.0 - d2/r2;
    float att = w*w / (1.0 + d2);
    if (s.w > -1.5) att *= smoothstep(s.w, x.x, dot(-L, s.xyz));
    if (b.w > 0.0) att *= flicker(b.w);
    if (att <= 0.0) continue;
    if (x.y >= 0.0) att *= lightShadow(int(x.y), world, n, L, d);
    sum += b.rgb * (ndl * att);
  }
  return sum;
}
// Shadowed local lights come in a later step: every light is unshadowed for now.
float lightShadow(int slot, vec3 world, vec3 n, vec3 toLight, float dist){ return 1.0; }
`;
