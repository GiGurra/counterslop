package maps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	c "github.com/gigurra/counterslop/internal/content"
)

// Custom maps: maps from files (--maps-dir), in the same JSON a built-in map
// exports to (Export; the map goldens in testdata are that format too). They
// are held to the built-in maps' rules (mapcheck.Problems) when loaded; a broken one
// is refused with its reason and the rest load. docs/custom-maps.md.

// custom is the loaded set, swapped whole on every (re)load. A map is kept
// as its JSON so every build hands out its own copy.
var custom struct {
	sync.RWMutex
	maps map[string][]byte
	gen  int
}

// SetCustom replaces the custom maps.
func SetCustom(ms map[string]c.Map) {
	enc := make(map[string][]byte, len(ms))
	for n, m := range ms {
		b, err := json.Marshal(m)
		if err != nil {
			continue // a Map always marshals (checked by Parse)
		}
		enc[n] = b
	}
	custom.Lock()
	defer custom.Unlock()
	custom.maps = enc
	custom.gen++
}

// Gen counts SetCustom calls: a cache of anything built from the map list
// (the catalog) is stale when it moves.
func Gen() int {
	custom.RLock()
	defer custom.RUnlock()
	return custom.gen
}

// customBuild is the custom map's builder, or nil.
func customBuild(name string) func() c.Map {
	custom.RLock()
	b, ok := custom.maps[name]
	custom.RUnlock()
	if !ok {
		return nil
	}
	return func() c.Map {
		var m c.Map
		_ = json.Unmarshal(b, &m) // it came from json.Marshal
		return m
	}
}

// CustomNames lists the custom maps, sorted.
func CustomNames() []string {
	custom.RLock()
	defer custom.RUnlock()
	out := make([]string, 0, len(custom.maps))
	for n := range custom.maps {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// Names is every playable map: the built-ins in Order, then the custom maps.
func Names() []string { return append(slices.Clone(Order), CustomNames()...) }

// Playable is a built-in or custom map by name (not a lab), or nil.
func Playable(name string) func() c.Map {
	if b := All[name]; b != nil {
		return b
	}
	return customBuild(name)
}

// IsCustom: the map came from a file.
func IsCustom(name string) bool {
	return All[name] == nil && Labs[name] == nil && customBuild(name) != nil
}

// Builtin: name is a built-in map's or a lab's (a custom map can't take it).
func Builtin(name string) bool { return All[name] != nil || Labs[name] != nil }

// Export is a map in the custom map format (indented JSON, as LoadDir reads it).
func Export(m c.Map) ([]byte, error) {
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Fingerprint is a cheap summary of dir's map files (names, sizes, modify
// times): a hot reload reads the maps again when it changes.
func Fingerprint(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var sb strings.Builder
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			fmt.Fprintf(&sb, "%s %d %d\n", filepath.Base(f), st.Size(), st.ModTime().UnixNano())
		}
	}
	return sb.String()
}
