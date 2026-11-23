package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// The module zip that `go install ...@latest` downloads leaves out every
// directory named vendor (and every subdirectory with its own go.mod), so an
// embedded path in one breaks the install with "pattern ...: no matching files".
func TestEmbeddedPathsSurviveTheModuleZip(t *testing.T) {
	err := filepath.WalkDir("..", func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		if n := d.Name(); n == ".git" || n == "node_modules" || n == "out" {
			return filepath.SkipDir
		}
		if d.Name() == "vendor" {
			t.Errorf("%s: go's module zip drops directories named vendor; rename it (web/third_party)", p)
		}
		if p != ".." {
			if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
				t.Errorf("%s has its own go.mod: the module zip leaves it out", p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(FS, "third_party/preact/preact.module.js"); err != nil {
		t.Fatal(err)
	}
}
