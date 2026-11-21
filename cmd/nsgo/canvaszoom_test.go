package main

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The regular command suite must exercise real browser pixels, not only a canvas's backing-store dimensions.
func TestWebCanvasScrollZoom(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs canvas zoom tests in a browser")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	for _, tc := range []struct {
		name, dir, run string
		cases          int
	}{
		{"pixels", filepath.Join("testdata", "canvaszoom"), "", 7},
		{"lifecycle", filepath.Join("..", "..", "internal", "platform", "web"), "^TestCanvasScrollZoom", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			p, err := loadForTest(tc.dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			report := filepath.Join(t.TempDir(), "tests.xml")
			if err := runTests(ctx, p, testOptions{target: "web", run: tc.run, junit: report, timeout: time.Minute}); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(report)
			if err != nil {
				t.Fatal(err)
			}
			var suites junitSuites
			if err := xml.Unmarshal(data, &suites); err != nil {
				t.Fatal(err)
			}
			if suites.Tests != tc.cases || suites.Failures != 0 {
				t.Fatalf("browser results: %s", data)
			}
			for _, suite := range suites.Suites {
				for _, c := range suite.Cases {
					if c.Skipped != nil {
						t.Fatalf("canvas zoom test skipped: %+v", c.Skipped)
					}
				}
			}
		})
	}
}
