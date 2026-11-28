package game

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"github.com/gigurra/counterslop/internal/content"
)

// Team size cap and spawn picking.
//
//   - Each side holds at most Map.MaxPlayers/2 players (bots included; no cap
//     when MaxPlayers is 0). A human joining a side that is full only because
//     of bots takes a bot's place (the newest bot is kicked, as in CS); a side
//     full of humans refuses ("teamfull"). Auto-assign picks the other side,
//     or leaves you spectating if both are full. Bot fill never goes over.
//   - Changing to a smaller map trims every side over the new cap: bots
//     first, then the humans who joined that side most recently go to
//     spectators, with a notice. Then autobalance runs.
//   - Every player in a round gets their own spawn point: round-robin over
//     the team's points, skipping one where a live player already stands.

// spawnClear is how close (xz, metres) a standing player makes a spawn point taken.
const spawnClear = 0.8

// teamCap is how many players one side may hold on this map (0: no cap).
func (s *Server) teamCap() int { return s.Map.MaxPlayers / 2 }

// onTeam counts the players on team (playing bots included, target-practice
// bots not), leaving out except; bots is how many of them are playing bots.
// Callers hold s.mu.
func (s *Server) onTeam(team string, except *Player) (all, bots int) {
	for _, o := range s.players {
		if o == except || o.Bot || o.Team != team {
			continue
		}
		all++
		if o.ai != nil {
			bots++
		}
	}
	return all, bots
}

// capFull reports whether p (nil: a new bot) can't join team for the cap:
// it is full, and (for a human) no bot on it could make room. Callers hold s.mu.
func (s *Server) capFull(team string, p *Player) bool {
	c := s.teamCap()
	if c <= 0 {
		return false
	}
	all, bots := s.onTeam(team, p)
	if p != nil && p.ai == nil {
		all -= bots // a bot makes room for a human
	}
	return all >= c
}

// otherTeam is the opposing side of team.
func otherTeam(team string) string {
	if team == content.Vibe {
		return content.Artisan
	}
	return content.Vibe
}

// makeRoom kicks the newest bot from team if p joining it would go over the
// cap. Callers hold s.mu.
func (s *Server) makeRoom(team string, p *Player) {
	if c := s.teamCap(); c > 0 {
		if all, bots := s.onTeam(team, p); all >= c && bots > 0 {
			s.kickAI(team, 1)
		}
	}
}

// autoTeam is where "auto" puts p: the smaller side if it has room, else the
// other one, else "" (both full). Callers hold s.mu.
func (s *Server) autoTeam(p *Player) string {
	t := s.smallerTeam(p)
	if !s.capFull(t, p) {
		return t
	}
	if o := otherTeam(t); !s.capFull(o, p) && !s.teamFull(o, p) {
		return o
	}
	return ""
}

// refuseTeam tells p it can't join team (or any side, team "") and why.
func (s *Server) refuseTeam(p *Player, team string, capped bool) {
	m := map[string]any{"t": "teamfull", "team": team}
	if capped {
		m["cap"] = s.teamCap()
		m["map"] = s.Map.Name
	} else {
		m["limit"] = s.limitTeams() // mp_limitteams: the client names it and how to lift it
	}
	b, _ := json.Marshal(m)
	trySend(p, b)
}

// trimTeams moves players off every side over the cap: playing bots first
// (newest first), then the humans who joined that side last, to spectators.
// Then autobalance evens the sides. Callers hold s.mu.
func (s *Server) trimTeams() {
	c := s.teamCap()
	if c <= 0 {
		return
	}
	for _, team := range content.Teams {
		for {
			all, bots := s.onTeam(team, nil)
			if all <= c {
				break
			}
			if bots > 0 {
				s.kickAI(team, 1)
				continue
			}
			var cands []*Player
			for _, o := range s.players {
				if !o.Bot && o.Team == team {
					cands = append(cands, o)
				}
			}
			p := slices.MaxFunc(cands, func(x, y *Player) int {
				if r := cmp.Compare(x.teamSeq, y.teamSeq); r != 0 {
					return r
				}
				return cmp.Compare(x.ID, y.ID)
			})
			s.dropBomb(p)
			p.Team = content.Spectator
			s.respawn(p)
			why := fmt.Sprintf("%s has room for %d per team", s.Map.Name, c)
			s.notice(p, "You were moved to spectators: "+why+". Open the team menu to join when there is room.")
			s.noticeAll(p.Name + " moved to spectators: " + why)
		}
	}
	s.balanceTeams()
}

// spawnFor hands p one of its team's spawn points: round-robin, skipping
// points where another live player stands (within spawnClear); if every
// point is taken, the least crowded one. Callers hold s.mu.
func (s *Server) spawnFor(p *Player) Vec3 {
	list := s.Map.TeamSpawns[p.Team]
	if len(list) == 0 {
		list = s.Map.Spawns
	}
	return s.spawnFrom(p, p.Team, list)
}

// spawnFrom is spawnFor over list, its round-robin kept under key (a team, or
// a tickets-mode spawn point). Callers hold s.mu.
func (s *Server) spawnFrom(p *Player, key string, list [][3]float64) Vec3 {
	v, _ := s.spawnFromClear(p, key, list)
	return v
}

// spawnFromClear is spawnFrom, and whether the spot it took is clear of
// everyone (spawnClear); a full list gives its least crowded spot, not clear.
func (s *Server) spawnFromClear(p *Player, key string, list [][3]float64) (Vec3, bool) {
	var others []Vec3
	for _, o := range s.players {
		if o == p || o.Dead || o.Team == content.Spectator || o.Team == "" || s.placing[o] {
			continue
		}
		others = append(others, o.Pos)
	}
	// nearest is the xz distance from point q to the closest other player
	// standing level with it (a hull's height).
	nearest := func(q [3]float64) float64 {
		d := math.Inf(1)
		for _, o := range others {
			if math.Abs(o[1]-q[1]) < 1.8 {
				d = math.Min(d, math.Hypot(o[0]-q[0], o[2]-q[2]))
			}
		}
		return d
	}
	start := s.spawnNext[key]
	best, bestD := start%len(list), -1.0
	for k := range len(list) {
		i := (start + k) % len(list)
		d := nearest(list[i])
		if d >= spawnClear {
			best = i
			break
		}
		if d > bestD {
			best, bestD = i, d
		}
	}
	s.spawnNext[key] = best + 1
	delete(s.placing, p)
	return Vec3(list[best]), bestD < 0
}

// placeAll marks ps as about to be (re)spawned together (a round start, a
// restart): where they stand now doesn't take a spawn point. Callers hold s.mu.
func (s *Server) placeAll(ps []*Player) {
	s.placing = map[*Player]bool{}
	for _, p := range ps {
		s.placing[p] = true
	}
}
