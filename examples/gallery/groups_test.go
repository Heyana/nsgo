package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGroupBuildsCompile builds the gallery as the site does for one group of
// each tab with -tags=gallery_group,gallery_<group>, so a group build that
// names no demo, or one its tab cannot show, fails here rather than in the
// site's build.
func TestGroupBuildsCompile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range []string{"apps", "primitives"} {
		t.Run(group, func(t *testing.T) {
			cmd := exec.Command("go", "build", "-tags", "gallery_group,gallery_"+group, "-o", filepath.Join(t.TempDir(), "gallery"), ".")
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("go build -tags gallery_group,gallery_%s: %v\n%s", group, err, out)
			}
		})
	}
}
