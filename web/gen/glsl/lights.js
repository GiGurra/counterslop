// Local lights and sky visibility from the per-map light grid (web/engine/lights.js
// builds it, the renderer uploads it). One grid cell per world cell:
//   uLightCells  (x, z, layer y) -> offset << 8 | count into uLightList (a sub-box of the grid)
//   uLightList   light indices, LIST_W per row
//   uLightData   4 texels per light (row = light): see packLights
//   uSkyVis      sky visibility 0..1 over the whole grid, filtered
// A shaded point looks up the cell half a cell out along its normal, so a wall's
// face reads the cell in front of it, not the wall's own.
export const LIST_W = 4096;
// Shadowed lights: up to MAX_SHADOW_SLOTS, each a row of 6 tiles in one depth atlas
// (a spot uses tile 0, a point light all six cube faces).
export const MAX_SHADOW_SLOTS = 4;

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

float lightShadow(int slot, bool spot, vec3 world, vec3 n, vec3 toLight, float dist);

// Sum of the local lights reaching this point (colour x intensity x N.L x falloff x cone).
// Compiled in only with HAS_LIGHTS (the renderer picks the variant per map): a map
// without lights runs none of this, which matters on CPU rasterisers (SwiftShader).
#ifndef HAS_LIGHTS
vec3 localLight(vec3 world, vec3 n){ return vec3(0.0); }
#else
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
    if (x.y >= 0.0) att *= lightShadow(int(x.y), s.w > -1.5, world, n, L, d);
    sum += b.rgb * (ndl * att);
  }
  return sum;
}
#endif
// Shadow of a shadowed light (slot = its row in the atlas). The tile is the spot's
// one view, or the cube face looking towards the point (engine/lights.js cubeFace).
uniform highp sampler2DShadow uLShadow; uniform highp sampler2D uLSMat;
uniform int uLSRows; uniform float uLSSize;
// uLSMat row slot*6 + face: texels 0-3 the view's world -> clip matrix (columns),
// texel 4 .x the texel size in metres per metre of distance. A texture, not a
// uniform array: SwiftShader copies every uniform on every draw.
mat4 lsMat(int r){
  return mat4(texelFetch(uLSMat, ivec2(0, r), 0), texelFetch(uLSMat, ivec2(1, r), 0),
              texelFetch(uLSMat, ivec2(2, r), 0), texelFetch(uLSMat, ivec2(3, r), 0));
}
#ifndef HAS_LIGHT_SHADOWS
float lightShadow(int slot, bool spot, vec3 world, vec3 n, vec3 toLight, float dist){ return 1.0; }
#else
float lightShadow(int slot, bool spot, vec3 world, vec3 n, vec3 toLight, float dist){
  if (slot >= uLSRows) return 1.0;
  int face = 0;
  if (!spot) {
    vec3 v = -toLight, a = abs(v);
    face = a.x >= a.y && a.x >= a.z ? (v.x > 0.0 ? 0 : 1) : a.y >= a.z ? (v.y > 0.0 ? 2 : 3) : (v.z > 0.0 ? 4 : 5);
  }
  // Normal offset of ~1.5 texels at this distance, more at grazing angles.
  float ndl = clamp(dot(n, toLight), 0.0, 1.0);
  vec3 p = world + n * texelFetch(uLSMat, ivec2(4, slot*6), 0).x * dist * (1.5 + 2.0*(1.0 - ndl));
  vec4 c = lsMat(slot*6 + face) * vec4(p, 1.0);
  vec3 s = c.xyz / c.w * 0.5 + 0.5;
  if (c.w <= 0.0 || s.z >= 1.0) return 1.0;
  vec2 tile = vec2(1.0/6.0, 1.0/float(uLSRows));
  vec2 tx = 1.0 / vec2(6.0*uLSSize, float(uLSRows)*uLSSize);
  vec2 lo = vec2(float(face), float(slot))*tile + tx*1.5, hi = vec2(float(face + 1), float(slot + 1))*tile - tx*1.5;
  vec2 uv = (vec2(float(face), float(slot)) + s.xy) * tile;
  float sum = 0.0;
  for (int y = 0; y < 2; y++) for (int x = 0; x < 2; x++)
    sum += texture(uLShadow, vec3(clamp(uv + (vec2(x, y) - 0.5)*tx, lo, hi), s.z - 0.00005));
  return sum * 0.25;
}
#endif
`;
