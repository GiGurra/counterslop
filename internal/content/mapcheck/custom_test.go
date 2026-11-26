package mapcheck

import (
	"encoding/json"
	"github.com/gigurra/counterslop/internal/content/maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every built-in map and lab exports to the custom map format and reads back
// the same, every feature with it (wedges, Rules, water, boats, pits, fog,
// cut, movers, lights, nav).
func TestExportRoundTrip(t *testing.T) {
	names := append(maps.Names(), maps.LabNames()...)
	seen := map[string]bool{}
	for _, n := range names {
		m := maps.Find(n)()
		b, err := maps.Export(m)
		if err != nil {
			t.Fatal(err)
		}
		back, err := Parse(b)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		x, _ := json.Marshal(m)
		y, _ := json.Marshal(back)
		if string(x) != string(y) {
			t.Errorf("%s: changed on the way through the format", n)
		}
		for _, f := range []string{`"ramp"`, `"rules"`, `"water"`, `"boats"`, `"pits"`, `"fog"`, `"cut"`, `"movers"`} {
			if strings.Contains(string(b), f) {
				seen[f] = true
			}
		}
	}
	for _, f := range []string{`"ramp"`, `"rules"`, `"water"`, `"boats"`, `"pits"`, `"fog"`, `"cut"`, `"movers"`} {
		if !seen[f] {
			t.Errorf("no built-in map exercises %s in the format", f)
		}
	}
}

// LoadDir loads the good maps and refuses each broken file with its reason.
func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	good := maps.DeSlop()
	good.Name = "de_slop_remix"
	b, _ := maps.Export(good)
	write("de_slop_remix.json", b)
	write("misnamed.json", b)
	builtin, _ := maps.Export(maps.DeSlop())
	write("de_slop.json", builtin)
	write("typo.json", []byte(strings.Replace(string(b), `"boxes"`, `"boxs"`, 1)))
	write("garbage.json", []byte("{not json"))
	broken := maps.DeSlop()
	broken.Name, broken.Sites = "de_broken", nil
	bb, _ := maps.Export(broken)
	write("de_broken.json", bb)
	write("notes.txt", []byte("not a map"))

	ok, refused, err := LoadDir(dir, nil, maps.Builtin)
	if err != nil {
		t.Fatal(err)
	}
	if len(ok) != 1 || ok["de_slop_remix"].Name != "de_slop_remix" {
		t.Fatalf("loaded %v", keys(ok))
	}
	for file, want := range map[string]string{
		"misnamed.json":  `not its file's`,
		"de_slop.json":   "built-in map's name",
		"typo.json":      `unknown field "boxs"`,
		"garbage.json":   "not a map",
		"de_broken.json": "site",
	} {
		if !strings.Contains(refused[file], want) {
			t.Errorf("%s: refused %q, want %q", file, refused[file], want)
		}
	}
	if len(refused) != 5 {
		t.Errorf("refused %v", refused)
	}
	if _, _, err := LoadDir(filepath.Join(dir, "nope"), nil, maps.Builtin); err != nil {
		t.Errorf("a missing dir: %v", err)
	}

	maps.SetCustom(ok)
	defer maps.SetCustom(nil)
	if maps.Find("de_slop_remix") == nil || maps.Playable("de_slop_remix") == nil || !maps.IsCustom("de_slop_remix") || maps.IsCustom("de_slop") {
		t.Fatal("the custom map isn't found")
	}
	if n := maps.Names(); n[len(n)-1] != "de_slop_remix" || len(n) != len(maps.Order)+1 {
		t.Fatalf("names %v", n)
	}
	m := maps.Find("de_slop_remix")()
	m.Boxes[0].Kind = "changed" // each build is its own copy
	if maps.Find("de_slop_remix")().Boxes[0].Kind == "changed" {
		t.Fatal("builds share the map")
	}
	if maps.Fingerprint(dir) == "" {
		t.Fatal("no fingerprint")
	}
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
