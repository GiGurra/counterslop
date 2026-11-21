// Drop-down developer console on the toggleconsole key, by default the key left
// of 1 (event.code "Backquote": ` on US layouts, § on Swedish). cvars are the
// shared settings store, binds the shared bind store (ui/binds.js).
import { settings, CVARS, coerce } from "./settings.js";
import { ACTIONS, bindKey } from "./binds.js";

const HIST_KEY = "cs.history", MAX_LINES = 400;

export function createConsole(ctx) {
  const root = document.createElement("div");
  root.className = "console";
  root.innerHTML = `<div class="console-log"></div><div class="console-line"><span>]</span><input spellcheck="false" autocomplete="off"></div>`;
  document.body.appendChild(root);
  const logEl = root.querySelector(".console-log"), inp = root.querySelector("input");

  let open = false, hist = loadHist(), hi = hist.length;

  // A help table row: name (highlighted), args (dim), current value, description
  // (wraps within its column; the name never wraps).
  function printRow({ name, args = "", value, desc = "" }) {
    const d = document.createElement("div");
    d.className = "help-row";
    for (const [cls, text] of [["n", name], ["a", args], ["v", value === undefined ? "" : `= ${value}`], ["d", desc]]) {
      const span = document.createElement("span");
      span.className = cls;
      span.textContent = text;
      d.appendChild(span);
    }
    logEl.appendChild(d);
    logEl.scrollTop = logEl.scrollHeight;
  }
  function printHeader(text) { print(text.toUpperCase(), "help-head"); }
  // "name <x> [y]: description" -> {args, desc}; plain text is all description.
  function splitHelp(name, h) {
    const i = h.indexOf(": ");
    if (h.startsWith(name) && i >= name.length) return { args: h.slice(name.length, i).trim(), desc: h.slice(i + 2) };
    return { args: "", desc: h };
  }

  const cvarArgs = (c) => (c.enum ? c.enum.join("|") : c.bool ? "0|1" : c.key ? "<key>" : c.min !== undefined ? `${c.min}..${c.max}` : "<value>");
  function printCvars() {
    printHeader("cvars (name to show, name + value to set)");
    for (const k of Object.keys(CVARS)) {
      const v = settings.get()[k];
      printRow({ name: k, args: cvarArgs(CVARS[k]), value: typeof v === "object" ? "…" : v, desc: CVARS[k].help });
    }
  }
  // Server help sections ({t:"help", sections:[{title, rows:[{name,args,value,desc}]}]}).
  function printSections(sections) {
    for (const s of sections) { printHeader(s.title); for (const r of s.rows) printRow(r); }
  }

  function print(text, kind = "") {
    for (const line of String(text).split("\n")) {
      const d = document.createElement("div");
      if (kind) d.className = kind;
      d.textContent = line;
      logEl.appendChild(d);
    }
    while (logEl.childElementCount > MAX_LINES) logEl.firstChild.remove();
    logEl.scrollTop = logEl.scrollHeight;
  }

  function toggle(want = !open) {
    open = want;
    root.classList.toggle("open", open);
    ctx.onToggle?.(open);
    if (open) {
      document.exitPointerLock?.();
      setTimeout(() => inp.focus(), 0);
    } else inp.blur();
  }

  // Capture phase, before the game's own key handling; preventDefault keeps § / ` out of the input.
  addEventListener("keydown", (e) => {
    if (e.code === (ctx.consoleKey?.() || "Backquote")) {
      e.preventDefault(); e.stopPropagation();
      if (!e.repeat) toggle();
      return;
    }
    if (!open) return;
    if (e.code === "Escape") { e.preventDefault(); toggle(false); }
  }, true);

  inp.addEventListener("keydown", (e) => {
    e.stopPropagation(); // typing must never move the player
    if (e.key === "Enter") {
      const line = inp.value.trim();
      inp.value = "";
      if (!line) return;
      print("] " + line, "echo");
      if (hist[hist.length - 1] !== line) { hist.push(line); hist = hist.slice(-100); saveHist(hist); }
      hi = hist.length;
      exec(line);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (hi > 0) inp.value = hist[--hi];
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      hi = Math.min(hist.length, hi + 1);
      inp.value = hist[hi] ?? "";
    } else if (e.key === "Tab") {
      e.preventDefault();
      complete();
    }
  });

  function names() { return [...Object.keys(COMMANDS), ...Object.keys(CVARS)].sort(); }

  function complete() {
    const v = inp.value.trimStart();
    if (v.includes(" ")) return;
    const m = names().filter((n) => n.startsWith(v.toLowerCase()));
    if (!m.length) return;
    if (m.length === 1) { inp.value = m[0] + " "; return; }
    let pre = m[0];
    for (const n of m) while (!n.startsWith(pre)) pre = pre.slice(0, -1);
    inp.value = pre;
    print(m.join("  "), "dim");
  }

  function fmt(k) {
    const v = settings.get()[k];
    return `"${k}" = "${v}"  (${CVARS[k].help})`;
  }

  function setCvar(k, raw) {
    if (k === "fps_max") { // CS semantics: fps_max 0 = uncapped
      const n = parseFloat(raw);
      if (n === 0) { settings.set({ fps_limit: "off" }); return print("fps_max 0: frame cap off"); }
      const v = coerce(k, raw);
      if (v === undefined) return print(`bad value for ${k}: ${raw}`, "warn");
      settings.set({ fps_max: v, fps_limit: "custom" });
      return print(fmt(k));
    }
    if (CVARS[k].key) raw = keyCode(raw);
    const v = coerce(k, raw);
    if (v === undefined) {
      const c = CVARS[k];
      return print(`bad value for ${k}: ${raw}${c.enum ? ` (one of: ${c.enum.join(", ")})` : ""}`, "warn");
    }
    settings.set({ [k]: v });
    print(fmt(k));
  }

  const COMMANDS = {
    help: ["list commands and cvars", () => {
      const server = new Set(SERVER_CMDS);
      const rows = (pick) => Object.entries(COMMANDS).filter(([n]) => pick(n)).map(([n, [h]]) => ({ name: n, ...splitHelp(n, h) }));
      printHeader("commands");
      rows((n) => !server.has(n) && !n.startsWith("bot_")).forEach(printRow);
      printHeader("server / admin (also: rcon help)");
      rows((n) => server.has(n) || n.startsWith("bot_")).forEach(printRow);
      printCvars();
    }],
    cvarlist: ["show every cvar and its value", () => printCvars()],
    echo: ["print text", (a) => print(a.join(" "))],
    clear: ["clear the console", () => { logEl.textContent = ""; }],
    name: ["name <x>: change your name", (a) => {
      const n = a.join(" ").trim().slice(0, 16);
      if (!n) return print(`name is "${ctx.getName()}"`);
      ctx.setName(n);
      print(`name changed to "${n}"`);
    }],
    connect: ["(re)connect to the server", () => { ctx.connect(); print("connecting…"); }],
    disconnect: ["leave the server", () => { ctx.disconnect(); print("disconnected"); }],
    retry: ["same as connect", () => COMMANDS.connect[1]([])],
    status: ["players and ping", () => {
      const s = ctx.status();
      print(`map: ${s.map}   connected: ${s.connected ? "yes" : "no"}   ping: ${s.ping ?? "-"} ms   friendly fire: ${s.friendlyFire ? "on" : "off"}`);
      if (s.openAdmin) print("No admin password set: everyone is admin on this experimental server", "warn");
      print(`# id   team      name`, "dim");
      for (const p of s.players) print(`# ${String(p.id).padEnd(4)} ${(p.team || "-").padEnd(9)} ${p.name}${p.me ? "  (you)" : ""}`);
    }],
    version: ["client and server build", async () => {
      print(`client: ${ctx.clientVersion}`);
      try { print(`server: ${(await (await fetch("/version", { cache: "no-store" })).text()).trim()}`); }
      catch { print("server: unreachable", "warn"); }
    }],
    jointeam: ["jointeam vibe|artisan|auto|spec: switch sides (respawns you)", (a) => {
      let t = (a[0] || "").toLowerCase();
      if (t === "spectate" || t === "spectator") t = "spec";
      if (!ctx.joinTeam || !["vibe", "artisan", "auto", "spec"].includes(t)) return print("jointeam vibe|artisan|auto|spec", "dim");
      ctx.joinTeam(t);
      print(`switching to ${t}…`);
    }],
    bind: ["bind <key> <command>: e.g. bind f slop, bind mwheeldown +jump, bind mouse4 reload (actions: Options → Controls)", (a) => {
      if (!a.length) return print("bind <key> [command]   e.g. bind f slop · keys: a-z 0-9 space ctrl shift mouse1-5 mwheelup mwheeldown or a KeyboardEvent.code", "dim");
      const code = keyCode(a[0]), binds = settings.get().binds;
      if (a.length === 1) return print(binds[code] ? `"${a[0]}" = "${binds[code]}"` : `"${a[0]}" is not bound`);
      if (code === "Escape") return print("Esc is the menu key", "warn");
      const { binds: next, moved } = bindKey(binds, code, a.slice(1).join(" "));
      settings.set({ binds: next });
      print(`bound ${code} → ${next[code]}${moved ? ` (was ${moved}, now unbound)` : ""}`);
    }],
    unbind: ["unbind <key>", (a) => {
      const binds = { ...settings.get().binds };
      delete binds[keyCode(a[0] || "")];
      settings.set({ binds });
    }],
    unbindall: ["remove every bind", () => settings.set({ binds: {} })],
    bot_add: ["add a target-practice bot (wanders, never shoots, never dies; admin)", () => ctx.bot("add")],
    bot_kick: ["remove all bots (admin)", () => ctx.bot("kick")],
    bot_freeze: ["bot_freeze 0|1: stop the bots where they stand", (a) => ctx.bot("freeze", a[0] === "0" ? "0" : "1")],
    bot_crouch: ["bot_crouch 0|1: keep the bots crouched", (a) => ctx.bot("crouch", a[0] === "0" ? "0" : "1")],
    say_announcer: ["say_announcer <line>: play an announcer line (no argument lists them)", (a) => {
      const lines = ctx.announcerLines();
      if (!a.length) return print(Object.entries(lines).map(([id, l]) => `  ${id.padEnd(18)} ${l.text}`).join("\n"));
      if (!lines[a[0]]) return print(`no line "${a[0]}"; say_announcer lists them`, "warn");
      ctx.vox({ line: a[0] });
    }],
    vox: ["vox [@voice] <phonemes>: speak ARPAbet with SlopVox, e.g. vox HH EH1 L OW1 _ W ER1 L D", (a) => {
      let voice;
      if (a[0]?.startsWith("@")) voice = a.shift().slice(1);
      const ph = a.join(" ").replace(/["']/g, "").trim();
      if (!ph) return print("vox [@announcer|@monster|@taunt] <phonemes>  (~3 before a syllable stutters it)", "dim");
      ctx.vox({ ph, voice });
    }],
    slop: ["release the slop", () => { ctx.slop(); print("*squelch*"); }],
    admin_login: ["admin_login <password>: unlock admin commands for this session", (a) => ctx.send({ t: "admin_login", pw: a.join(" ") })],
    admin_logout: ["drop admin rights", () => ctx.send({ t: "admin_logout" })],
    say: ["say <text>: chat to everyone", (a) => ctx.send({ t: "say", text: a.join(" "), tm: false })],
    say_team: ["say_team <text>: chat to your team", (a) => ctx.send({ t: "say", text: a.join(" "), tm: true })],
    rcon: ["rcon <command> [args]: run an admin command on the server (rcon help)", (a) => ctx.send({ t: "rcon", cmd: a[0] || "help", args: a.slice(1) })],
    maps: ["list the maps (Esc → MAPS shows them with previews)", async () => {
      try {
        const cur = ctx.status().map;
        for (const m of await ctx.maps()) print(`  ${m.name === cur ? "*" : " "} ${m.name.padEnd(13)} ${String(m.maxPlayers).padStart(2)}p  ${m.desc}`, m.name === cur ? "" : "dim");
        print("changelevel <map> to switch (admin)", "dim");
      } catch { print("couldn't load the map list", "warn"); }
    }],
    vote: ["vote <1-3|map>: vote in the map vote (also F1-F3)", (a) => {
      if (!ctx.vote(a[0] || "")) print("no map vote running, or no such option", "warn");
    }],
    noclip: ["no", () => print("you are already clipping. emotionally.")],
    quit: ["you can't quit slop", () => print("there is no escape from de_slop. (close the tab)")],
  };

  // Typed without "rcon": forwarded to the server as admin commands.
  const SERVER_CMDS = ["give", "bot_teleport", "kick", "ban", "changelevel", "mapvote", "mp_maprotation", "restart", "say_admin", "mp_friendlyfire", "mp_roundtime", "mp_freezetime", "mp_respawntime", "mp_respawn", "mp_c4timer", "mp_round_restart_delay", "sv_gravity", "sv_cheats", "bot_add_vibe", "bot_add_artisan", "bot_patrol", "bot_quota", "bot_difficulty", "mp_buytime", "mp_afk_rounds", "mp_autoteambalance"];
  const SERVER_HELP = {
    give: ["<weapon>", "give yourself a weapon"], bot_teleport: ["x y z [yaw]", "move every bot (tests)"],
    kick: ["<name|id>", "kick a player"], ban: ["<name|id> [minutes]", "ban by IP (in memory)"],
    changelevel: ["<map>", "change the map (maps lists them)"], mapvote: ["[seconds|end]", "vote on the next map (end: count it now)"],
    mp_maprotation: ["<map,map,...>|all", "maps the vote offers"], restart: ["", "restart the match"], say_admin: ["<msg>", "announce as admin"],
    mp_friendlyfire: ["0|1", "team damage"], mp_roundtime: ["<seconds>", "round length"], mp_freezetime: ["<seconds>", "buy time"],
    sv_gravity: ["<m/s²>", "gravity"], sv_cheats: ["0|1", "cheats for everyone"],
    bot_patrol: ["x0 z0 x1 z1 | off", "target bots walk back and forth (tests)"],
    bot_add_vibe: ["", "add a playing bot to the Vibe Coders"], bot_add_artisan: ["", "add a playing bot to the Artisans"],
    bot_quota: ["<n>", "playing bots fill the teams up to n players"], bot_difficulty: ["0-3", "playing bots' skill"],
    mp_buytime: ["<seconds>", "buy time after the freeze"],
    mp_afk_rounds: ["<n>|auto", "idle rounds before a player goes to spectators (0 = never)"],
    mp_autoteambalance: ["0|1|auto", "move the newest players off a side 2+ bigger at round start"],
  };
  for (const c of SERVER_CMDS) {
    const [args, desc] = SERVER_HELP[c] || ["", ""];
    COMMANDS[c] = [`${c}${args ? " " + args : ""}: ${desc} (admin)`, (a) => ctx.send({ t: "rcon", cmd: c, args: a })];
  }
  // Every plain bindable action (reload, slot1..3, drop, lastinv, ...) is a
  // console command too: binds run them through exec. Without these, every
  // non-"+" key bind silently does nothing.
  for (const x of ACTIONS) if (x.cmd[0] !== "+" && !COMMANDS[x.cmd]) COMMANDS[x.cmd] = [x.label.toLowerCase(), () => ctx.action?.(x.cmd)];

  function exec(line) {
    for (const part of line.split(";")) {
      const [cmd, ...args] = part.trim().split(/\s+/);
      if (!cmd) continue;
      const k = cmd.toLowerCase();
      if (COMMANDS[k]) { try { COMMANDS[k][1](args); } catch (e) { print(String(e), "warn"); } }
      else if (CVARS[k]) args.length ? setCvar(k, args.join(" ")) : print(fmt(k));
      else print(`Unknown command "${cmd}"`, "warn");
    }
  }

  // Mirror the browser console into the scrollback.
  for (const [lvl, kind] of [["log", ""], ["info", ""], ["warn", "warn"], ["error", "warn"]]) {
    const orig = console[lvl].bind(console);
    console[lvl] = (...a) => { orig(...a); print(a.map((x) => (typeof x === "string" ? x : safeJson(x))).join(" "), kind); };
  }
  addEventListener("error", (e) => print(`error: ${e.message}`, "warn"));

  print("counterslop console. type help. the key left of 1 closes it.", "dim");
  return { print, printSections, exec, toggle, get open() { return open; }, has: (c) => !!COMMANDS[c] || !!CVARS[c],
    commands: () => Object.keys(COMMANDS), serverCommands: () => [...SERVER_CMDS] };
}

// "w" -> KeyW, "5" -> Digit5, "space" -> Space, anything else as given (F1, ShiftLeft, ...).
export function keyCode(name) {
  if (/^[a-z]$/i.test(name)) return "Key" + name.toUpperCase();
  if (/^[0-9]$/.test(name)) return "Digit" + name;
  if (/^mouse\d+$/i.test(name)) return "Mouse" + name.slice(5); // raw event.button: mouse0 = primary, mouse2 = secondary
  const special = { mwheelup: "MWheelUp", mwheeldown: "MWheelDown", space: "Space", tab: "Tab", ctrl: "ControlLeft", shift: "ShiftLeft", alt: "AltLeft", enter: "Enter" };
  return special[name.toLowerCase()] || name;
}

function safeJson(x) { try { return JSON.stringify(x); } catch { return String(x); } }
function loadHist() { try { return JSON.parse(localStorage.getItem(HIST_KEY) || "[]"); } catch { return []; } }
function saveHist(h) { try { localStorage.setItem(HIST_KEY, JSON.stringify(h)); } catch {} }
