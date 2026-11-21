// Ray-traces the player model for each {st, o, d} with web/gen/boxmodel.js
// rayModel (the generated Go RayModel) and the client's registry helpers.
// Usage: node testdata/rays.mjs rays.json   (internal/game TestHitParityWithJS)
import { readFileSync } from "node:fs";
import { rayModel, partsFor } from "../../../web/gen/boxmodel.js";
const model = JSON.parse(readFileSync(new URL("../../../web/assets/models/player.json", import.meta.url), "utf8"));
const rays = JSON.parse(readFileSync(process.argv[2], "utf8"));
process.stdout.write(JSON.stringify({
  hits: rays.map(({ st, o, d }) => rayModel(model, st, o, d)),
  parts: rays.map(({ st }) => partsFor(model, st).map((p) => p.name)),
}));
