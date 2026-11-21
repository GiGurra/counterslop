package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// Map changes, the rotation (mp_maprotation) and the end-of-match map vote.
//
// A vote offers up to three maps from the rotation (the next ones after the
// current map, in rotation order). Players send {t:"vote", map}; everyone gets
// {t:"mapvote", options, votes:{map:n}, endsIn} on every change. When it ends
// the map with the most votes loads (ties and no votes: the earliest option),
// through the same path as changelevel. Match end (tk_87d8fded) calls
// StartMapVote; admins can start one with `mapvote [seconds]`.

const defaultVoteSeconds = 15

type mapVote struct {
	options []string
	votes   map[int]string // player id -> map
	endsAt  time.Time
}

// rotation is mp_maprotation as known map names (empty or unknown: every map).
func (s *Server) rotation() []string {
	var out []string
	for _, n := range strings.FieldsFunc(s.cvars["mp_maprotation"], func(r rune) bool { return r == ',' || r == ' ' }) {
		if maps.All[n] != nil && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return slices.Clone(maps.Order)
	}
	return out
}

// setRotation validates and stores mp_maprotation ("" or "all" = every map).
func (s *Server) setRotation(v string) error {
	if v == "all" || v == "" {
		s.cvars["mp_maprotation"] = strings.Join(maps.Order, ",")
		return nil
	}
	names := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
	for _, n := range names {
		if maps.All[n] == nil {
			return fmt.Errorf("unknown map %q (maps: %s)", n, strings.Join(maps.Order, " "))
		}
	}
	s.cvars["mp_maprotation"] = strings.Join(names, ",")
	return nil
}

// mapCategory is every known map's category (built once).
var mapCategory = sync.OnceValue(func() map[string]string {
	out := map[string]string{}
	for n, build := range maps.All {
		out[n] = build().Category
	}
	return out
})

// sameCategory is the rotation narrowed to the current map's category
// (Training or Competitive), so a competitive match votes among competitive
// maps; the whole rotation if that leaves no other map to vote for.
func (s *Server) sameCategory(rot []string) []string {
	cat := s.Map.Category
	if cat == "" {
		return rot
	}
	var out []string
	others := 0
	for _, n := range rot {
		if mapCategory()[n] == cat {
			out = append(out, n)
			if n != s.Map.Name {
				others++
			}
		}
	}
	if others == 0 {
		return rot
	}
	return out
}

// voteOptions: up to three maps following the current one in the rotation
// (narrowed to the current map's category, sameCategory);
// the current map only when the rotation has nothing else.
func (s *Server) voteOptions() []string {
	rot := s.sameCategory(s.rotation())
	i := slices.Index(rot, s.Map.Name)
	var out []string
	for k := 1; k <= len(rot) && len(out) < 3; k++ {
		n := rot[(i+k+len(rot))%len(rot)]
		if n != s.Map.Name && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	if len(out) < 2 && !slices.Contains(out, s.Map.Name) {
		out = append(out, s.Map.Name)
	}
	return out
}

// StartMapVote opens a vote for d. Callers hold s.mu.
func (s *Server) StartMapVote(d time.Duration) {
	s.vote = &mapVote{options: s.voteOptions(), votes: map[int]string{}, endsAt: s.clock().Add(d)}
	s.sendVote()
}

func (s *Server) voteCounts() map[string]int {
	counts := map[string]int{}
	for _, o := range s.vote.options {
		counts[o] = 0
	}
	for id, m := range s.vote.votes {
		if s.players[id] != nil {
			counts[m]++
		}
	}
	return counts
}

func (s *Server) sendVote() {
	v := s.vote
	for _, p := range s.players {
		b, _ := json.Marshal(map[string]any{
			"t": "mapvote", "options": v.options, "votes": s.voteCounts(), "mine": v.votes[p.ID],
			"endsIn": max(0, v.endsAt.Sub(s.clock()).Seconds()),
		})
		trySend(p, b)
	}
}

// castVote records p's vote (changing it is fine). Callers hold s.mu.
func (s *Server) castVote(p *Player, m string) {
	if s.vote == nil || p.Bot || !slices.Contains(s.vote.options, m) {
		return
	}
	s.vote.votes[p.ID] = m
	s.sendVote()
}

// tickVote ends a vote whose time is up and loads the winner. Callers hold s.mu.
func (s *Server) tickVote(now time.Time) {
	if s.vote == nil || now.Before(s.vote.endsAt) {
		return
	}
	counts := s.voteCounts()
	win := s.vote.options[0]
	for _, o := range s.vote.options {
		if counts[o] > counts[win] {
			win = o
		}
	}
	s.vote = nil
	b, _ := json.Marshal(map[string]any{"t": "mapvote", "done": true, "winner": win, "votes": counts})
	for _, p := range s.players {
		trySend(p, b)
	}
	s.announce(fmt.Sprintf("map vote: %s wins (%d votes)", win, counts[win]))
	s.changeLevel(win)
}

// changeLevel loads a map for everyone and restarts the match. Callers hold s.mu.
func (s *Server) changeLevel(name string) bool {
	build := maps.Find(name) // a lab map too (maps.Labs)
	if build == nil {
		return false
	}
	s.Map = build()
	s.vote = nil
	s.round = newRound() // a new map is a new match: rounds start over from warmup
	s.nades = nil
	s.spawnNext = map[string]int{}
	s.items, s.itemRespawn = nil, map[int]time.Time{}
	for i := range s.Map.Items {
		s.itemRespawn[i] = time.Time{}
	}
	for _, p := range s.players {
		if p.Bot { // off the old map's geometry
			p.Pos = s.spawnPoint(p)
			if p.brain != nil {
				p.brain.target = p.Pos
			}
		}
	}
	b, _ := json.Marshal(map[string]any{"t": "map", "map": s.Map})
	for _, o := range s.players {
		trySend(o, b)
	}
	s.restart()
	return true
}

// mapInfo is a map catalog entry for the selector and the vote cards: the
// whole map (clients render the preview from its boxes).
type mapInfo struct {
	content.Map
	Size [2]float64 `json:"size"` // x, z extent in metres
}

// MapCatalog lists every map in rotation order (Order), for /maps.json.
func MapCatalog() []mapInfo {
	names := slices.Clone(maps.Order)
	for n := range maps.All {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		return slices.Index(maps.Order, names[i]) >= 0 && slices.Index(maps.Order, names[j]) < 0
	})
	out := make([]mapInfo, 0, len(names))
	for _, n := range names {
		m := maps.All[n]()
		x0, x1, z0, z1 := 0.0, 0.0, 0.0, 0.0
		for i, b := range m.Boxes {
			if i == 0 || b.Min[0] < x0 {
				x0 = b.Min[0]
			}
			if i == 0 || b.Max[0] > x1 {
				x1 = b.Max[0]
			}
			if i == 0 || b.Min[2] < z0 {
				z0 = b.Min[2]
			}
			if i == 0 || b.Max[2] > z1 {
				z1 = b.Max[2]
			}
		}
		out = append(out, mapInfo{Map: m, Size: [2]float64{x1 - x0, z1 - z0}})
	}
	return out
}
