package game

import (
	"math"
	"testing"
)

// TestAIServerlessBalance pins de_serverless's bot balance: 2v2 to 5v5 bot
// matches (four seeds each, six minutes) go roughly even. They went ~90%
// to the attackers (4v4): the defenders watched the attackers' spawn pad (the
// last waypoint before the throw, across the map) instead of where it lands,
// walked their spots by A's pad down (thrown back to the hub), and the nav
// graph had no walking way from the hub down to B, nor out of B (a pad's
// piece counted as joined), so rotations and retakes went the long way round
// by their spawn's pad, into the planters' sight lines.
func TestAIServerlessBalance(t *testing.T) {
	t.Parallel() // long: overlap it with the rest of the package
	type res struct {
		perTeam int
		st      sweepStats
	}
	sizes := []int{2, 3, 4, 5}
	out := make([]res, len(sizes))
	for i, n := range sizes {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			out[i] = res{n, sweepMap(t, "de_serverless", 0, 4, n, 6)}
		})
	}
	t.Cleanup(func() {
		rounds, wins := 0, 0
		for _, r := range out {
			share := float64(r.st.wins) / float64(max(r.st.rounds, 1))
			t.Logf("%dv%d: %d rounds, attackers won %d (%.0f%%) %v, attacked (won) %v", r.perTeam, r.perTeam, r.st.rounds, r.st.wins, 100*share, r.st.reasons, r.st.attacked)
			if r.st.rounds < 40 {
				t.Errorf("%dv%d: only %d rounds in 24 minutes", r.perTeam, r.perTeam, r.st.rounds)
			}
			if share < 0.3 || share > 0.7 {
				t.Errorf("%dv%d: attackers won %.0f%% of the rounds, want 30-70%%", r.perTeam, r.perTeam, 100*share)
			}
			rounds += r.st.rounds
			wins += r.st.wins
		}
		if share := float64(wins) / float64(max(rounds, 1)); share < 0.4 || share > 0.62 {
			t.Errorf("attackers won %.0f%% of all %d rounds, want 40-62%%", 100*share, rounds)
		}
	})
}

// Off a jump pad, a site's entrance is where the throw lands, not the pad
// (de_serverless: the attackers' spawn pads, across the map; the defenders
// watched those).
func TestSiteEntranceThroughAPad(t *testing.T) {
	s := aiMatch(t, "de_serverless", 1, 0)
	for _, site := range []string{"A", "B"} {
		e, ok := s.siteEntrance(site)
		if !ok {
			t.Fatalf("%s: no entrance", site)
		}
		if s.onLauncher(e) || s.nearSite(e) != site {
			t.Errorf("%s: entrance %v: on a pad, or not at the site (%q)", site, e, s.nearSite(e))
		}
	}
}

// A route walks over no jump pad on its way (stepping on one throws you),
// and the hub and site B are a walk apart both ways (the bridge between
// them), and B reaches A.
func TestServerlessRoutes(t *testing.T) {
	s := aiMatch(t, "de_serverless", 1, 0)
	hub, b, a := Vec3{7, 4, -1}, Vec3{-5, 0, 51}, Vec3{-8, 8, -57}
	byPad := Vec3{4.5, 8, -47} // a defender's spot on A, beside its pad down
	for _, c := range []struct {
		name     string
		from, to Vec3
		pads     int // throws allowed on the way
	}{
		{"hub to B", hub, b, 0},
		{"B to hub", b, hub, 0},
		{"B to A", b, a, 1},
		{"on A to the spot by its pad down", Vec3{-10, 8, -55}, byPad, 0},
	} {
		path := s.navPath(c.from, c.to)
		pads := 0
		for _, w := range path {
			if s.onLauncher(w) {
				pads++
			}
		}
		if pads > c.pads || pathLen(c.from, path) > 3*math.Hypot(c.to[0]-c.from[0], c.to[2]-c.from[2])+20 {
			t.Errorf("%s: %d pads (want at most %d), %.0f m: %v", c.name, pads, c.pads, pathLen(c.from, path), path)
		}
		if end := path[len(path)-1]; end != c.to {
			t.Errorf("%s: ends at %v", c.name, end)
		}
		for i := 1; i < len(path); i++ { // a long leg is a throw, else a straight line no one walks
			if a, b := path[i-1], path[i]; math.Hypot(b[0]-a[0], b[2]-a[2]) > 40 && !s.onLauncher(a) {
				t.Errorf("%s: a %.0f m leg %v -> %v", c.name, math.Hypot(b[0]-a[0], b[2]-a[2]), a, b)
			}
		}
	}
}
