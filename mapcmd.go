package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GiGurra/boa/pkg/boa"
	"github.com/spf13/cobra"

	"github.com/gigurra/counterslop/internal/content/mapcheck"
	"github.com/gigurra/counterslop/internal/content/maps"
	"github.com/gigurra/counterslop/internal/game"
	"github.com/gigurra/counterslop/web"
)

// The map subcommands (docs/custom-maps.md): export a built-in map to the
// custom map format, check a map file the way --maps-dir loads it.

// ExportParams: counterslop map export <map|all>... [--out dir].
type ExportParams struct {
	Out string `descr:"write <map>.json files into this directory (\"-\": one map to stdout)" default:"."`
	As  string `descr:"export one map under this name (a copy to edit: a custom map can't take a built-in map's name)" optional:"true"`
}

// CheckParams: counterslop map check <file>...
type CheckParams struct{}

func mapCmd(out io.Writer) *cobra.Command {
	m := &cobra.Command{Use: "map", Short: "custom maps: export a built-in map to the JSON format, check a map file"}
	m.AddCommand(boa.CmdT[ExportParams]{
		Use:         "export <map|all>...",
		Short:       "write maps (built-in, labs; all = every playable map) in the custom map format",
		Args:        cobra.MinimumNArgs(1),
		ParamEnrich: boa.ParamEnricherCombine(boa.ParamEnricherName, boa.ParamEnricherBool),
		RunFuncE: func(p *ExportParams, _ *cobra.Command, args []string) error {
			return exportMaps(out, p.Out, p.As, args)
		},
	}.ToCobra())
	m.AddCommand(boa.CmdT[CheckParams]{
		Use:         "check <file.json>...",
		Short:       "check map files the way --maps-dir loads them (exits non-zero if one is refused)",
		Args:        cobra.MinimumNArgs(1),
		ParamEnrich: boa.ParamEnricherCombine(boa.ParamEnricherName, boa.ParamEnricherBool),
		RunFuncE:    func(_ *CheckParams, _ *cobra.Command, args []string) error { return checkMaps(out, args) },
	}.ToCobra())
	return m
}

// exportMaps writes each named map (or every playable one: "all") as
// <dir>/<name>.json, or one map to out with dir "-".
func exportMaps(out io.Writer, dir, as string, names []string) error {
	if len(names) == 1 && names[0] == "all" {
		names = maps.Names()
	}
	if as != "" && len(names) != 1 {
		return fmt.Errorf("--as takes one map")
	}
	if dir == "-" && len(names) != 1 {
		return fmt.Errorf("--out - takes one map")
	}
	for _, n := range names {
		build := maps.Find(n)
		if build == nil {
			return fmt.Errorf("no map %q (maps: %s; labs: %s)", n, strings.Join(maps.Names(), " "), strings.Join(maps.LabNames(), " "))
		}
		m := build()
		if as != "" {
			m.Name, n = as, as
		}
		b, err := maps.Export(m)
		if err != nil {
			return err
		}
		if dir == "-" {
			_, err = out.Write(b)
			return err
		}
		f := filepath.Join(dir, n+".json")
		if err := os.WriteFile(f, b, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote %s\n", f)
	}
	return nil
}

// checkMaps runs each file through what --maps-dir does to it.
func checkMaps(out io.Writer, files []string) error {
	wj, err := web.FS.ReadFile("assets/weapons/weapons.json")
	if err != nil {
		return err
	}
	reg, err := game.LoadWeapons(wj)
	if err != nil {
		return err
	}
	weapon := func(id string) bool { return reg.Get(id) != nil }
	bad := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err == nil && len(b) > mapcheck.MaxFileBytes {
			err = fmt.Errorf("%d bytes (at most %d)", len(b), mapcheck.MaxFileBytes)
		}
		if err != nil {
			bad++
			fmt.Fprintf(out, "%s: refused: %v\n", f, err)
			continue
		}
		m, err := mapcheck.Parse(b)
		if err != nil {
			bad++
			fmt.Fprintf(out, "%s: refused: %v\n", f, err)
			continue
		}
		if p := mapcheck.Check(m, f, weapon, maps.Builtin); len(p) > 0 {
			bad++
			fmt.Fprintf(out, "%s: refused:\n  %s\n", f, strings.Join(p, "\n  "))
			continue
		}
		fmt.Fprintf(out, "%s: ok (%s, %d boxes)\n", f, m.Name, len(m.Boxes))
	}
	if bad > 0 {
		return fmt.Errorf("%d of %d refused", bad, len(files))
	}
	return nil
}
