package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// profileRun is one __cs.profile() result (web/game/testapi.js): a frame-time
// breakdown of n drawn frames on one map (engine/prof.js, docs/profiling.md).
type profileRun struct {
	Map     string   `json:"map"`
	Label   string   `json:"label"`
	Frames  int      `json:"frames"`
	GPU     string   `json:"gpu"`
	Res     string   `json:"res"`
	FrameMs float64  `json:"frameMs"`
	CPUMs   float64  `json:"cpuMs"`
	GPUMs   *float64 `json:"gpuMs"`
	Verdict string   `json:"verdict"`
	Rows    []struct {
		Path  string   `json:"path"`
		Mean  float64  `json:"mean"`
		Self  float64  `json:"self"`
		P95   float64  `json:"p95"`
		Max   float64  `json:"max"`
		Calls float64  `json:"calls"`
		GPU   *float64 `json:"gpu"`
	} `json:"rows"`
}

// writeProfiles: the profiles a scenario's clients took (perf-profile), as
// profile.md next to its snapshots, in the log and, under GitHub Actions, in
// the job summary.
func writeProfiles(name, out string, clients []string, pages map[string]*page, lg *log.Logger) {
	var md strings.Builder
	for _, c := range clients {
		var raw string
		if pages[c] == nil || pages[c].eval("JSON.stringify(__cs.state().prof.log || [])", &raw) != nil {
			continue
		}
		var runs []profileRun
		if json.Unmarshal([]byte(raw), &runs) != nil {
			continue
		}
		for _, r := range runs {
			md.WriteString(profileMarkdown(name, c, r))
		}
	}
	if md.Len() == 0 {
		return
	}
	_ = os.WriteFile(filepath.Join(out, "profile.md"), []byte(md.String()), 0o644)
	lg.Printf("   profile.md:\n%s", md.String())
	if f := os.Getenv("GITHUB_STEP_SUMMARY"); f != "" {
		if fh, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644); err == nil {
			fh.WriteString(md.String())
			fh.Close()
		}
	}
}

// profileMarkdown: one profile as a table, the sections indented by depth.
func profileMarkdown(scenario, client string, r profileRun) string {
	var b strings.Builder
	ms := func(v *float64) string {
		if v == nil {
			return "n/a"
		}
		return fmt.Sprintf("%.2f", *v)
	}
	title := r.Map
	if r.Label != "" {
		title += " (" + r.Label + ")"
	}
	fmt.Fprintf(&b, "### Frame profile: %s (%s, %s)\n\n", title, scenario, client)
	fmt.Fprintf(&b, "%d frames · frame %.2f ms · cpu %.2f ms · gpu %s ms · %s · %s\n\n**%s**\n\n", r.Frames, r.FrameMs, r.CPUMs, ms(r.GPUMs), r.GPU, r.Res, r.Verdict)
	b.WriteString("| section | cpu mean | self | p95 | max | calls/frame | gpu mean |\n|---|---:|---:|---:|---:|---:|---:|\n")
	for _, row := range r.Rows {
		depth := strings.Count(row.Path, "/")
		name := row.Path[strings.LastIndex(row.Path, "/")+1:]
		fmt.Fprintf(&b, "| %s%s | %.2f | %.2f | %.2f | %.2f | %.2f | %s |\n", strings.Repeat("&nbsp;&nbsp;", depth), name, row.Mean, row.Self, row.P95, row.Max, row.Calls, ms(row.GPU))
	}
	b.WriteString("\n")
	return b.String()
}
