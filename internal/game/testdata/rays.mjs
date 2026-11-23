// Ray-traces the player model for each {st, o, d} with web/gen/boxmodel.js
// rayModel (the generated Go RayModel), the gait frames (Model.Frames) and the client's registry helpers.
// Usage: node testdata/rays.mjs rays.json   (internal/game TestHitParityWithJS)
import { readFileSync } from "node:fs";
import { rayModel, partsFor } from "../../../web/gen/boxmodel.js";
import * as G from "../../../web/game/shared.gen.js";
const model = JSON.parse(readFileSync(new URL("../../../web/assets/models/player.json", import.meta.url), "utf8"));
const rays = JSON.parse(readFileSync(process.argv[2], "utf8"));
process.stdout.write(JSON.stringify({
  hits: rays.map(({ st, o, d }) => rayModel(model, st, o, d)),
  parts: rays.map(({ st }) => partsFor(model, st).map((p) => p.name)),
  frames: rays.map(({ st }) => (G.Model_Frames(G.Model_fromJSON(model), { pos: st.p, yaw: st.yaw, pitch: st.pitch, crouch: st.crouch, team: st.team, id: st.id, gait: st.gait ?? null, t: st.t }) ?? []).flatMap((x) => [...x.r, ...x.t])),
}));
