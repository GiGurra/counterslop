package web

import (
	"os"
	"os/exec"
	"testing"
)

// TestJS runs the client's node unit tests (web/**/*.test.mjs).
func TestJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	cmd := exec.Command(node, "--test", "**/*.test.mjs")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
