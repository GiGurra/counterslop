package main

import (
	"slices"
	"testing"
)

// Shards by duration: every scenario runs in exactly one shard, and the
// slowest shard is no longer than the longest scenario or the even split
// allows (round-robin put two ~2 min scenarios in one shard).
func TestShardOfBalancesByDuration(t *testing.T) {
	names := []string{"a", "big1", "b", "big2", "c", "d", "big3", "e", "f", "unknown"}
	secs := map[string]float64{"a": 10, "b": 10, "c": 20, "d": 20, "e": 5, "f": 5, "big1": 150, "big2": 130, "big3": 120}
	const n = 4
	seen := map[string]int{}
	worst := 0.0
	for i := 1; i <= n; i++ {
		got := shardOf(names, i, n, secs)
		total := 0.0
		for _, s := range got {
			seen[s]++
			if d, ok := secs[s]; ok {
				total += d
			} else {
				total += defaultScenarioSecs
			}
		}
		worst = max(worst, total)
		if !slices.IsSortedFunc(got, func(x, y string) int { return slices.Index(names, x) - slices.Index(names, y) }) {
			t.Errorf("shard %d not in input order: %v", i, got)
		}
	}
	for _, s := range names {
		if seen[s] != 1 {
			t.Errorf("%s ran in %d shards", s, seen[s])
		}
	}
	if worst > 150 {
		t.Errorf("slowest shard %.0f s, want <= 150 (the longest scenario)", worst)
	}
	// Round-robin (the old split) put big1 and big3 together: 270 s.
}
