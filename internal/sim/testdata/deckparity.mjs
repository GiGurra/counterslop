// Steps bodies standing on boats' decks with web/engine/physics.js (the
// generated internal/sim boat.go StepDeck) for internal/sim's
// TestDeckParityWithJS. Usage: node deckparity.mjs decks.json
import { readFileSync } from "node:fs";
import { createPhysics, newBody } from "../../../web/engine/physics.js";
import { OnHull } from "../../../web/engine/sim.gen.js";

const c = JSON.parse(readFileSync(process.argv[2], "utf8"));
const phys = createPhysics();
phys.setMap(c.map);
const out = c.runs.map((rn) => {
  const b = { pos: [...rn.boat.Pos], vel: [...rn.boat.Vel], yaw: rn.boat.Yaw, yawVel: rn.boat.YawVel };
  const s = newBody([0, 0, 0]);
  s.onGround = true;
  let d = { pos: [...rn.deck.Pos], vel: [...rn.deck.Vel] };
  const rows = [];
  for (const st of rn.steps) {
    s.yaw = st.Yaw;
    phys.deck(b, s, d, { f: st.F, r: st.R, jump: st.Jump, crouch: st.Crouch, walk: false, speed: 1, pitch: 0 }, 0.01);
    if (!OnHull(d.pos[0], d.pos[2])) { d = { pos: [0, 0.1, 0], vel: [0, 0, 0] }; s.onGround = true; } // walked off: back to the middle (as the Go)
    phys.stepBoat(b, { f: st.BF, r: st.BR }, 0.01);
    rows.push([...d.pos, ...d.vel, ...s.pos, s.onGround ? 1 : 0, s.crouched ? 1 : 0]);
  }
  return rows;
});
process.stdout.write(JSON.stringify(out));
