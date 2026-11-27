package main

import "testing"

func TestParseCombo(t *testing.T) {
	for _, c := range []struct {
		in        string
		mods      int
		key, code string
		vk        int
	}{
		{"ctrl+c", 2, "c", "KeyC", 'C'},
		{"Shift+Insert", 8, "Insert", "Insert", 45},
		{"meta+v", 4, "v", "KeyV", 'V'},
		{"shift+a", 8, "A", "KeyA", 'A'},
		{"enter", 0, "Enter", "Enter", 13},
		{"space", 0, " ", "Space", 32},
		{"1", 0, "1", "Digit1", '1'},
	} {
		mods, k, err := parseCombo(c.in)
		if err != nil || mods != c.mods || k.key != c.key || k.code != c.code || k.vk != c.vk {
			t.Errorf("parseCombo(%q) = %d %+v %v, want %d %q %q %d", c.in, mods, k, err, c.mods, c.key, c.code, c.vk)
		}
	}
	for _, bad := range []string{"hyper+c", "f13", "ctrl+"} {
		if _, _, err := parseCombo(bad); err == nil {
			t.Errorf("parseCombo(%q): no error", bad)
		}
	}
}
