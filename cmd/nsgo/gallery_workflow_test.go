package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// galleryShards are the patterns the gallery job runs, one runner each; a test must match exactly one of them.
var galleryShards = []string{"^Test[A-M]", "^Test[N-Z]"}

// galleryTests returns every test function the gallery declares.
func galleryTests(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "examples", "gallery", "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(source), "\n") {
			if !strings.HasPrefix(line, "func Test") {
				continue
			}
			names = append(names, strings.Split(strings.Fields(line)[1], "(")[0])
		}
	}
	return names
}

// Every gallery test runs in exactly one shard, so the two runners between them keep testing all of the gallery.
func TestGalleryShardsCoverEveryTest(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "test.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pattern := range galleryShards {
		if !strings.Contains(string(workflow), "'"+pattern+"'") {
			t.Errorf("the gallery job does not run %q", pattern)
		}
	}
	tests := galleryTests(t)
	if len(tests) == 0 {
		t.Fatal("no gallery tests were read")
	}
	for _, name := range tests {
		matches := 0
		for _, pattern := range galleryShards {
			if regexp.MustCompile(pattern).MatchString(name) {
				matches++
			}
		}
		if matches != 1 {
			t.Errorf("%s runs in %d shards", name, matches)
		}
	}
}

// Neither shard is empty, which a change to the patterns could otherwise do.
func TestGalleryShardsAreBothUsed(t *testing.T) {
	counts := make([]int, len(galleryShards))
	for _, name := range galleryTests(t) {
		for i, pattern := range galleryShards {
			if regexp.MustCompile(pattern).MatchString(name) {
				counts[i]++
			}
		}
	}
	for i, count := range counts {
		if count == 0 {
			t.Errorf("shard %d runs no tests", i+1)
		}
	}
}
