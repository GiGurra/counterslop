package game

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// The lockstep harness hooks (harness.go) that cmd/sim -cases drives.
func TestHarnessHooks(t *testing.T) {
	s := authServer(t)
	a := cmdPlayer(t, s, "1.1.1.1", "vibe", Vec3{-26, 0, 0})
	a.Name = "alice.1"
	s.SetLockstep(true)
	if !s.lockstep {
		t.Fatal("lockstep not set")
	}

	// lookups: by name and by "#id" (survives a rename)
	if st := s.CaseState("alice.1"); st == nil || st["name"] != "alice.1" || st["team"] != "vibe" {
		t.Fatalf("case state by name: %v", st)
	}
	if s.CaseState(fmt.Sprintf("#%d", a.ID)) == nil || s.CaseState("#999") != nil || s.CaseState("nobody") != nil {
		t.Fatal("lookup by id / unknown")
	}

	// commands: WaitCommand sees the queued seq, times out on a missing one
	s.queueCmds(a, []Cmd{{Seq: 2, F: 1}})
	if err := s.WaitCommand("alice.1", 2, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := s.WaitCommand("alice.1", 9, 5*time.Millisecond); err == nil || !strings.Contains(err.Error(), "never arrived") {
		t.Fatalf("missing command: %v", err)
	}
	before := s.tick
	s.Step(2)
	if s.tick != before+2 || a.ackSeq != 2 {
		t.Fatalf("Step: tick %d -> %d, ack %d", before, s.tick, a.ackSeq)
	}

	// inventory
	if err := s.SetInventory("alice.1", map[string]*Held{"primary": {ID: "codereview", Ammo: 2, Reserve: 0}}, "primary"); err != nil {
		t.Fatal(err)
	}
	if st := s.CaseState("alice.1"); st["ammo"] != 2 || st["cur"] != "primary" {
		t.Fatalf("after SetInventory: %v", st)
	}
	if s.SetInventory("nobody", nil, "") == nil {
		t.Fatal("SetInventory on a missing player")
	}

	// world: dropped items and bans are cleared per case, map items stay
	s.items = append(s.items, &Item{mapSlot: -1}, &Item{mapSlot: 0})
	s.bans["9.9.9.9"] = time.Now().Add(time.Hour)
	if s.ItemCount() != 1 {
		t.Fatalf("ItemCount counts dropped weapons only: %d", s.ItemCount())
	}
	s.ClearDropped()
	if s.ItemCount() != 0 || len(s.items) != 1 || len(s.bans) != 0 {
		t.Fatalf("ClearDropped: %d items, %d bans", len(s.items), len(s.bans))
	}

	// registry, cvars, counts, round fields
	if cmds := s.AdminCommands(); !slices.Contains(cmds, "kick") || !slices.Contains(cmds, "sv_gravity") || !slices.IsSorted(cmds) {
		t.Fatalf("AdminCommands: %v", cmds)
	}
	if s.Cvar("mp_friendlyfire") != "0" || s.Cvar("nope") != "" {
		t.Fatal("Cvar")
	}
	s.addBot()
	if h, b := s.PlayerCount(); h != 1 || b != 1 {
		t.Fatalf("PlayerCount: %d humans %d bots", h, b)
	}
	for _, k := range []string{"phase", "number", "bomb", "site", "carrier", "winner", "reason", "score.vibe"} {
		_ = s.RoundField(k)
	}
	if s.RoundField("nope") != nil {
		t.Fatal("unknown round field")
	}

	// sync marker reaches the player's socket queue
	msgs(a)
	s.Sync("alice.1", 7)
	if m := ofType(a, "sync"); len(m) != 1 || m[0]["n"] != 7.0 {
		t.Fatalf("sync: %v", m)
	}
	s.SetLockstep(false)

	// ResetWorld: default cvars, no bots, de_slop, no dropped weapons
	s.cvars["sv_gravity"] = "5"
	s.items = append(s.items, &Item{mapSlot: -1})
	s.ResetWorld()
	if h, b := s.PlayerCount(); h != 1 || b != 0 || s.Cvar("sv_gravity") != defaultCvars["sv_gravity"] || s.ItemCount() != 0 || s.Map.Name != "de_slop" {
		t.Fatalf("ResetWorld: %d humans %d bots, gravity %s, %d items, map %s", h, b, s.Cvar("sv_gravity"), s.ItemCount(), s.Map.Name)
	}
}

func TestBalanceMarkdown(t *testing.T) {
	r := registry(t)
	md := BalanceMarkdown(r, &Economy{KillReward: map[string]int{"rifle": 300}}, BalanceTable(r, PlayerModel.Mult))
	for _, want := range []string{"### Balance", "**The Prompt**", "600 RPM", "(3-round bursts)", "× 9 pellets", "| head, helmet |", "kill $300"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
	if got := rpm(&Weapon{Action: "burst", Burst: 3, BurstGap: 0.05, Interval: 0.4}); got < 359 || got > 361 {
		t.Fatalf("burst rpm: %v", got)
	}
	if fallOr1(0) != 1 || fallOr1(0.9) != 0.9 || speedOr1(0) != 1 {
		t.Fatal("defaults")
	}
}

func TestCheckBandsReports(t *testing.T) {
	table := []TTKCell{{Weapon: "a", Armor: "none", Part: "body", Dist: 5, Shots: 3, TTK: 0.2}}
	bad := CheckBands(table, map[string]map[string]Band{
		"a":     {"body/none/5": {Shots: []int{1, 1}, TTKms: []float64{0, 100}}, "head/none/99": {}},
		"ghost": {"body/none/5": {}},
	})
	want := []string{"3 shots to kill", "TTK 200 ms", "no such cell", "doesn't exist"}
	for _, w := range want {
		if !slices.ContainsFunc(bad, func(l string) bool { return strings.Contains(l, w) }) {
			t.Errorf("no %q in %v", w, bad)
		}
	}
	if bad := CheckBands(table, nil); len(bad) != 1 || !strings.Contains(bad[0], "no balance bands") {
		t.Fatalf("missing bands: %v", bad)
	}
}

func TestHeldSpeed(t *testing.T) {
	s := authServer(t)
	p := &Player{}
	if s.heldSpeed(p) != 1 {
		t.Fatal("no inventory = 1")
	}
	p.inv = s.Weapons.Loadout("vibe")
	if got := s.heldSpeed(p); got != s.Weapons.Get("prompt").Speed {
		t.Fatalf("prompt speed: %v", got)
	}
}

// A command sent just before the player became a spectator (the team message
// got in first) is dropped by queueCmds, which has no body to move: waiting for
// it timed out the afk-autobalance case ("command 8 never arrived") whenever
// the server took the jointeam before the harness looked.
func TestWaitCommandSpectatorHasNothingToWaitFor(t *testing.T) {
	s := authServer(t)
	a := cmdPlayer(t, s, "1.1.1.1", "vibe", Vec3{-26, 0, 0})
	a.Name = "dave.1"
	s.SetLockstep(true)
	s.queueCmds(a, []Cmd{{Seq: 7}})
	s.handle(a, inbound{T: "team", Team: "spec"})
	s.queueCmds(a, []Cmd{{Seq: 8}}) // dropped: spectators have no body
	if err := s.WaitCommand("dave.1", 8, 50*time.Millisecond); err != nil {
		t.Fatalf("spectator: %v", err)
	}
}

// Commands overtaken by newer ones (datagrams reorder) still run, in seq order,
// as long as the server hasn't run past them; duplicates and commands it has
// already run are dropped. Before: anything at or below the highest seq seen
// was dropped, so a packet overtaken by 4+ newer ones lost its input.
func TestQueueCmdsTakesLateCommandsInOrder(t *testing.T) {
	s := authServer(t)
	a := cmdPlayer(t, s, "1.1.1.1", "vibe", Vec3{-26, 0, 0})
	s.SetLockstep(true)
	seqs := func() []int {
		var out []int
		for _, c := range a.cmds {
			out = append(out, c.Seq)
		}
		return out
	}
	s.queueCmds(a, []Cmd{{Seq: 1}})
	s.Step(5) // runs 1
	s.queueCmds(a, []Cmd{{Seq: 5}, {Seq: 6}})
	s.queueCmds(a, []Cmd{{Seq: 3}, {Seq: 4}, {Seq: 5}}) // late, with a duplicate
	s.queueCmds(a, []Cmd{{Seq: 2}})
	s.queueCmds(a, []Cmd{{Seq: 1}}) // already run
	if got := seqs(); !slices.Equal(got, []int{2, 3, 4, 5, 6}) {
		t.Fatalf("queue %v, want [2 3 4 5 6]", got)
	}
	if a.lastQueued != 6 {
		t.Fatalf("lastQueued %d", a.lastQueued)
	}
	s.Step(10) // the command budget runs about one per tick
	if a.ackSeq != 6 || len(a.cmds) != 0 {
		t.Fatalf("after the ticks: ack %d, queue %v", a.ackSeq, seqs())
	}
	s.queueCmds(a, []Cmd{{Seq: 4}}) // run past: dropped
	if len(a.cmds) != 0 {
		t.Fatalf("a command already run past was queued: %v", seqs())
	}
}

// ResetWorld kicks playing bots (bot_quota, bot_add) too, not only test bots:
// a scenario's bot_quota 10 left five a side in the next scenario on the same
// server (CI sim shard: spawn-overview then kill-feed, alice never hit).
func TestResetWorldKicksPlayingBots(t *testing.T) {
	s, h, _ := botRound(t, 2, 2)
	s.cvars["bot_quota"] = "10"
	s.fillQuota()
	if _, b := s.PlayerCount(); b == 0 {
		t.Fatal("setup: no playing bots")
	}
	s.ResetWorld()
	if _, b := s.PlayerCount(); b != 0 {
		t.Fatalf("ResetWorld left %d playing bots", b)
	}
	if s.players[h.ID] == nil {
		t.Fatal("ResetWorld dropped the human")
	}
}

// The read-only harness accessors and small setters cmd/sim uses.
func TestHarnessAccessors(t *testing.T) {
	s := authServer(t)
	a := cmdPlayer(t, s, "1.1.1.1", "vibe", Vec3{-26, 0, 0})
	a.Name = "alice"
	s.SetLockstep(true)
	if s.SnapTick() != -1 {
		t.Fatalf("SnapTick before any snapshot: %d", s.SnapTick())
	}
	before := s.Tick()
	s.Step(snapEvery)
	if s.Tick() != before+snapEvery || s.SnapTick() <= 0 {
		t.Fatalf("Tick %d -> %d, SnapTick %d", before, s.Tick(), s.SnapTick())
	}
	if err := s.SetBoost("alice", 5); err != nil || a.inv.boostEnd != a.cmdTicks+5 {
		t.Fatalf("SetBoost: %v, boostEnd %d", err, a.inv.boostEnd)
	}
	if s.SetBoost("nobody", 1) == nil {
		t.Fatal("SetBoost on a missing player")
	}
	if s.NadeCount() != 0 {
		t.Fatalf("NadeCount %d", s.NadeCount())
	}
	s.history.Record(s.tick, map[int]PoseState{a.ID: {Team: "vibe"}}) // something to forget
	s.ForgetPoses()
	if len(s.history.frames) != 0 {
		t.Fatal("ForgetPoses kept history")
	}
	// Connected counts humans' connections, not bots.
	bot := cmdPlayer(t, s, "", "artisan", Vec3{26, 0, 0})
	bot.Bot = true
	if s.Connected() != 1 {
		t.Fatalf("Connected %d, want 1 (the bot doesn't count)", s.Connected())
	}
	// RoundField: every key answers (nil only for unknown ones).
	for _, k := range []string{"phase", "number", "bomb", "site", "carrier", "mvp", "mvpWhy", "winner", "reason", "score.vibe", "max", "overtime", "played", "swapped", "matchwinner", "mode", "secured", "toWin", "gpus"} {
		if s.RoundField(k) == nil {
			t.Errorf("RoundField(%q) = nil", k)
		}
	}
	for _, k := range []string{"nope", "gpu0", "gpux", "gpu99.by"} {
		if s.RoundField(k) != nil {
			t.Errorf("RoundField(%q) = %v, want nil", k, s.RoundField(k))
		}
	}
}

// SlowRead (sim/cases "slowRead"): only the named player's command packets
// whose newest seq is a multiple of Every are read late; off with Every 0.
func TestSlowReadDelaysOnlyItsPackets(t *testing.T) {
	s := authServer(t)
	a := cmdPlayer(t, s, "1.1.1.1", "vibe", Vec3{-26, 0, 0})
	a.Name = "erin.1"
	if err := s.SetSlowRead("nobody", SlowRead{Every: 2, Ms: 1}); err == nil {
		t.Fatal("no such player: want an error")
	}
	if err := s.SetSlowRead("erin.1", SlowRead{Every: 2, Ms: 30}); err != nil {
		t.Fatal(err)
	}
	took := func(m inbound) time.Duration {
		t0 := time.Now()
		s.readLate(a, m)
		return time.Since(t0)
	}
	if d := took(inbound{T: "cmd", Cmds: []Cmd{{Seq: 3}, {Seq: 4}}}); d < 30*time.Millisecond {
		t.Fatalf("seq 4 of every 2: read after %v, want 30 ms late", d)
	}
	if d := took(inbound{T: "cmd", Cmds: []Cmd{{Seq: 4}, {Seq: 5}}}); d > 20*time.Millisecond {
		t.Fatalf("seq 5: read after %v, want at once", d)
	}
	if d := took(inbound{T: "say"}); d > 20*time.Millisecond {
		t.Fatalf("not a command packet: read after %v", d)
	}
	_ = s.SetSlowRead("erin.1", SlowRead{})
	if d := took(inbound{T: "cmd", Cmds: []Cmd{{Seq: 6}}}); d > 20*time.Millisecond {
		t.Fatalf("off: read after %v", d)
	}
}
