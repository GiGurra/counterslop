// Per-tick input commands: what the client samples once per sim tick and what
// the server-authoritative sim will receive. Shape:
//   {seq, ms, f, r, yaw, pitch, b}
//   seq    increasing command number
//   ms     tick length in ms (1000 / simHz)
//   f, r   move axes -1..1 (forward, right)
//   yaw, pitch  view angles at the tick
//   b      button bits (BTN), the *held* state; edges come from comparing with
//          the previous command's b, on the client and on the server alike.
// The button bits and the jump-edge rule are internal/game/cmd_shared.go,
// translated by cmd/gojs (the server runs the same MoveInput).
import { BtnJump, BtnCrouch, BtnWalk, BtnFire, BtnScope, BtnReload, BtnUse, BtnAlt, BtnHook, MoveInput } from "../game/shared.gen.js";

export const BTN = { JUMP: BtnJump, CROUCH: BtnCrouch, WALK: BtnWalk, FIRE: BtnFire, SCOPE: BtnScope, RELOAD: BtnReload, USE: BtnUse, ALT: BtnAlt, HOOK: BtnHook };

// Physics input for one command: {f, r, walk, crouch, jump, speed: 0, pitch}. A
// jump fires only on the JUMP down edge and never while crouching on the ground.
// The client's commands carry the view pitch as `pitch` (the wire's `p`, which
// the generated code reads): swimming goes along it.
export const moveInput = (cmd, prevB, body) => ({ ...MoveInput(cmd, prevB, body), pitch: cmd.pitch ?? cmd.p ?? 0 });
