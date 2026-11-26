package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/gigurra/counterslop/internal/game"
)

// The action registry: everything a player can do (bindable actions, command
// button bits, console commands, client cvars, admin/rcon commands), read live from the
// client and the server. Every entry must be exercised by a passing lockstep
// e2e case, or be exempted in sim/registry-exempt.json with a reason; the
// matrix goes to the CI job summary. Coverage is derived from what each case
// actually does (its inputs), not declared.

type registry struct {
	Actions   []string `json:"actions"`
	Buttons   []string `json:"buttons"`
	Console   []string `json:"console"`
	Forwarded []string `json:"forwarded"`
	Cvars     []string `json:"cvars"` // web/ui/settings.js CVARS: covered by a case that sets one ("exec": "<cvar> <value>")
	Admin     []string
}

func (r registry) entries() []string {
	var out []string
	for _, a := range r.Actions {
		out = append(out, "action:"+a)
	}
	for _, b := range r.Buttons {
		out = append(out, "btn:"+b)
	}
	for _, c := range r.Console {
		out = append(out, "console:"+c)
	}
	for _, c := range r.Admin {
		out = append(out, "rcon:"+c)
	}
	for _, c := range r.Cvars {
		out = append(out, "cvar:"+c)
	}
	sort.Strings(out)
	return slices.Compact(out)
}

func loadRegistry(p *page, srv *game.Server) (registry, error) {
	var r registry
	if err := p.eval("__cs.registry()", &r); err != nil {
		return r, err
	}
	r.Admin = srv.AdminCommands()
	return r, nil
}

var buttonAction = map[string]string{"jump": "+jump", "crouch": "+duck", "walk": "+speed", "fire": "+attack", "scope": "+zoom", "alt": "+zoom", "reload": "reload", "use": "+use", "hook": "+hook", "seat": "+seat"}
var slotAction = map[string]string{"primary": "slot1", "sidearm": "slot2", "melee": "slot3", "bomb": "slot5"}

// exercised lists the registry entries a case's inputs go through.
func exercised(c ecase, forwarded, cvars []string) []string {
	set := map[string]bool{}
	add := func(e ...string) {
		for _, x := range e {
			set[x] = true
		}
	}
	for _, step := range c.Script {
		for k, raw := range step {
			if k == "n" || k == "expect" {
				continue
			}
			var in einput
			if json.Unmarshal(raw, &in) != nil {
				continue
			}
			if in.F > 0 {
				add("action:+forward")
			} else if in.F < 0 {
				add("action:+back")
			}
			if in.R > 0 {
				add("action:+moveright")
			} else if in.R < 0 {
				add("action:+moveleft")
			}
			for _, b := range append(slices.Clone(in.B), in.Tap...) { // a tap is a press of the same bound key
				add("btn:"+b, "action:"+buttonAction[b])
				if b == "reload" {
					add("console:reload") // a bound plain action runs through the console
				}
			}
			if in.W != "" {
				add("action:"+slotAction[in.W], "console:"+slotAction[in.W])
			}
			if in.Drop {
				add("action:drop", "console:drop")
			}
			for _, a := range in.Act {
				add("action:"+a, "console:"+a)
			}
			for _, a := range in.Hold {
				add("action:" + a)
			}
			if in.Exec != "" {
				for _, part := range strings.Split(in.Exec, ";") {
					w := strings.Fields(part)
					if len(w) == 0 {
						continue
					}
					if slices.Contains(cvars, w[0]) {
						if len(w) > 1 { // set, not just shown
							add("cvar:" + w[0])
						}
						continue
					}
					add("console:" + w[0])
					if w[0] == "rcon" {
						add("rcon:help")
						if len(w) > 1 {
							add("rcon:" + w[1])
						}
					} else if slices.Contains(forwarded, w[0]) {
						add("rcon:" + w[0])
					}
					if strings.HasPrefix(w[0], "bot_") {
						add("rcon:" + w[0]) // the console's bot_* commands are admin commands on the server
					}
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// registryMatrix: entry -> the passing cases exercising it; missing entries
// (neither covered nor exempt) make the run fail.
func registryMatrix(reg registry, res []caseResult, cases map[string]ecase) (md string, missing []string) {
	exempt := map[string]string{}
	if b, err := os.ReadFile("sim/registry-exempt.json"); err == nil {
		json.Unmarshal(b, &exempt)
	}
	by := map[string][]string{}
	bad := map[string][]string{}
	for _, r := range res {
		for _, e := range exercised(cases[r.Name], reg.Forwarded, reg.Cvars) {
			if len(r.Fails) == 0 {
				by[e] = append(by[e], r.Name)
			} else {
				bad[e] = append(bad[e], r.Name)
			}
		}
	}
	var b strings.Builder
	b.WriteString("### Action registry coverage\n\n| entry | lockstep cases | status |\n|---|---|---|\n")
	covered := 0
	all := reg.entries()
	for _, e := range all {
		switch {
		case len(by[e]) > 0:
			covered++
			fmt.Fprintf(&b, "| %s | %s | ✅ |\n", e, strings.Join(by[e], ", "))
		case exempt[e] != "":
			fmt.Fprintf(&b, "| %s | | ⚠ exempt: %s |\n", e, exempt[e])
		case len(bad[e]) > 0:
			missing = append(missing, e)
			fmt.Fprintf(&b, "| %s | %s | ❌ failing |\n", e, strings.Join(bad[e], ", "))
		default:
			missing = append(missing, e)
			fmt.Fprintf(&b, "| %s | | ❌ no case |\n", e)
		}
	}
	for e := range exempt {
		if !slices.Contains(all, e) {
			fmt.Fprintf(&b, "| %s | | ❓ exempt but not in the registry (remove it from sim/registry-exempt.json) |\n", e)
		}
	}
	fmt.Fprintf(&b, "\n%d of %d entries covered by passing lockstep cases, %d exempt, %d missing.\n", covered, len(all), len(exempt), len(missing))
	return b.String(), missing
}
