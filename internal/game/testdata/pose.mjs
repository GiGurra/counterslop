// Prints world corners of every part for each state, using web/gen/boxmodel.js.
// Usage: node testdata/pose.mjs states.json   (internal/game hitbox parity test)
import { readFileSync } from "node:fs";
import { partCorners } from "../../../web/gen/boxmodel.js";
const model = JSON.parse(readFileSync(new URL("../../../web/assets/models/player.json", import.meta.url), "utf8"));
const states = JSON.parse(readFileSync(process.argv[2], "utf8"));
process.stdout.write(JSON.stringify(states.map((st) => model.parts.map((pt) => partCorners(pt, st)))));
