import { test } from "node:test";
import assert from "node:assert/strict";
import { boatHud, boatWords } from "./boatseat.js";
import { SeatDeck } from "./shared.gen.js";

const boat = (driven) => ({ pos: [0, -0.6, 0], vel: [0, 0, 0], yaw: 0, yawVel: 0, driven });

test("boatHud: driving, a passenger's seat (the tiller free or not), on the deck, near a boat on foot, else nothing", () => {
  assert.deepEqual(boatHud({ ride: 1, seat: 0, boat: boat(true), pos: [0, 0, 0] }, []), { role: "driver", seat: 0, tiller: false });
  assert.deepEqual(boatHud({ ride: 1, seat: 2, boat: boat(true), pos: [0, 0, 0] }, []), { role: "passenger", seat: 2, tiller: false });
  assert.deepEqual(boatHud({ ride: 1, seat: 1, boat: boat(false), pos: [0, 0, 0] }, []), { role: "passenger", seat: 1, tiller: true });
  assert.equal(boatHud({ ride: 1, seat: SeatDeck, boat: boat(false), pos: [0, 0, 0] }, []).role, "deck");
  const boats = [{ id: 1, p: [-12, -0.6, 8] }];
  assert.equal(boatHud({ ride: 0, pos: [-12, 0, 10.5] }, boats).role, "near");
  assert.equal(boatHud({ ride: 0, pos: [-12, 0, 11] }, boats), null);
  assert.equal(boatHud({ ride: 0, pos: [-12, 5, 9] }, boats), null, "a deck above isn't near");
  assert.equal(boatHud({ ride: 1, seat: 0, boat: boat(true), dead: true, pos: [0, 0, 0] }, boats), null);
  assert.equal(boatHud(null, boats), null);
});

test("boatWords: the role and the keys from the binds", () => {
  const k = (c) => ({ "+seat": "X", "+use": "E", "+forward": "W", "+back": "S", "+moveleft": "A", "+moveright": "D" })[c];
  assert.equal(boatWords(null, k), null);
  assert.deepEqual(boatWords({ role: "driver" }, k).keys, ["W/S throttle", "A/D tiller", "X passenger seat", "E stand up"]);
  assert.deepEqual(boatWords({ role: "passenger", seat: 1, tiller: true }, k), { title: "PASSENGER · SEAT 1", tag: "along for the ride", keys: ["X take the tiller", "E stand up"] });
  assert.equal(boatWords({ role: "passenger", seat: 2, tiller: false }, k).keys[0], "the tiller is taken");
  assert.deepEqual(boatWords({ role: "deck" }, k).keys, ["E sit down", "walk off the side to leave"]);
  assert.deepEqual(boatWords({ role: "near" }, k), { title: null, tag: "", keys: ["E sit down (dinghy)"] });
});
