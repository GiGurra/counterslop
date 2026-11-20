// SlopVox entry point: render a phoneme string through a voice preset to samples.
// Pure (no WebAudio), so Node tests and OfflineAudioContext checks get the same output.
//
// voice = { pitch, rate, shift, robot, excite, fall, tilt, breath, seed,   (see slopvox DEFAULTS)
//           layers: [{ pitch: 1, gain: 1, shift? }],  extra takes at pitch multiples, mixed
//           stutter: { rise, gap, cut },              how "~N" marks repeat
//           drive: 0..1, crush: { bits, hold },       sample FX
//           reverb: { seconds, decay, wet } }         applied at playback (gen/synth/fx.js)
import { speak, normalize, DEFAULTS } from "./slopvox.js";
import { stutter, mix, drive, bitcrush } from "./fx.js";

export { speak, parse, DEFAULTS } from "./slopvox.js";
export { PHONEMES } from "./phonemes.js";

export function render(phonemes, voice = {}) {
  const layers = voice.layers || [{ pitch: 1, gain: 1 }];
  let sr = DEFAULTS.sampleRate, marks = [];
  const parts = layers.map((l, i) => {
    const r = speak(phonemes, {
      ...voice, pitch: (voice.pitch ?? DEFAULTS.pitch) * l.pitch,
      shift: (voice.shift ?? 1) * (l.shift ?? 1), seed: (voice.seed ?? 1) + i,
    });
    sr = r.sampleRate; if (i === 0) marks = r.marks;
    return { x: r.samples, gain: l.gain };
  });
  let x = parts.length > 1 ? normalize(mix(parts), 0.9) : parts[0].x;
  // Apply stutters back to front so earlier sample offsets stay valid.
  for (const m of [...marks].sort((a, b) => b.s0 - a.s0)) x = stutter(x, sr, m, voice.stutter);
  x = drive(x, voice.drive || 0);
  if (voice.crush) x = bitcrush(x, voice.crush.bits, voice.crush.hold);
  return { samples: normalize(x, 0.85), sampleRate: sr };
}
