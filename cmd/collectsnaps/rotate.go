package main

import (
	"fmt"
	"io/fs"
	"os"
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

func proceedRotateArchives(dir, tag string, plan rotateConfig) error {
	archives, err := parseArchives(os.DirFS(dir), tag)
	if err != nil {
		return fmt.Errorf("parsing archives: %w", err)
	}

	toRemove := rotateArchives(archives, plan)

	if err := removeArchives(toRemove); err != nil {
		return fmt.Errorf("removing archives: %w", err)
	}

	return nil
}

func parseArchives(filesystem fs.FS, tag string) ([]archive, error) {
	var archives []archive
	prefix := fmt.Sprintf("%s-", tag)
	ext := ".json"

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

	// Keep within interval
	for _, a := range archives {
		if now.Sub(a.Time) <= plan.keepWithin {
			keep[a.Path] = a
		}
	}

	// Keep last N archives
	for i := 0; i < len(archives) && i < plan.keepLast; i++ {
		keep[archives[i].Path] = archives[i]
	}

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
	keepInterval(plan.keepDaily, "20060102")
	keepInterval(plan.keepWeekly, "200601")
	keepInterval(plan.keepMonthly, "200601")
	keepInterval(plan.keepYearly, "2006")

	var toRemove []archive
	for _, a := range archives {
		if _, ok := keep[a.Path]; !ok {
			toRemove = append(toRemove, a)
		}
	}

	return toRemove
}

func removeArchives(toRemove []archive) error {
	for _, a := range toRemove {
		if err := os.Remove(a.Path); err != nil {
			return fmt.Errorf("remove archive %s: %w", a.Path, err)
		}
	}

	return nil
}
