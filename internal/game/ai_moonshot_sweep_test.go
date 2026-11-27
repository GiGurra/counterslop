package game

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/content/rng"
)

// moonshotMatch plays n v n bots on cq_moonshot from seed for up to secs
// seconds of game time (or the match's end): the tickets left, the winner
// ("" none yet), the share of the time each team held more of the Moon's
// points, and the points Earth ever held.
type moonshotResult struct {
	tickets          map[string]int
	winner           string
	earthAhead, moon float64 // share of samples Earth / the Moon held more points
	earthHeld        map[string]bool
	kills            map[string]int
	front            map[string]float64 // bots alive at the front (x 75..170: the surface), on average
	where            map[string]float64 // Earth's bots by where they are, on average
}

func moonshotMatch(t *testing.T, n int, seed uint64, secs int) moonshotResult {
	s := authServer(t)
	s.Map = maps.CqMoonshot()
	s.autoNav()
	s.Seed(seed)
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["bot_difficulty"] = "1"
	for range n {
		s.addAI(content.Vibe)
		s.addAI(content.Artisan)
	}
	if seed > 1 { // the first seed keeps the bots' own rngs (a real server's)
		for _, p := range s.ordered() {
			p.ai.r = rng.New(uint32(seed)*2654435761 + uint32(p.ID)*40503 + 1)
		}
	}
	res := moonshotResult{earthHeld: map[string]bool{}, kills: map[string]int{}, front: map[string]float64{}, where: map[string]float64{}}
	last := map[*Player]int{}
	samples := 0
	started := false
	for range secs * SimHz / 50 {
		s.Step(50)
		if s.round.Phase == PhaseLive {
			started = true
		}
		if started && s.round.Phase != PhaseLive { // the match is over
			if s.round.Tickets[content.Vibe] > s.round.Tickets[content.Artisan] {
				res.winner = content.Vibe
			} else {
				res.winner = content.Artisan
			}
			break
		}
		own := map[string]int{}
		for i, pt := range s.round.Points {
			if s.Map.Points[i].Locked {
				continue
			}
			own[pt.Own]++
			if pt.Own == content.Vibe {
				res.earthHeld[s.Map.Points[i].ID] = true
			}
		}
		samples++
		for _, q := range s.players {
			if !q.Dead && q.Pos[0] > 75 && q.Pos[0] < 170 && q.Pos[1] > 25 {
				res.front[q.Team]++
			}
			if q.Team == content.Vibe {
				switch {
				case q.Dead:
					res.where["dead"]++
				case q.Pos[0] < -200:
					res.where["terminal"]++
				case q.Pos[0] < -150 && q.Pos[1] < 5:
					res.where["gate/dock"]++
				case q.Pos[0] < 75 || q.Pos[1] > 35:
					res.where["flight"]++
				case q.Pos[0] < 170:
					res.where["surface"]++
				default:
					res.where["campus"]++
				}
			}
		}
		if logEvery := os.Getenv("AI_MOONSHOT_LOG"); logEvery != "" && samples%(15*SimHz/50) == 0 {
			var b strings.Builder
			fmt.Fprintf(&b, "%3ds tk e%d m%d |", samples*50/SimHz, s.round.Tickets[content.Vibe], s.round.Tickets[content.Artisan])
			for i, pt := range s.round.Points {
				fmt.Fprintf(&b, " %s:%s", s.Map.Points[i].ID, map[string]string{"": "-", content.Vibe: "E", content.Artisan: "M"}[pt.Own])
			}
			b.WriteString(" |")
			for _, q := range s.ordered() {
				st := fmt.Sprintf("%.0f,%.0f", q.Pos[0], q.Pos[2])
				if q.Dead {
					st = "dead"
				}
				tgt := "-"
				if q.ai != nil && q.ai.tk.pt > 0 {
					tgt = s.Map.Points[q.ai.tk.pt-1].ID
				}
				fmt.Fprintf(&b, " %s%d>%s@%s", q.Team[:1], q.ID, tgt, st)
			}
			t.Log(b.String())
		}
		switch {
		case own[content.Vibe] > own[content.Artisan]:
			res.earthAhead++
		case own[content.Artisan] > own[content.Vibe]:
			res.moon++
		}
		for _, p := range s.players {
			if p.Kills > last[p] {
				res.kills[p.Team] += p.Kills - last[p]
			}
			last[p] = p.Kills
		}
	}
	for k := range res.where {
		res.where[k] /= float64(max(samples, 1))
	}
	for k := range res.front {
		res.front[k] /= float64(max(samples, 1))
	}
	res.earthAhead /= float64(max(samples, 1))
	res.moon /= float64(max(samples, 1))
	res.tickets = map[string]int{content.Vibe: s.round.Tickets[content.Vibe], content.Artisan: s.round.Tickets[content.Artisan]}
	return res
}

// TestAIMoonshotSweep (AI_MOONSHOT=1, minutes): cq_moonshot's bot balance,
// 2v2, 4v4 and 8v8 over seeds, 8 minutes each: who leads on points, the
// tickets left, who won.
func TestAIMoonshotSweep(t *testing.T) {
	if os.Getenv("AI_MOONSHOT") == "" {
		t.Skip("AI_MOONSHOT=1 to run")
	}
	sizes := []int{2, 4, 8}
	if l := os.Getenv("AI_MOONSHOT_LOG"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			n = 4
		}
		sizes = []int{n}
	}
	for _, n := range sizes {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			t.Parallel()
			var lines []string
			seeds := uint64(4)
			if os.Getenv("AI_MOONSHOT_LOG") != "" {
				seeds = 1
			}
			for seed := uint64(1); seed <= seeds; seed++ {
				r := moonshotMatch(t, n, seed, 8*60)
				lines = append(lines, fmt.Sprintf("seed %d: tickets earth %3d moon %3d won %-7q points ahead earth %.2f moon %.2f; earth held %v; kills %v; at the front earth %.1f moon %.1f; earth's bots %.1v",
					seed, r.tickets[content.Vibe], r.tickets[content.Artisan], r.winner, r.earthAhead, r.moon, r.earthHeld, r.kills, r.front[content.Vibe], r.front[content.Artisan], r.where))
			}
			t.Logf("%dv%d\n%s", n, n, strings.Join(lines, "\n"))
		})
	}
}
