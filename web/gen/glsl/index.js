// Assemble GLSL from chunks: a tiny #include by concatenation, no build step.
import { NOISE } from "./noise.js";
import { PATTERNS } from "./patterns.js";
import { LIGHTING } from "./lighting.js";

export { NOISE, PATTERNS, LIGHTING };

// vec3 constants from a {name: [r,g,b]} palette: sand -> PAL_SAND.
export function paletteGlsl(pal) {
  return Object.entries(pal)
    .map(([k, c]) => `const vec3 PAL_${k.replace(/[A-Z]/g, (m) => "_" + m).toUpperCase()} = vec3(${c.map(f).join(", ")});`)
    .join("\n");
}

const f = (x) => (Number.isInteger(x) ? x.toFixed(1) : String(x));

// Fragment shader prelude shared by every lit shader.
export function prelude(pal) {
  return `#version 300 es
precision highp float;
${paletteGlsl(pal)}
${NOISE}
${PATTERNS}
${LIGHTING}`;
}
