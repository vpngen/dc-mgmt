package main

import (
	"testing"
	"testing/fstest"
	"time"
)

func TestParseArchives(t *testing.T) {
	testCases := []struct {
		name        string
		files       map[string]string // filename: content
		tag         string
		expectedLen int
		expectErr   bool
	}{
		{
			name: "valid archives",
			files: map[string]string{
				"snap-20231026-100000.json": "",
				"snap-20231026-110000.json": "",
			},
			tag:         "snap",
			expectedLen: 2,
			expectErr:   false,
		},
		{
			name: "invalid timestamp format",
			files: map[string]string{
				"snap-20231026_100000.json": "",
			},
			tag:         "snap",
			expectedLen: 0,
			expectErr:   false,
		},
		{
			name: "mixed valid and invalid",
			files: map[string]string{
				"snap-20231026-100000.json": "",
				"snap-20231026_110000.json": "",
			},
			tag:         "snap",
			expectedLen: 1,
			expectErr:   false,
		},
		{
			name: "no matching files",
			files: map[string]string{
				"other-20231026-100000.json": "",
			},
			tag:         "snap",
			expectedLen: 0,
			expectErr:   false,
		},
		{
			name:        "empty directory",
			files:       map[string]string{},
			tag:         "snap",
			expectedLen: 0,
			expectErr:   false,
		},
		{
			name: "subdirectory",
			files: map[string]string{
				"subdir/snap-20231026-100000.json": "",
			},
			tag:         "snap",
			expectedLen: 0, // fs.WalkDir does traverse subdirectories, but forcefully skipping them
			expectErr:   false,
		},
		{
			name: "wrong extension",
			files: map[string]string{
				"snap-20231026-100000.txt": "",
			},
			tag:         "snap",
			expectedLen: 0,
			expectErr:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create an in-memory filesystem using fstest.MapFS
			fs := fstest.MapFS{}
			for filename, content := range tc.files {
				fs[filename] = &fstest.MapFile{
					Data: []byte(content),
				}
			}

			// Call parseArchives
			archives, err := parseArchives(fs, tc.tag)

			// Check for error
			if (err != nil) != tc.expectErr {
				t.Fatalf("Expected error: %v, got: %v", tc.expectErr, err)
			}

			// Check the length of the result
			if len(archives) != tc.expectedLen {
				t.Errorf("Expected %d archives, got %d, name: %s", tc.expectedLen, len(archives), tc.name)
			}
		})
	}
}

func TestRotateArchives(t *testing.T) {
	now := time.Now()

	testCases := []struct {
		name        string
		archives    []archive
		plan        rotateConfig
		expectedLen int
	}{
		{
			name: "keep within interval",
			archives: []archive{
				{Path: "a1", Time: now.Add(-time.Hour)},
				{Path: "a2", Time: now.Add(-2 * time.Hour)},
			},
			plan: rotateConfig{
				keepWithin: time.Hour * 3,
			},
			expectedLen: 0,
		},
		{
			name: "keep last N",
			archives: []archive{
				{Path: "a1", Time: now.Add(-time.Hour)},
				{Path: "a2", Time: now.Add(-2 * time.Hour)},
			},
			plan: rotateConfig{
				keepLast: 1,
			},
			expectedLen: 1,
		},
		{
			name: "keep hourly",
			archives: []archive{
				{Path: "a1", Time: now.Add(-time.Hour)},
				{Path: "a2", Time: now.Add(-2 * time.Hour)},
				{Path: "a3", Time: now.Add(-2*time.Hour - time.Minute)}, // Different hour
			},
			plan: rotateConfig{
				keepHourly: 2,
			},
			expectedLen: 1,
		},
		{
			name: "keep daily",
			archives: []archive{
				{Path: "a1", Time: now.Add(-24 * time.Hour)},
				{Path: "a2", Time: now.Add(-48 * time.Hour)},
				{Path: "a3", Time: now.Add(-49 * time.Hour)}, // Different day
			},
			plan: rotateConfig{
				keepDaily: 2,
			},
			expectedLen: 1,
		},
		{
			name: "keep weekly",
			archives: []archive{
				{Path: "a1", Time: now.AddDate(0, 0, -7)},
				{Path: "a2", Time: now.AddDate(0, 0, -14)},
				{Path: "a3", Time: now.AddDate(0, 0, -21)}, // Different week
			},
			plan: rotateConfig{
				keepWeekly: 2,
			},
			expectedLen: 1,
		},
		{
			name: "keep monthly",
			archives: []archive{
				{Path: "a1", Time: now.AddDate(0, -1, 0)},
				{Path: "a2", Time: now.AddDate(0, -2, 0)},
				{Path: "a3", Time: now.AddDate(0, -3, 0)}, // Different month
			},
			plan: rotateConfig{
				keepMonthly: 2,
			},
			expectedLen: 1,
		},
		{
			name: "keep yearly",
			archives: []archive{
				{Path: "a1", Time: now.AddDate(-1, 0, 0)},
				{Path: "a2", Time: now.AddDate(-2, 0, 0)},
				{Path: "a3", Time: now.AddDate(-3, 0, 0)}, // Different year
			},
			plan: rotateConfig{
				keepYearly: 2,
			},
			expectedLen: 1,
		},
		{
			name: "combination",
			archives: []archive{
				{Path: "a1", Time: now.Add(-time.Hour)},
				{Path: "a2", Time: now.Add(-2 * time.Hour)},
				{Path: "a3", Time: now.Add(-24 * time.Hour)},
			},
			plan: rotateConfig{
				keepWithin: time.Hour * 3,
				keepLast:   1,
				keepDaily:  1,
			},
			expectedLen: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			toRemove := rotateArchives(tc.archives, tc.plan)

			if len(toRemove) != tc.expectedLen {
				t.Errorf("Expected %d to remove, got %d, name: %s", tc.expectedLen, len(toRemove), tc.name)
			}
		})
	}
}
