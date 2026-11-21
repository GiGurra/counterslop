import test from "node:test";
import assert from "node:assert/strict";
import { RADIO, RADIO_BY_ID, renderVoice } from "./index.js";

test("every radio callout renders in the radio voice, 0.3-4 s long, with a wire-safe id", () => {
  assert.ok(RADIO.length >= 9);
  for (const r of RADIO) {
    assert.match(r.id, /^[a-z_]{1,24}$/); // = internal/game/chat.go radioID
    assert.equal(RADIO_BY_ID[r.id], r);
    const v = renderVoice(r.ph, "radio");
    const secs = v.samples.length / v.sampleRate;
    assert.ok(secs > 0.3 && secs < 4, `${r.id}: ${secs.toFixed(2)} s`);
    assert.ok(v.samples.some((x) => Math.abs(x) > 0.05), `${r.id} is silent`);
  }
});
