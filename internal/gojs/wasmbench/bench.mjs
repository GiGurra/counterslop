// Times internal/sim as Go WASM against the generated JS (see main.go).
// node internal/gojs/wasmbench/bench.mjs sim.wasm
import { readFileSync } from "node:fs";
import { execSync } from "node:child_process";
import { createPhysics, newBody } from "../../../web/engine/physics.js";

const goroot = execSync("go env GOROOT").toString().trim();
await import(`${goroot}/lib/wasm/wasm_exec.js`);

const map = JSON.parse(readFileSync(new URL("../../content/maps/testdata/de_slop.json", import.meta.url)));
const t0 = performance.now();
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(process.argv[2]), go.importObject);
go.run(instance);
const tInit = performance.now() - t0;
globalThis.simSetMap(map.boxes);
const ex = instance.exports;
const mem = () => new Float64Array(ex.mem.buffer, ex.statePtr(), 9);

const N = 200000;
const inputs = [];
let seed = 1;
const rnd = () => ((seed = (seed * 1103515245 + 12345) % 2147483648) / 2147483648);
for (let t = 0; t < 4000; t++) inputs.push({ f: Math.floor(rnd() * 3) - 1, r: Math.floor(rnd() * 3) - 1, walk: rnd() < 0.2, jump: rnd() < 0.03, crouch: rnd() < 0.1 });
const spawn = [0, 0.01, 0];

function timeIt(name, fn) {
  fn(N / 4); // warm up
  const s = performance.now();
  fn(N);
  console.log(`${name.padEnd(28)} ${(((performance.now() - s) * 1e6) / N).toFixed(0)} ns/tick`);
}

const phys = createPhysics();
phys.setMap(map);
timeIt("generated JS", (n) => {
  const b = newBody(spawn);
  for (let t = 0; t < n; t++) { if (t % 4000 === 0) b.pos = [...spawn]; phys.step(b, inputs[t % 4000], 0.01); }
});
timeIt("wasm, object via syscall/js", (n) => {
  const b = newBody(spawn);
  for (let t = 0; t < n; t++) { if (t % 4000 === 0) b.pos = [...spawn]; globalThis.simStepObj(b, inputs[t % 4000], 0.01); }
});
timeIt("wasm, wasmexport + memory", (n) => {
  const m = mem();
  m.set([...spawn, 0, 0, 0, 0, 0, 0]);
  for (let t = 0; t < n; t++) {
    if (t % 4000 === 0) m.set(spawn);
    const i = inputs[t % 4000];
    ex.step(i.f, i.r, (i.walk ? 1 : 0) | (i.jump ? 2 : 0) | (i.crouch ? 4 : 0), 0.01);
  }
});
console.log(`wasm instantiate + Go runtime start: ${tInit.toFixed(1)} ms`);
process.exit(0);
