// Compares Go's math results (stdin JSON from main.go) with V8's Math.*.
import { readFileSync } from "node:fs";
const d = JSON.parse(readFileSync(0, "utf8"));
const bits = (x) => { const b = new DataView(new ArrayBuffer(8)); b.setFloat64(0, x); return b.getBigUint64(0).toString(16).padStart(16, "0"); };
const js = {
  sin: (a) => Math.sin(a), cos: (a) => Math.cos(a), exp: (a) => Math.exp(a / 4), sqrt: (a) => Math.sqrt(Math.abs(a)),
  atan: (a) => Math.atan(a), log: (a) => Math.log(Math.abs(a)),
  hypot: (a, b) => Math.hypot(a, b), atan2: (a, b) => Math.atan2(a, b), pow: (a, b) => Math.pow(Math.abs(a), b / 10),
};
for (const [k, f] of Object.entries(js)) {
  let diff = 0, maxUlp = 0n;
  d.in.forEach(([a, b], i) => {
    const g = d.go[k][i], j = bits(f(a, b));
    if (g !== j) { diff++; const u = BigInt("0x" + g) - BigInt("0x" + j); const au = u < 0n ? -u : u; if (au > maxUlp) maxUlp = au; }
  });
  console.log(`${k.padEnd(6)} ${diff}/${d.in.length} differ (${(100 * diff / d.in.length).toFixed(3)}%), max ${maxUlp} ulp`);
}
