package main

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// goldenCheck compares (-diff) or refreshes (-update-golden) a scenario's PNGs
// against sim/golden/<scenario>. A pixel differs when any channel is off by
// more than 24/255; a snapshot fails when more than -tolerance of its pixels do.
// Advisory only: rendering is software (SwiftShader), so expect small noise.
func goldenCheck(name, out string, log *log.Logger) error {
	gdir := filepath.Join(*golden, name)
	pngs, _ := filepath.Glob(filepath.Join(out, "*.png"))
	if *update {
		if err := os.MkdirAll(gdir, 0o755); err != nil {
			return err
		}
		for _, p := range pngs {
			if err := copyFile(p, filepath.Join(gdir, filepath.Base(p))); err != nil {
				return err
			}
		}
		log.Printf("   golden updated: %s", gdir)
		return nil
	}
	bad := 0
	for _, p := range pngs {
		g := filepath.Join(gdir, filepath.Base(p))
		if _, err := os.Stat(g); err != nil {
			log.Printf("   %s: no golden image", filepath.Base(p))
			continue
		}
		frac, err := pixelDiff(p, g)
		if err != nil {
			return err
		}
		status := "ok"
		if frac > *tolerant {
			status = "DIFFERS"
			bad++
		}
		log.Printf("   %s: %.2f%% pixels differ (%s)", filepath.Base(p), frac*100, status)
	}
	if bad > 0 && *advisory {
		goldenMu.Lock()
		goldenDiffs = append(goldenDiffs, fmt.Sprintf("%s: %d snapshot(s)", name, bad))
		goldenMu.Unlock()
		return nil
	}
	if bad > 0 {
		return fmt.Errorf("%d snapshot(s) differ from golden", bad)
	}
	return nil
}

// goldenDiffs: scenarios whose snapshots differ, with -diff-advisory (reported, not failed).
var (
	goldenDiffs []string
	goldenMu    sync.Mutex
)

func pixelDiff(a, b string) (float64, error) {
	ia, err := readPNG(a)
	if err != nil {
		return 0, err
	}
	ib, err := readPNG(b)
	if err != nil {
		return 0, err
	}
	if ia.Bounds() != ib.Bounds() {
		return 1, nil
	}
	r := ia.Bounds()
	diff := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			r1, g1, b1, _ := ia.At(x, y).RGBA()
			r2, g2, b2, _ := ib.At(x, y).RGBA()
			if absDiff(r1, r2) > 24<<8 || absDiff(g1, g2) > 24<<8 || absDiff(b1, b2) > 24<<8 {
				diff++
			}
		}
	}
	return float64(diff) / float64(r.Dx()*r.Dy()), nil
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
