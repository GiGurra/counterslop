// Map previews kept in this browser (IndexedDB), so the Maps screen shows the
// ones drawn before at once. An entry is the JPEG the worker drew, keyed by
// the page's build, the map's name, a hash of the map's data (a custom map
// edited or hot-reloaded is drawn again) and the preview size; other builds'
// entries are deleted when the cache opens. Storage may be blocked (a private
// window, cleared site data): every access is wrapped and a failure means
// "not cached", never an error.

const DB = "counterslop-previews", STORE = "previews";

// Pure: FNV-1a (32-bit) of a string, as 8 hex digits.
export function hashString(s) {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = Math.imul(h, 0x01000193); }
  return (h >>> 0).toString(16).padStart(8, "0");
}

// Pure: the key of one preview.
export const previewKey = (build, name, hash, w, h) => `${build}|${name}|${hash}|${w}x${h}`;
const parse = (k) => { const [build, name] = String(k).split("|"); return { build, name }; };

// Pure: the keys to delete: every other build's; with `fresh` (a key about to
// be stored), this build's other entries for the same map (its older data).
export function staleKeys(keys, build, fresh = null) {
  const f = fresh && parse(fresh);
  return keys.filter((k) => {
    const p = parse(k);
    if (p.build !== build) return true;
    return !!f && p.name === f.name && k !== fresh;
  });
}

const req = (r) => new Promise((resolve, reject) => { r.onsuccess = () => resolve(r.result); r.onerror = () => reject(r.error); });

// The cache for this page's build. idb: the IndexedDB factory (globalThis.indexedDB).
export function openPreviewCache(build, idb = globalThis.indexedDB) {
  const stats = { hits: 0, misses: 0, stores: 0, evicted: 0, errors: 0 };
  let db = null;
  const open = () => (db ??= (async () => {
    if (!idb || !build) return null;
    const r = idb.open(DB, 1);
    r.onupgradeneeded = () => r.result.createObjectStore(STORE);
    const d = await req(r);
    d.onversionchange = () => d.close(); // another tab upgrades: let it
    return d;
  })().catch(() => { stats.errors++; return null; }));
  const tx = async (mode, fn) => {
    const d = await open();
    if (!d) return null;
    const t = d.transaction(STORE, mode);
    const done = new Promise((resolve, reject) => { t.oncomplete = resolve; t.onerror = t.onabort = () => reject(t.error); });
    const out = await fn(t.objectStore(STORE));
    await done;
    return out;
  };
  async function remove(keys) {
    if (!keys.length) return;
    await tx("readwrite", (s) => { for (const k of keys) s.delete(k); });
    stats.evicted += keys.length;
  }
  return {
    // The stored Blob, or null (not cached, or storage blocked).
    async get(key) {
      try {
        const b = await tx("readonly", (s) => req(s.get(key)));
        if (b instanceof Blob && b.size) { stats.hits++; return b; }
      } catch { stats.errors++; }
      stats.misses++;
      return null;
    },
    // Store a finished preview (replacing this map's older entries).
    async put(key, blob) {
      try { // one transaction: a page closed right after still keeps it
        const n = await tx("readwrite", async (s) => {
          s.put(blob, key);
          const old = staleKeys((await req(s.getAllKeys())) || [], build, key);
          for (const k of old) s.delete(k);
          return old.length;
        });
        if (n != null) { stats.stores++; stats.evicted += n; }
      } catch { stats.errors++; }
    },
    // Delete other builds' entries (once, when the Maps screen first opens).
    async evict() {
      try { await remove(staleKeys((await tx("readonly", (s) => req(s.getAllKeys()))) || [], build)); } catch { stats.errors++; }
    },
    stats,
  };
}
