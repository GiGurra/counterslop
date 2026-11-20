// Drop-down developer console on the key left of 1 (event.code "Backquote":
// ` on US layouts, § on Swedish). cvars are the shared settings store.
import { settings, CVARS, coerce } from "./settings.js";

const HIST_KEY = "cs.history", MAX_LINES = 400;

export function createConsole(ctx) {
  const root = document.createElement("div");
  root.className = "console";
  root.innerHTML = `<div class="console-log"></div><div class="console-line"><span>]</span><input spellcheck="false" autocomplete="off"></div>`;
  document.body.appendChild(root);
  const logEl = root.querySelector(".console-log"), inp = root.querySelector("input");

  let open = false, hist = loadHist(), hi = hist.length;

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
    if (e.code === "Backquote") {
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
      print("commands:", "dim");
      for (const [n, [h]] of Object.entries(COMMANDS)) print(`  ${n.padEnd(12)} ${h}`);
      print("cvars (type a name to see it, name + value to set it):", "dim");
      for (const k of Object.keys(CVARS)) print(`  ${k.padEnd(12)} ${CVARS[k].help}`);
    }],
    cvarlist: ["show every cvar and its value", () => Object.keys(CVARS).forEach((k) => print(fmt(k)))],
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
      print(`map: ${s.map}   connected: ${s.connected ? "yes" : "no"}   ping: ${s.ping ?? "-"} ms`);
      if (s.openAdmin) print("No admin password set: everyone is admin on this experimental server", "warn");
      print(`# id   name`, "dim");
      for (const p of s.players) print(`# ${String(p.id).padEnd(4)} ${p.name}${p.me ? "  (you)" : ""}`);
    }],
    version: ["client and server build", async () => {
      print(`client: ${ctx.clientVersion}`);
      try { print(`server: ${(await (await fetch("/version", { cache: "no-store" })).text()).trim()}`); }
      catch { print("server: unreachable", "warn"); }
    }],
    bind: ["bind <key> <command>: run a command on a key press", (a) => {
      if (!a.length) return print("bind <key> [command]   e.g. bind f slop", "dim");
      const code = keyCode(a[0]), binds = { ...settings.get().binds };
      if (a.length === 1) return print(binds[code] ? `"${a[0]}" = "${binds[code]}"` : `"${a[0]}" is not bound`);
      if (code === "Backquote") return print("that key opens the console", "warn");
      binds[code] = a.slice(1).join(" ");
      settings.set({ binds });
      print(`bound ${code} → ${binds[code]}`);
    }],
    unbind: ["unbind <key>", (a) => {
      const binds = { ...settings.get().binds };
      delete binds[keyCode(a[0] || "")];
      settings.set({ binds });
    }],
    unbindall: ["remove every bind", () => settings.set({ binds: {} })],
    bot_add: ["add a target-practice bot (wanders, never shoots, never dies)", () => ctx.bot("add")],
    bot_kick: ["remove all bots", () => ctx.bot("kick")],
    bot_freeze: ["bot_freeze 0|1: stop the bots where they stand", (a) => ctx.bot("freeze", a[0] === "0" ? "0" : "1")],
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
    announce_kill: ["announce_kill [hs] [streak]: fake a kill for the announcer (hs = headshot)", (a) => {
      const hs = a.includes("hs"), n = a.find((x) => /^\d+$/.test(x));
      print(`announcer: ${ctx.announceKill({ headshot: hs, streak: n ? +n : undefined }).join(", ") || "(nothing)"}`, "dim");
    }],
    jump: ["one jump press (bind it, e.g. bind mwheeldown jump: one press per wheel notch)", () => ctx.jump?.()],
    slop: ["release the slop", () => { ctx.slop(); print("*squelch*"); }],
    admin_login: ["admin_login <password>: unlock admin commands for this session", (a) => ctx.send({ t: "admin_login", pw: a.join(" ") })],
    admin_logout: ["drop admin rights", () => ctx.send({ t: "admin_logout" })],
    rcon: ["rcon <command> [args]: run an admin command on the server (rcon help)", (a) => ctx.send({ t: "rcon", cmd: a[0] || "help", args: a.slice(1) })],
    noclip: ["no", () => print("you are already clipping. emotionally.")],
    quit: ["you can't quit slop", () => print("there is no escape from de_slop. (close the tab)")],
  };

  // Typed without "rcon": forwarded to the server as admin commands.
  const SERVER_CMDS = ["kick", "ban", "changelevel", "restart", "say_admin", "mp_friendlyfire", "mp_roundtime", "mp_freezetime", "sv_gravity", "sv_cheats"];
  for (const c of SERVER_CMDS) COMMANDS[c] = [`admin: ${c} (same as rcon ${c})`, (a) => ctx.send({ t: "rcon", cmd: c, args: a })];

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
  return { print, exec, toggle, get open() { return open; } };
}

// "w" -> KeyW, "5" -> Digit5, "space" -> Space, anything else as given (F1, ShiftLeft, ...).
export function keyCode(name) {
  if (/^[a-z]$/i.test(name)) return "Key" + name.toUpperCase();
  if (/^[0-9]$/.test(name)) return "Digit" + name;
  const special = { mwheelup: "MWheelUp", mwheeldown: "MWheelDown", space: "Space", tab: "Tab", ctrl: "ControlLeft", shift: "ShiftLeft", alt: "AltLeft", enter: "Enter" };
  return special[name.toLowerCase()] || name;
}

function safeJson(x) { try { return JSON.stringify(x); } catch { return String(x); } }
function loadHist() { try { return JSON.parse(localStorage.getItem(HIST_KEY) || "[]"); } catch { return []; } }
function saveHist(h) { try { localStorage.setItem(HIST_KEY, JSON.stringify(h)); } catch {} }
