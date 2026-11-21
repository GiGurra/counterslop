// Per-tick cost of the movement step: node web/engine/physics.bench.mjs [old.mjs]
// (old.mjs: another physics.js to compare, e.g. git show <rev>:web/engine/physics.js > /tmp/old.mjs).
import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { createPhysics, newBody } from "./physics.js";

const map = JSON.parse(readFileSync(new URL("../../internal/content/maps/testdata/de_slop.json", import.meta.url)));
const spawns = map.spawns.map((s) => s.pos ?? s);

function run(mod, ticks) {
  const phys = mod.createPhysics();
  phys.setMap(map);
  let seed = 1;
  const rnd = () => ((seed = (seed * 1103515245 + 12345) % 2147483648) / 2147483648);
  const bodies = spawns.map((s) => mod.newBody(s));
  let inp = { f: 1, r: 0, walk: false, jump: false, crouch: false, speed: 1 };
  const t0 = process.hrtime.bigint();
  for (let t = 0; t < ticks; t++) {
    if (t % 40 === 0) inp = { f: Math.floor(rnd() * 3) - 1, r: Math.floor(rnd() * 3) - 1, walk: rnd() < 0.2, jump: false, crouch: rnd() < 0.2, speed: 1 };
    const b = bodies[t % bodies.length];
    if (t % 40 === 0) b.yaw = rnd() * 6.28;
    phys.step(b, { ...inp, jump: rnd() < 0.03 }, 0.01);
  }
  return Number(process.hrtime.bigint() - t0) / ticks;
}

const mods = { generated: { createPhysics, newBody } };
if (process.argv[2]) mods.old = await import(pathToFileURL(process.argv[2]).href);
const N = 200000;
for (const m of Object.values(mods)) run(m, N); // warm up
for (let round = 0; round < 3; round++) {
  for (const [name, m] of Object.entries(mods)) console.log(`${name.padEnd(10)} ${run(m, N).toFixed(0)} ns/tick (${map.boxes.length} boxes)`);
}
