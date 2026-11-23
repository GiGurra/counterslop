package game

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// joinTeam joins a new human to team and returns them with their first reply.
func joinTeam(t *testing.T, s *Server, team string) (*Player, map[string]any) {
	t.Helper()
	p := s.join("127.0.0.1")
	p.ready = true
	s.handle(p, inbound{T: "team", Team: team})
	return p, teamReply(t, p)
}

// teamReply is the answer to a team pick: the spawn, or teamfull.
func teamReply(t *testing.T, p *Player) map[string]any {
	t.Helper()
	for _, m := range msgs(p) {
		if m["t"] == "spawn" || m["t"] == "teamfull" {
			return m
		}
	}
	t.Fatal("no spawn or teamfull reply")
	return nil
}

// capServer is a test server on de_slop with a cap of c per team.
func capServer(t *testing.T, c int) *Server {
	t.Helper()
	s := authServer(t)
	s.Test = true
	s.Map.MaxPlayers = 2 * c
	return s
}

func TestTeamCapRefusesAFullSide(t *testing.T) {
	s := capServer(t, 2)
	for _, team := range []string{content.Vibe, content.Artisan, content.Vibe, content.Artisan} {
		if _, m := joinTeam(t, s, team); m["t"] != "spawn" {
			t.Fatalf("join %s: %v", team, m)
		}
	}
	// 2 vs 2 on a 2-per-team map: either side is full, and says why.
	p, m := joinTeam(t, s, content.Vibe)
	if m["t"] != "teamfull" || m["cap"] != 2.0 || m["map"] != "de_slop" || p.Team != content.Spectator {
		t.Fatalf("joined a full side: %v, team %s", m, p.Team)
	}
	// auto: both full, stays spectating, told so.
	s.handle(p, inbound{T: "team", Team: "auto"})
	if m := teamReply(t, p); m["t"] != "teamfull" || m["team"] != "" || p.Team != content.Spectator {
		t.Fatalf("auto with both sides full: %v, team %s", m, p.Team)
	}
	// Room again once someone leaves.
	for _, o := range s.ordered() {
		if o.Team == content.Artisan {
			s.handle(o, inbound{T: "team", Team: content.Spectator})
			break
		}
	}
	s.handle(p, inbound{T: "team", Team: "auto"})
	if m := teamReply(t, p); m["t"] != "spawn" || p.Team != content.Artisan {
		t.Fatalf("auto with room on artisan: %v, team %s", m, p.Team)
	}
}

func TestTeamCapAutoPicksTheSideWithRoom(t *testing.T) {
	s := capServer(t, 2)
	joinTeam(t, s, content.Vibe)
	art, _ := joinTeam(t, s, content.Artisan)
	joinTeam(t, s, content.Vibe)
	s.handle(art, inbound{T: "team", Team: content.Spectator}) // 2 vs 0 now
	drain(art)
	// 2 vs 0: the smaller side (artisan) has room, the full one doesn't.
	if p, m := joinTeam(t, s, "auto"); m["t"] != "spawn" || p.Team != content.Artisan {
		t.Fatalf("auto: %v on %s", m, p.Team)
	}
	// A map change lowered the cap under both sides (before trimming): auto has nowhere to go.
	s.Map.MaxPlayers = 2
	if got := s.autoTeam(&Player{}); got != "" {
		t.Fatalf("autoTeam %q with both sides over the cap", got)
	}
}

func TestTeamCapBotsMakeRoomForHumans(t *testing.T) {
	s := capServer(t, 2)
	b1, b2 := s.addAI(content.Vibe), s.addAI(content.Vibe)
	if b1 == nil || b2 == nil {
		t.Fatal("bots refused below the cap")
	}
	if s.addAI(content.Vibe) != nil {
		t.Fatal("a third bot joined a 2-per-team side")
	}
	// A human joining the bot-full side takes the newest bot's place (and the
	// imbalance rule counts the side after that: 2 vs 0, not 3 vs 0).
	s.addAI(content.Artisan)
	p, m := joinTeam(t, s, content.Vibe)
	if m["t"] != "spawn" || p.Team != content.Vibe {
		t.Fatalf("human refused by bots: %v", m)
	}
	if _, ok := s.players[b2.ID]; ok {
		t.Fatal("the newest bot stayed")
	}
	if _, ok := s.players[b1.ID]; !ok {
		t.Fatal("the older bot went")
	}
	if all, bots := s.onTeam(content.Vibe, nil); all != 2 || bots != 1 {
		t.Fatalf("vibe %d players, %d bots", all, bots)
	}
	// Humans only: a second human takes the last bot's place, a third is refused.
	joinTeam(t, s, content.Artisan)
	if _, m := joinTeam(t, s, content.Vibe); m["t"] != "spawn" {
		t.Fatalf("second human: %v", m)
	}
	if _, m := joinTeam(t, s, content.Vibe); m["t"] != "teamfull" {
		t.Fatalf("third human on a 2-per-team side: %v", m)
	}
}

func TestBotQuotaStopsAtTheCap(t *testing.T) {
	s := capServer(t, 3)
	s.cvars["bot_quota"] = "20"
	s.fillQuota()
	if v, _ := s.onTeam(content.Vibe, nil); v != 3 {
		t.Fatalf("vibe %d, want 3", v)
	}
	if a, _ := s.onTeam(content.Artisan, nil); a != 3 {
		t.Fatalf("artisan %d, want 3", a)
	}
	// bot_add_<team> on a full side says so instead of adding.
	admin := s.join("1.1.1.1")
	admin.admin = true
	s.adminCommand(admin, "bot_add_vibe", nil)
	if out := drain(admin); !strings.Contains(out, "full (3 per team on de_slop)") {
		t.Fatalf("bot_add_vibe on a full side: %s", out)
	}
	if v, _ := s.onTeam(content.Vibe, nil); v != 3 {
		t.Fatalf("vibe %d after bot_add_vibe, want 3", v)
	}
}

func TestSmallerMapTrimsBotsThenLatestJoiners(t *testing.T) {
	small := ""
	for _, n := range maps.Order {
		if small == "" || maps.All[n]().MaxPlayers < maps.All[small]().MaxPlayers {
			small = n
		}
	}
	c := maps.All[small]().MaxPlayers / 2
	s := capServer(t, 50)
	var vibes []*Player
	for range c + 2 {
		p, _ := joinTeam(t, s, content.Vibe)
		vibes = append(vibes, p)
		joinTeam(t, s, content.Artisan)
	}
	// The first vibe re-joins last: "latest joined" is by team join, not connection.
	first := vibes[0]
	s.handle(first, inbound{T: "team", Team: content.Spectator})
	s.handle(first, inbound{T: "team", Team: content.Vibe})
	if first.Team != content.Vibe {
		t.Fatal("rejoin refused")
	}
	s.addAI(content.Vibe)
	s.addAI(content.Vibe)
	for _, p := range s.players {
		drain(p)
	}
	if !s.changeLevel(small) {
		t.Fatal("changelevel")
	}
	for _, team := range content.Teams {
		all, bots := s.onTeam(team, nil)
		if all != c || bots != 0 {
			t.Fatalf("%s on %s: %d players (%d bots), want %d humans", team, small, all, bots, c)
		}
	}
	// vibes[0] re-joined last, then vibes[c+1]: those two went.
	for _, p := range []*Player{first, vibes[c+1]} {
		if p.Team != content.Spectator {
			t.Fatalf("player %d (latest joiner) still on %s", p.ID, p.Team)
		}
		out := drain(p)
		want := "moved to spectators: " + small + " has room for "
		if !strings.Contains(out, want) {
			t.Fatalf("player %d not told: %s", p.ID, out)
		}
	}
	if vibes[1].Team != content.Vibe || vibes[c].Team != content.Vibe {
		t.Fatal("an earlier joiner was moved")
	}
	if out := drain(vibes[1]); !strings.Contains(out, first.Name+" moved to spectators") || !strings.Contains(out, vibes[c+1].Name+" moved to spectators") {
		t.Fatalf("others not told: %s", out)
	}
	// They can rejoin once there is room.
	s.handle(vibes[1], inbound{T: "team", Team: content.Spectator})
	s.handle(first, inbound{T: "team", Team: content.Vibe})
	if first.Team != content.Vibe {
		t.Fatal("couldn't rejoin with room")
	}
}

func TestTrimRebalancesTheSides(t *testing.T) {
	s := capServer(t, 3)
	s.cvars["mp_autoteambalance"] = "1"
	for i := range 6 {
		p := s.join("127.0.0.1")
		p.Team = content.Vibe
		if i == 0 {
			p.Team = content.Artisan
		}
		s.teamSeqN++
		p.teamSeq = s.teamSeqN
	}
	// 5 vs 1 on a 3-per-team map: two vibes to spectators, then autobalance
	// moves one more over: 2 vs 2.
	s.trimTeams()
	v, _ := s.onTeam(content.Vibe, nil)
	a, _ := s.onTeam(content.Artisan, nil)
	if v != 2 || a != 2 {
		t.Fatalf("after trim %d vs %d, want 2 vs 2", v, a)
	}
}

// overlapping reports two live players whose hulls overlap.
func overlapping(s *Server) (string, bool) {
	ps := s.ordered()
	for i, p := range ps {
		for _, q := range ps[i+1:] {
			if p.Team == content.Spectator || q.Team == content.Spectator || p.Dead || q.Dead {
				continue
			}
			if math.Hypot(p.Pos[0]-q.Pos[0], p.Pos[2]-q.Pos[2]) < 2*0.4 && math.Abs(p.Pos[1]-q.Pos[1]) < 1.8 {
				return p.Name + " / " + q.Name, true
			}
		}
	}
	return "", false
}

// Every map: a full server (both sides at the cap) spawns with no two hulls
// overlapping, at a round start and on mid-round respawns.
func TestFullTeamsSpawnApart(t *testing.T) {
	for _, name := range maps.Order {
		t.Run(name, func(t *testing.T) {
			s := authServer(t)
			s.Test = true
			s.changeLevel(name)
			c := s.teamCap()
			for range c {
				joinTeam(t, s, content.Vibe)
				joinTeam(t, s, content.Artisan)
			}
			if v, _ := s.onTeam(content.Vibe, nil); v != c {
				t.Fatalf("vibe %d, want %d", v, c)
			}
			if who, ok := overlapping(s); ok {
				t.Fatalf("joining: %s overlap", who)
			}
			for r := range 3 {
				s.startRound(s.clock())
				if who, ok := overlapping(s); ok {
					t.Fatalf("round %d: %s overlap", r, who)
				}
			}
			// Mid-round respawns (deathmatch): each dead player comes back on a free point.
			for i, p := range s.ordered() {
				if i%2 == 0 {
					p.Dead = true
				}
			}
			for _, p := range s.ordered() {
				if p.Dead {
					s.respawn(p)
					if who, ok := overlapping(s); ok {
						t.Fatalf("respawn: %s overlap", who)
					}
				}
			}
		})
	}
}

// With every point taken, spawnFor still answers: the least crowded point.
func TestSpawnForFallsBackToLeastCrowded(t *testing.T) {
	s := authServer(t)
	s.Test = true
	pts := s.Map.TeamSpawns[content.Vibe]
	s.Map.MaxPlayers = 2 * len(pts)
	for range len(pts) {
		joinTeam(t, s, content.Vibe)
		joinTeam(t, s, content.Artisan)
	}
	// Stand everyone on a point, one nudged 0.5 m off: that point is the least crowded.
	i := 0
	for _, p := range s.ordered() {
		if p.Team == content.Vibe {
			p.Pos = Vec3(pts[i])
			if i == 2 {
				p.Pos[0] += 0.5
			}
			i++
		}
	}
	extra := &Player{ID: 999, Team: content.Vibe}
	if got := s.spawnFor(extra); got != Vec3(pts[2]) {
		t.Fatalf("fallback %v, want %v", got, pts[2])
	}
}

// A command a player sent just before the server took their switch to
// spectators is dropped there, but it counts as received: after they rejoin a
// side, nothing waits for it (the lockstep harness waited forever: sim/cases
// team-cap-map-switch, now and then).
func TestSpectatorCommandsCountAsReceived(t *testing.T) {
	s := authServer(t)
	s.Test = true
	p, _ := joinTeam(t, s, content.Vibe)
	joinTeam(t, s, content.Artisan)
	p.Name = "dave"
	s.queueCmds(p, []Cmd{{Seq: 1}, {Seq: 2}})
	s.handle(p, inbound{T: "team", Team: content.Spectator})
	s.queueCmds(p, []Cmd{{Seq: 3}}) // in flight when the switch happened
	if len(p.cmds) != 0 && p.Team == content.Spectator {
		t.Fatal("a spectator's command was queued")
	}
	s.handle(p, inbound{T: "team", Team: content.Vibe})
	if err := s.WaitCommand("dave", 3, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	s.queueCmds(p, []Cmd{{Seq: 4}})
	if len(p.cmds) != 1 || p.cmds[0].Seq != 4 {
		t.Fatalf("queue after rejoin %v", p.cmds)
	}
}
