package game

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/content/rng"
)

// An even share per site, the odd ones out anywhere, a stack now and then,
// and a site left empty only by a gamble (rarely, never with 2 or more for it).
func TestSplitDefenders(t *testing.T) {
	r := rng.New(7)
	seen := map[[2]int]int{}
	empty, rounds := 0, 0
	for n := 1; n <= 12; n++ {
		for k := 1; k <= 4; k++ {
			for range 200 {
				c := splitDefenders(n, k, r.Float, func(m int) int { return r.Int(0, m-1) })
				sum := 0
				if n >= k {
					rounds++
				}
				for _, v := range c {
					sum += v
					if v == 0 && n >= k {
						empty++
						if n >= 2*k+1 {
							t.Fatalf("%d over %d sites: %v leaves a site empty", n, k, c)
						}
					}
				}
				if sum != n || slices.Max(c) > (n+k-1)/k+1 {
					t.Fatalf("%d over %d sites: %v", n, k, c)
				}
				if n == 5 && k == 2 {
					seen[[2]int{c[0], c[1]}]++
				}
			}
		}
	}
	if float64(empty) > float64(rounds)*aiGambleChance*1.3 {
		t.Errorf("a site empty in %d of %d rounds", empty, rounds)
	}
	for _, want := range [][2]int{{3, 2}, {2, 3}, {4, 1}, {1, 4}} {
		if seen[want] < 5 {
			t.Errorf("5 over 2 sites: %v only %d of 200 rounds (%v)", want, seen[want], seen)
		}
	}
	if st := float64(seen[[2]int{4, 1}] + seen[[2]int{1, 4}]); st < 200*aiStackChance/2 || st > 200*aiStackChance*1.5 {
		t.Errorf("too many stacks: %v", seen)
	}
}

// Every bomb site has several hold spots, spread out, reached well before
// the attackers, in sight of where they come in.
func TestSiteSpots(t *testing.T) {
	for _, name := range maps.Order {
		s := authServer(t)
		s.Map = maps.All[name]()
		att, def := s.Map.TeamSpawns[content.Vibe], s.Map.TeamSpawns[content.Artisan]
		if len(s.Map.Sites) == 0 || len(att) == 0 || len(def) == 0 {
			continue
		}
		for _, z := range s.Map.Sites {
			spots := s.siteSpots(z.Name)
			t.Logf("%s %s: %d spots %v", name, z.Name, len(spots), spots)
			if len(spots) < 3 && name+"/"+z.Name != "de_standup/B" { // next to the attackers' spawn: held short of it, in a corridor
				t.Errorf("%s %s: %d hold spots, want 3 or more", name, z.Name, len(spots))
			}
			if h := s.siteHold(z.Name); (spots[0][0] != h[0] || spots[0][2] != h[2]) && (s.pointFree(Vec3{h[0], s.floorUnder(h), h[2]}, 0.6) || math.Hypot(spots[0][0]-h[0], spots[0][2]-h[2]) > 4) {
				t.Errorf("%s %s: the first spot %v isn't the site's hold %v", name, z.Name, spots[0], h)
			}
			for _, q := range spots {
				if !s.pointFree(q, 0.6) {
					t.Errorf("%s %s: spot %v in a prop", name, z.Name, q)
				}
			}
			e, _ := s.siteEntrance(z.Name)
			for i, q := range spots {
				for _, o := range spots[:i] {
					if math.Hypot(q[0]-o[0], q[2]-o[2]) < aiSpotGap {
						t.Errorf("%s %s: spots %v and %v too close", name, z.Name, q, o)
					}
				}
				if i > 0 && !s.lineClear(Vec3{q[0], q[1] + 1.2, q[2]}, Vec3{e[0], e[1] + 1, e[2]}) {
					t.Errorf("%s %s: spot %v can't see the entrance %v", name, z.Name, q, e)
				}
				if d, d0 := math.Hypot(q[0]-spots[0][0], q[2]-spots[0][2]), math.Hypot(spots[0][0]-e[0], spots[0][2]-e[2]); d > aiSpotNear+0.01 || math.Hypot(q[0]-e[0], q[2]-e[2]) > d0+aiSpotDeep+0.01 {
					t.Errorf("%s %s: spot %v too far from the hold %v or deeper in (entrance %v)", name, z.Name, q, spots[0], e)
				}
			}
		}
	}
}

// Over many rounds of de_hackathon (the operator's report: the Artisans held
// one site, from the same spots, nearly every round) both sites are held
// every round, each by its defenders from several spots, with a stack and a
// forward defender now and then.
func TestDefPlanMixesItUp(t *testing.T) {
	for _, name := range []string{"de_hackathon", "de_legacy", "de_standup"} {
		s := authServer(t)
		s.Map = maps.All[name]()
		s.Map.MaxPlayers = max(s.Map.MaxPlayers, 10)
		var first *Player
		for range 5 {
			if a := s.addAI(content.Artisan); first == nil {
				first = a
			}
		}
		s.Step(1)
		const rounds = 100
		empty, forward, stacks := 0, 0, 0
		spots := map[string]map[Vec3]int{}
		for round := 1; round <= rounds; round++ {
			s.round.Phase, s.round.Number = PhaseLive, round
			plan := s.defPlan(first)
			per := map[string]int{}
			for _, post := range plan.posts {
				per[post.site]++
				if spots[post.site] == nil {
					spots[post.site] = map[Vec3]int{}
				}
				spots[post.site][post.at]++
				if post.forward {
					forward++
				}
			}
			if len(plan.posts) != 5 {
				t.Fatalf("%s round %d: %d posts", name, round, len(plan.posts))
			}
			for _, z := range s.Map.Sites {
				if per[z.Name] == 0 {
					empty++
				}
				if per[z.Name] >= 4 {
					stacks++
				}
			}
			for _, p := range s.ordered() { // pickSite follows the plan
				if got := s.pickSite(p); got != plan.posts[p.ID].site {
					t.Fatalf("%s: pickSite %q, plan %q", name, got, plan.posts[p.ID].site)
				}
			}
		}
		t.Logf("%s: stacks %d forward %d spots %v", name, stacks, forward, spots)
		if empty > 0 { // 5 defenders over 2 sites: a gamble needs a lone defender
			t.Errorf("%s: a site without defenders in %d of %d rounds", name, empty, rounds)
		}
		if stacks < rounds/8 || stacks > rounds/2 {
			t.Errorf("%s: %d stacked rounds of %d", name, stacks, rounds)
		}
		if forward < rounds/10 && name != "de_standup" { // a small map: the attackers are everywhere first
			t.Errorf("%s: a forward defender in only %d of %d rounds", name, forward, rounds)
		}
		for site, used := range spots {
			if len(used) < 3 && name+"/"+site != "de_standup/B" {
				t.Errorf("%s %s: only %d distinct hold spots over %d rounds", name, site, len(used), rounds)
			}
		}
	}
}

// A forward defender falls back to its spot on the site in time; a callout
// at the other site takes a site's defenders there.
func TestDefForwardFallsBackRotates(t *testing.T) {
	s := authServer(t)
	s.Map = maps.All["de_hackathon"]()
	s.Map.MaxPlayers = max(s.Map.MaxPlayers, 10)
	for range 5 {
		s.addAI(content.Artisan)
	}
	s.Step(1)
	s.cvars["mp_roundtime"] = "115"
	s.round.endsAt = s.now.Add(115 * time.Second)
	forward := false
	for round := 1; round <= 30 && !forward; round++ {
		s.round.Phase, s.round.Number = PhaseLive, round
		var first *Player
		for _, p := range s.ordered() {
			if first == nil {
				first = p
			}
			p.ai.round, p.ai.rotated = round, 0
		}
		plan := s.defPlan(first)
		for _, p := range s.ordered() {
			p.ai.site = s.pickSite(p)
		}
		for _, p := range s.ordered() {
			post := plan.posts[p.ID]
			if !post.forward {
				continue
			}
			forward = true
			if got := s.defPost(p); got.at != post.at || slices.Contains(s.siteSpots(post.site), post.at) {
				t.Fatalf("forward post %v is a site spot", got.at)
			}
			s.round.endsAt = s.now.Add(time.Duration((115 - aiForwardSecs) * float64(time.Second)))
			if got := s.defPost(p); got.forward || got.at != post.back || !slices.Contains(s.siteSpots(post.site), got.at) {
				t.Fatalf("after %.0f s: %+v, want back at %v", aiForwardSecs, got, post.back)
			}
			s.round.endsAt = s.now.Add(115 * time.Second)
		}
		// B is called while A is quiet: A's defenders go.
		s.tick += aiQuietTicks
		s.noteIntel(content.Artisan, s.mustSite("B"))
		for _, p := range s.ordered() {
			was := p.ai.site
			s.aiRotate(p)
			if was == "A" && p.ai.site != "B" {
				t.Fatalf("round %d: A's defender stayed", round)
			}
		}
	}
	if !forward {
		t.Fatal("no forward defender in 30 rounds")
	}
}
