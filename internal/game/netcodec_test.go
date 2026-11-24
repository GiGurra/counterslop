package game

import (
	"strings"
	"testing"
)

func TestSetNetCodec(t *testing.T) {
	s := NewServer()
	if got := s.netCodec().name(); got != CodecJSON {
		t.Fatalf("default codec %q, want json", got)
	}
	if err := s.SetNetCodec(""); err != nil || s.netCodec().name() != CodecJSON {
		t.Fatalf(`"" = json: %v`, err)
	}
	if s.welcomeCodec() != "" {
		t.Errorf("json's welcome names a codec (%q): the json welcome must stay as it was", s.welcomeCodec())
	}
	err := s.SetNetCodec("carrier-pigeon")
	if err == nil || !strings.Contains(err.Error(), "json") {
		t.Errorf("unknown codec: %v, want an error listing the codecs", err)
	}
}
