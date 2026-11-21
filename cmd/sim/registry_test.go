package main

import (
	"encoding/json"
	"slices"
	"testing"
)

// Client cvars are registry entries; a case covers one by setting it through
// the console ("exec": "<cvar> <value>"), not by only showing it (tk_af359b19).
func TestRegistryCvars(t *testing.T) {
	reg := registry{Console: []string{"buy"}, Cvars: []string{"net_fakeloss", "cl_music"}}
	if e := reg.entries(); !slices.Contains(e, "cvar:net_fakeloss") || !slices.Contains(e, "cvar:cl_music") {
		t.Fatalf("entries %v: want the cvars in", e)
	}
	c := ecase{Script: []map[string]json.RawMessage{
		{"alice": json.RawMessage(`{"exec": "net_fakeloss 0.3; cl_music; buy prompt"}`)},
	}}
	got := exercised(c, nil, reg.Cvars)
	if !slices.Contains(got, "cvar:net_fakeloss") {
		t.Errorf("%v: setting net_fakeloss covers it", got)
	}
	if slices.Contains(got, "cvar:cl_music") {
		t.Errorf("%v: only showing cl_music doesn't cover it", got)
	}
	if !slices.Contains(got, "console:buy") || slices.Contains(got, "console:net_fakeloss") {
		t.Errorf("%v: commands stay console entries, cvars are not console commands", got)
	}
}
