package game

import (
	"bytes"
	"compress/flate"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// What one client receives in a 12v12 with everyone moving: snapshot bytes
// per second, raw (Player.bytesOut) and as the websocket sends them
// (permessage-deflate, BestSpeed, each message on its own). Logged for the
// record; TestSnapshotBandwidth bounds it.
func snapshotRates(t *testing.T, codec string) (raw, wire, perSnap float64) {
	s := authServer(t)
	s.Test = true
	if err := s.SetNetCodec(codec); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	s.lastTickAt = start
	var ps []*Player
	for i := range 24 {
		p := s.join(fmt.Sprintf("10.0.%d.1", i))
		p.Name, p.ready = fmt.Sprintf("p%02d", i), true
		p.Team = content.Vibe
		if i%2 == 1 {
			p.Team = content.Artisan
		}
		s.respawn(p)
		p.inv = s.Weapons.Loadout(p.Team)
		ps = append(ps, p)
	}
	const ticks = 300
	var rawB, wireB, snaps int
	var buf bytes.Buffer
	zw, _ := flate.NewWriter(&buf, flate.BestSpeed)
	drain := func(p *Player) {
		for {
			select {
			case b := <-p.send:
				if p != ps[0] {
					continue
				}
				rawB += len(b)
				buf.Reset()
				zw.Reset(&buf)
				zw.Write(b)
				zw.Flush()
				wireB += buf.Len()
				if bytes.Contains(b[:min(len(b), 16)], []byte(`"snap"`)) || b[0] == deltaMagic {
					snaps++
				}
			default:
				return
			}
		}
	}
	for _, p := range ps {
		drain(p)
	}
	rawB, wireB, snaps = 0, 0, 0
	for tick := 1; tick <= ticks; tick++ {
		for i, p := range ps {
			yaw := float64(i) + float64(tick)*0.02
			s.queueCmds(p, []Cmd{{Seq: tick, F: 1, R: math.Sin(float64(tick+i) * 0.1), Yaw: yaw, Pitch: 0.1}})
		}
		if s.advance(start.Add(time.Duration(tick) * time.Second / SimHz)) {
			s.broadcast()
		}
		for _, p := range ps {
			drain(p)
		}
	}
	secs := float64(ticks) / SimHz
	return float64(rawB) / 1024 / secs, float64(wireB) / 1024 / secs, float64(rawB) / float64(max(snaps, 1))
}

// 100 snapshots/s (two of three pose snapshots) cost about twice the old 33
// full ones: 33.7 KB/s deflated per client at 12v12 then, ~73 now.
func TestSnapshotBandwidth(t *testing.T) {
	raw, wire, per := snapshotRates(t, CodecJSON)
	t.Logf("12v12, %d snapshots/s: %.1f KB/s raw (%.0f B per snapshot), %.1f KB/s deflated, per client", SimHz/snapEvery, raw, per, wire)
	if wire > 80 {
		t.Errorf("%.1f KB/s per client on the wire at 12v12: over 80 (pose snapshots should keep it near 2x the 33 Hz full ones)", wire)
	}
}

// The delta codec (only what changed, binary): per-message deflate on top
// (the websocket still negotiates it) barely helps it.
func TestSnapshotBandwidthDelta(t *testing.T) {
	raw, wire, per := snapshotRates(t, CodecDelta)
	t.Logf("12v12, delta codec, %d snapshots/s: %.1f KB/s raw (%.0f B per snapshot), %.1f KB/s deflated, per client", SimHz/snapEvery, raw, per, wire)
	if raw > 25 {
		t.Errorf("%.1f KB/s per client at 12v12 with the delta codec: over 25", raw)
	}
}

// CPU per snapshot tick at 12v12, everyone moving, all 24 connected,
// building and compressing:
// go test ./internal/game -run '^$' -bench SnapshotTick
func BenchmarkSnapshotTickJSON(b *testing.B)  { benchSnapshotTick(b, CodecJSON) }
func BenchmarkSnapshotTickDelta(b *testing.B) { benchSnapshotTick(b, CodecDelta) }

func benchSnapshotTick(b *testing.B, codec string) {
	t := &testing.T{}
	s := authServer(t)
	s.Test = true
	_ = s.SetNetCodec(codec)
	start := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	s.lastTickAt = start
	var ps []*Player
	for i := range 24 {
		p := s.join(fmt.Sprintf("10.0.%d.1", i))
		p.Name, p.ready = fmt.Sprintf("p%02d", i), true
		p.Team = []string{content.Vibe, content.Artisan}[i%2]
		s.respawn(p)
		p.inv = s.Weapons.Loadout(p.Team)
		ps = append(ps, p)
	}
	tick := 0
	step := func(broadcast bool) {
		tick++
		for i, p := range ps {
			s.queueCmds(p, []Cmd{{Seq: tick, F: 1, R: math.Sin(float64(tick+i) * 0.1), Yaw: float64(i) + float64(tick)*0.02, Pitch: 0.1}})
		}
		s.advance(start.Add(time.Duration(tick) * time.Second / SimHz))
		if broadcast {
			s.broadcast()
		}
		for _, p := range ps {
			for len(p.send) > 0 {
				<-p.send
			}
		}
	}
	for range 200 { // everyone has a base
		step(true)
	}
	// What the websocket writers add: per-message deflate (BestSpeed) on
	// text messages; binary ones go uncompressed (writeLoop).
	var zb bytes.Buffer
	zw, _ := flate.NewWriter(&zb, flate.BestSpeed)
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		step(false)
		b.StartTimer()
		s.broadcast()
		for _, p := range ps {
			for len(p.send) > 0 {
				if m := <-p.send; m[0] == '{' {
					zb.Reset()
					zw.Reset(&zb)
					zw.Write(m)
					zw.Flush()
				}
			}
		}
	}
}
