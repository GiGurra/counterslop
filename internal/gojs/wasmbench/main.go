//go:build js && wasm

// Command wasmbench is the evidence for the Go→WASM option in
// docs/shared-logic.md: internal/sim compiled with GOOS=js GOARCH=wasm and
// driven from JS two ways. Build and run:
//
//	GOOS=js GOARCH=wasm go build -o /tmp/sim.wasm ./internal/gojs/wasmbench
//	node internal/gojs/wasmbench/bench.mjs /tmp/sim.wasm
package main

import (
	"syscall/js"
	"unsafe"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/sim"
)

var (
	world sim.World
	body  = sim.NewBody(sim.Vec3{})
	// state is shared with JS: pos[3] vel[3] yaw onGround crouched (as floats).
	state [9]float64
)

func load() {
	s := state[:]
	body.Pos, body.Vel = sim.Vec3{s[0], s[1], s[2]}, sim.Vec3{s[3], s[4], s[5]}
	body.Yaw, body.OnGround, body.Crouched = s[6], s[7] != 0, s[8] != 0
	if body.Crouched {
		body.H = sim.HCrouch
	} else {
		body.H = sim.H
	}
}

func store() {
	state = [9]float64{body.Pos[0], body.Pos[1], body.Pos[2], body.Vel[0], body.Vel[1], body.Vel[2], body.Yaw, b(body.OnGround), b(body.Crouched)}
}

func b(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func input(f, r float64, flags int32) sim.Input {
	return sim.Input{F: f, R: r, Walk: flags&1 != 0, Jump: flags&2 != 0, Crouch: flags&4 != 0, Speed: 1}
}

// step: one tick per call, state in the shared array (the cheapest per-tick shape).
//
//go:wasmexport step
func step(f, r float64, flags int32, dt float64) int32 {
	load()
	ev := world.Step(body, input(f, r, flags), dt)
	store()
	return int32(ev.Steps)
}

//go:wasmexport statePtr
func statePtr() int32 { return int32(uintptr(unsafe.Pointer(&state[0]))) }

func main() {
	// setMap via syscall/js (once per map; no encoding/json, it would add 3 MB), and a syscall/js step that takes
	// and returns a JS object, the shape physics.js callers use today.
	js.Global().Set("simSetMap", js.FuncOf(func(_ js.Value, a []js.Value) any {
		bs := a[0] // [{min, max}]
		world.Boxes = make([]content.Box, bs.Length())
		for i := range world.Boxes {
			mn, mx := bs.Index(i).Get("min"), bs.Index(i).Get("max")
			for k := range 3 {
				world.Boxes[i].Min[k], world.Boxes[i].Max[k] = mn.Index(k).Float(), mx.Index(k).Float()
			}
		}
		return nil
	}))
	js.Global().Set("simStepObj", js.FuncOf(func(_ js.Value, a []js.Value) any {
		s, in := a[0], a[1]
		p, v := s.Get("pos"), s.Get("vel")
		body.Pos = sim.Vec3{p.Index(0).Float(), p.Index(1).Float(), p.Index(2).Float()}
		body.Vel = sim.Vec3{v.Index(0).Float(), v.Index(1).Float(), v.Index(2).Float()}
		body.Yaw, body.OnGround, body.Crouched = s.Get("yaw").Float(), s.Get("onGround").Bool(), s.Get("crouched").Bool()
		world.Step(body, sim.Input{F: in.Get("f").Float(), R: in.Get("r").Float(), Walk: in.Get("walk").Truthy(), Jump: in.Get("jump").Truthy(), Crouch: in.Get("crouch").Truthy(), Speed: 1}, a[2].Float())
		for i := range 3 {
			p.SetIndex(i, body.Pos[i])
			v.SetIndex(i, body.Vel[i])
		}
		s.Set("onGround", body.OnGround)
		s.Set("crouched", body.Crouched)
		return nil
	}))
	select {}
}
