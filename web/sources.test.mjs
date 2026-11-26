// Repository hygiene for the client: no file under web/ carries merge
// conflict markers (a committed "<<<<<<< HEAD" in style.css silently
// dropped the rule after it: the car badge lost its place on screen).
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const SOURCE = /\.(js|mjs|css|html|json)$/;
const MARKER = /^(<{7}|={7}|>{7})( |$)/m;

function* files(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) yield* files(p);
    else if (SOURCE.test(name)) yield p;
  }
}

test("no merge conflict markers in the client's sources", () => {
  const bad = [];
  for (const f of files(root)) {
    const m = readFileSync(f, "utf8").match(MARKER);
    if (m) bad.push(`${f.slice(root.length + 1)}: ${JSON.stringify(m[0])}`);
  }
  assert.deepEqual(bad, []);
});
