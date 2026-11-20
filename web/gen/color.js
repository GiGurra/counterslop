export function hexRgb(hex) {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16 & 255) / 255, (n >> 8 & 255) / 255, (n & 255) / 255];
}
export const scaleRgb = (c, k) => c.map((x) => x * k);

export const rgbHex = (c) => "#" + c.map((x) => Math.round(Math.max(0, Math.min(1, x)) * 255).toString(16).padStart(2, "0")).join("");

// Hue (0..1) to a fully saturated RGB colour.
export function hue(h) {
  const f = (n) => { const k = (n + h * 6) % 6; return 1 - Math.max(0, Math.min(1, Math.min(k, 4 - k))); };
  return [f(5), f(3), f(1)];
}
