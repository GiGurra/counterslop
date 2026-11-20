// SlopVox entry point: render a phoneme string through a voice preset to samples.
// Pure (no WebAudio), so Node tests and OfflineAudioContext checks get the same output.
//
// voice = { pitch, rate, shift, robot, excite, fall, tilt, breath, seed,   (see slopvox DEFAULTS)
//           layers: [{ pitch: 1, gain: 1, shift? }],  takes at pitch multiples, FX bus only
//           stutter: { rise, gap, cut },              how "~N" marks repeat
//           drive: 0..1, crush: { bits, hold },       FX bus effects
//           fxMix: 0..1,                              FX bus level under the dry voice
//           reverb: { seconds, decay, wet } }         a send at playback (gen/synth/fx.js)
//
// The dry voice stays on top so the words are clear: layer 0 rendered clean, plus a
// parallel FX bus (all layers mixed, driven, crushed) mixed under it at fxMix.
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
  const st = (x) => { // stutters back to front so earlier sample offsets stay valid
    for (const m of [...marks].sort((a, b) => b.s0 - a.s0)) x = stutter(x, sr, m, voice.stutter);
    return x;
  };
  const dry = st(Float32Array.from(parts[0].x));
  const fxMix = voice.fxMix ?? 0;
  if (fxMix <= 0) return { samples: normalize(dry, 0.85), sampleRate: sr };
  let fx = st(normalize(mix(parts), 0.9));
  fx = drive(fx, voice.drive || 0);
  if (voice.crush) fx = bitcrush(fx, voice.crush.bits, voice.crush.hold);
  normalize(fx, 0.9);
  return { samples: normalize(mix([{ x: dry, gain: 1 }, { x: fx, gain: fxMix }]), 0.85), sampleRate: sr };
}
