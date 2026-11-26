// Direction towards the sun (normalized by the renderer).
export const SUN_DIR = [0.42, 0.86, 0.3]; // ~59° elevation: noon-ish, shadows read well

// The skies a map may ask for (content.Map.Sky): the renderer passes the index as uSkyMode.
export const SKIES = ["", "space"];

// Where the Earth and the Moon hang in the space sky (unit-ish directions, angular radius).
// West is Earth (seen from lab_moon's Moon), east and up the Moon, over the plateau.
const EARTH = "normalize(vec3(-0.86, 0.30, 0.18)), 0.17";
const MOON = "normalize(vec3(0.78, 0.48, -0.30)), 0.09";

// Sky: horizon-to-zenith gradient, sun disc + glow, drifting fbm clouds.
// Space (uSkyMode 1, Star Slop): black, a seeded star field (no twinkle: screenshots stay
// stable), a faint galactic band, a hard small sun, the Earth (oceans, land, clouds, a
// blue limb) and the Moon (grey, cratered), both lit by the sun (the Moon from a cheated angle) with a little earthshine.
// Body of the sky fragment shader; d is the normalized view direction.
export const SKY = `vec3 planet(vec3 col, vec3 d, vec3 c, float r, float earth){
  float ca = dot(d, c);
  if (ca < cos(r*1.25)) return col;
  vec3 u = normalize(cross(c, vec3(0.0, 1.0, 0.0)));
  vec3 v = cross(u, c);
  vec2 p = vec2(dot(d, u), dot(d, v)) / sin(r);
  float rr = dot(p, p);
  if (rr > 1.0) {
    float halo = earth * smoothstep(1.25, 1.0, sqrt(rr)) * max(0.15, dot(-c, uSun) * 0.5 + 0.5);
    return col + vec3(0.25, 0.5, 1.0) * 0.45 * halo;
  }
  vec3 n = p.x*u + p.y*v - sqrt(1.0 - rr)*c;
  // Earth: the sun's light. The Moon hangs near the sun, so the sun would show its night
  // side: it is lit from the sun pulled round towards the viewer (gibbous), a cheat.
  vec3 L = earth > 0.5 ? uSun : normalize(uSun - 1.6*c);
  float lit = max(dot(n, L), 0.0);
  vec3 surf;
  if (earth > 0.5) {
    float land = smoothstep(0.52, 0.58, fbm(p*2.2 + 4.0));
    surf = mix(vec3(0.05, 0.16, 0.42), mix(vec3(0.20, 0.42, 0.16), vec3(0.62, 0.55, 0.38), fbm(p*5.0)), land);
    float cloud = smoothstep(0.5, 0.75, fbm(p*3.5 + vec2(uTime*0.004, 1.0)));
    surf = mix(surf, vec3(0.95), cloud*0.85);
    surf = mix(surf, vec3(0.35, 0.6, 1.0), pow(1.0 - sqrt(1.0 - rr), 3.0)*0.6); // the atmosphere at the limb
  } else {
    float g = fbm(p*3.0 + 2.0);
    surf = vec3(0.62, 0.62, 0.64) * (0.7 + 0.45*g);
    vec2 cell = floor(p*4.0);
    vec2 cc = (cell + 0.5 + 0.35*(vec2(hash(cell), hash(cell + 3.0)) - 0.5))/4.0;
    float cr = length(p - cc) / (0.05 + 0.08*hash(cell + 7.0));
    surf *= 1.0 - 0.3*(1.0 - smoothstep(0.6, 1.0, cr));
  }
  return surf * (0.04 + 1.1*lit);
}
vec3 spaceSky(vec3 d){
  vec3 col = vec3(0.004, 0.005, 0.012);
  vec3 bn = normalize(vec3(0.35, 0.55, -0.76));
  float band = exp(-pow(dot(d, bn)*5.0, 2.0));
  col += vec3(0.06, 0.05, 0.085) * band * (0.4 + fbm(vec2(dot(d, vec3(1.0, 0.0, 0.3)), d.y)*7.0));
  vec3 q = d*160.0;
  vec3 cell = floor(q);
  vec3 f = fract(q) - 0.5;
  float h = fract(sin(dot(cell, vec3(12.9898, 78.233, 37.719)))*43758.5453);
  float star = step(0.986, h) * smoothstep(0.32, 0.05, length(f));
  col += mix(vec3(1.0, 0.85, 0.7), vec3(0.75, 0.85, 1.0), fract(h*53.1)) * star * (0.35 + 0.9*fract(h*91.7));
  float s = max(dot(d, uSun), 0.0);
  col += PAL_SUN_CORE*smoothstep(0.9993, 0.9996, s)*2.2 + PAL_SUN_GLOW*pow(s, 90.0)*0.3;
  col = planet(col, d, ${EARTH}, 1.0);
  col = planet(col, d, ${MOON}, 0.0);
  return col;
}
vec3 sky(vec3 d){
  if (uSkyMode == 1) return spaceSky(d);
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
