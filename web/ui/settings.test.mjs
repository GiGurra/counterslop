import { test } from "node:test";
import assert from "node:assert/strict";

// A localStorage with the player's saved settings, before settings.js loads them.
function storage(saved) {
  const m = new Map(saved ? [["cs.settings", JSON.stringify(saved)]] : []);
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => m.set(k, String(v)), map: m };
}
const fresh = async (saved) => {
  globalThis.localStorage = storage(saved);
  return import(`./settings.js?${Math.random()}`);
};

test("muzzle lights: on by default, the flood off; a software renderer's default is off", async () => {
  const { settings, CVARS } = await fresh();
  assert.equal(settings.get().r_muzzle_light, 1);
  assert.equal(settings.get().r_dynflood, 0);
  assert.equal(CVARS.r_dynflood.def, 0);
  settings.setDefaults({ r_muzzle_light: 0 }); // main.js on SwiftShader / llvmpipe
  assert.equal(settings.get().r_muzzle_light, 0);
  assert.equal(CVARS.r_muzzle_light.def, 0);
  // The player turns them on: saved (it differs from this machine's default).
  settings.set({ r_muzzle_light: 1 });
  assert.equal(JSON.parse(globalThis.localStorage.getItem("cs.settings")).r_muzzle_light, 1);
});

test("muzzle lights: a player's own choice beats the machine's default", async () => {
  const { settings } = await fresh({ _v: 4, r_muzzle_light: 1 });
  settings.setDefaults({ r_muzzle_light: 0 });
  assert.equal(settings.get().r_muzzle_light, 1, "kept");
  const b = await fresh({ _v: 4, r_dynflood: 1 });
  assert.equal(b.settings.get().r_dynflood, 1, "the flood opted into");
  b.settings.setDefaults({ r_muzzle_light: 0 });
  assert.equal(b.settings.get().r_muzzle_light, 0, "never set: the new default");
});
