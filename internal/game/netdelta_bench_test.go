package game

import (
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// The delta codec's cost at 12v12, all 24 connected and moving, each client
// acking lag ticks late (its round trip: the base is that old): CPU per
// snapshot tick (all 24 encoded), bytes per snapshot per client, the heap,
// the ring's length and what it holds per client.
// go test ./internal/game -run '^$' -bench DeltaAckLag
func benchAcked(b *testing.B, lag int) {
	t := &testing.T{}
	s := authServer(t)
	s.Test = true
	_ = s.SetNetCodec(CodecDelta)
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
	bytes := 0
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
				m := <-p.send
				if isDeltaSnap(m) {
					bytes += len(m)
				}
			}
			if s.tick-int64(lag) > 0 {
				benchAck(p, s.tick, s.tick-int64(lag))
			}
		}
	}
	for range 700 {
		step(true)
	}
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	bytes = 0
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		step(false)
		b.StartTimer()
		s.broadcast()
		b.StopTimer()
		for _, p := range ps {
			for len(p.send) > 0 {
				if m := <-p.send; isDeltaSnap(m) {
					bytes += len(m)
				}
			}
			if s.tick-int64(lag) > 0 {
				benchAck(p, s.tick, s.tick-int64(lag))
			}
		}
		b.StartTimer()
	}
	b.ReportMetric(float64(m.HeapAlloc)/1e6, "heapMB")
	b.ReportMetric(float64(bytes)/float64(b.N)/24, "B/snap/client")
	b.ReportMetric(benchRingLen(ps), "ring")
	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)
	for _, p := range ps {
		p.delta.sent, p.delta.last = nil, nil
	}
	s.deltaPrev = nil
	runtime.GC()
	runtime.ReadMemStats(&m2)
	b.ReportMetric(float64(m1.HeapAlloc-m2.HeapAlloc)/24/1024, "ringKB/client")
}

func benchRingLen(ps []*Player) float64 {
	n := 0
	for _, p := range ps {
		n += len(p.delta.sent)
	}
	return float64(n) / float64(len(ps))
}

// benchAck: the client's ack, lag ticks late: the newest kept snapshot it had then.
func benchAck(p *Player, now, t int64) {
	var k int64
	for _, e := range p.delta.sent {
		if e.tick <= t {
			k = e.tick
		}
	}
	if k > 0 {
		p.ackSnap(now, k)
	}
}

func BenchmarkDeltaAckLag1(b *testing.B)   { benchAcked(b, 1) }
func BenchmarkDeltaAckLag3(b *testing.B)   { benchAcked(b, 3) }
func BenchmarkDeltaAckLag6(b *testing.B)   { benchAcked(b, 6) }
func BenchmarkDeltaAckLag12(b *testing.B)  { benchAcked(b, 12) }
func BenchmarkDeltaAckLag25(b *testing.B)  { benchAcked(b, 25) }
func BenchmarkDeltaAckLag100(b *testing.B) { benchAcked(b, 100) }
