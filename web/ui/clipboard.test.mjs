import test from "node:test";
import assert from "node:assert/strict";
import { copyMethod, copyText, legacyCopy, copyFailure, pasteLines, lastLines } from "./clipboard.js";

// A tiny document: what legacyCopy touches (createElement, body.append, execCommand, activeElement).
function fakeDoc(execResult = true) {
  const doc = { calls: [], focused: null, body: { append: (el) => doc.calls.push(["append", el.value]) } };
  doc.activeElement = { focus: () => doc.calls.push(["refocus"]) };
  doc.createElement = () => ({ value: "", style: {}, setAttribute() {}, select() { doc.calls.push(["select", this.value]); }, remove() { doc.calls.push(["remove"]); } });
  doc.execCommand = (c) => { doc.calls.push(["exec", c]); if (execResult instanceof Error) throw execResult; return execResult; };
  return doc;
}

test("copyMethod: the API in a secure context, the legacy copy over plain http", () => {
  assert.equal(copyMethod({ clipboard: { writeText: async () => {} } }), "api");
  assert.equal(copyMethod({}), "legacy"); // insecure context: navigator.clipboard is undefined
  assert.equal(copyMethod(undefined), "legacy");
});

test("copyText: secure context writes with the API, no legacy copy", async () => {
  const wrote = [], doc = fakeDoc();
  assert.equal(await copyText("hi", { nav: { clipboard: { writeText: async (t) => { wrote.push(t); } } }, doc }), true);
  assert.deepEqual(wrote, ["hi"]);
  assert.deepEqual(doc.calls, []);
});

test("copyText: a refused API write falls back to the legacy copy", async () => {
  const doc = fakeDoc();
  assert.equal(await copyText("hi", { nav: { clipboard: { writeText: async () => { throw new Error("NotAllowedError"); } } }, doc }), true);
  assert.deepEqual(doc.calls.map((c) => c[0]), ["append", "select", "exec", "remove", "refocus"]);
});

test("copyText: insecure context copies synchronously, inside the gesture", () => {
  const doc = fakeDoc();
  const p = copyText("a\nb", { nav: {}, doc });
  // No await before the legacy copy: it has already run when copyText returns.
  assert.deepEqual(doc.calls, [["append", "a\nb"], ["select", "a\nb"], ["exec", "copy"], ["remove"], ["refocus"]]);
  return p.then((ok) => assert.equal(ok, true));
});

test("legacyCopy: false when execCommand refuses or throws", () => {
  assert.equal(legacyCopy("x", fakeDoc(false)), false);
  assert.equal(legacyCopy("x", fakeDoc(new Error("no"))), false);
});

test("copyFailure names plain http only there", () => {
  assert.match(copyFailure(false), /plain http/);
  assert.doesNotMatch(copyFailure(true), /plain http/);
});

test("pasteLines: one line stays in the input", () => {
  assert.deepEqual(pasteLines("echo ", "hi", ""), { run: [], skipped: 0, keep: "echo hi", caret: 7 });
  assert.deepEqual(pasteLines("a", "XY", "b"), { run: [], skipped: 0, keep: "aXYb", caret: 3 });
});

test("pasteLines: every complete line runs, the rest stays", () => {
  assert.deepEqual(pasteLines("", "bind f slop\nsensitivity 2\nname x", ""), { run: ["bind f slop", "sensitivity 2"], skipped: 0, keep: "name x", caret: 6 });
  assert.deepEqual(pasteLines("", "echo a\r\necho b\r\n", ""), { run: ["echo a", "echo b"], skipped: 0, keep: "", caret: 0 });
  assert.deepEqual(pasteLines("ec", "ho a\n", "tail"), { run: ["echo a"], skipped: 0, keep: "tail", caret: 0 });
});

test("pasteLines: blank lines are skipped, a console echo's ] is dropped", () => {
  assert.deepEqual(pasteLines("", "] echo hi\n\n  \n]status\n", ""), { run: ["echo hi", "status"], skipped: 0, keep: "", caret: 0 });
});

test("pasteLines: a copied log runs its commands, not its output", () => {
  const isCmd = (w) => ["echo", "status"].includes(w);
  const log = "counterslop console. type help.\n] echo live slop\nlive slop\n";
  assert.deepEqual(pasteLines("", log, "", isCmd), { run: ["echo live slop"], skipped: 2, keep: "", caret: 0 });
  // A help row starts with a command's name: in a copied log it never runs.
  const help = "] help\nCOMMANDS\nstatus  players and ping\n] echo x\nx\n";
  assert.deepEqual(pasteLines("", help, "", (w) => ["help", "echo", "status"].includes(w)), { run: ["help", "echo x"], skipped: 3, keep: "", caret: 0 });
  // Without echoes it is a config: every command line runs.
  assert.deepEqual(pasteLines("", "status\nnot a command\n", "", isCmd), { run: ["status"], skipped: 1, keep: "", caret: 0 });
});

test("lastLines: the last n, or all", () => {
  const l = ["a", "b", "c"];
  assert.deepEqual(lastLines(l, "2"), ["b", "c"]);
  assert.deepEqual(lastLines(l, 9), l);
  assert.deepEqual(lastLines(l, undefined), l);
  assert.deepEqual(lastLines(l, "x"), l);
  assert.deepEqual(lastLines(l, 0), l);
});
