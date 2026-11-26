package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Net codecs (docs/replication.md "Codecs"): how snapshots are put on the
// wire, chosen per server at startup (--net-codec) and announced in the
// welcome, so the client picks the matching decoder. Every codec carries the
// same state; the client turns each snapshot into the same object shape.
//
//	json   the hand-built JSON snapshots (full every fullEvery ticks, pose rows between)
//	delta  binary, only what changed since the last one sent (netdelta*.go)
//
// Adding one: implement snapCodec, list it in netCodecs, add its decoder to
// web/engine/net.js, and run the sim/cases with it (CI: the codec shards).
const (
	CodecJSON = "json"
)

// snapCodec builds and sends one tick's snapshot to every connected client.
// list: the ready players (status fields refreshed from their inventories);
// full: a full snapshot is due (the json codec's fullEvery). Callers hold s.mu.
type snapCodec interface {
	name() string
	send(s *Server, list []*Player, full bool)
}

var netCodecs = map[string]snapCodec{
	CodecJSON:  jsonCodec{},
	CodecDelta: deltaCodec{},
}

// NetCodecs lists the codec names (for --help and errors).
func NetCodecs() []string {
	names := make([]string, 0, len(netCodecs))
	for n := range netCodecs {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// SetNetCodec selects the snapshot codec by name ("" = json). Before Run.
func (s *Server) SetNetCodec(name string) error {
	if name == "" {
		name = CodecJSON
	}
	c, ok := netCodecs[name]
	if !ok {
		return fmt.Errorf("unknown net codec %q (one of: %s)", name, strings.Join(NetCodecs(), ", "))
	}
	s.codec = c
	return nil
}

// netCodec is the codec in use (json unless set).
func (s *Server) netCodec() snapCodec {
	if s.codec == nil {
		return jsonCodec{}
	}
	return s.codec
}

// welcomeCodec: the codec name for the welcome; empty for json, so its
// welcome stays what it was before codecs existed.
func (s *Server) welcomeCodec() string {
	if n := s.netCodec().name(); n != CodecJSON {
		return n
	}
	return ""
}

// welcomeSchema: the delta codec's schema hash, for its welcome.
func (s *Server) welcomeSchema() string {
	if s.netCodec().name() == CodecDelta {
		return netSchemaHash
	}
	return ""
}

// jsonCodec: the original snapshots. A pose snapshot ({"pose":1}, Player's hot
// fields as rows plus each client's own state) between full ones.
type jsonCodec struct{}

func (jsonCodec) name() string { return CodecJSON }

func (jsonCodec) send(s *Server, list []*Player, full bool) {
	ms := s.clock().UnixMilli()
	if !full {
		poses := s.posesJSON(list)
		for _, p := range s.players {
			if p.Bot || p.send == nil {
				continue
			}
			if trySend(p, poseBytes(ms, s.tick, poses, s.meBytes(p))) {
				p.snapSent = s.tick
			}
		}
		return
	}
	players, err := json.Marshal(list)
	if err != nil {
		return
	}
	items, _ := json.Marshal(s.items)
	round := s.roundJSON(s.clock()) // game time: lockstep scenarios step it
	nades := s.nadesJSON()
	boats := s.boatsJSON()
	cars := s.carsJSON()
	spot := s.spottedJSON()
	for _, p := range s.players {
		if p.Bot || p.send == nil { // nobody to send to (playing bots): don't build its snapshot
			continue
		}
		if trySend(p, snapBytes(ms, s.tick, players, items, s.meBytes(p), round, nades, boats, cars, spot[p.Team])) {
			p.snapSent = s.tick
		}
	}
}
