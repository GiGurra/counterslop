// The map editor's deltas (internal/game mapedit.go): every edit reaches the
// client as patches on the map's JSON, applied here exactly as the server's
// patchDoc applies them (pinned by testdata/mappatch_golden.json:
// mappatch.test.mjs), so the client holds the server's map.
//
// A patch: at path p (a field, or a field of a field: ["teamSpawns","vibe"])
// a = set | ins | del the array element i, the object key k, or (neither) the
// field itself, with the value v. Missing objects and arrays on the path are
// made. Throws on a bad patch (the caller asks for the whole map: mapget).

const clone = (v) => (v === undefined ? v : JSON.parse(JSON.stringify(v)));

export function applyPatch(doc, patch) {
  const path = patch.p || [];
  if (!path.length) throw new Error("patch without a path");
  let parent = doc;
  for (const k of path.slice(0, -1)) {
    if (!parent[k] || typeof parent[k] !== "object" || Array.isArray(parent[k])) parent[k] = {};
    parent = parent[k];
  }
  const field = path[path.length - 1], v = clone(patch.v);
  if (patch.i !== undefined && patch.i !== null) {
    const arr = Array.isArray(parent[field]) ? parent[field] : [];
    const i = patch.i;
    const bad = () => { throw new Error(`${field}[${i}]: out of range (${arr.length})`); };
    if (patch.a === "set") { if (i < 0 || i >= arr.length) bad(); arr[i] = v; }
    else if (patch.a === "ins") { if (i < 0 || i > arr.length) bad(); arr.splice(i, 0, v); }
    else if (patch.a === "del") { if (i < 0 || i >= arr.length) bad(); arr.splice(i, 1); }
    else throw new Error(`patch action ${patch.a}`);
    parent[field] = arr;
  } else if (patch.k) {
    if (!parent[field] || typeof parent[field] !== "object" || Array.isArray(parent[field])) parent[field] = {};
    if (patch.a === "set") parent[field][patch.k] = v;
    else if (patch.a === "del") delete parent[field][patch.k];
    else throw new Error(`patch action ${patch.a}`);
  } else if (patch.a === "set") parent[field] = v;
  else if (patch.a === "del") delete parent[field];
  else throw new Error(`patch action ${patch.a}`);
  return doc;
}

// applyPatches: all of ps to doc (in place).
export function applyPatches(doc, ps) {
  for (const p of ps) applyPatch(doc, p);
  return doc;
}
