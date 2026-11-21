package main

import (
	"encoding/json"
	"os"
	"sort"
)

// Sharding the scenarios for parallel CI jobs by how long each takes, so the
// shards finish together (round-robin put name-tags, rounds and teams, ~2 min
// each, wherever they fell). sim/timings.json holds seconds per scenario as
// measured in CI (refresh it from a run's out/snapshots/timings.json); a
// scenario without a timing counts as defaultScenarioSecs.

const defaultScenarioSecs = 30.0

// loadTimings reads a {scenario: seconds} file; missing or bad = no timings.
func loadTimings(path string) map[string]float64 {
	t := map[string]float64{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &t)
	}
	return t
}

// shardOf returns shard i of n (1-based): longest scenario first, each onto
// the shard with the least total so far (ties: the lower shard). Every name
// lands in exactly one shard, and every shard computes the same split.
// The result keeps the input order (so scenario order within a shard is stable).
func shardOf(names []string, i, n int, secs map[string]float64) []string {
	dur := func(s string) float64 {
		if d, ok := secs[s]; ok && d > 0 {
			return d
		}
		return defaultScenarioSecs
	}
	order := append([]string(nil), names...)
	sort.SliceStable(order, func(a, b int) bool {
		if da, db := dur(order[a]), dur(order[b]); da != db {
			return da > db
		}
		return order[a] < order[b]
	})
	load := make([]float64, n)
	owner := map[string]int{}
	for _, s := range order {
		best := 0
		for k := 1; k < n; k++ {
			if load[k] < load[best] {
				best = k
			}
		}
		load[best] += dur(s)
		owner[s] = best
	}
	var mine []string
	for _, s := range names {
		if owner[s] == i-1 {
			mine = append(mine, s)
		}
	}
	return mine
}

// writeTimings saves this run's seconds per scenario (merged into what the
// file already holds) for refreshing sim/timings.json.
func writeTimings(path string, secs map[string]float64) {
	all := loadTimings(path)
	for k, v := range secs {
		all[k] = float64(int(v*10+0.5)) / 10
	}
	if b, err := json.MarshalIndent(all, "", "  "); err == nil {
		_ = os.WriteFile(path, append(b, '\n'), 0o644)
	}
}
