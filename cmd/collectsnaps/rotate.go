package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type archive struct {
	Path string
	Time time.Time
}

type rotateConfig struct {
	keepWithin  time.Duration
	keepLast    int
	keepHourly  int
	keepDaily   int
	keepWeekly  int
	keepMonthly int
	keepYearly  int
}

func proceedRotateArchives(dir, baseTag string, plan rotateConfig) error {
	baseDir := filepath.Join(dir, baseTag)
	archives, err := parseArchives(os.DirFS(baseDir), baseTag)
	if err != nil {
		return fmt.Errorf("parsing archives: %w", err)
	}

	toRemove := rotateArchives(archives, plan)

	if err := removeArchives(baseDir, toRemove); err != nil {
		return fmt.Errorf("removing archives: %w", err)
	}

	return nil
}

func parseArchives(filesystem fs.FS, tag string) ([]archive, error) {
	var archives []archive
	prefix := fmt.Sprintf("%s-", tag)
	ext := ".json"

	fmt.Fprintf(os.Stderr, "prefix: %s\n", prefix)

	err := fs.WalkDir(filesystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && d.Name() != "." && d.Name() != ".." {
			return fs.SkipDir
		}

		if !strings.HasPrefix(d.Name(), prefix) || !strings.HasSuffix(d.Name(), ext) {
			return nil
		}

		timestamp := strings.TrimSuffix(strings.TrimPrefix(d.Name(), prefix), ext)

		t, err := time.Parse("20060102-150405", timestamp)
		if err != nil {
			return nil
		}

		archives = append(archives, archive{Path: path, Time: t})

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(archives, func(i, j int) bool {
		return archives[i].Time.After(archives[j].Time)
	})

	return archives, nil
}

func rotateArchives(archives []archive, plan rotateConfig) []archive {
	keep := map[string]archive{}
	now := time.Now()

	fmt.Fprintf(os.Stderr, "all archives: %d\n", len(archives))

	// Keep within interval
	for _, a := range archives {
		if now.Sub(a.Time) <= plan.keepWithin {
			keep[a.Path] = a
		}
	}

	fmt.Fprintf(os.Stderr, "keep within %s: %d\n", plan.keepWithin, len(keep))

	// Keep last N archives
	for i := 0; i < len(archives) && i < plan.keepLast; i++ {
		keep[archives[i].Path] = archives[i]
	}

	fmt.Fprintf(os.Stderr, "keep last %d: %d\n", plan.keepLast, len(keep))

	// Helper function for intervals
	keepInterval := func(limit int, format string) {
		intervals := map[string]bool{}
		for _, a := range archives {
			key := a.Time.Format(format)
			if !intervals[key] && len(intervals) < limit {
				keep[a.Path] = a
				intervals[key] = true
			}
		}
	}

	keepInterval(plan.keepHourly, "2006010215")

	fmt.Fprintf(os.Stderr, "keep hourly %d: %d\n", plan.keepHourly, len(keep))

	keepInterval(plan.keepDaily, "20060102")

	fmt.Fprintf(os.Stderr, "keep daily %d: %d\n", plan.keepDaily, len(keep))

	keepInterval(plan.keepWeekly, "200601")

	fmt.Fprintf(os.Stderr, "keep weekly %d: %d\n", plan.keepWeekly, len(keep))

	keepInterval(plan.keepMonthly, "200601")

	fmt.Fprintf(os.Stderr, "keep monthly %d: %d\n", plan.keepMonthly, len(keep))

	keepInterval(plan.keepYearly, "2006")

	fmt.Fprintf(os.Stderr, "keep yearly %d: %d\n", plan.keepYearly, len(keep))

	var toRemove []archive
	for _, a := range archives {
		if _, ok := keep[a.Path]; !ok {
			toRemove = append(toRemove, a)
		}
	}

	fmt.Fprintf(os.Stderr, "to remove: %d\n", len(toRemove))

	return toRemove
}

func removeArchives(dir string, toRemove []archive) error {
	for _, a := range toRemove {
		fn := filepath.Join(dir, a.Path)
		if err := os.Remove(fn); err != nil {
			return fmt.Errorf("remove archive %s: %w", fn, err)
		}
	}

	return nil
}
