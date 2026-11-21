// Music theory helpers: scales, degrees, chords, MIDI -> Hz. Pure.
export const SCALES = {
  major: [0, 2, 4, 5, 7, 9, 11], minor: [0, 2, 3, 5, 7, 8, 10], dorian: [0, 2, 3, 5, 7, 9, 10],
  mixolydian: [0, 2, 4, 5, 7, 9, 10], lydian: [0, 2, 4, 6, 7, 9, 11], harmonicMinor: [0, 2, 3, 5, 7, 8, 11],
};
export const minorish = (scale) => scale[2] === 3;

export const mtof = (m) => 440 * 2 ** ((m - 69) / 12);

// MIDI note of scale degree d (0-based; negative or past the octave wraps) above root.
export function note(root, scale, d) {
  const n = scale.length, o = Math.floor(d / n), i = ((d % n) + n) % n;
  return root + 12 * o + scale[i];
}

// Stacked thirds on degree d: size 3 = triad, 4 = seventh chord.
export const chord = (root, scale, d, size = 3) => Array.from({ length: size }, (_, k) => note(root, scale, d + 2 * k));

// 32-bit FNV-1a of a string: a seed from a name (MVP themes).
export function hashString(s) {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 0x01000193) >>> 0;
  return h >>> 0;
}
