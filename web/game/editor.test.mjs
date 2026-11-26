import { test } from "node:test";
import assert from "node:assert/strict";
import { createEditor, TOOLS } from "./editor.js";

// A floor (box 0, top at y 0) and a crate (box 1); pick: a ray straight down
// hits the floor, one along -z at crate height hits the crate's +z face.
function setup() {
  const map = { boxes: [{ min: [-20, -1, -20], max: [20, 0, 20], kind: "concrete" }, { min: [2, 0, -6], max: [4, 2, -4], kind: "crate" }], teamSpawns: { vibe: [[1, 0, 1]], artisan: [] }, buyZones: { vibe: [{ min: [-2, -0.5, -2], max: [2, 3, 2] }] }, items: [{ weapon: "prompt", pos: [5, 0.1, 5] }], sites: [] };
  const sent = [];
  let hit = null;
  const ed = createEditor({
    send: (cmd, args) => sent.push([cmd, ...args.map((a) => JSON.parse(a))]),
    pick: () => hit, map: () => map, materials: ["concrete", "crate", "metal"], weapons: ["prompt", "stub"],
  });
  ed.setActive(true);
  const aim = (h, eye = [0, 5, 0], dir = [0, -1, 0]) => { hit = h; ed.frame(eye, dir); };
  return { map, sent, ed, aim, ops: () => sent.map((s) => s[1]?.op ?? s[0]) };
}
const floorAt = (x, z) => ({ i: 0, t: 5, n: [0, 1, 0], x, z });

test("inactive: takes no keys and draws nothing", () => {
  const { ed } = setup();
  ed.setActive(false);
  assert.equal(ed.key("Mouse0", true), false);
  assert.deepEqual(ed.parts(), []);
  assert.equal(ed.hud(), null);
});

test("the box tool: two clicks on the floor, snapped, the height from the wheel, the material and ramp picked", () => {
  const s2 = setup();
  s2.ed.key("Digit2", true); s2.ed.key("MWheelUp", true); s2.ed.key("BracketRight", true); s2.ed.key("KeyR", true);
  s2.aim({ i: 0, t: 5, n: [0, 1, 0] }, [0.13, 5, 0.2]);
  s2.ed.key("Mouse0", true); s2.ed.key("Mouse0", false);
  assert.ok(s2.ed.parts().length > 0);
  s2.aim({ i: 0, t: 5, n: [0, 1, 0] }, [3.1, 5, 1.9]);
  assert.ok(s2.ed.parts().some((p) => p.color[1] === 1 && p.color[0] === 0.3), "the preview");
  s2.ed.key("Mouse0", true);
  assert.deepEqual(s2.sent, [["map_op", { op: "box_add", box: { min: [0, 0, 0], max: [3, 2.5, 2], kind: "crate", ramp: 1 } }]]);
});

test("select, drag a face to resize (one op on release), Shift+drag to move, nudge, ramp, material, duplicate, delete", () => {
  const s = setup();
  const crateFace = { i: 1, t: 10, n: [0, 0, 1] }; // the crate's +z face (z -4), looked at along -z from z 6 at y 1
  s.aim(crateFace, [3, 1, 6], [0, 0, -1]);
  s.ed.key("Mouse0", true); s.ed.key("Mouse0", false);
  assert.equal(s.ed.state.sel, 1);
  assert.equal(s.sent.length, 0);
  // grab the +z face and pull it out 1.5 m: the view ray now passes z -2.5 (looking down from above it)
  s.ed.key("Mouse0", true);
  assert.ok(s.ed.state.drag);
  s.aim(crateFace, [3, 6, -2.5], [0, -1, 0]);
  assert.equal(s.ed.hud().drag, "resize");
  s.ed.key("Mouse0", false);
  assert.deepEqual(s.sent.pop(), ["map_op", { op: "box_resize", i: 1, min: [2, 0, -6], max: [4, 2, -2.5] }]);
  // Shift: move it instead
  s.aim(crateFace, [3, 1, 6], [0, 0, -1]);
  s.ed.key("Mouse0", true, { shift: true });
  s.aim(crateFace, [3, 6, -3], [0, -1, 0]);
  s.ed.key("Mouse0", false);
  assert.deepEqual(s.sent.pop(), ["map_op", { op: "box_move", i: 1, by: [0, 0, 1] }]);
  // a drag that goes nowhere sends nothing
  s.aim(crateFace, [3, 1, 6], [0, 0, -1]);
  s.ed.key("Mouse0", true); s.ed.key("Mouse0", false);
  assert.equal(s.sent.length, 0);
  // looking along -z: ArrowUp nudges -z by a snap, PageUp +y
  s.ed.key("ArrowUp", true); s.ed.key("PageUp", true); s.ed.key("ArrowRight", true);
  assert.deepEqual(s.sent.splice(0).map((x) => x[1].by), [[0, 0, -0.5], [0, 0.5, 0], [0.5, 0, 0]]);
  s.ed.key("KeyR", true); s.ed.key("BracketRight", true); s.ed.key("KeyC", true);
  assert.deepEqual(s.sent.splice(0).map((x) => x[1]), [
    { op: "box_ramp", i: 1, ramp: 1 }, { op: "box_material", i: 1, kind: "crate" },
    { op: "box_add", box: { min: [2, 2, -6], max: [4, 4, -4], kind: "crate" } },
  ]);
  assert.equal(s.ed.hud().sel.kind, "crate");
  assert.ok(s.ed.parts().length > 30, "outline and handles");
  s.ed.key("Delete", true);
  assert.deepEqual(s.sent.pop(), ["map_op", { op: "box_delete", i: 1 }]);
  assert.equal(s.ed.state.sel, -1);
  // a click on nothing deselects; right click too
  s.ed.state.sel = 1;
  s.ed.key("Mouse2", true);
  assert.equal(s.ed.state.sel, -1);
});

test("zones: two corners paint a site or the next buy zone, 0.5 m under the floor to 3.5 m over it", () => {
  const s = setup();
  s.ed.key("Digit3", true);
  s.aim(floorAt(), [0, 5, 0]); s.ed.key("Mouse0", true);
  s.aim(floorAt(), [6, 5, 4]); s.ed.key("Mouse0", true);
  s.ed.key("KeyZ", true); s.ed.key("KeyZ", true); // site B -> buy zone (vibe)
  s.aim(floorAt(), [0, 5, 0]); s.ed.key("Mouse0", true);
  s.ed.key("Mouse2", true); // cancelled
  assert.equal(s.ed.state.anchor, null);
  s.ed.key("Mouse0", true);
  s.aim(floorAt(), [2, 5, 2]); s.ed.key("Mouse0", true);
  assert.deepEqual(s.sent.map((x) => x[1]), [
    { op: "zone_set", kind: "site", name: "A", min: [0, -0.5, 0], max: [6, 3.5, 4] },
    { op: "zone_set", kind: "buy", team: "vibe", i: 1, min: [0, -0.5, 0], max: [2, 3.5, 2] },
  ]);
});

test("spawns and items: click adds for the team / weapon picked, right click deletes the nearest", () => {
  const s = setup();
  s.ed.key("Digit4", true); s.ed.key("KeyT", true); s.ed.key("KeyT", true); // artisan, then vibe again
  s.aim(floorAt(), [1.2, 5, 1.1]); s.ed.key("Mouse0", true); s.ed.key("Mouse2", true);
  s.ed.key("Digit5", true); s.ed.key("BracketRight", true);
  s.aim(floorAt(), [5, 5, 5]); s.ed.key("Mouse0", true); s.ed.key("Mouse2", true);
  assert.deepEqual(s.sent.map((x) => x[1]), [
    { op: "spawn_add", team: "vibe", pos: [1, 0, 1] }, { op: "spawn_delete", team: "vibe", i: 0 },
    { op: "item_add", weapon: "stub", pos: [5, 0.1, 5] }, { op: "item_delete", i: 0 },
  ]);
});

test("session keys: undo, redo, save (Ctrl), playtest, the snap; keys it doesn't use pass", () => {
  const s = setup();
  s.ed.key("KeyZ", true, { ctrl: true }); s.ed.key("KeyY", true, { ctrl: true }); s.ed.key("KeyS", true, { ctrl: true }); s.ed.key("KeyP", true);
  assert.deepEqual(s.sent.map((x) => x[0]), ["map_undo", "map_redo", "map_save", "map_playtest"]);
  s.ed.key("KeyG", true);
  assert.equal(s.ed.state.snap, 1);
  assert.equal(s.ed.key("KeyW", true), false); // flying
  assert.equal(s.ed.key("KeyQ", true, { ctrl: true }), false);
  assert.equal(s.ed.key("KeyW", false), false);
  assert.deepEqual(TOOLS, ["select", "box", "zone", "spawn", "item", "hint"]);
  s.ed.state.sel = 5;
  s.ed.mapChanged();
  assert.equal(s.ed.state.sel, -1);
  assert.equal(s.ed.hud().boxes, 2);
});

test("presence: our cursor, hover and selection as edit-op refs; the others' drawn in their colours", () => {
  const s = setup();
  s.aim({ i: 1, t: 10, n: [0, 0, 1] }, [3.1, 1, 6], [0, 0, -1]);
  assert.deepEqual(s.ed.presence(), { at: [3, 1, -4], hover: { kind: "box", i: 1 }, sel: [], tool: "select" });
  s.ed.key("Mouse0", true); s.ed.key("Mouse0", false);
  assert.deepEqual(s.ed.presence().sel, [{ kind: "box", i: 1 }]);
  const before = s.ed.parts().length;
  s.ed.setEditors([
    { id: 7, name: "me", at: [0, 0, 0], sel: [{ kind: "box", i: 0 }] }, // us: not drawn
    { id: 3, name: "bob", at: [1, 0, 1], pos: [0, 10, 0], hover: { kind: "spawn", team: "vibe", i: 0 }, sel: [{ kind: "box", i: 1 }, { kind: "zone", zone: "buy", team: "vibe", i: 0 }, { kind: "item", i: 0 }, { kind: "zone", zone: "site", name: "A" }, { kind: "nav", name: "x" }, { kind: "box", i: 99 }], tool: "box" },
    { id: 0, name: "mcp", mcp: true, at: [2, 0, 2], sel: [] },
  ], 7);
  const added = s.ed.parts().length - before;
  assert.equal(added, 1 + 1 + 12 + 12 * 3 + 1, "bob: cursor, head, the hovered spawn, three outlined selections (the rest aren't on the map); mcp: its cursor");
  assert.deepEqual(s.ed.hud().editors.map((e) => e.name), ["bob", "mcp"]);
});

test("the panel: what the tool makes, the keys, the other editors", async () => {
  const { editorPanel } = await import("../ui/editorpanel.js");
  const flat = (x) => (Array.isArray(x) ? x.map(flat).join("") : x && typeof x === "object" ? flat(x.parts) : String(x ?? ""));
  const html = (strs, ...vals) => ({ parts: strs.flatMap((s, i) => (i < vals.length ? [s, vals[i]] : [s])) });
  const s = setup();
  for (const [k, want] of [["Digit1", "click a box"], ["Digit2", "2 m tall"], ["Digit3", "site A"], ["Digit4", "a vibe spawn"], ["Digit5", "prompt"]]) {
    s.ed.key(k, true);
    assert.match(flat(editorPanel(html, s.ed.hud())), new RegExp(want));
  }
  s.ed.key("Digit1", true); s.ed.state.sel = 1;
  s.ed.setEditors([{ id: 0, name: "mcp", mcp: true, tool: "box" }], 2);
  const out = flat(editorPanel(html, s.ed.hud()));
  assert.match(out, /box 1: crate, 2 × 2 × 2 m/);
  assert.match(out, /mcp \(agent\): box/);
});

test("presence: an admin who sent nothing yet (age -1, no aim) gets the head marker alone; the agent is cyan", () => {
  const s = setup();
  const before = s.ed.parts().length;
  s.ed.setEditors([{ id: 0, name: "claude", mcp: true, at: [1, 0, 1], sel: [{ kind: "box", i: 1 }], age: 0 }, { id: 4, name: "carol", age: -1, at: null, hover: null, sel: [], pos: [0, 0, 0] }], 9);
  const added = s.ed.parts().slice(before);
  assert.equal(added.length, 1 + 12 + 1, "claude: cursor + selection; carol: head");
  assert.deepEqual(added[0].color, [0, 0.9, 1]);
  assert.deepEqual(s.ed.hud().editors.map((e) => [e.name, e.mcp]), [["claude", true], ["carol", false]]);
});

test("nav hints (tool 6): click adds one, Shift+click a lane, L flips the nearest's lane, right click deletes it; drawn", () => {
  const s = setup();
  s.map.navHints = [{ pos: [3, 0, 3] }, { pos: [8, 0, 8], lane: true }];
  s.ed.key("Digit6", true);
  assert.equal(s.ed.state.tool, "hint");
  s.aim(floorAt(), [1.1, 5, 1.2]); s.ed.key("Mouse0", true);
  s.ed.key("Mouse0", true, { shift: true });
  s.aim(floorAt(), [3.2, 5, 3.1]); s.ed.key("KeyL", true); s.ed.key("Mouse2", true);
  assert.deepEqual(s.sent.map((x) => x[1]), [
    { op: "hint_add", pos: [1, 0, 1] }, { op: "hint_add", pos: [1, 0, 1], lane: true },
    { op: "hint_lane", i: 0, lane: true }, { op: "hint_delete", i: 0 },
  ]);
  assert.equal(s.ed.hud().hints, 2);
  s.ed.key("Digit1", true);
  assert.equal(s.ed.key("KeyL", true), false); // not the hint tool: L passes
  const lanes = s.ed.parts().filter((p) => p.color === undefined || p.color[0] === 1 && p.color[1] === 0.55);
  assert.equal(lanes.length, 2, "the lane hint: a post and a flag");
  s.ed.setEditors([{ id: 3, name: "bob", sel: [{ kind: "hint", i: 1 }] }], 1);
  assert.ok(s.ed.parts().length > 0);
});
