// A sim/case's map for the node runners (cases.test.mjs, casetrace.mjs): the
// golden JSON the Go map tests pin, by the case's "map" (default de_slop).
import { readFileSync } from "node:fs";

const maps = {};
export const mapFor = (name = "de_slop") =>
  (maps[name] ??= JSON.parse(readFileSync(new URL(`../../internal/content/maps/testdata/${name}.json`, import.meta.url))));
