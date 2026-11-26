package maps

import (
	c "github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/mapgen"
)

// HeMergeHell is a merge conflict you fight in, with grenades and nothing
// else (its Rules: the keyboard and one of every grenade, each back three
// seconds after you throw it, no guns for sale, team deathmatch). Two bases
// (HEAD and the feature branch) face each other over "main": a pit 40 m
// deep that kills whoever falls in ("merged into main"). The branches are
// bridges of diff lines (red: removed, green: added) with commits for cover;
// the middle line meets at the merge commit, an island under a padded LGTM
// canopy. The long walls are LGTM padding too: grenades bounce straight back
// off them (approved without reading), so bank shots are the game.
func HeMergeHell() c.Map {
	const (
		x0, x1 = -34.0, 34.0
		z0, z1 = -18.0, 18.0
		pit    = 20.0  // the pit runs x -pit..pit
		deep   = -40.0 // main is down there
	)
	b := mapgen.New("he_merge_hell")

	// The bases: HEAD (the Vibe Coders, west) and the feature branch (the
	// Artisans, east), and the pit's walls under their edges.
	b.Add(
		c.B(x0, -1, z0, -pit, 0, z1, c.Concrete),
		c.B(pit, -1, z0, x1, 0, z1, c.Concrete),
		c.B(-pit-1, deep, z0, -pit, -1, z1, c.Deprecated),
		c.B(pit, deep, z0, pit+1, -1, z1, c.Deprecated),
		c.B(-pit, deep-1, z0, pit, deep, z1, c.Deprecated), // main: deprecated code all the way down
	)
	// The long walls: LGTM padding from the bottom of the pit up (bank your
	// grenades off them); plain walls behind the bases.
	b.Add(
		c.B(x0-1, deep, z1, x1+1, 10, z1+1, c.LGTM),
		c.B(x0-1, deep, z0-1, x1+1, 10, z0, c.LGTM),
		c.B(x0-1, 0, z0, x0, 10, z1, c.Concrete),
		c.B(x1, 0, z0, x1+1, 10, z1, c.Concrete),
	)

	// The branches: diff-line bridges 3 m wide (the middle line and the links
	// up to the branches 4 m), 1 m thick, no railings.
	diff := func(xa, za, xb, zb float64) { b.Add(c.B(xa, -1, za, xb, 0, zb, c.Diff)) }
	diff(-pit, 9, pit, 12)   // north branch
	diff(-pit, -12, pit, -9) // south branch
	diff(-pit, -2, -5, 2)    // HEAD's line to the merge commit
	diff(5, -2, pit, 2)      // the feature branch's
	diff(-5, -5, 5, 5)       // the merge commit
	diff(-2, 5, 2, 9)        // ... and up to each branch
	diff(-2, -9, 2, -5)
	// Commits: half-width cubes along the branches, alternating sides (cover
	// you can't hide behind both ways at once).
	commit := func(xa, za, zb float64) { b.Add(c.B(xa-0.6, 0, za, xa+0.6, 1.4, zb, c.Screen)) }
	commit(-12, 9, 10.2)
	commit(0, 10.8, 12)
	commit(12, 9, 10.2)
	commit(-12, -10.2, -9)
	commit(0, -12, -10.8)
	commit(12, -10.2, -9)
	commit(-12, -2, -0.8)
	commit(12, 0.8, 2)
	// The merge commit: "=======" across its middle, LGTM posts at its
	// corners and a padded canopy over it (lob a grenade up: it comes down).
	b.Add(c.B(-3, 0, -0.3, 3, 2.2, 0.3, c.Screen))
	for _, p := range [][2]float64{{-5, -5}, {4, -5}, {-5, 4}, {4, 4}} {
		b.Add(c.B(p[0], 0, p[1], p[0]+1, 4, p[1]+1, c.LGTM))
	}
	b.Add(c.B(-6, 7, -6, 6, 7.5, 6, c.LGTM))
	// Base cover: crates of unreviewed pull requests.
	for _, cr := range [][2]float64{{-26, 7}, {-26, -9}, {-23, 2}, {24.5, 7}, {24.5, -9}, {21.5, -3.5}} {
		b.Add(c.B(cr[0], 0, cr[1], cr[0]+1.5, 1.2, cr[1]+1.5, c.Crate))
	}

	// main glows: hellfire from the pit, and a red haze over everything.
	glow := [3]float64{1, 0.35, 0.1}
	for _, p := range [][2]float64{{-12, 0}, {12, 0}, {0, 13}, {0, -13}} {
		b.Light(c.Point([3]float64{p[0], -9, p[1]}, glow, 110, 26))
	}
	b.Fog(0.009, [3]float64{0.42, 0.12, 0.07})
	b.Pit("main", -pit, deep-1, z0, pit, -2.5, z1)

	b.TeamSpawns(c.Vibe, [3]float64{-30, 0, -4}, [3]float64{-30, 0, -1.5}, [3]float64{-30, 0, 1.5},
		[3]float64{-30, 0, 4}, [3]float64{-28, 0, 0})
	b.TeamSpawns(c.Artisan, [3]float64{30, 0, -4}, [3]float64{30, 0, -1.5}, [3]float64{30, 0, 1.5},
		[3]float64{30, 0, 4}, [3]float64{28, 0, 0})
	b.BuyAroundSpawns(3)
	// No bomb here (team deathmatch): the "sites" are where the bots go to
	// fight, mid-branch.
	b.Site("A", -3, -0.5, 9, 3, 3, 12)
	b.Site("B", -3, -0.5, -12, 3, 3, -9)

	b.Nav("vibe", -29, 0, 0, "vN", "vS", "vM").
		Nav("vN", -23, 0, 10.5, "n1").
		Nav("n1", -8, 0, 11.2, "A").
		Nav("A", 0, 0, 9.6, "n3", "nC").
		Nav("n3", 8, 0, 11.2, "aN").
		Nav("vS", -23, 0, -10.5, "s1").
		Nav("s1", -8, 0, -11.2, "B").
		Nav("B", 0, 0, -9.6, "s3", "sC").
		Nav("s3", 8, 0, -11.2, "aS").
		Nav("vM", -23, 0, 0.4, "m1").
		Nav("m1", -6, 0, 0.6, "iW", "iSW").
		Nav("iW", -4, 0, 1.8, "iE", "iN", "iSW").
		Nav("iSW", -4, 0, -1.8, "iSE", "iS").
		Nav("iE", 4, 0, 1.8, "iSE", "iN", "m2").
		Nav("iSE", 4, 0, -1.8, "iS", "m2").
		Nav("iN", 0, 0, 3.5, "nC"). // the links' mouths: walk in straight
		Nav("iS", 0, 0, -3.5, "sC").
		Nav("nC", 0, 0, 7).
		Nav("sC", 0, 0, -7).
		Nav("m2", 6, 0, -0.6, "aM").
		Nav("aM", 23, 0, -0.4, "artisan").
		Nav("artisan", 29, 0, 0, "aN", "aS").
		Nav("aN", 23, 0, 10.5).
		Nav("aS", 23, 0, -10.5)

	b.Rule("mp_loadout", "nades"). // the keyboard and one of every grenade
					Rule("mp_nade_refill", "3"). // each back 3 s after you throw it
					Rule("mp_buy_guns", "0").    // guns don't merge cleanly
					Rule("mp_respawn", "1").     // team deathmatch
					Rule("mp_respawntime", "2")
	b.Meta("A merge conflict, resolved with grenades: no guns, every grenade back 3 s after you throw it. The branches are bridges over main; fall in and you're merged. LGTM walls bounce everything back unread.", 10)
	b.FillSpawns(true)
	b.Category(c.Fun)
	return b.Map()
}
