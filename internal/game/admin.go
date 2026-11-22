package game

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/sim"
)

// Admin, CS rcon style. With AdminPassword set, admin_login <pw> (or rcon
// <pw-less command> after logging in) unlocks the admin commands for that
// connection. With no password configured, everyone is admin: this is an open
// experimental server, and every join is told so. Every admin command is logged
// with who ran it, and announced to everyone.

const (
	loginFailLimit  = 5
	loginFailWindow = time.Minute
	loginBackoff    = 2 * time.Minute
)

// ServerCvars are the server-side settings admins change. Gameplay code reads
// them from here; clients get them in welcome and on every change.
var defaultCvars = map[string]string{
	"mp_friendlyfire":        "0",
	"mp_roundtime":           "115",
	"mp_freezetime":          "15",
	"mp_respawntime":         "3",
	"mp_respawn":             "auto",
	"mp_c4timer":             "40",
	"mp_gpus_to_win":         "2",
	"mp_round_restart_delay": "7",
	"sv_gravity":             "22",
	"sv_cheats":              "0",
	"sv_maxunlag":            "0.5",
	"weapon_spread":          "1",
	"weapon_recoil_scale":    "1",
	"sv_showimpacts":         "0",
	"mp_maprotation":         strings.Join(maps.Order, ","),
	"bot_quota":              "0",
	"bot_difficulty":         "1",
	"mp_buytime":             "20",
	"mp_afk_rounds":          "auto",
	"mp_autoteambalance":     "auto",
	"mp_maxrounds":           "24",
	"mp_overtime_enable":     "1",
	"mp_overtime_maxrounds":  "6",
	"mp_halftime_duration":   "10",
	"mp_match_restart_delay": "20",
	"mp_warmup_time":         "60",
}

type loginFails struct {
	times        []time.Time
	blockedUntil time.Time
}

func (s *Server) openAdmin() bool { return s.AdminPassword == "" }

func (s *Server) isAdmin(p *Player) bool { return s.openAdmin() || p.admin }

// clientIP prefers Cloudflare's header (the server sits behind a tunnel).
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) print(p *Player, text string) {
	b, _ := json.Marshal(map[string]string{"t": "print", "text": text})
	trySend(p, b)
}

func (s *Server) announce(text string) {
	b, _ := json.Marshal(map[string]string{"t": "notice", "text": text})
	for _, o := range s.players {
		trySend(o, b)
	}
}

func (s *Server) cvarsMsg() []byte {
	b, _ := json.Marshal(map[string]any{"t": "cvars", "cvars": s.cvars, "openAdmin": s.openAdmin()})
	return b
}

// motd is sent right after welcome.
func (s *Server) motd(p *Player) {
	trySend(p, s.cvarsMsg())
	if s.openAdmin() {
		s.print(p, "No admin password set: everyone is admin on this experimental server (try: rcon help)")
	}
}

// adminLogin checks a password with per-IP rate limiting. Callers hold s.mu.
func (s *Server) adminLogin(p *Player, pw string) {
	if s.openAdmin() {
		s.print(p, "no admin password is set: you're already admin")
		return
	}
	now := s.clock()
	f := s.loginFails[p.ip]
	if f == nil {
		f = &loginFails{}
		s.loginFails[p.ip] = f
	}
	if now.Before(f.blockedUntil) {
		slog.Warn("admin login blocked", "ip", p.ip, "id", p.ID)
		s.print(p, fmt.Sprintf("too many failed logins, try again in %ds", int(f.blockedUntil.Sub(now).Seconds())+1))
		return
	}
	if subtle.ConstantTimeCompare([]byte(pw), []byte(s.AdminPassword)) == 1 {
		p.admin = true
		f.times = nil
		slog.Info("admin login", "ip", p.ip, "id", p.ID, "name", p.Name)
		s.print(p, "admin login ok. rcon help lists the commands")
		return
	}
	kept := f.times[:0]
	for _, t := range f.times {
		if now.Sub(t) < loginFailWindow {
			kept = append(kept, t)
		}
	}
	f.times = append(kept, now)
	slog.Warn("admin login failed", "ip", p.ip, "id", p.ID, "name", p.Name, "recent", len(f.times))
	if len(f.times) >= loginFailLimit {
		f.blockedUntil = now.Add(loginBackoff)
		f.times = nil
	}
	s.print(p, "bad password")
}

type helpRow struct {
	Name  string `json:"name"`
	Args  string `json:"args,omitempty"`
	Value string `json:"value,omitempty"`
	Desc  string `json:"desc"`
}

type helpSection struct {
	Title string    `json:"title"`
	Rows  []helpRow `json:"rows"`
}

var adminHelp = []helpSection{
	{"players", []helpRow{
		{Name: "status", Desc: "players, IPs and admin mode"},
		{Name: "kick", Args: "<name|id>", Desc: "kick a player"},
		{Name: "ban", Args: "<name|id> [minutes]", Desc: "ban by IP (in memory, default 60 min)"},
		{Name: "give", Args: "<weapon>", Desc: "give yourself a weapon"},
		{Name: "setpos", Args: "x y z [yaw]", Desc: "move yourself there"},
	}},
	{"match", []helpRow{
		{Name: "changelevel", Args: "<map>", Desc: "change the map (everyone respawns)"},
		{Name: "mapvote", Args: "[seconds|end]", Desc: "vote on the next map (3 from mp_maprotation, same category as this map); end: count it now"},
		{Name: "restart", Desc: "restart the match: spawns, loadouts, scores"},
		{Name: "mp_warmup_end", Desc: "end the warmup now: the match starts (needs a player on each side)"},
		{Name: "say_admin", Args: "<msg>", Desc: "announce to everyone"},
	}},
	{"bots", []helpRow{
		{Name: "bot_add", Desc: "add a target-practice bot"},
		{Name: "bot_add_vibe", Desc: "add a playing bot to the Vibe Coders"},
		{Name: "bot_add_artisan", Desc: "add a playing bot to the Artisans"},
		{Name: "bot_kick", Desc: "remove all bots (target practice and playing)"},
		{Name: "bot_freeze", Args: "0|1", Desc: "stop the bots where they stand"},
		{Name: "bot_crouch", Args: "0|1", Desc: "keep the bots crouched"},
		{Name: "bot_teleport", Args: "x y z [yaw]", Desc: "move every bot"},
		{Name: "bot_patrol", Args: "x0 z0 x1 z1 | off", Desc: "target bots walk back and forth between two points, never stopping (tests)"},
	}},
}

var cvarHelp = map[string][2]string{
	"mp_friendlyfire":        {"0|1", "team damage"},
	"mp_roundtime":           {"<seconds>", "round length"},
	"mp_freezetime":          {"<seconds>", "freeze/buy time at round start"},
	"mp_respawntime":         {"<seconds>", "time dead before respawning (deathmatch / warmup)"},
	"mp_respawn":             {"0|1|auto", "1: deathmatch (timed respawns, no rounds); 0: rounds once both sides have players; auto: rounds (deathmatch on test-harness servers)"},
	"mp_c4timer":             {"<seconds>", "CI timer: how long the planted Big Diff ticks"},
	"mp_gpus_to_win":         {"<n>", "GPU Drop: GPUs the Vibe Coders must secure to win the round (at most the map's stock)"},
	"mp_buytime":             {"<seconds>", "buy time after the freeze (in your buy zone)"},
	"mp_afk_rounds":          {"<n>|auto", "move players to spectators after n whole rounds without input (0 = never; auto: 2, off on test-harness servers)"},
	"mp_autoteambalance":     {"0|1|auto", "at round start, move the newest players (bots first) off a side with 2+ more players (auto: on, off on test-harness servers)"},
	"mp_maxrounds":           {"<n>", "rounds in a match (24 = MR12: first to 13); sides swap at half"},
	"mp_overtime_enable":     {"0|1", "a tie after mp_maxrounds goes to overtime (0: a draw)"},
	"mp_overtime_maxrounds":  {"<n>", "rounds per overtime (6: first to 4 of it); sides swap at its half"},
	"mp_halftime_duration":   {"<seconds>", "the pause at halftime before the sides swap"},
	"mp_match_restart_delay": {"<seconds>", "the match-end screen (and map vote) before the next match"},
	"mp_warmup_time":         {"<seconds>", "warmup once both sides have a player (respawns, free buys)"},
	"bot_quota":              {"<n>", "playing bots fill the teams up to n players in total (humans replace them)"},
	"bot_difficulty":         {"0-3", "playing bots: reaction time, aim error and turn speed"},
	"mp_round_restart_delay": {"<seconds>", "round-end banner time before the next round"},
	"sv_gravity":             {"<m/s²>", "gravity"},
	"sv_cheats":              {"0|1", "cheats for everyone"},
	"sv_maxunlag":            {"<seconds>", "lag compensation: how far back a shot may be traced (0 = off)"},
	"sv_showimpacts":         {"0|1", "show each shot as the server traced it: its hitboxes (blue) and impact next to yours (red)"},
	"weapon_spread":          {"0|1", "debug: 0 = every shot goes exactly through the crosshair (server and clients): no spread, no recoil"},
	"weapon_recoil_scale":    {"<x>", "spray pattern strength (0 = no recoil, 1 = as designed)"},
	"mp_maprotation":         {"<map,map,...>|all", "maps the vote offers (in order after the current map; only the current map's category, Training or Competitive, unless that leaves none)"},
}

// adminCommand runs one rcon command. Callers hold s.mu.
func (s *Server) adminCommand(p *Player, cmd string, args []string) {
	cmd = strings.ToLower(cmd)
	if !s.isAdmin(p) {
		s.print(p, "not admin: admin_login <password> first")
		return
	}
	slog.Info("admin command", "by", p.Name, "id", p.ID, "ip", p.ip, "cmd", cmd, "args", args)
	who := p.Name
	switch cmd {
	case "help", "":
		sections := append([]helpSection{}, adminHelp...)
		cv := helpSection{Title: "cvars (name to show, name value to set)"}
		names := make([]string, 0, len(s.cvars))
		for k := range s.cvars {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			h := cvarHelp[k]
			cv.Rows = append(cv.Rows, helpRow{Name: k, Args: h[0], Value: s.cvars[k], Desc: h[1]})
		}
		b, _ := json.Marshal(map[string]any{"t": "help", "sections": append(sections, cv)})
		trySend(p, b)
	case "status":
		ids := make([]int, 0, len(s.players))
		for id := range s.players {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		mode := "password"
		if s.openAdmin() {
			mode = "OPEN (no password set: everyone is admin)"
		}
		s.print(p, fmt.Sprintf("map %s, %d players, admin: %s", s.Map.Name, len(ids), mode))
		for _, id := range ids {
			o := s.players[id]
			flags := ""
			if o.Bot {
				flags += " bot"
			}
			if s.isAdmin(o) && !o.Bot {
				flags += " admin"
			}
			rate := ""
			if !o.Bot {
				down, up := o.rates()
				rate = fmt.Sprintf(" %5.1f/%4.1f KB/s", down, up)
			}
			s.print(p, fmt.Sprintf("# %-4d %-18s %-15s%s%s", o.ID, o.Name, o.ip, rate, flags))
		}
	case "kick", "ban":
		if len(args) == 0 {
			s.print(p, cmd+" <name|id>")
			return
		}
		t := s.findPlayer(args[0])
		if t == nil {
			s.print(p, "no such player: "+args[0])
			return
		}
		if cmd == "ban" && t.ip != "" {
			mins := 60.0
			if len(args) > 1 {
				if m, err := strconv.ParseFloat(args[1], 64); err == nil && m > 0 {
					mins = m
				}
			}
			s.bans[t.ip] = s.clock().Add(time.Duration(mins * float64(time.Minute)))
			s.announce(fmt.Sprintf("%s banned %s for %g min", who, t.Name, mins))
		} else {
			s.announce(fmt.Sprintf("%s kicked %s", who, t.Name))
		}
		s.disconnect(t)
	case "changelevel":
		if len(args) == 0 || maps.Find(args[0]) == nil {
			s.print(p, "changelevel <map>; maps: "+strings.Join(maps.Order, " ")+" (labs: "+strings.Join(maps.LabNames(), " ")+")")
			return
		}
		s.announce(fmt.Sprintf("%s changed the map to %s", who, args[0]))
		s.changeLevel(args[0])
	case "mapvote":
		if len(args) > 0 && args[0] == "end" {
			if s.vote != nil {
				s.vote.endsAt = s.clock() // the next tick counts it
			}
			return
		}
		secs := float64(defaultVoteSeconds)
		if len(args) > 0 {
			if v, err := strconv.ParseFloat(args[0], 64); err == nil && v > 0 && v <= 600 {
				secs = v
			}
		}
		s.announce(fmt.Sprintf("%s started a map vote", who))
		s.StartMapVote(time.Duration(secs * float64(time.Second)))
	case "restart":
		s.announce(who + " restarted the match")
		s.restart()
	case "mp_warmup_end":
		if s.round.Phase != PhaseWarmup {
			s.print(p, "not in warmup")
			return
		}
		s.warmupEnd = true
		s.announce(who + " ended the warmup")
	case "give":
		if len(args) == 0 || s.Weapons == nil || s.Weapons.Get(args[0]) == nil || s.Weapons.Get(args[0]).Action == "plant" || p.inv == nil {
			ids := []string{}
			if s.Weapons != nil {
				for _, w := range s.Weapons.Weapons {
					if w.Action != "plant" { // The Big Diff comes with the round, not from give
						ids = append(ids, w.ID)
					}
				}
			}
			s.print(p, "give <weapon>; weapons: "+strings.Join(ids, " "))
			return
		}
		w := s.Weapons.Get(args[0])
		s.Weapons.settle(p.inv, p.cmdTicks)
		p.inv.Slots[w.Slot] = s.Weapons.newHeld(w.ID)
		p.inv.Current = w.Slot
		p.inv.drawn(p.cmdTicks) // as buying: deploy delay, and no reload or burst carries over
		s.sendInv(p)
		s.announce(fmt.Sprintf("%s gave themselves %s", who, w.Name))
	case "say_admin":
		s.announce("[admin] " + who + ": " + strings.Join(args, " "))
	case "setpos": // setpos x y z [yaw]: move yourself (CS: setpos; tests and level checks)
		var v [4]float64
		for i := 0; i < 4 && i < len(args); i++ {
			v[i], _ = strconv.ParseFloat(args[i], 64)
		}
		if len(args) < 3 || !finite(Vec3{v[0], v[1], v[2]}) || p.Dead || p.body == nil {
			s.print(p, "setpos x y z [yaw] (alive, on a team)")
			return
		}
		yaw := p.Yaw
		if len(args) > 3 {
			yaw = v[3]
		}
		s.setPos(p, Vec3{v[0], v[1], v[2]}, yaw)
	case "bot_teleport": // bot_teleport x y z [yaw]: every bot (tests)
		var v [4]float64
		for i := 0; i < 4 && i < len(args); i++ {
			v[i], _ = strconv.ParseFloat(args[i], 64)
		}
		if len(args) < 3 || !finite(Vec3{v[0], v[1], v[2]}) {
			s.print(p, "bot_teleport x y z [yaw]")
			return
		}
		for _, b := range s.players {
			if b.Bot {
				b.Pos, b.Yaw = Vec3{v[0], v[1], v[2]}, v[3]
				b.brain.target = b.Pos
			}
		}
	case "bot_patrol": // bot_patrol x0 z0 x1 z1 | off
		var v [4]float64
		for i := 0; i < 4 && i < len(args); i++ {
			v[i], _ = strconv.ParseFloat(args[i], 64)
		}
		off := len(args) > 0 && args[0] == "off"
		if !off && (len(args) < 4 || !finite(Vec3{v[0], v[1], v[2]}) || !finiteF(v[3])) {
			s.print(p, "bot_patrol x0 z0 x1 z1 | off")
			return
		}
		for _, b := range s.players {
			if b.Bot {
				if off {
					b.brain.patrol = nil
					continue
				}
				b.brain.patrol = &[2]Vec3{{v[0], 0, v[1]}, {v[2], 0, v[3]}}
				b.brain.leg, b.brain.idle, b.brain.vy = 1, 0, 0
				b.Pos, b.Crouch = Vec3{v[0], 0, v[1]}, false
			}
		}
		s.print(p, "bot_patrol "+strings.Join(args, " "))
	case "bot_add_vibe", "bot_add_artisan":
		team := strings.TrimPrefix(cmd, "bot_add_")
		b := s.addAI(team)
		s.announce(fmt.Sprintf("%s added %s to the %s", who, b.Name, team))
	case "bot_kick":
		n := s.kickAI("", -1)
		s.cvars["bot_quota"] = "0" // or the quota would add them right back
		s.botCommand(p, "kick", "")
		if n > 0 {
			s.print(p, fmt.Sprintf("kicked %d playing bot(s)", n))
		}
	case "bot_add", "bot_freeze", "bot_crouch":
		arg := ""
		if len(args) > 0 {
			arg = args[0]
		}
		s.botCommand(p, strings.TrimPrefix(cmd, "bot_"), arg)
	default:
		if _, ok := s.cvars[cmd]; !ok {
			s.print(p, "unknown admin command "+cmd+" (rcon help)")
			return
		}
		if len(args) == 0 {
			s.print(p, fmt.Sprintf("%q = %q", cmd, s.cvars[cmd]))
			return
		}
		v := args[0]
		if cmd == "mp_maprotation" {
			if err := s.setRotation(strings.Join(args, ",")); err != nil {
				s.print(p, err.Error())
				return
			}
			v = s.cvars[cmd]
		} else if _, err := strconv.ParseFloat(v, 64); err != nil {
			s.print(p, "numeric value expected")
			return
		}
		s.cvars[cmd] = v
		s.announce(fmt.Sprintf("%s changed %s to %s", who, cmd, v))
		b := s.cvarsMsg()
		for _, o := range s.players {
			trySend(o, b)
		}
	}
}

func (s *Server) findPlayer(key string) *Player {
	if id, err := strconv.Atoi(strings.TrimPrefix(key, "#")); err == nil {
		return s.players[id]
	}
	for _, o := range s.players {
		if strings.EqualFold(o.Name, key) {
			return o
		}
	}
	for _, o := range s.players {
		if strings.Contains(strings.ToLower(o.Name), strings.ToLower(key)) {
			return o
		}
	}
	return nil
}

// disconnect removes a player; their socket closes when the read loop notices.
func (s *Server) disconnect(p *Player) {
	b, _ := json.Marshal(map[string]string{"t": "kicked"})
	trySend(p, b) // the client stops auto-reconnecting
	s.dropBomb(p)
	delete(s.players, p.ID)
	if p.kick != nil {
		close(p.kick)
		p.kick = nil
	}
}

// restart puts everyone back at a spawn with a fresh loadout.
func (s *Server) restart() {
	if s.roundsOn() {
		// A round-based match starts over at round 1, as from warmup: start
		// money, no loss streaks, pistols, no armor, 0-0, everyone respawned.
		// startMatch also puts the sides back and clears the round history.
		s.round = newRound()
		s.startMatch(s.clock())
		return
	}
	i := 0
	for _, p := range s.players {
		if p.Bot {
			continue
		}
		if p.Team != content.Spectator && len(s.Map.TeamSpawns[p.Team]) > 0 {
			p.Pos = s.spawnFor(p.Team)
		} else {
			p.Pos = Vec3(s.Map.Spawns[i%len(s.Map.Spawns)])
			i++
		}
		p.HP, p.Dead, p.Kills, p.Deaths = maxHP, false, 0, 0
		if p.body != nil {
			// Queued commands stay: the client replays its unacked ones from the
			// corrected spawn, so the server runs them from there too.
			p.body = sim.NewBody(sim.Vec3(p.Pos))
		}
		if s.Weapons != nil {
			p.inv = s.Weapons.Loadout("")
			s.sendInv(p)
		}
		b, _ := json.Marshal(correctMsg{T: "correct", Pos: p.Pos})
		trySend(p, b)
	}
}

func (s *Server) banned(ip string) bool {
	until, ok := s.bans[ip]
	if ok && s.clock().After(until) {
		delete(s.bans, ip)
		return false
	}
	return ok
}
