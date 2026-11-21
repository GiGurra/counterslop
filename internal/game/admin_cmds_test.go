package game

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
)

// texts returns the text of every queued print/notice message for p.
func texts(p *Player) []string {
	var out []string
	for _, m := range msgs(p) {
		if s, ok := m["text"].(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func hasText(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestAdminCommandReplies(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		args []string
		want string // substring of some print/notice sent to the admin
	}{
		{"status", nil, "map de_slop, 2 players, admin: OPEN"},
		{"kick", nil, "kick <name|id>"},
		{"ban", []string{"nobody"}, "no such player: nobody"},
		{"changelevel", nil, "maps: de_slop"},
		{"changelevel", []string{"de_nope"}, "changelevel <map>"},
		{"give", nil, "weapons: stub prompt linter codereview rubberduck hotfix autocomplete copypaste tokenburner contextwindow mergeconflict stacktrace smoketest keyboard"},
		{"give", []string{"bfg"}, "give <weapon>"},
		{"say_admin", []string{"hello", "all"}, "[admin] slopper1: hello all"},
		{"bot_teleport", []string{"1", "2"}, "bot_teleport x y z [yaw]"},
		{"bot_teleport", []string{"1", "2", "NaN"}, "bot_teleport x y z [yaw]"},
		{"sv_gravity", nil, `"sv_gravity" = "22"`},
		{"sv_gravity", []string{"high"}, "numeric value expected"},
		{"mp_roundtime", []string{"60"}, "slopper1 changed mp_roundtime to 60"},
		{"frobnicate", nil, "unknown admin command frobnicate"},
		{"restart", nil, "slopper1 restarted the match"},
		{"bot_add", nil, "joined"},
		{"bot_kick", nil, "kicked 0 bot(s)"},
		{"bot_freeze", []string{"1"}, "bot_freeze 1"},
		{"bot_freeze", nil, "bot_freeze 0"},
		{"bot_crouch", []string{"1"}, "bot_crouch 1"},
	} {
		t.Run(tc.cmd+" "+strings.Join(tc.args, " "), func(t *testing.T) {
			s := authServer(t)
			p := s.join("1.1.1.1")
			s.join("2.2.2.2")
			msgs(p)
			s.adminCommand(p, tc.cmd, tc.args)
			if got := texts(p); !hasText(got, tc.want) {
				t.Fatalf("replies %q, want one containing %q", got, tc.want)
			}
		})
	}
}

func TestAdminHelpListsCommandsAndCvars(t *testing.T) {
	s := NewServer()
	p := s.join("1.1.1.1")
	msgs(p)
	s.adminCommand(p, "help", nil)
	m := reply(t, p)
	secs := m["sections"].([]any)
	if m["t"] != "help" || len(secs) != len(adminHelp)+1 {
		t.Fatalf("help: %v", m)
	}
	cv := secs[len(secs)-1].(map[string]any)["rows"].([]any)
	if len(cv) != len(defaultCvars) {
		t.Fatalf("%d cvar rows, want %d", len(cv), len(defaultCvars))
	}
	prev := ""
	for _, r := range cv {
		row := r.(map[string]any)
		name := row["name"].(string)
		if name <= prev || row["value"] != defaultCvars[name] || row["desc"] == "" {
			t.Fatalf("cvar row %v (after %q)", row, prev)
		}
		prev = name
	}
	s.adminCommand(p, "", nil) // empty command is help too
	if m := reply(t, p); m["t"] != "help" {
		t.Fatalf("empty command: %v", m)
	}
}

func TestCvarChangeBroadcasts(t *testing.T) {
	s := NewServer()
	a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
	msgs(a)
	msgs(b)
	s.adminCommand(a, "mp_respawntime", []string{"0.5"})
	if s.cvars["mp_respawntime"] != "0.5" || s.respawnDelay() != 500*time.Millisecond {
		t.Fatalf("cvar %q delay %v", s.cvars["mp_respawntime"], s.respawnDelay())
	}
	cv := ofType(b, "cvars")
	if len(cv) != 1 || cv[0]["cvars"].(map[string]any)["mp_respawntime"] != "0.5" || cv[0]["openAdmin"] != true {
		t.Fatalf("other player's cvars: %v", cv)
	}
}

func TestStatusListsPlayers(t *testing.T) {
	s := NewServer()
	s.AdminPassword = "pw"
	a := s.join("1.1.1.1")
	a.admin = true
	s.join("2.2.2.2")
	s.addBot()
	msgs(a)
	s.adminCommand(a, "status", nil)
	got := texts(a)
	if len(got) != 4 || !strings.Contains(got[0], "3 players, admin: password") {
		t.Fatalf("status %q", got)
	}
	if !strings.Contains(got[1], "slopper1") || !strings.Contains(got[1], "1.1.1.1") || !strings.HasSuffix(got[1], " admin") {
		t.Fatalf("admin row %q", got[1])
	}
	if strings.Contains(got[2], "admin") || !strings.Contains(got[2], "KB/s") {
		t.Fatalf("player row %q", got[2])
	}
	if !strings.HasSuffix(got[3], " bot") || strings.Contains(got[3], "KB/s") {
		t.Fatalf("bot row %q", got[3])
	}
}

func TestKickAndBan(t *testing.T) {
	s := NewServer()
	admin := s.join("1.1.1.1")
	for _, tc := range []struct {
		args     []string
		ban      bool
		wantMins float64
	}{
		{[]string{"slopper2"}, false, 0},
		{[]string{"#3"}, true, 60},
		{[]string{"4", "2.5"}, true, 2.5},
		{[]string{"LOPPER5", "-1"}, true, 60}, // case-insensitive substring; bad minutes = default
	} {
		v := s.join("10.0.0." + tc.args[0][len(tc.args[0])-1:])
		cmd := "kick"
		if tc.ban {
			cmd = "ban"
		}
		before := time.Now()
		s.adminCommand(admin, cmd, tc.args)
		if s.players[v.ID] != nil || v.kick != nil {
			t.Fatalf("%s %v: player still on the server", cmd, tc.args)
		}
		if last := msgs(v); last[len(last)-1]["t"] != "kicked" {
			t.Fatalf("victim's last message %v", last[len(last)-1])
		}
		until, banned := s.bans[v.ip]
		if banned != tc.ban {
			t.Fatalf("%s %v: banned=%v", cmd, tc.args, banned)
		}
		if tc.ban {
			d := until.Sub(before).Minutes()
			if d < tc.wantMins-0.01 || d > tc.wantMins+0.01 {
				t.Fatalf("ban for %.2f min, want %g", d, tc.wantMins)
			}
		}
	}
	// Bans expire.
	s.bans["7.7.7.7"] = time.Now().Add(-time.Second)
	if s.banned("7.7.7.7") || len(s.bans) != 3 {
		t.Fatalf("expired ban still active (%d bans)", len(s.bans))
	}
	// A bot has no IP: ban just kicks.
	bot := s.addBot()
	s.adminCommand(admin, "ban", []string{"#" + strconv.Itoa(bot.ID)})
	if s.players[bot.ID] != nil || len(s.bans) != 3 {
		t.Fatal("bot ban")
	}
}

func TestFindPlayer(t *testing.T) {
	s := NewServer()
	a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
	a.Name, b.Name = "Alice", "alicesmith"
	for _, tc := range []struct {
		key  string
		want *Player
	}{
		{"1", a},
		{"#2", b},
		{"alice", a}, // exact (any case) beats substring
		{"SMITH", b},
		{"zed", nil},
		{"99", nil},
	} {
		if got := s.findPlayer(tc.key); got != tc.want {
			t.Errorf("findPlayer(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestGiveWeapon(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	msgs(p)
	s.adminCommand(p, "give", []string{"codereview"})
	if h := p.inv.Slots["primary"]; h == nil || h.ID != "codereview" || h.Ammo != 5 || p.inv.Current != "primary" {
		t.Fatalf("after give: %+v cur %s", h, p.inv.Current)
	}
	if len(ofType(p, "inv")) != 1 {
		t.Fatal("no inventory update")
	}
	// Without a registry, give just prints usage.
	s2 := NewServer()
	q := s2.join("1.1.1.1")
	msgs(q)
	s2.adminCommand(q, "give", []string{"stub"})
	if got := texts(q); !hasText(got, "give <weapon>; weapons: ") {
		t.Fatalf("no registry: %q", got)
	}
}

func TestRestartResetsEveryone(t *testing.T) {
	s := authServer(t)
	a, b := s.join("1.1.1.1"), s.join("2.2.2.2")
	a.Team, b.Team = content.Vibe, content.Artisan
	s.queueCmds(a, []Cmd{{Seq: 1}, {Seq: 2}})
	a.HP, a.Kills, a.Deaths, a.Dead = 5, 3, 2, true
	b.Kills = 7
	delete(a.inv.Slots, "primary")
	bot := s.addBot()
	botPos := bot.Pos
	msgs(a)
	s.adminCommand(a, "restart", nil)
	for _, p := range []*Player{a, b} {
		if p.HP != maxHP || p.Dead || p.Kills != 0 || p.Deaths != 0 || p.inv.Slots["primary"] == nil {
			t.Fatalf("player %d after restart: hp %d dead %v k %d d %d", p.ID, p.HP, p.Dead, p.Kills, p.Deaths)
		}
		if len(ofType(p, "correct")) != 1 {
			t.Fatalf("player %d got no correct", p.ID)
		}
	}
	queued := len(a.cmds)
	if a.body == nil || [3]float64(a.Pos) != a.body.Pos {
		t.Fatalf("command-driven body not reset: %+v", a.body)
	}
	if queued == 0 {
		t.Fatal("test setup: a has no queued commands")
	}
	// Queued commands stay: the client replays them from the spawn too
	// (dropping them made server and client disagree for a tick: maps-vote-console).
	if bot.Pos != botPos {
		t.Fatal("restart moved a bot")
	}
}

func TestChangelevelResetsItemsAndSendsMap(t *testing.T) {
	s := authServer(t)
	p := s.join("1.1.1.1")
	s.spawnItem(*s.Weapons.newHeld("stub"), Vec3{1, 0, 1}, -1)
	msgs(p)
	s.adminCommand(p, "changelevel", []string{"de_slop"})
	if len(s.items) != 0 || len(s.itemRespawn) != len(s.Map.Items) {
		t.Fatalf("items %d respawns %d", len(s.items), len(s.itemRespawn))
	}
	all := msgs(p)
	types := []string{}
	for _, m := range all {
		types = append(types, m["t"].(string))
	}
	if !strings.Contains(strings.Join(types, ","), "notice,map,inv,correct") {
		t.Fatalf("messages %v", types)
	}
	s.tickItems(time.Now())
	if len(s.items) != 1 || s.items[0].Weapon != "codereview" || s.items[0].mapSlot != 0 {
		t.Fatalf("map item not respawned: %+v", s.items)
	}
}

func TestBotTeleportAndCrouch(t *testing.T) {
	s := NewServer()
	p := s.join("1.1.1.1")
	a, b := s.addBot(), s.addBot()
	s.adminCommand(p, "bot_teleport", []string{"3", "0", "-4", "1.5"})
	for _, bot := range []*Player{a, b} {
		if bot.Pos != (Vec3{3, 0, -4}) || bot.Yaw != 1.5 || bot.brain.target != bot.Pos {
			t.Fatalf("bot %d at %v yaw %v target %v", bot.ID, bot.Pos, bot.Yaw, bot.brain.target)
		}
	}
	s.adminCommand(p, "bot_crouch", []string{"1"})
	if !a.Crouch || !b.brain.holdCrouch {
		t.Fatal("bot_crouch 1")
	}
	s.adminCommand(p, "bot_kick", nil)
	if countBots(s) != 0 || s.players[p.ID] == nil {
		t.Fatal("bot_kick")
	}
}

func TestLoginBackoffAndWindow(t *testing.T) {
	s := NewServer()
	s.AdminPassword = "pw"
	p := s.join("1.1.1.1")
	// Old failures outside the window don't count towards the limit.
	s.loginFails[p.ip] = &loginFails{times: []time.Time{time.Now().Add(-2 * loginFailWindow), time.Now().Add(-2 * loginFailWindow)}}
	for range loginFailLimit - 1 {
		s.adminLogin(p, "bad")
	}
	if f := s.loginFails[p.ip]; len(f.times) != loginFailLimit-1 || !f.blockedUntil.IsZero() {
		t.Fatalf("fails %d blocked %v", len(f.times), f.blockedUntil)
	}
	s.adminLogin(p, "bad")
	if f := s.loginFails[p.ip]; f.blockedUntil.Before(time.Now().Add(loginBackoff - time.Second)) {
		t.Fatalf("no backoff after %d fails", loginFailLimit)
	}
	msgs(p)
	s.adminLogin(p, "pw")
	if got := texts(p); p.admin || !hasText(got, "too many failed logins") {
		t.Fatalf("during backoff: admin %v, %q", p.admin, got)
	}
	s.loginFails[p.ip].blockedUntil = time.Now().Add(-time.Second)
	s.adminLogin(p, "pw")
	if !p.admin || s.loginFails[p.ip].times != nil {
		t.Fatal("login after the backoff")
	}
	// With no password everyone is admin already.
	o := NewServer()
	q := o.join("1.1.1.1")
	msgs(q)
	o.adminLogin(q, "x")
	if got := texts(q); !hasText(got, "already admin") {
		t.Fatalf("open admin login: %q", got)
	}
}
